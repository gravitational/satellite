package health

import (
	"testing"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"

	check "gopkg.in/check.v1"
)

func TestHealth(t *testing.T) { check.TestingT(t) }

type HealthSuite struct{}

var _ = check.Suite(&HealthSuite{})

func (_ *HealthSuite) TestSetsNodeStatusFromProbes(c *check.C) {
	var r Probes
	for _, probe := range probes() {
		r.Add(probe)
	}
	c.Assert(r.Status(), check.Equals, pb.NodeStatus_Degraded)
}

func probes() []*pb.Probe {
	probes := []*pb.Probe{
		&pb.Probe{
			Checker: "foo",
			Status:  pb.Probe_Failed,
			Error:   "not found",
		},
		&pb.Probe{
			Checker: "bar",
			Status:  pb.Probe_Running,
		},
	}
	return probes
}
