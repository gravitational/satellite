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
	"os/exec"

	"github.com/gravitational/satellite/agent/health"
	"github.com/gravitational/satellite/agent/proto/agentpb"

	"github.com/gravitational/trace"
	"github.com/sirupsen/logrus"
)

// NewTillerChecker returns a checker that validates tiller server accessibility.
func NewTillerChecker() health.Checker {
	return &tillerChecker{
		FieldLogger: logrus.WithField(trace.Component, "tiller"),
	}
}

type tillerChecker struct {
	logrus.FieldLogger
}

// Name returns the checker id.
func (c *tillerChecker) Name() string { return "tiller" }

// Check validates the tiller server accessibility.
func (c *tillerChecker) Check(ctx context.Context, reporter health.Reporter) {
	out, err := exec.Command("/usr/bin/helm", "version", "--server").CombinedOutput()
	if err != nil {
		c.WithError(err).Warning("Failed to check tiller server.")
		reporter.Add(&agentpb.Probe{
			Checker:  c.Name(),
			Detail:   fmt.Sprintf("failed to check tiller server: %v", string(out)),
			Error:    trace.UserMessage(err),
			Status:   agentpb.Probe_Failed,
			Severity: agentpb.Probe_Warning,
		})
	} else {
		reporter.Add(&agentpb.Probe{
			Checker: c.Name(),
			Detail:  string(out),
			Status:  agentpb.Probe_Running,
		})
	}
}
