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
	"fmt"
	"strings"
	"sync"
	"time"

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
	// capacity specifies the max number of events that can be stored in the timeline.
	capacity int
	// size specifies the current number of events stored in the timeline.
	size int
	// database points to underlying sqlite database.
	database *sql.DB
	// lastStatus holds the last recorded cluster status.
	lastStatus *Cluster
	// mu locks timeline access.
	mu sync.Mutex
}

// SQLiteTimelineConfig defines SQLiteTimeline configuration.
type SQLiteTimelineConfig struct {
	// DBPath specifies the database location.
	DBPath string
	// Capacity specifies the max number of events that can be stored in the timeline.
	Capacity int
}

// NewSQLiteTimeline initializes and returns a new SQLiteTimeline with the
// specified configuration.
func NewSQLiteTimeline(config SQLiteTimelineConfig) (*SQLiteTimeline, error) {
	database, err := initSQLite(config.DBPath)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return &SQLiteTimeline{
		capacity: config.Capacity,
		size:     0,
		database: database,
		// TODO: store and recover lastStatus in case satellite agent restarts.
		lastStatus: &Cluster{Status: pb.SystemStatus_Unknown.String()},
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
func (t *SQLiteTimeline) RecordStatus(ctx context.Context, status *pb.SystemStatus) error {
	cluster := parseSystemStatus(status)
	events := t.lastStatus.diffCluster(cluster)
	if len(events) == 0 {
		return nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.size = t.size + len(events)

	tx, err := t.database.BeginTx(ctx, nil)
	if err != nil {
		return trace.Wrap(err)
	}

	defer func() {
		// revert timeline size
		t.size = t.size - len(events)

		if err := tx.Rollback(); err != nil {
			log.WithError(err).Error("Failed to rollback sql transaction.")
		}
	}()

	if err := t.insertEvents(events); err != nil {
		return trace.Wrap(err, "failed to insert events")
	}

	if err := t.evictEvents(); err != nil {
		return trace.Wrap(err, "failed to evict old events")
	}

	if err := tx.Commit(); err != nil {
		return trace.Wrap(err)
	}

	t.lastStatus = cluster
	return nil
}

// GetEvents returns the current timeline.
func (t *SQLiteTimeline) GetEvents() (events []*Event, err error) {
	rows, err := t.database.Query(selectAllFromEvents)
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
		eventType EventType
		node      string
		probe     string
		old       string
		new       string
	)

	for rows.Next() {
		if err := rows.Scan(&id, &timestamp, &eventType, &node, &probe, &old, &new); err != nil {
			return nil, trace.Wrap(err)
		}

		var event *Event
		switch eventType {
		case ClusterRecovered:
			event = NewClusterRecoveredEvent(timestamp, old, new)
		case ClusterDegraded:
			event = NewClusterDegradedEvent(timestamp, old, new)
		case NodeAdded:
			event = NewNodeAddedEvent(timestamp, node)
		case NodeRemoved:
			event = NewNodeRemovedEvent(timestamp, node)
		case NodeRecovered:
			event = NewNodeRecoveredEvent(timestamp, node, old, new)
		case NodeDegraded:
			event = NewNodeDegradedEvent(timestamp, node, old, new)
		case ProbePassed:
			event = NewProbePassedEvent(timestamp, node, probe, old, new)
		case ProbeFailed:
			event = NewProbeFailedEvent(timestamp, node, probe, old, new)
		default:
			event = NewUnknownEvent(timestamp)
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, trace.Wrap(err)
	}

	return events, nil
}

// insertEvents inserts the provided events into the timeline.
func (t *SQLiteTimeline) insertEvents(events []*Event) error {
	insertEvents, valueArgs := prepareBulkInsert(events)
	if _, err := t.database.Exec(insertEvents, valueArgs...); err != nil {
		return trace.Wrap(err)
	}
	return nil
}

// prepareBulkInsert prepares bulk insert events statement and value arguments.
func prepareBulkInsert(events []*Event) (insertEvents string, valueArgs []interface{}) {
	// eventValueString specifies the insert value string for an Event.
	// It should be consistent with the number of fields in an event.
	const eventValueString = "(?, ?, ?, ?, ?, ?)"

	// prepare bulk insert statement
	valueStrings := make([]string, 0, len(events))
	for _, event := range events {
		valueStrings = append(valueStrings, eventValueString)
		valueArgs = append(valueArgs, event.ToArgs())
	}
	insertEvents = fmt.Sprintf(insertIntoEvents, strings.Join(valueStrings, ","))
	return insertEvents, valueArgs
}

// evictEvents deletes oldest events if the timeline is larger than its max
// capcity.
func (t *SQLiteTimeline) evictEvents() error {
	if t.size <= t.capacity {
		return nil
	}

	if _, err := t.database.Exec(deleteOldFromEvents, t.capacity); err != nil {
		return trace.Wrap(err)
	}

	t.size = t.capacity
	return nil
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

// insertIntoEvents is sql statement to insert entry into `event` table. Used for
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
const deleteOldFromEvents = `DELETE FROM events WHERE id IN (SELECT id FROM event ORDER BY id DESC LIMIT -1 OFFSET ?);`
