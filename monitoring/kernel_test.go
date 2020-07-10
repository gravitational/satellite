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

package monitoring

import (
	. "gopkg.in/check.v1"
)

type KernelSuite struct{}

var _ = Suite(&KernelSuite{})

// TestKernelSupported verifies that the checker correctly identifies
// supported/unsupported kernel versions.
func (s *KernelSuite) TestKernelSupported(c *C) {
	var testCases = []struct {
		comment  CommentInterface
		expected bool
		version  KernelVersion
	}{
		{
			comment:  Commentf("Exactly matches min kernel requirement."),
			expected: true,
			version:  KernelVersion{Release: 3, Major: 10, Minor: 1, Patch: 1127},
		},
		{
			comment:  Commentf("Does not meet release version requirement."),
			expected: false,
			version:  KernelVersion{Release: 2},
		},
		{
			comment:  Commentf("Does not meet major version requirement."),
			expected: false,
			version:  KernelVersion{Release: 3, Major: 9},
		},
		{
			comment:  Commentf("Does not meet minor version requirement."),
			expected: false,
			version:  KernelVersion{Release: 3, Major: 10, Minor: 0},
		},
		{
			comment:  Commentf("Does not meet patch number requirement."),
			expected: false,
			version:  KernelVersion{Release: 3, Major: 10, Minor: 1, Patch: 1126},
		},
		{
			comment:  Commentf("Minor version is greater than minimum supported version."),
			expected: true,
			version:  KernelVersion{Release: 3, Major: 10, Minor: 2, Patch: 1126},
		},
		{
			comment:  Commentf("Major version is greater than minimum supported version."),
			expected: true,
			version:  KernelVersion{Release: 3, Major: 11, Minor: 0, Patch: 1126},
		},
		{
			comment:  Commentf("Release version is greater than minimum supported version."),
			expected: true,
			version:  KernelVersion{Release: 4, Major: 9, Minor: 0, Patch: 1126},
		},
	}
	for _, testCase := range testCases {
		checker := NewKernelChecker(testMinKernelVersion)
		c.Assert(checker.isSupportedVersion(testCase.version), Equals, testCase.expected, testCase.comment)
	}
}

// testMinKernelVersion is the minimum supported kernel version used for test cases.
var testMinKernelVersion = KernelVersion{Release: 3, Major: 10, Minor: 1, Patch: 1127}
