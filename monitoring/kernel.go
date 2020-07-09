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

	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

const (
	kernelCheckerID = "kernel-check"
)

// kernelChecker is a checker that verifies that the linux kernel version is
// supported by Gravity.
type kernelChecker struct {
	// MinKernelVersion specifies the minimum supported kernel version.
	MinKernelVersion KernelVersion
}

// NewKernelChecker returns a new instance of kernel checker.
func NewKernelChecker(version KernelVersion) *kernelChecker {
	return &kernelChecker{MinKernelVersion: version}
}

// Name returns the checker name.
// Implements health.Checker.
func (r *kernelChecker) Name() string {
	return kernelCheckerID
}

// Check verifies kernel version.
// Implements health.Checker.
func (r *kernelChecker) Check(ctx context.Context, reporter health.Reporter) {
	if err := r.check(ctx, reporter); err != nil {
		log.WithError(err).Debug("Failed to verify kernel version.")
		return
	}
	if reporter.NumProbes() == 0 {
		reporter.Add(r.successProbe())
	}
}

func (r *kernelChecker) check(_ context.Context, reporter health.Reporter) error {
	release, err := realKernelVersionReader()
	if err != nil {
		return trace.Wrap(err, "failed to read kernel version")
	}

	kernelVersion, err := parseKernelVersion(release)
	if err != nil {
		reporter.Add(r.warningProbe(fmt.Sprintf("Failed to determine kernel version: %v.", release), err))
		return trace.Wrap(err, "failed to determine kernel version")
	}

	if !r.isSupportedVersion(*kernelVersion) {
		reporter.Add(r.warningProbe(fmt.Sprintf("Installed linux kernel is not supported: %v.", release), err))
	}
	return nil
}

func (r *kernelChecker) successProbe() *pb.Probe {
	return &pb.Probe{
		Checker: r.Name(),
		Detail:  "Installed linux kernel is supported.",
		Status:  pb.Probe_Running,
	}
}

func (r *kernelChecker) warningProbe(detail string, err error) *pb.Probe {
	return &pb.Probe{
		Checker:  r.Name(),
		Detail:   fmt.Sprintf("%s Gravity supports version %s and newer.", detail, r.MinKernelVersion.String()),
		Error:    trace.UserMessage(err),
		Status:   pb.Probe_Failed,
		Severity: pb.Probe_Warning,
	}
}

// isSupportedVersion returns true if the provided linux kernel version is
// supported by Gravity.
func (r *kernelChecker) isSupportedVersion(version KernelVersion) bool {
	return !KernelVersionLessThan(r.MinKernelVersion)(version)
}
