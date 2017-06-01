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

// NewSocketsChecker creates and returns new SocketChecker
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

// SocketsChecker is a health.Checker which is able to check if specified TCP/UDP/Unix sockets are open on a host
type SocketsChecker struct {
	TCPPorts    []int
	UDPPorts    []int
	UNIXSockets []string
}

// Name returns name of this checker
func (c *SocketsChecker) Name() string {
	return "sockets"
}

// checkTCPPorts checks if specified slice of TCP ports are opened and returns slice of errors found
func checkTCPPorts(ports []int) []error {
	if len(ports) == 0 {
		return nil
	}
	allSocksIPv4, err := GetTCPSockets(ProcNetTCP)
	if err != nil {
		return []error{trace.Errorf("failed to get TCP sockets %v", err)}
	}
	allSocksIPv6, err := GetTCPSockets(ProcNetTCP6)
	if err != nil {
		return []error{trace.Errorf("failed to get TCP6 sockets %v", err)}
	}
	allSocks := append(allSocksIPv4, allSocksIPv6...)
	listeningPorts := make(map[int]bool)
	for _, sock := range allSocks {
		if sock.State == Listen {
			listeningPorts[sock.LocalAddress.Port] = true
		}
	}
	var errors []error
	for _, port := range ports {
		if _, listening := listeningPorts[port]; !listening {
			errors = append(errors, trace.NotFound("not listening tcp/%d", port))
		}
	}
	return errors
}

// checkUDPPorts checks if specified slice of UDP ports are opened and returns slice of errors found
func checkUDPPorts(ports []int) []error {
	if len(ports) == 0 {
		return nil
	}
	allSocksIPv4, err := GetUDPSockets(ProcNetUDP)
	if err != nil {
		return []error{trace.Errorf("failed to get UDP sockets %v", err)}
	}
	allSocksIPv6, err := GetUDPSockets(ProcNetUDP6)
	if err != nil {
		return []error{trace.Errorf("failed to get UDP6 sockets %v", err)}
	}
	allSocks := append(allSocksIPv4, allSocksIPv6...)
	listeningPorts := make(map[int]bool)
	for _, sock := range allSocks {
		listeningPorts[sock.LocalAddress.Port] = true
	}
	var errors []error
	for _, port := range ports {
		if _, listening := listeningPorts[port]; !listening {
			errors = append(errors, trace.NotFound("not listening udp/%d", port))
		}
	}
	return errors
}

// checkUnixPorts checks if specified slice of Unix ports are opened and returns slice of errors found
func checkUnixSockets(socks []string) []error {
	if len(socks) == 0 {
		return nil
	}
	allSocks, err := GetUnixSockets(ProcNetUnix)
	if err != nil {
		return []error{trace.Errorf("failed to get UNIX sockets %v", err)}
	}
	openSockets := make(map[string]bool)
	for _, sock := range allSocks {
		if len(sock.Path) > 0 {
			openSockets[sock.Path] = true
		}
	}
	var errors []error
	for _, sock := range socks {
		if _, listening := openSockets[sock]; !listening {
			errors = append(errors, trace.Errorf("not listening unix/%s", sock))
		}
	}
	return errors
}

// Check if all specified TCP/UDP/Unix sockets present and opened on host
func (c *SocketsChecker) Check(ctx context.Context, reporter health.Reporter) {
	errors := append([]error{}, checkTCPPorts(c.TCPPorts)...)
	errors = append(errors, checkUDPPorts(c.UDPPorts)...)
	errors = append(errors, checkUnixSockets(c.UNIXSockets)...)

	if len(errors) == 0 {
		reporter.Add(&pb.Probe{Checker: c.Name(), Status: pb.Probe_Running})
		return
	}

	for _, err := range errors {
		reporter.Add(NewProbeFromErr(c.Name(), err))
	}
}
