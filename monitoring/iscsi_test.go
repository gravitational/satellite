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
	"fmt"

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/lib/test"

	"github.com/coreos/go-systemd/v22/dbus"
	. "gopkg.in/check.v1"
)

const (
	inactiveAndMaskedService = "Service unit ActiveState is inactive and LoadState is masked."
	inactiveAndMaskedSocket  = "Socket activate unit ActiveState is inactive and LoadState is masked."
	activeAndLoadedService   = "Service unit ActiveState is active and LoadState is loaded."
	activeAndLoadedSocket    = "Socket activated unit ActiveState is active and LoadState is loaded."
	inactiveAndLoadedService = "Service unit ActiveState is inactive and LoadState is loaded."
	inactiveAndLoadedSocket  = "Socket activated unit ActiveState is inactive and LoadState is loaded."

	failedProbeMessage = "Found conflicting systemd service: %v. " +
		"Please stop and mask this service and try again."
)

var (
	fmtISCSICheckFailedMsg = func(unitName string) string {
		return fmt.Sprintf(failedProbeMessage, unitName)
	}
)

type ISCSISuite struct{}

var _ = Suite(&ISCSISuite{})

// TestISCSI verifies that the checker correctly identifies
// if there are running or enabled iscsi related systemd services.
func (s *ISCSISuite) TestISCSI(c *C) {
	var testCases = []struct {
		comment    CommentInterface
		unitStatus []dbus.UnitStatus
		probe      *pb.Probe
	}{
		{
			comment:    Commentf(activeAndLoadedService),
			unitStatus: []dbus.UnitStatus{{Name: ISCSIDService, ActiveState: activeStateActive, LoadState: loadStateLoaded}},
			probe: &pb.Probe{
				Checker: iscsiCheckerID,
				Detail:  fmt.Sprintf(failedProbeMessage, ISCSIDService),
				Status:  pb.Probe_Failed,
			},
		},
		{
			comment:    Commentf(inactiveAndLoadedService),
			unitStatus: []dbus.UnitStatus{{Name: ISCSIDService, ActiveState: activeStateInactive, LoadState: loadStateLoaded}},
			probe: &pb.Probe{
				Checker: iscsiCheckerID,
				Detail:  fmt.Sprintf(failedProbeMessage, ISCSIDService),
				Status:  pb.Probe_Failed,
			},
		},
		{
			comment:    Commentf(activeAndLoadedSocket),
			unitStatus: []dbus.UnitStatus{{Name: ISCSIDSocket, ActiveState: activeStateActive, LoadState: loadStateLoaded}},
			probe: &pb.Probe{
				Checker: iscsiCheckerID,
				Detail:  fmt.Sprintf(failedProbeMessage, ISCSIDSocket),
				Status:  pb.Probe_Failed,
			},
		},
		{
			comment:    Commentf(inactiveAndLoadedSocket),
			unitStatus: []dbus.UnitStatus{{Name: ISCSIDSocket, ActiveState: activeStateInactive, LoadState: loadStateLoaded}},
			probe: &pb.Probe{
				Checker: iscsiCheckerID,
				Detail:  fmt.Sprintf(failedProbeMessage, ISCSIDSocket),
				Status:  pb.Probe_Failed,
			},
		},
		{
			comment:    Commentf(inactiveAndMaskedService),
			unitStatus: []dbus.UnitStatus{{Name: ISCSIDService, ActiveState: activeStateInactive, LoadState: loadStateMasked}},
			probe: &pb.Probe{
				Checker: iscsiCheckerID,
				Detail:  "",
				Status:  pb.Probe_Running,
			},
		},
		{
			comment:    Commentf(inactiveAndMaskedSocket),
			unitStatus: []dbus.UnitStatus{{Name: ISCSIDSocket, ActiveState: activeStateInactive, LoadState: loadStateMasked}},
			probe: &pb.Probe{
				Checker: iscsiCheckerID,
				Detail:  "",
				Status:  pb.Probe_Running,
			},
		},
	}

	for _, testCase := range testCases {
		checker := iscsiChecker{FailedProbeMsgFmt: fmtISCSICheckFailedMsg}
		var reporter health.Probes
		checker.CheckISCSIUnits(testCase.unitStatus, &reporter)
		c.Assert(reporter.GetProbes(), test.DeepCompare, []*pb.Probe{testCase.probe}, testCase.comment)
	}
}
