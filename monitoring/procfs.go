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
	"bufio"
	"fmt"
	"net"
	"os"

	"github.com/gravitational/trace"
)

// ProcNet* are standard paths to Linux procfs information on sockets
const (
	ProcNetTCP  = "/proc/net/tcp"
	ProcNetTCP6 = "/proc/net/tcp6"
	ProcNetUDP  = "/proc/net/udp"
	ProcNetUDP6 = "/proc/net/udp6"
	ProcNetUnix = "/proc/net/unix"
)

// SocketState stores Linux socket state
type SocketState uint8

// Possible Linux socket states
const (
	Established SocketState = 0x01
	SynSent                 = 0x02
	SynRecv                 = 0x03
	FinWait1                = 0x04
	FinWait2                = 0x05
	TimeWait                = 0x06
	Close                   = 0x07
	CloseWait               = 0x08
	LastAck                 = 0x09
	Listen                  = 0x0A
	Closing                 = 0x0B
)

// TCPSocket contains details on a TCP socket
type TCPSocket struct {
	LocalAddress  net.TCPAddr
	RemoteAddress net.TCPAddr
	State         SocketState
	TXQueue       uint
	RXQueue       uint
	TimerState    uint
	TimeToTimeout uint
	Retransmit    uint
	UID           uint
	Inode         uint
}

// UDPSocket contains details on a UDP socket
type UDPSocket struct {
	LocalAddress  net.UDPAddr
	RemoteAddress net.UDPAddr
	State         SocketState
	TXQueue       uint
	RXQueue       uint
	UID           uint
	Inode         uint
	RefCount      uint
	Drops         uint
}

// UnixSocket contains details on a Unix socket
type UnixSocket struct {
	RefCount uint
	Protocol uint
	Flags    uint
	Type     uint
	State    uint
	Inode    uint
	Path     string
}

type socketparser func(string) (interface{}, error)

func parseSocketFile(parse socketparser, fpath string) (interface{}, error) {
	var sockets []interface{}
	fp, err := os.Open(fpath)
	defer fp.Close()
	if err != nil {
		return nil, trace.ConvertSystemError(err)
	}
	lineScanner := bufio.NewScanner(fp)
	lineScanner.Scan() // Drop header line
	for lineScanner.Scan() {
		socket, err := parse(lineScanner.Text())
		if err != nil {
			return nil, trace.Wrap(err)
		}
		sockets = append(sockets, socket)
	}
	if err := lineScanner.Err(); err != nil {
		return nil, trace.Wrap(err)
	}
	return sockets, nil
}

// GetTCPSockets reads specified file formatted as /proc/net/tcp{,6} and returns slice of *TCPSocket
func GetTCPSockets(fpath string) ([]*TCPSocket, error) {
	sockets, err := parseSocketFile(newTCPSocketFromLine, fpath)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	if socketsAsserted, ok := sockets.([]*TCPSocket); !ok {
		return nil, trace.BadParameter("type assertion failed")
	} else {
		return socketsAsserted, nil
	}
}

// GetUDPSockets reads specified file formatted as /proc/net/udp{,6} and returns slice of *UDPSocket
func GetUDPSockets(fpath string) ([]*UDPSocket, error) {
	sockets, err := parseSocketFile(newUDPSocketFromLine, fpath)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	if socketsAsserted, ok := sockets.([]*UDPSocket); !ok {
		return nil, trace.BadParameter("type assertion failed")
	} else {
		return socketsAsserted, nil
	}
}

// GetUnixSockets reads specified file formatted as /proc/net/unix and returns slice of *UnixSocket
func GetUnixSockets(fpath string) ([]*UnixSocket, error) {
	sockets, err := parseSocketFile(newUnixSocketFromLine, fpath)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	if socketsAsserted, ok := sockets.([]*UnixSocket); !ok {
		return nil, trace.BadParameter("type assertion failed")
	} else {
		return socketsAsserted, nil
	}
}

