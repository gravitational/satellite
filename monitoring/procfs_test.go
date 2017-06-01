package monitoring

import (
	"testing"
)

func TestParseSocketFile(t *testing.T) {
	err := parseSocketFile("fixtures/tcp", func(string) error {
		return nil
	})
	if err != nil {
		t.Error()
	}
}

func TestGetTCPSockets(t *testing.T) {
	_, err := GetTCPSockets("fixtures/tcp")
	if err != nil {
		t.Error(err.Error())
	}
	_, err = GetTCPSockets("fixtures/tcp6")
	if err != nil {
		t.Error(err.Error())
	}
}

func TestGetUDPSockets(t *testing.T) {
	_, err := GetUDPSockets("fixtures/udp")
	if err != nil {
		t.Error(err.Error())
	}
	_, err = GetUDPSockets("fixtures/udp6")
	if err != nil {
		t.Error(err.Error())
	}
}

func TestGetUnixSockets(t *testing.T) {
	_, err := GetUnixSockets("fixtures/unix")
	if err != nil {
		t.Error(err.Error())
	}
}

func TestNewTCPSocketFromLine(t *testing.T) {
	sock, err := newTCPSocketFromLine("   13: 6408A8C0:D370 03AD7489:01BB 01 00000000:00000000 02:00000998 00000000  1000        0 4128613 2 ffff8cea5ed24080 56 4 30 10 -1")
	if err != nil {
		t.Error(err.Error())
	}
	if sock.LocalAddress.IP.String() != "192.168.8.100" {
		t.Error()
	}
	if sock.LocalAddress.Port != 54128 {
		t.Error()
	}
	if sock.RemoteAddress.IP.String() != "137.116.173.3" {
		t.Error()
	}
	if sock.RemoteAddress.Port != 443 {
		t.Error()
	}
	if sock.State != Established {
		t.Error()
	}
	if sock.TXQueue != 0 {
		t.Error()
	}
	if sock.RXQueue != 0 {
		t.Error()
	}
	if sock.TimerState != 0 {
		t.Error()
	}
	if sock.TimeToTimeout != 0 {
		t.Error()
	}
	if sock.Retransmit != 0 {
		t.Error()
	}
	if sock.UID != 1000 {
		t.Error()
	}
	if sock.Inode != 4128613 {
		t.Error()
	}
	sock, err = newTCPSocketFromLine("    1: 00000000000000000000000000000000:0016 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 4118579 1 ffff8ceb27309800 100 0 0 10 0")
	if err != nil {
		t.Error(err.Error())
	}
	if sock.LocalAddress.IP.String() != "::" {
		t.Error()
	}
	if sock.LocalAddress.Port != 22 {
		t.Error()
	}
	if sock.RemoteAddress.IP.String() != "::" {
		t.Error()
	}
	if sock.RemoteAddress.Port != 0 {
		t.Error()
	}
	if sock.State != Listen {
		t.Error()
	}
	if sock.TXQueue != 0 {
		t.Error()
	}
	if sock.RXQueue != 0 {
		t.Error()
	}
	if sock.TimerState != 0 {
		t.Error()
	}
	if sock.TimeToTimeout != 0 {
		t.Error()
	}
	if sock.Retransmit != 0 {
		t.Error()
	}
	if sock.UID != 0 {
		t.Error()
	}
	if sock.Inode != 4118579 {
		t.Error()
	}
}

func TestNewUDPSocketFromLine(t *testing.T) {
	sock, err := newUDPSocketFromLine(" 6620: 6408A8C0:A44D BDDCC2AD:01BB 01 00000000:00000000 00:00000000 00000000  1000        0 4216043 2 ffff8cec9b9c2440 0")
	if err != nil {
		t.Error(err.Error())
	}
	if sock.LocalAddress.IP.String() != "192.168.8.100" {
		t.Error()
	}
	if sock.LocalAddress.Port != 42061 {
		t.Error()
	}
	if sock.RemoteAddress.IP.String() != "173.194.220.189" {
		t.Error()
	}
	if sock.RemoteAddress.Port != 443 {
		t.Error()
	}
	if sock.State != 1 {
		t.Error()
	}
	if sock.TXQueue != 0 {
		t.Error()
	}
	if sock.RXQueue != 0 {
		t.Error()
	}
	if sock.UID != 1000 {
		t.Error()
	}
	if sock.Inode != 4216043 {
		t.Error()
	}
	if sock.RefCount != 2 {
		t.Error()
	}
	if sock.Drops != 0 {
		t.Error()
	}
	sock, err = newUDPSocketFromLine(" 2680: 00000000000000000000000000000000:14E9 00000000000000000000000000000000:0000 07 00000000:00000000 00:00000000 00000000  1000        0 4361657 2 ffff8ceb2722a580 0")
	if err != nil {
		t.Error(err.Error())
	}
	if sock.LocalAddress.IP.String() != "::" {
		t.Error()
	}
	if sock.LocalAddress.Port != 5353 {
		t.Error()
	}
	if sock.RemoteAddress.IP.String() != "::" {
		t.Error()
	}
	if sock.RemoteAddress.Port != 0 {
		t.Error()
	}
	if sock.State != 7 {
		t.Error()
	}
	if sock.TXQueue != 0 {
		t.Error()
	}
	if sock.RXQueue != 0 {
		t.Error()
	}
	if sock.UID != 1000 {
		t.Error()
	}
	if sock.Inode != 4361657 {
		t.Error()
	}
	if sock.RefCount != 2 {
		t.Error()
	}
	if sock.Drops != 0 {
		t.Error()
	}
}

func TestNewUnixSocketFromLine(t *testing.T) {
	sock, err := newUnixSocketFromLine("ffff8cec77d65000: 00000003 00000000 00000000 0001 03 4116864")
	if err != nil {
		t.Error(err.Error())
	}
	if sock.Path != "" {
		t.Error()
	}
	if sock.Inode != 4116864 {
		t.Error()
	}
	if sock.RefCount != 3 {
		t.Error()
	}
	if sock.Protocol != 0 {
		t.Error()
	}
	if sock.Flags != 0 {
		t.Error()
	}
	if sock.Type != 1 {
		t.Error()
	}
	if sock.State != 3 {
		t.Error()
	}
	sock, err = newUnixSocketFromLine("ffff8cec9ca9c400: 00000003 00000000 00000000 0001 03 11188 /run/systemd/journal/stdout")
	if err != nil {
		t.Error(err.Error())
	}
	if sock.Path != "/run/systemd/journal/stdout" {
		t.Error()
	}
	if sock.Inode != 11188 {
		t.Error()
	}
	if sock.RefCount != 3 {
		t.Error()
	}
	if sock.Protocol != 0 {
		t.Error()
	}
	if sock.Flags != 0 {
		t.Error()
	}
	if sock.Type != 1 {
		t.Error()
	}
	if sock.State != 3 {
		t.Error()
	}
}
