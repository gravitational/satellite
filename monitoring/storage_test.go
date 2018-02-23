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

package monitoring

import (
	"context"
	"fmt"
	"path"
	"time"

	"github.com/gravitational/satellite/agent/health"

	sigar "github.com/cloudfoundry/gosigar"
	. "gopkg.in/check.v1"
)

const (
	shallSucceed = true
	shallFail    = false
)

type StorageSuite struct{}

var _ = Suite(&StorageSuite{})

func (_ *StorageSuite) TestStorage(c *C) {
	mounts := mountList{
		sigar.FileSystem{DevName: "tmpfs", DirName: "/tmp", SysTypeName: "tmpfs"},
	}

	tmp, err := fsFromPath("/tmp", mounts)
	c.Assert(err, IsNil)

	c.Logf("%+v", tmp)

	storageChecker{
		StorageConfig: StorageConfig{
			Path: "/tmp",
		},
		osInterface: testOS{mountList: mounts},
	}.probe(c, "no conditions", shallSucceed)

	storageChecker{
		StorageConfig: StorageConfig{
			Path:              path.Join("/tmp", "does-not-exist"),
			WillBeCreated:     true,
			Filesystems:       []string{tmp.SysTypeName},
			MinFreeBytes:      uint64(1024),
			MinBytesPerSecond: uint64(1024),
		},
		osInterface: testOS{mountList: mounts, bytesPerSecond: 512, bytesAvail: 512},
	}.probe(c, "unreasonable requirements", shallFail)

	storageChecker{
		StorageConfig: StorageConfig{
			Path:              path.Join("/tmp", "does-not-exist"),
			WillBeCreated:     true,
			Filesystems:       []string{tmp.SysTypeName},
			MinFreeBytes:      uint64(1024),
			MinBytesPerSecond: uint64(1024),
		},
		osInterface: testOS{mountList: mounts, bytesPerSecond: 2048, bytesAvail: 2048},
	}.probe(c, "reasonable requirements", shallSucceed)

	storageChecker{
		StorageConfig: StorageConfig{
			Path:          path.Join("/tmp", fmt.Sprintf("%d", time.Now().Unix())),
			WillBeCreated: true,
			Filesystems:   []string{"no-such-fs"},
		},
		osInterface: testOS{mountList: mounts},
	}.probe(c, "fs type mismatch", shallFail)

	storageChecker{
		StorageConfig: StorageConfig{
			Path:          path.Join("/tmp", fmt.Sprintf("%d", time.Now().Unix())),
			WillBeCreated: false,
		},
		osInterface: testOS{mountList: mounts},
	}.probe(c, "missing folder", shallFail)

	storageChecker{
		StorageConfig: StorageConfig{
			Path:          path.Join("/tmp", fmt.Sprintf("%d", time.Now().Unix())),
			WillBeCreated: true,
		},
		osInterface: testOS{mountList: mounts},
	}.probe(c, "create if missing", shallSucceed)
}

func (_ *StorageSuite) TestMatchesFilesystem(c *C) {
	mounts := mountList{
		sigar.FileSystem{DevName: "rootfs", DirName: "/", SysTypeName: "rootfs", Options: "rw"},
		sigar.FileSystem{DevName: "/dev/mapper/VolGroup00-LogVol00", DirName: "/", SysTypeName: "xfs"},
		sigar.FileSystem{DevName: "sysfs", DirName: "/sys", SysTypeName: "sysfs", Options: "rw,seclabel,nosuid,nodev,noexec,relatime"},
	}

	storageChecker{
		StorageConfig: StorageConfig{
			Path:          "/var/lib/data",
			WillBeCreated: true,
			Filesystems:   []string{"xfs", "ext4"},
		},
		osInterface: testOS{mountList: mounts},
	}.probe(c, "discards rootfs", shallSucceed)
}

func (ch storageChecker) probe(c *C, msg string, success bool) {
	var probes health.Probes

	ch.Check(context.TODO(), &probes)

	failed := probes.GetFailed()
	if success && len(failed) == 0 {
		return
	}

	c.Logf("%q failed probes:\n", msg)
	for i, probe := range failed {
		c.Logf("[%d] %s: %s %s\n",
			i+1, probe.Checker, probe.Detail, probe.Error)
	}

	if success != (len(failed) == 0) {
		c.Fail()
	}
}

type testOS struct {
	mountList
	bytesPerSecond
	bytesAvail
}

func (r mountList) mounts() ([]sigar.FileSystem, error) {
	return r, nil
}

type mountList []sigar.FileSystem

func (r bytesPerSecond) diskSpeed(context.Context, string, string) (uint64, error) {
	return uint64(r), nil
}

func (r bytesAvail) diskCapacity(string) (uint64, error) {
	return uint64(r), nil
}

type bytesPerSecond uint64
type bytesAvail uint64
