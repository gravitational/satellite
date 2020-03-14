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

package memory

import (
	"context"
	"strings"
	"time"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/lib/history"

	"github.com/gravitational/trace"
)

// memEvent defines an event in a comparable struct.
// Used when filtering duplicate events.
type memEvent struct {
	timestamp time.Time
	eventType history.EventType
	node      string
	probe     string
	old       string
	new       string
}

// ProtoBuf returns the event as a protobuf message.
func (r memEvent) ProtoBuf() (event *pb.TimelineEvent, err error) {
	switch r.eventType {
	case history.ClusterDegraded:
		return pb.NewClusterDegraded(r.timestamp), nil
	case history.ClusterHealthy:
		return pb.NewClusterHealthy(r.timestamp), nil
	case history.NodeAdded:
		return pb.NewNodeAdded(r.timestamp, r.node), nil
	case history.NodeRemoved:
		return pb.NewNodeRemoved(r.timestamp, r.node), nil
	case history.NodeDegraded:
		return pb.NewNodeDegraded(r.timestamp, r.node), nil
	case history.NodeHealthy:
		return pb.NewNodeHealthy(r.timestamp, r.node), nil
	case history.ProbeFailed:
		return pb.NewProbeFailed(r.timestamp, r.node, r.probe), nil
	case history.ProbeSucceeded:
		return pb.NewProbeSucceeded(r.timestamp, r.node, r.probe), nil
	case history.LeaderElected:
		return pb.NewLeaderElected(r.timestamp, r.node), nil
	default:
		return event, trace.BadParameter("unknown event type %s", r.eventType)
	}
}

// memExecer inserts events into memory.
//
// Implements history.Execer
type memExecer struct {
	events *[]memEvent
}

// newMemExecer constructs a new memExecer with the provided array.
func newMemExecer(events *[]memEvent) *memExecer {
	return &memExecer{events: events}
}

// Exec executes the provided stmt with the provided args.
// stmt is a comma separated list of tokens e.g. "timestamp,type,node".
func (r *memExecer) Exec(_ context.Context, stmt string, args ...interface{}) error {
	if len(stmt) == 0 {
		return trace.BadParameter("received empty statement")
	}

	tokens := strings.Split(stmt, ",")
	if len(tokens) != len(args) {
		return trace.BadParameter("expected %d args, received %d", len(tokens), len(args))
	}

	var event memEvent
	var ok bool

	for i, token := range tokens {
		switch token {
		case "timestamp":
			if event.timestamp, ok = args[i].(time.Time); !ok {
				return trace.BadParameter("expected time.Time, received %T for arg %d", args[i], i)
			}
		case "type":
			if event.eventType, ok = args[i].(history.EventType); !ok {
				return trace.BadParameter("expected history.EventType, received %T for arg %d", args[i], i)
			}
		case "node":
			if event.node, ok = args[i].(string); !ok {
				return trace.BadParameter("expected string, received %T for arg %d", args[i], i)
			}
		case "probe":
			if event.probe, ok = args[i].(string); !ok {
				return trace.BadParameter("expected string, received %T for arg %d", args[i], i)
			}
		case "old":
			if event.old, ok = args[i].(string); !ok {
				return trace.BadParameter("expected string, received %T for arg %d", args[i], i)
			}
		case "new":
			if event.new, ok = args[i].(string); !ok {
				return trace.BadParameter("expected string, received %T for arg %d", args[i], i)
			}
		default:
			return trace.BadParameter("received unknown token %s", token)
		}
	}

	*r.events = append(*r.events, event)
	return nil
}

// newDataInserter returns the event as a history.DataInserter.
func newDataInserter(event *pb.TimelineEvent) (row history.DataInserter, err error) {
	switch t := event.GetData().(type) {
	case *pb.TimelineEvent_ClusterDegraded:
		return &clusterDegraded{TimelineEvent: event}, nil
	case *pb.TimelineEvent_ClusterHealthy:
		return &clusterHealthy{TimelineEvent: event}, nil
	case *pb.TimelineEvent_NodeAdded:
		return &nodeAdded{TimelineEvent: event}, nil
	case *pb.TimelineEvent_NodeRemoved:
		return &nodeRemoved{TimelineEvent: event}, nil
	case *pb.TimelineEvent_NodeHealthy:
		return &nodeHealthy{TimelineEvent: event}, nil
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
	const insertStmt = "timestamp,type"
	args := []interface{}{r.GetTimestamp().ToTime(), history.ClusterDegraded}
	return trace.Wrap(execer.Exec(ctx, insertStmt, args...))
}

// clusterHealthy represents a cluster healthy event.
//
// Implements history.DataInserter.
type clusterHealthy struct {
	*pb.TimelineEvent
}

func (r *clusterHealthy) Insert(ctx context.Context, execer history.Execer) error {
	const insertStmt = "timestamp,type"
	args := []interface{}{r.GetTimestamp().ToTime(), history.ClusterHealthy}
	return trace.Wrap(execer.Exec(ctx, insertStmt, args...))
}

// nodeAdded represents a node added event.
//
// Implements history.DataInserter.
type nodeAdded struct {
	*pb.TimelineEvent
}

func (r *nodeAdded) Insert(ctx context.Context, execer history.Execer) error {
	const insertStmt = "timestamp,type,node"
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
	const insertStmt = "timestamp,type,node"
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
	const insertStmt = "timestamp,type,node"
	data, ok := r.GetData().(*pb.TimelineEvent_NodeDegraded)
	if !ok {
		return trace.BadParameter("expected %T, got %T", data, r.GetData())
	}
	event := data.NodeDegraded
	args := []interface{}{r.GetTimestamp().ToTime(), history.NodeDegraded, event.GetNode()}
	return trace.Wrap(execer.Exec(ctx, insertStmt, args...))
}

// nodeHealthy represents a node healthy event.
//
// Implements history.DataInserter.
type nodeHealthy struct {
	*pb.TimelineEvent
}

func (r *nodeHealthy) Insert(ctx context.Context, execer history.Execer) error {
	const insertStmt = "timestamp,type,node"
	data, ok := r.GetData().(*pb.TimelineEvent_NodeHealthy)
	if !ok {
		return trace.BadParameter("expected %T, got %T", data, r.GetData())
	}
	event := data.NodeHealthy
	args := []interface{}{r.GetTimestamp().ToTime(), history.NodeHealthy, event.GetNode()}
	return trace.Wrap(execer.Exec(ctx, insertStmt, args...))
}

// probeFailed represents a probe failed event.
//
// Implements history.DataInserter.
type probeFailed struct {
	*pb.TimelineEvent
}

func (r *probeFailed) Insert(ctx context.Context, execer history.Execer) error {
	const insertStmt = "timestamp,type,node,probe"
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
	const insertStmt = "timestamp,type,node,probe"
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
	const insertStmt = "timestamp,type,node"
	data, ok := r.GetData().(*pb.TimelineEvent_LeaderElected)
	if !ok {
		return trace.BadParameter("expected %T, got %T", data, r.GetData())
	}
	event := data.LeaderElected
	args := []interface{}{r.GetTimestamp().ToTime(), history.LeaderElected, event.GetNode()}
	return trace.Wrap(execer.Exec(ctx, insertStmt, args...))
}
