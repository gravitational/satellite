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
	"bytes"

	. "gopkg.in/check.v1"
)

func (_ *ProtoSuite) TestMarshalsMemberStatus(c *C) {
	var tests = []struct {
		status   MemberStatus_Type
		expected []byte
	}{
		{
			status:   MemberStatus_Alive,
			expected: []byte("alive"),
		},
		{
			status:   MemberStatus_Leaving,
			expected: []byte("leaving"),
		},
		{
			status:   MemberStatus_Left,
			expected: []byte("left"),
		},
		{
			status:   MemberStatus_Failed,
			expected: []byte("failed"),
		},
		{
			status:   MemberStatus_None,
			expected: nil,
		},
	}

	for _, test := range tests {
		text, err := test.status.MarshalText()
		c.Assert(err, IsNil)
		if !bytes.Equal(text, test.expected) {
			c.Errorf("expected %s but got %s", test.expected, text)
		}
		status := new(MemberStatus_Type)
		c.Assert(status.UnmarshalText(text), IsNil)
		c.Assert(test.status, Equals, *status)
	}
}

func (_ *ProtoSuite) TestMarshalsSystemStatus(c *C) {
	var tests = []struct {
		status   SystemStatus_Type
		expected []byte
	}{
		{
			status:   SystemStatus_Running,
			expected: []byte("running"),
		},
		{
			status:   SystemStatus_Degraded,
			expected: []byte("degraded"),
		},
		{
			status:   SystemStatus_Unknown,
			expected: nil,
		},
	}

	for _, test := range tests {
		text, err := test.status.MarshalText()
		c.Assert(err, IsNil)
		if !bytes.Equal(text, test.expected) {
			c.Errorf("expected %s but got %s", test.expected, text)
		}
		status := new(SystemStatus_Type)
		c.Assert(status.UnmarshalText(text), IsNil)
		c.Assert(test.status, DeepEquals, *status)
	}
}

func (_ *ProtoSuite) TestMarshalsNodeStatus(c *C) {
	var tests = []struct {
		status   NodeStatus_Type
		expected []byte
	}{
		{
			status:   NodeStatus_Running,
			expected: []byte("running"),
		},
		{
			status:   NodeStatus_Degraded,
			expected: []byte("degraded"),
		},
		{
			status:   NodeStatus_Unknown,
			expected: nil,
		},
	}

	for _, test := range tests {
		text, err := test.status.MarshalText()
		c.Assert(err, IsNil)
		if !bytes.Equal(text, test.expected) {
			c.Errorf("expected %s but got %s", test.expected, text)
		}
		status := new(NodeStatus_Type)
		c.Assert(status.UnmarshalText(text), IsNil)
		c.Assert(test.status, DeepEquals, *status)
	}
}
