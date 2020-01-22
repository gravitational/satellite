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
	"github.com/gravitational/satellite/lib/test"

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
	s.clock = clockwork.NewFakeClock()

	timeline, err := s.newDefaultTimeline()
	c.Assert(err, IsNil)
	s.timeline = timeline
}

// TearDownTest closes database and removed file.
func (s *SQLiteSuite) TearDownTest(c *C) {
	if fileExists(TestDBPath) {
		c.Assert(os.Remove(TestDBPath), IsNil)
	}
}

func (s *SQLiteSuite) TestRecordStatus(c *C) {
	var testCases = []struct {
		comment  string
		statuses []*pb.NodeStatus
		expected []*pb.TimelineEvent
	}{
		{
			comment: "Expected the status to be recorded.",
			statuses: []*pb.NodeStatus{
				&pb.NodeStatus{Name: "node-1", Status: pb.NodeStatus_Running},
			},
			expected: []*pb.TimelineEvent{history.NewNodeRecovered(s.clock.Now(), "node-1")},
		},
		{
			comment: "Expected only the first status to be recorded.",
			statuses: []*pb.NodeStatus{
				&pb.NodeStatus{Name: "node-1", Status: pb.NodeStatus_Running},
				&pb.NodeStatus{Name: "node-1", Status: pb.NodeStatus_Running},
			},
			expected: []*pb.TimelineEvent{history.NewNodeRecovered(s.clock.Now(), "node-1")},
		},
		{
			comment: "Expected events recovered then degraded.",
			statuses: []*pb.NodeStatus{
				&pb.NodeStatus{Name: "node-1", Status: pb.NodeStatus_Running},
				&pb.NodeStatus{Name: "node-1", Status: pb.NodeStatus_Degraded},
			},
			expected: []*pb.TimelineEvent{
				history.NewNodeRecovered(s.clock.Now(), "node-1"),
				history.NewNodeDegraded(s.clock.Now().Add(time.Second), "node-1"),
			},
		},
		{
			comment: "Expected failed probe.",
			statuses: []*pb.NodeStatus{
				&pb.NodeStatus{
					Name:   "node-1",
					Status: pb.NodeStatus_Running,
					Probes: []*pb.Probe{healthyProbe},
				},
				&pb.NodeStatus{
					Name:   "node-1",
					Status: pb.NodeStatus_Degraded,
					Probes: []*pb.Probe{failedProbe},
				},
			},
			expected: []*pb.TimelineEvent{
				history.NewNodeRecovered(s.clock.Now(), "node-1"),
				history.NewNodeDegraded(s.clock.Now().Add(time.Second), "node-1"),
				history.NewProbeFailed(s.clock.Now().Add(time.Second), "node-1", failedProbe.GetChecker()),
			},
		},
	}

	for _, testCase := range testCases {
		clock := clockwork.NewFakeClock()
		timeline, err := s.newTimelineWithClock(clock)
		c.Assert(err, IsNil)

		test.WithTimeout(func(ctx context.Context) {
			for _, status := range testCase.statuses {
				c.Assert(timeline.RecordStatus(ctx, status), IsNil)
				clock.Advance(time.Second) // Advance clock so events can be ordered
			}

			actual, err := timeline.GetEvents(ctx, nil)
			c.Assert(err, IsNil)
			c.Assert(actual, test.DeepCompare, testCase.expected, Commentf(testCase.comment))
		})
	}
}

// TestEviction tests timeline correctly implements an eviction policy.
func (s *SQLiteSuite) TestEviction(c *C) {
	comment := Commentf("Expected all events to be evicted.")
	node := "test-node"
	status := &pb.NodeStatus{Name: node, Status: pb.NodeStatus_Running}
	var expected []*pb.TimelineEvent

	test.WithTimeout(func(ctx context.Context) {
		c.Assert(s.timeline.RecordStatus(ctx, status), IsNil)

		// Advance clock and evict all events before this time.
		s.clock.Advance(time.Second)
		c.Assert(s.timeline.evictEvents(ctx, s.clock.Now()), IsNil)

		actual, err := s.timeline.GetEvents(ctx, nil)
		c.Assert(err, IsNil)
		c.Assert(actual, test.DeepCompare, expected, comment)
	})
}

// TestFilterEvents tests that events can be filtered.
func (s *SQLiteSuite) TestFilterEvents(c *C) {
	var testCases = []struct {
		comment  string
		statuses []*pb.NodeStatus
		params   map[string]string
		expected []*pb.TimelineEvent
	}{
		{
			comment: "Expected one matching event.",
			statuses: []*pb.NodeStatus{
				&pb.NodeStatus{Name: "node-1", Status: pb.NodeStatus_Running},
			},
			params:   map[string]string{"type": nodeRecoveredType, "node": "node-1"},
			expected: []*pb.TimelineEvent{history.NewNodeRecovered(s.clock.Now(), "node-1")},
		},
		{
			comment: "Expected no matching events.",
			statuses: []*pb.NodeStatus{
				&pb.NodeStatus{Name: "node-1", Status: pb.NodeStatus_Running},
			},
			params:   map[string]string{"type": nodeDegradedType, "node": "node-1"},
			expected: nil,
		},
		{
			comment: "Expected two matching events.",
			statuses: []*pb.NodeStatus{
				&pb.NodeStatus{Name: "node-1", Status: pb.NodeStatus_Running},
				&pb.NodeStatus{Name: "node-1", Status: pb.NodeStatus_Degraded},
			},
			params: map[string]string{"node": "node-1"},
			expected: []*pb.TimelineEvent{
				history.NewNodeRecovered(s.clock.Now(), "node-1"),
				history.NewNodeDegraded(s.clock.Now().Add(time.Second), "node-1"),
			},
		},
	}

	for _, testCase := range testCases {
		clock := clockwork.NewFakeClock()
		timeline, err := s.newTimelineWithClock(clock)
		c.Assert(err, IsNil)

		test.WithTimeout(func(ctx context.Context) {
			for _, status := range testCase.statuses {
				c.Assert(timeline.RecordStatus(ctx, status), IsNil)
				clock.Advance(time.Second) // Advance clock so events can be ordered
			}

			actual, err := timeline.GetEvents(ctx, testCase.params)
			c.Assert(err, IsNil)
			c.Assert(actual, test.DeepCompare, testCase.expected, Commentf(testCase.comment))
		})
	}
}

