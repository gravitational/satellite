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
	"context"
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
	// clock is used to record event timestamps.
	clock clockwork.Clock
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
// capacity. The provided clock will be used to record event timestamps.
func NewMemTimeline(clock clockwork.Clock, capacity int) *MemTimeline {
	return &MemTimeline{
		clock:      clock,
		capacity:   capacity,
		events:     make([]*pb.TimelineEvent, 0, capacity),
		lastStatus: nil,
	}
}

// RecordStatus records the differences between the previously stored status
// and the newly provided status into the timeline.
// The ctx is unused for MemTimeline.
func (t *MemTimeline) RecordStatus(ctx context.Context, status *pb.SystemStatus) error {
	events := diffCluster(t.clock, t.lastStatus, status)
	if len(events) == 0 {
		return nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	for _, event := range events {
		t.addEvent(event)
	}
	t.lastStatus = status
	return nil
}

// GetEvents returns the current timeline.
// The ctx is unused for MemTimeline.
func (t *MemTimeline) GetEvents(ctx context.Context) (events []*pb.TimelineEvent, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.events, nil
}

// addEvent appends the provided event to the timeline.
func (t *MemTimeline) addEvent(event *pb.TimelineEvent) {
	if len(t.events) >= t.capacity {
		t.events = t.events[1:]
	}
	t.events = append(t.events, event)
}
