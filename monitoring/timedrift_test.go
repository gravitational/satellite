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
	debugpb "github.com/gravitational/satellite/agent/proto/debug"
	"github.com/gravitational/satellite/lib/rpc/client"

	"github.com/gravitational/trace"
	serf "github.com/hashicorp/serf/client"
	"github.com/jonboulle/clockwork"
	"github.com/sirupsen/logrus"
	"gopkg.in/check.v1"
)

type TimeDriftSuite struct {
	c     *timeDriftChecker
	clock clockwork.Clock
}

var _ = check.Suite(&TimeDriftSuite{})

func (s *TimeDriftSuite) SetUpSuite(c *check.C) {
	s.c = &timeDriftChecker{TimeDriftCheckerConfig: TimeDriftCheckerConfig{
		SerfMember: &node1,
	}}
	s.clock = clockwork.NewFakeClock()
}

func (s *TimeDriftSuite) TestTimeDriftChecker(c *check.C) {
	tests := []struct {
		// comment is the test case description.
		comment string
		// nodes is a list of all serf members.
		nodes []serf.Member
		// times maps serf member to its drift value.
		times map[string]time.Time
		// result is the time drift check result.
		result []*agentpb.Probe
	}{
		{
			comment: "Acceptable time drift",
			nodes:   []serf.Member{node1, node2, node3},
			times: map[string]time.Time{
				node2.Name: s.clock.Now().Add(driftUnderThreshold()),
				node3.Name: s.clock.Now().Add(driftUnderThreshold()),
			},
			result: []*agentpb.Probe{
				s.c.successProbe(),
			},
		},
		{
			comment: "Acceptable time drift, one node is lagging behind",
			nodes:   []serf.Member{node1, node2, node3},
			times: map[string]time.Time{
				node2.Name: s.clock.Now().Add(driftUnderThreshold()),
				node3.Name: s.clock.Now().Add(-driftUnderThreshold()),
			},
			result: []*agentpb.Probe{
				s.c.successProbe(),
			},
		},
		{
			comment: "Time drift to node-3 exceeds threshold",
			nodes:   []serf.Member{node1, node2, node3},
			times: map[string]time.Time{
				node2.Name: s.clock.Now().Add(driftUnderThreshold()),
				node3.Name: s.clock.Now().Add(driftOverThreshold()),
			},
			// Since we're using frozen time, latency will be 0 so
			// time drift will be exactly the amount we specified.
			result: []*agentpb.Probe{
				s.c.failureProbe(node3, driftOverThreshold()),
			},
		},
		{
			comment: "Time drift to node-2 exceeds threshold",
			nodes:   []serf.Member{node1, node2, node3},
			times: map[string]time.Time{
				node2.Name: s.clock.Now().Add(-driftOverThreshold()),
				node3.Name: s.clock.Now().Add(driftUnderThreshold()),
			},
			result: []*agentpb.Probe{
				s.c.failureProbe(node2, -driftOverThreshold()),
			},
		},
		{
			comment: "Time drift to both nodes exceeds threshold",
			nodes:   []serf.Member{node1, node2, node3},
			times: map[string]time.Time{
				node2.Name: s.clock.Now().Add(-driftOverThreshold()),
				node3.Name: s.clock.Now().Add(driftOverThreshold()),
			},
			result: []*agentpb.Probe{
				s.c.failureProbe(node2, -driftOverThreshold()),
				s.c.failureProbe(node3, driftOverThreshold()),
			},
		},
	}
	for _, test := range tests {
		checker := &timeDriftChecker{
			TimeDriftCheckerConfig: TimeDriftCheckerConfig{
				SerfClient: agent.NewMockSerfClient(test.nodes, nil),
				SerfMember: &node1,
				Clock:      s.clock,
			},
			FieldLogger: logrus.WithField(trace.Component, "test"),
			clients:     newClientsCache(c, test.times),
		}
		var probes health.Probes
		checker.Check(context.TODO(), &probes)
		c.Assert(probes.GetProbes(), check.DeepEquals, test.result,
			check.Commentf(test.comment))
	}
}

// driftOverThreshold returns time drift value that exceeds configured threshold.
func driftOverThreshold() time.Duration {
	return timeDriftThreshold * 2
}

// driftUnderThreshold returns time drift value that is within configured threshold.
func driftUnderThreshold() time.Duration {
	return timeDriftThreshold / 2
}

type mockedTimeAgentClient struct {
	time time.Time
}

func newMockedTimeAgentClient(time time.Time) *mockedTimeAgentClient {
	return &mockedTimeAgentClient{time: time}
}

func (a *mockedTimeAgentClient) Status(ctx context.Context) (*agentpb.SystemStatus, error) {
	return nil, nil
}

func (a *mockedTimeAgentClient) LocalStatus(ctx context.Context) (*agentpb.NodeStatus, error) {
	return nil, nil
}

func (a *mockedTimeAgentClient) LastSeen(ctx context.Context,
	req *agentpb.LastSeenRequest) (*agentpb.LastSeenResponse, error) {
	return nil, trace.NotImplemented("LastSeen not implemented")
}

func (a *mockedTimeAgentClient) Time(ctx context.Context, req *agentpb.TimeRequest) (*agentpb.TimeResponse, error) {
	return &agentpb.TimeResponse{
		Timestamp: agentpb.NewTimeToProto(a.time),
	}, nil
}

func (a *mockedTimeAgentClient) Timeline(ctx context.Context,
	req *agentpb.TimelineRequest) (*agentpb.TimelineResponse, error) {
	return nil, nil
}

func (a *mockedTimeAgentClient) UpdateTimeline(ctx context.Context,
	req *agentpb.UpdateRequest) (*agentpb.UpdateResponse, error) {
	return nil, nil
}

func (a *mockedTimeAgentClient) Profile(context.Context, *debugpb.ProfileRequest) (debugpb.Debug_ProfileClient, error) {
	return nil, nil
}

func (a *mockedTimeAgentClient) Close() error {
	return nil
}

func newClientsCache(c *check.C, times map[string]time.Time) map[string]client.Client {
	clients := make(map[string]client.Client)
	for nodeName, nodeTime := range times {
		clients[nodes[nodeName].Addr.String()] = newMockedTimeAgentClient(nodeTime)
	}
	return clients
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
