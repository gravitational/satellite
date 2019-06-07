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

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"gopkg.in/check.v1"
)

type SysctlSuite struct{}

var _ = check.Suite(&SysctlSuite{})

func (s *SysctlSuite) TestFileHandleAllocatableChecker(c *check.C) {
	testCases := []struct {
		sysctl  string
		checker FileHandleAllocatableChecker
		probes  health.Probes
		comment string
	}{
		{
			sysctl: "2976	0	1533180",
			checker: FileHandleAllocatableChecker{
				Min: 1000,
			},
			probes: health.Probes{
				{
					Checker: "file-nr",
					Status:  pb.Probe_Running,
				},
			},
			comment: "healthy test",
		},
		{
			sysctl: "3000	0	3000",
			checker: FileHandleAllocatableChecker{
				Min: 1000,
			},
			probes: health.Probes{
				{
					Checker: "file-nr",
					Status:  pb.Probe_Failed,
					Detail:  "Available filehandles (0) is low. Please increase fs.file-max sysctl.",
				},
			},
			comment: "all handles in use",
		},
		{
			sysctl: "3001	0	3000",
			checker: FileHandleAllocatableChecker{
				Min: 1000,
			},
			probes: health.Probes{
				{
					Checker: "file-nr",
					Status:  pb.Probe_Failed,
					Detail:  "Available filehandles (-1) is low. Please increase fs.file-max sysctl.",
				},
			},
			comment: "filehandles exceed max",
		},
		{
			sysctl: "3001	0	3000	10",
			checker: FileHandleAllocatableChecker{
				Min: 1000,
			},
			probes: health.Probes{
				{
					Checker: "file-nr",
					Status:  pb.Probe_Failed,
					Detail:  "fs.file-nr expected 3 fields: 3001\t0\t3000\t10",
					Error:   "expected 3 fields",
				},
			},
			comment: "unexpected number of fields",
		},
		{
			sysctl: "invalid	0	3000",
			checker: FileHandleAllocatableChecker{
				Min: 1000,
			},
			probes: health.Probes{
				{
					Checker: "file-nr",
					Status:  pb.Probe_Failed,
					Detail:  "invalid is not a number",
					Error:   "strconv.Atoi: parsing \"invalid\": invalid syntax",
				},
			},
			comment: "allocatedFH invalid",
		},
		{
			sysctl: "3001	0	invalid",
			checker: FileHandleAllocatableChecker{
				Min: 1000,
			},
			probes: health.Probes{
				{
					Checker: "file-nr",
					Status:  pb.Probe_Failed,
					Detail:  "invalid is not a number",
					Error:   "strconv.Atoi: parsing \"invalid\": invalid syntax",
				},
			},
			comment: "maxFH invalid",
		},
	}

	for _, testCase := range testCases {
		var reporter health.Probes
		testCase.checker.check(context.TODO(), &reporter, testCase.sysctl)
		c.Assert(reporter, check.DeepEquals, testCase.probes, check.Commentf(testCase.comment))
	}
}
