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
	pingChecker, err := NewPingChecker(PingCheckerConfig{
		SerfRPCAddr:    "127.0.0.1",
		SerfMemberName: "member-1",
		NewSerfClient: func(serf.Config) (agent.SerfClient, error) {
			client, _ := agent.NewMockSerfClient(
				[]serf.Member{
					{
						Name: "member-1",
						Addr: net.IPv4(127, 0, 0, 1),
					},
					{
						Name: "member-2",
						Addr: net.IPv4(127, 0, 0, 2),
					},
				},
				map[string]*coordinate.Coordinate{
					"member-1": &coordinate.Coordinate{
						Vec:        []float64{1.0}, // in seconds
						Error:      0,
						Adjustment: 0,
						Height:     0,
					},
					"member-2": &coordinate.Coordinate{
						Vec:        []float64{1.0}, // in seconds
						Error:      0,
						Adjustment: 0,
						Height:     0,
					},
				},
			)
			return client, nil
		},
	})
	c.Assert(err, check.IsNil)

	var probes health.Probes
	pingChecker.Check(context.TODO(), &probes)
	c.Assert(probes.Status(), check.Equals, agentpb.NodeStatus_Running)

	pingChecker, err = NewPingChecker(PingCheckerConfig{
		SerfRPCAddr:    "127.0.0.1",
		SerfMemberName: "member-1",
		NewSerfClient: func(serf.Config) (agent.SerfClient, error) {
			client, _ := agent.NewMockSerfClient(
				[]serf.Member{
					{
						Name: "member-1",
						Addr: net.IPv4(127, 0, 0, 1),
					},
					{
						Name: "member-2",
						Addr: net.IPv4(127, 0, 0, 2),
					},
				},
				map[string]*coordinate.Coordinate{},
			)
			return client, nil
		},
	})
	c.Assert(err, check.IsNil)

	pingChecker.Check(context.TODO(), &probes)
	c.Assert(probes.Status(), check.Equals, agentpb.NodeStatus_Degraded)

	pingChecker, err = NewPingChecker(PingCheckerConfig{
		SerfRPCAddr:    "127.0.0.1",
		SerfMemberName: "member-1",
		NewSerfClient: func(serf.Config) (agent.SerfClient, error) {
			client, _ := agent.NewMockSerfClient(
				[]serf.Member{
					{
						Name: "member-1",
						Addr: net.IPv4(127, 0, 0, 1),
					},
					{
						Name: "member-2",
						Addr: net.IPv4(127, 0, 0, 2),
					},
				},
				map[string]*coordinate.Coordinate{
					"member-1": &coordinate.Coordinate{
						Vec:        []float64{1.0}, // in seconds
						Error:      0,
						Adjustment: 0,
						Height:     0,
					},
					"member-2": &coordinate.Coordinate{
						Vec:        []float64{1.0}, // in seconds
						Error:      0,
						Adjustment: 0,
						Height:     0,
					},
				},
			)
			return client, nil
		},
	})
	c.Assert(err, check.IsNil)

	pingChecker.Check(context.TODO(), &probes)
	c.Assert(probes.Status(), check.Equals, agentpb.NodeStatus_Running)
}
