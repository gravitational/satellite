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
	"time"

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
	timeline *Timeline
}

var _ = Suite(&SQLiteSuite{})

// TestDBPath specifies location of test database.
const TestDBPath = "/tmp/test.db"

// SetupTest initializes test database.
func (s *SQLiteSuite) SetUpTest(c *C) {
	// timelineInitTimeout specifies the amount of time given to initialize database.
	const timelineInitTimeout = 5 * time.Second

	clock := clockwork.NewFakeClock()
	config := Config{
		DBPath:            TestDBPath,
		RetentionDuration: time.Hour,
		Clock:             clock,
	}

	ctx, cancel := context.WithTimeout(context.TODO(), timelineInitTimeout)
	defer cancel()

	timeline, err := NewTimeline(ctx, config)
	c.Assert(err, IsNil)

	s.clock = clock
	s.timeline = timeline
}

// TearDownTest closes database and removed file.
func (s *SQLiteSuite) TearDownTest(c *C) {
	os.Remove(TestDBPath)
}

// TestRecordStatus simply tests that a status can be recorded.
func (s *SQLiteSuite) TestRecordStatus(c *C) {
	withTimeout(func(ctx context.Context) {
		node := "test-node"
		status := &pb.NodeStatus{Name: node, Status: pb.NodeStatus_Running}

		err := s.timeline.RecordStatus(ctx, status)
		c.Assert(err, IsNil)

		actual, err := s.timeline.GetEvents(ctx, nil)
		c.Assert(err, IsNil)

		expected := []*pb.TimelineEvent{history.NewNodeRecovered(s.clock.Now(), node)}

		c.Assert(actual, DeepEquals, expected, Commentf("Expected the status to be recorded."))
	})
}

// TestEviction tests timeline correctly implements an eviction policy.
func (s *SQLiteSuite) TestEviction(c *C) {
	withTimeout(func(ctx context.Context) {
		node := "test-node"
		status := &pb.NodeStatus{Name: node, Status: pb.NodeStatus_Running}

		err := s.timeline.RecordStatus(ctx, status)
		c.Assert(err, IsNil)

		// Advance clock and evict all events before this time.
		s.clock.Advance(time.Second)
		err = s.timeline.evictEvents(ctx, s.clock.Now())
		c.Assert(err, IsNil)

		actual, err := s.timeline.GetEvents(ctx, nil)
		c.Assert(err, IsNil)

		var expected []*pb.TimelineEvent
		c.Assert(actual, DeepEquals, expected, Commentf("Expected all events to be evicted."))
	})
}

// TestFilterEvents tests that events can be filtered.
func (s *SQLiteSuite) TestFilterEvents(c *C) {
	withTimeout(func(ctx context.Context) {
		node := "test-node"
		status := &pb.NodeStatus{Name: node, Status: pb.NodeStatus_Running}

		err := s.timeline.RecordStatus(ctx, status)
		c.Assert(err, IsNil)

		params := map[string]string{"type": nodeRecoveredType, "node": node}
		actual, err := s.timeline.GetEvents(ctx, params)
		c.Assert(err, IsNil)
		expected := []*pb.TimelineEvent{history.NewNodeRecovered(s.clock.Now(), node)}
		c.Assert(actual, DeepEquals, expected, Commentf("Expected one matching event."))

		params = map[string]string{"type": nodeDegradedType, "node": node}
		actual, err = s.timeline.GetEvents(ctx, params)
		c.Assert(err, IsNil)
		expected = nil
		c.Assert(actual, DeepEquals, expected, Commentf("Expected no matching events."))
	})
}

// TestMergeEvents tests that another timeline can be merged into the existing
// timeline.
func (s *SQLiteSuite) TestMergeEvents(c *C) {
	withTimeout(func(ctx context.Context) {
		events := []*pb.TimelineEvent{history.NewNodeDegraded(s.clock.Now(), "test-node")}
		err := s.timeline.RecordTimeline(ctx, events)
		c.Assert(err, IsNil)

		actual, err := s.timeline.GetEvents(ctx, nil)
		c.Assert(err, IsNil)

		c.Assert(actual, DeepEquals, events, Commentf("Expected events to be recorded."))
	})
}

// TestIgnoreDuplicateEvents tests that duplicate events will not be recoreded.
func (s *SQLiteSuite) TestIgnoreDuplicateEvents(c *C) {
	withTimeout(func(ctx context.Context) {
		ts := s.clock.Now()

		events := []*pb.TimelineEvent{
			history.NewNodeDegraded(ts, "test-node"),
			history.NewNodeDegraded(ts, "test-node"),
		}

		err := s.timeline.RecordTimeline(ctx, events)
		c.Assert(err, IsNil)

		actual, err := s.timeline.GetEvents(ctx, nil)
		c.Assert(err, IsNil)

		expected := []*pb.TimelineEvent{history.NewNodeDegraded(ts, "test-node")}

		c.Assert(actual, DeepEquals, expected, Commentf("Expected duplicate event to be ignored."))
	})
}

// withTimeout will run the provided test case with the default test timeout.
func withTimeout(fn func(ctx context.Context)) {
	// testTimeout specifies the overall time limit for a test.
	const testTimeout = 10 * time.Second

	ctx, cancel := context.WithTimeout(context.TODO(), testTimeout)
	defer cancel()
	fn(ctx)
}
