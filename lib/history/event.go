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
	"time"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"
)

// Event represents a single timeline event.
type Event interface {
	// ToProto returns the event as a protobuf TimelineEvent message.
	ToProto() *pb.TimelineEvent
	// ToArgs returns the event as a list of arguments.
	ToArgs() []interface{}
	// PrepareInsert returns a SQL insert statement and list of arguments.
	PrepareInsert() (statement string, args []interface{})
}

// newTimelineEvent constructs a new TimelineEvent with the provided timestamp.
func newTimelineEvent(timestamp time.Time) *pb.TimelineEvent {
	return &pb.TimelineEvent{
		Timestamp: &pb.Timestamp{
			Seconds:     timestamp.Unix(),
			Nanoseconds: int32(timestamp.Nanosecond()),
		},
	}
}

// ClusterRecovered defines an event that caused the cluster's status to
// recover.
type ClusterRecovered struct {
	event *pb.TimelineEvent
}

// NewClusterRecovered constructs a new ClusterRecovered event with the
// provided data.
func NewClusterRecovered(timestamp time.Time) ClusterRecovered {
	event := newTimelineEvent(timestamp)
	event.Data = &pb.TimelineEvent_ClusterRecovered{
		ClusterRecovered: &pb.ClusterRecovered{},
	}
	return ClusterRecovered{event: event}
}

// ToProto returns the event as a protobuf TimelineEvent message.
func (e ClusterRecovered) ToProto() *pb.TimelineEvent {
	return e.event
}

// ToArgs returns the event as a list of arguments.
func (e ClusterRecovered) ToArgs() (args []interface{}) {
	return []interface{}{
		e.event.GetTimestamp().ToTime(),
		clusterRecoveredType,
		"", // no node name
		"", // no probe name
		"", // no old value
		"", // no new value
	}
}

// PrepareInsert returns a SQL insert statement and a list of arguments.
func (e ClusterRecovered) PrepareInsert() (statement string, args []interface{}) {
	statement = `INSERT INTO events (timestamp, type) VALUES (?,?)`
	args = []interface{}{
		e.event.GetTimestamp().ToTime(),
		clusterRecoveredType,
	}
	return
}

// ClusterDegraded defines an event that caused the cluster's status to
// degrade.
type ClusterDegraded struct {
	event *pb.TimelineEvent
}

// NewClusterDegraded constructs a new ClusterDegraded event with the provided
// data.
func NewClusterDegraded(timestamp time.Time) ClusterDegraded {
	event := newTimelineEvent(timestamp)
	event.Data = &pb.TimelineEvent_ClusterDegraded{
		ClusterDegraded: &pb.ClusterDegraded{},
	}
	return ClusterDegraded{event: event}
}

// ToProto returns the event as a protobuf TimelineEvent message.
func (e ClusterDegraded) ToProto() *pb.TimelineEvent {
	return e.event
}

// ToArgs returns the event as a list of arguments.
func (e ClusterDegraded) ToArgs() (args []interface{}) {
	return []interface{}{
		e.event.GetTimestamp().ToTime(),
		clusterDegradedType,
		"", // no node name
		"", // no probe name
		"", // no old value
		"", // no new value
	}
}

// PrepareInsert returns a SQL insert statement and a list of arguments.
func (e ClusterDegraded) PrepareInsert() (statement string, args []interface{}) {
	statement = `INSERT INTO events (timestamp, type) VALUES (?,?)`
	args = []interface{}{
		e.event.GetTimestamp().ToTime(),
		clusterDegradedType,
	}
	return
}

// NodeAdded defines a cluster resize event.
type NodeAdded struct {
	event *pb.TimelineEvent
}

// NewNodeAdded constructs a new NodeAdded event with the provided data.
func NewNodeAdded(timestamp time.Time, node string) NodeAdded {
	event := newTimelineEvent(timestamp)
	event.Data = &pb.TimelineEvent_NodeAdded{
		NodeAdded: &pb.NodeAdded{Node: node},
	}
	return NodeAdded{event: event}
}

// ToProto returns the event as a protobuf TimelineEvent message.
func (e NodeAdded) ToProto() *pb.TimelineEvent {
	return e.event
}

