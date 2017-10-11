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
	"runtime"

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/trace"

	"github.com/cloudfoundry/gosigar"
	"github.com/dustin/go-humanize"
)

const (
	hostCheckerID = "cpu-ram"
)

// HostChecker validates CPU and RAM requirements
type HostChecker struct {
	// MinCPU is minimum amount of logical CPUs
	MinCPU int
	// MinRamBytes is minimum amount of RAM
	MinRAMBytes uint64
}

// Name returns checker id
func (c *HostChecker) Name() string {
	return hostCheckerID
}

// Check runs check
func (c *HostChecker) Check(ctx context.Context, reporter health.Reporter) {
	failed := false

	var mem sigar.Mem
	err := mem.Get()
	if err != nil {
		reporter.Add(NewProbeFromErr(hostCheckerID, "querying memory info", trace.Wrap(err)))
	}

	if mem.Total < c.MinRAMBytes {
		failed = true
		reporter.Add(&pb.Probe{
			Checker: hostCheckerID,
			Detail: fmt.Sprintf("need minimum %s RAM, have %s",
				humanize.Bytes(c.MinRAMBytes), humanize.Bytes(mem.Total)),
			Status: pb.Probe_Failed,
		})
	}

	if runtime.NumCPU() < c.MinCPU {
		failed = true
		reporter.Add(&pb.Probe{
			Checker: hostCheckerID,
			Detail:  fmt.Sprintf("need minimum %d CPU, have %d", c.MinCPU, runtime.NumCPU()),
			Status:  pb.Probe_Failed,
		})
	}

	if failed {
		return
	}

	reporter.Add(&pb.Probe{
		Checker: hostCheckerID,
		Status:  pb.Probe_Running,
	})
}
