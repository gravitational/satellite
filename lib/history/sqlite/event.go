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
	"database/sql"
	"time"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/lib/history"

	"github.com/gravitational/trace"
	"github.com/jmoiron/sqlx"
)

// sqlEvent defines an sql event row.
type sqlEvent struct {
	// ID specifies sqlite id.
	ID int `db:"id"`
	// Timestamp specifies event timestamp.
	Timestamp time.Time `db:"timestamp"`
	// EventType specifies event type.
	EventType string `db:"type"`
	// Node specifies name of node.
	Node sql.NullString `db:"node"`
	// Probe specifies name of probe.
	Probe sql.NullString `db:"probe"`
	// Old specifies previous probe state.
	Old sql.NullString `db:"oldState"`
	// New specifies new probe state.
	New sql.NullString `db:"newState"`
}

// ProtoBuf returns the sql event row as a protobuf message.
func (r sqlEvent) ProtoBuf() (event *pb.TimelineEvent, err error) {
	switch history.EventType(r.EventType) {
	case history.ClusterDegraded:
		return pb.NewClusterDegraded(r.Timestamp), nil
	case history.ClusterRecovered:
		return pb.NewClusterRecovered(r.Timestamp), nil
	case history.NodeAdded:
		return pb.NewNodeAdded(r.Timestamp, r.Node.String), nil
	case history.NodeRemoved:
		return pb.NewNodeRemoved(r.Timestamp, r.Node.String), nil
	case history.NodeDegraded:
		return pb.NewNodeDegraded(r.Timestamp, r.Node.String), nil
	case history.NodeRecovered:
		return pb.NewNodeRecovered(r.Timestamp, r.Node.String), nil
	case history.ProbeFailed:
		return pb.NewProbeFailed(r.Timestamp, r.Node.String, r.Probe.String), nil
	case history.ProbeSucceeded:
		return pb.NewProbeSucceeded(r.Timestamp, r.Node.String, r.Probe.String), nil
	case history.LeaderElected:
		return pb.NewLeaderElected(r.Timestamp, r.Node.String), nil
	default:
		return event, trace.BadParameter("unknown event type %s", r.EventType)
	}
}

// sqlExecer executes sql statements.
type sqlExecer struct {
	db *sqlx.DB
}

// newSQLExecer constructs a new sqlExecer with the provided database.
func newSQLExecer(db *sqlx.DB) *sqlExecer {
	return &sqlExecer{db: db}
}

// Exec executes the provided stmt with the provided args.
func (r *sqlExecer) Exec(ctx context.Context, stmt string, args ...interface{}) error {
	_, err := r.db.ExecContext(ctx, stmt, args...)
	return trace.Wrap(err)
}

// newDataInserter constructs a new DataInserter from the provided event.
func newDataInserter(event *pb.TimelineEvent) (row history.DataInserter, err error) {
	switch t := event.GetData().(type) {
	case *pb.TimelineEvent_ClusterDegraded:
		return &clusterDegraded{TimelineEvent: event}, nil
	case *pb.TimelineEvent_ClusterRecovered:
		return &clusterRecovered{TimelineEvent: event}, nil
	case *pb.TimelineEvent_NodeAdded:
		return &nodeAdded{TimelineEvent: event}, nil
	case *pb.TimelineEvent_NodeRemoved:
		return &nodeRemoved{TimelineEvent: event}, nil
	case *pb.TimelineEvent_NodeRecovered:
		return &nodeRecovered{TimelineEvent: event}, nil
	case *pb.TimelineEvent_NodeDegraded:
		return &nodeDegraded{TimelineEvent: event}, nil
	case *pb.TimelineEvent_ProbeSucceeded:
		return &probeSucceeded{TimelineEvent: event}, nil
	case *pb.TimelineEvent_ProbeFailed:
		return &probeFailed{TimelineEvent: event}, nil
	case *pb.TimelineEvent_LeaderElected:
		return &leaderElected{TimelineEvent: event}, nil
	default:
		return row, trace.BadParameter("unknown event type %T", t)
	}
}

// clusterDegraded represents a cluster degraded event.
//
// Implements history.DataInserter.
type clusterDegraded struct {
	*pb.TimelineEvent
}

func (r *clusterDegraded) Insert(ctx context.Context, execer history.Execer) error {
	const insertStmt = "INSERT INTO events (timestamp, type) VALUES (?,?)"
	args := []interface{}{r.GetTimestamp().ToTime(), history.ClusterDegraded}
	return trace.Wrap(execer.Exec(ctx, insertStmt, args...))
}

// clusterRecovered represents a cluster recovered event.
//
// Implements history.DataInserter.
type clusterRecovered struct {
	*pb.TimelineEvent
}

func (r *clusterRecovered) Insert(ctx context.Context, execer history.Execer) error {
	const insertStmt = "INSERT INTO events (timestamp, type) VALUES (?,?)"
	args := []interface{}{r.GetTimestamp().ToTime(), history.ClusterRecovered}
	return trace.Wrap(execer.Exec(ctx, insertStmt, args...))
}

// nodeAdded represents a node added event.
//
// Implements history.DataInserter.
type nodeAdded struct {
	*pb.TimelineEvent
}

