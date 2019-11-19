package history

import (
	"time"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"
)

// Event represents a single timeline event. An event exposes a type and
// metadata.
type Event struct {
	// TimeStamp specifies the when the event occurred.
	timeStamp time.Time
	// EventType specifies the type of event.
	eventType EventType
	// Metadata is a collection of event-specific metadata.
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
	e.metadata[key] = value
}

// ToProto converts Event into protobuf message.
func (e *Event) ToProto() *pb.TimelineEvent {
	return &pb.TimelineEvent{
		Timestamp: &pb.Timestamp{
			Seconds:     int64(e.timeStamp.UTC().Second()),
			Nanoseconds: int32(e.timeStamp.UTC().Nanosecond()),
		},
		Type:     pb.TimelineEvent_ClusterDegraded,
		Metadata: e.metadata,
	}
}
