package monitoring

import (
	"context"
	"fmt"

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"

	"github.com/gravitational/trace"

	"github.com/go-ini/ini"
)

// OsRelease is used to represent a certain OS release based on https://www.freedesktop.org/software/systemd/man/os-release.html fields
type OsRelease struct {
	// Id is either ubuntu, redhat or centos
	Id string
	// VersionId is major release version i.e. 16.04
	VersionId string
}

// OsChecker validates host OS based on https://www.freedesktop.org/software/systemd/man/os-release.html
type OsChecker struct {
	Releases []OsRelease
}

const OsCheckerId = "os-checker"

// DefaultOsChecker returns standard distributions supported by Telekube
func DefaultOsChecker() *OsChecker {
	return &OsChecker{
		[]OsRelease{
			OsRelease{"redhat", "7.2"},
			OsRelease{"redhat", "7.3"},
			OsRelease{"redhat", "7.4"},
			OsRelease{"centos", "7.2"},
			OsRelease{"centos", "7.3"},
			OsRelease{"ubuntu", "16.04"},
			OsRelease{"ubuntu-core", "16"},
		},
	}
}

func (c *OsChecker) Name() string {
	return OsCheckerId
}

func (c *OsChecker) Check(ctx context.Context, reporter health.Reporter) {
	id, versionId, err := parseOsRelease()
	if err != nil {
		reporter.Add(NewProbeFromErr(OsCheckerId, "failed to parse /etc/os-release", trace.Wrap(err)))
		return
	}

	for _, rel := range c.Releases {
		if rel.Id == id && rel.VersionId == versionId {
			reporter.Add(&pb.Probe{
				Checker: OsCheckerId,
				Status:  pb.Probe_Running})
			return
		}
	}

	reporter.Add(&pb.Probe{
		Checker: OsCheckerId,
		Detail:  fmt.Sprintf("%s %s is not supported, please see https://gravitational.com/docs/pack/#distributions", id, versionId),
		Status:  pb.Probe_Failed})
}

func parseOsRelease() (id, versionId string, err error) {
	osrelFile, err := ini.Load("/etc/os-release")
	if err != nil {
		return "", "", trace.Wrap(err)
	}

	osrel, err := osrelFile.GetSection("")
	if err != nil {
		return "", "", trace.Wrap(err)
	}

	return osrel.Key("ID").String(), osrel.Key("VERSION_ID").String(), nil

}
