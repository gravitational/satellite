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
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gravitational/trace"
	"github.com/prometheus/procfs"
	log "github.com/sirupsen/logrus"
)

func newPortCollector() (*portCollector, error) {
	procs, err := procfs.AllProcs()
	if err != nil {
		return nil, trace.Wrap(err, "failed to query running processes")
	}
	fds, err := getFds(procs)
	if err != nil {
		return nil, trace.Wrap(err, "failed to query open file descriptors")
	}
	return &portCollector{fds}, nil
}

func (r *portCollector) tcp() (ret []process, err error) {
	sockets, err := getTCPSockets(procNetTCP)
	if err != nil {
		return nil, trace.Wrap(err, "failed to query tcp connections")
	}

	sockets6, err := getTCPSockets(procNetTCP6)
	if err != nil {
		return nil, trace.Wrap(err, "failed to query tcp6 connections")
	}

	for _, socket := range append(sockets, sockets6...) {
		proc, err := r.findProcessByInode(socket.inode())
		if err != nil {
			log.Warnf(err.Error())
		}
		ret = append(ret, process{
			name:   proc.name,
			pid:    proc.pid,
			socket: socket,
		})
	}
	return ret, nil
}

func (r *portCollector) udp() (ret []process, err error) {
	sockets, err := getUDPSockets(procNetUDP)
	if err != nil {
		return nil, trace.Wrap(err, "failed to query udp connections")
	}

	sockets6, err := getUDPSockets(procNetUDP6)
	if err != nil {
		return nil, trace.Wrap(err, "failed to query udp6 connections")
	}

	for _, socket := range append(sockets, sockets6...) {
		proc, err := r.findProcessByInode(socket.inode())
		if err != nil {
			log.Warnf(err.Error())
		}
		ret = append(ret, process{
			name:   proc.name,
			pid:    proc.pid,
			socket: socket,
		})
	}
	return ret, nil
}

func (r *portCollector) findProcessByInode(inode string) (proc process, err error) {
	pid := pidUnknown
	if fds, exists := r.fds[inode]; exists {
		for _, fd := range fds {
			pid = fd.pid
			break
		}
	}
	proc = process{pid: pid, name: valueUnknown}

	comm := valueUnknown
	if pid != pidUnknown {
		p, err := procfs.NewProc(pid)
		if err != nil {
			return proc, trace.Wrap(err, "failed to query process metadata for pid %v", pid)
		}
		comm, err = p.Comm()
		if err != nil {
			return proc, trace.Wrap(err, "failed to query process name for pid %v", pid)
		}
		proc.name = comm
		return proc, nil
	}
	return proc, trace.NotFound("no process found for inode %v", inode)
}

type portCollector struct {
	fds map[string][]pidToFd
}

type process struct {
	name string
	pid  int
	socket
}

func (r tcpSocket) localAddr() addr {
	return addr{r.LocalAddress.IP, r.LocalAddress.Port}
}

func (r tcpSocket) inode() string {
	return strconv.FormatUint(uint64(r.Inode), 10)
}

func (r tcpSocket) state() socketState {
	return r.State
}

func (r udpSocket) localAddr() addr {
	return addr{r.LocalAddress.IP, r.LocalAddress.Port}
}

func (r udpSocket) inode() string {
	return strconv.FormatUint(uint64(r.Inode), 10)
}

func (r udpSocket) state() socketState {
	return r.State
}

