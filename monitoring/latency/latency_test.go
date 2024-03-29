/*
Copyright 2019-2020 Gravitational, Inc.

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

package latency

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/lib/membership"
	"github.com/gravitational/satellite/lib/nethealth"
	"github.com/gravitational/satellite/lib/test"

	. "gopkg.in/check.v1"
)

func TestLatency(t *testing.T) { TestingT(t) }

type LatencySuite struct{}

var _ = Suite(&LatencySuite{})

func (r *LatencySuite) TestLatency(c *C) {
	var tests = []struct {
		comment  string
		expected health.Probes
		quantile float64
	}{
		{
			comment: "Latency at 95th percentile is above the threshold",
			expected: health.Probes{
				failureProbe(node1, node2, 50*time.Millisecond, latencyThreshold),
			},
			quantile: 0.95,
		},
		{
			comment: "Latency at 90th percentile is at the threshold",
			expected: health.Probes{
				successProbe(node1, latencyThreshold),
			},
			quantile: 0.90,
		},
		{
			comment: "Latency at 80th percentile is below the threshold",
			expected: health.Probes{
				successProbe(node1, latencyThreshold),
			},
			quantile: 0.80,
		},
	}
	for _, tc := range tests {
		comment := Commentf(tc.comment)
		checker, err := NewChecker(&Config{
			NodeName:        node1,
			LatencyQuantile: tc.quantile,
			Cluster:         r.newMockCluster(node1, node2),
			LatencyClient:   nethealth.NewMockClient(testMetrics),
		})
		c.Assert(err, IsNil, comment)

		test.WithTimeout(func(ctx context.Context) {
			var probes health.Probes
			checker.Check(ctx, &probes)
			sort.Sort(health.ByDetail(probes))
			c.Assert(probes, test.DeepCompare, tc.expected, comment)
		})
	}
}

// mockCluster implements a mock cluster to be used for testing.
type mockCluster struct {
	membership.Cluster
	members []string
}

// newMockCluster constructs a new mock cluster with the provided members.
func (r *LatencySuite) newMockCluster(members ...string) *mockCluster {
	return &mockCluster{
		members: members,
	}
}

// Members returns the list of members.
func (r *mockCluster) Members() (members []*pb.MemberStatus, err error) {
	for _, member := range r.members {
		members = append(members, pb.NewMemberStatus(member, "", make(map[string]string)))
	}
	return members, nil
}

// testMetrics is an example output of Prometheus metrics containing latency
// summaries.
const testMetrics = `# HELP nethealth_echo_latency_summary_milli The round trip time between peers in milliseconds
# TYPE nethealth_echo_latency_summary_milli summary
nethealth_echo_latency_summary_milli{node_name="node-1",peer_name="node-2",quantile="0.1"} 1
nethealth_echo_latency_summary_milli{node_name="node-1",peer_name="node-2",quantile="0.2"} 1
nethealth_echo_latency_summary_milli{node_name="node-1",peer_name="node-2",quantile="0.3"} 1
nethealth_echo_latency_summary_milli{node_name="node-1",peer_name="node-2",quantile="0.4"} 1
nethealth_echo_latency_summary_milli{node_name="node-1",peer_name="node-2",quantile="0.5"} 1
nethealth_echo_latency_summary_milli{node_name="node-1",peer_name="node-2",quantile="0.6"} 1
nethealth_echo_latency_summary_milli{node_name="node-1",peer_name="node-2",quantile="0.7"} 1
nethealth_echo_latency_summary_milli{node_name="node-1",peer_name="node-2",quantile="0.8"} 1
nethealth_echo_latency_summary_milli{node_name="node-1",peer_name="node-2",quantile="0.9"} 15
nethealth_echo_latency_summary_milli{node_name="node-1",peer_name="node-2",quantile="0.95"} 50
nethealth_echo_latency_summary_milli{node_name="node-1",peer_name="node-2",quantile="0.99"} 50
nethealth_echo_latency_summary_milli_sum{node_name="node-1",peer_name="node-2"} 373
nethealth_echo_latency_summary_milli_count{node_name="node-1",peer_name="node-2"} 6
`

const (
	// Test node names
	node1 = "node-1"
	node2 = "node-2"
)
