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
	"database/sql"
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
	// size specifies the max size of the timeline.
	size int
	// database points to underlying sqlite database.
	database *sql.DB
	// lastStatus holds the last recorded cluster status.
	lastStatus *Cluster
}

// NewSQLiteTimeline initializes and returns a new SQLiteTimeline with the
// specifies size.
func NewSQLiteTimeline(database *sql.DB, size int) (Timeline, error) {
	if err := initEventTable(database); err != nil {
		return nil, trace.Wrap(err)
	}

	return &SQLiteTimeline{
		size:       size,
		database:   database,
		lastStatus: &Cluster{},
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
func (t *SQLiteTimeline) RecordStatus(status *pb.SystemStatus) {
	cluster := parseSystemStatus(status)
	events := t.lastStatus.diffCluster(cluster)
	if len(events) == 0 {
		return
	}

	for _, event := range events {
		t.addEvent(event)
	}

	if err := t.evictEvent(); err != nil {
		log.WithError(err).Warn("Failed to evict old events.")
	}

	t.lastStatus = cluster
}

// GetEvents returns the current timeline.
func (t *SQLiteTimeline) GetEvents() []*Event {
	events := []*Event{}

	rows, err := t.database.Query(selectAllFromEvent)
	if err != nil {
		log.WithError(err).Warn("Failed to query event.")
		return events
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
			log.WithError(err).Warn("Failed to query event.")
			continue
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
			log.Warnf("Unknown event: %v\n", eventType)
		}

		events = append(events, event)
	}

	return events
}

// addEvent appends the provided event to the timeline.
func (t *SQLiteTimeline) addEvent(event *Event) error {
	insertEvent, err := t.database.Prepare(insertIntoEvent)
	if err != nil {
		return trace.Wrap(err)
	}

	timestamp := event.timestamp
	eventType := event.eventType
	node := event.metadata["node"]
	probe := event.metadata["probe"]
	old := event.metadata["old"]
	new := event.metadata["new"]

	if _, err = insertEvent.Exec(timestamp, eventType, node, probe, old, new); err != nil {
		return trace.Wrap(err)
	}

	return nil
}

// evictEvent deletes oldest events if the timeline is larger than its max
// size.
func (t *SQLiteTimeline) evictEvent() error {
	deleteEvents, err := t.database.Prepare(deleteOldFromEvent)
	if err != nil {
		return trace.Wrap(err)
	}

	if _, err := deleteEvents.Exec(t.size); err != nil {
		return trace.Wrap(err)
	}

	return nil
}

// createTableEvent is sql query to create an `event` table.
var createTableEvent = `
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

// insertIntoEvent is sql query to insert entry into `event` table.
var insertIntoEvent = `
INSERT INTO event (
	timestamp,
	type,
	node,
	probe,
	old,
	new
) VALUES (?, ?, ?, ?, ?, ?)
`

// selectAllFromEvent is sql query to select all entries from `event` table.
var selectAllFromEvent = `SELECT * FROM event`

// deleteOldFromEvent is sql query to delete entries from `event` table when full.
var deleteOldFromEvent = `DELETE FROM event WHERE id IN (SELECT id FROM event ORDER BY id DESC LIMIT -1 OFFSET ?);`