// getTCPSockets reads specified file formatted as /proc/net/tcp{,6} and returns slice of *TCPSocket
func getTCPSockets(fpath string) (ret []tcpSocket, err error) {
	err = parseSocketFile(fpath, func(line string) error {
		socket, err := newTCPSocketFromLine(line)
		if err != nil {
			return trace.Wrap(err)
		}
		ret = append(ret, *socket)
		return nil
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return ret, nil
}

// getUDPSockets reads specified file formatted as /proc/net/udp{,6} and returns slice of *UDPSocket
func getUDPSockets(fpath string) (ret []udpSocket, err error) {
	err = parseSocketFile(fpath, func(line string) error {
		socket, err := newUDPSocketFromLine(line)
		if err != nil {
			return trace.Wrap(err)
		}
		ret = append(ret, *socket)
		return nil
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return ret, nil
}

// getUnixSockets reads specified file formatted as /proc/net/unix and returns slice of *UnixSocket
func getUnixSockets(fpath string) (ret []unixSocket, err error) {
	err = parseSocketFile(fpath, func(line string) error {
		socket, err := newUnixSocketFromLine(line)
		if err != nil {
			return trace.Wrap(err)
		}
		ret = append(ret, *socket)
		return nil
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return ret, nil
}

func parseSocketFile(fpath string, parse socketparser) error {
	fp, err := os.Open(fpath)
	defer fp.Close()
	if err != nil {
		return trace.ConvertSystemError(err)
	}
	lineScanner := bufio.NewScanner(fp)
	lineScanner.Scan() // Drop header line
	for lineScanner.Scan() {
		err := parse(lineScanner.Text())
		if err != nil {
			return trace.Wrap(err)
		}
	}
	if err := lineScanner.Err(); err != nil {
		return trace.Wrap(err)
	}
	return nil
}

type socketparser func(string) error

// newTCPSocketFromLine parses line in /proc/net/tcp{,6} format and returns TCPSocket object and error
func newTCPSocketFromLine(line string) (*tcpSocket, error) {
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
	socket := &tcpSocket{}
	_, err := fmt.Sscanf(line, "%d: %32X:%4X %32X:%4X %2X %8X:%8X %2X:%8X %8X %d %d %d %1s",
		&sl, &localip, &socket.LocalAddress.Port, &remoteip, &socket.RemoteAddress.Port,
		&socket.State, &socket.TXQueue, &socket.RXQueue, &tr, &tmwhen, &retransmit,
		&socket.UID, &timeout, &socket.Inode, &tails)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	socket.LocalAddress.IP = hexToIP(localip)
	socket.RemoteAddress.IP = hexToIP(remoteip)
	return socket, nil
}

// newUDPSocketFromLine parses line in /proc/net/udp{,6} format and returns UDPSocket object and error
func newUDPSocketFromLine(line string) (*udpSocket, error) {
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
	socket := &udpSocket{}
	_, err := fmt.Sscanf(line, "%d: %32X:%4X %32X:%4X %2X %8X:%8X %2X:%8X %8X %d %d %d %d %128X %d",
		&sl, &localip, &socket.LocalAddress.Port, &remoteip, &socket.RemoteAddress.Port,
		&socket.State, &socket.TXQueue, &socket.RXQueue, &tr, &tmwhen, &retransmit,
		&socket.UID, &timeout, &socket.Inode, &socket.RefCount, &pointer,
		&socket.Drops)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	socket.LocalAddress.IP = hexToIP(localip)
	socket.RemoteAddress.IP = hexToIP(remoteip)
	return socket, nil
}

// newUnixSocketFromLine parses line in /proc/net/unix format and returns UnixSocket object and error
func newUnixSocketFromLine(line string) (*unixSocket, error) {
	// Num               RefCount Protocol Flags    Type St Inode Path
	// ffff91e759dfb800: 00000002 00000000 00010000 0001 01 16163 /tmp/sddm-auth3949710e-7c3f-4aa2-b5fc-25cc34a7f31e
	var (
		pointer []byte
	)
	socket := &unixSocket{}
	n, err := fmt.Sscanf(line, "%128X: %8X %8X %8X %4X %2X %d %32000s",
		&pointer, &socket.RefCount, &socket.Protocol, &socket.Flags,
		&socket.Type, &socket.State, &socket.Inode, &socket.Path)
	if err != nil && n < 7 {
		return nil, trace.Wrap(err)
	}
	return socket, nil
}

// hexToIP converts byte slice to net.IP
func hexToIP(in []byte) net.IP {
	ip := make([]byte, len(in))
	for i, v := range in {
		ip[len(ip)-i-1] = v
	}
	return ip
}

// getFds fetches open file descriptors for all running processes.
// It will skip processes it does not have access to.
func getFds(procs []procfs.Proc) (map[string][]pidToFd, error) {
	fds := make(map[string][]pidToFd, len(procs))
	for _, proc := range procs {
		err := mapPidToFds(proc.PID, fds, -1)
		if err != nil {
			log.Warnf("failed to query open file descriptors for process %v: %v", proc.PID, err)
			continue
		}
	}
	return fds, nil
}

// mapPidToFds retrieves the list of open file descriptors for the process
// identified with pid as a map of inode -> file descriptor
// With max > 0, the number of file descriptor entries consumed is limited to max.
func mapPidToFds(pid int, fds map[string][]pidToFd, max int) error {
	dir := fdDir(pid)
	f, err := os.Open(dir)
	if err != nil {
		return trace.ConvertSystemError(err)
	}
	defer f.Close()

	files, err := f.Readdir(max)
	if err != nil {
		return err
	}

	for _, f := range files {
		inodePath := filepath.Join(dir, f.Name())
		inode, err := os.Readlink(inodePath)
		if err != nil {
			log.Debugf("failed to readlink(%q): %v", inodePath, err)
			continue
		}
		if !strings.HasPrefix(inode, socketPrefix) {
			continue
		}
		// Extract inode value from 'socket:[inode]'
		inode = inode[len(socketPrefix) : len(inode)-1]
		fd, err := strconv.Atoi(f.Name())
		if err != nil {
			log.Debugf("file descriptor is not a valid number: %q", f.Name())
			continue
		}

		item := pidToFd{
			pid: pid,
			fd:  uint(fd),
		}
		fds[inode] = append(fds[inode], item)
	}
	return nil
}

func fdDir(pid int) string {
	return filepath.Join("/proc", strconv.FormatInt(int64(pid), 10), "fd")
}

type pidToFd struct {
	pid int
	fd  uint
}

const socketPrefix = "socket:["

// valueUnknown specifies a value placeholder when the actual data is unavailable
const valueUnknown = "<unknown>"

// pidUnknown identifies a process we failed to match to an id
const pidUnknown = -1

type socket interface {
	localAddr() addr
	inode() string
	state() socketState
}

type addr struct {
	ip   net.IP
	port int
}

// procNet* are standard paths to Linux procfs information on sockets
const (
	procNetTCP  = "/proc/net/tcp"
	procNetTCP6 = "/proc/net/tcp6"
	procNetUDP  = "/proc/net/udp"
	procNetUDP6 = "/proc/net/udp6"
	procNetUnix = "/proc/net/unix"
)

// SocketState stores Linux socket state
type socketState uint8

// Possible Linux socket states
const (
	Established socketState = 0x01
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

// tcpSocket contains details on a TCP socket
type tcpSocket struct {
	LocalAddress  net.TCPAddr
	RemoteAddress net.TCPAddr
	State         socketState
	TXQueue       uint
	RXQueue       uint
	TimerState    uint
	TimeToTimeout uint
	Retransmit    uint
	UID           uint
	Inode         uint
}

// udpSocket contains details on a UDP socket
type udpSocket struct {
	LocalAddress  net.UDPAddr
	RemoteAddress net.UDPAddr
	State         socketState
	TXQueue       uint
	RXQueue       uint
	UID           uint
	Inode         uint
	RefCount      uint
	Drops         uint
}

// unixSocket contains details on a unix socket
type unixSocket struct {
	RefCount uint
	Protocol uint
	Flags    uint
	Type     uint
	State    uint
	Inode    uint
	Path     string
}
