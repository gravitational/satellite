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
	"io"
	"io/ioutil"
	"strings"

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/lib/test"
	"github.com/gravitational/trace"

	. "gopkg.in/check.v1"
)

func (*MonitoringSuite) TestValidatesOS(c *C) {
	// setup
	var testCases = []struct {
		releases   []OSRelease
		getRelease osReleaseGetter
		probes     health.Probes
		comment    string
	}{
		{
			releases:   staticOSReleases("ubuntu", "16.04.3", "centos", "7.2"),
			getRelease: testGetOSRelease(OSRelease{ID: "CentOS", VersionID: "7.2"}),
			probes:     health.Probes{&pb.Probe{Checker: osCheckerID, Status: pb.Probe_Running}},
			comment:    "requirements match",
		},
		{
			// Release can relax the version constraint by using a prefix to capture
			// a subset of distribution releases: i.e. "16" to capture multiple 16.x versions
			releases:   staticOSReleases("ubuntu", "16", "centos", "7.2"),
			getRelease: testGetOSRelease(OSRelease{ID: "ubuntu", VersionID: "16.04.3"}),
			probes:     health.Probes{&pb.Probe{Checker: osCheckerID, Status: pb.Probe_Running}},
			comment:    "requirements match on prefix",
		},
		{
			releases:   staticOSReleases("ubuntu", "16.04.3", "centos", "7.2"),
			getRelease: testGetOSRelease(OSRelease{ID: "debian", VersionID: "9.3"}),
			probes: health.Probes{&pb.Probe{
				Checker: osCheckerID,
				Detail:  "debian 9.3 is not supported",
				Status:  pb.Probe_Failed,
			}},
			comment: "missing requirement",
		},
		{
			releases:   staticOSReleases(),
			getRelease: testGetOSRelease(OSRelease{ID: "debian", VersionID: "9.3"}),
			probes:     health.Probes{&pb.Probe{Checker: osCheckerID, Status: pb.Probe_Running}},
			comment:    "no error for empty requirements",
		},
		{
			releases:   staticOSReleases("fedora", "25"),
			getRelease: testFailingGetOSRelease(trace.NotFound("file or directory not found")),
			probes:     health.Probes{&pb.Probe{Checker: osCheckerID, Status: pb.Probe_Running}},
			comment:    "skip test if OS no distribution files are available",
		},
		{
			releases:   staticOSReleases("debian", "9"),
			getRelease: testFailingGetOSRelease(trace.AccessDenied("permission denied")),
			probes: health.Probes{&pb.Probe{
				Checker: osCheckerID,
				Detail:  "failed to validate OS distribution",
				Error:   "permission denied, failed to query OS version",
				Status:  pb.Probe_Failed,
			}},
			comment: "fail if error prevents from reading the file (other than not found)",
		},
	}

	// exercise / verify
	for _, testCase := range testCases {
		checker := &osReleaseChecker{
			Releases:   testCase.releases,
			getRelease: testCase.getRelease,
		}
		var reporter health.Probes
		checker.Check(context.TODO(), &reporter)
		c.Assert(reporter, test.DeepCompare, testCase.probes, Commentf(testCase.comment))
	}
}

func (*MonitoringSuite) TestReadsReleaseInfo(c *C) {
	// setup
	var testCases = []struct {
		files    releaseFiles
		expected OSRelease
		comment  string
	}{
		{
			files: releaseFiles{
				lsbRelease: testLsbRelease(OSRelease{ID: "centos", VersionID: "7.3.1711"}),
				release:    testReader(testOSRelease),
			},
			expected: OSRelease{ID: "centos", VersionID: "7.3.1711"},
			comment:  "lsb_release is preferred",
		},
		{
			files: releaseFiles{
				release: testReader(testOSRelease),
				version: testReader(testOSVersion),
			},
			expected: OSRelease{ID: "centos", VersionID: "7.4.1708", Like: []string{"rhel", "fedora"}},
			comment:  "version is updated",
		},
	}

	// exercise / verify
	for _, testCase := range testCases {
		info, err := getOSRelease(testCase.files)
		c.Assert(err, IsNil)
		c.Assert(info, test.DeepCompare, &testCase.expected, Commentf(testCase.comment))
	}
}

func (*MonitoringSuite) TestFailsOnNoReleaseFiles(c *C) {
	_, err := getOSReleaseFromFiles(
		[]string{"/etc/no-such-release"},
		[]string{"/etc/no-such-version"},
	)
	c.Assert(err, ErrorMatches, "no release version file found")
}

func testGetOSRelease(release OSRelease) osReleaseGetter {
	return func() (*OSRelease, error) {
		return &release, nil
	}
}

func testFailingGetOSRelease(err error) osReleaseGetter {
	return func() (*OSRelease, error) {
		return nil, err
	}
}

func staticOSReleases(releases ...string) (result []OSRelease) {
	if len(releases)%2 != 0 {
		return nil
	}
	for len(releases) > 1 {
		distro, release := releases[0], releases[1]
		result = append(result, OSRelease{ID: distro, VersionID: release})
		releases = releases[2:]
	}
	return result
}

func testLsbRelease(release OSRelease) func() (*OSRelease, error) {
	return func() (*OSRelease, error) {
		return &release, nil
	}
}

func testReader(data string) io.ReadCloser {
	return ioutil.NopCloser(strings.NewReader(data))
}

const testOSRelease = `
NAME="CentOS Linux"
VERSION="7 (Core)"
ID="centos"
ID_LIKE="rhel fedora"
VERSION_ID="7"
PRETTY_NAME="CentOS Linux 7 (Core)"
ANSI_COLOR="0;31"
CPE_NAME="cpe:/o:centos:centos:7"
HOME_URL="https://www.centos.org/"
BUG_REPORT_URL="https://bugs.centos.org/"

CENTOS_MANTISBT_PROJECT="CentOS-7"
CENTOS_MANTISBT_PROJECT_VERSION="7"
REDHAT_SUPPORT_PRODUCT="centos"
REDHAT_SUPPORT_PRODUCT_VERSION="7"
`

const testOSVersion = `CentOS Linux release 7.4.1708 (Core)`
