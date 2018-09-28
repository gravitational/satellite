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
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
)

func newErrorProber(checkerID string) prober {
	return prober{checkerID: checkerID}
}

func (r prober) newCleared() *pb.Probe {
	return &pb.Probe{
		Checker: r.checkerID,
		Status:  pb.Probe_Running,
	}
}

func (r prober) newRaised(detail string) *pb.Probe {
	return &pb.Probe{
		Checker:  r.checkerID,
		Detail:   detail,
		Status:   pb.Probe_Failed,
		Severity: pb.Probe_Critical,
	}
}

func (r prober) newRaisedProbe(probe probe) *pb.Probe {
	return &pb.Probe{
		Checker:     r.checkerID,
		Detail:      probe.detail,
		Error:       probe.error,
		Status:      pb.Probe_Failed,
		Severity:    pb.Probe_Critical,
		CheckerData: probe.data,
	}
}

type prober struct {
	checkerID string
}

type probe struct {
	detail, error string
	data          []byte
}
