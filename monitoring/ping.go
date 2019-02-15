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
	"log"

	"github.com/codahale/hdrhistogram"
	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/trace"
	serf "github.com/hashicorp/serf/client"
	ping "github.com/sparrc/go-ping"
)

const (
	pingCheckerID     = "ping-checker"
	slidingWindowSize = 10 // number of ping results to consider per iteration
)

func NewPingChecker(serfRPCAddr string) health.Checker {
	return &pingChecker{
		serfRPCAddr: serfRPCAddr,
	}
}

// pingChecker is a checker that verify that ping times (RTT) between nodes in
// the cluster are within a predefined threshold
type pingChecker struct {
	serfRPCAddr string
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
	RttThreshold := int64(25) // ms
	// FIXME: #1 RttThreshold will become configurable in future
	// FIXME: #2 Send RttThreshold value to metrics

	// set Probe to be running
	r.Add(&pb.Probe{
		Checker: c.Name(),
		Status:  pb.Probe_Running,
	})

	// fetch serf config and intantiate client
	clientConfig := serf.Config{
		Addr: c.serfRPCAddr,
	}
	client, err := serf.ClientFromConfig(&clientConfig)
	if err != nil {
		return // nil, trace.Wrap(err, "failed to connect to serf")
	}
	defer client.Close()

	// ping other Master nodes and store results in Serf
	nodes, err := client.Members()
	if err != nil {
		log.Printf("failed to fetch Serf Members - %v", trace.Wrap(err))
		return
	}
	for _, node := range nodes {
		// FIXME: BEGINIF if other node is master {
		pinger, err := ping.NewPinger(node.Addr.String())
		if err != nil {
			log.Printf("got an error while trying to ping %v - %v", node.Addr.String(),
				trace.Wrap(err))
			r.Add(&pb.Probe{
				Checker: c.Name(),
				Status:  pb.Probe_Failed,
			})
			return
		}
		pinger.Count = slidingWindowSize // FIXME: does need to be set to actually use the last nth check results?
		pinger.Run()
		stats := pinger.Statistics()
		if stats.AvgRtt.Nanoseconds()/1000 >= RttThreshold { // convert to ms then compare
			r.Add(&pb.Probe{
				Checker: c.Name(),
				Status:  pb.Probe_Failed,
			})
		}
		// FIXME: ENDIF}
	}

	return
}
