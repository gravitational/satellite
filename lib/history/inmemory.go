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
)

// MemTimeline represents a timeline of cluster status events. The Timeline
// can hold a specified amount of events and uses a FIFO eviction policy.
// Timeline events are only stored in memory.
//
// Implements Timeline
type MemTimeline struct {
	// Size specifies the max size of the timeline.
	Size int
	// Events holds the latest status events.
	Events []*Event
	// LastStatus holds the last recorded cluster status.
	LastStatus *Cluster
	// mu locks timeline access
	mu sync.Mutex
}

// NewMemTimeline initializes and returns a new MemTimeline with the specified
// size.
func NewMemTimeline(size int) Timeline {
	return &MemTimeline{
		Size:       size,
		Events:     make([]*Event, 0, size),
		LastStatus: &Cluster{},
	}
}

// RecordStatus records differences of the previous status to the provided
// status into the Timeline.
func (t *MemTimeline) RecordStatus(status *pb.SystemStatus) {
	cluster := parseSystemStatus(status)
	events := t.LastStatus.diffCluster(cluster)
	if len(events) == 0 {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	for _, event := range events {
		t.addEvent(event)
	}
	t.LastStatus = cluster
}

// GetEvents returns the current timeline.
func (t *MemTimeline) GetEvents() []*Event {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.Events
}

// addEvent appends the provided event to the timeline.
func (t *MemTimeline) addEvent(event *Event) {
	if len(t.Events) > t.Size {
		t.Events = t.Events[1:]
	}
	t.Events = append(t.Events, event)
}
