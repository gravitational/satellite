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

	"github.com/gravitational/trace"
	"github.com/jonboulle/clockwork"
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
	lastStatus ClusterStatus
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
		lastStatus: NewClusterStatus(nil),
	}, nil
}

// initSQLite initializes connection to database provided by dbPath and
// initializes `events` table.
func initSQLite(dbPath string) (*sql.DB, error) {
	database, _ := sql.Open("sqlite3", dbPath)
	if _, err := database.Exec(createTableEvents); err != nil {
		return nil, trace.Wrap(err)
	}
	return database, nil
}

// RecordStatus records the differences between the previously stored status and
// the provided status.
func (t *SQLiteTimeline) RecordStatus(ctx context.Context, status ClusterStatus) error {
	events := t.lastStatus.diffCluster(t.clock, status)
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
		if err := tx.Rollback(); err != nil {
			log.WithError(err).Error("Failed to rollback sql transaction.")
		}
	}()

	if err := t.insertEvents(events); err != nil {
		return trace.Wrap(err, "failed to insert events")
	}

	if t.size+len(events) > t.capacity {
		if err := t.evictEvents(); err != nil {
			return trace.Wrap(err, "failed to evict old events")
		}
	}

	if err := tx.Commit(); err != nil {
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
func (t *SQLiteTimeline) GetEvents() (events []Event, err error) {
	rows, err := t.database.Query(selectAllFromEvents)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	defer func() {
		if err := rows.Close(); err != nil {
			log.WithError(err).Error("Failed to close sql rows.")
		}
	}()

	for rows.Next() {
		row, err := scanEventRow(rows)
		if err != nil {
			return nil, trace.Wrap(err)
		}

		event, err := rowToEvent(row)
		if err != nil {
			return nil, trace.Wrap(err)
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, trace.Wrap(err)
	}

	return events, nil
}

// insertEvents inserts the provided events into the timeline.
func (t *SQLiteTimeline) insertEvents(events []Event) error {
	for _, event := range events {
		insertEvent, args := event.PrepareInsert()
		if _, err := t.database.Exec(insertEvent, args...); err != nil {
			return trace.Wrap(err)
		}
	}
	return nil
}

// evictEvents deletes oldest events if the timeline is larger than its max
// capacity.
func (t *SQLiteTimeline) evictEvents() error {
	if _, err := t.database.Exec(deleteOldFromEvents, t.capacity); err != nil {
		return trace.Wrap(err)
	}
	return nil
}

type eventRow struct {
	id        int
	timestamp time.Time
	eventType string
	node      sql.NullString
	probe     sql.NullString
	old       sql.NullString
	new       sql.NullString
}

func scanEventRow(rows *sql.Rows) (row eventRow, err error) {
	if err = rows.Scan(
		&row.id,
		&row.timestamp,
		&row.eventType,
		&row.node,
		&row.probe,
		&row.old,
		&row.new,
	); err != nil {
		return row, trace.Wrap(err)
	}
	return row, nil
}

func rowToEvent(row eventRow) (Event, error) {
	switch row.eventType {
	case clusterRecoveredType:
		return NewClusterRecovered(row.timestamp), nil
	case clusterDegradedType:
		return NewClusterDegraded(row.timestamp), nil
	case nodeAddedType:
		return NewNodeAdded(row.timestamp, row.node.String), nil
	case nodeRemovedType:
		return NewNodeRemoved(row.timestamp, row.node.String), nil
	case nodeRecoveredType:
		return NewNodeRecovered(row.timestamp, row.node.String), nil
	case nodeDegradedType:
		return NewNodeDegraded(row.timestamp, row.node.String), nil
	case probeSucceededType:
		return NewProbeSucceeded(row.timestamp, row.node.String, row.probe.String), nil
	case probeFailedType:
		return NewProbeFailed(row.timestamp, row.node.String, row.probe.String), nil
	default:
		return nil, trace.NotFound("unknown event type %v", row.eventType)
	}
}

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
