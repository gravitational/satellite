/*
Copyright 2020 Gravitational, Inc.

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

package agentpb

import (
	"time"
)

// newTimelineEvent constructs a new TimelineEvent with the provided timestamp.
func newTimelineEvent(timestamp time.Time) *TimelineEvent {
	return &TimelineEvent{
		Timestamp: &Timestamp{
			Seconds:     timestamp.Unix(),
			Nanoseconds: int32(timestamp.Nanosecond()),
		},
	}
}

// NewClusterRecovered constructs a new ClusterRecovered event with the
// provided data.
func NewClusterRecovered(timestamp time.Time) *TimelineEvent {
	event := newTimelineEvent(timestamp)
	event.Data = &TimelineEvent_ClusterRecovered{
		ClusterRecovered: &ClusterRecovered{},
	}
	return event
}

// NewClusterDegraded constructs a new ClusterDegraded event with the provided
// data.
func NewClusterDegraded(timestamp time.Time) *TimelineEvent {
	event := newTimelineEvent(timestamp)
	event.Data = &TimelineEvent_ClusterDegraded{
		ClusterDegraded: &ClusterDegraded{},
	}
	return event
}

// NewNodeAdded constructs a new NodeAdded event with the provided data.
func NewNodeAdded(timestamp time.Time, node string) *TimelineEvent {
	event := newTimelineEvent(timestamp)
	event.Data = &TimelineEvent_NodeAdded{
		NodeAdded: &NodeAdded{Node: node},
	}
	return event
}

// NewNodeRemoved constructs a new NodeRemoved event with the provided data.
func NewNodeRemoved(timestamp time.Time, node string) *TimelineEvent {
	event := newTimelineEvent(timestamp)
	event.Data = &TimelineEvent_NodeRemoved{
		NodeRemoved: &NodeRemoved{Node: node},
	}
	return event
}

// NewNodeRecovered constructs a new NodeRecovered event with the provided data.
func NewNodeRecovered(timestamp time.Time, node string) *TimelineEvent {
	event := newTimelineEvent(timestamp)
	event.Data = &TimelineEvent_NodeRecovered{
		NodeRecovered: &NodeRecovered{Node: node},
	}
	return event
}

// NewNodeDegraded constructs a new NodeDegraded event with the provided data.
func NewNodeDegraded(timestamp time.Time, node string) *TimelineEvent {
	event := newTimelineEvent(timestamp)
	event.Data = &TimelineEvent_NodeDegraded{
		NodeDegraded: &NodeDegraded{Node: node},
	}
	return event
}

// NewProbeSucceeded constructs a new ProbeSucceeded event with the provided
// data.
func NewProbeSucceeded(timestamp time.Time, node, probe string) *TimelineEvent {
	event := newTimelineEvent(timestamp)
	event.Data = &TimelineEvent_ProbeSucceeded{
		ProbeSucceeded: &ProbeSucceeded{
			Node:  node,
			Probe: probe,
		},
	}
	return event
}

// NewProbeFailed constructs a new ProbeFailed event with the provided data.
func NewProbeFailed(timestamp time.Time, node, probe string) *TimelineEvent {
	event := newTimelineEvent(timestamp)
	event.Data = &TimelineEvent_ProbeFailed{
		ProbeFailed: &ProbeFailed{
			Node:  node,
			Probe: probe,
		},
	}
	return event
}

// NewLeaderElected constructs a new LeaderElected event with the provided data.
func NewLeaderElected(timestamp time.Time, node string) *TimelineEvent {
	event := newTimelineEvent(timestamp)
	event.Data = &TimelineEvent_LeaderElected{
		LeaderElected: &LeaderElected{
			Node: node,
		},
	}
	return event
}
