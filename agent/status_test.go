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

	. "gopkg.in/check.v1"
)

type StatusSuite struct {
	members []*pb.MemberStatus
}

var _ = Suite(&StatusSuite{})

func (r *StatusSuite) SetUpSuite(*C) {
	r.members = []*pb.MemberStatus{
		{
			//nolint:godox
			// TODO: remove in 10
			Name:     "foo",
			NodeName: "foo",
		},
		{
			//nolint:godox
			// TODO: remove in 10
			Name:     "bar",
			NodeName: "bar",
		},
	}
}

func (r *StatusSuite) TestSetsSystemStatusFromMemberStatuses(c *C) {
	status := pb.SystemStatus{
		Nodes: []*pb.NodeStatus{
			newNode("foo").build(),
			newMaster("bar").memberFailed().build(),
		},
	}
	actual := status
	setSystemStatus(&actual, r.members)

	expected := status
	expected.Status = pb.SystemStatus_Degraded

	c.Assert(actual, test.DeepCompare, expected, Commentf("Expected degraded system status."))
}

func (r *StatusSuite) TestSetsSystemStatusFromNodeStatuses(c *C) {
	status := pb.SystemStatus{
		Nodes: []*pb.NodeStatus{
			newNode("foo").build(),
			newMaster("bar").degraded(&pb.Probe{
				Checker:  "qux",
				Status:   pb.Probe_Failed,
				Severity: pb.Probe_Critical,
				Error:    "not available",
			}).build(),
		},
	}

	actual := status
	setSystemStatus(&actual, r.members)

	expected := status
	expected.Status = pb.SystemStatus_Degraded

	c.Assert(actual, test.DeepCompare, expected, Commentf("Expected degraded system status."))
}

func (r *StatusSuite) TestDetectsNoMaster(c *C) {
	status := pb.SystemStatus{
		Nodes: []*pb.NodeStatus{
			newNode("foo").build(),
			newNode("bar").build(),
		},
	}

	actual := status
	setSystemStatus(&actual, r.members)

	expected := status
	expected.Status = pb.SystemStatus_Degraded
	expected.Summary = errNoMaster.Error()

	c.Assert(actual, test.DeepCompare, expected, Commentf("Expected degraded system status."))
}

func (r *StatusSuite) TestSetsOkSystemStatus(c *C) {
	status := pb.SystemStatus{
		Nodes: []*pb.NodeStatus{
			newNode("foo").build(),
			newMaster("bar").build(),
		},
	}
	actual := status
	setSystemStatus(&actual, r.members)

	expected := status
	expected.Status = pb.SystemStatus_Running

	c.Assert(actual, test.DeepCompare, expected, Commentf("Expected running system status."))
}
