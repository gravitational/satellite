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
	"sort"

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/lib/test"

	. "gopkg.in/check.v1"
)

func (*MonitoringSuite) TestValidatesPorts(c *C) {
	// setup
	prober := newErrorProber(portCheckerID)
	var testCases = []struct {
		ports   []PortRange
		probes  health.Probes
		reader  portCollectorFunc
		comment string
	}{
		{
			ports: []PortRange{
				PortRange{From: 9001, To: 9002, Protocol: "tcp"},
			},
			reader:  testGetProcs(),
			probes:  []*pb.Probe{NewSuccessProbe(portCheckerID)},
			comment: "validates port availability",
		},
		{
			ports: []PortRange{
				PortRange{From: 9001, To: 9002, Protocol: "tcp"},
				PortRange{From: 8001, To: 8012, Protocol: "udp"},
			},
			reader: testGetProcs(
				process{
					name:   "foo",
					pid:    100,
					socket: tcpSocket{State: Listen, LocalAddress: tcpAddr("127.0.0.1:9001", c)},
				},
				process{
					name:   "bar",
					pid:    101,
					socket: udpSocket{State: Listen, LocalAddress: udpAddr("127.0.0.1:8005", c)},
				},
			),
			probes: []*pb.Probe{
				prober.newRaised(`conflicting program "bar"(pid=101) is occupying port udp/8005(listen)`),
				prober.newRaised(`conflicting program "foo"(pid=100) is occupying port tcp/9001(listen)`),
			},
			comment: "detects port occupancy",
		},
		{
			ports: []PortRange{
				PortRange{From: 9001, To: 9002, Protocol: "tcp"},
			},
			reader: testFailingGetProcs(fmt.Errorf("unable to read processes")),
			probes: []*pb.Probe{
				prober.newRaisedProbe(probe{
					detail: "failed to query socket connections",
					error:  "unable to read processes",
				}),
			},
			comment: "fails if unable to read process information",
		},
		{
			ports: []PortRange{
				PortRange{From: 9001, To: 9001, Protocol: "tcp"},
				PortRange{From: 9001, To: 9001, Protocol: "udp"},
			},
			reader: testGetProcs(
				process{
					name: "foo",
					pid:  101,
					socket: tcpSocket{
						State:         Listen,
						LocalAddress:  tcpAddr("127.0.0.1:9001", c),
						RemoteAddress: tcpAddr("192.168.178.1:45001", c),
					},
				},
				process{
					name: "foo",
					pid:  101,
					socket: tcpSocket{
						State:         Listen,
						LocalAddress:  tcpAddr("127.0.0.1:9001", c),
						RemoteAddress: tcpAddr("192.168.178.2:45002", c),
					},
				},
				process{
					name: "foo",
					pid:  101,
					socket: udpSocket{
						State:         Listen,
						LocalAddress:  udpAddr("127.0.0.1:9001", c),
						RemoteAddress: udpAddr("192.168.178.2:45003", c),
					},
				},
				process{
					name: "foo",
					pid:  101,
					socket: udpSocket{
						State:         Listen,
						LocalAddress:  udpAddr("127.0.0.1:9001", c),
						RemoteAddress: udpAddr("192.168.178.3:46001", c),
					},
				},
			),
			probes: []*pb.Probe{
				prober.newRaised(`conflicting program "foo"(pid=101) is occupying port tcp/9001(listen)`),
				prober.newRaised(`conflicting program "foo"(pid=101) is occupying port udp/9001(listen)`),
			},
			comment: "dedups processes that have multiple connections on the same port",
		},
		{
			ports: []PortRange{
				PortRange{From: 9001, To: 9001, Protocol: "tcp", ListenAddr: "127.0.0.2"},
			},
			reader: testGetProcs(
				process{
					name: "foo",
					pid:  9001,
					socket: tcpSocket{
						State:         Listen,
						LocalAddress:  tcpAddr("127.0.0.1:9001", c),
						RemoteAddress: tcpAddr("192.168.178.1:45001", c),
					},
				},
			),
			probes: []*pb.Probe{
				&pb.Probe{
					Checker: portCheckerID,
					Status:  pb.Probe_Running,
				},
			},
			comment: "only validates the specified listen address",
		},
		{
			ports: []PortRange{
				PortRange{From: 9001, To: 9001, Protocol: "tcp", ListenAddr: "127.0.0.2"},
			},
			reader: testGetProcs(
				process{
					name:   "foo",
					pid:    100,
					socket: tcpSocket{State: Listen, LocalAddress: tcpAddr("0.0.0.0:9001", c)},
				},
			),
			probes: []*pb.Probe{
				prober.newRaised(`conflicting program "foo"(pid=100) is occupying port tcp/9001(listen)`),
			},
			comment: "detects process bound to any address",
		},
		{
			ports: []PortRange{
				PortRange{From: 9001, To: 9001, Protocol: "tcp"},
			},
			reader: testGetProcs(
				process{
					name: "foo",
					pid:  9001,
					socket: tcpSocket{
						State:         TimeWait,
						LocalAddress:  tcpAddr("127.0.0.1:9001", c),
						RemoteAddress: tcpAddr("192.168.178.1:45001", c),
					},
				},
			),
			probes: []*pb.Probe{
				&pb.Probe{
					Checker: portCheckerID,
					Status:  pb.Probe_Running,
				},
			},
			comment: "ignores open ports in TimeWait",
		},
	}

	// exercise & verify
	for _, testCase := range testCases {
		checker := &portChecker{
			ranges:   testCase.ports,
			getPorts: testCase.reader,
		}
		var probes health.Probes
		checker.Check(context.TODO(), &probes)
		sort.Slice(probes, func(i, j int) bool {
			return probes[i].Detail < probes[j].Detail
		})
		c.Assert(probes, test.DeepCompare, testCase.probes, Commentf(testCase.comment))
	}
}

func testGetProcs(procs ...process) portCollectorFunc {
	return func() ([]process, error) {
		return procs, nil
	}
}

func testFailingGetProcs(err error) portCollectorFunc {
	return func() ([]process, error) {
		return nil, err
	}
}
