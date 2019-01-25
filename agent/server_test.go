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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/lib/test"

	"github.com/gravitational/trace"

	"github.com/gravitational/roundtrip"
	serf "github.com/hashicorp/serf/client"
	"github.com/jonboulle/clockwork"
	log "github.com/sirupsen/logrus"
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
	c.Assert(generateCert(r.certFile, r.keyFile), IsNil)
}

func (_ *AgentSuite) TestSetsSystemStatusFromMemberStatuses(c *C) {
	resp := &pb.StatusResponse{Status: &pb.SystemStatus{}}
	resp.Status.Nodes = []*pb.NodeStatus{
		{
			MemberStatus: &pb.MemberStatus{
				Name:   "foo",
				Status: pb.MemberStatus_Alive,
				Tags:   tags{"role": string(RoleNode)},
			},
		},
		{
			MemberStatus: &pb.MemberStatus{
				Name:   "bar",
				Status: pb.MemberStatus_Failed,
				Tags:   tags{"role": string(RoleMaster)},
			},
		},
	}

	setSystemStatus(resp.Status, []serf.Member{{Name: "foo"}, {Name: "bar"}})
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
				Tags:   tags{"role": string(RoleNode)},
			},
		},
		{
			Name:   "bar",
			Status: pb.NodeStatus_Degraded,
			MemberStatus: &pb.MemberStatus{
				Name:   "bar",
				Status: pb.MemberStatus_Alive,
				Tags:   tags{"role": string(RoleMaster)},
			},
			Probes: []*pb.Probe{
				{
					Checker:  "qux",
					Status:   pb.Probe_Failed,
					Severity: pb.Probe_Critical,
					Error:    "not available",
				},
			},
		},
	}

	setSystemStatus(resp.Status, []serf.Member{{Name: "foo"}, {Name: "bar"}})
	c.Assert(resp.Status.Status, Equals, pb.SystemStatus_Degraded)
}

func (_ *AgentSuite) TestDetectsNoMaster(c *C) {
	resp := &pb.StatusResponse{Status: &pb.SystemStatus{}}
	resp.Status.Nodes = []*pb.NodeStatus{
		{
			Name: "foo",
			MemberStatus: &pb.MemberStatus{
				Name:   "foo",
				Status: pb.MemberStatus_Alive,
				Tags:   tags{"role": string(RoleNode)},
			},
		},
		{
			Name: "bar",
			MemberStatus: &pb.MemberStatus{
				Name:   "bar",
				Status: pb.MemberStatus_Alive,
				Tags:   tags{"role": string(RoleNode)},
			},
		},
	}

	setSystemStatus(resp.Status, []serf.Member{{Name: "foo"}, {Name: "bar"}})
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
				Tags:   tags{"role": string(RoleNode)},
			},
		},
		{
			Name:   "bar",
			Status: pb.NodeStatus_Running,
			MemberStatus: &pb.MemberStatus{
				Name:   "bar",
				Status: pb.MemberStatus_Alive,
				Tags:   tags{"role": string(RoleMaster)},
			},
		},
	}

	expectedStatus := pb.SystemStatus_Running
	setSystemStatus(resp.Status, []serf.Member{{Name: "foo"}, {Name: "bar"}})
	c.Assert(resp.Status.Status, Equals, expectedStatus)
}

