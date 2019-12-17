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
	"time"

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

func (s *InMemorySuite) TestRecordStatus(c *C) {
	ctx, cancel := context.WithTimeout(context.TODO(), testTimeout)
	defer cancel()

	var timeline history.Timeline
	timeline = NewTimeline(s.clock, 1)

	node := "test-node"
	old := &pb.NodeStatus{Name: node, Status: pb.NodeStatus_Running}
	new := &pb.NodeStatus{Name: node, Status: pb.NodeStatus_Degraded}

	var err error
	err = timeline.RecordStatus(ctx, old)
	c.Assert(err, IsNil)
	err = timeline.RecordStatus(ctx, new)
	c.Assert(err, IsNil)

	actual, err := timeline.GetEvents(ctx, nil)
	c.Assert(err, IsNil)

	expected := []*pb.TimelineEvent{history.NewNodeDegraded(s.clock.Now(), node)}
	c.Assert(actual, DeepEquals, expected, Commentf("Test record status"))
}

func (s *InMemorySuite) TestFIFOEviction(c *C) {
	ctx, cancel := context.WithTimeout(context.TODO(), testTimeout)
	defer cancel()

	var timeline history.Timeline
	timeline = NewTimeline(s.clock, 1)

	node := "test-node"
	old := &pb.NodeStatus{Name: node, Status: pb.NodeStatus_Running}
	new := &pb.NodeStatus{Name: node, Status: pb.NodeStatus_Degraded}

	var err error
	err = timeline.RecordStatus(ctx, old)
	c.Assert(err, IsNil)
	err = timeline.RecordStatus(ctx, new)
	c.Assert(err, IsNil)
	err = timeline.RecordStatus(ctx, old)
	c.Assert(err, IsNil)

	actual, err := timeline.GetEvents(ctx, nil)
	c.Assert(err, IsNil)

	expected := []*pb.TimelineEvent{history.NewNodeRecovered(s.clock.Now(), node)}
	c.Assert(actual, DeepEquals, expected, Commentf("Test FIFO eviction"))
}

// testTimeout specifies the overall time limit for a test.
const testTimeout = 10 * time.Second
