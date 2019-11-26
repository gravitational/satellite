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
	"testing"
	"time"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"

	. "gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner.
func TestInMemory(t *testing.T) { TestingT(t) }

type InMemorySuite struct{}

var _ = Suite(&InMemorySuite{})

func (s *InMemorySuite) TestRecordStatus(c *C) {
	timeline := NewMemTimeline(1)
	timeline.RecordStatus(&pb.SystemStatus{Status: pb.SystemStatus_Running})

	actual := timeline.GetEvents()
	expected := []*Event{NewClusterRecoveredEvent(time.Time{}, "", pb.SystemStatus_Running.String())}

	removeTimestamps(actual)
	c.Assert(actual, DeepEquals, expected, Commentf("Test record status"))
}

func (s *InMemorySuite) TestFIFOEviction(c *C) {
	old := pb.SystemStatus_Running
	new := pb.SystemStatus_Degraded

	timeline := NewMemTimeline(1)
	timeline.RecordStatus(&pb.SystemStatus{Status: old})
	timeline.RecordStatus(&pb.SystemStatus{Status: new})

	actual := timeline.GetEvents()
	expected := []*Event{NewClusterDegradedEvent(time.Time{}, old.String(), new.String())}

	removeTimestamps(actual)
	c.Assert(actual, DeepEquals, expected, Commentf("Test FIFO eviction"))
}
