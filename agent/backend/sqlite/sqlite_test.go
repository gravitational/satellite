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

package sqlite

import (
	"os"
	"testing"
	"time"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"

	log "github.com/Sirupsen/logrus"
	"github.com/jonboulle/clockwork"
	_ "github.com/mattn/go-sqlite3"
	. "gopkg.in/check.v1"
)

func init() {
	if testing.Verbose() {
		log.SetOutput(os.Stderr)
		log.SetLevel(log.InfoLevel)
	}
}

var nodes = [2]string{"node-1", "node-2"}

func TestBackend(t *testing.T) { TestingT(t) }

type BackendSuite struct {
	backend *backend
}

type BackendWithClockSuite struct {
	backend *backend
	clock   clockwork.FakeClock
}

var _ = Suite(&BackendSuite{})
var _ = Suite(&BackendWithClockSuite{})

func (r *BackendSuite) SetUpTest(c *C) {
	var err error
	r.backend, err = newTestBackend()
	c.Assert(err, IsNil)
}

func (r *BackendSuite) TearDownTest(c *C) {
	if r.backend != nil {
		r.backend.Close()
	}
}

func (r *BackendWithClockSuite) SetUpTest(c *C) {
	var err error
	r.clock = clockwork.NewFakeClock()
	r.backend, err = newTestBackendWithClock(r.clock)
	c.Assert(err, IsNil)
}

func (r *BackendWithClockSuite) TearDownTest(c *C) {
	r.backend.Close()
}

func (r *BackendSuite) TestUpdatesStatus(c *C) {
	ts := time.Now()
	status := newStatus(nodes, ts)
	c.Assert(r.backend.UpdateStatus(status), IsNil)

	var count int64
	when := timestamp(pb.TimeToProto(ts))
	err := r.backend.QueryRow(`SELECT count(*) FROM system_snapshot WHERE captured_at = ?`, &when).Scan(&count)
	c.Assert(err, IsNil)

	c.Assert(count, Equals, int64(1))
}

func (r *BackendSuite) TestExplicitlyRecycles(c *C) {
	ts := time.Now().Add(-valueTTL)
	status := newStatus(nodes, ts)
	err := r.backend.UpdateStatus(status)
	c.Assert(err, IsNil)

	err = r.backend.deleteOlderThan(ts.Add(time.Second))
	c.Assert(err, IsNil)

	var count int64
	when := timestamp(pb.TimeToProto(ts))
	err = r.backend.QueryRow(`SELECT count(*) FROM system_snapshot WHERE captured_at = ?`, when).Scan(&count)
	c.Assert(err, IsNil)

	c.Assert(count, Equals, int64(0))
}

func (r *BackendSuite) TestObtainsRecentStatus(c *C) {
	clock := clockwork.NewFakeClock()

	tags := [2]map[string]string{
		{"publicip": "178.168.192.1", "role": "master"},
		{"publicip": "178.168.192.2", "role": "node"},
	}
	time := clock.Now()
	status := newStatusWithTags(nodes, time, tags)
	c.Assert(r.backend.UpdateStatus(status), IsNil)

	actualStatus, err := r.backend.RecentStatus()
	c.Assert(err, IsNil)

	c.Assert(actualStatus, DeepEquals, status)
}

func (r *BackendWithClockSuite) TestUpdatesNodeEvenWithNoProbes(c *C) {
	clock := clockwork.NewFakeClock()

	time := clock.Now()
	status := newStatus(nodes, time)
	status.Nodes[0].Probes = nil // reset probes on the node
	c.Assert(r.backend.UpdateStatus(status), IsNil)

	actualStatus, err := r.backend.RecentStatus()
	c.Assert(err, IsNil)

	c.Assert(actualStatus, DeepEquals, status)
}

func (r *BackendWithClockSuite) TestUpdatesMemberAddrForNode(c *C) {
	clock := clockwork.NewFakeClock()

	ts := clock.Now()
	status := newStatus(nodes, ts)
	status.Nodes[0].MemberStatus.Addr = "" // reset member address
	c.Assert(r.backend.UpdateStatus(status), IsNil)

	status.Timestamp = pb.NewTimeToProto(ts.Add(time.Second)) // skew time to force new snapshot
	status.Nodes[0].MemberStatus.Addr = "198.168.172.1"
	c.Assert(r.backend.UpdateStatus(status), IsNil)

	actualStatus, err := r.backend.RecentStatus()
	c.Assert(err, IsNil)

	c.Assert(actualStatus, DeepEquals, status)
}

func updateStatus(b *backend, nodes [2]string, clock clockwork.Clock) error {
	baseTime := clock.Now()
	for i := 0; i < 3; i++ {
		status := newStatus(nodes, baseTime)
		if err := b.UpdateStatus(status); err != nil {
			return err
		}
		baseTime = baseTime.Add(-10 * time.Second)
	}
	return nil
}

func newTestBackend() (*backend, error) {
	return newTestBackendWithClock(clockwork.NewRealClock())
}

func newTestBackendWithClock(clock clockwork.Clock) (*backend, error) {
	backend, err := newInMemory(clock)
	if err != nil {
		return nil, err
	}
	return backend, nil
}

func newStatus(names [2]string, time time.Time) *pb.SystemStatus {
	defaultTags := map[string]string{"key": "value", "key2": "value2"}

	return newStatusWithTags(names, time, [2]map[string]string{defaultTags, defaultTags})
}

func newStatusWithTags(names [2]string, time time.Time, tags [2]map[string]string) *pb.SystemStatus {
	when := pb.NewTimeToProto(time)
	var nodes []*pb.NodeStatus
	for i, name := range names {
		nodes = append(nodes, &pb.NodeStatus{
			Name:   name,
			Status: pb.NodeStatus_Degraded,
			MemberStatus: &pb.MemberStatus{
				Name:   name,
				Status: pb.MemberStatus_Alive,
				Tags:   tags[i],
			},
			Probes: []*pb.Probe{
				&pb.Probe{
					Checker: "checker a",
					Status:  pb.Probe_Failed,
					Error:   "sync error",
				},
				&pb.Probe{
					Checker: "checker b",
					Status:  pb.Probe_Failed,
					Error:   "invalid state",
				},
			},
		})
	}
	status := &pb.SystemStatus{
		Status:    pb.SystemStatus_Degraded,
		Timestamp: when,
		Summary:   "system status summary",
		Nodes:     nodes,
	}
	return status
}
