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

// Package sqlite provides Timeline implementation backed by a SQLite database.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/lib/history"

	"github.com/gravitational/trace"
	"github.com/jmoiron/sqlx"
	"github.com/jonboulle/clockwork"
	"github.com/mattn/go-sqlite3" // initialize sqlite3
	log "github.com/sirupsen/logrus"
)

// Timeline represents a timeline of cluster status events. The timeline
// can hold a specified amount of events and uses a FIFO eviction policy.
// Timeline events are stored in a local sqlite database.
//
// Implements history.Timeline
type Timeline struct {
	// Config contains timeline configuration.
	Config Config
	// size specifies the current number of events stored in the timeline.
	size int
	// database points to underlying sqlite database.
	database *sqlx.DB
	// lastStatus holds the last recorded status.
	lastStatus *pb.NodeStatus
	// mu locks timeline access.
	mu sync.Mutex
}

// Config defines Timeline configuration.
type Config struct {
	// DBPath specifies the database location.
	DBPath string
	// Capacity specifies the max number of events that can be stored in the timeline.
	Capacity int
	// Clock will be used to record event timestamps.
	Clock clockwork.Clock
}

// NewTimeline initializes and returns a new Timeline with the
// specified configuration.
func NewTimeline(ctx context.Context, config Config) (*Timeline, error) {
	database, err := initSQLite(ctx, config.DBPath)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	size, err := getSize(ctx, database)
	if err != nil {
		database.Close()
		return nil, trace.Wrap(err)
	}

	return &Timeline{
		Config:   config,
		size:     size,
		database: database,
		// TODO: store and recover lastStatus in case satellite agent restarts.
		lastStatus: nil,
	}, nil
}

// initSQLite initializes connection to database provided by dbPath and
// initializes `events` table.
func initSQLite(ctx context.Context, dbPath string) (*sqlx.DB, error) {
	database, err := sqlx.ConnectContext(ctx, "sqlite3", dbPath)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	if _, err := database.ExecContext(ctx, createTableEvents); err != nil {
		return nil, trace.Wrap(err)
	}

	return database, nil
}

// getSize returns the current number of rows stored in the provided database.
func getSize(ctx context.Context, database *sqlx.DB) (size int, err error) {
	row := database.QueryRowContext(ctx, "SELECT COUNT(*) FROM events")
	if err := row.Scan(&size); err != nil {
		return -1, trace.Wrap(err)
	}
	return size, nil
}

// RecordStatus records the differences between the previously stored status and
// the provided status.
func (t *Timeline) RecordStatus(ctx context.Context, status *pb.NodeStatus) (err error) {
	t.mu.Lock()

	events := history.DiffNode(t.Config.Clock, t.lastStatus, status)
	if len(events) == 0 {
		t.mu.Unlock()
		return nil
	}

	t.size = t.size + len(events)
	if t.size > t.Config.Capacity {
		t.size = t.Config.Capacity
	}
	t.lastStatus = status

	t.mu.Unlock()

	if err = t.insertEvents(ctx, events); err != nil {
		return trace.Wrap(err, "failed to insert events")
	}

	// TODO: cannot update use an eviction policy based on size of timeline if
	// timeline needs to be a CRDT. Will replace eviction policy with a time
	// based policy.
	if t.size == t.Config.Capacity {
		if err = t.evictEvents(ctx); err != nil {
			return trace.Wrap(err, "failed to evict old events")
		}
	}

	return nil
}

// RecordTimeline merges the provided events into the current timeline.
// Duplicate events will be ignored.
func (t *Timeline) RecordTimeline(ctx context.Context, events []*pb.TimelineEvent) (err error) {
	if len(events) == 0 {
		return nil
	}

	t.mu.Lock()
	t.size = t.size + len(events)
	if t.size > t.Config.Capacity {
		t.size = t.Config.Capacity
	}
	t.mu.Unlock()

	if err = t.insertEvents(ctx, events); err != nil {
		return trace.Wrap(err, "failed to insert events")
	}

	// TODO: cannot use an eviction policy based on size of timeline if
	// timeline needs to be a CRDT. Will replace eviction policy with a time
	// based policy.
	if t.size == t.Config.Capacity {
		if err = t.evictEvents(ctx); err != nil {
			return trace.Wrap(err, "failed to evict old events")
		}
	}

	return nil
}

