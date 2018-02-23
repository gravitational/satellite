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

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/lib/test"

	. "gopkg.in/check.v1"
)

func (*MonitoringSuite) TestReadsMounts(c *C) {
	// exercise
	mounts, err := parseProcMounts(testCgroups)

	// verify
	c.Assert(err, IsNil)
	c.Assert(mounts, test.DeepCompare, []mountPoint{
		{Device: "rootfs", Path: "/", FsType: "rootfs", Options: []string{"rw"}},
		{Device: "cgroup", Path: "/sys/fs/cgroup/pids", FsType: cgroupMountType,
			Options: []string{"rw", "nosuid", "nodev", "noexec", "pids"}},
		{Device: "cgroup", Path: "/sys/fs/cgroup/cpu,cpuacct", FsType: cgroupMountType,
			Options: []string{"rw", "nosuid", "nodev", "noexec", "cpuacct", "cpu"}},
		{Device: "cgroup", Path: "/sys/fs/cgroup/memory", FsType: cgroupMountType,
			Options: []string{"rw", "nosuid", "nodev", "noexec", "memory"}},
	})
}

func (*MonitoringSuite) TestValidatesCGroupMounts(c *C) {
	// setup
	var testCases = []struct {
		cgroups []string
		probes  health.Probes
		comment string
	}{
		{
			cgroups: []string{"cpu", "memory"},
			probes:  health.Probes{&pb.Probe{Checker: cgroupCheckerID, Status: pb.Probe_Running}},
			comment: "all mounts available",
		},
		{
			cgroups: []string{"cpu", "memory", "blkio"},
			probes: health.Probes{
				&pb.Probe{
					Checker: cgroupCheckerID,
					Error:   `Following CGroups have not been mounted: ["blkio"]`,
					Status:  pb.Probe_Failed,
				},
			},
			comment: "missing cgroup mount",
		},
	}

	// exercise / verify
	for _, testCase := range testCases {
		checker := cgroupChecker{
			cgroups:   testCase.cgroups,
			getMounts: mountsReader(testCgroups),
		}
		var reporter health.Probes
		checker.Check(context.TODO(), &reporter)
		c.Assert(reporter, test.DeepCompare, testCase.probes, Commentf(testCase.comment))
	}
}

func mountsReader(data []byte) func() ([]mountPoint, error) {
	return func() ([]mountPoint, error) {
		return parseProcMounts(data)
	}
}

var testCgroups = []byte(`
rootfs / rootfs rw 0 0
cgroup /sys/fs/cgroup/pids cgroup rw,nosuid,nodev,noexec,pids 0 0
cgroup /sys/fs/cgroup/cpu,cpuacct cgroup rw,nosuid,nodev,noexec,cpuacct,cpu 0 0
cgroup /sys/fs/cgroup/memory cgroup rw,nosuid,nodev,noexec,memory 0 0
`)
