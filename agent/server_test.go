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
	"io/ioutil"
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

	"github.com/gravitational/roundtrip"
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
					Checker: "qux",
					Status:  pb.Probe_Failed,
					Error:   "not available",
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

func (r *AgentSuite) TestReflectsStatusInStatusCode(c *C) {
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

	client, err := httpClient(fmt.Sprintf("https://127.0.0.1:%v", rpcPort))
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
	// The agent might receive status about the failed node but in state "unknown"
	if len(resp.Status.Nodes) == 2 {
		expected = append(expected, &pb.NodeStatus{
			Name:   "node",
			Status: pb.NodeStatus_Unknown,
			MemberStatus: &pb.MemberStatus{Name: "master", Addr: "<nil>:0",
				Tags: tags{"role": string(RoleMaster)}, Status: pb.MemberStatus_Failed},
		})
	}
	c.Assert(resp.Status, test.DeepCompare, &pb.SystemStatus{
		Status:    pb.SystemStatus_Degraded,
		Nodes:     expected,
		Timestamp: pb.NewTimeToProto(clock.Now()),
		Summary:   fmt.Sprintf(msgNoStatus, "node,"),
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
		dialRPC:      testDialRPC(rpcPort, r.certFile),
		cache:        &testCache{c: c, SystemStatus: &pb.SystemStatus{Status: pb.SystemStatus_Unknown}},
		Checkers:     checkers,
		statusClock:  statusClock,
		recycleClock: recycleClock,
		localStatus:  emptyNodeStatus(node),
	}
}

func httpClient(url string) (*roundtrip.Client, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
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

// openssl req -new -out server.csr -key server.key -config san.cnf
// openssl x509 -req -days 3650 -in server.csr -signkey server.key -out server.crt -extensions req_ext -extfile san.cnf
// san.cnf:
// [req]
// req_extensions=req_ext
// prompt=no
// distinguished_name=dn
// [dn]
// C=US
// ST=Test
// L=Test
// O=Acme LLC
// CN=agent
// [req_ext]
// subjectAltName=@alt_names
// [alt_names]
// DNS.1=agent
// IP.1=127.0.0.1
var certFile = []byte(`-----BEGIN CERTIFICATE-----
MIIDOTCCAiGgAwIBAgIJAJkeMEm1V6iYMA0GCSqGSIb3DQEBCwUAME4xCzAJBgNV
BAYTAlVTMQ0wCwYDVQQIDARUZXN0MQ0wCwYDVQQHDARUZXN0MREwDwYDVQQKDAhB
Y21lIExMQzEOMAwGA1UEAwwFYWdlbnQwHhcNMTcxMTE0MjE1OTI5WhcNMjcxMTEy
MjE1OTI5WjBOMQswCQYDVQQGEwJVUzENMAsGA1UECAwEVGVzdDENMAsGA1UEBwwE
VGVzdDERMA8GA1UECgwIQWNtZSBMTEMxDjAMBgNVBAMMBWFnZW50MIIBIjANBgkq
hkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAxyBjYyMJ6DQXerhK7VhUna6iVXr+dXBe
delPqox3ZnfGiuOcm70xPwJW0yeDndvHNc21i12cI86pv3g8YNnH6IKqYY7uyD3C
Cpa0AE2Tt3C0Iowc6HPQQzUeAfAomh4M1LZzSaPKbtVvAcApOGG9ZJxw4NS1tVeq
pBC+k4ZYzdzy5b4bqugw/6uxM7kND/7jzDB3zLyxqPpwd58mHI+RTWkNSeIuJmi9
09njqMO6q8lypEDXdikX0PL5Mph9Dr2i3uV3lGJWk/QHov/AuN2DsubdeYH3udIu
pKDmy89s7uX8aqdgeF4c21vwsZ1htHcWBAKXaHuDptgPFAq/C+gbTQIDAQABoxow
GDAWBgNVHREEDzANggVhZ2VudIcEfwAAATANBgkqhkiG9w0BAQsFAAOCAQEAgHDT
Rgq8K/i56L9tc5a+uRqnv9b1xyg9z2Brk7L6TFGtZBoExe1I9FVlHY9EbnE11xp0
FPlpoDjk+giwOclwnFFIZGmjKFiyWslrvxNT3sSAFwbL42y//u9ddszxez4S09kt
g9huQ1wgMGe1E2EsO2XSCSG+NAHrW/FFX09DB/l25b0GqpOSXG//ztiVXYdVYIjl
r6tb7YzYC2U5ssdeNow//EzpuDWv/TPG35UWEMuK+8ruisSASDZC/0exUBOkGn8W
PC6DCBH+wA8cbO7V6CguMZN0rNapnXOjyYyjPCazKxCftHNMVg90BGQjwyXTbnIj
rzz+3PuKaaMFNJhMJA==
-----END CERTIFICATE-----`)

// openssl genrsa -out server.key 2048
var keyFile = []byte(`-----BEGIN RSA PRIVATE KEY-----
MIIEpgIBAAKCAQEAxyBjYyMJ6DQXerhK7VhUna6iVXr+dXBedelPqox3ZnfGiuOc
m70xPwJW0yeDndvHNc21i12cI86pv3g8YNnH6IKqYY7uyD3CCpa0AE2Tt3C0Iowc
6HPQQzUeAfAomh4M1LZzSaPKbtVvAcApOGG9ZJxw4NS1tVeqpBC+k4ZYzdzy5b4b
qugw/6uxM7kND/7jzDB3zLyxqPpwd58mHI+RTWkNSeIuJmi909njqMO6q8lypEDX
dikX0PL5Mph9Dr2i3uV3lGJWk/QHov/AuN2DsubdeYH3udIupKDmy89s7uX8aqdg
eF4c21vwsZ1htHcWBAKXaHuDptgPFAq/C+gbTQIDAQABAoIBAQCLr8bIxs2uXMyT
xDCbqzlAnD84o91ZWQiKwq6mP3+LHD7lM6KrBd9ECkoKOk/0LzbiIXpXV8WuwM0H
ijsg3eWE0BTh9zi+s8QpVWrUQ5d6Oc/D5HJrBsN0QhDY3zY8VxQ9K/hYElRxx7vl
iH3iFX6c07nDnrQRkHweN7jZGIe3cSfd7dS4rLBpV5EE7px7S5zT/DD3Tkmj1gXE
LR60KrZkNuv1WtQ9LQMZmB5ik7AQVEdqOZ5qCwjFbGwMbVJpRAOeYzhxIYiPzRkP
rH0VRqVPld0QRNArd+81c3VWwXGR3QpIEdYQtaCH5ZItQju9tyBvpGngrzYj+PV1
IaV0R76pAoGBAPZ7AM5LDG9f6f5fANqexcApn9CjswpE6qSDFLg14nmze4+kxk6y
tStl18SYYLQrk6YYy8ViI66AUWCLTHC9b88Ok04uqRWbT9TpKc4QSzb/ZHAgRjF0
DK8ysbUwQPSKNtSybIH1DWvg2Qc7GUkZknDAd1LU4zW2NuhXV6pG3jB3AoGBAM7R
MBtvPdTbZNmBipWSIbfM4QKj5CEDr7M4LigWwbQC8JMgTIrpu5EOeHuM+RbZdT8S
c3PFf8b46WXSHDM3BNrIedwKKFgpYKvd8YpdRDSiKb3jfyY2wjKVMIR2tjzcNhkc
+7kikngVasoWDvoI9rg5tJ8IuZlDH9AA9ieGO2dbAoGBANB8zPqyWote2yPSInvK
L0VTMB6gSVKXZs7PHdiPo8kDu7GOVDu/SCW0WKWvqqTb82Fcugh08e+qFKuQSJFY
e9nt30YTi+x92jIjI7xs5eJYdxGtCxLLsesD+3NipJ70xlp1rfjjWn30zD8ki0fc
/JSpCIWlE6ecQKeZMcsTdOATAoGBAM4YL7RnGlqvdsQ5Dv0V7nvWsrOK1p7/qWsT
JQvWAZl9BHfYy+3yFXPr06xrQx29/dSoclyAB2EkUpGg23E99px/AtB/XszcDvW1
6ilT39ADeU09E0vlbYgym3KlSd1EJLTJ6R8IkKUR0qUnbi1EGXhkKNYCP9G2zlDd
ZG7mmPPZAoGBALYsBoLEvRsUnfZwg+6EC8L8BYbch/UgaXjqg0nmFHsWbKJNMPzr
EGailjuSn0ofbz2gcLDD5Na1/3hnI7qOP7th+rHXBWh3/4ypdbCOJeGepS82MPGW
3bggrvOh3D8sfevWppMw6Hzt0FsevUC2TGp0okbId/BGcMs9WYIV+pq2
-----END RSA PRIVATE KEY-----`)

const sharedReadWriteMask = 0666
