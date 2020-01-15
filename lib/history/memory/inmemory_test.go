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
	"testing"

	"github.com/gravitational/satellite/lib/test"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/lib/history"

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

// TestRecordStatus simply tests that the timeline can successfully record
// a status.
func (s *InMemorySuite) TestRecordStatus(c *C) {
	test.WithTimeout(func(ctx context.Context) {
		timeline := s.newTimeline()
		node := "test-node"
		old := &pb.NodeStatus{Name: node, Status: pb.NodeStatus_Running}

		err := timeline.RecordStatus(ctx, old)
		c.Assert(err, IsNil)

		actual, err := timeline.GetEvents(ctx, nil)
		c.Assert(err, IsNil)

		expected := []*pb.TimelineEvent{history.NewNodeRecovered(s.clock.Now(), node)}
		c.Assert(actual, test.DeepCompare, expected, Commentf("Expected the status to be recorded."))
	})
}

// TestFIFOEviction tests the FIFO eviction policy of the timeline.
func (s *InMemorySuite) TestFIFOEviction(c *C) {
	test.WithTimeout(func(ctx context.Context) {
		// Limit timeline capacity to 1.
		// Necessary to easily test if events are evicted if max capacity is reached.
		timeline := s.newLimitedTimeline(1)
		node := "test-node"
		old := &pb.NodeStatus{Name: node, Status: pb.NodeStatus_Running}
		new := &pb.NodeStatus{Name: node, Status: pb.NodeStatus_Degraded}

		// Recording two statuses should result in timeline to exceed capacity.
		err := timeline.RecordStatus(ctx, old)
		c.Assert(err, IsNil)
		err = timeline.RecordStatus(ctx, new)
		c.Assert(err, IsNil)

		actual, err := timeline.GetEvents(ctx, nil)
		c.Assert(err, IsNil)

		expected := []*pb.TimelineEvent{history.NewNodeDegraded(s.clock.Now(), node)}
		c.Assert(actual, test.DeepCompare, expected, Commentf("Expected degraded event."))
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
