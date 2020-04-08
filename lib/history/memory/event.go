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
// Format to use when storing events into MemTimeline.
//
// Implements history.ProtoBuffer
type memEvent struct {
	timestamp time.Time
	eventType history.EventType
	node      string
	probe     string
	old       string
	new       string
}

// ProtoBuf returns the event as a protobuf message.
func (r memEvent) ProtoBuf() (event *pb.TimelineEvent) {
	switch r.eventType {
	case history.ClusterDegraded:
		return pb.NewClusterDegraded(r.timestamp)
	case history.ClusterHealthy:
		return pb.NewClusterHealthy(r.timestamp)
	case history.NodeAdded:
		return pb.NewNodeAdded(r.timestamp, r.node)
	case history.NodeRemoved:
		return pb.NewNodeRemoved(r.timestamp, r.node)
	case history.NodeDegraded:
		return pb.NewNodeDegraded(r.timestamp, r.node)
	case history.NodeHealthy:
		return pb.NewNodeHealthy(r.timestamp, r.node)
	case history.ProbeFailed:
		return pb.NewProbeFailed(r.timestamp, r.node, r.probe)
	case history.ProbeSucceeded:
		return pb.NewProbeSucceeded(r.timestamp, r.node, r.probe)
	case history.LeaderElected:
		return pb.NewLeaderElected(r.timestamp, r.node)
	default:
		return pb.NewUnknownEvent(r.timestamp)
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
	switch data := event.GetData().(type) {
	case *pb.TimelineEvent_ClusterDegraded:
		return &clusterDegraded{ts: event.GetTimestamp()}, nil
	case *pb.TimelineEvent_ClusterHealthy:
		return &clusterHealthy{ts: event.GetTimestamp()}, nil
	case *pb.TimelineEvent_NodeAdded:
		return &nodeAdded{ts: event.GetTimestamp(), data: data.NodeAdded}, nil
	case *pb.TimelineEvent_NodeRemoved:
		return &nodeRemoved{ts: event.GetTimestamp(), data: data.NodeRemoved}, nil
	case *pb.TimelineEvent_NodeHealthy:
		return &nodeHealthy{ts: event.GetTimestamp(), data: data.NodeHealthy}, nil
	case *pb.TimelineEvent_NodeDegraded:
		return &nodeDegraded{ts: event.GetTimestamp(), data: data.NodeDegraded}, nil
	case *pb.TimelineEvent_ProbeSucceeded:
		return &probeSucceeded{ts: event.GetTimestamp(), data: data.ProbeSucceeded}, nil
	case *pb.TimelineEvent_ProbeFailed:
		return &probeFailed{ts: event.GetTimestamp(), data: data.ProbeFailed}, nil
	case *pb.TimelineEvent_LeaderElected:
		return &leaderElected{ts: event.GetTimestamp(), data: data.LeaderElected}, nil
	default:
		return row, trace.BadParameter("unknown event type %T", data)
	}
}

// clusterDegraded represents a cluster degraded event.
//
// Implements history.DataInserter.
type clusterDegraded struct {
	ts *pb.Timestamp
}

func (r *clusterDegraded) Insert(ctx context.Context, execer history.Execer) error {
	const insertStmt = "timestamp,type"
	return trace.Wrap(execer.Exec(ctx, insertStmt, r.ts.ToTime(), history.ClusterDegraded))
}

// clusterHealthy represents a cluster healthy event.
//
// Implements history.DataInserter.
type clusterHealthy struct {
	ts *pb.Timestamp
}

func (r *clusterHealthy) Insert(ctx context.Context, execer history.Execer) error {
	const insertStmt = "timestamp,type"
	return trace.Wrap(execer.Exec(ctx, insertStmt, r.ts.ToTime(), history.ClusterHealthy))
}

// nodeAdded represents a node added event.
//
// Implements history.DataInserter.
type nodeAdded struct {
	ts   *pb.Timestamp
	data *pb.NodeAdded
}

func (r *nodeAdded) Insert(ctx context.Context, execer history.Execer) error {
	const insertStmt = "timestamp,type,node"
	return trace.Wrap(execer.Exec(ctx, insertStmt, r.ts.ToTime(), history.NodeAdded, r.data.GetNode()))
}

// nodeRemoved represents a node removed event.
//
// Implements history.DataInserter.
type nodeRemoved struct {
	ts   *pb.Timestamp
	data *pb.NodeRemoved
}

func (r *nodeRemoved) Insert(ctx context.Context, execer history.Execer) error {
	const insertStmt = "timestamp,type,node"
	return trace.Wrap(execer.Exec(ctx, insertStmt, r.ts.ToTime(), history.NodeRemoved, r.data.GetNode()))
}

// nodeDegraded represents a node degraded event.
//
// Implements history.DataInserter.
type nodeDegraded struct {
	ts   *pb.Timestamp
	data *pb.NodeDegraded
}

func (r *nodeDegraded) Insert(ctx context.Context, execer history.Execer) error {
	const insertStmt = "timestamp,type,node"
	return trace.Wrap(execer.Exec(ctx, insertStmt, r.ts.ToTime(), history.NodeDegraded, r.data.GetNode()))
}

// nodeHealthy represents a node healthy event.
//
// Implements history.DataInserter.
type nodeHealthy struct {
	ts   *pb.Timestamp
	data *pb.NodeHealthy
}

func (r *nodeHealthy) Insert(ctx context.Context, execer history.Execer) error {
	const insertStmt = "timestamp,type,node"
	return trace.Wrap(execer.Exec(ctx, insertStmt, r.ts.ToTime(), history.NodeHealthy, r.data.GetNode()))
}

// probeFailed represents a probe failed event.
//
// Implements history.DataInserter.
type probeFailed struct {
	ts   *pb.Timestamp
	data *pb.ProbeFailed
}

func (r *probeFailed) Insert(ctx context.Context, execer history.Execer) error {
	const insertStmt = "timestamp,type,node,probe"
	return trace.Wrap(execer.Exec(ctx, insertStmt,
		r.ts.ToTime(), history.ProbeFailed, r.data.GetNode(), r.data.GetProbe()))
}

// probeSucceeded represents a probe succeeded event.
//
// Implements history.DataInserter.
type probeSucceeded struct {
	ts   *pb.Timestamp
	data *pb.ProbeSucceeded
}

func (r *probeSucceeded) Insert(ctx context.Context, execer history.Execer) error {
	const insertStmt = "timestamp,type,node,probe"
	return trace.Wrap(execer.Exec(ctx, insertStmt,
		r.ts.ToTime(), history.ProbeSucceeded, r.data.GetNode(), r.data.GetProbe()))
}

// leaderElected represents a leader elected event.
//
// Implements history.DataInserter.
type leaderElected struct {
	ts   *pb.Timestamp
	data *pb.LeaderElected
}

func (r *leaderElected) Insert(ctx context.Context, execer history.Execer) error {
	const insertStmt = "timestamp,type,node"
	return trace.Wrap(execer.Exec(ctx, insertStmt, r.ts.ToTime(), history.LeaderElected, r.data.GetNode()))
}
