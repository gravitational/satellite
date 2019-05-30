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
	"net"
	"time"

	"github.com/gravitational/satellite/agent"
	"github.com/gravitational/satellite/agent/health"
	"github.com/gravitational/satellite/agent/proto/agentpb"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/trace"
	"github.com/gravitational/ttlmap"
	"github.com/jonboulle/clockwork"
	"github.com/sirupsen/logrus"

	serf "github.com/hashicorp/serf/client"
	"gopkg.in/check.v1"
)

type TimeDriftSuite struct {
	c     *timeDriftChecker
	clock clockwork.Clock
}

var _ = check.Suite(&TimeDriftSuite{})

func (s *TimeDriftSuite) SetUpSuite(c *check.C) {
	s.c = &timeDriftChecker{self: node1}
	s.clock = clockwork.NewFakeClock()
}

func (s *TimeDriftSuite) TestTimeDriftChecker(c *check.C) {
	tests := []struct {
		Comment string
		Nodes   []serf.Member
		Times   map[string]time.Time
		Result  []*pb.Probe
	}{
		{
			Comment: "Time drift to both nodes is acceptable",
			Nodes:   []serf.Member{node1, node2, node3},
			Times: map[string]time.Time{
				node2.Name: s.clock.Now().Add(100 * time.Millisecond),
				node3.Name: s.clock.Now().Add(200 * time.Millisecond),
			},
			// Since we're using frozen time, latency will be 0 so
			// time drift will be exactly the amount we specified.
			Result: []*pb.Probe{
				s.c.successProbe(node2, 100*time.Millisecond),
				s.c.successProbe(node3, 200*time.Millisecond),
			},
		},
		{
			Comment: "Time drift to both nodes is acceptable, one node is lagging behind",
			Nodes:   []serf.Member{node1, node2, node3},
			Times: map[string]time.Time{
				node2.Name: s.clock.Now().Add(100 * time.Millisecond),
				node3.Name: s.clock.Now().Add(-100 * time.Millisecond),
			},
			// Since we're using frozen time, latency will be 0 so
			// time drift will be exactly the amount we specified.
			Result: []*pb.Probe{
				s.c.successProbe(node2, 100*time.Millisecond),
				s.c.successProbe(node3, -100*time.Millisecond),
			},
		},
		{
			Comment: "Time drift to node-3 is high",
			Nodes:   []serf.Member{node1, node2, node3},
			Times: map[string]time.Time{
				node2.Name: s.clock.Now().Add(100 * time.Millisecond),
				node3.Name: s.clock.Now().Add(500 * time.Millisecond),
			},
			// Since we're using frozen time, latency will be 0 so
			// time drift will be exactly the amount we specified.
			Result: []*pb.Probe{
				s.c.successProbe(node2, 100*time.Millisecond),
				s.c.failureProbe(node3, 500*time.Millisecond),
			},
		},
		{
			Comment: "Time drift to node-2 is high",
			Nodes:   []serf.Member{node1, node2, node3},
			Times: map[string]time.Time{
				node2.Name: s.clock.Now().Add(-time.Second),
				node3.Name: s.clock.Now().Add(100 * time.Millisecond),
			},
			// Since we're using frozen time, latency will be 0 so
			// time drift will be exactly the amount we specified.
			Result: []*pb.Probe{
				s.c.failureProbe(node2, -time.Second),
				s.c.successProbe(node3, 100*time.Millisecond),
			},
		},
		{
			Comment: "Time drift to both nodes is high",
			Nodes:   []serf.Member{node1, node2, node3},
			Times: map[string]time.Time{
				node2.Name: s.clock.Now().Add(-time.Second),
				node3.Name: s.clock.Now().Add(time.Second),
			},
			// Since we're using frozen time, latency will be 0 so
			// time drift will be exactly the amount we specified.
			Result: []*pb.Probe{
				s.c.failureProbe(node2, -time.Second),
				s.c.failureProbe(node3, time.Second),
			},
		},
	}
	for _, test := range tests {
		clients, err := ttlmap.New(10)
		c.Assert(err, check.IsNil)
		for nodeName, nodeTime := range test.Times {
			clients.Set(
				nodes[nodeName].Addr.String(),
				newMockedTimeAgentClient(nodeTime),
				time.Hour)
		}
		checker := &timeDriftChecker{
			TimeDriftCheckerConfig: TimeDriftCheckerConfig{
				Clock: s.clock,
			},
			FieldLogger: logrus.WithField(trace.Component, "test"),
			clients:     clients,
			serfClient:  agent.NewMockSerfClient(test.Nodes, nil),
			name:        node1.Name,
			self:        node1,
		}
		var probes health.Probes
		checker.Check(context.TODO(), &probes)
		c.Assert(probes.GetProbes(), check.DeepEquals, test.Result,
			check.Commentf(test.Comment))
	}
}

type mockedTimeAgentClient struct {
	agent.Client
	time time.Time
}

func newMockedTimeAgentClient(time time.Time) *mockedTimeAgentClient {
	return &mockedTimeAgentClient{time: time}
}

func (a *mockedTimeAgentClient) Time(ctx context.Context, req *pb.TimeRequest) (*pb.TimeResponse, error) {
	return &pb.TimeResponse{
		Timestamp: pb.NewTimeToProto(a.time),
	}, nil
}

var (
	node1 serf.Member = serf.Member{
		Name:   "node-1",
		Status: agentpb.MemberStatus_Alive.String(),
		Addr:   net.IPv4(192, 168, 0, 1),
	}
	node2 serf.Member = serf.Member{
		Name:   "node-2",
		Status: agentpb.MemberStatus_Alive.String(),
		Addr:   net.IPv4(192, 168, 0, 2),
	}
	node3 serf.Member = serf.Member{
		Name:   "node-3",
		Status: agentpb.MemberStatus_Alive.String(),
		Addr:   net.IPv4(192, 168, 0, 3),
	}
	nodes map[string]serf.Member = map[string]serf.Member{
		node1.Name: node1,
		node2.Name: node2,
		node3.Name: node3,
	}
)
