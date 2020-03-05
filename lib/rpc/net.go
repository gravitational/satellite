/*
Copyright 2020 Gravitational, Inc.

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

package rpc

import (
	"fmt"
	"net"

	"github.com/gravitational/trace"
)

// SplitHostPort splits the specified host:port into host/port parts.
// If the input address does not have a port, defaultPort as appended.
// Returns the extracted IP address and port
func SplitHostPort(hostPort string, defaultPort int) (addr *net.TCPAddr, err error) {
	_, _, err = net.SplitHostPort(hostPort)
	if ae, ok := err.(*net.AddrError); ok && ae.Err == "missing port in address" {
		hostPort = fmt.Sprintf("%s:%d", hostPort, defaultPort)
		_, _, err = net.SplitHostPort(hostPort)
	}
	if err != nil {
		return nil, trace.Wrap(err)
	}
	addr, err = net.ResolveTCPAddr("tcp", hostPort)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return addr, nil
}
