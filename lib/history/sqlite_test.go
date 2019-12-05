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
	"testing"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"

	"github.com/jonboulle/clockwork"
	_ "github.com/mattn/go-sqlite3" // initialize sqlite3
	. "gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner.
func TestSQLite(t *testing.T) { TestingT(t) }

type SQLiteSuite struct {
	timeline *SQLiteTimeline
}

var _ = Suite(&SQLiteSuite{})

// TestDBPath specifies location of test database.
const TestDBPath = "/tmp/test.db"

// SetupTest initializes test database.
func (s *SQLiteSuite) SetUpTest(c *C) {
	config := SQLiteTimelineConfig{
		DBPath:   TestDBPath,
		Capacity: 1,
		Clock:    clockwork.NewFakeClock(),
	}

	timeline, err := NewSQLiteTimeline(config)
	c.Assert(err, IsNil)

	s.timeline = timeline
}

// TearDownTest closes database and removed file.
func (s *SQLiteSuite) TearDownTest(c *C) {
	os.Remove(TestDBPath)
}

func (s *SQLiteSuite) TestRecordStatus(c *C) {

	err := s.timeline.RecordStatus(context.TODO(), NewClusterStatus(&pb.SystemStatus{Status: pb.SystemStatus_Running}))
	c.Assert(err, IsNil)

	actual, err := s.timeline.GetEvents()
	c.Assert(err, IsNil)

	expected := []Event{NewClusterRecovered(s.timeline.clock.Now())}
	c.Assert(actual, DeepEquals, expected, Commentf("Test record status"))
}

func (s *SQLiteSuite) TestFIFOEviction(c *C) {
	old := NewClusterStatus(&pb.SystemStatus{Status: pb.SystemStatus_Running})
	new := NewClusterStatus(&pb.SystemStatus{Status: pb.SystemStatus_Degraded})
	timeline := NewMemTimeline(s.timeline.clock, 1)

	err := timeline.RecordStatus(context.TODO(), old)
	c.Assert(err, IsNil)

	err = timeline.RecordStatus(context.TODO(), new)
	c.Assert(err, IsNil)

	actual, err := timeline.GetEvents()
	c.Assert(err, IsNil)

	expected := []Event{NewClusterDegraded(timeline.clock.Now())}
	c.Assert(actual, DeepEquals, expected, Commentf("Test FIFO eviction"))
}
