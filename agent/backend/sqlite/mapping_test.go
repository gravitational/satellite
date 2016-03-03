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
package sqlite

import (
	"testing"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"

	. "gopkg.in/check.v1"
)

func TestMapping(t *testing.T) { TestingT(t) }

type Mapping struct{}

var _ = Suite(&Mapping{})

func (_ *Mapping) TestTranslatesMemberStatus(c *C) {
	var tests = []struct {
		status   pb.MemberStatus_Type
		expected []byte
	}{
		{
			status:   pb.MemberStatus_Alive,
			expected: []byte("A"),
		},
		{
			status:   pb.MemberStatus_Leaving,
			expected: []byte("G"),
		},
		{
			status:   pb.MemberStatus_Left,
			expected: []byte("L"),
		},
		{
			status:   pb.MemberStatus_Failed,
			expected: []byte("F"),
		},
		{
			status:   pb.MemberStatus_None,
			expected: []byte("F"),
		},
	}

	for _, test := range tests {
		status := protoToMemberStatus(test.status)
		c.Assert(status, Equals, memberStatusType(string(test.expected)))
		var actual memberStatusType
		c.Assert(actual.Scan(test.expected), IsNil)
		c.Assert(string(actual), Equals, string(test.expected))
	}
}

func (_ *Mapping) TestTranslatesSystemStatus(c *C) {
	var tests = []struct {
		status   pb.SystemStatus_Type
		expected []byte
	}{
		{
			status:   pb.SystemStatus_Running,
			expected: []byte("H"),
		},
		{
			status:   pb.SystemStatus_Degraded,
			expected: []byte("F"),
		},
		{
			status:   pb.SystemStatus_Unknown,
			expected: []byte("F"),
		},
	}

	for _, test := range tests {
		status := protoToSystemStatus(test.status)
		c.Assert(status, Equals, systemStatusType(string(test.expected)))
		var actual systemStatusType
		c.Assert(actual.Scan(test.expected), IsNil)
		c.Assert(string(actual), Equals, string(test.expected))
	}
}

func (_ *Mapping) TestTranslatesNodeStatus(c *C) {
	var tests = []struct {
		status   pb.NodeStatus_Type
		expected []byte
	}{
		{
			status:   pb.NodeStatus_Running,
			expected: []byte("H"),
		},
		{
			status:   pb.NodeStatus_Degraded,
			expected: []byte("F"),
		},
		{
			status:   pb.NodeStatus_Unknown,
			expected: []byte("F"),
		},
	}

	for _, test := range tests {
		status := protoToNodeStatus(test.status)
		c.Assert(status, Equals, nodeStatusType(string(test.expected)))
		var actual nodeStatusType
		c.Assert(actual.Scan(test.expected), IsNil)
		c.Assert(string(actual), Equals, string(test.expected))
	}
}

func (_ *Mapping) TestTranslatesProbeType(c *C) {
	var tests = []struct {
		status   pb.Probe_Type
		expected []byte
	}{
		{
			status:   pb.Probe_Running,
			expected: []byte("H"),
		},
		{
			status:   pb.Probe_Failed,
			expected: []byte("F"),
		},
		{
			status:   pb.Probe_Unknown,
			expected: []byte("F"),
		},
	}

	for _, test := range tests {
		status := protoToProbe(test.status)
		c.Assert(status, Equals, probeType(string(test.expected)))
		var actual probeType
		c.Assert(actual.Scan(test.expected), IsNil)
		c.Assert(string(actual), Equals, string(test.expected))
	}
}
