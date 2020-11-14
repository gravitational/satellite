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
	"sort"
	"time"

	"github.com/gravitational/satellite/agent/health"
	"github.com/gravitational/satellite/agent/proto/agentpb"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	debugpb "github.com/gravitational/satellite/agent/proto/debug"
	"github.com/gravitational/satellite/lib/membership"
	"github.com/gravitational/satellite/lib/rpc/client"

	"github.com/gravitational/trace"
	"github.com/jonboulle/clockwork"
	"gopkg.in/check.v1"
)

type TimeDriftSuite struct {
	clock clockwork.Clock
}

var _ = check.Suite(&TimeDriftSuite{})

func (s *TimeDriftSuite) SetUpSuite(c *check.C) {
	s.clock = clockwork.NewFakeClock()
}

func (s *TimeDriftSuite) TestTimeDriftChecker(c *check.C) {
	tests := []struct {
		// comment is the test case description.
		comment string
		// cluster is a mock cluster interface.
		cluster mockCluster
		// result is the time drift check result.
		result []*agentpb.Probe
		// slow indicates the server should respond slowly
		slow bool
	}{
		{
			comment: "Acceptable time drift",
			cluster: mockCluster{
				clients: map[string]*mockedTimeAgentClient{
					node2: newMockedTimeAgentClient(node2, s.clock.Now().Add(driftUnderThreshold)),
					node3: newMockedTimeAgentClient(node2, s.clock.Now().Add(driftUnderThreshold)),
				},
			},
			result: []*agentpb.Probe{
				successProbe(node1),
			},
		},
		{
			comment: "Acceptable time drift, one node is lagging behind",
			cluster: mockCluster{
				clients: map[string]*mockedTimeAgentClient{
					node2: newMockedTimeAgentClient(node2, s.clock.Now().Add(driftUnderThreshold)),
					node3: newMockedTimeAgentClient(node3, s.clock.Now().Add(-driftUnderThreshold)),
				},
			},
			result: []*agentpb.Probe{
				successProbe(node1),
			},
		},
		{
			comment: "Time drift to node-3 exceeds threshold",
			cluster: mockCluster{
				clients: map[string]*mockedTimeAgentClient{
					node2: newMockedTimeAgentClient(node2, s.clock.Now().Add(driftUnderThreshold)),
					node3: newMockedTimeAgentClient(node3, s.clock.Now().Add(driftOverThreshold)),
				},
			},
			// Since we're using frozen time, latency will be 0 so
			// time drift will be exactly the amount we specified.
			result: []*agentpb.Probe{
				failureProbe(node1, node3, driftOverThreshold),
			},
		},
		{
			comment: "Time drift to node-2 exceeds threshold",
			cluster: mockCluster{
				clients: map[string]*mockedTimeAgentClient{
					node2: newMockedTimeAgentClient(node2, s.clock.Now().Add(-driftOverThreshold)),
					node3: newMockedTimeAgentClient(node3, s.clock.Now().Add(driftUnderThreshold)),
				},
			},
			result: []*agentpb.Probe{
				failureProbe(node1, node2, -driftOverThreshold),
			},
		},
		{
			comment: "Time drift to both nodes exceeds threshold",
			cluster: mockCluster{
				clients: map[string]*mockedTimeAgentClient{
					node2: newMockedTimeAgentClient(node2, s.clock.Now().Add(-driftOverThreshold)),
					node3: newMockedTimeAgentClient(node3, s.clock.Now().Add(driftOverThreshold)),
				},
			},
			result: []*agentpb.Probe{
				failureProbe(node1, node2, -driftOverThreshold),
				failureProbe(node1, node3, driftOverThreshold),
			},
		},
		{
			comment: "Time drift takes too long to respond silently discarding results",
			cluster: mockCluster{
				clients: map[string]*mockedTimeAgentClient{
					node2: newMockedTimeAgentClient(node2, s.clock.Now().Add(-driftOverThreshold)),
					node3: newMockedTimeAgentClient(node3, s.clock.Now().Add(driftOverThreshold)),
				},
			},
			slow: true,
			result: []*agentpb.Probe{
				successProbe(node1),
			},
		},
	}
	for _, test := range tests {

		checker, err := NewTimeDriftChecker(TimeDriftCheckerConfig{
			NodeName: node1,
			Cluster:  test.cluster,
			DialRPC:  test.cluster.dial,
			Clock:    s.clock,
		})
		c.Assert(err, check.IsNil, check.Commentf(test.comment))

		var probes health.Probes

		ctx, cancel := context.WithCancel(context.Background())
		if test.slow {
			cancel()
		}

		checker.Check(ctx, &probes)
		sort.Sort(health.ByDetail(probes))
		c.Assert(probes.GetProbes(), check.DeepEquals, test.result,
			check.Commentf(test.comment))
	}
}

type mockCluster struct {
	membership.Cluster
	clients map[string]*mockedTimeAgentClient
}

func (r mockCluster) Members() ([]*pb.MemberStatus, error) {
	members := make([]*pb.MemberStatus, 0, len(r.clients))
	for _, client := range r.clients {
		members = append(members, memberFromMockClient(client))
	}
	return members, nil
}

func (r mockCluster) dial(_ context.Context, name string) (client.Client, error) {
	client, exists := r.clients[name]
	if !exists {
		return nil, trace.NotFound("member %s does not exist in this cluster", name)
	}
	return client, nil
}

// memberFromMockClient constructs a new ClusterMember from the provided client.
func memberFromMockClient(client *mockedTimeAgentClient) *pb.MemberStatus {
	return &pb.MemberStatus{
		Name:   client.name,
		Addr:   client.name, // mock dial function will use name to dial node
		Status: pb.MemberStatus_Alive,
	}
}

type mockedTimeAgentClient struct {
	time time.Time
	name string
}

func newMockedTimeAgentClient(name string, time time.Time) *mockedTimeAgentClient {
	return &mockedTimeAgentClient{
		name: name,
		time: time,
	}
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
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
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

func (a *mockedTimeAgentClient) UpdateLocalTimeline(ctx context.Context,
	req *agentpb.UpdateRequest) (*agentpb.UpdateResponse, error) {
	return nil, nil
}

func (a *mockedTimeAgentClient) Profile(context.Context, *debugpb.ProfileRequest) (debugpb.Debug_ProfileClient, error) {
	return nil, nil
}

func (a *mockedTimeAgentClient) Close() error {
	return nil
}

const (
	// Test nodes
	node1 = "node-1"
	node2 = "node-2"
	node3 = "node-3"

	// Arbitrary values over and under the default time drift threshold
	driftOverThreshold  = timeDriftThreshold * 2
	driftUnderThreshold = timeDriftThreshold / 2
)
