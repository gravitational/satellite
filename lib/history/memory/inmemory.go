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

// Package memory provide Timeline implementation stored in memory.
// Mainly used for development or testing purposes and has not been optimized.
// TODO: Proper LRU caching implementation for storing timeline events.
package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/lib/history"

	"github.com/jonboulle/clockwork"
)

// Timeline represents a timeline of cluster status events. The Timeline
// can hold a specified amount of events and uses a FIFO eviction policy.
// Timeline events are only stored in memory.
//
// Implements history.Timeline
type Timeline struct {
	sync.Mutex
	// clock is used to record event timestamps.
	clock clockwork.Clock
	// capacity specifies the max number of events stored in the timeline.
	capacity int
	// events holds the latest status events.
	events []*pb.TimelineEvent
	// lastStatus holds the last recorded status.
	lastStatus *pb.NodeStatus
}

// NewTimeline initializes and returns a new Timeline with the specified
// capacity. The provided clock will be used to record event timestamps.
func NewTimeline(clock clockwork.Clock, capacity int) *Timeline {
	return &Timeline{
		clock:      clock,
		capacity:   capacity,
		events:     make([]*pb.TimelineEvent, 0, capacity),
		lastStatus: nil,
	}
}

// RecordStatus records the differences between the previously stored status
// and the newly provided status into the timeline.
// Context unused for memory Timeline.
func (t *Timeline) RecordStatus(ctx context.Context, status *pb.NodeStatus) error {
	events := history.DiffNode(t.clock, t.lastStatus, status)
	if len(events) == 0 {
		return nil
	}

	t.Lock()
	defer t.Unlock()

	t.addEvents(events)
	t.filterDuplicates()
	t.lastStatus = status
	return nil
}

// RecordTimeline merges the provided events into the current timeline.
// Duplicate events will be ignored.
func (t *Timeline) RecordTimeline(ctx context.Context, events []*pb.TimelineEvent) error {
	if len(events) == 0 {
		return nil
	}
	t.Lock()
	defer t.Unlock()

	t.addEvents(events)
	t.filterDuplicates()
	return nil
}

// GetEvents returns a filtered list of events based on the provided params.
// Events will be returned in sorted order by timestamp.
// Context unused for memory Timeline.
func (t *Timeline) GetEvents(ctx context.Context, params map[string]string) (events []*pb.TimelineEvent, err error) {
	t.Lock()
	defer t.Unlock()
	return t.getFilteredEvents(params), nil
}

// addEvents adds the events into the timeline. Duplicates will be filtered out
// and events will be stored in sorted order by timestamp.
func (t *Timeline) addEvents(events []*pb.TimelineEvent) {
	t.events = append(t.events, events...)
	t.filterDuplicates()
	sort.Sort(byTimestamp(t.events))

	// remove events oldest events if timeline is over capacity.
	index := len(t.events) - t.capacity
	if index > 0 {
		t.events = t.events[index:]
	}
}

// getFilteredEvents returns a filtered list of events based on the provided params.
func (t *Timeline) getFilteredEvents(params map[string]string) (events []*pb.TimelineEvent) {
	// TODO: for now just return the unfiltered events.
	return t.events
}

// filterDuplicates removes duplicate events.
func (t *Timeline) filterDuplicates() {
	set := make(map[comparableEvent]struct{})
	filteredEvents := make([]*pb.TimelineEvent, 0, t.capacity)
	for _, event := range t.events {
		comparable := newComparable(event)
		if _, ok := set[comparable]; !ok {
			set[comparable] = struct{}{}
			filteredEvents = append(filteredEvents, event)
		}
	}
	t.events = filteredEvents
}

// comparableEvent defines an event in a comparable struct.
// Used when filtering duplicate events.
type comparableEvent struct {
	timestamp time.Time
	eventType string
	node      string
	probe     string
	old       string
	new       string
}

func newComparable(event *pb.TimelineEvent) comparableEvent {
	comparable := comparableEvent{
		timestamp: event.GetTimestamp().ToTime(),
	}
	switch event.GetData().(type) {
	case *pb.TimelineEvent_ClusterRecovered:
		comparable.eventType = clusterRecoveredType
	case *pb.TimelineEvent_ClusterDegraded:
		comparable.eventType = clusterDegradedType
	case *pb.TimelineEvent_NodeAdded:
		e := event.GetNodeAdded()
		comparable.eventType = nodeAddedType
		comparable.node = e.GetNode()
	case *pb.TimelineEvent_NodeRemoved:
		e := event.GetNodeRemoved()
		comparable.eventType = nodeRemovedType
		comparable.node = e.GetNode()
	case *pb.TimelineEvent_NodeRecovered:
		e := event.GetNodeRecovered()
		comparable.eventType = nodeRecoveredType
		comparable.node = e.GetNode()
	case *pb.TimelineEvent_NodeDegraded:
		e := event.GetNodeDegraded()
		comparable.eventType = nodeDegradedType
		comparable.node = e.GetNode()
	case *pb.TimelineEvent_ProbeSucceeded:
		e := event.GetProbeSucceeded()
		comparable.eventType = probeSucceededType
		comparable.node = e.GetNode()
		comparable.probe = e.GetProbe()
	case *pb.TimelineEvent_ProbeFailed:
		e := event.GetProbeFailed()
		comparable.eventType = probeFailedType
		comparable.node = e.GetNode()
		comparable.probe = e.GetProbe()
	default:
		comparable.eventType = unknownType
	}
	return comparable
}

// byTimestamp implements sort.Interface. Events are sorted by timestamp.
type byTimestamp []*pb.TimelineEvent

func (r byTimestamp) Len() int { return len(r) }
func (r byTimestamp) Less(i, j int) bool {
	return r[i].GetTimestamp().ToTime().Before(r[j].GetTimestamp().ToTime())
}
func (r byTimestamp) Swap(i, j int) { r[i], r[j] = r[j], r[i] }

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
