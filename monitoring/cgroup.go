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
	"bufio"
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"os"
	"strings"

	"github.com/gravitational/satellite/agent/health"
	"github.com/gravitational/satellite/utils"

	"github.com/gravitational/trace"
)

// NewCGroupChecker creates a new checker to verify existence
// of cgroup mounts given with cgroups.
// The checker should be executed in the host mount namespace.
func NewCGroupChecker(cgroups ...string) health.Checker {
	return &cgroupChecker{
		cgroups:   cgroups,
		getMounts: listProcMounts,
	}
}

// cgroupChecker is a checker that verifies existence
// of a set of cgroup mounts
type cgroupChecker struct {
	cgroups   []string
	getMounts mountGetterFunc
}

// Name returns name of the checker
// Implements health.Checker
func (c *cgroupChecker) Name() string {
	return cgroupCheckerID
}

// Check verifies existence of cgroup mounts given in c.cgroups.
// Implements health.Checker
func (c *cgroupChecker) Check(ctx context.Context, reporter health.Reporter) {
	probes, err := c.check(ctx)
	if err != nil {
		probes.Add(NewProbeFromErr(c.Name(), "failed to validate cgroup mounts", err))
	}
	health.AddFrom(reporter, probes)
}

// check verifies that all expected cgroups have been mounted. Skips check if
// mounts file is not available.
func (c *cgroupChecker) check(ctx context.Context) (probes health.Reporter, err error) {
	probes = &health.Probes{}

	mounts, err := c.getMounts(ctx)

	// Skip check if mounts file is not available
	if trace.IsNotFound(err) {
		probes.Add(NewSuccessProbe(c.Name()))
		return probes, nil
	}

	if err != nil {
		return probes, trace.Wrap(err, "failed to read mounts file")
	}

	expectedCgroups := utils.NewStringSetFromSlice(c.cgroups)
	for _, mount := range mounts {
		if mount.FsType == cgroupMountType {
			for _, opt := range mount.Options {
				if expectedCgroups.Has(opt) {
					expectedCgroups.Remove(opt)
				}
			}
		}
	}
	unmountedCgroups := expectedCgroups.Slice()
	if len(unmountedCgroups) > 0 {
		return probes, trace.NotFound("following CGroups have not been mounted: %q", unmountedCgroups)
	}

	probes.Add(NewSuccessProbe(c.Name()))
	return probes, nil
}

// listProcMounts returns the set of active mounts by interpreting
// the /proc/mounts file.
// The code is adopted from the kubernetes project.
func listProcMounts(ctx context.Context) ([]mountPoint, error) {
	content, err := consistentRead(ctx, mountFilePath, maxListTries)
	if err != nil {
		return nil, err
	}
	return parseProcMounts(content)
}

// consistentRead repeatedly reads a file until it gets the same content twice.
// This is useful when reading files in /proc that are larger than page size
// and kernel may modify them between individual read() syscalls.
func consistentRead(ctx context.Context, filename string, attempts int) ([]byte, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, trace.Wrap(err, "unable to open %s", filename)
	}
	defer file.Close()

	// Wrap file reader with a context. ReadAll will be interrupted when context is done
	reader := &ReaderWithContext{ctx, bufio.NewReader(file)}

	oldContent, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, trace.ConvertSystemError(err)
	}
	for i := 0; i < attempts; i++ {
		newContent, err := ioutil.ReadAll(reader)
		if err != nil {
			return nil, trace.ConvertSystemError(err)
		}
		if bytes.Compare(oldContent, newContent) == 0 {
			return newContent, nil
		}
		// Files are different, continue reading
		oldContent = newContent
	}
	return nil, trace.LimitExceeded("failed to get consistent content of %v after %v attempts",
		filename, attempts)
}

func parseProcMounts(content []byte) (mounts []mountPoint, err error) {
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		if line == "" {
			// the last split() item is empty string following the last \n
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != expectedNumFieldsPerLine {
			return nil, trace.BadParameter("wrong number of fields (expected %v, got %v): %s",
				expectedNumFieldsPerLine, len(fields), line)
		}

		mount := mountPoint{
			Device:  fields[0],
			Path:    fields[1],
			FsType:  fields[2],
			Options: strings.Split(fields[3], ","),
		}

		mounts = append(mounts, mount)
	}
	return mounts, nil
}

// ReaderWithContext wraps a reader with a context.
type ReaderWithContext struct {
	ctx    context.Context
	reader io.Reader
}

// Read reads from the underlying reader. Return without reading if context is
// done.
//
// Implements io.Reader
func (r *ReaderWithContext) Read(p []byte) (n int, err error) {
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	default:
		return r.reader.Read(p)
	}
}

// mountPoint desribes a mounting point as defined in /etc/mtab or /proc/mounts
// See https://linux.die.net/man/5/fstab
type mountPoint struct {
	// Device specifies the mounted device
	Device string
	// Path is the mounting path
	Path string
	// FsType defines the type of the file system
	FsType string
	// Options for the mount (read-only or read-write, etc.)
	Options []string
}

type mountGetterFunc func(ctx context.Context) ([]mountPoint, error)

// mountFilePath specifies the location of the mount information file
const mountFilePath = "/proc/mounts"

// maxListTries defines the maximum number of attempts to read file contents
const maxListTries = 3

// expectedNumFieldsPerLine specifies the number of fields per line in
// /proc/mounts as per the fstab man page.
const expectedNumFieldsPerLine = 6

// cgroupMountType specifies the filesystem type for CGroup mounts
const cgroupMountType = "cgroup"

const cgroupCheckerID = "cgroup-mounts"
