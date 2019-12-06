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
	"database/sql"
	"sync"
	"time"

	"github.com/jonboulle/clockwork"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"

	"github.com/gravitational/trace"
	_ "github.com/mattn/go-sqlite3" // initialize sqlite3
	log "github.com/sirupsen/logrus"
)

// SQLiteTimeline represents a timeline of cluster status events. The timeline
// can hold a specified amount of events and uses a FIFO eviction policy.
// Timeline events are stored in a local sqlite database.
//
// Implements Timeline
type SQLiteTimeline struct {
	// clock is used to record event timestamps.
	clock clockwork.Clock
	// capacity specifies the max number of events that can be stored in the timeline.
	capacity int
	// size specifies the current number of events stored in the timeline.
	size int
	// database points to underlying sqlite database.
	database *sql.DB
	// lastStatus holds the last recorded cluster status.
	lastStatus *pb.SystemStatus
	// mu locks timeline access.
	mu sync.Mutex
}

// SQLiteTimelineConfig defines SQLiteTimeline configuration.
type SQLiteTimelineConfig struct {
	// DBPath specifies the database location.
	DBPath string
	// Capacity specifies the max number of events that can be stored in the timeline.
	Capacity int
	// Clock will be used to record event timestamps.
	Clock clockwork.Clock
}

// NewSQLiteTimeline initializes and returns a new SQLiteTimeline with the
// specified configuration.
func NewSQLiteTimeline(config SQLiteTimelineConfig) (*SQLiteTimeline, error) {
	database, err := initSQLite(config.DBPath)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return &SQLiteTimeline{
		clock:    config.Clock,
		capacity: config.Capacity,
		size:     0,
		database: database,
		// TODO: store and recover lastStatus in case satellite agent restarts.
		lastStatus: nil,
	}, nil
}

// initSQLite initializes connection to database provided by dbPath and
// initializes `events` table.
func initSQLite(dbPath string) (*sql.DB, error) {
	database, _ := sql.Open("sqlite3", dbPath)
	// TODO add context
	if _, err := database.Exec(createTableEvents); err != nil {
		return nil, trace.Wrap(err)
	}
	return database, nil
}

// RecordStatus records the differences between the previously stored status and
// the provided status.
func (t *SQLiteTimeline) RecordStatus(ctx context.Context, status *pb.SystemStatus) (err error) {
	events := diffCluster(t.clock, t.lastStatus, status)
	if len(events) == 0 {
		return nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	tx, err := t.database.BeginTx(ctx, nil)
	if err != nil {
		return trace.Wrap(err)
	}

	defer func() {
		// The rollback will be ignored if the tx has already been committed.
		if err == nil {
			return
		}
		if err := tx.Rollback(); err != nil {
			log.WithError(err).Error("Failed to rollback sql transaction.")
		}
	}()

	if err = t.insertEvents(ctx, tx, events); err != nil {
		return trace.Wrap(err, "failed to insert events")
	}

	if t.size+len(events) > t.capacity {
		if err = t.evictEvents(ctx, tx); err != nil {
			return trace.Wrap(err, "failed to evict old events")
		}
	}

	if err = tx.Commit(); err != nil {
		return trace.Wrap(err)
	}

	t.size = t.size + len(events)
	if t.size > t.capacity {
		t.size = t.capacity
	}

	t.lastStatus = status
	return nil
}

// GetEvents returns the current timeline.
func (t *SQLiteTimeline) GetEvents(ctx context.Context) (events []*pb.TimelineEvent, err error) {
	rows, err := t.database.QueryContext(ctx, selectAllFromEvents)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	defer func() {
		if err := rows.Close(); err != nil {
			log.WithError(err).Error("Failed to close sql rows.")
		}
	}()

	var (
		id        int
		timestamp time.Time
		eventType string
		node      string
		probe     string
		old       string
		new       string
	)

	for rows.Next() {
		if err := rows.Scan(&id, &timestamp, &eventType, &node, &probe, &old, &new); err != nil {
			return nil, trace.Wrap(err)
		}

		switch eventType {
		case clusterRecoveredType:
			events = append(events, NewClusterRecovered(timestamp))
		case clusterDegradedType:
			events = append(events, NewClusterDegraded(timestamp))
		case nodeAddedType:
			events = append(events, NewNodeAdded(timestamp, node))
		case nodeRemovedType:
			events = append(events, NewNodeRemoved(timestamp, node))
		case nodeRecoveredType:
			events = append(events, NewNodeRecovered(timestamp, node))
		case nodeDegradedType:
			events = append(events, NewNodeDegraded(timestamp, node))
		case probeSucceededType:
			events = append(events, NewProbeSucceeded(timestamp, node, probe))
		case probeFailedType:
			events = append(events, NewProbeFailed(timestamp, node, probe))
		default:
			return nil, trace.NotFound("unknown event type %v", eventType)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, trace.Wrap(err)
	}

	return events, nil
}

// insertEvents inserts the provided events into the timeline.
func (t *SQLiteTimeline) insertEvents(ctx context.Context, tx *sql.Tx, events []*pb.TimelineEvent) error {
	// TODO
	return nil
}

// evictEvents deletes oldest events if the timeline is larger than its max
// capacity.
func (t *SQLiteTimeline) evictEvents(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, deleteOldFromEvents, t.capacity); err != nil {
		return trace.Wrap(err)
	}
	return nil
}

// These types are used to specify the type of an event when storing event
// into a database.
const (
	clusterRecoveredType = "ClusterRecovered"
	clusterDegradedType  = "ClusterDegraded"
	nodeAddedType        = "NodeAdded"
	nodeRemovedType      = "NodeRemoved"
	nodeRecoveredType    = "NodeRecovered"
	nodeDegradedType     = "NodeDegraded"
	probeSucceededType   = "ProbeSucceeded"
	probeFailedType      = "ProbeFailed"
	unknownType          = "Unknown"
)

// createTableEvents is sql statement to create an `events` table.
const createTableEvents = `
CREATE TABLE IF NOT EXISTS events (
	id INTEGER PRIMARY KEY,
	timestamp DATETIME DEFAULT CURRENT_TIMESTAMP NOT NULL,
	type TEXT NOT NULL,
	node TEXT,
	probe TEXT,
	oldState TEXT,
	newState TEXT
)
`

// TODO: index node/probe fields to improve filtering performance.

// insertIntoEvents is sql statement to insert entry into `events` table. Used for
// batch insert statement.
const insertIntoEvents = `
INSERT INTO events (
	timestamp,
	type,
	node,
	probe,
	oldState,
	newState
) VALUES %s
`

// selectAllFromEvents is sql query to select all entries from `events` table.
const selectAllFromEvents = `SELECT * FROM events`

// deleteOldFromEvents is sql statement to delete entries from `events` table when full.
const deleteOldFromEvents = `DELETE FROM events WHERE id IN (SELECT id FROM events ORDER BY id DESC LIMIT -1 OFFSET ?);`
