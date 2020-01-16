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
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/gravitational/satellite/agent/backend/inmemory"

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/lib/history/memory"
	"github.com/gravitational/satellite/lib/test"
	"github.com/gravitational/satellite/utils"

	"github.com/gravitational/roundtrip"
	"github.com/gravitational/trace"
	serf "github.com/hashicorp/serf/client"
	"github.com/jonboulle/clockwork"
	log "github.com/sirupsen/logrus"
	. "gopkg.in/check.v1"
)

func TestAgent(t *testing.T) { TestingT(t) }

type AgentSuite struct {
	certFile, keyFile string
	clock             clockwork.Clock
}

var _ = Suite(&AgentSuite{})

func (r *AgentSuite) SetUpSuite(c *C) {
	// Set logging level
	if testing.Verbose() {
		log.SetOutput(os.Stderr)
		log.SetLevel(log.DebugLevel)
	}

	// Initialize credentials
	dir := c.MkDir()
	r.certFile = filepath.Join(dir, "server.crt")
	r.keyFile = filepath.Join(dir, "server.key")
	c.Assert(utils.GenerateCert(r.certFile, r.keyFile), IsNil)

	r.clock = clockwork.NewFakeClock()
}

// TestAgentProvidesStatus validates the communication between several agents
// to exchange health status information.
func (r *AgentSuite) TestAgentProvidesStatus(c *C) {
	var agentTestCases = []struct {
		comment      string
		status       pb.SystemStatus
		localConfig  testAgentConfig
		remoteConfig testAgentConfig
	}{
		{
			comment: "Degraded due to a failed checker",
			status: pb.SystemStatus{
				Timestamp: pb.NewTimeToProto(r.clock.Now()),
				Status:    pb.SystemStatus_Degraded,
				Nodes: []*pb.NodeStatus{
					{
						Name:   "master",
						Status: pb.NodeStatus_Degraded,
						MemberStatus: &pb.MemberStatus{Name: "master", Addr: "<nil>:0",
							Tags: tags{"role": string(RoleMaster)}, Status: pb.MemberStatus_Alive},
						Probes: []*pb.Probe{failedProbe, healthyProbe},
					},
					{
						Name:   "node",
						Status: pb.NodeStatus_Running,
						MemberStatus: &pb.MemberStatus{Name: "node", Addr: "<nil>:0",
							Tags: tags{"role": string(RoleNode)}, Status: pb.MemberStatus_Alive},
						Probes: []*pb.Probe{healthyProbe, healthyProbe},
					},
				},
			},
			localConfig: testAgentConfig{
				node: "master",
				members: []serf.Member{
					newMember("master", "alive"),
					newMember("node", "alive"),
				},
				checkers: []health.Checker{healthyTest, failedTest},
			},
			remoteConfig: testAgentConfig{
				node: "node",
				members: []serf.Member{
					newMember("master", "alive"),
					newMember("node", "alive"),
				},
				checkers: []health.Checker{healthyTest, healthyTest},
			},
		},
		{
			comment: "Degraded due to a missing master node",
			status: pb.SystemStatus{
				Timestamp: pb.NewTimeToProto(r.clock.Now()),
				Status:    pb.SystemStatus_Degraded,
				Nodes: []*pb.NodeStatus{
					{
						Name:   "node-1",
						Status: pb.NodeStatus_Running,
						MemberStatus: &pb.MemberStatus{Name: "node-1", Addr: "<nil>:0",
							Tags: tags{"role": string(RoleNode)}, Status: pb.MemberStatus_Alive},
						Probes: []*pb.Probe{healthyProbe, healthyProbe},
					},
					{
						Name:   "node-2",
						Status: pb.NodeStatus_Running,
						MemberStatus: &pb.MemberStatus{Name: "node-2", Addr: "<nil>:0",
							Tags: tags{"role": string(RoleNode)}, Status: pb.MemberStatus_Alive},
						Probes: []*pb.Probe{healthyProbe, healthyProbe},
					},
				},
				Summary: errNoMaster.Error(),
			},
			localConfig: testAgentConfig{
				node: "node-1",
				members: []serf.Member{
					newMember("node-1", "alive"),
					newMember("node-2", "alive"),
				},
				checkers: []health.Checker{healthyTest, healthyTest},
			},
			remoteConfig: testAgentConfig{
				node: "node-2",
				members: []serf.Member{
					newMember("node-1", "alive"),
					newMember("node-2", "alive"),
				},
				checkers: []health.Checker{healthyTest, healthyTest},
			},
		},
		{
			comment: "Running with all systems running",
			status: pb.SystemStatus{
				Timestamp: pb.NewTimeToProto(r.clock.Now()),
				Status:    pb.SystemStatus_Running,
				Nodes: []*pb.NodeStatus{
					{
						Name:   "master",
						Status: pb.NodeStatus_Running,
						MemberStatus: &pb.MemberStatus{Name: "master", Addr: "<nil>:0",
							Tags: tags{"role": string(RoleMaster)}, Status: pb.MemberStatus_Alive},
						Probes: []*pb.Probe{healthyProbe, healthyProbe},
					},
					{
						Name:   "node",
						Status: pb.NodeStatus_Running,
						MemberStatus: &pb.MemberStatus{Name: "node", Addr: "<nil>:0",
							Tags: tags{"role": string(RoleNode)}, Status: pb.MemberStatus_Alive},
						Probes: []*pb.Probe{healthyProbe, healthyProbe},
					},
				},
			},
			localConfig: testAgentConfig{
				node: "master",
				members: []serf.Member{
					newMember("master", "alive"),
					newMember("node", "alive"),
				},
				checkers: []health.Checker{healthyTest, healthyTest},
			},
			remoteConfig: testAgentConfig{
				node: "node",
				members: []serf.Member{
					newMember("master", "alive"),
					newMember("node", "alive"),
				},
				checkers: []health.Checker{healthyTest, healthyTest},
			},
		},
		{
			comment: `Ignores members in "left" state`,
			status: pb.SystemStatus{
				Timestamp: pb.NewTimeToProto(r.clock.Now()),
				Status:    pb.SystemStatus_Running,
				Nodes: []*pb.NodeStatus{
					{
						Name:   "master",
						Status: pb.NodeStatus_Running,
						MemberStatus: &pb.MemberStatus{Name: "master", Addr: "<nil>:0",
							Tags: tags{"role": string(RoleMaster)}, Status: pb.MemberStatus_Alive},
						Probes: []*pb.Probe{healthyProbe, healthyProbe},
					},
				},
			},
			localConfig: testAgentConfig{
				node: "master",
				members: []serf.Member{
					newMember("master", "alive"),
					newMember("node", "left"),
				},
				checkers: []health.Checker{healthyTest, healthyTest},
			},
			remoteConfig: testAgentConfig{
				node: "node",
				members: []serf.Member{
					newMember("master", "alive"),
					newMember("node", "left"),
				},
				checkers: []health.Checker{healthyTest, healthyTest},
			},
		},
	}

	for _, testCase := range agentTestCases {
		comment := Commentf(testCase.comment)
		localAgent := r.newLocalNode(testCase.localConfig)
		remoteAgent, err := r.newRemoteNode(testCase.remoteConfig)
		c.Assert(err, IsNil)

		test.WithTimeout(func(ctx context.Context) {
			c.Assert(localAgent.Start(), IsNil)
			defer localAgent.Close()

			c.Assert(remoteAgent.Start(), IsNil)
			defer remoteAgent.Close()

			// Update remote agent status first so local agent will see remote agent's status as well.
			c.Assert(remoteAgent.updateStatus(ctx), IsNil)
			c.Assert(localAgent.updateStatus(ctx), IsNil)

			resp, err := localAgent.rpc.Status(ctx, &pb.StatusRequest{})
			c.Assert(err, IsNil, comment)
			c.Assert(sortProbes(*resp.Status), test.DeepCompare, testCase.status, comment)
		})
	}
}

