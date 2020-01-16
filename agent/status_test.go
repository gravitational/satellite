/*
Copyright 2020 Gravitational, Inc.

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

package agent

import (
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/lib/test"

	serf "github.com/hashicorp/serf/client"
	. "gopkg.in/check.v1"
)

func (*AgentSuite) TestSetsSystemStatusFromMemberStatuses(c *C) {
	status := pb.SystemStatus{
		Nodes: []*pb.NodeStatus{
			{
				Name: "foo",
				MemberStatus: &pb.MemberStatus{
					Name:   "foo",
					Status: pb.MemberStatus_Alive,
					Tags:   tags{"role": string(RoleNode)},
				},
			},
			{
				Name: "bar",
				MemberStatus: &pb.MemberStatus{
					Name:   "bar",
					Status: pb.MemberStatus_Failed,
					Tags:   tags{"role": string(RoleMaster)},
				},
			},
		},
	}
	actual := status
	setSystemStatus(&actual, []serf.Member{{Name: "foo"}, {Name: "bar"}})

	expected := status
	expected.Status = pb.SystemStatus_Degraded

	c.Assert(actual, test.DeepCompare, expected, Commentf("Expected degraded system status."))
}

func (*AgentSuite) TestSetsSystemStatusFromNodeStatuses(c *C) {
	status := pb.SystemStatus{
		Nodes: []*pb.NodeStatus{
			{
				Name:   "foo",
				Status: pb.NodeStatus_Running,
				MemberStatus: &pb.MemberStatus{
					Name:   "foo",
					Status: pb.MemberStatus_Alive,
					Tags:   tags{"role": string(RoleNode)},
				},
			},
			{
				Name:   "bar",
				Status: pb.NodeStatus_Degraded,
				MemberStatus: &pb.MemberStatus{
					Name:   "bar",
					Status: pb.MemberStatus_Alive,
					Tags:   tags{"role": string(RoleMaster)},
				},
				Probes: []*pb.Probe{
					{
						Checker:  "qux",
						Status:   pb.Probe_Failed,
						Severity: pb.Probe_Critical,
						Error:    "not available",
					},
				},
			},
		},
	}

	actual := status
	setSystemStatus(&actual, []serf.Member{{Name: "foo"}, {Name: "bar"}})

	expected := status
	expected.Status = pb.SystemStatus_Degraded

	c.Assert(actual, test.DeepCompare, expected, Commentf("Expected degraded system status."))
}

func (*AgentSuite) TestDetectsNoMaster(c *C) {
	status := pb.SystemStatus{
		Nodes: []*pb.NodeStatus{
			{
				Name: "foo",
				MemberStatus: &pb.MemberStatus{
					Name:   "foo",
					Status: pb.MemberStatus_Alive,
					Tags:   tags{"role": string(RoleNode)},
				},
			},
			{
				Name: "bar",
				MemberStatus: &pb.MemberStatus{
					Name:   "bar",
					Status: pb.MemberStatus_Alive,
					Tags:   tags{"role": string(RoleNode)},
				},
			},
		},
	}

	actual := status
	setSystemStatus(&actual, []serf.Member{{Name: "foo"}, {Name: "bar"}})

	expected := status
	expected.Status = pb.SystemStatus_Degraded
	expected.Summary = errNoMaster.Error()

	c.Assert(actual, test.DeepCompare, expected, Commentf("Expected degraded system status."))
}

func (*AgentSuite) TestSetsOkSystemStatus(c *C) {
	status := pb.SystemStatus{
		Nodes: []*pb.NodeStatus{
			{
				Name:   "foo",
				Status: pb.NodeStatus_Running,
				MemberStatus: &pb.MemberStatus{
					Name:   "foo",
					Status: pb.MemberStatus_Alive,
					Tags:   tags{"role": string(RoleNode)},
				},
			},
			{
				Name:   "bar",
				Status: pb.NodeStatus_Running,
				MemberStatus: &pb.MemberStatus{
					Name:   "bar",
					Status: pb.MemberStatus_Alive,
					Tags:   tags{"role": string(RoleMaster)},
				},
			},
		},
	}
	actual := status
	setSystemStatus(&actual, []serf.Member{{Name: "foo"}, {Name: "bar"}})

	expected := status
	expected.Status = pb.SystemStatus_Running

	c.Assert(actual, test.DeepCompare, expected, Commentf("Expected running system status."))
}
