package monitoring

import (
	"context"
	"fmt"

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"

	netstat "github.com/drael/GOnetstat"
)

const (
	ProtoTCP      = "tcp"
	ProtoUDP      = "udp"
	PortCheckerID = "port-checker"
)

// PortRange defines ports and protocol family to check
type PortRange struct {
	Protocol    string
	From, To    int64
	Description string
}

func DefaultPortChecker() *PortChecker {
	return &PortChecker{[]PortRange{
		PortRange{ProtoTCP, 53, 53, "internal cluster DNS"},
		PortRange{ProtoUDP, 53, 53, "internal cluster DNS"},
		PortRange{ProtoUDP, 8472, 8472, "overlay network"},
		PortRange{ProtoTCP, 7496, 7496, "serf (Health check agents) peer to peer"},
		PortRange{ProtoTCP, 7373, 7373, "serf (Health check agents) peer to peer"},
		PortRange{ProtoTCP, 2379, 2380, "etcd"},
		PortRange{ProtoTCP, 4001, 4001, "etcd"},
		PortRange{ProtoTCP, 7001, 7001, "etcd"},
		PortRange{ProtoTCP, 6443, 6443, "k8s API server"},
		PortRange{ProtoTCP, 30000, 32767, "k8s internal services range"},
		PortRange{ProtoTCP, 10248, 10255, "k8s internal services range"},
		PortRange{ProtoTCP, 5000, 5000, "Docker registry"},
		PortRange{ProtoTCP, 3022, 3025, "Teleport internal ssh control panel"},
		PortRange{ProtoTCP, 3080, 3080, "Teleport Web UI"},
		PortRange{ProtoTCP, 3008, 3012, "Internal Telekube services"},
		PortRange{ProtoTCP, 32009, 32009, "Telekube OpsCenter control panel"},
		PortRange{ProtoTCP, 7575, 7575, "Telekube RPC agent"},
	}}
}

func InstallTimePortChecker() *PortChecker {
	return &PortChecker{[]PortRange{
		PortRange{ProtoTCP, 4242, 4242, "bandwidth checker"},
		PortRange{ProtoTCP, 61008, 61010, "installer agent ports"},
		PortRange{ProtoTCP, 61022, 61024, "installer agent ports"},
		PortRange{ProtoTCP, 61009, 61009, "install wizard"},
	}}
}

// PortChecker will validate that all required ports are in fact unoccupied
type PortChecker struct {
	Ranges []PortRange
}

func (c *PortChecker) Name() string {
	return PortCheckerID
}

func (c *PortChecker) Check(ctx context.Context, reporter health.Reporter) {
	used := map[string][]netstat.Process{
		ProtoTCP: netstat.Tcp(),
		ProtoUDP: netstat.Udp()}
	conflicts := false

	for proto, processes := range used {
		for _, proc := range processes {
			for _, r := range c.Ranges {
				if r.Protocol != proto {
					continue
				}
				if proc.Port >= r.From && proc.Port <= r.To {
					conflicts = true
					reporter.Add(&pb.Probe{
						Checker: PortCheckerID,
						Detail: fmt.Sprintf("a conflicting program %q(pid=%s) is occupying port %s/%d(%s)",
							proc.Name, proc.Pid, proto, proc.Port, proc.State),
						Status: pb.Probe_Failed})
				}
			}
		}
	}

	if conflicts {
		return
	}
	reporter.Add(&pb.Probe{
		Checker: PortCheckerID,
		Status:  pb.Probe_Running})
}
