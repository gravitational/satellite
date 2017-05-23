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

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/trace"
)

func NewSocketsChecker(tcpPorts []int, udpPorts []int, unixSocks []string) health.Checker {
	c := &SocketsChecker{
		TCPPorts:    make([]int, len(tcpPorts)),
		UDPPorts:    make([]int, len(udpPorts)),
		UNIXSockets: make([]string, len(unixSocks)),
	}
	copy(c.TCPPorts, tcpPorts)
	copy(c.UDPPorts, udpPorts)
	copy(c.UNIXSockets, unixSocks)
	return c
}

type SocketsChecker struct {
	TCPPorts    []int
	UDPPorts    []int
	UNIXSockets []string
}

func (c *SocketsChecker) Name() string {
	return "sockets"
}

func (c *SocketsChecker) Check(ctx context.Context, reporter health.Reporter) {
	if len(c.TCPPorts) > 0 {
		allSocks, err := GetTCPSockets()
		if err != nil {
			reporter.Add(NewProbeFromErr(c.Name(), trace.Errorf("failed to get TCP sockets %v", err)))
			return
		}
		openPorts := make(map[int]bool)
		for _, sock := range allSocks {
			if sock.State == Listen {
				openPorts[sock.LocalAddress.Port] = true
			}
		}
		for _, port := range c.TCPPorts {
			if _, ok := openPorts[port]; !ok {
				reporter.Add(NewProbeFromErr(c.Name(), trace.Errorf("not listening tcp/%d", port)))
				return
			}
		}
	}

	if len(c.UDPPorts) > 0 {
		allSocks, err := GetUDPSockets()
		if err != nil {
			reporter.Add(NewProbeFromErr(c.Name(), trace.Errorf("failed to get UDP sockets %v", err)))
			return
		}
		openPorts := make(map[int]bool)
		for _, sock := range allSocks {
			if sock.State == Listen {
				openPorts[sock.LocalAddress.Port] = true
			}
		}
		for _, port := range c.UDPPorts {
			if _, ok := openPorts[port]; !ok {
				reporter.Add(NewProbeFromErr(c.Name(), trace.Errorf("not listening udp/%d", port)))
				return
			}
		}
	}

	if len(c.UNIXSockets) > 0 {
		allSocks, err := GetUnixSockets()
		if err != nil {
			reporter.Add(NewProbeFromErr(c.Name(), trace.Errorf("failed to get UNIX sockets %v", err)))
			return
		}
		openSockets := make(map[string]bool)
		for _, sock := range allSocks {
			if len(sock.Path) > 0 {
				openSockets[sock.Path] = true
			}
		}
		for _, sock := range c.UNIXSockets {
			if _, ok := openSockets[sock]; !ok {
				reporter.Add(NewProbeFromErr(c.Name(), trace.Errorf("not listening unix/%d", sock)))
				return
			}
		}
	}

	reporter.Add(&pb.Probe{Checker: c.Name(), Status: pb.Probe_Running})
}
