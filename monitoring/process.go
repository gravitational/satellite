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
	"github.com/gravitational/satellite/utils"

	"github.com/gravitational/trace"

	"github.com/mitchellh/go-ps"
)

// ProcessChecker is checking aginst known list of conflicting processes
type ProcessChecker struct {
	// ProcessNames contains list of processes which should not be running at the time of check
	ProcessNames []string
}

const processCheckerID = "process-checker"

// DefaultProcessChecker returns checker which will ensure no conflicting program is running
func DefaultProcessChecker() *ProcessChecker {
	return &ProcessChecker{[]string{
		"dockerd",
		"lxd",
		"dnsmasq",
		"kube-apiserver",
		"kube-scheduler",
		"kube-controller-manager",
		"kube-proxy",
		"kubelet",
		"planet",
		"teleport",
	}}
}

// Name returns checker name
func (c *ProcessChecker) Name() string {
	return processCheckerID
}

// Check will query current process list and report for each conflicting program found
func (c *ProcessChecker) Check(ctx context.Context, r health.Reporter) {
	running, err := ps.Processes()
	if err != nil {
		r.Add(NewProbeFromErr(processCheckerID, "failed to obtain running process list", trace.Wrap(err)))
		return
	}

	prohibited := utils.NewStringSet()
	for _, process := range running {
		if utils.StringInSlice(c.ProcessNames, process.Executable()) {
			prohibited.Add(process.Executable())
		}
	}

	if len(prohibited) == 0 {
		r.Add(&pb.Probe{
			Checker: processCheckerID,
			Status:  pb.Probe_Running,
		})
		return
	}

	r.Add(&pb.Probe{
		Checker: processCheckerID,
		Detail:  fmt.Sprintf("conflicting programs: %v", prohibited.Slice()),
		Status:  pb.Probe_Failed,
	})

}
