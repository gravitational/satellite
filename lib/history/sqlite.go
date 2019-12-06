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

	"github.com/jonboulle/clockwork"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"

	"github.com/gravitational/trace"
	"github.com/jmoiron/sqlx"
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
	database *sqlx.DB
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
func initSQLite(dbPath string) (*sqlx.DB, error) {
	// TODO: Store sql db connection timeout as const
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	database, err := sqlx.ConnectContext(ctx, "sqlite3", dbPath)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	if _, err := database.ExecContext(ctx, createTableEvents); err != nil {
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

// GetEvents returns a filtered list of events based on the provided params.
func (t *SQLiteTimeline) GetEvents(ctx context.Context, params map[string]string) (events []*pb.TimelineEvent, err error) {
	query, args := prepareQuery(params)
	rows, err := t.database.QueryxContext(ctx, query, args...)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	defer func() {
		if err := rows.Close(); err != nil {
			log.WithError(err).Error("Failed to close sql rows.")
		}
	}()

	for rows.Next() {
		var row sqlEvent
		if err = rows.StructScan(&row); err != nil {
			return nil, trace.Wrap(err)
		}

		event, err := row.toProto()
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
func (t *SQLiteTimeline) insertEvents(ctx context.Context, tx *sql.Tx, events []*pb.TimelineEvent) error {
	for _, event := range events {
		row, err := newSQLEvent(event)
		if err != nil {
			return trace.Wrap(err)
		}

		args := []interface{}{
			row.Timestamp,
			row.EventType,
			row.Node.String,
			row.Probe.String,
			row.Old.String,
			row.New.String,
		}

		if _, err := tx.ExecContext(ctx, insertIntoEvents, args...); err != nil {
			return trace.Wrap(err)
		}
	}
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

// prepareQuery prepares a query string and a list of arguments constructed from
// the provided params.
func prepareQuery(params map[string]string) (query string, args []interface{}) {
	var fields = []string{"type", "node", "probe", "old", "new"}
	var sb strings.Builder
	index := 0

	sb.WriteString("SELECT * FROM EVENTS ")
	if len(params) == 0 {
		return sb.String(), args
	}
	sb.WriteString("WHERE ")

	for _, key := range fields {
		if val, ok := params[key]; ok {
			sb.WriteString(fmt.Sprintf("%s = ?", key))
			args = append(args, val)
		}
		if index < len(params)-1 {
			sb.WriteString("AND ")
		}
		index++
	}
	return sb.String(), args
}

// sqlEvent defines an sql event row.
type sqlEvent struct {
	ID        int            `db:"id"`
	Timestamp time.Time      `db:"timestamp"`
	EventType string         `db:"type"`
	Node      sql.NullString `db:"node"`
	Probe     sql.NullString `db:"probe"`
	Old       sql.NullString `db:"oldState"`
	New       sql.NullString `db:"newState"`
}

// newSQLEvent constructs a new sqlEvent from the provided TimelineEvent.
func newSQLEvent(event *pb.TimelineEvent) (row sqlEvent, err error) {
	row = sqlEvent{Timestamp: event.GetTimestamp().ToTime()}
	switch t := event.GetData().(type) {
	case *pb.TimelineEvent_ClusterRecovered:
		row.EventType = clusterRecoveredType
	case *pb.TimelineEvent_ClusterDegraded:
		row.EventType = clusterDegradedType
	case *pb.TimelineEvent_NodeAdded:
		e := event.GetNodeAdded()
		row.EventType = nodeAddedType
		row.Node = sql.NullString{String: e.GetNode(), Valid: true}
	case *pb.TimelineEvent_NodeRemoved:
		e := event.GetNodeRemoved()
		row.EventType = nodeRemovedType
		row.Node = sql.NullString{String: e.GetNode(), Valid: true}
	case *pb.TimelineEvent_NodeRecovered:
		e := event.GetNodeRecovered()
		row.EventType = nodeRecoveredType
		row.Node = sql.NullString{String: e.GetNode(), Valid: true}
	case *pb.TimelineEvent_NodeDegraded:
		e := event.GetNodeDegraded()
		row.EventType = nodeDegradedType
		row.Node = sql.NullString{String: e.GetNode(), Valid: true}
	case *pb.TimelineEvent_ProbeSucceeded:
		e := event.GetProbeSucceeded()
		row.EventType = probeSucceededType
		row.Node = sql.NullString{String: e.GetNode(), Valid: true}
		row.Probe = sql.NullString{String: e.GetProbe(), Valid: true}
	case *pb.TimelineEvent_ProbeFailed:
		e := event.GetProbeFailed()
		row.EventType = probeFailedType
		row.Node = sql.NullString{String: e.GetNode(), Valid: true}
		row.Probe = sql.NullString{String: e.GetProbe(), Valid: true}
	default:
		return row, trace.NotFound("unknown event type %v", t)
	}
	return row, nil
}

func (e *sqlEvent) toArgs() (args []interface{}) {
	return []interface{}{
		e.Timestamp,
		e.EventType,
		e.Node,
		e.Probe,
		e.New,
		e.Old,
	}
}

// toProto returns sqlEvent as a protobuf TimelineEvent.
func (e *sqlEvent) toProto() (*pb.TimelineEvent, error) {
	switch e.EventType {
	case clusterRecoveredType:
		return NewClusterRecovered(e.Timestamp), nil
	case clusterDegradedType:
		return NewClusterDegraded(e.Timestamp), nil
	case nodeAddedType:
		return NewNodeAdded(e.Timestamp, e.Node.String), nil
	case nodeRemovedType:
		return NewNodeRemoved(e.Timestamp, e.Node.String), nil
	case nodeRecoveredType:
		return NewNodeRecovered(e.Timestamp, e.Node.String), nil
	case nodeDegradedType:
		return NewNodeDegraded(e.Timestamp, e.Node.String), nil
	case probeSucceededType:
		return NewProbeSucceeded(e.Timestamp, e.Node.String, e.Probe.String), nil
	case probeFailedType:
		return NewProbeFailed(e.Timestamp, e.Node.String, e.Probe.String), nil
	default:
		return nil, trace.NotFound("unknown event type %v", e.EventType)
	}
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
) VALUES (?,?,?,?,?,?)
`

// selectAllFromEvents is sql query to select all entries from `events` table.
const selectAllFromEvents = `SELECT * FROM events`

// deleteOldFromEvents is sql statement to delete entries from `events` table when full.
const deleteOldFromEvents = `DELETE FROM events WHERE id IN (SELECT id FROM events ORDER BY id DESC LIMIT -1 OFFSET ?);`
