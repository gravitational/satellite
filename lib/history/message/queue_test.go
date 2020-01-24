/*
Copyright 2020 Gravitational, Inc.

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

package message

import (
	"context"
	"time"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/lib/history"
	"github.com/gravitational/satellite/lib/history/memory"
	"github.com/gravitational/satellite/lib/test"

	"github.com/jonboulle/clockwork"
	. "gopkg.in/check.v1"
)

type QueueSuite struct {
	clock clockwork.Clock
}

var _ = Suite(&QueueSuite{})

func (s *QueueSuite) SetUpTest(c *C) {
	s.clock = clockwork.NewFakeClock()
}

func (s *QueueSuite) TearDownTest(c *C) {}

// TestSubscribe verifies that new subscribers will be brought up to date of
// all previously available events stored in the queue.
func (s *QueueSuite) TestSubscribe(c *C) {
	queue := s.newQueue()
	sub := s.newSubscriber()
	events := []*pb.TimelineEvent{history.NewNodeDegraded(s.clock.Now(), "node-1")}

	test.WithTimeout(func(ctx context.Context) {
		c.Assert(queue.timeline.RecordTimeline(ctx, events), IsNil)
		c.Assert(queue.Subscribe("sub-1", sub), IsNil)
		c.Assert(sub.notified, test.DeepCompare, events, Commentf("Expected sub to be up to date."))
	})
}

// TestPublish verifies that subscribers will be notified of new events that
// have been publish to the queue.
func (s *QueueSuite) TestPublish(c *C) {
	queue := s.newQueue()
	sub := s.newSubscriber()
	events := []*pb.TimelineEvent{history.NewNodeDegraded(s.clock.Now(), "node-1")}

	test.WithTimeout(func(ctx context.Context) {
		c.Assert(queue.Subscribe("sub-1", sub), IsNil)
		c.Assert(queue.Publish(ctx, events), IsNil)
		c.Assert(sub.notified, test.DeepCompare, events, Commentf("Expected sub to be up to date."))
	})
}

// TestUnsubscribe verifies that a subscriber that has unsunscibred will no
// longer be notified of new events.
func (s *QueueSuite) TestUnsubscribe(c *C) {
	queue := s.newQueue()
	sub := s.newSubscriber()
	events := []*pb.TimelineEvent{history.NewNodeDegraded(s.clock.Now(), "node-1")}

	test.WithTimeout(func(ctx context.Context) {
		c.Assert(queue.Subscribe("sub-1", sub), IsNil)
		c.Assert(queue.Unsubscribe("sub-1"), IsNil)
		c.Assert(queue.Publish(ctx, events), IsNil)
		c.Assert(sub.notified, test.DeepCompare, nil, Commentf("Expected zero value."))
	})
}

// newQueue constructs a queue for testing.
func (s *QueueSuite) newQueue() *Queue {
	// capacity defines default test timeline capacity.
	const capacity = 256
	timeline := memory.NewTimeline(s.clock, capacity)
	return NewQueue(timeline)
}

func (s *QueueSuite) newSubscriber() *mockSubscriber {
	return &mockSubscriber{}
}

// mockSubscriber implements Subscriber with mocked functionality for testing.
type mockSubscriber struct {
	// notified keeps a list of events that have been notified to the
	// this subscriber.
	notified []*pb.TimelineEvent
}

// Notify saves notified events
func (m *mockSubscriber) Notify(ctx context.Context, events []*pb.TimelineEvent) error {
	m.notified = append(m.notified, events...)
	return nil
}

// Timestamp returns zero value. Will not be used for this test suite.
func (m *mockSubscriber) Timestamp() time.Time {
	return time.Time{}
}
