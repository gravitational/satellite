/*
Copyright 2016 Gravitational, Inc.

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

package health

import (
	"testing"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"

	check "gopkg.in/check.v1"
)

func TestHealth(t *testing.T) { check.TestingT(t) }

type HealthSuite struct{}

var _ = check.Suite(&HealthSuite{})

func (_ *HealthSuite) TestSetsNodeStatusFromProbes(c *check.C) {
	var r Probes
	for _, probe := range probes() {
		r.Add(probe)
	}
	c.Assert(r.Status(), check.Equals, pb.NodeStatus_Degraded)
}

func probes() []*pb.Probe {
	probes := []*pb.Probe{
		&pb.Probe{
			Checker: "foo",
			Status:  pb.Probe_Failed,
			Error:   "not found",
		},
		&pb.Probe{
			Checker: "bar",
			Status:  pb.Probe_Running,
		},
	}
	return probes
}
