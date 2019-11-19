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
	"sync"
	"time"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"
)

// Event represents a single timeline event. An event exposes a type and
// metadata.
type Event struct {
	// timeStamp specifies the when the event occurred.
	timeStamp time.Time
	// eventType specifies the type of event.
	eventType EventType
	// mu locks access to metadata
	mu sync.Mutex
	// metadata is a collection of event-specific metadata.
	metadata map[string]string
}

// newEvent initializes and returns a new Event with the specified eventType.
func newEvent(eventType EventType) Event {
	return Event{
		timeStamp: time.Now(),
		eventType: eventType,
		metadata:  make(map[string]string),
	}
}

// NewClusterRecoveredEvent initializes and returns a new cluster recovered
// event.
func NewClusterRecoveredEvent() Event {
	return newEvent(ClusterRecovered)
}

// NewClusterDegradedEvent initializes and returns a new cluster degraded
// event.
func NewClusterDegradedEvent() Event {
	return newEvent(ClusterDegraded)
}

// NewNodeAddedEvent initializes and returns a new node added event.
func NewNodeAddedEvent() Event {
	return newEvent(NodeAdded)
}

// NewNodeRemovedEvent initializes and returns a new node removed event.
func NewNodeRemovedEvent() Event {
	return newEvent(NodeRemoved)
}

// NewNodeRecoveredEvent initializes and returns a new node recovered event.
func NewNodeRecoveredEvent() Event {
	return newEvent(NodeRecovered)
}

// NewNodeDegradedEvent initializes and returns a new node degraded event.
func NewNodeDegradedEvent() Event {
	return newEvent(NodeDegraded)
}

// NewProbeAddedEvent initializes and returns a new probe added event.
func NewProbeAddedEvent() Event {
	return newEvent(ProbeAdded)
}

// NewProbeRemovedEvent initializes and returns a new probe removed event.
func NewProbeRemovedEvent() Event {
	return newEvent(ProbeRemoved)
}

// NewProbePassedEvent initializes and returns a new probe passed event.
func NewProbePassedEvent() Event {
	return newEvent(ProbePassed)
}

// NewProbeFailedEvent initializes and returns a new probe failed event.
func NewProbeFailedEvent() Event {
	return newEvent(ProbeFailed)
}

// SetMetadata stores the key/value pair in event metadata.
func (e *Event) SetMetadata(key, value string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.metadata[key] = value
}

// ToProto converts Event into protobuf message.
func (e *Event) ToProto() *pb.TimelineEvent {
	e.mu.Lock()
	defer e.mu.Unlock()
	return &pb.TimelineEvent{
		Timestamp: &pb.Timestamp{
			Seconds:     int64(e.timeStamp.UTC().Second()),
			Nanoseconds: int32(e.timeStamp.UTC().Nanosecond()),
		},
		Type:     pb.TimelineEvent_ClusterDegraded,
		Metadata: e.metadata,
	}
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

	// ProbeAdded specifies an event when a probe is added to a node.
	ProbeAdded = "ProbeAdded"
	// ProbeRemoved specifies an event when a probe is removed from a node.
	ProbeRemoved = "ProbeRemoved"
	// ProbePassed specifies an event when a probe result changed to passsing.
	ProbePassed = "ProbePassed"
	// ProbeFailed specifies an event when a probe result changed to failing.
	ProbeFailed = "ProbeFailed"
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
	case ProbeAdded:
		return pb.TimelineEvent_ProbeAdded
	case ProbeRemoved:
		return pb.TimelineEvent_ProbeRemoved
	case ProbePassed:
		return pb.TimelineEvent_ProbePassed
	case ProbeFailed:
		return pb.TimelineEvent_ProbeFailed
	default:
		return pb.TimelineEvent_Unknown
	}
}
