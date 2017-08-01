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
	"net"
	"os"
	"testing"
	"time"

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"

	serf "github.com/hashicorp/serf/client"
	"github.com/jonboulle/clockwork"
	log "github.com/sirupsen/logrus"
	. "gopkg.in/check.v1"
)

func TestAgent(t *testing.T) { TestingT(t) }

type AgentSuite struct{}

var _ = Suite(&AgentSuite{})

func (_ *AgentSuite) SetUpSuite(_ *C) {
	if testing.Verbose() {
		log.SetOutput(os.Stderr)
		log.SetLevel(log.DebugLevel)
	}
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
func (_ *AgentSuite) TestAgentProvidesStatus(c *C) {
	for _, testCase := range agentTestCases {
		comment := Commentf(testCase.comment)
		clock := clockwork.NewFakeClock()
		localNode := testCase.members[0].Name
		remoteNode := testCase.members[1].Name
		localAgent := newLocalNode(localNode, testCase.rpcPort,
			testCase.members[:], testCase.checkers[0], clock, c)
		remoteAgent := newRemoteNode(remoteNode, testCase.rpcPort,
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

	agent := newAgent(node, 7676,
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

	agent := newAgent("node1", 7676, []serf.Member{newMember("node1", "alive")},
		[]health.Checker{healthyTest}, statusClock, recycleClock, c)
	c.Assert(agent.IsMember(), Equals, false)

	agent = newAgent("node1", 7676, []serf.Member{newMember("node2", "alive")},
		[]health.Checker{healthyTest}, statusClock, recycleClock, c)
	c.Assert(agent.IsMember(), Equals, false)

	agent = newAgent("node1", 7676, []serf.Member{newMember("node1", "alive"), newMember("node2", "alive")},
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
func newLocalNode(node string, rpcPort int, members []serf.Member,
	checkers []health.Checker, clock clockwork.Clock, c *C) *agent {
	agent := newAgent(node, rpcPort, members, checkers, clock, nil, c)
	agent.rpc = &testServer{&server{agent: agent}}
	err := agent.Start()
	c.Assert(err, IsNil)
	return agent
}

// newRemoteNode creates a new instance of a remote agent - agent used to answer
// status query requests via a running RPC endpoint.
func newRemoteNode(node string, rpcPort int, members []serf.Member,
	checkers []health.Checker, clock clockwork.Clock, c *C) *agent {
	addr := fmt.Sprintf("127.0.0.1:%v", rpcPort)
	listener, err := net.Listen("tcp", addr)
	c.Assert(err, IsNil)

	agent := newAgent(node, rpcPort, members, checkers, clock, nil, c)
	err = agent.Start()
	c.Assert(err, IsNil)
	server := newRPCServer(agent, []net.Listener{listener})
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
func testDialRPC(port int) dialRPC {
	return func(member *serf.Member) (*client, error) {
		addr := fmt.Sprintf(":%d", port)
		client, err := NewClient(addr)
		if err != nil {
			return nil, err
		}
		return client, err
	}
}

// newAgent creates a new agent instance.
func newAgent(node string, rpcPort int, members []serf.Member,
	checkers []health.Checker, statusClock, recycleClock clockwork.Clock, c *C) *agent {
	if recycleClock == nil {
		recycleClock = clockwork.NewFakeClock()
	}
	return &agent{
		name:         node,
		serfClient:   &testSerfClient{members: members},
		dialRPC:      testDialRPC(rpcPort),
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
