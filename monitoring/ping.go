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

package monitoring

import (
	"context"
	"sync"
	"time"

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"

	"github.com/codahale/hdrhistogram"
	"github.com/gravitational/trace"
	"github.com/gravitational/ttlmap"
	serf "github.com/hashicorp/serf/client"
	log "github.com/sirupsen/logrus"
)

const (
	// pingCheckerID specifies the check name
	pingCheckerID = "ping-checker"
	// slidingWindowSize specifies the number of retained check results
	slidingWindowSize = 10
	// statsTTLPeriod specifies how long check results will be kept before being dropped
	statsTTLPeriod = 1 * time.Hour
	// pingMinimum sets the minimum value that can be recorded
	pingMinimum = 0 * time.Second
	// pingMaximum sets the maximum value that can be recorded
	pingMaximum = 10 * time.Second
	// pingSignificantFigures specifies how many decimals should be recorded
	pingSignificantFigures = 3
	// latencyThreshold sets the RTT threshold
	latencyThreshold = 15 * time.Millisecond
	// latencyQuantile sets the quantile used while checking Histograms against RTT results
	latencyQuantile = 95.0
)

// NewPingChecker returns a checker that verifies accessibility of nodes in the cluster by exchanging ping requests
func NewPingChecker(serfRPCAddr string, serfMemberName string) (c health.Checker, err error) {
	latencyTTLMap, err := ttlmap.New(int(statsTTLPeriod.Seconds())) //FIXME: why is number of seconds used as capacity?
	if err != nil {
		return nil, trace.Wrap(err)
	}

	log.WithFields(log.Fields{trace.Component: "ping"})
	log.Debugf("using Serf IP: %v", serfRPCAddr)
	log.Debugf("using Serf Name: %v", serfMemberName)
	// fetch serf config and instantiate client
	clientConfig := serf.Config{
		Addr: serfRPCAddr,
	}
	client, err := serf.ClientFromConfig(&clientConfig)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	// FIXME: defer client.Close() # cannot be used here or causes error: ping-checker (set tcp 127.0.0.1:52026: use of closed network connection)

	// retrieve other nodes using Serf members
	nodes, err := client.Members()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	// finding what is the current node
	var self serf.Member
	for _, node := range nodes {
		if node.Name == serfMemberName {
			self = node
		}
	}
	if self.Name == "" {
		return nil, trace.NotFound("failed to find Serf member with name %s", serfMemberName)
	}

	return &pingChecker{
		self:           self,
		serfClient:     *client,
		serfMemberName: serfMemberName,
		latencyStats:   *latencyTTLMap,
	}, nil
}

// pingChecker is a checker that verifies that ping times (RTT) between nodes in
// the cluster are within a predefined threshold
type pingChecker struct {
	self           serf.Member
	serfClient     serf.RPCClient
	serfMemberName string
	latencyStats   ttlmap.TTLMap
	mux            sync.Mutex
}

// Name returns the checker name
// Implements health.Checker
func (c *pingChecker) Name() string {
	return pingCheckerID
}

// Check verifies that all nodes' ping with Master Nodes is lower than the
// desired threshold
// Implements health.Checker
func (c *pingChecker) Check(ctx context.Context, r health.Reporter) {
	// FIXME: #1 RTTThreshold will become configurable in future
	// FIXME: #2 Send RTTThreshold value to metrics

	err := c.check(ctx, r)

	if err != nil {
		c.setProbeStatus(ctx, r, err, pb.Probe_Failed)
		return
	}
	c.setProbeStatus(ctx, r, nil, pb.Probe_Running)

	return
}

// check runs the actual system status verification code and returns an error
// in case issues arise in the process
func (c *pingChecker) check(ctx context.Context, r health.Reporter) error {

	client := &c.serfClient
	if client == nil {
		return trace.NotFound("serf client not initialized yet")
	}

	// retrieve other nodes using Serf members
	nodes, err := client.Members()
	if err != nil {
		return trace.Wrap(err)
	}

	err = c.checkNodesRTT(nodes, client)
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}

