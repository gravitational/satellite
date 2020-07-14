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

package monitoring_test

import (
	"github.com/gravitational/satellite/lib/test"
	"github.com/gravitational/satellite/monitoring"

	. "gopkg.in/check.v1"
)

type KernelSuite struct{}

var _ = Suite(&KernelSuite{})

// TestParseKernelVersion verifies that the ParseKernelVersion correctly
// parses the kernel version string into a KernelVersion struct.
func (*KernelSuite) TestParsesKernelVersion(c *C) {
	// setup
	var testCases = []struct {
		release  string
		expected monitoring.KernelVersion
		comment  string
	}{
		{
			release:  "4.4.0-112-generic",
			expected: monitoring.KernelVersion{4, 4, 0, 112},
			comment:  "ubuntu 16.04",
		},
		{
			release:  "3.10.0-514.16.1.el7.x86_64",
			expected: monitoring.KernelVersion{3, 10, 0, 514},
			comment:  "centos 7.4",
		},
		{
			release:  "4.9.0-4-amd64",
			expected: monitoring.KernelVersion{4, 9, 0, 4},
			comment:  "debian 9.3",
		},
	}

	// exercise / verify
	for _, testCase := range testCases {
		version, err := monitoring.ParseKernelVersion(testCase.release)
		c.Assert(err, IsNil)
		c.Assert(version, test.DeepCompare, &testCase.expected, Commentf(testCase.comment))
	}
}

// TestKernelVersionLessThan verifies that KernelVersionLessThan correctly
// compares the the specified kernel versions.
func (*KernelSuite) TestKernelVersionLessThan(c *C) {
	var testCases = []struct {
		comment  CommentInterface
		expected bool
		version  monitoring.KernelVersion
	}{
		{
			comment:  Commentf("Exactly matches test version."),
			expected: false,
			version:  monitoring.KernelVersion{Release: 3, Major: 10, Minor: 1, Patch: 1127},
		},
		{
			comment:  Commentf("Older release version."),
			expected: true,
			version:  monitoring.KernelVersion{Release: 2},
		},
		{
			comment:  Commentf("Older release version & greater major component."),
			expected: true,
			version:  monitoring.KernelVersion{Release: 2, Major: 11, Minor: 1, Patch: 1127},
		},
		{
			comment:  Commentf("Older major component."),
			expected: true,
			version:  monitoring.KernelVersion{Release: 3, Major: 9},
		},
		{
			comment:  Commentf("Older minor component."),
			expected: true,
			version:  monitoring.KernelVersion{Release: 3, Major: 10, Minor: 0},
		},
		{
			comment:  Commentf("Older patch number."),
			expected: true,
			version:  monitoring.KernelVersion{Release: 3, Major: 10, Minor: 1, Patch: 1126},
		},
		{
			comment:  Commentf("Newer minor component."),
			expected: false,
			version:  monitoring.KernelVersion{Release: 3, Major: 10, Minor: 2, Patch: 1126},
		},
		{
			comment:  Commentf("Newer major component."),
			expected: false,
			version:  monitoring.KernelVersion{Release: 3, Major: 11, Minor: 0, Patch: 1126},
		},
		{
			comment:  Commentf("Newer release version."),
			expected: false,
			version:  monitoring.KernelVersion{Release: 4, Major: 9, Minor: 0, Patch: 1126},
		},
	}

	for _, testCase := range testCases {
		result := monitoring.KernelVersionLessThan(testKernelVersion)(testCase.version)
		c.Assert(result, Equals, testCase.expected, testCase.comment)
	}
}

// testKernelVersion is arbitrary kernel version used to test KernelVersionLessThan function.
var testKernelVersion = monitoring.KernelVersion{Release: 3, Major: 10, Minor: 1, Patch: 1127}
