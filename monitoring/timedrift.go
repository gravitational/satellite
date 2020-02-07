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
	"github.com/gravitational/satellite/lib/rpc/client"

	"github.com/gravitational/trace"
	"github.com/gravitational/ttlmap"
	serf "github.com/hashicorp/serf/client"
	"github.com/jonboulle/clockwork"
	log "github.com/sirupsen/logrus"
)

const (
	// timeDriftCheckerID is the time drift check name.
	timeDriftCheckerID = "time-drift"
	// timeDriftThreshold sets the default threshold of the acceptable time
	// difference between nodes.
	timeDriftThreshold = 300 * time.Millisecond
	// clientsCacheCapacity is the capacity of the TTL map that holds
	// clients to satellite agents on other cluster nodes.
	clientsCacheCapacity = 1000
)

// timeDriftChecker is a checker that verifies that the time difference between
// cluster nodes remains within the specified threshold.
type timeDriftChecker struct {
	// TimeDriftCheckerConfig contains checker configuration.
	TimeDriftCheckerConfig
	// FieldLogger is used for logging.
	log.FieldLogger
	// mu protects the clients map.
	mu sync.Mutex
	// clients contains RPC clients for other cluster nodes.
	clients *ttlmap.TTLMap
}

// TimeDriftCheckerConfig stores configuration for the time drift check.
type TimeDriftCheckerConfig struct {
	// CAFile is the path to the certificate authority file for Satellite agent.
	CAFile string
	// CertFile is the path to the Satellite agent client certificate file.
	CertFile string
	// KeyFile is the path to the Satellite agent private key file.
	KeyFile string
	// SerfClient is the client to the local serf agent.
	SerfClient agent.SerfClient
	// SerfMember is the local serf member.
	SerfMember *serf.Member
	// DialRPC is used to create Satellite RPC client.
	DialRPC client.DialRPC
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
	if c.SerfClient == nil {
		return trace.BadParameter("local serf client can't be empty")
	}
	if c.SerfMember == nil {
		return trace.BadParameter("local serf member can't be empty")
	}
	if c.DialRPC == nil {
		c.DialRPC = client.DefaultDialRPC(c.CAFile, c.CertFile, c.KeyFile)
	}
	if c.Clock == nil {
		c.Clock = clockwork.NewRealClock()
	}
	return nil
}

// NewTimeDriftChecker returns a new instance of time drift checker.
func NewTimeDriftChecker(conf TimeDriftCheckerConfig) (c health.Checker, err error) {
	if err := conf.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	clientsCache, err := ttlmap.New(clientsCacheCapacity)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &timeDriftChecker{
		TimeDriftCheckerConfig: conf,
		FieldLogger:            log.WithField(trace.Component, timeDriftCheckerID),
		clients:                clientsCache,
	}, nil
}

// Name returns the checker name.
func (c *timeDriftChecker) Name() string {
	return timeDriftCheckerID
}

// Check fills in provided reporter with probes according to time drift check results.
func (c *timeDriftChecker) Check(ctx context.Context, r health.Reporter) {
	failedProbes, err := c.check(ctx, r)
	if err != nil {
		c.Error(err.Error())
		r.Add(NewProbeFromErr(c.Name(), "failed to check time drift", err))
		return
	}
	if len(failedProbes) == 0 {
		r.Add(c.successProbe())
		return
	}
	for i := range failedProbes {
		r.Add(failedProbes[i])
	}
}

// check does a time drift check between this and other cluster nodes.
//
// Returns a list of probes that failed.
func (c *timeDriftChecker) check(ctx context.Context, r health.Reporter) (probes []*pb.Probe, err error) {
	nodes, err := c.nodesToCheck()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	for _, node := range nodes {
		drift, err := c.getTimeDrift(ctx, node)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		if isDriftHigh(drift) {
			probes = append(probes, c.failureProbe(node, drift))
		}
	}
	return probes, nil
}

// getTimeDrift calculates the time drift value between this and the specified
// node using the following algorithm.
//
// Every coordinator node (Kubernetes masters) executes an instance
// of this algorithm.

