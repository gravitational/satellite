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

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"

	"github.com/codahale/hdrhistogram"
	serf "github.com/hashicorp/serf/client"
	log "github.com/sirupsen/logrus"
)

const (
	// pingCheckerID specifies the check name
	pingCheckerID = "ping-checker"
	// slidingWindowSize specifies the sliding window use by checks
	slidingWindowSize = 10
	// msToNanoSec is used to convert nanoSeconds to milliSeconds
	msToNanoSec = 1e6
	// second constant is used to represent a second, expressed in milliseconds
	second = 1000
	// pingRoundtripMinimum set the minim value that can be recorded
	pingRoundtripMinimum = 0 * second
	// pingRoundtripMaximum set the maximum value that can be recorded
	pingRoundtripMaximum = 10 * msToNanoSec * second
	// pingRoundtripSignificativeFigures specifies how many decimals should be recorded
	pingRoundtripSignificativeFigures = 3
	// pingRoundtripThreshold sets the RTT threshold expressed in milliseconds (ms)
	pingRoundtripThreshold = 15.0
	// pingRoundtripQuantile sets the quantile used while checking Histograms against Rtt results
	pingRoundtripQuantile = 95.0
)

// NewPingChecker implements and return an health.Checker
func NewPingChecker(serfRPCAddr string, serfMemberName string) health.Checker {
	return &pingChecker{
		serfRPCAddr:    serfRPCAddr,
		serfMemberName: serfMemberName,
		rttStats:       make(map[string]*hdrhistogram.WindowedHistogram),
	}
}

// pingChecker is a checker that verify that ping times (RTT) between nodes in
// the cluster are within a predefined threshold
type pingChecker struct {
	serfRPCAddr    string
	serfMemberName string
	rttStats       map[string]*hdrhistogram.WindowedHistogram
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

	err = c.tempFunc(nodes, client)
	if err != nil {
		return err
	}

	return err
}

// tempFunc FIXME
func (c *pingChecker) tempFunc(nodes []serf.Member, client *serf.RPCClient) error {
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

	selfCoord, err := client.GetCoordinate(self.Name)
	if err != nil {
		return err
	}
	if selfCoord == nil {
		errMsg := fmt.Sprintf("self node %s coordinates not found", self.Name)
		return errors.New(errMsg)
	}

	// ping each other node and fail in case the results are over a specified
	// threshold
	for _, node := range nodes {
		// skip pinging self
		if node.Addr.String() == self.Addr.String() {
			continue
		}

		coord2, err := client.GetCoordinate(node.Name)
		if err != nil {
			errMsg := fmt.Sprintf("error getting coordinates: %s -> %#v", node.Name, err)
			return errors.New(errMsg)
		}
		if coord2 == nil {
			errMsg := fmt.Sprintf("could not find a coordinate for node %q", nodes[1])
			return errors.New(errMsg)
		}

		rttNanoSec := int64(selfCoord.DistanceTo(coord2).Nanoseconds())

		_, exists := c.rttStats[node.Name]
		if !exists {
			c.rttStats[node.Name] = hdrhistogram.NewWindowed(slidingWindowSize, pingRoundtripMinimum, pingRoundtripMaximum, pingRoundtripSignificativeFigures)
		}

		log.Debugf("%d recorded ping RoundTrip values for node %s",
			c.rttStats[node.Name].Current.TotalCount(),
			node.Name)
		log.Debugf("%s <-ping-> %s = %dms(latest)", self.Name, node.Name, rttNanoSec)
		log.Debugf("%s <-ping-> %s = %dms(%.3f percentile)",
			self.Name, node.Name,
			c.rttStats[node.Name].Current.ValueAtQuantile(pingRoundtripQuantile),
			pingRoundtripQuantile)

		err = c.rttStats[node.Name].Current.RecordValue(rttNanoSec)
		if err != nil {
			return err
		}

		if c.rttStats[node.Name].Current.ValueAtQuantile(pingRoundtripQuantile) >= int64(pingRoundtripThreshold*msToNanoSec) {
			errMsg := fmt.Sprintf("slow ping between nodes detected. Value %v over threshold %v",
				pingRoundtripQuantile, pingRoundtripThreshold)
			return errors.New(errMsg)
		} else {
			log.Debugf("ping value %dns below threshold %vns(%vms)",
				c.rttStats[node.Name].Current.ValueAtQuantile(pingRoundtripQuantile),
				pingRoundtripThreshold*msToNanoSec,
				pingRoundtripThreshold)
		}
	}

	return err
}

// setProbeStatus set the Probe according to status or raise an error if one is passed via arguments
func (c *pingChecker) setProbeStatus(ctx context.Context, r health.Reporter, err error, status pb.Probe_Type) {
	switch status {
	case pb.Probe_Failed:
		log.Error("%v", err.Error())
		r.Add(NewProbeFromErr(c.Name(), "", err))
	case pb.Probe_Running:
		r.Add(&pb.Probe{
			Checker: c.Name(),
			Status:  pb.Probe_Running,
		})
	}
	return
}
