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

	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/jonboulle/clockwork"
)

// MemTimeline represents a timeline of cluster status events. The Timeline
// can hold a specified amount of events and uses a FIFO eviction policy.
// Timeline events are only stored in memory.
//
// Implements Timeline
type MemTimeline struct {
	// capacity specifies the max number of events stored in the timeline.
	capacity int
	// events holds the latest status events.
	events []*pb.TimelineEvent
	// lastStatus holds the last recorded cluster status.
	lastStatus *pb.SystemStatus
	// mu locks timeline access
	mu sync.Mutex
}

// NewMemTimeline initializes and returns a new MemTimeline with the specified
// size.
func NewMemTimeline(capacity int) *MemTimeline {
	return &MemTimeline{
		capacity:   capacity,
		events:     make([]*pb.TimelineEvent, 0, capacity),
		lastStatus: nil,
	}
}

// RecordStatus records differences of the previous status to the provided
// status into the Timeline. Timestamps will be recorded from the provided
// clock.
func (t *MemTimeline) RecordStatus(clock clockwork.Clock, status *pb.SystemStatus) {
	events := diffCluster(clock, t.lastStatus, status)
	if len(events) == 0 {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	for _, event := range events {
		t.addEvent(event)
	}
	t.lastStatus = status
}

// GetEvents returns the current timeline.
func (t *MemTimeline) GetEvents() (events []*pb.TimelineEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, event := range t.events {
		events = append(events, event)
	}

	return events
}

// addEvent appends the provided event to the timeline.
func (t *MemTimeline) addEvent(event *pb.TimelineEvent) {
	if len(t.events) >= t.capacity {
		t.events = t.events[1:]
	}
	t.events = append(t.events, event)
}
