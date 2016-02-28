package monitoring

import pb "github.com/gravitational/satellite/agent/proto/agentpb"

// NewProbeFromErr creates a new Probe given an error and a checker name
func NewProbeFromErr(name string, err error) *pb.Probe {
	return &pb.Probe{
		Checker: name,
		Error:   err.Error(),
		Status:  pb.Probe_Failed,
	}
}
