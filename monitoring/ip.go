package monitoring

import (
	"context"
	"fmt"

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"

	"github.com/gravitational/trace"
)

const (
	// IPForwardCheck is a name of the checker
	IPForwardCheck = "ip-forward"
	// Kernel network parameter for enabling ip forwarding
	IPForwardParam = "net.ipv4.ip_forward"
	// IPForwardEnabled stands for enabled forwarding
	IPForwardEnabled = "1"
)

// NewIPForwardChecker returns new IP forward checker
func NewIPForwardChecker() *IPForwardChecker {
	return &IPForwardChecker{}
}

// IPForwardChecker checks ip forwarding on the hosts
type IPForwardChecker struct {
}

// Name returns unique and user friendly name of the check
func (s *IPForwardChecker) Name() string {
	return IPForwardCheck
}

// Check runs a health check and records any errors into the specified reporter.
func (s *IPForwardChecker) Check(ctx context.Context, reporter health.Reporter) {
	value, err := Sysctl(IPForwardParam)
	if err != nil {
		reporter.Add(NewProbeFromErr(
			IPForwardCheck,
			trace.Wrap(
				trace.ConvertSystemError(err),
				fmt.Sprintf("failed to execute sysctl %v", IPForwardParam))))
		return
	}
	if value != IPForwardEnabled {
		reporter.Add(NewProbeFromErr(
			IPForwardCheck,
			trace.BadParameter(
				"ip forwarding is disabled on this host, kubernetes networking will not work, check %v sysctl parameter",
				IPForwardParam)))
		return
	}
	reporter.Add(&pb.Probe{
		Checker: IPForwardCheck,
		Status:  pb.Probe_Running,
	})
}
