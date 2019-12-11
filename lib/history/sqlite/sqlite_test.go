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

package sqlite

import (
	"context"
	"os"
	"testing"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/lib/history"

	"github.com/jonboulle/clockwork"
	_ "github.com/mattn/go-sqlite3" // initialize sqlite3
	. "gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner.
func TestSQLite(t *testing.T) { TestingT(t) }

type SQLiteSuite struct {
	clock    clockwork.FakeClock
	timeline history.Timeline
}

var _ = Suite(&SQLiteSuite{})

// TestDBPath specifies location of test database.
const TestDBPath = "/tmp/test.db"

// SetupTest initializes test database.
func (s *SQLiteSuite) SetUpTest(c *C) {
	clock := clockwork.NewFakeClock()
	config := Config{
		DBPath:   TestDBPath,
		Capacity: 1,
		Clock:    clock,
	}
	timeline, err := NewTimeline(context.TODO(), config)
	c.Assert(err, IsNil)

	s.clock = clock
	s.timeline = timeline
}

// TearDownTest closes database and removed file.
func (s *SQLiteSuite) TearDownTest(c *C) {
	os.Remove(TestDBPath)
}

func (s *SQLiteSuite) TestRecordStatus(c *C) {
	node := "test-node"
	old := &pb.NodeStatus{Name: node, Status: pb.NodeStatus_Running}
	new := &pb.NodeStatus{Name: node, Status: pb.NodeStatus_Degraded}

	var err error
	err = s.timeline.RecordStatus(context.TODO(), old)
	c.Assert(err, IsNil)
	err = s.timeline.RecordStatus(context.TODO(), new)
	c.Assert(err, IsNil)

	actual, err := s.timeline.GetEvents(context.TODO(), nil)
	c.Assert(err, IsNil)

	expected := []*pb.TimelineEvent{history.NewNodeDegraded(s.clock.Now(), node)}
	c.Assert(actual, DeepEquals, expected, Commentf("Test record status"))
}

func (s *SQLiteSuite) TestFIFOEviction(c *C) {
	node := "test-node"
	old := &pb.NodeStatus{Name: node, Status: pb.NodeStatus_Running}
	new := &pb.NodeStatus{Name: node, Status: pb.NodeStatus_Degraded}

	var err error
	err = s.timeline.RecordStatus(context.TODO(), old)
	c.Assert(err, IsNil)
	err = s.timeline.RecordStatus(context.TODO(), new)
	c.Assert(err, IsNil)
	err = s.timeline.RecordStatus(context.TODO(), old)
	c.Assert(err, IsNil)

	actual, err := s.timeline.GetEvents(context.TODO(), nil)
	c.Assert(err, IsNil)

	expected := []*pb.TimelineEvent{history.NewNodeRecovered(s.clock.Now(), node)}
	c.Assert(actual, DeepEquals, expected, Commentf("Test FIFO eviction"))
}

func (s *SQLiteSuite) TestFilterEvents(c *C) {
	node := "test-node"
	old := &pb.NodeStatus{Name: node, Status: pb.NodeStatus_Running}
	new := &pb.NodeStatus{Name: node, Status: pb.NodeStatus_Degraded}

	var err error
	err = s.timeline.RecordStatus(context.TODO(), old)
	c.Assert(err, IsNil)
	err = s.timeline.RecordStatus(context.TODO(), new)
	c.Assert(err, IsNil)

	params := map[string]string{"type": nodeDegradedType, "node": node}
	actual, err := s.timeline.GetEvents(context.TODO(), params)
	c.Assert(err, IsNil)
	expected := []*pb.TimelineEvent{history.NewNodeDegraded(s.clock.Now(), node)}
	c.Assert(actual, DeepEquals, expected, Commentf("Test filter events - one match"))

	params = map[string]string{"type": nodeRecoveredType, "node": node}
	actual, err = s.timeline.GetEvents(context.TODO(), params)
	c.Assert(err, IsNil)
	expected = nil
	c.Assert(actual, DeepEquals, expected, Commentf("Test filter events - no match"))
}