// TestAgentProvidesStatus validates the communication between several agents
// to exchange health status information.
func (r *AgentSuite) TestAgentProvidesStatus(c *C) {
	var agentTestCases = []struct {
		comment  string
		members  [2]serf.Member
		checkers [2][]health.Checker
		status   pb.SystemStatus
	}{
		{
			comment: "Degraded due to a failed checker",
			status: pb.SystemStatus{
				Status: pb.SystemStatus_Degraded,
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
			members: [2]serf.Member{
				newMember("master", "alive"),
				newMember("node", "alive"),
			},
			checkers: [2][]health.Checker{{healthyTest, failedTest}, {healthyTest, healthyTest}},
		},
		{
			comment: "Degraded due to a missing master node",
			status: pb.SystemStatus{
				Status: pb.SystemStatus_Degraded,
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
			members: [2]serf.Member{
				newMember("node-1", "alive"),
				newMember("node-2", "alive"),
			},
			checkers: [2][]health.Checker{{healthyTest, healthyTest}, {healthyTest, healthyTest}},
		},
		{
			comment: "Running with all systems running",
			status: pb.SystemStatus{
				Status: pb.SystemStatus_Running,
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
			members: [2]serf.Member{
				newMember("master", "alive"),
				newMember("node", "alive"),
			},
			checkers: [2][]health.Checker{{healthyTest, healthyTest}, {healthyTest, healthyTest}},
		},
		{
			comment: `Ignores members in "left" state`,
			status: pb.SystemStatus{
				Status: pb.SystemStatus_Running,
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
			members: [2]serf.Member{
				newMember("master", "alive"),
				newMember("node", "left"),
			},
			checkers: [2][]health.Checker{{healthyTest, healthyTest}, {healthyTest, healthyTest}},
		},
	}

	rpcPort := 7676
	for _, testCase := range agentTestCases {
		comment := Commentf(testCase.comment)
		clock := clockwork.NewFakeClock()
		localNode := testCase.members[0].Name
		remoteNode := testCase.members[1].Name
		localAgent := r.newLocalNode(localNode, rpcPort,
			testCase.members[:], testCase.checkers[0], clock, c)
		c.Assert(localAgent.Start(), IsNil)
		remoteAgent := r.newRemoteNode(remoteNode, rpcPort,
			testCase.members[:], testCase.checkers[1], clock, c)
		c.Assert(remoteAgent.Start(), IsNil)
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
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		resp, err := localAgent.rpc.Status(ctx, req)
		c.Assert(err, IsNil, comment)

		testCase.status.Timestamp = pb.NewTimeToProto(clock.Now())
		for _, node := range resp.Status.Nodes {
			sort.Sort(byChecker(node.Probes))
		}
		c.Assert(resp.Status, test.DeepCompare, &testCase.status, comment)
		rpcPort = rpcPort + 1
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

func (r *AgentSuite) TestReflectsSystemStatusInStatusCode(c *C) {
	// setup
	member := newMember("master", "alive")
	cluster := []serf.Member{member}
	checkers := []health.Checker{healthyTest, failedTest}
	clock := clockwork.NewFakeClock()
	expected := pb.SystemStatus{
		Status: pb.SystemStatus_Degraded,
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
	rpcPort := 7575

	// exercise
	agent := r.newRemoteNode(member.Name, rpcPort, cluster, checkers, clock, c)
	c.Assert(agent.Start(), IsNil)
	defer agent.Close()

	// wait until agent has started waiting to collect statuses
	clock.BlockUntil(1)
	clock.Advance(statusUpdateTimeout + time.Second)
	// ensure that the status update loop has finished updating status
	clock.BlockUntil(1)

	client, err := r.httpClient(fmt.Sprintf("https://127.0.0.1:%v", rpcPort))
	c.Assert(err, IsNil)

	resp, err := client.Get(client.Endpoint(""), url.Values{})
	c.Assert(err, IsNil)
	c.Assert(resp.Code(), Equals, http.StatusServiceUnavailable)

	// verify
	var status pb.SystemStatus
	err = json.Unmarshal(resp.Bytes(), &status)
	c.Assert(err, IsNil)

	expected.Timestamp = pb.NewTimeToProto(clock.Now())
	sort.Sort(byChecker(status.Nodes[0].Probes))
	c.Assert(status, test.DeepCompare, expected)
}

func (r *AgentSuite) TestReflectsNodeStatusInStatusCode(c *C) {
	// setup
	member := newMember("master", "alive")
	cluster := []serf.Member{member}
	checkers := []health.Checker{healthyTest, failedTest}
	clock := clockwork.NewFakeClock()
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
	rpcPort := 7575

	// exercise
	agent := r.newRemoteNode(member.Name, rpcPort, cluster, checkers, clock, c)
	c.Assert(agent.Start(), IsNil)
	defer agent.Close()

	// wait until agent has started waiting to collect statuses
	clock.BlockUntil(1)
	clock.Advance(statusUpdateTimeout + time.Second)
	// ensure that the status update loop has finished updating status
	clock.BlockUntil(1)

	client, err := r.httpClient(fmt.Sprintf("https://127.0.0.1:%v/local", rpcPort))
	c.Assert(err, IsNil)

	resp, err := client.Get(client.Endpoint(""), url.Values{})
	c.Assert(err, IsNil)
	c.Assert(resp.Code(), Equals, http.StatusServiceUnavailable)

	// verify
	var status pb.NodeStatus
	err = json.Unmarshal(resp.Bytes(), &status)
	c.Assert(err, IsNil)
	sort.Sort(byChecker(status.Probes))
	c.Assert(status, test.DeepCompare, expected)
}

func (r *AgentSuite) TestFailsIfTimesOutToCollectStatus(c *C) {
	clock := clockwork.NewFakeClock()
	members := [2]serf.Member{
		newMember("master", "alive"),
		newMember("node", "failed"),
	}
	const rpcPort = 7575
	localAgent := r.newLocalNode("master", rpcPort, members[:], []health.Checker{healthyTest}, clock, c)
	localAgent.statusQueryReplyTimeout = 1 * time.Second
	c.Assert(localAgent.Start(), IsNil)
	defer localAgent.Close()

	// wait until master agent has started waiting to collect statuses
	clock.BlockUntil(1)
	clock.Advance(statusUpdateTimeout + time.Second)
	// ensure master's status update loop has finished updating status
	clock.BlockUntil(1)

	req := &pb.StatusRequest{}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	resp, err := localAgent.rpc.Status(ctx, req)
	c.Assert(err, IsNil)

	for _, node := range resp.Status.Nodes {
		sort.Sort(byChecker(node.Probes))
	}
	expected := []*pb.NodeStatus{
		{
			Name:   "master",
			Status: pb.NodeStatus_Running,
			MemberStatus: &pb.MemberStatus{Name: "master", Addr: "<nil>:0",
				Tags: tags{"role": string(RoleMaster)}, Status: pb.MemberStatus_Alive},
			Probes: []*pb.Probe{healthyProbe},
		},
	}
	summary := fmt.Sprintf(msgNoStatus, "node,")
	// The agent might receive status about the failed node but in state "unknown"
	if len(resp.Status.Nodes) == 2 {
		expected = append(expected, &pb.NodeStatus{
			Name:   "node",
			Status: pb.NodeStatus_Unknown,
			MemberStatus: &pb.MemberStatus{Name: "master", Addr: "<nil>:0",
				Tags: tags{"role": string(RoleMaster)}, Status: pb.MemberStatus_Failed},
		})
		// Summary is empty with statuses from both nodes
		summary = ""
	}
	c.Assert(resp.Status, test.DeepCompare, &pb.SystemStatus{
		Status:    pb.SystemStatus_Degraded,
		Nodes:     expected,
		Timestamp: pb.NewTimeToProto(clock.Now()),
		Summary:   summary,
	})
}

// newLocalNode creates a new instance of the local agent - agent used to make status queries.
func (r *AgentSuite) newLocalNode(node string, rpcPort int, members []serf.Member,
	checkers []health.Checker, clock clockwork.Clock, c *C) *agent {
	agent := r.newAgent(node, rpcPort, members, checkers, clock, nil, c)
	agent.rpc = &testServer{&server{agent: agent}}
	return agent
}

// newRemoteNode creates a new instance of a remote agent - agent used to answer
// status query requests via a running RPC endpoint.
func (r *AgentSuite) newRemoteNode(node string, rpcPort int, members []serf.Member,
	checkers []health.Checker, clock clockwork.Clock, c *C) *agent {
	addr := fmt.Sprintf("127.0.0.1:%v", rpcPort)

	agent := r.newAgent(node, rpcPort, members, checkers, clock, nil, c)

	// Use the same CA certificate for client and server
	server, err := newRPCServer(agent, r.certFile, r.certFile, r.keyFile, []string{addr})
	c.Assert(err, IsNil)

	agent.rpc = server

	return agent
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
		dialRPC:      testDialRPC(rpcPort, r.certFile, r.keyFile),
		cache:        &testCache{c: c, SystemStatus: &pb.SystemStatus{Status: pb.SystemStatus_Unknown}},
		Checkers:     checkers,
		statusClock:  statusClock,
		recycleClock: recycleClock,
		localStatus:  emptyNodeStatus(node),
	}
}

// testDialRPC is a test implementation of the dialRPC interface,
// that creates an RPC client bound to localhost.
func testDialRPC(port int, certFile, keyFile string) dialRPC {
	return func(member *serf.Member) (*client, error) {
		return newClient(fmt.Sprintf(":%d", port), "agent", certFile, certFile, keyFile)
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

var healthyTest = &testChecker{
	name: "healthy service",
}

var healthyProbe = &pb.Probe{
	Checker: "healthy service",
	Status:  pb.Probe_Running,
}

var failedTest = &testChecker{
	name: "failing service",
	err:  errInvalidState,
}

var failedProbe = &pb.Probe{
	Checker: "failing service",
	Status:  pb.Probe_Failed,
	Error:   "invalid state",
}

// testSerfClient implements serfClient
type testSerfClient struct {
	members []serf.Member
}

func (r *testSerfClient) Members() ([]serf.Member, error) {
	return r.members, nil
}

func (r *testSerfClient) Stop(handle serf.StreamHandle) error {
	return nil
}

func (r *testSerfClient) UpdateTags(tags map[string]string, delTags []string) error {
	return nil
}

func (r *testSerfClient) Join(peers []string, replay bool) (int, error) {
	return 0, nil
}

func (r *testSerfClient) Close() error {
	return nil
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
	sync.Mutex
}

func (r *testCache) UpdateStatus(status *pb.SystemStatus) error {
	r.Lock()
	defer r.Unlock()
	r.SystemStatus = status
	return nil
}

func (r *testCache) RecentStatus() (*pb.SystemStatus, error) {
	r.Lock()
	defer r.Unlock()
	return r.SystemStatus, nil
}

func (r *testCache) Recycle() error {
	r.Lock()
	defer r.Unlock()
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

func (r byChecker) Len() int           { return len(r) }
func (r byChecker) Swap(i, j int)      { r[i], r[j] = r[j], r[i] }
func (r byChecker) Less(i, j int) bool { return r[i].Checker < r[j].Checker }

type byChecker []*pb.Probe

type tags map[string]string

func generateCert(certFile, keyFile string) error {
	priv, err := ecdsa.GenerateKey(elliptic.P224(), rand.Reader)
	if err != nil {
		return trace.Wrap(err)
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return trace.Wrap(err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Acme Co"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(1 * time.Hour),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:              []string{"agent"},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, publicKey(priv), priv)
	if err != nil {
		return trace.Wrap(err)
	}

	certOut, err := os.Create(certFile)
	if err != nil {
		return trace.Wrap(err)
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return trace.Wrap(err)
	}
	if err := certOut.Close(); err != nil {
		return trace.Wrap(err)
	}

	keyOut, err := os.OpenFile(keyFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return trace.Wrap(err)
	}
	if err := pem.Encode(keyOut, pemBlockForKey(priv)); err != nil {
		return trace.Wrap(err)
	}
	if err := keyOut.Close(); err != nil {
		return trace.Wrap(err)
	}
	return nil
}

// pemBlockForKey
// reference: https://golang.org/src/crypto/tls/generate_cert.go
func pemBlockForKey(priv interface{}) *pem.Block {
	switch k := priv.(type) {
	case *rsa.PrivateKey:
		return &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)}
	case *ecdsa.PrivateKey:
		b, err := x509.MarshalECPrivateKey(k)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Unable to marshal ECDSA private key: %v", err)
			os.Exit(2)
		}
		return &pem.Block{Type: "EC PRIVATE KEY", Bytes: b}
	default:
		return nil
	}
}

// publicKey
// reference: https://golang.org/src/crypto/tls/generate_cert.go
func publicKey(priv interface{}) interface{} {
	switch k := priv.(type) {
	case *rsa.PrivateKey:
		return &k.PublicKey
	case *ecdsa.PrivateKey:
		return &k.PublicKey
	default:
		return nil
	}
}

const sharedReadWriteMask = 0666
