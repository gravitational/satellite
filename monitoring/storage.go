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

// +build linux

package monitoring

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/utils"

	"github.com/gravitational/trace"

	sigar "github.com/cloudfoundry/gosigar"
	"github.com/dustin/go-humanize"
	syscall "golang.org/x/sys/unix"
)

// StorageChecker verifies volume matches requirements
type StorageChecker struct {
	// Path represents volume to be checked
	Path string
	// WillBeCreated when true, then all checks will be applied to first existing dir, or fail otherwise
	WillBeCreated bool
	// path is normalized path, representing parent directory
	// in case Path does not exist yet
	path string
	// MinBytesPerSecond is minimum write speed for probe to succeed
	MinBytesPerSecond uint64
	// Filesystems define list of supported filesystems, or any if empty
	Filesystems []string
	// MinFreeBytes define minimum free volume capacity
	MinFreeBytes uint64
}

const (
	storageWriteCheckerID = "io-check"
	blockSize             = 1e5
	cycles                = 1024
	stRdonly              = int64(1)
)

// Name returns name of the checker
func (c *StorageChecker) Name() string {
	return fmt.Sprintf("%s(%s)", storageWriteCheckerID, c.Path)
}

func (c *StorageChecker) Check(ctx context.Context, reporter health.Reporter) {
	err := c.check(ctx, reporter)
	if err != nil {
		reporter.Add(NewProbeFromErr(c.Name(), "internal error", trace.Wrap(err)))
	} else {
		reporter.Add(&pb.Probe{Checker: c.Name(), Status: pb.Probe_Running})
	}
}

func (c *StorageChecker) check(ctx context.Context, reporter health.Reporter) error {
	err := c.evalPath()
	if err != nil {
		return trace.Wrap(err)
	}

	return trace.NewAggregate(c.checkFsType(ctx, reporter),
		c.checkCapacity(ctx, reporter),
		c.checkWriteSpeed(ctx, reporter))
}

// cleanPath returns fully evaluated path with symlinks resolved
// if path doesn't exist but should be created, then it returns
// first available parent directory, and checks will be applied to it
func (c *StorageChecker) evalPath() error {
	p := c.Path
	for {
		fi, err := os.Stat(p)

		if err != nil && !os.IsNotExist(err) {
			return trace.ConvertSystemError(err)
		}

		if os.IsNotExist(err) && !c.WillBeCreated {
			return trace.BadParameter("%s does not exist", c.Path)
		}

		if err == nil && fi.IsDir() {
			c.path = p
			return nil
		} else if err == nil && !fi.IsDir() {
			return trace.BadParameter("%s is not a directory", p)
		}

		parent := filepath.Dir(p)
		if parent == p {
			// shouldn't happen: root reached and it's not a dir
			return trace.BadParameter("%s is root and is not a dir", p)
		}
		p = parent
	}
}

func (c *StorageChecker) checkFsType(ctx context.Context, reporter health.Reporter) error {
	if len(c.Filesystems) == 0 {
		return nil
	}

	mnt, err := fsFromPath(c.path)
	if err != nil {
		return trace.Wrap(err)
	}

	probe := &pb.Probe{Checker: c.Name()}

	if utils.StringInSlice(c.Filesystems, mnt.SysTypeName) {
		probe.Status = pb.Probe_Running
	} else {
		probe.Status = pb.Probe_Failed
		probe.Detail = fmt.Sprintf("path %s requires filesystem %v, belongs to %s mount point of type %s",
			c.Path, c.Filesystems, mnt.DirName, mnt.SysTypeName)
	}
	reporter.Add(probe)
	return nil
}

type childPathFirst []sigar.FileSystem

func (a childPathFirst) Len() int           { return len(a) }
func (a childPathFirst) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a childPathFirst) Less(i, j int) bool { return strings.HasPrefix(a[i].DirName, a[j].DirName) }

func fsFromPath(path string) (*sigar.FileSystem, error) {
	cleanpath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	mounts := sigar.FileSystemList{}
	err = mounts.Get()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	sort.Sort(childPathFirst(mounts.List))

	for _, mnt := range mounts.List {
		if strings.HasPrefix(cleanpath, mnt.DirName) {
			return &mnt, nil
		}
	}

	return nil, trace.BadParameter("failed to locate filesystem for %s", path)
}

func (c *StorageChecker) checkCapacity(ctx context.Context, reporter health.Reporter) error {
	var stat syscall.Statfs_t

	err := syscall.Statfs(c.path, &stat)
	if err != nil {
		return trace.Wrap(err)
	}

	avail := uint64(stat.Bsize) * stat.Bavail
	if avail < c.MinFreeBytes {
		reporter.Add(&pb.Probe{
			Checker: c.Name(),
			Detail: fmt.Sprintf("%s available space left on %s, minimum of %s required",
				humanize.Bytes(avail), c.Path, humanize.Bytes(c.MinFreeBytes)),
			Status: pb.Probe_Failed,
		})
	}

	return nil
}

func (c *StorageChecker) checkWriteSpeed(ctx context.Context, reporter health.Reporter) (err error) {
	if c.MinBytesPerSecond == 0 {
		return
	}

	var file *os.File
	file, err = ioutil.TempFile(c.path, "probe")
	if err != nil {
		return trace.ConvertSystemError(err)
	}

	defer func() {
		err = trace.NewAggregate(
			trace.ConvertSystemError(file.Close()),
			trace.ConvertSystemError(os.Remove(file.Name())))
	}()

	start := time.Now()
	for i := 0; i < cycles; i++ {
		buf := make([]byte, blockSize)
		_, err = file.Write(buf)
		if err != nil {
			return trace.Wrap(err)
		}
		if ctx.Err() != nil {
			return trace.Wrap(ctx.Err())
		}
	}
	err = file.Sync()
	if err != nil {
		return trace.Wrap(err)
	}

	elapsed := time.Since(start).Seconds()
	bps := uint64(blockSize * cycles / elapsed)

	if bps >= c.MinBytesPerSecond {
		return nil
	}
	reporter.Add(&pb.Probe{
		Checker: c.Name(),
		Detail: fmt.Sprintf("min write speed %s/sec required, have %s",
			humanize.Bytes(c.MinBytesPerSecond), humanize.Bytes(bps)),
		Status: pb.Probe_Failed,
	})
	return nil
}
