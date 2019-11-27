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
	"time"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"

	"github.com/gravitational/trace"
	_ "github.com/mattn/go-sqlite3" // initialize sqlite3
)

// SQLiteTimeline represents a timeline of cluster status events. The timeline
// can hold a specified amount of events and uses a FIFO eviction policy.
// Timeline events are stored in a local sqlite database.
//
// Implements Timeline
type SQLiteTimeline struct {
	// size specifies the max number of events stored in the timeline.
	size int
	// database points to underlying sqlite database.
	database *sql.DB
	// lastStatus holds the last recorded cluster status.
	// Initial cluster status is `Unknown`.
	lastStatus *Cluster
}

// NewSQLiteTimeline initializes and returns a new SQLiteTimeline with the
// specified size.
func NewSQLiteTimeline(database *sql.DB, size int) (Timeline, error) {
	if err := initEventTable(database); err != nil {
		return nil, trace.Wrap(err)
	}

	return &SQLiteTimeline{
		size:       size,
		database:   database,
		lastStatus: &Cluster{Status: pb.SystemStatus_Unknown.String()},
	}, nil
}

// initEventTable initializes event table schema.
func initEventTable(db *sql.DB) error {
	createTables, err := db.Prepare(createTableEvent)
	if err != nil {
		return trace.Wrap(err)
	}

	if _, err := createTables.Exec(); err != nil {
		return trace.Wrap(err)
	}

	return nil
}

// RecordStatus records differences of the previous status to the provided
// status into the Timeline.
func (t *SQLiteTimeline) RecordStatus(ctx context.Context, status *pb.SystemStatus) error {
	cluster := parseSystemStatus(status)
	events := t.lastStatus.diffCluster(cluster)
	if len(events) == 0 {
		return nil
	}

	tx, err := t.database.BeginTx(ctx, nil)
	if err != nil {
		return trace.Wrap(err)
	}

	if err := t.insertEvents(tx, events); err != nil {
		return trace.Wrap(err, "failed to insert events.")
	}

	if err := t.evictEvents(tx); err != nil {
		return trace.Wrap(err, "failed to evict old events.")
	}

	if err := tx.Commit(); err != nil {
		return trace.Wrap(err)
	}

	t.lastStatus = cluster
	return nil
}

// GetEvents returns the current timeline.
func (t *SQLiteTimeline) GetEvents() (events []*Event, err error) {
	rows, err := t.database.Query(selectAllFromEvent)
	if err != nil {
		return nil, trace.Wrap(err)
	}

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
			rows.Close()
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
func (t *SQLiteTimeline) insertEvents(tx *sql.Tx, events []*Event) error {

	// prepare bulk insert statement
	valueStrings := make([]string, 0, len(events))
	valueArgs := make([]interface{}, 0, len(events)*eventSize)
	for _, event := range events {
		valueStrings = append(valueStrings, eventValueString)
		valueArgs = append(valueArgs, event.timestamp)
		valueArgs = append(valueArgs, event.eventType)
		valueArgs = append(valueArgs, event.metadata["node"])
		valueArgs = append(valueArgs, event.metadata["probe"])
		valueArgs = append(valueArgs, event.metadata["old"])
		valueArgs = append(valueArgs, event.metadata["new"])
	}
	insertEvents := fmt.Sprintf(insertIntoEvent, strings.Join(valueStrings, ","))

	if _, err := t.database.Exec(insertEvents, valueArgs...); err != nil {
		tx.Rollback()
		return trace.Wrap(err)
	}

	return nil
}

// evictEvents deletes oldest events if the timeline is larger than its max
// size.
func (t *SQLiteTimeline) evictEvents(tx *sql.Tx) error {
	if _, err := t.database.Exec(deleteOldFromEvent, t.size); err != nil {
		tx.Rollback()
		return trace.Wrap(err)
	}
	return nil
}

// createTableEvent is sql statement to create an `event` table.
const createTableEvent = `
CREATE TABLE event (
	id INTEGER PRIMARY KEY,
	timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
	type TEXT,
	node TEXT,
	probe TEXT,
	old TEXT,
	new TEXT
)
`

// insertIntoEvent is sql statement to insert entry into `event` table. Used for
// batch insert statement.
const insertIntoEvent = `
INSERT INTO event (
	timestamp,
	type,
	node,
	probe,
	old,
	new
) VALUES %s
`

// eventSize specifies the number of fields contained in an Event entry.
const eventSize = 6

// eventValueString specifies the insert values tring for an Event.
const eventValueString = "(?, ?, ?, ?, ?, ?)"

// selectAllFromEvent is sql query to select all entries from `event` table.
const selectAllFromEvent = `SELECT * FROM event`

// deleteOldFromEvent is sql statement to delete entries from `event` table when full.
const deleteOldFromEvent = `DELETE FROM event WHERE id IN (SELECT id FROM event ORDER BY id DESC LIMIT -1 OFFSET ?);`
