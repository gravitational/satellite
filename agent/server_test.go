/*
Copyright 2016 Gravitational, Inc.

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

package agent

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"

	serf "github.com/hashicorp/serf/client"
	"github.com/jonboulle/clockwork"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/credentials"
	. "gopkg.in/check.v1"
)

func TestAgent(t *testing.T) { TestingT(t) }

type AgentSuite struct {
	certFile, keyFile string
}

var _ = Suite(&AgentSuite{})

func (r *AgentSuite) SetUpSuite(c *C) {
	if testing.Verbose() {
		log.SetOutput(os.Stderr)
		log.SetLevel(log.DebugLevel)
	}

	dir := c.MkDir()
	r.certFile = filepath.Join(dir, "server.crt")
	r.keyFile = filepath.Join(dir, "server.key")
	c.Assert(ioutil.WriteFile(r.certFile, certFile, sharedReadWriteMask), IsNil)
	c.Assert(ioutil.WriteFile(r.keyFile, keyFile, sharedReadWriteMask), IsNil)
}

func (_ *AgentSuite) TestSetsSystemStatusFromMemberStatuses(c *C) {
	resp := &pb.StatusResponse{Status: &pb.SystemStatus{}}
	resp.Status.Nodes = []*pb.NodeStatus{
		{
			MemberStatus: &pb.MemberStatus{
				Name:   "foo",
				Status: pb.MemberStatus_Alive,
				Tags:   map[string]string{"role": string(RoleNode)},
			},
		},
		{
			MemberStatus: &pb.MemberStatus{
				Name:   "bar",
				Status: pb.MemberStatus_Failed,
				Tags:   map[string]string{"role": string(RoleMaster)},
			},
		},
	}

	setSystemStatus(resp.Status)
	c.Assert(resp.Status.Status, Equals, pb.SystemStatus_Degraded)
}

func (_ *AgentSuite) TestSetsSystemStatusFromNodeStatuses(c *C) {
	resp := &pb.StatusResponse{Status: &pb.SystemStatus{}}
	resp.Status.Nodes = []*pb.NodeStatus{
		{
			Name:   "foo",
			Status: pb.NodeStatus_Running,
			MemberStatus: &pb.MemberStatus{
				Name:   "foo",
				Status: pb.MemberStatus_Alive,
				Tags:   map[string]string{"role": string(RoleNode)},
			},
		},
		{
			Name:   "bar",
			Status: pb.NodeStatus_Degraded,
			MemberStatus: &pb.MemberStatus{
				Name:   "bar",
				Status: pb.MemberStatus_Alive,
				Tags:   map[string]string{"role": string(RoleMaster)},
			},
			Probes: []*pb.Probe{
				{
					Checker: "qux",
					Status:  pb.Probe_Failed,
					Error:   "not available",
				},
			},
		},
	}

	setSystemStatus(resp.Status)
	c.Assert(resp.Status.Status, Equals, pb.SystemStatus_Degraded)
}

func (_ *AgentSuite) TestDetectsNoMaster(c *C) {
	resp := &pb.StatusResponse{Status: &pb.SystemStatus{}}
	resp.Status.Nodes = []*pb.NodeStatus{
		{
			MemberStatus: &pb.MemberStatus{
				Name:   "foo",
				Status: pb.MemberStatus_Alive,
				Tags:   map[string]string{"role": string(RoleNode)},
			},
		},
		{
			MemberStatus: &pb.MemberStatus{
				Name:   "bar",
				Status: pb.MemberStatus_Alive,
				Tags:   map[string]string{"role": string(RoleNode)},
			},
		},
	}

	setSystemStatus(resp.Status)
	c.Assert(resp.Status.Status, Equals, pb.SystemStatus_Degraded)
	c.Assert(resp.Status.Summary, Equals, errNoMaster.Error())
}

func (_ *AgentSuite) TestSetsOkSystemStatus(c *C) {
	resp := &pb.StatusResponse{Status: &pb.SystemStatus{}}
	resp.Status.Nodes = []*pb.NodeStatus{
		{
			Name:   "foo",
			Status: pb.NodeStatus_Running,
			MemberStatus: &pb.MemberStatus{
				Name:   "foo",
				Status: pb.MemberStatus_Alive,
				Tags:   map[string]string{"role": string(RoleNode)},
			},
		},
		{
			Name:   "bar",
			Status: pb.NodeStatus_Running,
			MemberStatus: &pb.MemberStatus{
				Name:   "bar",
				Status: pb.MemberStatus_Alive,
				Tags:   map[string]string{"role": string(RoleMaster)},
			},
		},
	}

	expectedStatus := pb.SystemStatus_Running
	setSystemStatus(resp.Status)
	c.Assert(resp.Status.Status, Equals, expectedStatus)
}

// TestAgentProvidesStatus validates the communication between several agents
// to exchange health status information.
func (r *AgentSuite) TestAgentProvidesStatus(c *C) {
	for _, testCase := range agentTestCases {
		comment := Commentf(testCase.comment)
		clock := clockwork.NewFakeClock()
		localNode := testCase.members[0].Name
		remoteNode := testCase.members[1].Name
		localAgent := r.newLocalNode(localNode, testCase.rpcPort,
			testCase.members[:], testCase.checkers[0], clock, c)
		remoteAgent := r.newRemoteNode(remoteNode, testCase.rpcPort,
			testCase.members[:], testCase.checkers[1], clock, c)
		defer func() {
			localAgent.Close()
			remoteAgent.Close()
		}()

		// Wait until both agents have started waiting to collect statuses
		clock.BlockUntil(2)
		clock.Advance(statusUpdateTimeout + time.Second)
		// Ensure that the each status update loop has finished updating status
		clock.BlockUntil(2)

		req := &pb.StatusRequest{}
		resp, err := localAgent.rpc.Status(context.TODO(), req)
		c.Assert(err, IsNil, comment)

		c.Assert(resp.Status.Status, Equals, testCase.status, comment)
		c.Assert(resp.Status.Nodes, HasLen, len(testCase.members), comment)
	}
}

func (r *AgentSuite) TestRecyclesCache(c *C) {
	node := "node"
	at := time.Date(1984, time.April, 4, 0, 0, 0, 0, time.UTC)
	statusClock := clockwork.NewFakeClockAt(at)
	recycleClock := clockwork.NewFakeClockAt(at.Add(recycleTimeout + time.Second))
	cache := &testCache{c: c, SystemStatus: &pb.SystemStatus{Status: pb.SystemStatus_Unknown}}

	agent := r.newAgent(node, 7676,
		[]serf.Member{newMember("node", "alive")},
		[]health.Checker{healthyTest},
		statusClock, recycleClock, c)
	agent.rpc = &testServer{&server{agent: agent}}
	agent.cache = cache

	err := agent.Start()
	c.Assert(err, IsNil)

	statusClock.BlockUntil(1)
	// Run the status update loop once to update the status
	statusClock.Advance(statusUpdateTimeout + time.Second)
	statusClock.BlockUntil(1)

	// Run the status update loop to recycle the stats
	recycleClock.Advance(recycleTimeout + time.Second)
	recycleClock.BlockUntil(1)

	status, err := cache.RecentStatus()
	c.Assert(err, IsNil)
	c.Assert(status, DeepEquals, pb.EmptyStatus())
}

func (r *AgentSuite) TestIsMember(c *C) {
	at := time.Date(1984, time.April, 4, 0, 0, 0, 0, time.UTC)
	statusClock := clockwork.NewFakeClockAt(at)
	recycleClock := clockwork.NewFakeClockAt(at.Add(recycleTimeout + time.Second))

	agent := r.newAgent("node1", 7676, []serf.Member{newMember("node1", "alive")},
		[]health.Checker{healthyTest}, statusClock, recycleClock, c)
	c.Assert(agent.IsMember(), Equals, false)

	agent = r.newAgent("node1", 7676, []serf.Member{newMember("node2", "alive")},
		[]health.Checker{healthyTest}, statusClock, recycleClock, c)
	c.Assert(agent.IsMember(), Equals, false)

	agent = r.newAgent("node1", 7676, []serf.Member{newMember("node1", "alive"), newMember("node2", "alive")},
		[]health.Checker{healthyTest}, statusClock, recycleClock, c)
	c.Assert(agent.IsMember(), Equals, true)
}

var healthyTest = &testChecker{
	name: "healthy service",
}

var failedTest = &testChecker{
	name: "failing service",
	err:  errInvalidState,
}

var agentTestCases = []struct {
	comment  string
	status   pb.SystemStatus_Type
	members  [2]serf.Member
	checkers [][]health.Checker
	rpcPort  int
}{
	{
		comment: "Degraded due to a failed checker",
		status:  pb.SystemStatus_Degraded,
		members: [2]serf.Member{
			newMember("master", "alive"),
			newMember("node", "alive"),
		},
		checkers: [][]health.Checker{{healthyTest, failedTest}, {healthyTest, healthyTest}},
		rpcPort:  7676,
	},
	{
		comment: "Degraded due to a missing master node",
		status:  pb.SystemStatus_Degraded,
		members: [2]serf.Member{
			newMember("node-1", "alive"),
			newMember("node-2", "alive"),
		},
		checkers: [][]health.Checker{{healthyTest, healthyTest}, {healthyTest, healthyTest}},
		rpcPort:  7677,
	},
	{
		comment: "Running with all systems running",
		status:  pb.SystemStatus_Running,
		members: [2]serf.Member{
			newMember("master", "alive"),
			newMember("node", "alive"),
		},
		checkers: [][]health.Checker{{healthyTest, healthyTest}, {healthyTest, healthyTest}},
		rpcPort:  7678,
	},
}

// newLocalNode creates a new instance of the local agent - agent used to make status queries.
func (r *AgentSuite) newLocalNode(node string, rpcPort int, members []serf.Member,
	checkers []health.Checker, clock clockwork.Clock, c *C) *agent {
	agent := r.newAgent(node, rpcPort, members, checkers, clock, nil, c)
	agent.rpc = &testServer{&server{agent: agent}}
	err := agent.Start()
	c.Assert(err, IsNil)
	return agent
}

// newRemoteNode creates a new instance of a remote agent - agent used to answer
// status query requests via a running RPC endpoint.
func (r *AgentSuite) newRemoteNode(node string, rpcPort int, members []serf.Member,
	checkers []health.Checker, clock clockwork.Clock, c *C) *agent {
	addr := fmt.Sprintf("127.0.0.1:%v", rpcPort)

	agent := r.newAgent(node, rpcPort, members, checkers, clock, nil, c)
	err := agent.Start()
	c.Assert(err, IsNil)

	server, err := newRPCServer(agent, r.certFile, r.keyFile, []string{addr})
	c.Assert(err, IsNil)

	agent.rpc = server

	return agent
}

// newMember creates a new value describing a member of a serf cluster.
func newMember(name string, status string) serf.Member {
	result := serf.Member{
		Name:   name,
		Status: status,
		Tags:   map[string]string{"role": string(RoleNode)},
	}
	if name == "master" {
		result.Tags["role"] = string(RoleMaster)
	}
	return result
}

// testSerfClient implements serfClient
type testSerfClient struct {
	members []serf.Member
}

func (r *testSerfClient) Members() ([]serf.Member, error) {
	return r.members, nil
}

// Stream returns a dummy stream handle.
func (r *testSerfClient) Stream(filter string, eventc chan<- map[string]interface{}) (serf.StreamHandle, error) {
	return serf.StreamHandle(0), nil
}

func (r *testSerfClient) Stop(handle serf.StreamHandle) error {
	return nil
}

func (r *testSerfClient) Close() error {
	return nil
}

func (r *testSerfClient) Join(peers []string, replay bool) (int, error) {
	return 0, nil
}

func newTestCache(c *C, clock clockwork.Clock) *testCache {
	if clock == nil {
		clock = clockwork.NewFakeClock()
	}
	return &testCache{
		c:            c,
		clock:        clock,
		SystemStatus: &pb.SystemStatus{Status: pb.SystemStatus_Unknown},
	}
}

// testCache implements cache.Cache.
type testCache struct {
	*pb.SystemStatus
	c     *C
	clock clockwork.Clock
}

func (r *testCache) UpdateStatus(status *pb.SystemStatus) error {
	r.SystemStatus = status
	return nil
}

func (r *testCache) RecentStatus() (*pb.SystemStatus, error) {
	return r.SystemStatus, nil
}

func (r *testCache) Recycle() error {
	r.SystemStatus = pb.EmptyStatus()
	return nil
}

func (r *testCache) Close() error {
	return nil
}

// testServer implements RPCServer.
// It is used to mock parts of an RPCServer that are not functional
// during the test.
type testServer struct {
	*server
}

// Stop is a no-op.
func (_ *testServer) Stop() {}

// testDialRPC is a test implementation of the dialRPC interface,
// that creates an RPC client bound to localhost.
func testDialRPC(port int, certFile string) dialRPC {
	return func(member *serf.Member) (*client, error) {
		addr := fmt.Sprintf(":%d", port)
		creds, err := credentials.NewClientTLSFromFile(certFile, "agent")
		if err != nil {
			return nil, err
		}
		client, err := NewClientWithCreds(addr, creds)
		if err != nil {
			return nil, err
		}
		return client, err
	}
}

// newAgent creates a new agent instance.
func (r *AgentSuite) newAgent(node string, rpcPort int, members []serf.Member,
	checkers []health.Checker, statusClock, recycleClock clockwork.Clock, c *C) *agent {
	if recycleClock == nil {
		recycleClock = clockwork.NewFakeClock()
	}
	return &agent{
		name:         node,
		serfClient:   &testSerfClient{members: members},
		dialRPC:      testDialRPC(rpcPort, r.certFile),
		cache:        &testCache{c: c, SystemStatus: &pb.SystemStatus{Status: pb.SystemStatus_Unknown}},
		Checkers:     checkers,
		statusClock:  statusClock,
		recycleClock: recycleClock,
		localStatus:  emptyNodeStatus(node),
	}
}

var errInvalidState = errors.New("invalid state")

// testChecker implements a health.Checker interface for the tests.
type testChecker struct {
	err  error
	name string
}

func (r testChecker) Name() string { return r.name }

func (r *testChecker) Check(ctx context.Context, reporter health.Reporter) {
	if r.err != nil {
		reporter.Add(&pb.Probe{
			Checker: r.name,
			Error:   r.err.Error(),
			Status:  pb.Probe_Failed,
		})
		return
	}
	reporter.Add(&pb.Probe{
		Checker: r.name,
		Status:  pb.Probe_Running,
	})
}

// openssl req -new -x509 -sha256 -key server.key -out server.crt -days 3650, CN=agent
var certFile = []byte(`-----BEGIN CERTIFICATE-----
MIIDjTCCAnWgAwIBAgIJAJ6LjB36T/bKMA0GCSqGSIb3DQEBCwUAMF0xCzAJBgNV
BAYTAlVTMQ0wCwYDVQQIDARUZXN0MQ0wCwYDVQQHDARUZXN0MREwDwYDVQQKDAhB
Y21lIEluYzENMAsGA1UECwwEVGVzdDEOMAwGA1UEAwwFYWdlbnQwHhcNMTcxMDI2
MTUzMjU1WhcNMjcxMDI0MTUzMjU1WjBdMQswCQYDVQQGEwJVUzENMAsGA1UECAwE
VGVzdDENMAsGA1UEBwwEVGVzdDERMA8GA1UECgwIQWNtZSBJbmMxDTALBgNVBAsM
BFRlc3QxDjAMBgNVBAMMBWFnZW50MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIB
CgKCAQEArU+zyHtvQp5ReFcd2ozM54KJS0F6AYyqynikB+YeJzpW6jtNl1ZUjJPz
hzBY37yw3D6XwKyn8jZ/XVJHxKBWXA1IPWYXA4OGR4aIIDFgHFCGMRGayHKe1HSZ
fnGVtBfECtilnekDb0FD7ClDjs4FMoumkY2WfL8M12te/Ns44ZQax1Yh+vcuWRlK
2Ixcqj4+10kvVsc2eBEiWp9b+TVvsi4zh2MkSTiWgY3QW/Z0aeWqkedmW9AWacKc
XAyqb66zfqKt987Rd6ltq5XfQDScqqAgSooj/tJ6t+bx/kLG8mbTTKS90ZXOnl2r
947lxyokbO36k7wwddXGQyZDEWFt8QIDAQABo1AwTjAdBgNVHQ4EFgQUoPfCCLwQ
HyJdy9YoGcVV9iF1x14wHwYDVR0jBBgwFoAUoPfCCLwQHyJdy9YoGcVV9iF1x14w
DAYDVR0TBAUwAwEB/zANBgkqhkiG9w0BAQsFAAOCAQEACJK/WyOeaXSe+l/0k/on
5UDrzl7nAH2DGM8hx8DONDy18lucJ/rkcoWQ04WeZkDUOfKFQXUOnApeEoh8qivl
ZCvBexfJVtZZIVhz5vlZhMIQ8AQsJuIv17F7Ch6C8Cw60nJ4n0eo1h6J3XKFmgHn
NtmRd3zbn+szmZT4ondzHBQF3aH89hm1zMLPJLx3HZnKPeXwCq+eox5/zK3qrJmM
QxI91x30QwxYPRHVygYBU2oHr7sWBYxlhZnvRG1uC2+mlUJtvJhlwsgQuBsTKrKQ
a7wdQWVFansjp4s1c4K/z1QpczEv8y5E2yH5ngatrbv0xHQCtQ1WjPCk177DoNwa
KA==
-----END CERTIFICATE-----`)

// openssl genrsa -out server.key 2048
var keyFile = []byte(`-----BEGIN RSA PRIVATE KEY-----
MIIEogIBAAKCAQEArU+zyHtvQp5ReFcd2ozM54KJS0F6AYyqynikB+YeJzpW6jtN
l1ZUjJPzhzBY37yw3D6XwKyn8jZ/XVJHxKBWXA1IPWYXA4OGR4aIIDFgHFCGMRGa
yHKe1HSZfnGVtBfECtilnekDb0FD7ClDjs4FMoumkY2WfL8M12te/Ns44ZQax1Yh
+vcuWRlK2Ixcqj4+10kvVsc2eBEiWp9b+TVvsi4zh2MkSTiWgY3QW/Z0aeWqkedm
W9AWacKcXAyqb66zfqKt987Rd6ltq5XfQDScqqAgSooj/tJ6t+bx/kLG8mbTTKS9
0ZXOnl2r947lxyokbO36k7wwddXGQyZDEWFt8QIDAQABAoIBAFsiVCmSLtlbIwAi
30HzVDRRAh0emyeBbrX1Zlv499Ys6VNWR+DStrcNfbuTAsj0EhRenbHlmJLXcXYD
NFYC8iaJnXkb2/IvEUc/SQmUrTN2bHoVBc1t6HNTtPs2g0AmVyJU9hHpW7L/INZo
hGvtjfIcWUSkrYN/eyM0BMj2Bh0nxIu4QtLNLZERAMzhd3qzSzMLkO1CYwY+RsqK
5bMqEbPHLly4RZzCBre2bmiVJo9XQ1cDnIKwpK7318HqxKPn1jFarwmY+nc2EJQP
ZnSZmgdq1d5aGpAtc5JnFGKJLakdY1zx8ukvIx1mOTeW/cKR85XitZ/Bej9Bs8Ss
tt6J9CECgYEA2752qVIlohcWsOVd7mCkh1O1MpDmw51PQL8KsjpLimgzENbEf+n1
dpYIzjhbJmgvBRqSwdpiZMKq5NA5B7wun4pfEcREMrUx2pskorhxGDhGnNQcxQZ4
eaOnCUzLzHTPWPKo2ScYUfjt03WoKvZPyNKD+6kx4FMW98+djE9/7eUCgYEAyegE
q0i1fsfhwO93FB6Vd/BH2IQQyoF8turrSsebT5OimhiN3bolEq806eZentb03LeQ
o3LQx5HQR4bQ5dHhFAUEJzsDsF0b7G2FcCSyfJRs0Wh92vqcFvq0qfC6l0nhurli
sIJ65ko8KzANhqvU46oOZDjjO1hsgZfx3+513x0CgYA79F552jDsZbJKN3qGZJXf
WmZw0noz2wLZnoYzlJYxwDZWnNJmOBZB8bObWGL+OqTBlrt96rC33ykzXuCAjMaH
vwArX8pfr3JXu8amIv6wZgJWHcVvuFE8lvsnHW3pbeF42lRZU0JeczWoYUyt1CB2
oYFjM4mpM+JrYJkSxEoaRQKBgDMFZ5ClCgAkoH6xxKSX6etqE626CcgymoJasOSv
tiaQxykrhUX/kPi8v6FPrp9y8GOKG4nCLNIRndFFVyqMM9VsQxVqy07Y6IKBVpP1
IglrNGhigFNCuwjvh5HeHDi42crmp/K0tjvVjIjZVsGuUFjLk2FuIrXPbXP+IogU
6UJdAoGAXHcQvV//kj7oCrqS5NyHHw3P7UigSUCgwzfLnhv1FtIUTlO6rujFro/z
DBsrq5X2wHEJ5a6NT8N17qdFQbvVLQ4fLe7eYRGhdo0lALrB2ULT3LV61Ie8S+AM
0/0n+sOxS6SFc/NS028ePAZZD8veorWCKquh2cIXUS2NyAT6Odg=
-----END RSA PRIVATE KEY-----`)

const sharedReadWriteMask = 0666