// TestMergeEvents tests that another timeline can be merged into the existing
// timeline.
func (s *SQLiteSuite) TestMergeEvents(c *C) {
	comment := Commentf("Expected events to be recorded.")
	events := []*pb.TimelineEvent{history.NewNodeDegraded(s.clock.Now(), "test-node")}

	test.WithTimeout(func(ctx context.Context) {
		c.Assert(s.timeline.RecordTimeline(ctx, events), IsNil)

		actual, err := s.timeline.GetEvents(ctx, nil)
		c.Assert(err, IsNil)
		c.Assert(actual, test.DeepCompare, events, comment)
	})
}

// TestIgnoreDuplicateEvents tests that duplicate events will not be recoreded.
func (s *SQLiteSuite) TestIgnoreDuplicateEvents(c *C) {
	comment := Commentf("Expected duplicate event to be ignored.")
	events := []*pb.TimelineEvent{
		history.NewNodeDegraded(s.clock.Now(), "test-node"),
		history.NewNodeDegraded(s.clock.Now(), "test-node"),
	}
	expected := []*pb.TimelineEvent{history.NewNodeDegraded(s.clock.Now(), "test-node")}

	test.WithTimeout(func(ctx context.Context) {
		c.Assert(s.timeline.RecordTimeline(ctx, events), IsNil)

		actual, err := s.timeline.GetEvents(ctx, nil)
		c.Assert(err, IsNil)
		c.Assert(actual, test.DeepCompare, expected, comment)
	})
}

// TestIgnoreExpiredEvents tests that expired events will not be recorded.
func (s *SQLiteSuite) TestIgnoreExpiredEvents(c *C) {
	expiredTimestamp := s.clock.Now().Add(-(time.Hour + time.Second))
	var testCases = []struct {
		comment  string
		events   []*pb.TimelineEvent
		expected []*pb.TimelineEvent
	}{
		{
			comment:  "Expected expired event to be ignored.",
			events:   []*pb.TimelineEvent{history.NewNodeRecovered(expiredTimestamp, "node-1")},
			expected: nil,
		},
		{
			comment: "Expected one event to be recoreded.",
			events: []*pb.TimelineEvent{
				history.NewNodeRecovered(expiredTimestamp, "node-1"),
				history.NewNodeRecovered(s.clock.Now(), "node-2"),
			},
			expected: []*pb.TimelineEvent{history.NewNodeRecovered(s.clock.Now(), "node-2")},
		},
	}

	for _, testCase := range testCases {
		timeline, err := s.newDefaultTimeline()
		c.Assert(err, IsNil)

		test.WithTimeout(func(ctx context.Context) {
			c.Assert(timeline.RecordTimeline(ctx, testCase.events), IsNil)

			actual, err := timeline.GetEvents(ctx, nil)
			c.Assert(err, IsNil)
			c.Assert(actual, test.DeepCompare, testCase.expected, Commentf(testCase.comment))
		})
	}
}

// newDefaultTimeline constructs a new timeline with default configuration.
func (s *SQLiteSuite) newDefaultTimeline() (*Timeline, error) {
	config := Config{
		DBPath:            TestDBPath,
		RetentionDuration: time.Hour,
		Clock:             s.clock,
	}
	return s.newTimeline(config)
}

// newTimelineWithClock constructs a new timeline with the provided clock.
func (s *SQLiteSuite) newTimelineWithClock(clock clockwork.Clock) (*Timeline, error) {
	config := Config{
		DBPath:            TestDBPath,
		RetentionDuration: time.Hour,
		Clock:             clock,
	}
	return s.newTimeline(config)
}

// newTimeline constructs a new timeline with the provided configuration.
func (s *SQLiteSuite) newTimeline(config Config) (*Timeline, error) {
	// timelineInitTimeout specifies the amount of time given to initialize database.
	const timelineInitTimeout = 5 * time.Second

	ctx, cancel := context.WithTimeout(context.TODO(), timelineInitTimeout)
	defer cancel()

	if fileExists(TestDBPath) {
		if err := os.Remove(TestDBPath); err != nil {
			return nil, err
		}
	}
	return NewTimeline(ctx, config)
}

// fileExists checks if a file exists and is not a directory.
func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

var healthyProbe = &pb.Probe{
	Checker: "test-probe",
	Status:  pb.Probe_Running,
}

var failedProbe = &pb.Probe{
	Checker: "test-probe",
	Status:  pb.Probe_Failed,
	Error:   "invalid state",
}
