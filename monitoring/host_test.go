/*
Copyright 2017 Gravitational, Inc.

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

	sigar "github.com/cloudfoundry/gosigar"
	"github.com/gravitational/trace"
	. "gopkg.in/check.v1"
)

func (*MonitoringSuite) TestValidatesHostEnviron(c *C) {
	// setup
	var testCases = []struct {
		config    HostConfig
		probes    health.Probes
		getMemory memoryGetterFunc
		getCPU    cpuGetterFunc
		comment   string
	}{
		{
			config:    HostConfig{MinRAMBytes: 512, MinCPU: 2},
			probes:    health.Probes{NewSuccessProbe(hostCheckerID)},
			getMemory: testGetMemory(1024),
			getCPU:    testGetCPU(4),
			comment:   "host passes validation",
		},
		{
			config: HostConfig{MinRAMBytes: 1024, MinCPU: 4},
			probes: health.Probes{
				&pb.Probe{
					Checker: hostCheckerID,
					Detail:  "at least 1.0 kB of RAM required, only 512 B available",
					Status:  pb.Probe_Failed,
				},
				&pb.Probe{
					Checker: hostCheckerID,
					Detail:  "at least 4 CPUs required, only 2 available",
					Status:  pb.Probe_Failed,
				},
			},
			getMemory: testGetMemory(512),
			getCPU:    testGetCPU(2),
			comment:   "host fails validation",
		},
		{
			config:    HostConfig{MinRAMBytes: 512, MinCPU: 2},
			probes:    health.Probes{NewSuccessProbe(hostCheckerID)},
			getMemory: testFailingGetMemory(trace.NotFound("file or directory not found")),
			getCPU:    testGetCPU(4),
			comment:   "no error if memory information file is not available",
		},
		{
			config: HostConfig{MinRAMBytes: 512, MinCPU: 2},
			probes: health.Probes{
				&pb.Probe{
					Checker: hostCheckerID,
					Detail:  "failed to validate host environment",
					Error:   "failed to query memory info",
					Status:  pb.Probe_Failed,
				}},
			getMemory: testFailingGetMemory(fmt.Errorf("unable to read")),
			getCPU:    testGetCPU(4),
			comment:   "fails if unable to read memory information (other than not found)",
		},
	}

	// exercise / verify
	for _, testCase := range testCases {
		checker := &hostChecker{
			HostConfig: testCase.config,
			getMemory:  testCase.getMemory,
			getCPU:     testCase.getCPU,
		}
		var reporter health.Probes
		checker.Check(context.TODO(), &reporter)
		c.Assert(reporter, test.DeepCompare, testCase.probes, Commentf(testCase.comment))
	}
}

func testGetMemory(totalMB uint64) memoryGetterFunc {
	return func() (*sigar.Mem, error) {
		return &sigar.Mem{Total: totalMB}, nil
	}
}

func testFailingGetMemory(err error) memoryGetterFunc {
	return func() (*sigar.Mem, error) {
		return nil, err
	}
}

func testGetCPU(numCPU int) cpuGetterFunc {
	return func() int {
		return numCPU
	}
}
