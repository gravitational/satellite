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
}

// newTimelineEvent constructs a new TimelineEvent with the provided timestamp.
func newTimelineEvent(timestamp time.Time) pb.TimelineEvent {
	return pb.TimelineEvent{
		Timestamp: &pb.Timestamp{
			Seconds:     timestamp.Unix(),
			Nanoseconds: int32(timestamp.Nanosecond()),
		},
	}
}

type ClusterRecovered struct {
	pb.TimelineEvent
}

func NewClusterRecovered(timestamp time.Time) *ClusterRecovered {
	event := newTimelineEvent(timestamp)
	event.Data = &pb.TimelineEvent_ClusterRecovered{
		ClusterRecovered: &pb.ClusterRecovered{},
	}
	return &ClusterRecovered{event}
}

func (e *ClusterRecovered) ToProto() *pb.TimelineEvent {
	return &e.TimelineEvent
}

func (e *ClusterRecovered) ToArgs() (args []interface{}) {
	// TODO
	return args
}

type ClusterDegraded struct {
	pb.TimelineEvent
}

func NewClusterDegraded(timestamp time.Time) *ClusterDegraded {
	event := newTimelineEvent(timestamp)
	event.Data = &pb.TimelineEvent_ClusterDegraded{
		ClusterDegraded: &pb.ClusterDegraded{},
	}
	return &ClusterDegraded{event}
}

func (e *ClusterDegraded) ToProto() *pb.TimelineEvent {
	return &e.TimelineEvent
}

func (e *ClusterDegraded) ToArgs() (args []interface{}) {
	// TODO
	return args
}

type NodeAdded struct {
	pb.TimelineEvent
}

func NewNodeAdded(timestamp time.Time, node string) *NodeAdded {
	event := newTimelineEvent(timestamp)
	event.Data = &pb.TimelineEvent_NodeAdded{
		NodeAdded: &pb.NodeAdded{Node: node},
	}
	return &NodeAdded{event}
}

func (e *NodeAdded) ToProto() *pb.TimelineEvent {
	return &e.TimelineEvent
}

func (e *NodeAdded) ToArgs() (args []interface{}) {
	// TODO
	return args
}

type NodeRemoved struct {
	pb.TimelineEvent
}

func NewNodeRemoved(timestamp time.Time, node string) *NodeRemoved {
	event := newTimelineEvent(timestamp)
	event.Data = &pb.TimelineEvent_NodeRemoved{
		NodeRemoved: &pb.NodeRemoved{Node: node},
	}
	return &NodeRemoved{event}
}

func (e *NodeRemoved) ToProto() *pb.TimelineEvent {
	return &e.TimelineEvent
}

func (e *NodeRemoved) ToArgs() (args []interface{}) {
	// TODO
	return args
}

type NodeRecovered struct {
	pb.TimelineEvent
}

func NewNodeRecovered(timestamp time.Time, node string) *NodeRecovered {
	event := newTimelineEvent(timestamp)
	event.Data = &pb.TimelineEvent_NodeRecovered{
		NodeRecovered: &pb.NodeRecovered{Node: node},
	}
	return &NodeRecovered{event}
}

func (e *NodeRecovered) ToProto() *pb.TimelineEvent {
	return &e.TimelineEvent
}

func (e *NodeRecovered) ToArgs() (args []interface{}) {
	// TODO
	return args
}

type NodeDegraded struct {
	pb.TimelineEvent
}

func NewNodeDegraded(timestamp time.Time, node string) *NodeDegraded {
	event := newTimelineEvent(timestamp)
	event.Data = &pb.TimelineEvent_NodeDegraded{
		NodeDegraded: &pb.NodeDegraded{Node: node},
	}
	return &NodeDegraded{event}
}

func (e *NodeDegraded) ToProto() *pb.TimelineEvent {
	return &e.TimelineEvent
}

func (e *NodeDegraded) ToArgs() (args []interface{}) {
	// TODO
	return args
}

type ProbePassed struct {
	pb.TimelineEvent
}

func NewProbePassed(timestamp time.Time, node, probe string) *ProbePassed {
	event := newTimelineEvent(timestamp)
	event.Data = &pb.TimelineEvent_ProbePassed{
		ProbePassed: &pb.ProbePassed{
			Node:  node,
			Probe: probe,
		},
	}
	return &ProbePassed{event}
}

func (e *ProbePassed) ToProto() *pb.TimelineEvent {
	return &e.TimelineEvent
}

func (e *ProbePassed) ToArgs() (args []interface{}) {
	// TODO
	return args
}

type ProbeFailed struct {
	pb.TimelineEvent
}

func NewProbeFailed(timestamp time.Time, node, probe string) *ProbeFailed {
	event := newTimelineEvent(timestamp)
	event.Data = &pb.TimelineEvent_ProbeFailed{
		ProbeFailed: &pb.ProbeFailed{
			Node:  node,
			Probe: probe,
		},
	}
	return &ProbeFailed{event}
}

func (e *ProbeFailed) ToProto() *pb.TimelineEvent {
	return &e.TimelineEvent
}

func (e *ProbeFailed) ToArgs() (args []interface{}) {
	// TODO
	return args
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
	probePassedType      = "ProbePassed"
	probeFailedType      = "ProbeFailed"
	unknownType          = "Unknown"
)
