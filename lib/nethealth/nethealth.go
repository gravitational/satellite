/*
Copyright 2016 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

/*
Nethealth is a library designed for testing the network health between nodes in a cluster.

Notes:
- It's designed to be deployed asa DaemonSet within a kubernetes cluster
- It will use DNS service discovery, to discover other pods within the DaemonSet
  - Errors in the service discovery will be available as prometheus metrics
- It will periodically send heartbeats to the other pods
  - A lack of heartbeats will indicate whether the node is up or down
  - An embedded sequence number, will allow estimation of packet loss between nodes
  - Embedded timestamps will allow RTT estimations
*/
package nethealth

import (
	"fmt"
	"math/rand"
	"net"
	"time"

	"github.com/jonboulle/clockwork"

	"github.com/davecgh/go-spew/spew"

	"github.com/gogo/protobuf/proto"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/trace"
)

const (
	// pingInterval is how often we should send pings in seconds, and any ping that takes longer than this interval
	// will be considered timed out
	pingInterval = 1 * time.Second
	// syncInterval is how often we should sync service discovery information with the cluster
	syncInterval = 5 * time.Second
)

var (
	log = logrus.WithField(trace.Component, "nethealth")

	promPacketLoss = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "nethealth",
		Subsystem: "heartbeat",
		Name:      "lost",
		Help:      "Number of heartbeats lost from peer to local node",
	}, []string{"peer_name"})

	promPacketFiltered = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "nethealth",
		Subsystem: "heartbeat",
		Name:      "filtered",
		Help:      "The number of heartbeats filtered due to not being within allowed network range",
	}, []string{"peer_addr"})

	promPacketRTT = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "newhealth",
		Subsystem: "heartbeat",
		Name:      "rtt",
		Help:      "The round trip time to reach the peer",
	}, []string{"peer_name"})

	promPeerDown = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "nethealth",
		Subsystem: "heartbeat",
		Name:      "peer_down",
		Help:      "Indicates whether the peer is down. 1=Down, 0=Up",
	}, []string{"peer_name"})

	promDNSError = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "nethealth",
		Subsystem: "dns",
		Name:      "error",
		Help:      "indicates if we're encountering an error with cluster service discovery",
	}, []string{"error"})
)

func init() {
	prometheus.MustRegister(
		promPacketLoss,
		promPacketFiltered,
		promPacketRTT,
		promPeerDown,
		promDNSError,
	)
}

// Server is a simple server, that binds to a port, and will answer any pings that come from within the cluster
type Server struct {
	// AllowedSubnets is a list of subnets to accept packets on
	AllowedSubnets []net.IPNet
	// BindPort is the port to bind the server to
	BindPort uint32
	// ServiceName is the DNS record name to query to discovery other pods in the cluster
	ServiceName string
	// NodeName is the name of the cluster node this pod is running on
	NodeName string
	// PodIP is the IP address of the local pod
	PodIP string

	clock clockwork.Clock
	conn  net.PacketConn
	stopC chan bool

	// rxHeartbeat is a processing queue of received heartbeats
	rxHeartbeat chan heartbeat
	// podList is a processing queue of pod discovery updates
	podList chan []string

	peers map[string]*peer
}

type peer struct {
	addr     net.Addr
	nodeName string
	// lastRxHeartbeat is the timestamp of the last rx heartbeat from the peer
	lastRxHeartbeat time.Time

	localID      string
	localCounter int64

	remoteID      string
	remoteCounter int64
}

type heartbeat struct {
	*agentpb.Heartbeat
	rxTime   time.Time
	peerAddr net.Addr
}

func (s *Server) ListenAndServe() error {
	var err error
	s.conn, err = net.ListenPacket("udp", fmt.Sprint(":", s.BindPort))
	if err != nil {
		return trace.Wrap(err)
	}
	s.clock = clockwork.NewRealClock()
	go s.syncServiceDiscovery()
	go s.loop()

	return nil
}

func (s *Server) Stop() error {
	close(s.stopC)
	return trace.Wrap(s.conn.Close())
}

func (s *Server) loop() {
	ticker := s.clock.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {

		//
		// Stop the node
		//
		case <-s.stopC:
			log.Debug("Dispatch stop request")
			break

		//
		// Rx heartbeats from peers
		//
		case env := <-s.rxHeartbeat:
			log.Debugf("Dispatch rx heartbeat.: %v", spew.Sdump(env))
			switch env.GetType() {
			case agentpb.Heartbeat_REQUEST:
				s.processHeartbeat(env)
			case agentpb.Heartbeat_ACK:
				s.processAck(env)
			case agentpb.Heartbeat_UNKNOWN:
				log.WithFields(logrus.Fields{
					"node":      s.NodeName,
					"peer_addr": env.peerAddr,
				}).Error("Message received with unknown message type.")
			default:
				log.WithFields(logrus.Fields{
					"node":      s.NodeName,
					"peer_addr": env.peerAddr,
					"type":      env.GetType(),
				}).Error("Message received with unsupported type.")
			}

		//
		// Rx service discovery updates on pods in cluster
		//
		case pods := <-s.podList:
			log.Debug("Dispatch pod update.")
			s.updatePods(pods)

		//
		// Send a heartbeat to each peer we know about
		// Check for peers that are timing out / down
		//
		case <-ticker.Chan():
			log.Debug("Dispatch ticker.")
			for _, peer := range s.peers {
				s.sendPing(peer)
			}
			s.timeouts()
		}
	}

	err := s.conn.Close()
	if err != nil {
		log.Info("Error closing listening socket: ", err)
	}
}

