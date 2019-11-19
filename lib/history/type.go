package history

import (
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
)

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
