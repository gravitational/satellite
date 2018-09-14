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

	"golang.org/x/sys/unix"
)

func NewINotifyChecker() health.Checker {
	return iNotifyChecker{}
}

// iNotifyChecker tries to create an inotify watch, and reports if it fails
type iNotifyChecker struct {
}

// Name returns name of the checker
func (c iNotifyChecker) Name() string {
	return "inotify"
}

func (c iNotifyChecker) Check(ctx context.Context, reporter health.Reporter) {
	fd, err := unix.InotifyInit()
	if fd < 0 {
		reporter.Add(&pb.Probe{
			Checker: c.Name(),
			Status:  pb.Probe_Failed,
			Error:   "Unable to initialize inotify",
		})
	}
	defer unix.Close(fd)

	if _, err = unix.InotifyAddWatch(fd, "/", unix.IN_CREATE); err != nil {
		reporter.Add(&pb.Probe{
			Checker: c.Name(),
			Status:  pb.Probe_Failed,
			Error:   "Unable to create intofiy watch",
		})
	}
}