// ToArgs returns the event as a list of arguments.
func (e NodeAdded) ToArgs() (args []interface{}) {
	return []interface{}{
		e.event.GetTimestamp().ToTime(),
		nodeAddedType,
		e.event.GetNodeAdded().GetNode(),
		"", // no probe name
		"", // no old value
		"", // no new value
	}
}

// PrepareInsert returns a SQL insert statement and a list of arguments.
func (e NodeAdded) PrepareInsert() (statement string, args []interface{}) {
	statement = `INSERT INTO events (timestamp, type, node) VALUES (?,?,?)`
	args = []interface{}{
		e.event.GetTimestamp().ToTime(),
		nodeAddedType,
		e.event.GetNodeAdded().GetNode(),
	}
	return
}

// NodeRemoved defines a cluster resize event.
type NodeRemoved struct {
	event *pb.TimelineEvent
}

// NewNodeRemoved constructs a new NodeRemoved event with the provided data.
func NewNodeRemoved(timestamp time.Time, node string) NodeRemoved {
	event := newTimelineEvent(timestamp)
	event.Data = &pb.TimelineEvent_NodeRemoved{
		NodeRemoved: &pb.NodeRemoved{Node: node},
	}
	return NodeRemoved{event: event}
}

// ToProto returns the event as a protobuf TimelineEvent message.
func (e NodeRemoved) ToProto() *pb.TimelineEvent {
	return e.event
}

// ToArgs returns the event as a list of arguments.
func (e NodeRemoved) ToArgs() (args []interface{}) {
	return []interface{}{
		e.event.GetTimestamp().ToTime(),
		nodeRemovedType,
		e.event.GetNodeRemoved().GetNode(),
		"", // no probe name
		"", // no old value
		"", // no new value
	}
}

// PrepareInsert returns a SQL insert statement and a list of arguments.
func (e NodeRemoved) PrepareInsert() (statement string, args []interface{}) {
	statement = `INSERT INTO events (timestamp, type, node) VALUES (?,?,?)`
	args = []interface{}{
		e.event.GetTimestamp().ToTime(),
		nodeRemovedType,
		e.event.GetNodeRemoved().GetNode(),
	}
	return
}

// NodeRecovered defines an event that caused a node's status to recover.
type NodeRecovered struct {
	event *pb.TimelineEvent
}

// NewNodeRecovered constructs a new NodeRecovered event with the provided data.
func NewNodeRecovered(timestamp time.Time, node string) NodeRecovered {
	event := newTimelineEvent(timestamp)
	event.Data = &pb.TimelineEvent_NodeRecovered{
		NodeRecovered: &pb.NodeRecovered{Node: node},
	}
	return NodeRecovered{event: event}
}

// ToProto returns the event as a protobuf TimlineEvent message.
func (e NodeRecovered) ToProto() *pb.TimelineEvent {
	return e.event
}

// ToArgs returns the event as a list of arguments.
func (e NodeRecovered) ToArgs() (args []interface{}) {
	return []interface{}{
		e.event.GetTimestamp().ToTime(),
		nodeRecoveredType,
		e.event.GetNodeRecovered().GetNode(),
		"", // no probe name
		"", // no old value
		"", // no new value
	}
}

// PrepareInsert returns a SQL insert statement and a list of arguments.
func (e NodeRecovered) PrepareInsert() (statement string, args []interface{}) {
	statement = `INSERT INTO events (timestamp, type, node) VALUES (?,?,?)`
	args = []interface{}{
		e.event.GetTimestamp().ToTime(),
		nodeRecoveredType,
		e.event.GetNodeRecovered().GetNode(),
	}
	return
}

// NodeDegraded defines an event that caused the node's status to degrade.
type NodeDegraded struct {
	event *pb.TimelineEvent
}

// NewNodeDegraded constructs a new NodeDegraded event with the provided data.
func NewNodeDegraded(timestamp time.Time, node string) NodeDegraded {
	event := newTimelineEvent(timestamp)
	event.Data = &pb.TimelineEvent_NodeDegraded{
		NodeDegraded: &pb.NodeDegraded{Node: node},
	}
	return NodeDegraded{event: event}
}