func (s *Server) serve() {
	for {
		select {
		case <-s.stopC:
			return
		default:
		}

		buf := make([]byte, 256)
		n, peerAddr, err := s.conn.ReadFrom(buf)
		log.WithFields(logrus.Fields{
			"node":      s.NodeName,
			"n":         n,
			"peer_addr": peerAddr,
			"err":       err,
		}).Debug("read message on udp connection")
		if err != nil {
			log.WithFields(logrus.Fields{
				"node":      s.NodeName,
				"n":         n,
				"peer_addr": peerAddr,
				"err":       err,
			}).Error("error in udp socket read")
			continue
		}

		rxTime := s.clock.Now()

		// don't do any processing and discard packets with src addr not in allowed ranges
		if !s.allowed(peerAddr) {
			log.WithFields(logrus.Fields{
				"node":      s.NodeName,
				"peer_addr": peerAddr,
				"allowed":   s.AllowedSubnets,
			}).Info("Packet dropped due to filter.")
			promPacketFiltered.WithLabelValues(peerAddr.String()).Inc()
			continue
		}

		msg, err := unmarshalHeartbeat(buf[:n])
		if err != nil {
			log.WithFields(logrus.Fields{
				"node":      s.NodeName,
				"peer_addr": peerAddr,
				"error":     err,
				"buffer":    buf[:n],
			}).Error("Failed to unmarshal message")
			continue
		}
		log.WithFields(logrus.Fields{
			"node":      s.NodeName,
			"peer_addr": peerAddr,
			"msg":       msg,
		}).Debug("Rx heartbeat.")

		// Respond to the heartbeat in the reader routine, so that we respond as quickly as possible
		// as this is used to calculate network latency
		if msg.GetType() == agentpb.Heartbeat_REQUEST {
			r := agentpb.Heartbeat{
				Type:      agentpb.Heartbeat_ACK,
				NodeName:  s.NodeName,
				Timestamp: msg.Timestamp,
			}
			s.sendHeartbeat(r, peerAddr)
		}

		// Queue the message to an internal processing thread
		select {
		case s.rxHeartbeat <- heartbeat{
			Heartbeat: msg,
			rxTime:    rxTime,
			peerAddr:  peerAddr,
		}:
			log.WithField("node", s.NodeName).Info("sent to rxChan")
		default:
			log.WithFields(logrus.Fields{
				"node":      s.NodeName,
				"peer_addr": peerAddr,
				"msg":       msg,
				"error":     err,
			}).Info("Dropped rx message, channel is full.")
		}
	}
}

func (s *Server) sendHeartbeat(h agentpb.Heartbeat, addr net.Addr) {
	log := log.WithFields(logrus.Fields{
		"heartbeat": h,
		"dst_addr":  addr,
	})

	resp, err := h.Marshal()
	if err != nil {
		log.WithError(err).Error("Failed to marshal heartbeat.")
	}

	_, err = s.conn.WriteTo(resp, addr)
	if err != nil {
		log.WithError(err).Error("Failed to send heartbeat.")
	}
}

func unmarshalHeartbeat(buf []byte) (*agentpb.Heartbeat, error) {
	msg := &agentpb.Heartbeat{}
	err := trace.Wrap(proto.Unmarshal(buf, msg))

	return msg, err
}

// timeouts iterates over each peer, and checks whether we've timed out to each node
func (s *Server) timeouts() {
	log.Info("checking for timeouts")
	for _, peer := range s.peers {
		if peer.nodeName == "" {
			// skip if we haven't discovered the node name of the peer yet
			continue
		}
		// if the last time we received a ping was more than 10 intervals ago (10 seconds) consider the peer down
		lastPingDelta := s.clock.Now().Sub(peer.lastRxHeartbeat).Seconds()
		if lastPingDelta > 10*pingInterval.Seconds() {
			promPeerDown.WithLabelValues(peer.nodeName).Set(1) // 1 = down
			log.WithFields(logrus.Fields{
				"node":      s.NodeName,
				"peer_name": peer.nodeName,
				"peer_ip":   peer.addr.String(),
			}).Debug("peer is down")
			continue
		}

		promPeerDown.WithLabelValues(peer.nodeName).Set(0) // 0 = up
		log.WithFields(logrus.Fields{
			"node":      s.NodeName,
			"peer_name": peer.nodeName,
			"peer_ip":   peer.addr.String(),
		}).Debug("peer is up")

	}
}

