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
)

const (
	protoTCP      = "tcp"
	protoUDP      = "udp"
	portCheckerID = "port-checker"
)

// PortRange defines ports and protocol family to check
type PortRange struct {
	Protocol    string
	From, To    uint64
	Description string
}

func DefaultPortChecker() *PortChecker {
	return &PortChecker{[]PortRange{
		PortRange{protoTCP, 53, 53, "internal cluster DNS"},
		PortRange{protoUDP, 53, 53, "internal cluster DNS"},
		PortRange{protoUDP, 8472, 8472, "overlay network"},
		PortRange{protoTCP, 7496, 7496, "serf (health check agents) peer to peer"},
		PortRange{protoTCP, 7373, 7373, "serf (health check agents) peer to peer"},
		PortRange{protoTCP, 2379, 2380, "etcd"},
		PortRange{protoTCP, 4001, 4001, "etcd"},
		PortRange{protoTCP, 7001, 7001, "etcd"},
		PortRange{protoTCP, 6443, 6443, "kubernetes API server"},
		PortRange{protoTCP, 30000, 32767, "kubernetes internal services range"},
		PortRange{protoTCP, 10248, 10255, "kubernetes internal services range"},
		PortRange{protoTCP, 5000, 5000, "docker registry"},
		PortRange{protoTCP, 3022, 3025, "teleport internal ssh control panel"},
		PortRange{protoTCP, 3080, 3080, "teleport Web UI"},
		PortRange{protoTCP, 3008, 3012, "internal Telekube services"},
		PortRange{protoTCP, 32009, 32009, "telekube OpsCenter control panel"},
		PortRange{protoTCP, 7575, 7575, "telekube RPC agent"},
	}}
}

// PreInstallPortChecker validates no actual checkers
func PreInstallPortChecker() *PortChecker {
	return &PortChecker{[]PortRange{
		PortRange{protoTCP, 4242, 4242, "bandwidth checker"},
		PortRange{protoTCP, 61008, 61010, "installer agent ports"},
		PortRange{protoTCP, 61022, 61024, "installer agent ports"},
		PortRange{protoTCP, 61009, 61009, "install wizard"},
	}}
}

// PortChecker will validate that all required ports are in fact unoccupied
type PortChecker struct {
	Ranges []PortRange
}

// Name returns this checker name
func (c *PortChecker) Name() string {
	return portCheckerID
}

func (c *PortChecker) checkProcess(proto string, proc process, reporter health.Reporter) bool {
	conflicts := false
	for _, r := range c.Ranges {
		if r.Protocol != proto {
			continue
		}
		if uint64(proc.localAddr().port) >= r.From && uint64(proc.localAddr().port) <= r.To {
			conflicts = true
			reporter.Add(&pb.Probe{
				Checker: portCheckerID,
				Detail: fmt.Sprintf("a conflicting program %q(pid=%v) is occupying port %v/%d(%v)",
					proc.name, proc.pid, proto, proc.localAddr().port, proc.state()),
				Status: pb.Probe_Failed})
		}
	}
	return conflicts
}

// Check will scan current open ports and report every conflict detected
func (c *PortChecker) Check(ctx context.Context, reporter health.Reporter) {
	collector, err := newPortCollector()
	if err != nil {
		reporter.Add(NewProbeFromErr(hostCheckerID, "querying connections", trace.Wrap(err)))
		return
	}
	procsTCP, err := collector.tcp()
	if err != nil {
		reporter.Add(NewProbeFromErr(hostCheckerID, "querying tcp connections", trace.Wrap(err)))
		return
	}

	procsUDP, err := collector.udp()
	if err != nil {
		reporter.Add(NewProbeFromErr(hostCheckerID, "querying udp connections", trace.Wrap(err)))
		return
	}

	used := map[string][]process{
		protoTCP: procsTCP,
		protoUDP: procsUDP}
	conflicts := false

	for proto, processes := range used {
		for _, proc := range processes {
			conflicts = conflicts || c.checkProcess(proto, proc, reporter)
		}
	}

	if conflicts {
		return
	}
	reporter.Add(&pb.Probe{
		Checker: portCheckerID,
		Status:  pb.Probe_Running})
}
