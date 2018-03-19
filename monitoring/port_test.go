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

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/lib/test"

	. "gopkg.in/check.v1"
)

func (*MonitoringSuite) TestValidatesPorts(c *C) {
	// setup
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
				&pb.Probe{
					Checker: portCheckerID,
					Detail:  `conflicting program "foo"(pid=100) is occupying port tcp/9001(listen)`,
					Status:  pb.Probe_Failed,
				},
				&pb.Probe{
					Checker: portCheckerID,
					Detail:  `conflicting program "bar"(pid=101) is occupying port udp/8005(listen)`,
					Status:  pb.Probe_Failed,
				},
			},
			comment: "detects port occupancy",
		},
		{
			ports: []PortRange{
				PortRange{From: 9001, To: 9002, Protocol: "tcp"},
			},
			reader: testFailingGetProcs(fmt.Errorf("unable to read processes")),
			probes: []*pb.Probe{
				&pb.Probe{
					Checker: portCheckerID,
					Detail:  "failed to query socket connections",
					Error:   "unable to read processes",
					Status:  pb.Probe_Failed,
				}},
			comment: "fails if unable to read process information",
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