// For each of the remaining cluster nodes (including other coordinator nodes):

// * Selected coordinator node records its local timestamp (in UTC). Let’s call
//   this timestamp T1Start.

// * Coordinator initiates a "ping" grpc request to the node.

// * The node responds to the ping request replying with node's local timestamp
//   (in UTC) in the payload. Let's call this timestamp T2.

// * After receiving the remote response, coordinator records the second local
//   timestamp. Let's call it T1End.

// * Coordinator calculates the latency between itself and the node:
//   (T1End-T1Start)/2. Let's call this value Latency.

// * Coordinator calculates the time drift between itself and the node:
//   T2-T1Start-Latency. Let's call this value Drift. Can be negative which would
//   mean the node time is falling behind.

// * Compare abs(Drift) with the threshold.
func (c *timeDriftChecker) getTimeDrift(ctx context.Context, node serf.Member) (time.Duration, error) {
	agentClient, err := c.getAgentClient(ctx, node)
	if err != nil {
		return 0, trace.Wrap(err)
	}

	// Obtain this node's local timestamp.
	t1Start := c.Clock.Now().UTC()

	// Send "time" request to the specified node.
	t2Response, err := agentClient.Time(ctx, &pb.TimeRequest{})
	if err != nil {
		// If the agent we're making request to is of an older version,
		// it may not support Time() method yet. This can happen, e.g.,
		// during a rolling upgrade. In this case fallback to success.
		if trace.IsNotImplemented(err) {
			c.WithField("node", node.Name).Warnf(trace.UserMessage(err))
			return 0, nil
		}
		return 0, trace.Wrap(err)
	}

	// Calculate how much time has elapsed since T1Start. This value will
	// roughly be the request roundtrip time, so the latency b/w the nodes
	// is half that.
	latency := c.Clock.Now().UTC().Sub(t1Start) / 2

	// Finally calculate the time drift between this and the specified node
	// using formula: T2 - T1Start - Latency.
	t2 := t2Response.GetTimestamp().ToTime()
	drift := t2.Sub(t1Start) - latency

	c.WithField("node", node.Name).Debugf("T1Start: %v; T2: %v; Latency: %v; Drift: %v.",
		t1Start, t2, latency, drift)
	return drift, nil
}

// successProbe constructs a probe that represents successful time drift check.
func (c *timeDriftChecker) successProbe() *pb.Probe {
	return &pb.Probe{
		Checker: c.Name(),
		Detail: fmt.Sprintf("time drift between %s and other nodes is within the allowed threshold of %s",
			c.SerfMember.Addr, timeDriftThreshold),
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
			c.SerfMember.Addr, node.Addr, timeDriftThreshold, drift),
		Status:   pb.Probe_Failed,
		Severity: pb.Probe_Warning,
	}
}

// nodesToCheck returns nodes to check time drift against.
func (c *timeDriftChecker) nodesToCheck() (result []serf.Member, err error) {
	nodes, err := c.SerfClient.Members()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	for _, node := range nodes {
		if c.shouldCheckNode(node) {
			result = append(result, node)
		}
	}
	return result, nil
}

// shouldCheckNode returns true if the check should be run against specified
// serf member.
func (c *timeDriftChecker) shouldCheckNode(node serf.Member) bool {
	return strings.ToLower(node.Status) == strings.ToLower(pb.MemberStatus_Alive.String()) &&
		c.SerfMember.Addr.String() != node.Addr.String()
}

// getAgentClient returns Satellite agent client for the provided node.
func (c *timeDriftChecker) getAgentClient(ctx context.Context, node serf.Member) (client.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	clientI, exists := c.clients.Get(node.Addr.String())
	if exists {
		return clientI.(client.Client), nil
	}
	client, err := c.DialRPC(ctx, &node)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	c.clients.Set(node.Addr.String(), client, time.Hour)
	return client, nil
}

// isDriftHigh returns true if the provided drift value is over the threshold.
func isDriftHigh(drift time.Duration) bool {
	return drift < 0 && -drift > timeDriftThreshold || drift > timeDriftThreshold
}
