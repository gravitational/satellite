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

	"github.com/coreos/go-systemd/v22/dbus"
	"github.com/gravitational/trace"
)

const (
	iscsiCheckerID = "iscsi"

	// ISCSIDService is the name of the iscsid systemd service unit.
	ISCSIDService = "iscsid.service"
	// ISCSIDSocket is the name of the iscsid systemd socket unit.
	ISCSIDSocket = "iscsid.socket"
)

// NewISCSIChecker verifies that the iscsid service
// is not running on the host.
// The failedProbeMessage gets displayed to the user when
// the probe fails. The message takes a single parameter:
// systemd unit name.
func NewISCSIChecker(failedProbeMessage string) health.Checker {
	return &iscsiChecker{FailedProbeMessage: failedProbeMessage}
}

type iscsiChecker struct {
	FailedProbeMessage string
}

// Name returns the name of this checker
// Implements health.Checker
func (c iscsiChecker) Name() string {
	return iscsiCheckerID
}

// Check will check the systemd unit data coming from dbus to verify that
// there are no iscsid services present on the host.
func (c iscsiChecker) Check(ctx context.Context, reporter health.Reporter) {
	conn, err := dbus.New()
	if err != nil {
		reason := "failed to connect to dbus"
		reporter.Add(NewProbeFromErr(c.Name(), reason, trace.Wrap(err)))
		return
	}
	defer conn.Close()

	units, err := conn.ListUnits()
	if err != nil {
		reason := "failed to list systemd units via dbus"
		reporter.Add(NewProbeFromErr(c.Name(), reason, trace.Wrap(err)))
		return
	}

	c.CheckISCSIUnits(units, reporter)
}

// CheckISCSIUnits verifies that the systemd units are in the expected state.
func (c iscsiChecker) CheckISCSIUnits(units []dbus.UnitStatus, reporter health.Reporter) {
	for _, unit := range units {
		switch unit.Name {
		case ISCSIDService, ISCSIDSocket:
			if unit.ActiveState == activeStateActive || unit.LoadState != loadStateMasked {
				reporter.Add(&pb.Probe{
					Checker: iscsiCheckerID,
					Detail:  fmt.Sprintf(c.FailedProbeMessage, unit.Name),
					Status:  pb.Probe_Failed,
				})
			}
		}
	}

	if reporter.NumProbes() == 0 {
		reporter.Add(NewSuccessProbe(c.Name()))
	}
}
