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
	timeline history.Timeline
}

var _ = Suite(&SQLiteSuite{})

// TestDBPath specifies location of test database.
const TestDBPath = "/tmp/test.db"

// SetupTest initializes test database.
func (s *SQLiteSuite) SetUpTest(c *C) {
	clock := clockwork.NewFakeClock()
	config := Config{
		Context:       context.TODO(),
		DBPath:        TestDBPath,
		DBConnTimeout: 5 * time.Second,
		DBTimeout:     5 * time.Second,
		Capacity:      1,
		Clock:         clock,
	}
	timeline, err := NewTimeline(config)
	c.Assert(err, IsNil)

	s.clock = clock
	s.timeline = timeline
}

// TearDownTest closes database and removed file.
func (s *SQLiteSuite) TearDownTest(c *C) {
	os.Remove(TestDBPath)
}

func (s *SQLiteSuite) TestRecordStatus(c *C) {
	err := s.timeline.RecordStatus(&pb.SystemStatus{Status: pb.SystemStatus_Running})
	c.Assert(err, IsNil)

	actual, err := s.timeline.GetEvents(nil)
	c.Assert(err, IsNil)

	expected := []*pb.TimelineEvent{history.NewClusterRecovered(s.clock.Now())}
	c.Assert(actual, DeepEquals, expected, Commentf("Test record status"))
}

func (s *SQLiteSuite) TestFIFOEviction(c *C) {
	old := &pb.SystemStatus{Status: pb.SystemStatus_Running}
	new := &pb.SystemStatus{Status: pb.SystemStatus_Degraded}

	err := s.timeline.RecordStatus(old)
	c.Assert(err, IsNil)

	err = s.timeline.RecordStatus(new)
	c.Assert(err, IsNil)

	actual, err := s.timeline.GetEvents(nil)
	c.Assert(err, IsNil)

	expected := []*pb.TimelineEvent{history.NewClusterDegraded(s.clock.Now())}
	c.Assert(actual, DeepEquals, expected, Commentf("Test FIFO eviction"))
}

func (s *SQLiteSuite) TestFilterEvents(c *C) {
	err := s.timeline.RecordStatus(&pb.SystemStatus{Status: pb.SystemStatus_Running})
	c.Assert(err, IsNil)

	params := map[string]string{"type": clusterRecoveredType}
	actual, err := s.timeline.GetEvents(params)
	c.Assert(err, IsNil)

	expected := []*pb.TimelineEvent{history.NewClusterRecovered(s.clock.Now())}
	c.Assert(actual, DeepEquals, expected, Commentf("Test filter events - one match"))

	params = map[string]string{"type": clusterDegradedType}
	actual, err = s.timeline.GetEvents(params)
	c.Assert(err, IsNil)

	expected = nil
	c.Assert(actual, DeepEquals, expected, Commentf("Test filter events - no match"))
}
