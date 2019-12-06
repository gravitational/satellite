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
	"os"

	"github.com/jonboulle/clockwork"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"

	_ "github.com/mattn/go-sqlite3" // initialize sqlite3
	. "gopkg.in/check.v1"
)

type SQLiteSuite struct {
	clock    clockwork.FakeClock
	timeline Timeline
}

var _ = Suite(&SQLiteSuite{})

// TestDBPath specifies location of test database.
const TestDBPath = "/tmp/test.db"

// SetupTest initializes test database.
func (s *SQLiteSuite) SetUpTest(c *C) {
	clock := clockwork.NewFakeClock()
	config := SQLiteTimelineConfig{
		DBPath:   TestDBPath,
		Capacity: 1,
		Clock:    clock,
	}
	timeline, err := NewSQLiteTimeline(config)
	c.Assert(err, IsNil)

	s.clock = clock
	s.timeline = timeline
}

// TearDownTest closes database and removed file.
func (s *SQLiteSuite) TearDownTest(c *C) {
	os.Remove(TestDBPath)
}

func (s *SQLiteSuite) TestRecordStatus(c *C) {
	err := s.timeline.RecordStatus(context.TODO(), &pb.SystemStatus{Status: pb.SystemStatus_Running})
	c.Assert(err, IsNil)

	actual, err := s.timeline.GetEvents(context.TODO())
	c.Assert(err, IsNil)

	expected := []*pb.TimelineEvent{NewClusterRecovered(s.clock.Now())}
	c.Assert(actual, DeepEquals, expected, Commentf("Test record status"))
}

func (s *SQLiteSuite) TestFIFOEviction(c *C) {
	old := &pb.SystemStatus{Status: pb.SystemStatus_Running}
	new := &pb.SystemStatus{Status: pb.SystemStatus_Degraded}

	err := s.timeline.RecordStatus(context.TODO(), old)
	c.Assert(err, IsNil)

	err = s.timeline.RecordStatus(context.TODO(), new)
	c.Assert(err, IsNil)

	actual, err := s.timeline.GetEvents(context.TODO())
	c.Assert(err, IsNil)

	expected := []*pb.TimelineEvent{NewClusterDegraded(s.clock.Now())}
	c.Assert(actual, DeepEquals, expected, Commentf("Test FIFO eviction"))
}
