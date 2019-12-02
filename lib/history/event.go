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

// Event represents a single timeline event. An event exposes a type and
// metadata.
type Event struct {
	// timeStamp specifies the time when the event occurred.
	timestamp time.Time
	// eventType specifies the type of event.
	eventType EventType
	// metadata is a collection of event-specific metadata.
	metadata map[string]string
}

// newEvent initializes and returns a new Event with the specified eventType.
func newEvent(timestamp time.Time, eventType EventType) *Event {
	return &Event{
		timestamp: timestamp,
		eventType: eventType,
		metadata:  make(map[string]string),
	}
}

// NewClusterRecoveredEvent initializes and returns a new cluster recovered
// event.
func NewClusterRecoveredEvent(timestamp time.Time, old, new string) *Event {
	event := newEvent(timestamp, ClusterRecovered)
	event.metadata["old"] = old
	event.metadata["new"] = new
	return event
}

// NewClusterDegradedEvent initializes and returns a new cluster degraded
// event.
func NewClusterDegradedEvent(timestamp time.Time, old, new string) *Event {
	event := newEvent(timestamp, ClusterDegraded)
	event.metadata["old"] = old
	event.metadata["new"] = new
	return event
}

// NewNodeAddedEvent initializes and returns a new node added event.
func NewNodeAddedEvent(timestamp time.Time, node string) *Event {
	event := newEvent(timestamp, NodeAdded)
	event.metadata["node"] = node
	return event
}

// NewNodeRemovedEvent initializes and returns a new node removed event.
func NewNodeRemovedEvent(timestamp time.Time, node string) *Event {
	event := newEvent(timestamp, NodeRemoved)
	event.metadata["node"] = node
	return event
}

// NewNodeRecoveredEvent initializes and returns a new node recovered event.
func NewNodeRecoveredEvent(timestamp time.Time, node, old, new string) *Event {
	event := newEvent(timestamp, NodeRecovered)
	event.metadata["node"] = node
	event.metadata["old"] = old
	event.metadata["new"] = new
	return event
}

// NewNodeDegradedEvent initializes and returns a new node degraded event.
func NewNodeDegradedEvent(timestamp time.Time, node, old, new string) *Event {
	event := newEvent(timestamp, NodeDegraded)
	event.metadata["node"] = node
	event.metadata["old"] = old
	event.metadata["new"] = new
	return event
}

// NewProbePassedEvent initializes and returns a new probe passed event.
func NewProbePassedEvent(timestamp time.Time, node, probe, old, new string) *Event {
	event := newEvent(timestamp, ProbePassed)
	event.metadata["node"] = node
	event.metadata["probe"] = probe
	event.metadata["old"] = old
	event.metadata["new"] = new
	return event
}

// NewProbeFailedEvent initializes and returns a new probe failed event.
func NewProbeFailedEvent(timestamp time.Time, node, probe, old, new string) *Event {
	event := newEvent(timestamp, ProbeFailed)
	event.metadata["node"] = node
	event.metadata["probe"] = probe
	event.metadata["old"] = old
	event.metadata["new"] = new
	return event
}

// NewUnknownEvent initializes an returns a new unknown event.
func NewUnknownEvent(timestamp time.Time) *Event {
	event := newEvent(timestamp, Unknown)
	return event
}

// ToProto returns the event as a protobuf message.
func (e *Event) ToProto() *pb.TimelineEvent {
	return &pb.TimelineEvent{
		Timestamp: &pb.Timestamp{
			Seconds:     e.timestamp.Unix(),
			Nanoseconds: int32(e.timestamp.UnixNano()),
		},
		Type:     e.eventType.ToProto(),
		Metadata: e.metadata,
	}
}

// ToArgs returns the event as a list of arguments.
func (e *Event) ToArgs() (valueArgs []interface{}) {
	valueArgs = append(valueArgs, e.timestamp)
	valueArgs = append(valueArgs, e.eventType)
	valueArgs = append(valueArgs, e.metadata["node"])
	valueArgs = append(valueArgs, e.metadata["probe"])
	valueArgs = append(valueArgs, e.metadata["old"])
	valueArgs = append(valueArgs, e.metadata["new"])
	return valueArgs
}

// EventType specifies the type of event.
type EventType string

const (
	// ClusterRecovered specifies an event that causes the cluster's state to recover.
	ClusterRecovered = "ClusterRecovered"
	// ClusterDegraded specifies an event that causes the cluster's state to degrade.
	ClusterDegraded = "ClusterDegraded"

	// NodeAdded specifies an event when a node is added to the cluster.
	NodeAdded = "NodeAdded"
	// NodeRemoved specifies an event when a node is removed from the cluster.
	NodeRemoved = "NodeRemoved"
	// NodeRecovered specifies an event that caused the cluster's state to recover.
	NodeRecovered = "NodeRecovered"
	//NodeDegraded specifies an event that caused the cluster's state to degrade.
	NodeDegraded = "NodeDegraded"

	// ProbePassed specifies an event when a probe result changed to passsing.
	ProbePassed = "ProbePassed"
	// ProbeFailed specifies an event when a probe result changed to failing.
	ProbeFailed = "ProbeFailed"

	// Unknown specifies an unknown event type.
	Unknown = "Unknown"
)

// ToProto converts the EventType into a protobuf TimelineEvent_Type.
func (t EventType) ToProto() pb.TimelineEvent_Type {
	switch t {
	case ClusterRecovered:
		return pb.TimelineEvent_ClusterRecovered
	case ClusterDegraded:
		return pb.TimelineEvent_ClusterDegraded
	case NodeAdded:
		return pb.TimelineEvent_NodeAdded
	case NodeRemoved:
		return pb.TimelineEvent_NodeRemoved
	case NodeRecovered:
		return pb.TimelineEvent_NodeRecovered
	case NodeDegraded:
		return pb.TimelineEvent_NodeDegraded
	case ProbePassed:
		return pb.TimelineEvent_ProbePassed
	case ProbeFailed:
		return pb.TimelineEvent_ProbeFailed
	default:
		return pb.TimelineEvent_Unknown
	}
}
