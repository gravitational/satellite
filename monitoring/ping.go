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

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/trace"

	"github.com/codahale/hdrhistogram"
	serf "github.com/hashicorp/serf/client"
	log "github.com/sirupsen/logrus"
)

const (
	pingCheckerID     = "ping-checker"
	slidingWindowSize = 10   // number of ping results to consider per iteration
	pingRttThreshold  = 15.0 // pingRTT threshold expressed in ms
	pingRttQuantile   = 95.0 // quantile used to check against Rtt results
)

// NewPingChecker implements and return an health.Checker
func NewPingChecker(serfRPCAddr string, serfRPCName string) health.Checker {
	return &pingChecker{
		serfRPCAddr: serfRPCAddr,
		serfRPCName: serfRPCName,
	}
}

// pingChecker is a checker that verify that ping times (RTT) between nodes in
// the cluster are within a predefined threshold
type pingChecker struct {
	serfRPCAddr string
	serfRPCName string
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

	// fetch serf config and intantiate client
	log.Debugf("Using Serf IP: %v", c.serfRPCAddr)
	log.Debugf("Using Serf Name: %v", c.serfRPCName)
	clientConfig := serf.Config{
		Addr: c.serfRPCAddr,
	}
	client, err := serf.ClientFromConfig(&clientConfig)
	if err != nil {
		log.Errorf("error while connecting to Serf via IP %v. Error: %v",
			c.serfRPCAddr, err.Error())
		r.Add(&pb.Probe{
			Checker: c.Name(),
			Status:  pb.Probe_Failed,
		})
		return
	}
	defer client.Close()

	// retrieve other nodes using Serf members
	nodes, err := client.Members()
	if err != nil {
		log.Errorf("failed fetching Serf Members - %v", err.Error())
		r.Add(&pb.Probe{
			Checker: c.Name(),
			Status:  pb.Probe_Failed,
		})
		return
	}

	// finding what is the current node
	var self serf.Member
	for _, node := range nodes {
		// cut the port portion of the address after the ":" away
		if node.Name == c.serfRPCName {
			self = node
		}
	}
	if self.Name == "" {
		log.Errorf("error getting selfNode config: %s", c.serfRPCName)
		r.Add(&pb.Probe{
			Checker: c.Name(),
			Status:  pb.Probe_Failed,
		})
		return
	}

	selfCoord, err := client.GetCoordinate(self.Name)
	if err != nil || selfCoord == nil {
		log.Errorf("error getting selfNode coordinates: %s -> %#v", self.Name, err)
		r.Add(&pb.Probe{
			Checker: c.Name(),
			Status:  pb.Probe_Failed,
		})
		return
	}
	// ping each other node and fail in case the results are over a specified
	// threshold
	err = nil
	for _, node := range nodes {
		// skip pinging self
		if node.Addr.String() == self.Addr.String() {
			continue
		}

		coord2, err := client.GetCoordinate(node.Name)
		if err != nil {
			log.Errorf("error getting coordinates: %s -> %#v", node.Name, err)
			r.Add(&pb.Probe{
				Checker: c.Name(),
				Status:  pb.Probe_Failed,
			})
			continue
		}
		if coord2 == nil {
			err = trace.NotFound("could not find a coordinate for node %q", nodes[1])
			log.Error(err)
			r.Add(&pb.Probe{
				Checker: c.Name(),
				Status:  pb.Probe_Failed,
			})
			continue
		}

		// pingStats store ping statistics from 0 to 10000 ms (10 seconds)
		// up to 3 digits precision
		pingStats := hdrhistogram.New(0, 10000, 3)
		for i := 0; i < slidingWindowSize; i++ {
			rttNanoSec := selfCoord.DistanceTo(coord2).Nanoseconds()
			pingStats.RecordValue(rttNanoSec)
		}

		log.Debugf("%s <-ping-> %s = %d", self.Name, node.Name, pingStats.ValueAtQuantile(pingRttQuantile))

		if pingStats.ValueAtQuantile(pingRttQuantile) >= int64(pingRttThreshold*1e6) { // converting from ms to nanoseconds for comparison
			log.Errorf("slow ping between nodes detected. Value %v over threshold %v",
				pingRttQuantile, pingRttThreshold)
			r.Add(&pb.Probe{
				Checker: c.Name(),
				Status:  pb.Probe_Failed,
			})
		} else {
			log.Debugf("ping value %v below threshold %v ms", pingStats.ValueAtQuantile(pingRttQuantile), pingRttThreshold)
		}
	}

	// set Probe to be running
	if err != nil {
		r.Add(&pb.Probe{
			Checker: c.Name(),
			Status:  pb.Probe_Running,
		})
	}
	return
}
