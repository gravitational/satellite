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

// Package message provides interfaces for implementing internal communication
// of timeline messages.
package message

import (
	"context"
	"time"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"
)

// Subscriber can subscribe to a Messenger.
type Subscriber interface {
	// Notifiy notifies the subscriber of new events.
	Notify(ctx context.Context, events []*pb.TimelineEvent) error
	// Timestamp returns the last seen timestamp.
	Timestamp() time.Time
}

// Messenger notifies subscribers when new events are available.
type Messenger interface {
	// Publish an event to the messenger.
	Publish(ctx context.Context, events []*pb.TimelineEvent) error
	// Subscribe a new subscriber uniquely identified by the provided id.
	Subscribe(id string, sub Subscriber) error
	// Unsubscribe a subscriber specified by the provided id.
	Unsubscribe(id string) error
	// IsSubscribed returns true if the id is already subscribed.
	IsSubscribed(id string) bool
}
