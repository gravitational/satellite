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
	"fmt"
	"strings"
	"sync"
	"time"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/lib/history"

	"github.com/gravitational/trace"
	"github.com/jmoiron/sqlx"
	"github.com/jonboulle/clockwork" // initialize sqlite3
	log "github.com/sirupsen/logrus"
)

// Timeline represents a timeline of status events.
// Timeline events are stored in a local sqlite database.
// The timeline will retain events for a specified duration and then deleted.
//
// Implements history.Timeline
type Timeline struct {
	// Config contains timeline configuration.
	config Config
	// database points to underlying sqlite database.
	database *sqlx.DB
	// lastStatus holds the last recorded status.
	lastStatus *pb.NodeStatus
	// statusMutex locks access to the lastStatus field.
	statusMutex sync.Mutex
}

// Config defines Timeline configuration.
type Config struct {
	// DBPath specifies the database location.
	DBPath string
	// RetentionDuration specifies the duration to store events.
	RetentionDuration time.Duration
	// Clock will be used to record event timestamps.
	Clock clockwork.Clock
}

// NewTimeline initializes and returns a new Timeline with the
// specified configuration.
func NewTimeline(ctx context.Context, config Config) (*Timeline, error) {
	timeline := &Timeline{
		config: config,
	}

	if err := timeline.initSQLite(ctx); err != nil {
		return nil, trace.Wrap(err, "failed to init sqlite")
	}

	// if err := timeline.initPrevStatus(ctx); err != nil {
	// 	return nil, trace.Wrap(err, "failed to init previous status")
	// }

	// TODO: should this be called in constructor method?
	// Let caller run event eviction loop?
	// Create separate init function?
	go timeline.eventEvictionLoop()

	return timeline, nil
}

// initSQLite initializes connection to database and initializes `events` table.
func (t *Timeline) initSQLite(ctx context.Context) error {
	database, err := sqlx.ConnectContext(ctx, "sqlite3", t.config.DBPath)
	if err != nil {
		return trace.Wrap(err)
	}

	if _, err := database.ExecContext(ctx, createTableEvents); err != nil {
		return trace.Wrap(err)
	}

	t.database = database
	return nil
}

// initPrevStatus initializes the previously record status in case satellite
// is restarted.
func (t *Timeline) initPrevStatus(ctx context.Context) error {
	// TODO
	return trace.NotImplemented("not implemented")
}

// eventEvictionLoop periodically evicts old events to free up storage.
func (t *Timeline) eventEvictionLoop() {
	ticker := t.config.Clock.NewTicker(evictionFrequency)
	for {
		<-ticker.Chan()

		// All events before this time will be deleted.
		retentionCutOff := t.config.Clock.Now().Add(-(t.config.RetentionDuration))

		ctx, cancel := context.WithTimeout(context.Background(), evictionTimeout)
		if err := t.evictEvents(ctx, retentionCutOff); err != nil {
			cancel()
			log.WithError(err).Warnf("Error evicting expired events.")
		}
		cancel()
	}
}

// RecordStatus records the differences between the previously stored status and
// the provided status.
func (t *Timeline) RecordStatus(ctx context.Context, status *pb.NodeStatus) (err error) {
	t.statusMutex.Lock()

	events := history.DiffNode(t.config.Clock, t.lastStatus, status)
	if len(events) == 0 {
		t.statusMutex.Unlock()
		return nil
	}
	t.lastStatus = status

	t.statusMutex.Unlock()

	if err = t.insertEvents(ctx, events); err != nil {
		return trace.Wrap(err)
	}

	return nil
}

// RecordTimeline merges the provided events into the current timeline.
// Duplicate events will be ignored.
func (t *Timeline) RecordTimeline(ctx context.Context, events []*pb.TimelineEvent) (err error) {
	if len(events) == 0 {
		return nil
	}

	// All events before this time will be deleted.
	retentionCutOff := t.config.Clock.Now().Add(-(t.config.RetentionDuration))

	// Filter out expired events.
	filtered := []*pb.TimelineEvent{}
	for _, event := range events {
		if event.GetTimestamp().ToTime().After(retentionCutOff) {
			filtered = append(filtered, event)
		}
	}

	if err = t.insertEvents(ctx, filtered); err != nil {
		return trace.Wrap(err)
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

	var row sqlEvent
	for rows.Next() {
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

		_, err = tx.ExecContext(ctx, insertIntoEvents, row.toArgs()...)
		// Unique constraint error indicates duplicate row.
		// Just ignore duplicates and continue.
		if isErrConstraintUnique(err) {
			continue
		}
		if err != nil {
			return trace.Wrap(err)
		}
	}

	if err = tx.Commit(); err != nil {
		return trace.Wrap(err)
	}
	return nil
}

// evictEvents deletes events that have outlived the timeline retention
// duration. All events before this cut off time will be deleted.
func (t *Timeline) evictEvents(ctx context.Context, retentionCutOff time.Time) (err error) {
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

	if _, err := tx.ExecContext(ctx, deleteOldFromEvents, retentionCutOff); err != nil {
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

	// TODO: checkout text/template package for cleaner code
	sb.WriteString("SELECT * FROM events ")
	if len(params) == 0 {
		sb.WriteString("ORDER BY timestamp ASC ")
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

	sb.WriteString("ORDER BY timestamp ASC ")
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
