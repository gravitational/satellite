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
	"errors"
	"fmt"
	"github.com/gravitational/ttlmap"
	"time"

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"

	"github.com/codahale/hdrhistogram"
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
	// pingRoundtripMinimum set the minim value that can be recorded
	pingRoundtripMinimum = 0 * time.Second
	// pingRoundtripMaximum set the maximum value that can be recorded
	pingRoundtripMaximum = 10 * time.Second
	// pingRoundtripSignificativeFigures specifies how many decimals should be recorded
	pingRoundtripSignificativeFigures = 3
	// pingRoundtripThreshold sets the RTT threshold
	pingRoundtripThreshold = 15 * time.Millisecond
	// pingRoundtripQuantile sets the quantile used while checking Histograms against Rtt results
	pingRoundtripQuantile = 95.0
)

// NewPingChecker implements and return an health.Checker
func NewPingChecker(serfRPCAddr string, serfMemberName string) health.Checker {
	rttStatsTTLMap, err := ttlmap.New(int(statsTTLPeriod.Seconds()))
	if err != nil {
		log.Error(err)
		return nil
	}
	return &pingChecker{
		serfRPCAddr:    serfRPCAddr,
		serfMemberName: serfMemberName,
		rttStats:       *rttStatsTTLMap,
	}
}

// pingChecker is a checker that verify that ping times (RTT) between nodes in
// the cluster are within a predefined threshold
type pingChecker struct {
	serfRPCAddr    string
	serfMemberName string
	rttStats       ttlmap.TTLMap
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
	// FIXME: #1 RttThreshold will become configurable in future
	// FIXME: #2 Send RttThreshold value to metrics

	err := c.check(ctx, r)

	if err != nil {
		c.setProbeStatus(ctx, r, err, pb.Probe_Failed)
	} else {
		c.setProbeStatus(ctx, r, nil, pb.Probe_Running)
	}
	return
}

// check runs the actual system status verification code and returns an error
// in case issues arise in the process
func (c *pingChecker) check(ctx context.Context, r health.Reporter) error {
	// fetch serf config and intantiate client
	log.Debugf("Using Serf IP: %v", c.serfRPCAddr)
	log.Debugf("Using Serf Name: %v", c.serfMemberName)
	clientConfig := serf.Config{
		Addr: c.serfRPCAddr,
	}
	client, err := serf.ClientFromConfig(&clientConfig)
	if err != nil {
		return err
	}
	defer client.Close()

	// retrieve other nodes using Serf members
	nodes, err := client.Members()
	if err != nil {
		return err
	}

	err = c.checkNodesRTT(nodes, client)
	if err != nil {
		return err
	}

	return err
}

// checkNodesRTT implements the bulk of the logic by checking the ping RoundTrip time
// between this node (self) and the other Serf Cluster member nodes
func (c *pingChecker) checkNodesRTT(nodes []serf.Member, client *serf.RPCClient) error {
	// finding what is the current node
	var self serf.Member
	for _, node := range nodes {
		// cut the port portion of the address after the ":" away
		if node.Name == c.serfMemberName {
			self = node
		}
	}
	if self.Name == "" {
		errMsg := fmt.Sprintf("self node Serf Member not found for %s",
			c.serfMemberName)
		return errors.New(errMsg)
	}

	// ping each other node and fail in case the results are over a specified
	// threshold
	for _, node := range nodes {
		// skip pinging self
		if self.Addr.String() == node.Addr.String() {
			continue
		}

		rttNanoSec, err := calculateRTT(client, self, node)
		if err != nil {
			return err
		}

		err = c.storePingInHDR(rttNanoSec, node)
		if err != nil {
			return err
		}

		rttStatsHDRInterface, _ := c.rttStats.Get(node.Name)
		rttStatsHDR := rttStatsHDRInterface.(hdrhistogram.Histogram)
		log.Debugf("%s <-ping-> %s = %dns [latest]", self.Name, node.Name, rttNanoSec)
		log.Debugf("%s <-ping-> %s = %dns [%.2f percentile]",
			self.Name, node.Name,
			rttStatsHDR.ValueAtQuantile(pingRoundtripQuantile),
			pingRoundtripQuantile)

		pingRoundtripPercentile := rttStatsHDR.ValueAtQuantile(pingRoundtripQuantile)
		if pingRoundtripPercentile >= pingRoundtripThreshold.Nanoseconds() {
			log.Warningf("%s <-ping-> %s : slow ping RoundTrip detected. Value %dns over threshold %dms (%dns)",
				self.Name, node.Name, pingRoundtripPercentile,
				pingRoundtripThreshold, pingRoundtripThreshold.Nanoseconds())
		} else {
			log.Debugf("%s <-ping-> %s : ping RoundTrip okay. Value %dns within threshold %dms (%dns)",
				self.Name, node.Name, pingRoundtripPercentile,
				pingRoundtripThreshold, pingRoundtripThreshold.Nanoseconds())
		}
	}

	return nil
}

// storePingInHDR is used to store ping RoundTrip values in HDR Histograms in memory
func (c *pingChecker) storePingInHDR(pingRttStats int64, node serf.Member) error {
	nodeTTLMapInterface, exists := c.rttStats.Get(node.Name)
	nodeTTLMap := nodeTTLMapInterface.(hdrhistogram.Histogram)

	if !exists {
		c.rttStats.Set(node.Name,
			hdrhistogram.New(pingRoundtripMinimum.Nanoseconds(),
				pingRoundtripMaximum.Nanoseconds(),
				pingRoundtripSignificativeFigures),
			statsTTLPeriod)
	}

	if nodeTTLMap.TotalCount() >= slidingWindowSize {
		tmpSnapshot := nodeTTLMap.Export()
		// pop element at index 0 (oldest)
		_, tmpSnapshot.Counts = tmpSnapshot.Counts[0], tmpSnapshot.Counts[1:]
		c.rttStats.Set(node.Name, hdrhistogram.Import(tmpSnapshot),
			statsTTLPeriod)
	}

	err := nodeTTLMap.RecordValue(pingRttStats)
	if err != nil {
		return err
	}

	log.Debugf("%d recorded ping RoundTrip values for node %s",
		nodeTTLMap.TotalCount(), node.Name)

	return nil
}

// calculateRTT calculates the RoundTrip time between two Serf Cluster members
func calculateRTT(serfClient *serf.RPCClient, self serf.Member, node serf.Member) (int64, error) {
	selfCoord, err := serfClient.GetCoordinate(self.Name)
	if err != nil {
		return 0, err
	}
	if selfCoord == nil {
		errMsg := fmt.Sprintf("self node %s coordinates not found", self.Name)
		return 0, errors.New(errMsg)
	}

	otherNodeCoord, err := serfClient.GetCoordinate(node.Name)
	if err != nil {
		errMsg := fmt.Sprintf("error getting coordinates: %s -> %v", node.Name, err)
		return 0, errors.New(errMsg)
	}
	if otherNodeCoord == nil {
		errMsg := fmt.Sprintf("could not find a coordinate for node %s -> %v", node.Name, err)
		return 0, errors.New(errMsg)
	}

	return selfCoord.DistanceTo(otherNodeCoord).Nanoseconds(), nil
}

// setProbeStatus set the Probe according to status or raise an error if one is passed via arguments
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
	return
}
