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

package agentpb

import (
	"time"

	. "gopkg.in/check.v1"
)

func (_ *ProtoSuite) TestClonesSystemStatus(c *C) {
	// setup
	timestamp := time.Date(1984, time.April, 4, 0, 0, 0, 0, time.UTC)
	systemStatus := &SystemStatus{
		Status:    SystemStatus_Degraded,
		Timestamp: NewTimeToProto(timestamp),
		Nodes: []*NodeStatus{
			&NodeStatus{
				Name: "node-1",
				MemberStatus: &MemberStatus{
					Name:   "node-1",
					Addr:   "addr",
					Status: MemberStatus_Alive,
					Tags: map[string]string{
						"foo": "bar",
					},
				},
				Status: NodeStatus_Running,
				Probes: []*Probe{
					&Probe{
						Checker: "service-1",
						Status:  Probe_Running,
					},
				},
			},
		},
		Summary: "master node unavailable",
	}

	// verify
	statusCopy := systemStatus.Clone()
	// edit the original object to ensure that cloning returns a deep-copy
	systemStatus.Nodes[0].Probes[0].Status = Probe_Failed
	c.Assert(statusCopy, DeepEquals, systemStatus)
}
