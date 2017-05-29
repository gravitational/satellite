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

const (
	ProcNetTCP  = "/proc/net/tcp"
	ProcNetTCP6 = "/proc/net/tcp6"
	ProcNetUDP  = "/proc/net/udp"
	ProcNetUDP6 = "/proc/net/udp6"
	ProcNetUnix = "/proc/net/unix"
)

type SocketState uint8

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

type UDPSocket struct {
	LocalAddress  net.UDPAddr
	RemoteAddress net.UDPAddr
	State         SocketState
	TXQueue       uint
	RXQueue       uint
	UID           uint
	Inode         uint
	RefCount      uint
	Pointer       uintptr
	Drops         uint
}

type UnixSocket struct {
	RefCount uint
	Protocol uint
	Flags    uint
	Type     uint
	State    uint
	Inode    uint
	Path     string
}

func GetTCPSockets(fpath string) ([]*TCPSocket, error) {
	var sockets []*TCPSocket
	fp, err := os.Open(fpath)
	defer fp.Close()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	lineScanner := bufio.NewScanner(fp)
	lineScanner.Scan() // Drop header line
	for lineScanner.Scan() {
		socket, err := NewTCPSocketFromLine(lineScanner.Text())
		if err != nil {
			return nil, err
		}
		sockets = append(sockets, socket)
	}
	if err := lineScanner.Err(); err != nil {
		return nil, trace.Wrap(err)
	}
	return sockets, nil
}

func GetUDPSockets(fpath string) ([]*UDPSocket, error) {
	var sockets []*UDPSocket
	fp, err := os.Open(fpath)
	defer fp.Close()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	lineScanner := bufio.NewScanner(fp)
	lineScanner.Scan() // Drop header line
	for lineScanner.Scan() {
		socket, err := NewUDPSocketFromLine(lineScanner.Text())
		if err != nil {
			return nil, err
		}
		sockets = append(sockets, socket)
	}
	if err := lineScanner.Err(); err != nil {
		return nil, trace.Wrap(err)
	}
	return sockets, nil
}

func GetUnixSockets() ([]*UnixSocket, error) {
	var sockets []*UnixSocket
	fp, err := os.Open(ProcNetUnix)
	defer fp.Close()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	lineScanner := bufio.NewScanner(fp)
	lineScanner.Scan() // Drop header line
	for lineScanner.Scan() {
		socket, err := NewUnixSocketFromLine(lineScanner.Text())
		if err != nil {
			return nil, err
		}
		sockets = append(sockets, socket)
	}
	if err := lineScanner.Err(); err != nil {
		return nil, trace.Wrap(err)
	}
	return sockets, nil

}

func NewTCPSocketFromLine(line string) (*TCPSocket, error) {
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
	_, err := fmt.Sscanf(line, "%d: %X:%X %X:%X %X %X:%X %X:%X %X %d %d %d %s",
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

func NewUDPSocketFromLine(line string) (*UDPSocket, error) {
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
	)
	udpSocket := &UDPSocket{}
	_, err := fmt.Sscanf(line, "%d: %X:%X %X:%X %X %X:%X %X:%X %X %d %d %d %d %X %d",
		&sl, &localip, &udpSocket.LocalAddress.Port, &remoteip, &udpSocket.RemoteAddress.Port,
		&udpSocket.State, &udpSocket.TXQueue, &udpSocket.RXQueue, &tr, &tmwhen, &retransmit,
		&udpSocket.UID, &timeout, &udpSocket.Inode, &udpSocket.RefCount, &udpSocket.Pointer,
		&udpSocket.Drops)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	udpSocket.LocalAddress.IP = hexToIP(localip)
	udpSocket.RemoteAddress.IP = hexToIP(remoteip)
	return udpSocket, nil
}

func NewUnixSocketFromLine(line string) (*UnixSocket, error) {
	// Num               RefCount Protocol Flags    Type St Inode Path
	// ffff91e759dfb800: 00000002 00000000 00010000 0001 01 16163 /tmp/sddm-auth3949710e-7c3f-4aa2-b5fc-25cc34a7f31e
	var (
		pointer uintptr
	)
	unixSocket := &UnixSocket{}
	n, err := fmt.Sscanf(line, "%X: %X %X %X %X %X %d %s",
		&pointer, &unixSocket.RefCount, &unixSocket.Protocol, &unixSocket.Flags,
		&unixSocket.Type, &unixSocket.State, &unixSocket.Inode, &unixSocket.Path)
	if err != nil && n < 7 {
		return nil, trace.Wrap(err)
	}
	return unixSocket, nil
}

func hexToIP(in []byte) net.IP {
	ip := make([]byte, len(in))
	for i, v := range in {
		ip[len(ip)-i-1] = v
	}
	return ip
}