func (r *AgentSuite) TestIsMember(c *C) {
	// Define base agent config
	// Only need to update members field for these tests.
	name := "node1"
	config := testAgentConfig{
		node:     name,
		rpcPort:  7676,
		checkers: []health.Checker{healthyTest},
		clock:    clockwork.NewFakeClock(),
	}

	// Cannot be a member of a single node cluster
	configTest1 := config
	configTest1.members = []serf.Member{newMember(name, "alive")}
	agent := r.newAgent(configTest1)
	c.Assert(agent.IsMember(), Equals, false)

	configTest2 := config
	configTest2.members = []serf.Member{newMember("node2", "alive"), newMember("node3", "alive")}
	agent = r.newAgent(configTest2)
	c.Assert(agent.IsMember(), Equals, false)

	configTest3 := config
	configTest3.members = []serf.Member{newMember(name, "alive"), newMember("node2", "alive")}
	agent = r.newAgent(configTest3)
	c.Assert(agent.IsMember(), Equals, true)
}

func (r *AgentSuite) TestReflectsSystemStatusInStatusCode(c *C) {
	rpcPort := 7575
	config := testAgentConfig{
		node:     "master",
		rpcPort:  rpcPort,
		members:  []serf.Member{newMember("master", "alive")},
		checkers: []health.Checker{healthyTest, failedTest},
	}
	expected := pb.SystemStatus{
		Timestamp: pb.NewTimeToProto(r.clock.Now()),
		Status:    pb.SystemStatus_Degraded,
		Nodes: []*pb.NodeStatus{
			&pb.NodeStatus{Name: "master", Status: pb.NodeStatus_Degraded,
				MemberStatus: &pb.MemberStatus{
					Name:   "master",
					Addr:   "<nil>:0",
					Status: pb.MemberStatus_Alive,
					Tags:   tags{"role": string(RoleMaster)},
				},
				Probes: []*pb.Probe{failedProbe, healthyProbe},
			},
		},
	}

	test.WithTimeout(func(ctx context.Context) {
		agent, err := r.newRemoteNode(config)
		c.Assert(err, IsNil)
		c.Assert(agent.Start(), IsNil)
		defer agent.Close()

		c.Assert(agent.updateStatus(ctx), IsNil)

		client, err := r.httpClient(fmt.Sprintf("https://127.0.0.1:%v", rpcPort))
		c.Assert(err, IsNil)

		resp, err := client.Get(ctx, client.Endpoint(""), url.Values{})
		c.Assert(err, IsNil)
		c.Assert(resp.Code(), Equals, http.StatusServiceUnavailable)

		var status pb.SystemStatus
		c.Assert(json.Unmarshal(resp.Bytes(), &status), IsNil)
		c.Assert(sortProbes(status), test.DeepCompare, expected)
	})
}