func (s *Server) sendPing(peer *peer) {
	peer.localCounter++
	log.WithFields(logrus.Fields{
		"node":       s.NodeName,
		"peer_name":  peer.nodeName,
		"peer_addr":  peer.addr,
		"count":      peer.localCounter,
		"restart_id": peer.localID,
	}).Debug("Send heartbeat request.")
	request := agentpb.Heartbeat{
		Timestamp: s.clock.Now().UTC().Format(time.RFC3339Nano),
		Type:      agentpb.Heartbeat_REQUEST,
		RestartId: peer.localID,
		Count:     peer.localCounter,
		NodeName:  s.NodeName,
	}
	s.sendHeartbeat(request, peer.addr)
}

// processHeartbeat processes a rx ping message
func (s *Server) processHeartbeat(e heartbeat) {
	if peer, ok := s.peers[e.peerAddr.String()]; ok {
		peer.lastRxHeartbeat = e.rxTime

		peer := s.peers[e.peerAddr.String()]
		if peer.nodeName != e.NodeName {
			log.Info("Updated peer name")
			peer.nodeName = e.NodeName
		}

		// if the restart counter has changed, reset our internal state
		if peer.remoteID != e.RestartId {
			peer.remoteID = e.RestartId
			peer.remoteCounter = e.Count - 1
			log.WithFields(logrus.Fields{
				"remote_id":      peer.remoteID,
				"remote_counter": peer.remoteCounter,
			}).Debug("Updated restart counter")
		}

		// check for packet loss
		lost := e.Count - peer.remoteCounter
		switch {
		case lost == 0:
			log.Error("RX duplicate heartbeat.")
		case lost < 0:
			log.Error("RX old heartbeat.")
		case lost > 1 && lost < 5:
			promPacketLoss.WithLabelValues(peer.nodeName).Add(float64(lost - 1))
			// lost == 1 is the expected case
		}
		peer.remoteCounter = e.Count
		return
	}
	log.WithField("peer_addr", e.peerAddr.String()).Info("rx ping from unknown peer")
}

// processAck processes a rx ack message
func (s *Server) processAck(e heartbeat) error {
	log := log.WithFields(logrus.Fields{
		"node":      s.NodeName,
		"peer_addr": e.peerAddr.String(),
		"peer_name": e.NodeName,
	})

	log.Debug("processAck: ", spew.Sdump(e))
	t, err := time.Parse(time.RFC3339Nano, e.Timestamp)
	if err != nil {
		return trace.Wrap(err)
	}
	delta := e.rxTime.Sub(t)

	peer := s.peers[e.peerAddr.String()]
	if peer.nodeName != e.NodeName {
		peer.nodeName = e.NodeName
	}
	if peer.nodeName != "" {
		promPacketRTT.WithLabelValues(peer.nodeName).Observe(delta.Seconds())
	}
	return nil
}

// allowed indicated whether the rx ping is from within an allowed IP range
func (s *Server) allowed(peer net.Addr) bool {
	for _, subnet := range s.AllowedSubnets {
		if subnet.Contains(net.ParseIP(peer.String())) {
			return true
		}
	}
	return false
}

func (s *Server) syncServiceDiscovery() {
	ticker := s.clock.NewTicker(syncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopC:
			return
		case <-ticker.Chan():
			recs, err := net.LookupHost(s.ServiceName)
			if err != nil {
				promDNSError.Reset()
				promDNSError.WithLabelValues(err.Error()).Set(1)
				log.Error("DNS resolution error: ", err)
				continue
			}

			s.podList <- recs
		}
	}
}

func (s *Server) updatePods(addr []string) {
	if s.peers == nil {
		s.peers = make(map[string]*peer, 0)
	}
	podMap := make(map[string]bool)
	for _, a := range addr {
		// don't add our own node as a peer
		if a == s.conn.LocalAddr().String() {
			continue
		}

		podMap[a] = true
		if _, ok := s.peers[a]; !ok {
			s.peers[a] = &peer{
				addr: &net.IPAddr{
					IP: net.ParseIP(a),
				},
				localID: fmt.Sprintf("%v:%v", rand.Uint64(), rand.Uint64()), // set to a non standardized 128bit random number
			}
			log.WithField("node", s.NodeName).Infof("Peer added. PodIP: %v", a)
		}
	}

	// check for pods that have been deleted
	for key, peer := range s.peers {
		if _, ok := podMap[key]; !ok {
			// the pod has been deleted
			promPeerDown.WithLabelValues(peer.nodeName).Set(1)
			log.WithField("node", s.NodeName).Infof("Peer removed. PodIP: %v Node: %v", key, peer.nodeName)
		}
	}
}
