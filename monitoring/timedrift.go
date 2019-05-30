/*
Copyright 2019 Gravitational, Inc.

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

/* Time drift check algorigthm.

Every coordinator node (one of Kubernetes masters) performs an instance
of this algorithm.

For each of remaining cluster nodes (including other coordinator nodes):

* Selected coordinator node records it’s local timestamp (in UTC). Let’s call
  this timestamp T1Start.

* Coordinator initiates a “ping” grpc request to the node. Can be with empty
  payload.

* The node responds to the ping request replying with node’s local timestamp
  (in UTC) in the payload. Let’s call this timestamp T2.

* As the coordinator received the response, coordinator gets second local
  timestamp. Let’s call it T1End.

* Coordinator calculates the latency between itself and the node:
  (T1End-T1Start)/2. Let’s call this value Latency.

* Coordinator calculates the time drift between itself and the node:
  T2-T1Start-Latency. Let’s call this value Drift. Can be negative which would
  mean the node time is falling behind.

* Compare abs(Drift) with the threshold.

*/

package monitoring

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gravitational/satellite/agent"
	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"

	"github.com/gravitational/trace"
	"github.com/gravitational/ttlmap"
	serf "github.com/hashicorp/serf/client"
	"github.com/jonboulle/clockwork"
	log "github.com/sirupsen/logrus"
)

const (
	// timeDriftCheckerID is the time drift check name.
	timeDriftCheckerID = "time-drift"
	// timeDriftThreshold set the default threshold of the acceptable time
	// difference between nodes.
	timeDriftThreshold = 300 * time.Millisecond
)

// timeDriftChecker is a checker that verifies that the time difference between
// cluster nodes remains withing the specified threshold.
type timeDriftChecker struct {
	TimeDriftCheckerConfig
	log.FieldLogger
	self        serf.Member
	clients     *ttlmap.TTLMap
	serfClient  agent.SerfClient
	serfRPCAddr string
	name        string
	mux         sync.Mutex
}

// TimeDriftCheckerConfig stores configuration for the time drift check.
type TimeDriftCheckerConfig struct {
	// CAFile is the path to the certificate authority file for Satellite agent.
	CAFile string
	// CertFile is the path to the Satellite agent client certificate file.
	CertFile string
	// KeyFile is the path to the Satellite agent private key file.
	KeyFile string
	// SerfRPCAddr is the address used by the Serf RPC client to communicate
	SerfRPCAddr string
	// Name is the name associated to this node in Serf
	Name string
	// NewSerfClient is used to create Serf client.
	NewSerfClient agent.NewSerfClientFunc
	// DialRPC is used to create Satellite RPC client.
	DialRPC agent.DialRPC
	// Clock is used in tests to mock time.
	Clock clockwork.Clock
}

// CheckAndSetDefaults validates the config and sets default values.
func (c *TimeDriftCheckerConfig) CheckAndSetDefaults() error {
	if c.CAFile == "" {
		return trace.BadParameter("agent CA certificate file can't be empty")
	}
	if c.CertFile == "" {
		return trace.BadParameter("agent certificate file can't be empty")
	}
	if c.KeyFile == "" {
		return trace.BadParameter("agent certificate key file can't be empty")
	}
	if c.SerfRPCAddr == "" {
		return trace.BadParameter("serf RPC address can't be empty")
	}
	if c.Name == "" {
		return trace.BadParameter("node name can't be empty")
	}
	if c.NewSerfClient == nil {
		c.NewSerfClient = agent.NewSerfClient
	}
	if c.DialRPC == nil {
		c.DialRPC = agent.DefaultDialRPC(c.CAFile, c.CertFile, c.KeyFile)
	}
	if c.Clock == nil {
		c.Clock = clockwork.NewRealClock()
	}
	return nil
}

