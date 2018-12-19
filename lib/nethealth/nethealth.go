/*
Copyright 2016 The Kubernetes Authors.

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

package nethealth

import (
	"fmt"
	"net"

	"github.com/gravitational/trace"
)

// Server is a simple server, that binds to a port, and will answer any pings that come from within the cluster
type Server struct {
	AllowedSubnets []net.IPNet
	BindPort       uint32

	conn net.PacketConn
}

func (s *Server) ListenAndServe() error {
	var err error
	s.conn, err = net.ListenPacket("udp", fmt.Sprint(":", s.BindPort))
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}

func (s *Server) Stop() error {
	return trace.Wrap(s.conn.Close())
}

func (s *Server) listen() {
	buf := make([]byte, 1024)
	for {
		n, remote, err := s.conn.ReadFrom(buf)
		if err != nil {
			// we need to return on error
		}
		// don't do any processing and silently discard packets with src addr not in allowed ranges
		if !s.allowed(peer) {
			continue
		}

	}
}

func (s *Server) processPing(buf []byte, n int, peer net.Addr) error {

	return nil
}

func (s *Server) allowed(peer net.Addr) bool {
	for _, subnet := range s.AllowedSubnets {
		if subnet.Contains(peer) {
			return true
		}
	}
	return false
}
