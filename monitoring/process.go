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
	"github.com/gravitational/trace"
	"github.com/prometheus/procfs"
)

func NewProcessChecker(process string) health.Checker {
	return &ProcessChecker{
		Process: process,
	}
}

type ProcessChecker struct {
	Process string
}

func (c *ProcessChecker) Name() string {
	return fmt.Sprintf("process %s", c.Process)
}

func (c *ProcessChecker) Check(ctx context.Context, reporter health.Reporter) {
	procs, err := procfs.AllProcs()
	if err != nil {
		reporter.Add(NewProbeFromErr(c.Name(), trace.Errorf("failed to list processes: %v", err)))
		return
	}
	for _, proc := range procs {
		command, _ := proc.Comm()
		procstat, err := proc.NewStat()
		if err != nil {
			reporter.Add(NewProbeFromErr(c.Name(), trace.Errorf("failed to get %s process statistics: %v", command, err)))
			return
		}
		if command == c.Process && procstat.PPID == 1 {
			reporter.Add(&pb.Probe{Checker: c.Name(), Status: pb.Probe_Running})
			return
		}
	}
	reporter.Add(NewProbeFromErr(c.Name(), trace.Errorf("no process %v found", c.Process)))
}