func (r *AgentSuite) TestReflectsNodeStatusInStatusCode(c *C) {
	rpcPort := 7575
	config := testAgentConfig{
		node:     "master",
		members:  []serf.Member{newMember("master", "alive")},
		checkers: []health.Checker{healthyTest, failedTest},
	}
	expected := pb.NodeStatus{
		Name:   "master",
		Status: pb.NodeStatus_Degraded,
		MemberStatus: &pb.MemberStatus{
			Name:   "master",
			Addr:   "<nil>:0",
			Status: pb.MemberStatus_Alive,
			Tags:   tags{"role": string(RoleMaster)},
		},
		Probes: []*pb.Probe{failedProbe, healthyProbe},
	}

	test.WithTimeout(func(ctx context.Context) {
		agent, err := r.newRemoteNode(config)
		c.Assert(err, IsNil)
		c.Assert(agent.Start(), IsNil)
		defer agent.Close()

		c.Assert(agent.updateStatus(ctx), IsNil)

		client, err := r.httpClient(fmt.Sprintf("https://127.0.0.1:%v/local", rpcPort))
		c.Assert(err, IsNil)

		resp, err := client.Get(ctx, client.Endpoint(""), url.Values{})
		c.Assert(err, IsNil)
		c.Assert(resp.Code(), Equals, http.StatusServiceUnavailable)

		var status pb.NodeStatus
		c.Assert(json.Unmarshal(resp.Bytes(), &status), IsNil)
		sort.Sort(byChecker(status.Probes))
		c.Assert(status, test.DeepCompare, expected)
	})
}

func (r *AgentSuite) TestFailsIfTimesOutToCollectStatus(c *C) {
	localConfig := testAgentConfig{
		node:     "master",
		members:  []serf.Member{newMember("master", "alive")},
		checkers: []health.Checker{healthyTest},
	}
	expected := pb.SystemStatus{
		Timestamp: pb.NewTimeToProto(r.clock.Now()),
		Status:    pb.SystemStatus_Degraded,
		Summary:   fmt.Sprintf(msgNoStatus, "master,"),
	}
	localAgent := r.newLocalNode(localConfig)
	localAgent.statusQueryReplyTimeout = 0 * time.Second

	test.WithTimeout(func(ctx context.Context) {
		c.Assert(localAgent.Start(), IsNil)
		defer localAgent.Close()

		c.Assert(localAgent.updateStatus(ctx), IsNil)

		resp, err := localAgent.rpc.Status(ctx, &pb.StatusRequest{})
		c.Assert(err, IsNil)
		c.Assert(sortProbes(*resp.Status), test.DeepCompare, expected)
	})
}

// testAgentConfig specifies config values for testAgent.
type testAgentConfig struct {
	node     string
	rpcPort  int
	members  []serf.Member
	checkers []health.Checker
	clock    clockwork.Clock
}

