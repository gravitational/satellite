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
	"errors"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"

	log "github.com/Sirupsen/logrus"
	serf "github.com/hashicorp/serf/client"
	"github.com/jonboulle/clockwork"
	"golang.org/x/net/context"
	. "gopkg.in/check.v1"
)

func init() {
	if testing.Verbose() {
		log.SetOutput(os.Stderr)
		log.SetLevel(log.InfoLevel)
	}
}

func TestAgent(t *testing.T) { TestingT(t) }

type AgentSuite struct{}

var _ = Suite(&AgentSuite{})

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
		c.Logf("running test %s", testCase.comment)

		clock := clockwork.NewFakeClock()
		localNode := testCase.members[0].Name
		remoteNode := testCase.members[1].Name
		localAgent := newLocalNode(localNode, remoteNode, testCase.rpcPort,
			testCase.members[:], testCase.checkers[0], clock, c)
		remoteAgent, err := newRemoteNode(remoteNode, localNode, testCase.rpcPort,
			testCase.members[:], testCase.checkers[1], clock, c)
		c.Assert(err, IsNil)

		// Wait until both agents have started waiting to collect statuses
		clock.BlockUntil(2)
		clock.Advance(statusUpdateTimeout + time.Second)
		// Ensure that the each status update loop has finished updating status
		clock.BlockUntil(2)

		req := &pb.StatusRequest{}
		resp, err := localAgent.rpc.Status(context.TODO(), req)
		c.Assert(err, IsNil)

		c.Assert(resp.Status.Status, Equals, testCase.status)
		c.Assert(resp.Status.Nodes, HasLen, len(testCase.members))
		localAgent.Close()
		remoteAgent.Close()
	}
}

var healthyTest = &fakeChecker{
	name: "healthy service",
}

var failedTest = &fakeChecker{
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
func newLocalNode(node, peerNode string, rpcPort int, members []serf.Member,
	checkers []health.Checker, clock clockwork.Clock, c *C) *agent {
	agent := newAgent(node, peerNode, rpcPort, members, checkers, clock, c)
	agent.rpc = &fakeServer{&server{agent: agent}}
	err := agent.Start()
	c.Assert(err, IsNil)
	return agent
}

// newRemoteNode creates a new instance of a remote agent - agent used to answer
// status query requests via a running RPC endpoint.
func newRemoteNode(node, peerNode string, rpcPort int, members []serf.Member,
	checkers []health.Checker, clock clockwork.Clock, c *C) (*agent, error) {
	network := "tcp"
	addr := fmt.Sprintf(":%d", rpcPort)
	listener, err := net.Listen(network, addr)
	if err != nil {
		return nil, err
	}

	agent := newAgent(node, peerNode, rpcPort, members, checkers, clock, c)
	err = agent.Start()
	c.Assert(err, IsNil)
	server := newRPCServer(agent, []net.Listener{listener})
	agent.rpc = server

	return agent, nil
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

// fakeSerfClient implements serfClient
type fakeSerfClient struct {
	members []serf.Member
}

func (r *fakeSerfClient) Members() ([]serf.Member, error) {
	return r.members, nil
}

// Stream returns a dummy stream handle.
func (r *fakeSerfClient) Stream(filter string, eventc chan<- map[string]interface{}) (serf.StreamHandle, error) {
	return serf.StreamHandle(0), nil
}

func (r *fakeSerfClient) Stop(handle serf.StreamHandle) error {
	return nil
}

func (r *fakeSerfClient) Close() error {
	return nil
}

func (r *fakeSerfClient) Join(peers []string, replay bool) (int, error) {
	return 0, nil
}

// fakeCache implements cache.Cache.
type fakeCache struct {
	*pb.SystemStatus
	c *C
}

func (r *fakeCache) UpdateStatus(status *pb.SystemStatus) error {
	r.SystemStatus = status
	return nil
}

func (r fakeCache) RecentStatus() (*pb.SystemStatus, error) {
	return r.SystemStatus, nil
}

func (r *fakeCache) Close() error {
	return nil
}

// fakeServer implements RPCServer.
// It is used to mock parts of an RPCServer that are not functional
// during the test.
type fakeServer struct {
	*server
}

// Stop is a no-op.
func (_ *fakeServer) Stop() {}

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
func newAgent(node, peerNode string, rpcPort int, members []serf.Member,
	checkers []health.Checker, clock clockwork.Clock, c *C) *agent {
	return &agent{
		name:        node,
		serfClient:  &fakeSerfClient{members: members},
		dialRPC:     testDialRPC(rpcPort),
		cache:       &fakeCache{c: c, SystemStatus: &pb.SystemStatus{Status: pb.SystemStatus_Unknown}},
		Checkers:    checkers,
		clock:       clock,
		localStatus: emptyNodeStatus(node),
	}
}

var errInvalidState = errors.New("invalid state")

// fakeChecker implements a health.Checker interface for the tests.
type fakeChecker struct {
	err  error
	name string
}

func (r fakeChecker) Name() string { return r.name }

func (r *fakeChecker) Check(reporter health.Reporter) {
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