// ToProto returns the event as a protobuf TimelineEvent message.
func (e NodeDegraded) ToProto() *pb.TimelineEvent {
	return e.event
}

// ToArgs returns the event as a list of arguments.
func (e NodeDegraded) ToArgs() (args []interface{}) {
	return []interface{}{
		e.event.GetTimestamp().ToTime(),
		nodeDegradedType,
		e.event.GetNodeDegraded().GetNode(),
		"", // no probe name
		"", // no old value
		"", // no new value
	}
}

// PrepareInsert returns a SQL insert statement and a list of arguments.
func (e NodeDegraded) PrepareInsert() (statement string, args []interface{}) {
	statement = `INSERT INTO events (timestamp, type, node) VALUES (?,?,?)`
	args = []interface{}{
		e.event.GetTimestamp().ToTime(),
		nodeDegradedType,
		e.event.GetNodeDegraded().GetNode(),
	}
	return
}

// ProbeSucceeded defines an event that caused the probe's status to be
// succeeding.
type ProbeSucceeded struct {
	event *pb.TimelineEvent
}

// NewProbeSucceeded constructs a new ProbeSucceeded event with the provided
// data.
func NewProbeSucceeded(timestamp time.Time, node, probe string) ProbeSucceeded {
	event := newTimelineEvent(timestamp)
	event.Data = &pb.TimelineEvent_ProbeSucceeded{
		ProbeSucceeded: &pb.ProbeSucceeded{
			Node:  node,
			Probe: probe,
		},
	}
	return ProbeSucceeded{event: event}

}

// ToProto returns the event as a protobuf TimelineEvent message.
func (e ProbeSucceeded) ToProto() *pb.TimelineEvent {
	return e.event
}

// ToArgs returns the event as a list of arguments.
func (e ProbeSucceeded) ToArgs() (args []interface{}) {
	return []interface{}{
		e.event.GetTimestamp().ToTime(),
		probeSucceededType,
		e.event.GetProbeSucceeded().GetNode(),
		e.event.GetProbeSucceeded().GetProbe(),
		"", // no old value
		"", // no new value
	}
}

// PrepareInsert returns a SQL insert statement and a list of arguments.
func (e ProbeSucceeded) PrepareInsert() (statement string, args []interface{}) {
	statement = `INSERT INTO events (timestamp, type, node, probe) VALUES (?,?,?,?)`
	args = []interface{}{
		e.event.GetTimestamp().ToTime(),
		probeSucceededType,
		e.event.GetProbeSucceeded().GetNode(),
		e.event.GetProbeSucceeded().GetProbe(),
	}
	return
}

// ProbeFailed defines an event that caused the probe's status to be failing.
type ProbeFailed struct {
	event *pb.TimelineEvent
}

// NewProbeFailed constructs a new ProbeFailed event with the provided data.
func NewProbeFailed(timestamp time.Time, node, probe string) ProbeFailed {
	event := newTimelineEvent(timestamp)
	event.Data = &pb.TimelineEvent_ProbeFailed{
		ProbeFailed: &pb.ProbeFailed{
			Node:  node,
			Probe: probe,
		},
	}
	return ProbeFailed{event: event}
}

// ToProto returns the event as a protobuf TimelineEvent message.
func (e ProbeFailed) ToProto() *pb.TimelineEvent {
	return e.event
}

// ToArgs returns the event as a list of arguments.
func (e ProbeFailed) ToArgs() (args []interface{}) {
	return []interface{}{
		e.event.GetTimestamp().ToTime(),
		probeFailedType,
		e.event.GetProbeFailed().GetNode(),
		e.event.GetProbeFailed().GetProbe(),
		"", // no old value
		"", // no new value
	}
}

// PrepareInsert returns a SQL insert statement and a list of arguments.
func (e ProbeFailed) PrepareInsert() (statement string, args []interface{}) {
	statement = `INSERT INTO events (timestamp, type, node, probe) VALUES (?,?,?,?)`
	args = []interface{}{
		e.event.GetTimestamp().ToTime(),
		probeFailedType,
		e.event.GetProbeFailed().GetNode(),
		e.event.GetProbeFailed().GetProbe(),
	}
	return
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