// checkNodesRTT implements the bulk of the logic by checking the ping time
// between this node (self) and the other Serf Cluster member nodes
func (c *pingChecker) checkNodesRTT(nodes []serf.Member, client *serf.RPCClient) error {
	// ping each other node and fail in case the results are over a specified
	// threshold
	for _, node := range nodes {
		// skip pinging self
		if c.self.Addr.String() == node.Addr.String() {
			continue
		}

		rttNanoSec, err := calculateRTT(client, c.self, node)
		if err != nil {
			return trace.Wrap(err)
		}

		err = c.saveLatencyStats(rttNanoSec, node)
		if err != nil {
			return trace.Wrap(err)
		}

		latencyInterface, exists := c.latencyStats.Get(node.Name)
		if !exists {
			return trace.NotFound("latency for %s not found", node.Name)
		}
		latency, ok := latencyInterface.([]int64)
		if !ok {
			return trace.BadParameter("expected latency for %s to be []int64 got %T", c.serfMemberName, latency)
		}
		latencyHistogram, err := c.buildLatencyHistogram(node.Name)
		if err != nil {
			return trace.Wrap(err)
		}

		log.Debugf("%s <-ping-> %s = %dns [latest]", c.self.Name, node.Name, rttNanoSec)
		log.Debugf("%s <-ping-> %s = %dns [%.2f percentile]",
			c.self.Name, node.Name,
			latencyHistogram.ValueAtQuantile(latencyQuantile),
			latencyQuantile)

		latencyPercentile := latencyHistogram.ValueAtQuantile(latencyQuantile)
		if latencyPercentile >= latencyThreshold.Nanoseconds() {
			log.Warningf("%s <-ping-> %s = slow ping detected. Value %dns over threshold %s (%dns)",
				c.self.Name, node.Name, latencyPercentile,
				latencyThreshold.String(), latencyThreshold.Nanoseconds())
		} else {
			log.Debugf("%s <-ping-> %s = ping okay. Value %dns within threshold %s (%dns)",
				c.self.Name, node.Name, latencyPercentile,
				latencyThreshold.String(), latencyThreshold.Nanoseconds())
		}
	}

	return nil
}

// buildLatencyHistogram converts the []int64 of latencies in a HDRHistrogram
func (c *pingChecker) buildLatencyHistogram(nodeName string) (latencyHDR *hdrhistogram.Histogram, err error) {
	latencyHDR = hdrhistogram.New(pingMinimum.Nanoseconds(),
		pingMaximum.Nanoseconds(), pingSignificantFigures)

	latency, exists := c.latencyStats.Get(nodeName)
	if !exists {
		return nil, trace.NotFound("latency for %s not found", nodeName)
	}
	latencySlice, ok := latency.([]int64)
	if !ok {
		return nil, trace.BadParameter("couldn't parse node latency as []int64 for %s", nodeName)
	}

	for _, v := range latencySlice {
		latencyHDR.RecordValue(v)
	}

	return latencyHDR, nil
}

// saveLatencyStats is used to store ping values in HDR Histograms in memory
func (c *pingChecker) saveLatencyStats(pingLatency int64, node serf.Member) error {
	c.mux.Lock()
	var latencySlice []int64

	s, exists := c.latencyStats.Get(node.Name)
	if !exists {
		latencySlice = make([]int64, 0, slidingWindowSize)
	} else {
		var ok bool
		latencySlice, ok = s.([]int64)
		if !ok {
			c.mux.Unlock()
			return trace.BadParameter("couldn't parse node latency as []int64 on %s", c.serfMemberName)
		}

		// upperLimit needs to be the highest between `len(latencySlice)` and `slidingWindowSize`
		upperLimit := max(len(latencySlice), slidingWindowSize)
		// shift by popping first (older) element up to desired size
		_, latencySlice = latencySlice[0], latencySlice[1:upperLimit]
	}

	latencySlice = append(latencySlice, pingLatency)
	log.Debugf("%d recorded ping values for node %s => %v", len(latencySlice), node.Name, latencySlice)

	err := c.latencyStats.Set(node.Name, latencySlice, statsTTLPeriod)
	if err != nil {
		c.mux.Unlock()
		return trace.Wrap(err)
	}

	c.mux.Unlock()
	return nil
}

func max(x, y int) (max int) {
	if x > y {
		return x
	}
	return y
}

// calculateRTT calculates and returns the latency time (in nanoseconds) between two Serf Cluster members
func calculateRTT(serfClient *serf.RPCClient, self, node serf.Member) (rttNanos int64, err error) {
	selfCoord, err := serfClient.GetCoordinate(self.Name)
	if err != nil {
		return 0, trace.Wrap(err)
	}
	if selfCoord == nil {
		return 0, trace.NotFound("self node %s coordinates not found", self.Name)
	}

	otherNodeCoord, err := serfClient.GetCoordinate(node.Name)
	if err != nil {
		return 0, trace.NotFound("error getting coordinates: %s -> %v", node.Name, err)
	}
	if otherNodeCoord == nil {
		return 0, trace.NotFound("could not find a coordinate for node %s", node.Name)
	}

	return selfCoord.DistanceTo(otherNodeCoord).Nanoseconds(), nil
}

// setProbeStatus sets the Probe according to status or raises an error if one is passed via arguments
func (c *pingChecker) setProbeStatus(ctx context.Context, r health.Reporter, err error, status pb.Probe_Type) {
	switch status {
	case pb.Probe_Failed:
		log.Error(err.Error())
		r.Add(NewProbeFromErr(c.Name(), "", err))
	case pb.Probe_Running:
		r.Add(&pb.Probe{
			Checker: c.Name(),
			Status:  pb.Probe_Running,
		})
	}
}
