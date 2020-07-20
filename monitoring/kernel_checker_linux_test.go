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
	"context"
	"fmt"

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/lib/test"

	. "gopkg.in/check.v1"
)

type KernelVersionSuite struct{}

var _ = Suite(&KernelVersionSuite{})

// TestKernelSupported verifies that the checker correctly identifies
// supported/unsupported kernel versions.
func (s *KernelVersionSuite) TestKernelSupported(c *C) {
	var testCases = []struct {
		comment CommentInterface
		kernelVersionReader
		probe *pb.Probe
	}{
		{
			comment:             Commentf("RHEL 6.0"),
			kernelVersionReader: staticKernelVersion("2.6.32-71"),
			probe: &pb.Probe{
				Checker: kernelCheckerID,
				Detail: fmt.Sprintf("Minimum recommended kernel version is %s (%s is installed).",
					testMinKernelVersion.String(), "2.6.32-71"),
				Status:   pb.Probe_Failed,
				Severity: pb.Probe_Warning,
			},
		},
		{
			comment:             Commentf("RHEL 7.6"),
			kernelVersionReader: staticKernelVersion("3.10.0-957.12.2.el7.x86_64"),
			probe: &pb.Probe{
				Checker: kernelCheckerID,
				Detail: fmt.Sprintf("Minimum recommended kernel version is %s (%s is installed).",
					testMinKernelVersion.String(), "3.10.0-957.12.2.el7.x86_64"),
				Status:   pb.Probe_Failed,
				Severity: pb.Probe_Warning,
			},
		},
		{
			comment:             Commentf("RHEL 7.8"),
			kernelVersionReader: staticKernelVersion("3.10.0-1127.13.1.el7.x86_64"),
			probe: &pb.Probe{
				Checker: kernelCheckerID,
				Detail:  fmt.Sprintf("Installed Linux kernel is supported: %s.", "3.10.0-1127.13.1.el7.x86_64"),
				Status:  pb.Probe_Running,
			},
		},
		{
			comment:             Commentf("Debian 9.3"),
			kernelVersionReader: staticKernelVersion("4.9.0-4-amd64"),
			probe: &pb.Probe{
				Checker: kernelCheckerID,
				Detail:  fmt.Sprintf("Installed Linux kernel is supported: %s.", "4.9.0-4-amd64"),
				Status:  pb.Probe_Running,
			},
		},
		{
			comment:             Commentf("Ubuntu 19.10"),
			kernelVersionReader: staticKernelVersion("5.3.0-62-generic"),
			probe: &pb.Probe{
				Checker: kernelCheckerID,
				Detail:  fmt.Sprintf("Installed Linux kernel is supported: %s.", "5.3.0-62-generic"),
				Status:  pb.Probe_Running,
			},
		},
	}
	for _, testCase := range testCases {
		checker := kernelChecker{
			MinKernelVersion:    testMinKernelVersion,
			kernelVersionReader: testCase.kernelVersionReader,
		}
		var reporter health.Probes
		checker.Check(context.TODO(), &reporter)
		c.Assert(reporter.GetProbes(), test.DeepCompare, []*pb.Probe{testCase.probe}, testCase.comment)
	}
}

// testMinKernelVersion is the minimum supported kernel version used for test cases.
var testMinKernelVersion = KernelVersion{Release: 3, Major: 10, Minor: 0, Patch: 1127}
