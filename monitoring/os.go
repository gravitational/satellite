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

	"github.com/go-ini/ini"
)

// OSRelease is used to represent a certain OS release based on https://www.freedesktop.org/software/systemd/man/os-release.html fields
type OSRelease struct {
	// ID is either ubuntu, redhat or centos
	ID string
	// VersionID is major release version i.e. 16.04
	VersionID string
}

// OSChecker validates host OS based on https://www.freedesktop.org/software/systemd/man/os-release.html
type OSChecker struct {
	Releases []OSRelease
}

const osCheckerID = "os-checker"

// DefaultOSChecker returns standard distributions supported by Telekube
func DefaultOSChecker() *OSChecker {
	return &OSChecker{
		[]OSRelease{
			OSRelease{"rhel", "7.2"},
			OSRelease{"rhel", "7.3"},
			OSRelease{"rhel", "7.4"},
			OSRelease{"centos", "7.2"},
			OSRelease{"centos", "7.3"},
			OSRelease{"ubuntu", "16.04"},
			OSRelease{"ubuntu-core", "16"},
		},
	}
}

// Name returns name of the checker
func (c *OSChecker) Name() string {
	return osCheckerID
}

// Check checks current OS and release is within supported list
func (c *OSChecker) Check(ctx context.Context, reporter health.Reporter) {
	id, versionID, err := parseOSRelease()
	if err != nil {
		reporter.Add(NewProbeFromErr(osCheckerID, "failed to parse /etc/os-release", trace.Wrap(err)))
		return
	}

	for _, rel := range c.Releases {
		if rel.ID == id && rel.VersionID == versionID {
			reporter.Add(&pb.Probe{
				Checker: osCheckerID,
				Status:  pb.Probe_Running})
			return
		}
	}

	reporter.Add(&pb.Probe{
		Checker: osCheckerID,
		Detail:  fmt.Sprintf("%s %s is not supported", id, versionID),
		Status:  pb.Probe_Failed})
}

const (
	osRelFile      = "/etc/os-release"
	osRelID        = "ID"
	osRelVersionID = "VERSION_ID"
)

func parseOSRelease() (id, versionID string, err error) {
	osrelFile, err := ini.Load(osRelFile)
	if err != nil {
		return "", "", trace.Wrap(err)
	}

	osrel, err := osrelFile.GetSection("")
	if err != nil {
		return "", "", trace.Wrap(err)
	}

	return osrel.Key(osRelID).String(), osrel.Key(osRelVersionID).String(), nil

}
