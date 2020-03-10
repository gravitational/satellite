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

	pb "github.com/gravitational/satellite/agent/proto/agentpb"

	"github.com/gravitational/trace"
	"github.com/jonboulle/clockwork"
)

// Timeline represents a timeline of cluster status events. The Timeline
// can hold a specified amount of events and uses a FIFO eviction policy.
// Timeline events are only stored in memory.
//
// Implements history.Timeline
type Timeline struct {
	// clock is used to record event timestamps.
	clock clockwork.Clock
	// capacity specifies the max number of events stored in the timeline.
	capacity int
	// lock access to events
	sync.Mutex
	// events holds the latest status events.
	events []memEvent
}

// NewTimeline initializes and returns a new Timeline with the specified
// capacity. The provided clock will be used to record event timestamps.
func NewTimeline(clock clockwork.Clock, capacity int) *Timeline {
	return &Timeline{
		clock:    clock,
		capacity: capacity,
		events:   make([]memEvent, 0, capacity),
	}
}

// RecordEvents records the provided events into the current timeline.
// Duplicate events will be ignored.
func (t *Timeline) RecordEvents(ctx context.Context, events []*pb.TimelineEvent) error {
	if len(events) == 0 {
		return nil
	}

	sort.Sort(byTimestamp(events))

	t.Lock()
	defer t.Unlock()

	if err := t.insertEvents(ctx, events); err != nil {
		return trace.Wrap(err)
	}

	return nil
}

// GetEvents returns a filtered list of events based on the provided params.
// Events will be returned in sorted order by timestamp.
func (t *Timeline) GetEvents(_ context.Context, params map[string]string) (pbEvents []*pb.TimelineEvent, err error) {
	t.Lock()
	defer t.Unlock()
	events := t.getFilteredEvents(params)
	pbEvents = make([]*pb.TimelineEvent, 0, len(events))
	for _, event := range events {
		pbEvent, err := event.ProtoBuf()
		if err != nil {
			return pbEvents, trace.Wrap(err)
		}
		pbEvents = append(pbEvents, pbEvent)
	}
	return pbEvents, nil
}

// insertEvents inserts the provided events into the timeline. Duplicates will
// be filtered out and events will be stored in sorted order by timestamp.
func (t *Timeline) insertEvents(ctx context.Context, events []*pb.TimelineEvent) error {
	execer := newMemExecer(&t.events)
	for _, event := range events {
		row, err := newDataInserter(event)
		if err != nil {
			return trace.Wrap(err)
		}
		if err := row.Insert(ctx, execer); err != nil {
			return trace.Wrap(err)
		}
	}

	t.filterDuplicates()

	// remove events oldest events if timeline is over capacity.
	index := len(t.events) - t.capacity
	if index > 0 {
		t.events = t.events[index:]
	}

	return nil
}

// getFilteredEvents returns a filtered list of events based on the provided params.
func (t *Timeline) getFilteredEvents(_ map[string]string) (events []memEvent) {
	// TODO: for now just return the unfiltered events.
	return t.events
}

// filterDuplicates removes duplicate events.
func (t *Timeline) filterDuplicates() {
	set := make(map[memEvent]struct{})
	filteredEvents := make([]memEvent, 0, t.capacity)
	for _, event := range t.events {
		if _, ok := set[event]; !ok {
			set[event] = struct{}{}
			filteredEvents = append(filteredEvents, event)
		}
	}
	t.events = filteredEvents
}

// byTimestamp implements sort.Interface. Events are sorted by timestamp.
type byTimestamp []*pb.TimelineEvent

func (r byTimestamp) Len() int { return len(r) }
func (r byTimestamp) Less(i, j int) bool {
	return r[i].GetTimestamp().ToTime().Before(r[j].GetTimestamp().ToTime())
}
func (r byTimestamp) Swap(i, j int) { r[i], r[j] = r[j], r[i] }
