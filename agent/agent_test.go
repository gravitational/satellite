/*
Copyright 2016-2020 Gravitational, Inc.

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
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/gravitational/satellite/agent/backend/inmemory"
	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	debugpb "github.com/gravitational/satellite/agent/proto/debug"
	"github.com/gravitational/satellite/lib/history/memory"
	"github.com/gravitational/satellite/lib/rpc/client"
	"github.com/gravitational/satellite/lib/test"
	"github.com/gravitational/satellite/utils"

	"github.com/gravitational/trace"
	"github.com/gravitational/ttlmap/v2"
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
	log.SetOutput(os.Stderr)
	log.SetLevel(log.DebugLevel)

	// Initialize credentials
	dir := c.MkDir()
	r.certFile = filepath.Join(dir, "server.crt")
	r.keyFile = filepath.Join(dir, "server.key")
	c.Assert(utils.GenerateCert(r.certFile, r.keyFile), IsNil)

	r.clock = clockwork.NewFakeClock()
}

func (r *AgentSuite) TestAgentProvidesStatus(c *C) {
	var testCases = []struct {
		comment      CommentInterface
		expected     *pb.SystemStatus
		membership   *mockClusterMembership
		agentConfigs []testAgentConfig
	}{
		{
			comment: Commentf("Expected degraded due to a missing master node."),
			expected: &pb.SystemStatus{
				Timestamp: pb.NewTimeToProto(r.clock.Now()),
				Status:    pb.SystemStatus_Degraded,
				Nodes: []*pb.NodeStatus{
					{
						Name:   "node-1",
						Status: pb.NodeStatus_Running,
						MemberStatus: &pb.MemberStatus{Name: "node-1", Addr: "node-1",
							Tags: tags{"role": string(RoleNode)}, Status: pb.MemberStatus_Alive},
						Probes: []*pb.Probe{healthyProbe},
					},
					{
						Name:   "node-2",
						Status: pb.NodeStatus_Running,
						MemberStatus: &pb.MemberStatus{Name: "node-2", Addr: "node-2",
							Tags: tags{"role": string(RoleNode)}, Status: pb.MemberStatus_Alive},
						Probes: []*pb.Probe{healthyProbe},
					},
				},
				Summary: errNoMaster.Error(),
			},
			membership: newMockClusterMembership(),
			agentConfigs: []testAgentConfig{
				{
					node:         "node-1",
					role:         RoleNode,
					memberStatus: MemberAlive,
					checkers:     []health.Checker{healthyTest},
				},
				{
					node:         "node-2",
					role:         RoleNode,
					memberStatus: MemberAlive,
					checkers:     []health.Checker{healthyTest},
				},
			},
		},
		{
			comment: Commentf("Expected degraded due to failed checker"),
			expected: &pb.SystemStatus{
				Timestamp: pb.NewTimeToProto(r.clock.Now()),
				Status:    pb.SystemStatus_Degraded,
				Nodes: []*pb.NodeStatus{
					{
						Name:   "master-1",
						Status: pb.NodeStatus_Running,
						MemberStatus: &pb.MemberStatus{Name: "master-1", Addr: "master-1",
							Tags: tags{"role": string(RoleMaster)}, Status: pb.MemberStatus_Alive},
						Probes: []*pb.Probe{healthyProbe},
					},
					{
						Name:   "node-1",
						Status: pb.NodeStatus_Degraded,
						MemberStatus: &pb.MemberStatus{Name: "node-1", Addr: "node-1",
							Tags: tags{"role": string(RoleNode)}, Status: pb.MemberStatus_Alive},
						Probes: []*pb.Probe{failedProbe},
					},
				},
			},
			membership: newMockClusterMembership(),
			agentConfigs: []testAgentConfig{
				{
					node:         "master-1",
					role:         RoleMaster,
					memberStatus: MemberAlive,
					checkers:     []health.Checker{healthyTest},
				},
				{
					node:         "node-1",
					role:         RoleNode,
					memberStatus: MemberAlive,
					checkers:     []health.Checker{failedTest},
				},
			},
		},
		{
			comment: Commentf("Expected all systems running."),
			expected: &pb.SystemStatus{
				Timestamp: pb.NewTimeToProto(r.clock.Now()),
				Status:    pb.SystemStatus_Running,
				Nodes: []*pb.NodeStatus{
					{
						Name:   "master-1",
						Status: pb.NodeStatus_Running,
						MemberStatus: &pb.MemberStatus{Name: "master-1", Addr: "master-1",
							Tags: tags{"role": string(RoleMaster)}, Status: pb.MemberStatus_Alive},
						Probes: []*pb.Probe{healthyProbe},
					},
					{
						Name:   "node-1",
						Status: pb.NodeStatus_Running,
						MemberStatus: &pb.MemberStatus{Name: "node-1", Addr: "node-1",
							Tags: tags{"role": string(RoleNode)}, Status: pb.MemberStatus_Alive},
						Probes: []*pb.Probe{healthyProbe},
					},
				},
			},
			membership: newMockClusterMembership(),
			agentConfigs: []testAgentConfig{
				{
					node:         "master-1",
					role:         RoleMaster,
					memberStatus: MemberAlive,
					checkers:     []health.Checker{healthyTest},
				},
				{
					node:         "node-1",
					role:         RoleNode,
					memberStatus: MemberAlive,
					checkers:     []health.Checker{healthyTest},
				},
			},
		},
	}

	for _, testCase := range testCases {
		agents := make([]*agent, 0, len(testCase.agentConfigs))
		for _, agentConfig := range testCase.agentConfigs {
			agent, err := r.newAgent(agentConfig, testCase.membership)
			c.Assert(err, IsNil, testCase.comment)
			agents = append(agents, agent)
		}

		test.WithTimeout(func(ctx context.Context) {
			for _, agent := range agents {
				c.Assert(agent.updateStatus(ctx), IsNil, testCase.comment)
			}
			status, err := agents[len(agents)-1].Status()
			c.Assert(err, IsNil, testCase.comment)

			sortStatus(status)
			c.Assert(status, test.DeepCompare, testCase.expected, testCase.comment)
		})
	}
}

// TestRecordLocalTimeline validates that an agent correctly records events
// in to it's local timeline.
func (r *AgentSuite) TestRecordLocalTimeline(c *C) {
	var testCases = []struct {
		comment     CommentInterface
		expected    []*pb.TimelineEvent
		membership  *mockClusterMembership
		agentConfig testAgentConfig
		events      []*pb.TimelineEvent
	}{
		{
			comment:    Commentf("Expected master to record local events."),
			expected:   []*pb.TimelineEvent{pb.NewNodeHealthy(r.clock.Now(), "master-1")},
			membership: newMockClusterMembership(),
			agentConfig: testAgentConfig{
				node: "master-1",
				role: RoleMaster,
			},
		},
		{
			comment:    Commentf("Expected non master to record local events."),
			expected:   []*pb.TimelineEvent{pb.NewNodeHealthy(r.clock.Now(), "node-1")},
			membership: newMockClusterMembership(),
			agentConfig: testAgentConfig{
				node: "node-1",
				role: RoleNode,
			},
		},
	}

	for _, testCase := range testCases {
		agent, err := r.newAgent(testCase.agentConfig, testCase.membership)
		c.Assert(err, IsNil, testCase.comment)

		test.WithTimeout(func(ctx context.Context) {
			_, err := agent.collectLocalStatus(ctx)
			c.Assert(err, IsNil, testCase.comment)

			events, err := agent.LocalTimeline.GetEvents(ctx, nil)
			c.Assert(err, IsNil, testCase.comment)
			c.Assert(events, test.DeepCompare, testCase.expected, testCase.comment)
		})
	}
}

// TestUpdateTimeline validates that an agent can correctly record it's cluster
// timeline with new events.
func (r *AgentSuite) TestRecordTimeline(c *C) {
	var testCases = []struct {
		comment     CommentInterface
		expected    []*pb.TimelineEvent
		membership  *mockClusterMembership
		agentConfig testAgentConfig
		events      []*pb.TimelineEvent
	}{
		{
			comment:    Commentf("Expected master-1 to record events to cluster timeline."),
			expected:   []*pb.TimelineEvent{pb.NewNodeHealthy(r.clock.Now(), "master-1")},
			membership: newMockClusterMembership(),
			agentConfig: testAgentConfig{
				node: "master-1",
				role: RoleMaster,
			},
			events: []*pb.TimelineEvent{pb.NewNodeHealthy(r.clock.Now(), "master-1")},
		},
	}

	for _, testCase := range testCases {
		agent, err := r.newAgent(testCase.agentConfig, testCase.membership)
		c.Assert(err, IsNil, testCase.comment)

		test.WithTimeout(func(ctx context.Context) {
			c.Assert(agent.RecordClusterEvents(ctx, testCase.events), IsNil, testCase.comment)

			events, err := agent.ClusterTimeline.GetEvents(ctx, nil)
			c.Assert(err, IsNil, testCase.comment)
			c.Assert(events, test.DeepCompare, testCase.expected, testCase.comment)
		})
	}
}

// TestAgentProvidesLastSeen validates that the agent is correctly recording
// last seen timestamps on master nodes.
func (r *AgentSuite) TestAgentProvidesLastSeen(c *C) {
	var testCases = []struct {
		comment     CommentInterface
		expected    time.Time
		agentConfig testAgentConfig
		timestamps  []time.Time
	}{
		{
			comment:  Commentf("Expected the latest timestamp to be last seen."),
			expected: r.clock.Now(),
			agentConfig: testAgentConfig{
				node: "master-1",
				role: RoleMaster,
			},
			timestamps: []time.Time{
				r.clock.Now().Add(-time.Second),
				r.clock.Now(),
			},
		},
		{
			comment:  Commentf("Expected attempt to record older timestamp to be ignored."),
			expected: r.clock.Now(),
			agentConfig: testAgentConfig{
				node: "master-1",
				role: RoleMaster,
			},
			timestamps: []time.Time{
				r.clock.Now(),
				r.clock.Now().Add(-time.Second),
			},
		},
	}

	client := newMockClusterMembership()
	for _, testCase := range testCases {
		agent, err := r.newAgent(testCase.agentConfig, client)
		c.Assert(err, IsNil, testCase.comment)

		test.WithTimeout(func(ctx context.Context) {
			for _, timestamp := range testCase.timestamps {
				c.Assert(agent.RecordLastSeen(agent.Name, timestamp), IsNil, testCase.comment)
			}

			timestamp, err := agent.LastSeen(agent.Name)
			c.Assert(err, IsNil, testCase.comment)
			c.Assert(timestamp, test.DeepCompare, testCase.expected, testCase.comment)
		})
	}
}

// TestProvidesTimeline validates communication between cluster members. Members
// should be able to notify all master nodes of their local timeline events.
func (r *AgentSuite) TestProvidesTimeline(c *C) {
	var testCases = []struct {
		comment       CommentInterface
		expected      []*pb.TimelineEvent
		membership    *mockClusterMembership
		masterConfigs []testAgentConfig
		nodeConfigs   []testAgentConfig
		events        []*pb.TimelineEvent
	}{
		{
			comment:    Commentf("Expected master to push local events to it's own cluster timeline."),
			expected:   []*pb.TimelineEvent{pb.NewNodeHealthy(r.clock.Now(), "master-1")},
			membership: newMockClusterMembership(),
			masterConfigs: []testAgentConfig{
				{
					node: "master-1",
					role: RoleMaster,
				},
			},
		},
		{
			comment: Commentf("Expected node to push push it's local events to the master."),
			expected: []*pb.TimelineEvent{
				pb.NewNodeHealthy(r.clock.Now(), "master-1"),
				pb.NewNodeHealthy(r.clock.Now(), "node-1"),
			},
			membership: newMockClusterMembership(),
			masterConfigs: []testAgentConfig{
				{
					node: "master-1",
					role: RoleMaster,
				},
			},
			nodeConfigs: []testAgentConfig{
				{
					node: "node-1",
					role: RoleMaster,
				},
			},
		},
		{
			comment: Commentf("Expected master nodes to notify each other of local events."),
			expected: []*pb.TimelineEvent{
				pb.NewNodeHealthy(r.clock.Now(), "master-1"),
				pb.NewNodeHealthy(r.clock.Now(), "master-2"),
				pb.NewNodeHealthy(r.clock.Now(), "master-3"),
			},
			membership: newMockClusterMembership(),
			masterConfigs: []testAgentConfig{
				{
					node: "master-1",
					role: RoleMaster,
				},
				{
					node: "master-2",
					role: RoleMaster,
				},
				{
					node: "master-3",
					role: RoleMaster,
				},
			},
		},
	}

	for _, testCase := range testCases {
		masters := make([]*agent, 0, len(testCase.masterConfigs))
		for _, masterConfig := range testCase.masterConfigs {
			master, err := r.newAgent(masterConfig, testCase.membership)
			c.Assert(err, IsNil, testCase.comment)
			masters = append(masters, master)
		}

		nodes := make([]*agent, 0, len(testCase.nodeConfigs))
		for _, nodeConfig := range testCase.nodeConfigs {
			node, err := r.newAgent(nodeConfig, testCase.membership)
			c.Assert(err, IsNil, testCase.comment)
			nodes = append(nodes, node)
		}

		test.WithTimeout(func(ctx context.Context) {
			for _, master := range masters {
				_, err := master.collectLocalStatus(ctx)
				c.Assert(err, IsNil, testCase.comment)
			}

			for _, node := range nodes {
				_, err := node.collectLocalStatus(ctx)
				c.Assert(err, IsNil, testCase.comment)
			}

			for _, master := range masters {
				events, err := master.GetTimeline(ctx, nil)
				c.Assert(err, IsNil, testCase.comment)
				c.Assert(events, test.DeepCompare, testCase.expected, testCase.comment)
			}
		})
	}
}

// testAgentConfig specifies config values for testAgent.
type testAgentConfig struct {
	node         string
	role         Role
	memberStatus MemberStatus
	checkers     []health.Checker
	localStatus  *pb.NodeStatus
	clock        clockwork.Clock
}

// setDefaults sets default config values if not previously defined.
func (config *testAgentConfig) setDefaults() {
	if config.clock == nil {
		config.clock = clockwork.NewFakeClock()
	}
	if config.localStatus == nil {
		config.localStatus = emptyNodeStatus(config.node)
	}
	if config.role == "" {
		config.role = RoleNode
	}
}

// newAgent creates a new agent instance.
func (r *AgentSuite) newAgent(config testAgentConfig, client *mockClusterMembership) (*agent, error) {
	// timelineCapacity specifies the default timeline capacity for tests.
	const timelineCapacity = 256
	// clusterCapacity specifies the max number of nodes in a test cluster.
	const clusterCapacity = 3

	config.setDefaults()

	agentConfig := Config{
		Cache:   inmemory.New(),
		Name:    config.node,
		Clock:   config.clock,
		Tags:    tags{"role": string(config.role)},
		DialRPC: client.dial,
		Cluster: client,
	}

	var lastSeen *ttlmap.TTLMap
	if config.role == RoleMaster {
		lastSeen = ttlmap.NewTTLMap(clusterCapacity)
	}

	agent := &agent{
		ClusterTimeline:         memory.NewTimeline(config.clock, timelineCapacity),
		LocalTimeline:           memory.NewTimeline(config.clock, timelineCapacity),
		Config:                  agentConfig,
		Checkers:                config.checkers,
		localStatus:             config.localStatus,
		lastSeen:                lastSeen,
		statusQueryReplyTimeout: statusQueryReplyTimeout,
	}

	client.addAgent(agent)

	return agent, nil
}

func sortStatus(status *pb.SystemStatus) {
	sort.Sort(byName(status.Nodes))
	for _, node := range status.Nodes {
		sort.Sort(byChecker(node.Probes))
	}
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
// Enables probes to be sorted by checker.
type byChecker []*pb.Probe

func (r byChecker) Len() int           { return len(r) }
func (r byChecker) Swap(i, j int)      { r[i], r[j] = r[j], r[i] }
func (r byChecker) Less(i, j int) bool { return r[i].Checker < r[j].Checker }

type tags map[string]string

// byName implements sort.Interface.
// Enables nodes to be sorted by name.
type byName []*pb.NodeStatus

func (r byName) Len() int           { return len(r) }
func (r byName) Swap(i, j int)      { r[i], r[j] = r[j], r[i] }
func (r byName) Less(i, j int) bool { return r[i].Name < r[j].Name }

// implements membership.Cluster
type mockClusterMembership struct {
	agents map[string]*agent
}

func newMockClusterMembership() *mockClusterMembership {
	return &mockClusterMembership{
		agents: make(map[string]*agent),
	}
}

// Members returns the list of cluster members.
func (r mockClusterMembership) Members() ([]*pb.MemberStatus, error) {
	members := make([]*pb.MemberStatus, 0, len(r.agents))
	for _, agent := range r.agents {
		members = append(members, memberFromAgent(agent))
	}
	return members, nil
}

// Member returns the member with the specified name.
func (r mockClusterMembership) Member(name string) (*pb.MemberStatus, error) {
	if agent, ok := r.agents[name]; ok {
		return memberFromAgent(agent), nil
	}
	return nil, trace.BadParameter("node does not have membership")
}

// addAgent adds the agent as a member to the mock cluster.
func (r *mockClusterMembership) addAgent(agent *agent) {
	r.agents[agent.Name] = agent
}

// dial returns a new mockClient for the member specified by name.
func (r *mockClusterMembership) dial(_ context.Context, name string) (client.Client, error) {
	if agent, exists := r.agents[name]; exists {
		return newMockClient(agent)
	}
	return nil, trace.NotFound("%s does not exist in this cluster", name)
}

// memberFromAgent constructs a new member from the provided agent.
func memberFromAgent(agent *agent) *pb.MemberStatus {
	return pb.NewMemberStatus(
		agent.Name,
		agent.Name, // mock dial function will use name to dial node
		agent.Tags,
	)
}

type mockClient struct {
	agent *agent
}

func newMockClient(agent *agent) (client.Client, error) {
	return &mockClient{agent: agent}, nil
}

// Status reports the health status of the cluster.
func (r *mockClient) Status(ctx context.Context) (*pb.SystemStatus, error) {
	return r.agent.Status()
}

// LocalStatus reports the health status of the local cluster node.
func (r *mockClient) LocalStatus(ctx context.Context) (*pb.NodeStatus, error) {
	return r.agent.LocalStatus(), nil
}

// LastSeen requests the last seen timestamp for the specified member.
func (r *mockClient) LastSeen(ctx context.Context, req *pb.LastSeenRequest) (*pb.LastSeenResponse, error) {
	timestamp, err := r.agent.LastSeen(req.GetName())
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return &pb.LastSeenResponse{
		Timestamp: pb.NewTimeToProto(timestamp),
	}, nil
}

// Time returns the current time on the target node.
func (r *mockClient) Time(ctx context.Context, req *pb.TimeRequest) (*pb.TimeResponse, error) {
	return &pb.TimeResponse{
		Timestamp: pb.NewTimeToProto(r.agent.Time().UTC()),
	}, nil
}

// Timeline returns the current status timeline.
func (r *mockClient) Timeline(ctx context.Context, req *pb.TimelineRequest) (*pb.TimelineResponse, error) {
	events, err := r.agent.GetTimeline(ctx, req.GetParams())
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &pb.TimelineResponse{Events: events}, nil
}

// UpdateTimeline requests that the timeline be updated with the specified event.
func (r *mockClient) UpdateTimeline(ctx context.Context, req *pb.UpdateRequest) (*pb.UpdateResponse, error) {
	if err := r.agent.RecordClusterEvents(ctx, []*pb.TimelineEvent{req.GetEvent()}); err != nil {
		return nil, trace.Wrap(err)
	}
	if err := r.agent.RecordLastSeen(req.GetName(), req.GetEvent().GetTimestamp().ToTime()); err != nil {
		return nil, trace.Wrap(err)
	}
	return &pb.UpdateResponse{}, nil
}

// UpdateLocalTimeline requests to update the local timeline with a new event.
func (r *mockClient) UpdateLocalTimeline(ctx context.Context, req *pb.UpdateRequest) (*pb.UpdateResponse, error) {
	if err := r.agent.RecordLocalEvents(ctx, []*pb.TimelineEvent{req.GetEvent()}); err != nil {
		return nil, utils.GRPCError(err)
	}
	return &pb.UpdateResponse{}, nil
}

func (r *mockClient) Profile(context.Context, *debugpb.ProfileRequest) (debugpb.Debug_ProfileClient, error) {
	return nil, nil
}

// Close closes the RPC client connection.
func (r *mockClient) Close() error {
	return nil
}
