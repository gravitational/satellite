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
	"database/sql"
	"time"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/lib/history"

	"github.com/gravitational/trace"
)

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

// toArgs returns sqlEvent as a list of arguments.
func (e *sqlEvent) toArgs() (args []interface{}) {
	return []interface{}{
		e.Timestamp,
		e.EventType,
		e.Node.String,
		e.Probe.String,
		e.New.String,
		e.Old.String,
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
		return nil, trace.BadParameter("unknown event type %v", e.EventType)
	}
}
