package monitoring

import (
	"context"
	"fmt"

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/utils"

	"github.com/gravitational/trace"

	"github.com/mitchellh/go-ps"
)

// ProcessChecker is checking aginst known list of conflicting processes
type ProcessChecker struct {
	ProcessNames []string
}

const ProcessCheckerID = "process-checker"

func DefaultProcessChecker() *ProcessChecker {
	return &ProcessChecker{[]string{
		"dockerd",
		"dnsmasq",
		"kube-apiserver",
		"kube-scheduler",
		"kube-controller-manager",
		"kube-proxy",
		"kubelet",
		"planet",
		"teleport",
	}}
}

func (c *ProcessChecker) Name() string {
	return ProcessCheckerID
}

func (c *ProcessChecker) Check(ctx context.Context, r health.Reporter) {
	running, err := ps.Processes()
	if err != nil {
		r.Add(NewProbeFromErr(ProcessCheckerID, "failed to obtain running process list", trace.Wrap(err)))
		return
	}

	prohibited := utils.NewStringSet()
	for _, process := range running {
		if utils.StringInSlice(c.ProcessNames, process.Executable()) {
			prohibited.Add(process.Executable())
		}
	}

	if len(prohibited) == 0 {
		r.Add(&pb.Probe{
			Checker: ProcessCheckerID,
			Status:  pb.Probe_Running,
		})
		return
	}

	r.Add(&pb.Probe{
		Checker: ProcessCheckerID,
		Detail:  fmt.Sprintf("conflicting programs: %v", prohibited.Slice()),
		Status:  pb.Probe_Failed,
	})

}