// setDefaults sets default config values if not previously defined.
func (config *testAgentConfig) setDefaults() {
	if config.rpcPort == 0 {
		config.rpcPort = 7575
	}
	if config.clock == nil {
		config.clock = clockwork.NewFakeClock()
	}
}

// testAgent adds additional testing functionality to agent.
type testAgent struct {
	*agent
}

// newLocalNode creates a new instance of the local agent - agent used to make status queries.
func (r *AgentSuite) newLocalNode(config testAgentConfig) *testAgent {
	config.setDefaults()
	agent := r.newAgent(config)
	agent.rpc = &testServer{&server{agent: agent}}
	return &testAgent{agent: agent}
}

// newRemoteNode creates a new instance of a remote agent - agent used to answer
// status query requests via a running RPC endpoint.
func (r *AgentSuite) newRemoteNode(config testAgentConfig) (*testAgent, error) {
	config.setDefaults()
	agent := r.newAgent(config)
	addr := fmt.Sprintf("127.0.0.1:%v", config.rpcPort)

	// Use the same CA certificate for client and server
	server, err := newRPCServer(agent, r.certFile, r.certFile, r.keyFile, []string{addr})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	agent.rpc = server

	return &testAgent{agent: agent}, nil
}

// newAgent creates a new agent instance.
func (r *AgentSuite) newAgent(config testAgentConfig) *agent {
	// timelineCapacity specifies the default timeline capacity for tests.
	timelineCapacity := 256

	config.setDefaults()
	timeline := memory.NewTimeline(config.clock, timelineCapacity)

	agentConfig := Config{
		Cache: inmemory.New(),
		Name:  config.node,
		Clock: config.clock,
	}

	return &agent{
		SerfClient:              &MockSerfClient{members: config.members},
		Timeline:                timeline,
		dialRPC:                 testDialRPC(config.rpcPort, r.certFile, r.keyFile),
		Config:                  agentConfig,
		Checkers:                config.checkers,
		localStatus:             emptyNodeStatus(config.node),
		statusQueryReplyTimeout: statusQueryReplyTimeout,
		done:                    make(chan struct{}),
	}
}

// testDialRPC is a test implementation of the dialRPC interface,
// that creates an RPC client bound to localhost.
func testDialRPC(port int, certFile, keyFile string) DialRPC {
	return func(ctx context.Context, member *serf.Member) (*client, error) {
		return newClient(ctx, fmt.Sprintf(":%d", port), "agent", certFile, certFile, keyFile)
	}
}

func (r *AgentSuite) httpClient(url string) (*roundtrip.Client, error) {
	// Load client cert/key
	cert, err := tls.LoadX509KeyPair(r.certFile, r.keyFile)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			Certificates:       []tls.Certificate{cert},
		},
	}
	return roundtrip.NewClient(url, "", roundtrip.HTTPClient(&http.Client{Transport: tr}))
}

// newMember creates a new value describing a member of a serf cluster.
func newMember(name string, status string) serf.Member {
	result := serf.Member{
		Name:   name,
		Status: status,
		Tags:   tags{"role": string(RoleNode)},
	}
	if name == "master" {
		result.Tags["role"] = string(RoleMaster)
	}
	return result
}

// sortProbes return system status with sorted probes.
func sortProbes(status pb.SystemStatus) pb.SystemStatus {
	for _, node := range status.Nodes {
		sort.Sort(byChecker(node.Probes))
	}
	return status
}

var healthyTest = &testChecker{
	name: "healthy service",
}

var failedTest = &testChecker{
	name: "failing service",
	err:  errInvalidState,
}

var healthyProbe = &pb.Probe{
	Checker: "healthy service",
	Status:  pb.Probe_Running,
}

var failedProbe = &pb.Probe{
	Checker: "failing service",
	Status:  pb.Probe_Failed,
	Error:   "invalid state",
}

// errInvalidState is a mock error for a failed testChecker.
var errInvalidState = errors.New("invalid state")

// testServer implements RPCServer.
// It is used to mock parts of an RPCServer that are not functional
// during the test.
type testServer struct {
	*server
}

// Stop only shuts down HTTP servers.
func (r *testServer) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := r.stopHTTPServers(ctx); err != nil {
		// TODO: return error
	}
}

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

// byChecker implements sort.Interface.
// Enables probes to be sorted.
type byChecker []*pb.Probe

func (r byChecker) Len() int           { return len(r) }
func (r byChecker) Swap(i, j int)      { r[i], r[j] = r[j], r[i] }
func (r byChecker) Less(i, j int) bool { return r[i].Checker < r[j].Checker }

type tags map[string]string
