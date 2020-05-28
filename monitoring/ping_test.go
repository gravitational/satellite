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

	"github.com/gravitational/satellite/agent"
	"github.com/gravitational/satellite/agent/health"
	"github.com/gravitational/satellite/agent/proto/agentpb"

	serf "github.com/hashicorp/serf/client"
	"github.com/hashicorp/serf/coordinate"
	"gopkg.in/check.v1"
)

type PingSuite struct{}

var _ = check.Suite(&PingSuite{})

func (*PingSuite) TestPingChecker(c *check.C) {
	testCases := []struct {
		Nodes       []serf.Member
		Coords      map[string]*coordinate.Coordinate
		Status      agentpb.NodeStatus_Type
		Description string
	}{
		{
			Nodes: []serf.Member{
				{
					Name:   "member-1",
					Status: agentpb.MemberStatus_Alive.String(),
					Addr:   net.IPv4(127, 0, 0, 1),
				},
				{
					Name:   "member-2",
					Status: agentpb.MemberStatus_Alive.String(),
					Addr:   net.IPv4(127, 0, 0, 2),
				},
			},
			Coords: map[string]*coordinate.Coordinate{
				"member-1": &coordinate.Coordinate{
					Height: 0.001,
				},
				"member-2": &coordinate.Coordinate{
					Height: 0.001,
				},
			},
			Status:      agentpb.NodeStatus_Running,
			Description: "Testing standard working condition with values below threshold",
		},
		{
			Nodes: []serf.Member{
				{
					Name:   "member-1",
					Status: agentpb.MemberStatus_Alive.String(),
					Addr:   net.IPv4(127, 0, 0, 1),
				},
				{
					Name:   "member-2",
					Status: agentpb.MemberStatus_Alive.String(),
					Addr:   net.IPv4(127, 0, 0, 2),
				},
			},
			Coords:      map[string]*coordinate.Coordinate{},
			Status:      agentpb.NodeStatus_Running,
			Description: "Testing missing coordinates for cluster members",
		},
		{
			Nodes: []serf.Member{
				{
					Name:   "member-1",
					Status: agentpb.MemberStatus_Alive.String(),
					Addr:   net.IPv4(127, 0, 0, 1),
				},
				{
					Name:   "member-2",
					Status: agentpb.MemberStatus_Failed.String(),
					Addr:   net.IPv4(127, 0, 0, 2),
				},
			},
			Coords: map[string]*coordinate.Coordinate{
				"member-1": &coordinate.Coordinate{
					Height: 3600,
				},
				"member-2": &coordinate.Coordinate{
					Height: 3600,
				},
			},
			Status:      agentpb.NodeStatus_Running,
			Description: "Testing nodes with latency value over the threshold",
		},
	}

	for _, testCase := range testCases {
		pingChecker, err := NewPingChecker(PingCheckerConfig{
			SerfRPCAddr:    "127.0.0.1",
			SerfMemberName: "member-1",
			NewSerfClient: func(serf.Config) (agent.SerfClient, error) {
				return agent.NewMockSerfClient(testCase.Nodes, testCase.Coords), nil
			},
		})
		c.Assert(err, check.IsNil)
		var probes health.Probes
		pingChecker.Check(context.TODO(), &probes)
		c.Assert(probes.Status(), check.Equals, testCase.Status,
			check.Commentf(testCase.Description))
	}

}
