/*
Copyright 2019 Gravitational, Inc.

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

// Package nethealth implements a daemonset that when deployed to a kubernetes cluster, will locate and send ICMP echos
// (pings) to the nethealth pod on every other node in the cluster. This will give an indication into whether the
// overlay network is functional for pod -> pod communications, and also record packet loss on the network.
package nethealth

import (
	"fmt"
	"net"
	"net/http"
	"reflect"
	"sort"
	"time"

	"github.com/gravitational/satellite/utils"
	"github.com/gravitational/trace"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	// heartbeatInterval is the duration between sending heartbeats to each peer. Any heartbeat that takes more
	// than one interval to respond will also be considered timed out.
	heartbeatInterval = 1 * time.Second

	// resyncInterval is the duration between full resyncs of local state with kubernetes. If a node is deleted it
	// may not be detected until the full resync completes.
	resyncInterval = 15 * time.Minute

	// dnsDiscoveryInterval is the duration of time for doing DNS based service discovery for pod changes. This is a
	// lightweight test for whether there is a change to the nethealth pods within the cluster.
	dnsDiscoveryInterval = 10 * time.Second

	// DefaultPrometheusPort is the default port to serve prometheus metrics
	DefaultPrometheusPort = 9801

	// DefaultNamespace is the default namespace to search for nethealth resources
	DefaultNamespace = "monitoring"

	// Default selector to use for finding nethealth pods
	DefaultSelector = "k8s-app=nethealth"

	// DefaultServiceDiscoveryQuery is the default name to query for service discovery changes
	DefaultServiceDiscoveryQuery = "any.nethealth"
)

const (
	// Init is peer state that we've found the node but don't know anything about it yet.
	Init = "init"
	// Up is a peer state that the peer is currently reachable
	Up = "up"
	// Timeout is a peer state that the peer is currently timing out to pings
	Timeout = "timeout"
)

type Config struct {
	// PrometheusPort is the port to bind to for serving prometheus metrics
	PrometheusPort uint32
	// Namespace is the kubernetes namespace to monitor for other nethealth instances
	Namespace string
	// HostIP is the host IP address
	HostIP string
	// Selector is a kubernetes selector to find all the nethealth pods in the configured namespace
	Selector string
	// ServiceDiscoveryQuery is a DNS name that will be used for lightweight service discovery checks. A query to
	// any.<service>.default.svc.cluster.local will return a list of pods for the service. If the list of pods
	// changes we know to resync with the kubernetes API. This method uses significantly less resources than running a
	// kubernetes watcher on the API. Defaults to any.nethealth which will utilize the search path from resolv.conf.
	ServiceDiscoveryQuery string
}

// New creates a new server to ping each peer.
func (c Config) New() (*Server, error) {

	promPeerRTT := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "nethealth",
		Subsystem: "echo",
		Name:      "duration_seconds",
		Help:      "The round trip time to reach the peer",
		Buckets: []float64{
			0.0001, // 0.1 ms
			0.0002, // 0.2 ms
			0.0003, // 0.3 ms
			0.0004, // 0.4 ms
			0.0005, // 0.5 ms
			0.0006, // 0.6 ms
			0.0007, // 0.7 ms
			0.0008, // 0.8 ms
			0.0009, // 0.9 ms
			0.001,  // 1ms
			0.0015, // 1.5ms
			0.002,  // 2ms
			0.003,  // 3ms
			0.004,  // 4ms
			0.005,  // 5ms
			0.01,   // 10ms
			0.02,   // 20ms
			0.04,   // 40ms
			0.08,   // 80ms
		},
	}, []string{"node_name", "peer_name"})
	promPeerTimeout := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "nethealth",
		Subsystem: "echo",
		Name:      "timeout_total",
		Help:      "The number of echo requests that have timed out",
	}, []string{"node_name", "peer_name"})
	promPeerRequest := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "nethealth",
		Subsystem: "echo",
		Name:      "request_total",
		Help:      "The number of echo requests that have been sent",
	}, []string{"node_name", "peer_name"})

	prometheus.MustRegister(
		promPeerRTT,
		promPeerTimeout,
		promPeerRequest,
	)

	if c.HostIP == "" {
		return nil, trace.BadParameter("host ip must be provided")
	}

	selector := DefaultSelector
	if c.Selector != "" {
		selector = c.Selector
	}

	labelSelector, err := labels.Parse(selector)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	if c.ServiceDiscoveryQuery == "" {
		c.ServiceDiscoveryQuery = DefaultServiceDiscoveryQuery
	}

	if c.Namespace == "" {
		c.Namespace = DefaultNamespace
	}

	return &Server{
		config:          c,
		FieldLogger:     logrus.WithField(trace.Component, "nethealth"),
		promPeerRTT:     promPeerRTT,
		promPeerTimeout: promPeerTimeout,
		promPeerRequest: promPeerRequest,
		selector:        labelSelector,
		triggerResync:   make(chan bool, 1),
		rxMessage:       make(chan messageWrapper, 100),
		peers:           make(map[string]*peer),
		podToHost:       make(map[string]string),
	}, nil
}

// Server is an instance of nethealth that is running on each node responsible for sending and responding to heartbeats.
type Server struct {
	logrus.FieldLogger

	config     Config
	clock      clockwork.Clock
	conn       *icmp.PacketConn
	httpServer *http.Server
	selector   labels.Selector

	// rxMessage is a processing queue of received echo responses
	rxMessage     chan messageWrapper
	triggerResync chan bool

	// peers maps the peer's host IP to peer data.
	peers map[string]*peer
	// podToHost maps the nethealth pod IP to the node host IP.
	podToHost map[string]string

	client kubernetes.Interface

	promPeerRTT     *prometheus.HistogramVec
	promPeerTimeout *prometheus.CounterVec
	promPeerRequest *prometheus.CounterVec
}

type peer struct {
	hostIP      string
	podAddr     net.Addr
	echoCounter int
	echoTime    time.Time
	echoTimeout bool

	status           string
	lastStatusChange time.Time
}

type messageWrapper struct {
	message *icmp.Message
	rxTime  time.Time
	podAddr net.Addr
}

// Start sets up the server and begins normal operation
func (s *Server) Start() error {
	config, err := rest.InClusterConfig()
	if err != nil {
		return trace.Wrap(err)
	}
	s.client, err = kubernetes.NewForConfig(config)
	if err != nil {
		return trace.Wrap(err)
	}

	s.conn, err = icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		return trace.Wrap(err)
	}

	s.clock = clockwork.NewRealClock()
	go s.loop()
	go s.loopServiceDiscovery()
	go s.serve()

	mux := http.ServeMux{}
	mux.Handle("/metrics", promhttp.Handler())
	s.httpServer = &http.Server{Addr: fmt.Sprint(":", s.config.PrometheusPort), Handler: &mux}
	go func() {
		if err := s.httpServer.ListenAndServe(); err != http.ErrServerClosed {
			s.Fatalf("ListenAndServe(): %s", err)
		}
	}()

	s.Info("Started nethealth with config:")
	s.Info("  PrometheusPort: ", s.config.PrometheusPort)
	s.Info("  Namespace: ", s.config.Namespace)
	s.Info("  HostIP: ", s.config.HostIP)
	s.Info("  Selector: ", s.selector)
	s.Info("  ServiceDiscoveryQuery: ", s.config.ServiceDiscoveryQuery)

	return nil
}

// loop is the main processing loop for sending/receiving heartbeats.
func (s *Server) loop() {
	heartbeatTicker := s.clock.NewTicker(heartbeatInterval)
	defer heartbeatTicker.Stop()

	resyncTicker := s.clock.NewTicker(resyncInterval)
	defer resyncTicker.Stop()

	for {
		select {
		//
		// Re-sync cluster peers
		//
		case <-resyncTicker.Chan():
			if err := s.resyncPeerList(); err != nil {
				s.WithError(err).Error("Unexpected error re-syncing the list of peer nodes.")
			}

		case <-s.triggerResync:
			if err := s.resyncPeerList(); err != nil {
				s.WithError(err).Error("Unexpected error re-syncing the list of peer nodes.")
			}

		//
		// Send a heartbeat to each peer we know about
		// Check for peers that are timing out / down
		//
		case <-heartbeatTicker.Chan():
			s.checkTimeouts()
			for _, peer := range s.peers {
				s.sendHeartbeat(peer)
			}

		//
		// Rx heartbeats responses from peers
		//
		case rx := <-s.rxMessage:
			err := s.processAck(rx)
			if err != nil {
				s.WithFields(logrus.Fields{
					logrus.ErrorKey: err,
					"pod_addr":      rx.podAddr,
					"rx_time":       rx.rxTime,
					"message":       rx.message,
				}).Error("Error processing icmp message.")
			}
		}
	}
}

// loopServiceDiscovery uses cluster-dns service discovery as a lightweight check for pod changes
// and will trigger a resync if the cluster DNS service discovery changes
func (s *Server) loopServiceDiscovery() {
	s.Info("Starting DNS service discovery for nethealth pod.")
	ticker := s.clock.NewTicker(dnsDiscoveryInterval)
	defer ticker.Stop()

	query := s.config.ServiceDiscoveryQuery
	previousNames := []string{}

	for {
		<-ticker.Chan()

		s.Debugf("Querying %v for service discovery", query)
		names, err := net.LookupHost(query)
		if err != nil {
			s.WithError(err).WithField("query", query).Error("Error querying service discovery.")
			continue
		}

		sort.Strings(names)
		if reflect.DeepEqual(names, previousNames) {
			continue
		}
		previousNames = names
		s.Info("Triggering peer resync due to service discovery change")

		select {
		case s.triggerResync <- true:
		default:
			// Don't block
		}
	}
}

// resyncPeerList contacts the kubernetes API to sync the list of kubernetes nodes
func (s *Server) resyncPeerList() error {
	pods, err := s.client.CoreV1().Pods(s.config.Namespace).List(metav1.ListOptions{
		LabelSelector: s.selector.String(),
	})
	if err != nil {
		return utils.ConvertError(err)
	}

	s.resyncNethealth(pods.Items)
	return nil
}

func (s *Server) resyncNethealth(pods []corev1.Pod) {
	// keep track of current peers so we can prune nodes that are no longer part of the cluster
	peerMap := make(map[string]bool)
	for _, pod := range pods {
		// Don't add our own node as a peer
		if pod.Status.HostIP == s.config.HostIP {
			continue
		}

		peerMap[pod.Status.HostIP] = true
		if peer, ok := s.peers[pod.Status.HostIP]; ok {
			newAddr := &net.IPAddr{
				IP: net.ParseIP(pod.Status.PodIP),
			}
			if peer.podAddr.String() == newAddr.String() {
				s.WithFields(logrus.Fields{
					"host_ip":      peer.hostIP,
					"new_pod_addr": newAddr,
					"old_pod_addr": peer.podAddr,
				}).Info("Updating peer pod IP address.")
				s.podToHost[pod.Status.PodIP] = pod.Status.HostIP // update pod to host mapping
			}
			continue
		}

		// Initialize new peers
		s.peers[pod.Status.HostIP] = &peer{
			hostIP:           pod.Status.HostIP,
			podAddr:          &net.IPAddr{IP: net.ParseIP(pod.Status.PodIP)},
			lastStatusChange: s.clock.Now(),
		}
		s.podToHost[pod.Status.PodIP] = pod.Status.HostIP
		s.WithField("peer", pod.Status.HostIP).Info("Adding peer.")

		// Initialize the peer so it shows up in prometheus with a 0 count
		s.promPeerTimeout.WithLabelValues(s.config.HostIP, pod.Status.HostIP).Add(0)
		s.promPeerRequest.WithLabelValues(s.config.HostIP, pod.Status.HostIP).Add(0)
	}

	// check for peers that have been deleted
	for _, peer := range s.peers {
		if _, ok := peerMap[peer.hostIP]; !ok {
			s.WithField("peer", peer.hostIP).Info("Deleting peer.")
			delete(s.peers, peer.hostIP)
			delete(s.podToHost, peer.podAddr.String())

			s.promPeerRTT.DeleteLabelValues(s.config.HostIP, peer.hostIP)
			s.promPeerRequest.DeleteLabelValues(s.config.HostIP, peer.hostIP)
			s.promPeerTimeout.DeleteLabelValues(s.config.HostIP, peer.hostIP)
		}
	}
}

// serve monitors for incoming icmp messages
func (s *Server) serve() {
	buf := make([]byte, 256)

	for {
		n, podAddr, err := s.conn.ReadFrom(buf)
		rxTime := s.clock.Now()
		log := s.WithFields(logrus.Fields{
			"pod_addr": podAddr,
			"length":   n,
		})
		if err != nil {
			log.WithError(err).Error("Error in udp socket read.")
			continue
		}

		// The ICMP package doesn't export the protocol numbers
		// 1 - ICMP
		// 58 - ICMPv6
		// https://www.iana.org/assignments/protocol-numbers/protocol-numbers.xhtml
		msg, err := icmp.ParseMessage(1, buf[:n])
		if err != nil {
			log.WithError(err).Error("Error parsing icmp message.")
			continue
		}

		select {
		case s.rxMessage <- messageWrapper{
			message: msg,
			rxTime:  rxTime,
			podAddr: podAddr,
		}:
		default:
			// Don't block
			log.Warn("Dropped icmp message due to full rxMessage queue")
		}
	}
}

func (s *Server) lookupPeer(podIP string) (*peer, error) {
	hostIP, ok := s.podToHost[podIP]
	if !ok {
		return nil, trace.BadParameter("address not found in address table").AddField("address", podIP)
	}

	p, ok := s.peers[hostIP]
	if !ok {
		return nil, trace.BadParameter("peer not found in peer table").AddField("host_ip", hostIP)
	}
	return p, nil
}

// processAck processes a received ICMP Ack message
func (s *Server) processAck(e messageWrapper) error {
	switch e.message.Type {
	case ipv4.ICMPTypeEchoReply:
		// ok
	case ipv4.ICMPTypeEcho:
		// nothing to do with echo requests
		return nil
	default:
		//unexpected / unknown
		return trace.BadParameter("received unexpected icmp message type").AddField("type", e.message.Type)
	}

	switch pkt := e.message.Body.(type) {
	case *icmp.Echo:
		peer, err := s.lookupPeer(e.podAddr.String())
		if err != nil {
			return trace.Wrap(err)
		}
		if uint16(pkt.Seq) != uint16(peer.echoCounter) {
			return trace.BadParameter("response sequence doesn't match latest request.").
				AddField("expected", uint16(peer.echoCounter)).
				AddField("received", uint16(pkt.Seq))
		}

		rtt := e.rxTime.Sub(peer.echoTime)
		s.promPeerRTT.WithLabelValues(s.config.HostIP, peer.hostIP).Observe(rtt.Seconds())
		s.updatePeerStatus(peer, Up)
		peer.echoTimeout = false

		s.WithFields(logrus.Fields{
			"peer_ip":  peer.hostIP,
			"pod_addr": peer.podAddr,
			"counter":  peer.echoCounter,
			"seq":      uint16(peer.echoCounter),
			"rtt":      rtt,
		}).Debug("Ack.")
	default:
		s.WithFields(logrus.Fields{
			"peer_addr": e.podAddr.String(),
		}).Warn("Unexpected icmp message")
	}
	return nil
}

func (s *Server) sendHeartbeat(peer *peer) {
	peer.echoCounter++
	log := s.WithFields(logrus.Fields{
		"peer_ip":  peer.hostIP,
		"pod_addr": peer.podAddr,
		"id":       peer.echoCounter,
	})

	if peer.podAddr == nil || peer.podAddr.String() == "" || peer.podAddr.String() == "0.0.0.0" {
		return
	}

	msg := icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Code: 0,
		Body: &icmp.Echo{
			ID:  1,
			Seq: peer.echoCounter,
		},
	}
	buf, err := msg.Marshal(nil)
	if err != nil {
		log.WithError(err).Warn("Failed to marshal ping.")
		return
	}

	peer.echoTime = s.clock.Now()
	_, err = s.conn.WriteTo(buf, peer.podAddr)
	if err != nil {
		log.WithError(err).Warn("Failed to send ping.")
		return
	}
	s.promPeerRequest.WithLabelValues(s.config.HostIP, peer.hostIP).Inc()
	peer.echoTimeout = true

	log.Debug("Sent echo request.")
}

// checkTimeouts iterates over each peer, and checks whether our last heartbeat has timed out
func (s *Server) checkTimeouts() {
	s.Debug("checking for timeouts")
	for _, peer := range s.peers {
		// if the echoTimeout flag is set, it means we didn't receive a response to our last request
		if peer.echoTimeout {
			s.WithFields(logrus.Fields{
				"peer_ip":  peer.hostIP,
				"pod_addr": peer.podAddr,
				"id":       peer.echoCounter,
			}).Debug("echo timeout")
			s.promPeerTimeout.WithLabelValues(s.config.HostIP, peer.hostIP).Inc()
			s.updatePeerStatus(peer, Timeout)
		}
	}
}

func (s *Server) updatePeerStatus(peer *peer, status string) {
	if peer.status == status {
		return
	}

	s.WithFields(logrus.Fields{
		"peer_ip":    peer.hostIP,
		"pod_addr":   peer.podAddr,
		"duration":   s.clock.Now().Sub(peer.lastStatusChange),
		"old_status": peer.status,
		"new_status": status,
	}).Info("Peer status changed.")

	peer.status = status
	peer.lastStatusChange = s.clock.Now()
}