// NewTimeDriftChecker returns a new instance of time drift checker.
func NewTimeDriftChecker(conf TimeDriftCheckerConfig) (c health.Checker, err error) {
	err = conf.CheckAndSetDefaults()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	client, err := conf.NewSerfClient(serf.Config{
		Addr: conf.SerfRPCAddr,
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	self, err := client.FindMember(conf.Name)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	clientsCache, err := ttlmap.New(1000)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &timeDriftChecker{
		TimeDriftCheckerConfig: conf,
		FieldLogger:            log.WithField(trace.Component, timeDriftCheckerID),
		clients:                clientsCache,
		self:                   *self,
		serfClient:             client,
		serfRPCAddr:            conf.SerfRPCAddr,
		name:                   conf.Name,
	}, nil
}

// Name returns the checker name.
func (c *timeDriftChecker) Name() string {
	return timeDriftCheckerID
}

// Check fills in provided reporter with probes according to time drift check results.
func (c *timeDriftChecker) Check(ctx context.Context, r health.Reporter) {
	probes, err := c.check(ctx, r)
	if err != nil {
		c.Error(err.Error())
		r.Add(NewProbeFromErr(c.Name(), "", err))
		return
	}
	for i := range probes {
		r.Add(&probes[i])
	}
}

// check does a time drift check between this and other cluster nodes.
//
// Returns a list of probe results, one for each node this node has been
// checked against.
func (c *timeDriftChecker) check(ctx context.Context, r health.Reporter) (probes []pb.Probe, err error) {
	nodes, err := c.serfClient.Members()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	for _, node := range nodes {
		if c.shouldSkipNode(node) {
			continue
		}
		probe, err := c.checkNode(ctx, node)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		probes = append(probes, *probe)
	}
	return probes, nil
}

// checkNode checks time drift between this node and the specified node and
// returns the probe result.
func (c *timeDriftChecker) checkNode(ctx context.Context, node serf.Member) (*pb.Probe, error) {
	drift, err := c.getTimeDrift(ctx, node)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	// Drift may be negative which means the node is lagging behind.
	if drift < 0 && -drift > timeDriftThreshold || drift > timeDriftThreshold {
		return c.failureProbe(node, drift), nil
	}
	return c.successProbe(node, drift), nil
}

// getTimeDrift calculates the time drift value between this and the specified
// node.
//
// It implements the algorithm described in more detail in the header of this
// module.
func (c *timeDriftChecker) getTimeDrift(ctx context.Context, node serf.Member) (time.Duration, error) {
	agentClient, err := c.getAgentClient(node)
	if err != nil {
		return 0, trace.Wrap(err)
	}

	// Obtain this node's local timestamp.
	//
	// Let’s call this timestamp T1Start.
	//
	t1Start := c.Clock.Now().UTC()

	// Send "time" request to the specified node.
	//
	// Let's call the timestamp remote node returns T2.
	//
	t2Response, err := agentClient.Time(ctx, &pb.TimeRequest{})
	if err != nil {
		return 0, trace.Wrap(err)
	}

	// Calculate how much time has elapsed since T1Start. This value will
	// roughly be the request roundtrip time, so the latency b/w the nodes
	// is half that.
	//
	// Let's call this value Latency.
	//
	latency := c.Clock.Now().UTC().Sub(t1Start) / 2

	// Finally calculate the time drift between this and the specified node
	// using formula: T2 - T1Start - Latency.
	//
	t2 := t2Response.GetTimestamp().ToTime()
	drift := t2.Sub(t1Start) - latency

	c.WithField("node", node.Name).Debugf("T1Start: %v; T2: %v; Latency: %v; Drift: %v.",
		t1Start, t2, latency, drift)
	return drift, nil
}

// successProbe constructs a probe that represents successful time drift check
// against the specified node.
func (c *timeDriftChecker) successProbe(node serf.Member, drift time.Duration) *pb.Probe {
	return &pb.Probe{
		Checker: c.Name(),
		Detail: fmt.Sprintf("time drift between %s and %s is within the allowed threshold of %s: %s",
			c.self.Addr, node.Addr, timeDriftThreshold, drift),
		Status:   pb.Probe_Running,
		Severity: pb.Probe_None,
	}
}

// failureProbe constructs a probe that represents failed time drift check
// against the specified node.
func (c *timeDriftChecker) failureProbe(node serf.Member, drift time.Duration) *pb.Probe {
	return &pb.Probe{
		Checker: c.Name(),
		Detail: fmt.Sprintf("time drift between %s and %s is higher than the allowed threshold of %s: %s",
			c.self.Addr, node.Addr, timeDriftThreshold, drift),
		Status:   pb.Probe_Failed,
		Severity: pb.Probe_Warning,
	}
}

// shouldSkipNode returns true if the check should not be run against specified
// serf member.
func (c *timeDriftChecker) shouldSkipNode(node serf.Member) bool {
	return strings.ToLower(node.Status) != strings.ToLower(pb.MemberStatus_Alive.String()) ||
		c.self.Addr.String() == node.Addr.String()
}

// getAgentClient returns Satellite agent client for the provided node.
func (c *timeDriftChecker) getAgentClient(node serf.Member) (agent.Client, error) {
	c.mux.Lock()
	defer c.mux.Unlock()
	clientI, exists := c.clients.Get(node.Addr.String())
	if exists {
		return clientI.(agent.Client), nil
	}
	client, err := c.DialRPC(&node)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	c.clients.Set(node.Addr.String(), client, time.Hour)
	return client, nil
}