func (r *nodeAdded) Insert(ctx context.Context, execer history.Execer) error {
	const insertStmt = "INSERT INTO events (timestamp, type, node) VALUES (?,?,?)"
	data, ok := r.GetData().(*pb.TimelineEvent_NodeAdded)
	if !ok {
		return trace.BadParameter("expected %T, got %T", data, r.GetData())
	}
	event := data.NodeAdded
	args := []interface{}{r.GetTimestamp().ToTime(), history.NodeAdded, event.GetNode()}
	return trace.Wrap(execer.Exec(ctx, insertStmt, args...))
}

// nodeRemoved represents a node removed event.
//
// Implements history.DataInserter.
type nodeRemoved struct {
	*pb.TimelineEvent
}

func (r *nodeRemoved) Insert(ctx context.Context, execer history.Execer) error {
	const insertStmt = "INSERT INTO events (timestamp, type, node) VALUES (?,?,?)"
	data, ok := r.GetData().(*pb.TimelineEvent_NodeRemoved)
	if !ok {
		return trace.BadParameter("expected %T, got %T", data, r.GetData())
	}
	event := data.NodeRemoved
	args := []interface{}{r.GetTimestamp().ToTime(), history.NodeRemoved, event.GetNode()}
	return trace.Wrap(execer.Exec(ctx, insertStmt, args...))
}

// nodeDegraded represents a node degraded event.
//
// Implements history.DataInserter.
type nodeDegraded struct {
	*pb.TimelineEvent
}

func (r *nodeDegraded) Insert(ctx context.Context, execer history.Execer) error {
	const insertStmt = "INSERT INTO events (timestamp, type, node) VALUES (?,?,?)"
	data, ok := r.GetData().(*pb.TimelineEvent_NodeDegraded)
	if !ok {
		return trace.BadParameter("expected %T, got %T", data, r.GetData())
	}
	event := data.NodeDegraded
	args := []interface{}{r.GetTimestamp().ToTime(), history.NodeDegraded, event.GetNode()}
	return trace.Wrap(execer.Exec(ctx, insertStmt, args...))
}

// nodeRecovered represents a node recovered event.
//
// Implements history.DataInserter.
type nodeRecovered struct {
	*pb.TimelineEvent
}

func (r *nodeRecovered) Insert(ctx context.Context, execer history.Execer) error {
	const insertStmt = "INSERT INTO events (timestamp, type, node) VALUES (?,?,?)"
	data, ok := r.GetData().(*pb.TimelineEvent_NodeRecovered)
	if !ok {
		return trace.BadParameter("expected %T, got %T", data, r.GetData())
	}
	event := data.NodeRecovered
	args := []interface{}{r.GetTimestamp().ToTime(), history.NodeRecovered, event.GetNode()}
	return trace.Wrap(execer.Exec(ctx, insertStmt, args...))
}

// probeFailed represents a probe failed event.
//
// Implements history.DataInserter.
type probeFailed struct {
	*pb.TimelineEvent
}

func (r *probeFailed) Insert(ctx context.Context, execer history.Execer) error {
	const insertStmt = "INSERT INTO events (timestamp, type, node, probe) VALUES (?,?,?,?)"
	data, ok := r.GetData().(*pb.TimelineEvent_ProbeFailed)
	if !ok {
		return trace.BadParameter("expected %T, got %T", data, r.GetData())
	}
	event := data.ProbeFailed
	args := []interface{}{r.GetTimestamp().ToTime(), history.ProbeFailed, event.GetNode(), event.GetProbe()}
	return trace.Wrap(execer.Exec(ctx, insertStmt, args...))
}

// probeSucceeded represents a probe succeeded event.
//
// Implements history.DataInserter.
type probeSucceeded struct {
	*pb.TimelineEvent
}

func (r *probeSucceeded) Insert(ctx context.Context, execer history.Execer) error {
	const insertStmt = "INSERT INTO events (timestamp, type, node, probe) VALUES (?,?,?,?)"
	data, ok := r.GetData().(*pb.TimelineEvent_ProbeSucceeded)
	if !ok {
		return trace.BadParameter("expected %T, got %T", data, r.GetData())
	}
	event := data.ProbeSucceeded
	args := []interface{}{r.GetTimestamp().ToTime(), history.ProbeSucceeded, event.GetNode(), event.GetProbe()}
	return trace.Wrap(execer.Exec(ctx, insertStmt, args...))
}

// leaderElected represents a leader elected event.
//
// Implements history.DataInserter.
type leaderElected struct {
	*pb.TimelineEvent
}

func (r *leaderElected) Insert(ctx context.Context, execer history.Execer) error {
	const insertStmt = "INSERT INTO events (timestamp, type, node) VALUES (?,?,?)"
	data, ok := r.GetData().(*pb.TimelineEvent_LeaderElected)
	if !ok {
		return trace.BadParameter("expected %T, got %T", data, r.GetData())
	}
	event := data.LeaderElected
	args := []interface{}{r.GetTimestamp().ToTime(), history.LeaderElected, event.GetNode()}
	return trace.Wrap(execer.Exec(ctx, insertStmt, args...))
}
