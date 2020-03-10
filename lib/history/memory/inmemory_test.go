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

package memory

import (
	"context"
	"fmt"
	"testing"
	"time"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/lib/test"

	"github.com/jonboulle/clockwork"
	. "gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner.
func TestInMemory(t *testing.T) { TestingT(t) }

type InMemorySuite struct {
	clock clockwork.FakeClock
}

var _ = Suite(&InMemorySuite{})

func (s *InMemorySuite) SetUpSuite(c *C) {
	s.clock = clockwork.NewFakeClock()
}

// TestRecordEvents verifies that the timeline can successfully record a new
// event.
func (s *InMemorySuite) TestRecordEvents(c *C) {
	timeline := s.newTimeline()
	events := []*pb.TimelineEvent{pb.NewNodeRecovered(s.clock.Now(), node)}
	expected := []*pb.TimelineEvent{pb.NewNodeRecovered(s.clock.Now(), node)}
	comment := Commentf("Expected the a node recovered event to be recorded.")

	test.WithTimeout(func(ctx context.Context) {
		c.Assert(timeline.RecordEvents(ctx, events), IsNil, comment)

		actual, err := timeline.GetEvents(ctx, nil)
		c.Assert(err, IsNil, comment)
		c.Assert(actual, test.DeepCompare, expected, comment)
	})
}

// TestFIFOEviction tests the FIFO eviction policy of the timeline.
func (s *InMemorySuite) TestFIFOEviction(c *C) {
	// Limit timeline capacity to 1.
	// Necessary to easily test if events are evicted if max capacity is reached.
	timeline := s.newLimitedTimeline(1)
	events := []*pb.TimelineEvent{
		pb.NewNodeRecovered(s.clock.Now().Add(-time.Second), node), // older event should be evicted first
		pb.NewNodeDegraded(s.clock.Now(), node),
	}
	expected := []*pb.TimelineEvent{pb.NewNodeDegraded(s.clock.Now(), node)}
	comment := Commentf("Expected node degraded event.")

	test.WithTimeout(func(ctx context.Context) {

		// Recording two statuses should result in timeline exceeding capacity.
		c.Assert(timeline.RecordEvents(ctx, events), IsNil, comment)

		fmt.Println(timeline.events)

		actual, err := timeline.GetEvents(ctx, nil)
		c.Assert(err, IsNil, comment)
		c.Assert(actual, test.DeepCompare, expected, comment)
	})
}

// TestIgnoreDuplicate verifies duplicate events will not be recorded.
func (s *InMemorySuite) TestIgnoreDuplicate(c *C) {
	timeline := s.newTimeline()
	events := []*pb.TimelineEvent{pb.NewNodeDegraded(s.clock.Now(), node)}
	expected := []*pb.TimelineEvent{pb.NewNodeDegraded(s.clock.Now(), node)}
	comment := Commentf("Expected a single event.")

	test.WithTimeout(func(ctx context.Context) {
		c.Assert(timeline.RecordEvents(ctx, events), IsNil, comment)
		c.Assert(timeline.RecordEvents(ctx, events), IsNil, comment)

		actual, err := timeline.GetEvents(ctx, nil)
		c.Assert(err, IsNil, comment)
		c.Assert(actual, test.DeepCompare, expected, comment)
	})
}

// newTimeline initializes a new timeline with default test capacity.
func (s *InMemorySuite) newTimeline() *Timeline {
	// defaultCapacity specifies default capacity of test timeline.
	const defaultCapacity = 256

	return NewTimeline(s.clock, defaultCapacity)
}

// newLimitedTimeline initializes a new timeline with provided capcity.
func (s *InMemorySuite) newLimitedTimeline(capacity int) *Timeline {
	return NewTimeline(s.clock, capacity)
}

// node defines node name used for tests
const node = "test-node"