// newTCPSocketFromLine parses line in /proc/net/tcp{,6} format and returns TCPSocket object and error
func newTCPSocketFromLine(line string) (interface{}, error) {
	// sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
	//  0: 00000000:0035 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 18616 1 ffff91e759d47080 100 0 0 10 0
	// reference: https://github.com/ecki/net-tools/blob/master/netstat.c#L1070
	var (
		sl         int
		localip    []byte
		remoteip   []byte
		tr         int
		tmwhen     int
		retransmit int
		timeout    int
		tails      string
	)
	tcpSocket := &TCPSocket{}
	_, err := fmt.Sscanf(line, "%d: %32X:%4X %32X:%4X %2X %8X:%8X %2X:%8X %8X %d %d %d %1s",
		&sl, &localip, &tcpSocket.LocalAddress.Port, &remoteip, &tcpSocket.RemoteAddress.Port,
		&tcpSocket.State, &tcpSocket.TXQueue, &tcpSocket.RXQueue, &tr, &tmwhen, &retransmit,
		&tcpSocket.UID, &timeout, &tcpSocket.Inode, &tails)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	tcpSocket.LocalAddress.IP = hexToIP(localip)
	tcpSocket.RemoteAddress.IP = hexToIP(remoteip)
	return tcpSocket, nil
}

// newUDPSocketFromLine parses line in /proc/net/udp{,6} format and returns UDPSocket object and error
func newUDPSocketFromLine(line string) (interface{}, error) {
	//    sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode ref pointer drops
	//  2511: 00000000:14E9 00000000:0000 07 00000000:00000000 00:00000000 00000000  1000        0 1662497 2 ffff91e6a9fcbc00 0
	var (
		sl         int
		localip    []byte
		remoteip   []byte
		tr         int
		tmwhen     int
		retransmit int
		timeout    int
		pointer    []byte
	)
	udpSocket := &UDPSocket{}
	_, err := fmt.Sscanf(line, "%d: %32X:%4X %32X:%4X %2X %8X:%8X %2X:%8X %8X %d %d %d %d %128X %d",
		&sl, &localip, &udpSocket.LocalAddress.Port, &remoteip, &udpSocket.RemoteAddress.Port,
		&udpSocket.State, &udpSocket.TXQueue, &udpSocket.RXQueue, &tr, &tmwhen, &retransmit,
		&udpSocket.UID, &timeout, &udpSocket.Inode, &udpSocket.RefCount, &pointer,
		&udpSocket.Drops)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	udpSocket.LocalAddress.IP = hexToIP(localip)
	udpSocket.RemoteAddress.IP = hexToIP(remoteip)
	return udpSocket, nil
}

// newUnixSocketFromLine parses line in /proc/net/unix format and returns UnixSocket object and error
func newUnixSocketFromLine(line string) (interface{}, error) {
	// Num               RefCount Protocol Flags    Type St Inode Path
	// ffff91e759dfb800: 00000002 00000000 00010000 0001 01 16163 /tmp/sddm-auth3949710e-7c3f-4aa2-b5fc-25cc34a7f31e
	var (
		pointer []byte
	)
	unixSocket := &UnixSocket{}
	n, err := fmt.Sscanf(line, "%128X: %8X %8X %8X %4X %2X %d %32000s",
		&pointer, &unixSocket.RefCount, &unixSocket.Protocol, &unixSocket.Flags,
		&unixSocket.Type, &unixSocket.State, &unixSocket.Inode, &unixSocket.Path)
	if err != nil && n < 7 {
		return nil, trace.Wrap(err)
	}
	return unixSocket, nil
}

// hexToIP converts byte slice to net.IP
func hexToIP(in []byte) net.IP {
	ip := make([]byte, len(in))
	for i, v := range in {
		ip[len(ip)-i-1] = v
	}
	return ip
}
