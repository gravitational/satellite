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

	"github.com/gravitational/satellite/agent"
	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"

	"github.com/codahale/hdrhistogram"
	"github.com/gravitational/trace"
	"github.com/gravitational/ttlmap"
	serf "github.com/hashicorp/serf/client"
	log "github.com/sirupsen/logrus"
)

// TODO: latencyThreshold should be configurable
// TODO: latency stats should be sent to metrics

const (
	// pingCheckerID specifies the check name
	pingCheckerID = "ping-checker"
	// latencyStatsTTL specifies how long check results will be kept before being dropped
	latencyStatsTTL = 1 * time.Hour
	// latencyStatsCapacity sets the number of TTLMaps that can be stored; this will be the size of the cluster -1
	latencyStatsCapacity = 1000
	// latencyStatsSlidingWindowSize specifies the number of retained check results
	latencyStatsSlidingWindowSize = 20
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

// pingChecker is a checker that verifies that ping times (RTT) between nodes in
// the cluster are within a predefined threshold
type pingChecker struct {
	self           serf.Member
	serfClient     agent.SerfClient
	serfRPCAddr    string
	serfMemberName string
	latencyStats   ttlmap.TTLMap
	mux            sync.Mutex
	logger         log.Entry
}

type PingCheckerConfig struct {
	SerfRPCAddr    string
	SerfMemberName string
	NewSerfClient  agent.NewSerfClientFunc
}

func (c *PingCheckerConfig) CheckAndSetDefaults() error {
	if c.SerfRPCAddr == "" {
		return trace.BadParameter("serf rpc address can't be empty")
	}
	if c.SerfMemberName == "" {
		return trace.BadParameter("serf member name can't be empty")
	}
	if c.NewSerfClient == nil {
		c.NewSerfClient = agent.NewSerfClient
	}
	return nil
}

// NewPingChecker returns a checker that verifies accessibility of nodes in the cluster by exchanging ping requests
func NewPingChecker(conf PingCheckerConfig) (c health.Checker, err error) {
	err = conf.CheckAndSetDefaults()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	latencyTTLMap, err := ttlmap.New(latencyStatsCapacity)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	logger := log.WithFields(log.Fields{trace.Component: "ping"})
	logger.Debugf("using Serf IP: %v", conf.SerfRPCAddr)
	logger.Debugf("using Serf Name: %v", conf.SerfMemberName)

	client, err := conf.NewSerfClient(serf.Config{
		Addr: conf.SerfRPCAddr,
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}

	// retrieve other nodes using Serf members
	nodes, err := client.Members()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	// finding what is the current node
	var self serf.Member
	for _, node := range nodes {
		if node.Name == conf.SerfMemberName {
			self = node
			break // self node found, breaking out of the for loop
		}
	}
	if self.Name == "" {
		return nil, trace.NotFound("failed to find Serf member with name %s", conf.SerfMemberName)
	}

	return &pingChecker{
		self:           self,
		serfClient:     client,
		serfRPCAddr:    conf.SerfRPCAddr,
		serfMemberName: conf.SerfMemberName,
		latencyStats:   *latencyTTLMap,
		logger:         *logger,
	}, nil
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
	err := c.check(ctx, r)

	if err != nil {
		c.logger.Error(err.Error())
		r.Add(NewProbeFromErr(c.Name(), "", err))
		return
	}
	r.Add(&pb.Probe{Checker: c.Name(), Status: pb.Probe_Running})
}

// check runs the actual system status verification code and returns an error
// in case issues arise in the process
func (c *pingChecker) check(ctx context.Context, r health.Reporter) (err error) {

	client := c.serfClient

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
func (c *pingChecker) checkNodesRTT(nodes []serf.Member, client agent.SerfClient) error {
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

		latencies, err := c.saveLatencyStats(rttNanoSec, node)
		if err != nil {
			return trace.Wrap(err)
		}

		latencyHistogram, err := c.buildLatencyHistogram(node.Name, latencies)
		if err != nil {
			return trace.Wrap(err)
		}

		c.logger.Debugf("%s <-ping-> %s = %dns [latest]", c.self.Name, node.Name, rttNanoSec)
		c.logger.Debugf("%s <-ping-> %s = %dns [%.2f percentile]",
			c.self.Name, node.Name,
			latencyHistogram.ValueAtQuantile(latencyQuantile),
			latencyQuantile)

		latencyPercentile := latencyHistogram.ValueAtQuantile(latencyQuantile)
		if latencyPercentile >= latencyThreshold.Nanoseconds() {
			c.logger.Warningf("%s <-ping-> %s = slow ping detected. Value %dns over threshold %s (%dns)",
				c.self.Name, node.Name, latencyPercentile,
				latencyThreshold.String(), latencyThreshold.Nanoseconds())
		} else {
			c.logger.Debugf("%s <-ping-> %s = ping okay. Value %dns within threshold %s (%dns)",
				c.self.Name, node.Name, latencyPercentile,
				latencyThreshold.String(), latencyThreshold.Nanoseconds())
		}
	}

	return nil
}

// buildLatencyHistogram maps latencies to a HDRHistrogram
func (c *pingChecker) buildLatencyHistogram(nodeName string, latencies []int64) (latencyHDR *hdrhistogram.Histogram, err error) {
	latencyHDR = hdrhistogram.New(pingMinimum.Nanoseconds(),
		pingMaximum.Nanoseconds(), pingSignificantFigures)

	for _, v := range latencies {
		err := latencyHDR.RecordValue(v)
		if err != nil {
			return nil, trace.Wrap(err)
		}
	}

	return latencyHDR, nil
}

// saveLatencyStats is used to store ping values in HDR Histograms in memory
func (c *pingChecker) saveLatencyStats(pingLatency int64, node serf.Member) (latencies []int64, err error) {
	c.mux.Lock()
	defer c.mux.Unlock()

	if value, exists := c.latencyStats.Get(node.Name); exists {
		var ok bool
		if latencies, ok = value.([]int64); !ok {
			return nil, trace.BadParameter("couldn't parse node latency as []int64 on %s", c.serfMemberName)
		}
	}

	if len(latencies) >= latencyStatsSlidingWindowSize {
		// keep the slice within the sliding window size
		// slidingWindowSize is -1 because another element will be added a few lines below
		latencies = latencies[1:latencyStatsSlidingWindowSize]
	}

	latencies = append(latencies, pingLatency)
	c.logger.Debugf("%d recorded ping values for node %s => %v", len(latencies), node.Name, latencies)

	err = c.latencyStats.Set(node.Name, latencies, latencyStatsTTL)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return latencies, nil
}

// calculateRTT calculates and returns the latency time (in nanoseconds) between two Serf Cluster members
func calculateRTT(serfClient agent.SerfClient, self, node serf.Member) (rttNanos int64, err error) {
	selfCoord, err := serfClient.GetCoordinate(self.Name)
	if err != nil {
		return 0, trace.Wrap(err)
	}
	if selfCoord == nil {
		return 0, trace.NotFound("could not find a coordinate for node %s", self.Name)
	}

	otherNodeCoord, err := serfClient.GetCoordinate(node.Name)
	if err != nil {
		return 0, trace.Wrap(err)
	}
	if otherNodeCoord == nil {
		return 0, trace.NotFound("could not find a coordinate for node %s", node.Name)
	}

	return selfCoord.DistanceTo(otherNodeCoord).Nanoseconds(), nil
}
