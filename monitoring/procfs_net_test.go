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
	"net"

	"github.com/gravitational/satellite/lib/test"

	"github.com/prometheus/procfs"
	. "gopkg.in/check.v1"
)

type S struct{}

var _ = Suite(&S{})

func (_ *S) TestParseSocketFile(c *C) {
	err := parseSocketFile("fixtures/tcp", func(string) error {
		return nil
	})
	c.Assert(err, IsNil)
}

func (_ *S) TestGetTCPSockets(c *C) {
	sockets, err := getTCPSocketsFromPath("fixtures/tcp")
	c.Assert(err, IsNil)
	c.Assert(sockets, Not(IsNil))

	sockets, err = getTCPSocketsFromPath("fixtures/tcp6")
	c.Assert(err, IsNil)
	c.Assert(sockets, Not(IsNil))
}

func (_ *S) TestGetUDPSockets(c *C) {
	sockets, err := getUDPSocketsFromPath("fixtures/udp")
	c.Assert(err, IsNil)
	c.Assert(sockets, Not(IsNil))

	sockets, err = getUDPSocketsFromPath("fixtures/udp6")
	c.Assert(err, IsNil)
	c.Assert(sockets, Not(IsNil))
}

func (_ *S) TestGetUnixSockets(c *C) {
	sockets, err := getUnixSocketsFromPath("fixtures/unix")
	c.Assert(err, IsNil)
	c.Assert(sockets, Not(IsNil))
}

func (_ *S) TestTCPSocketFromLine(c *C) {
	// setup / exercise
	sock, err := newTCPSocketFromLine("   13: 6408A8C0:D370 03AD7489:01BB 01 00000000:00000000 02:00000998 00000000  1000        0 4128613 2 ffff8cea5ed24080 56 4 30 10 -1")

	// verify
	c.Assert(err, IsNil)
	c.Assert(sock, test.DeepCompare, &tcpSocket{
		LocalAddress:  tcpAddr("192.168.8.100:54128", c),
		RemoteAddress: tcpAddr("137.116.173.3:443", c),
		State:         Established,
		UID:           1000,
		Inode:         4128613,
	})

	// setup / exercise
	sock, err = newTCPSocketFromLine("    1: 00000000000000000000000000000000:0016 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 4118579 1 ffff8ceb27309800 100 0 0 10 0")

	// verify
	c.Assert(err, IsNil)
	c.Assert(sock, test.DeepCompare, &tcpSocket{
		LocalAddress:  tcpAddr("[::]:22", c),
		RemoteAddress: tcpAddr("[::]:0", c),
		State:         Listen,
		Inode:         4118579,
	})
}

func (_ *S) TestUDPSocketFromLine(c *C) {
	// setup / exercise
	sock, err := newUDPSocketFromLine(" 6620: 6408A8C0:A44D BDDCC2AD:01BB 01 00000000:00000000 00:00000000 00000000  1000        0 4216043 2 ffff8cec9b9c2440 0")

	// verify
	c.Assert(err, IsNil)
	c.Assert(sock, test.DeepCompare, &udpSocket{
		LocalAddress:  udpAddr("192.168.8.100:42061", c),
		RemoteAddress: udpAddr("173.194.220.189:443", c),
		State:         1,
		UID:           1000,
		Inode:         4216043,
		RefCount:      2,
	})

	// setup / exercise
	sock, err = newUDPSocketFromLine(" 2680: 00000000000000000000000000000000:14E9 00000000000000000000000000000000:0000 07 00000000:00000000 00:00000000 00000000  1000        0 4361657 2 ffff8ceb2722a580 0")

	// verify
	c.Assert(err, IsNil)
	c.Assert(sock, test.DeepCompare, &udpSocket{
		LocalAddress:  udpAddr("[::]:5353", c),
		RemoteAddress: udpAddr("[::]:0", c),
		State:         7,
		UID:           1000,
		Inode:         4361657,
		RefCount:      2,
	})
}

func (_ *S) TestUnixSocketFromLine(c *C) {
	// setup / exercise
	sock, err := newUnixSocketFromLine("ffff8cec77d65000: 00000003 00000000 00000000 0001 03 4116864")

	// verify
	c.Assert(err, IsNil)
	c.Assert(sock, test.DeepCompare, &unixSocket{
		Type:     1,
		State:    3,
		Inode:    4116864,
		RefCount: 3,
	})

	// setup / exercise
	sock, err = newUnixSocketFromLine("ffff8cec9ca9c400: 00000003 00000000 00000000 0001 03 11188 /run/systemd/journal/stdout")
	c.Assert(err, IsNil)
	c.Assert(sock, test.DeepCompare, &unixSocket{
		Path:     "/run/systemd/journal/stdout",
		Type:     1,
		State:    3,
		Inode:    11188,
		RefCount: 3,
	})
}

func (_ *S) TestDoesNotFailForNonExistingStacks(c *C) {
	fetchTCP := func() ([]socket, error) {
		return getTCPSocketsFromPath("non-existing/tcp")
	}
	fetchUDP := func() ([]socket, error) {
		return getUDPSocketsFromPath("non-existing/udp")
	}
	fetchProcs := func() (procfs.Procs, error) {
		return nil, nil
	}

	procs, err := getPorts(fetchProcs, fetchTCP, fetchUDP)
	c.Assert(err, IsNil)

	c.Assert(procs, DeepEquals, []process(nil))
}

func tcpAddr(addrS string, c *C) net.TCPAddr {
	addr, err := net.ResolveTCPAddr("tcp", addrS)
	c.Assert(err, IsNil)
	ipv4 := addr.IP.To4()
	if ipv4 != nil {
		addr.IP = ipv4
	}
	return *addr
}

func udpAddr(addrS string, c *C) net.UDPAddr {
	addr, err := net.ResolveUDPAddr("udp", addrS)
	c.Assert(err, IsNil)
	ipv4 := addr.IP.To4()
	if ipv4 != nil {
		addr.IP = ipv4
	}
	return *addr
}
