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

package rpc

import (
	"net"
	"testing"

	"github.com/gravitational/trace"
	. "gopkg.in/check.v1"
)

func Test(t *testing.T) { TestingT(t) }

type S struct{}

var _ = Suite(&S{})

func (*S) TestSplitsHostPort(c *C) {
	var testCases = []struct {
		input       string
		defaultPort int
		expected    *net.TCPAddr
		err         error
		comment     string
	}{
		{
			input:    "127.0.0.1:53",
			expected: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 53},
			comment:  "successful split",
		},
		{
			input:       "127.0.0.1",
			defaultPort: 53,
			expected:    &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 53},
			comment:     "missing port added",
		},
		{
			// See: https://tools.ietf.org/html/rfc6761#section-6.4
			input:       "something.invalid",
			defaultPort: 53,
			err:         &net.DNSError{},
			comment:     "unresolvable host",
		},
	}
	for _, tc := range testCases {
		comment := Commentf(tc.comment)
		addr, err := ParseTCPAddr(tc.input, tc.defaultPort)
		if tc.err != nil {
			c.Assert(trace.Unwrap(err), FitsTypeOf, tc.err)
		} else {
			c.Assert(err, IsNil)
			c.Assert(addr, DeepEquals, tc.expected, comment)
		}
	}
}