// GetEvents returns a filtered list of events based on the provided params.
func (t *Timeline) GetEvents(ctx context.Context, params map[string]string) (events []*pb.TimelineEvent, err error) {
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
// TODO: Batch inserts. Not expected to handle a large number of inserts, so
// optimization here is not a high priority.
func (t *Timeline) insertEvents(ctx context.Context, events []*pb.TimelineEvent) (err error) {
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
			// Unique constraint error indicates duplicate row.
			// Just ignore duplicates and continue.
			if sqliteErr, ok := err.(sqlite3.Error); ok {
				if sqliteErr.ExtendedCode == sqlite3.ErrConstraintUnique {
					continue
				}
			}
			return trace.Wrap(err)
		}
	}

	if err = tx.Commit(); err != nil {
		return trace.Wrap(err)
	}
	return nil
}

// evictEvents deletes oldest events if the timeline is larger than its max
// capacity.
func (t *Timeline) evictEvents(ctx context.Context) (err error) {
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

	if _, err := tx.ExecContext(ctx, deleteOldFromEvents, t.Config.Capacity); err != nil {
		return trace.Wrap(err)
	}

	if err = tx.Commit(); err != nil {
		return trace.Wrap(err)
	}
	return nil
}

// prepareQuery prepares a query string and a list of arguments constructed from
// the provided params.
func prepareQuery(params map[string]string) (query string, args []interface{}) {
	var sb strings.Builder
	index := 0

	// Need to filter params beforehand to check if WHERE clause is needed.
	filterParams(params)

	sb.WriteString("SELECT * FROM events ")
	if len(params) == 0 {
		sb.WriteString("ORDER BY timestamp DESC ")
		return sb.String(), args
	}
	sb.WriteString("WHERE ")

	for key, val := range params {
		sb.WriteString(fmt.Sprintf("%s = ? ", key))
		args = append(args, val)
		if index < len(params)-1 {
			sb.WriteString("AND ")
		}
		index++
	}

	sb.WriteString("ORDER BY timestamp DESC ")
	return sb.String(), args
}

// filterParams will filter out unknown query parameters.
func filterParams(params map[string]string) (filtered map[string]string) {
	filtered = make(map[string]string)
	var fields = []string{"type", "node", "probe", "old", "new"}
	for _, key := range fields {
		if val, ok := params[key]; ok {
			filtered[key] = val
		}
	}
	return filtered
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
		return row, trace.BadParameter("unknown event type %T", t)
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
		return history.NewClusterRecovered(e.Timestamp), nil
	case clusterDegradedType:
		return history.NewClusterDegraded(e.Timestamp), nil
	case nodeAddedType:
		return history.NewNodeAdded(e.Timestamp, e.Node.String), nil
	case nodeRemovedType:
		return history.NewNodeRemoved(e.Timestamp, e.Node.String), nil
	case nodeRecoveredType:
		return history.NewNodeRecovered(e.Timestamp, e.Node.String), nil
	case nodeDegradedType:
		return history.NewNodeDegraded(e.Timestamp, e.Node.String), nil
	case probeSucceededType:
		return history.NewProbeSucceeded(e.Timestamp, e.Node.String, e.Probe.String), nil
	case probeFailedType:
		return history.NewProbeFailed(e.Timestamp, e.Node.String, e.Probe.String), nil
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
// Rows must be unique, excluding id.
// TODO: might not need oldState/newState.
const createTableEvents = `
CREATE TABLE IF NOT EXISTS events (
	id INTEGER PRIMARY KEY,
	timestamp DATETIME DEFAULT CURRENT_TIMESTAMP NOT NULL,
	type TEXT NOT NULL,
	node TEXT,
	probe TEXT,
	oldState TEXT,
	newState TEXT,
	UNIQUE(timestamp, type, node, probe, oldState, newState)
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
