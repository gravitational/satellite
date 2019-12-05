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

package history

import (
	"context"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"

	"github.com/jonboulle/clockwork"
	. "gopkg.in/check.v1"
)

type InMemorySuite struct {
	clock clockwork.FakeClock
}

var _ = Suite(&InMemorySuite{})

func (s *InMemorySuite) SetUpSuite(c *C) {
	s.clock = clockwork.NewFakeClock()
}

func (s *InMemorySuite) TestRecordStatus(c *C) {
	timeline := NewMemTimeline(s.clock, 1)
	err := timeline.RecordStatus(context.TODO(), NewClusterStatus(&pb.SystemStatus{Status: pb.SystemStatus_Running}))
	c.Assert(err, IsNil)

	actual, err := timeline.GetEvents()
	c.Assert(err, IsNil)

	expected := []Event{NewClusterRecovered(timeline.clock.Now())}
	c.Assert(actual, DeepEquals, expected, Commentf("Test record status"))
}

func (s *InMemorySuite) TestFIFOEviction(c *C) {
	old := NewClusterStatus(&pb.SystemStatus{Status: pb.SystemStatus_Running})
	new := NewClusterStatus(&pb.SystemStatus{Status: pb.SystemStatus_Degraded})
	timeline := NewMemTimeline(s.clock, 1)

	err := timeline.RecordStatus(context.TODO(), old)
	c.Assert(err, IsNil)

	err = timeline.RecordStatus(context.TODO(), new)
	c.Assert(err, IsNil)

	actual, err := timeline.GetEvents()
	c.Assert(err, IsNil)

	expected := []Event{NewClusterDegraded(timeline.clock.Now())}
	c.Assert(actual, DeepEquals, expected, Commentf("Test FIFO eviction"))
}
