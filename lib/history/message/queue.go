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

package message

import (
	"context"
	"sync"
	"time"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/lib/history"

	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

// Queue can be used to temporarily store timeline events and notify
// subscribers when new events are available.
// Implements Messenger.
type Queue struct {
	// lock access to queue.
	sync.Mutex
	// timeline is used to temporarily store local events.
	timeline history.Timeline
	// subscribers maps a unique id to a subscriber.
	subscribers map[string]Subscriber
}

// NewQueue constructs a new queue with the provided configuration.
func NewQueue(timeline history.Timeline) *Queue {
	return &Queue{
		timeline:    timeline,
		subscribers: make(map[string]Subscriber),
	}
}

// Publish an event to the queue.
func (q *Queue) Publish(ctx context.Context, events []*pb.TimelineEvent) error {
	if err := q.timeline.RecordTimeline(ctx, events); err != nil {
		return trace.Wrap(err, "failed to record events")
	}

	if err := q.notifySubscribers(ctx); err != nil {
		return trace.Wrap(err, "failed to notify subscribers")
	}

	return nil
}

// Subscribe maps the unique id to subscriber.
// The subscriber will be notified of new events.
func (q *Queue) Subscribe(id string, sub Subscriber) error {
	q.Lock()
	defer q.Unlock()

	if _, ok := q.subscribers[id]; ok {
		return trace.BadParameter("id [%s] is already subscribed", id)
	}

	if err := q.initSubscriber(sub); err != nil {
		return trace.Wrap(err, "failed to initialize subscriber: %s", id)
	}
	q.subscribers[id] = sub

	return nil
}

// Unsubscribe subscriber specified by the provided the id.
func (q *Queue) Unsubscribe(id string) error {
	q.Lock()
	defer q.Unlock()

	if _, ok := q.subscribers[id]; !ok {
		return trace.NotFound("id [%s] is not subscribed", id)
	}
	delete(q.subscribers, id)
	return nil
}

// initSubscriber brings the subscriber up to date with previously stored
// events.
func (q *Queue) initSubscriber(sub Subscriber) error {
	// initSubscriberTimeout defines the max amount of time given to initalize subscriber.
	const initSubscriberTimeout = 5 * time.Second

	ctx, cancel := context.WithTimeout(context.TODO(), initSubscriberTimeout)
	defer cancel()

	events, err := q.timeline.GetEvents(ctx, nil)
	if err != nil {
		return trace.Wrap(err)
	}

	if err := q.notifySubscriber(ctx, sub, events); err != nil {
		return trace.Wrap(err)
	}

	return nil
}

// notifySubscribers notifies subscribers with new events.
func (q *Queue) notifySubscribers(ctx context.Context) error {
	events, err := q.timeline.GetEvents(ctx, nil)
	if err != nil {
		return trace.Wrap(err)
	}

	for id, sub := range q.subscribers {
		if err := q.notifySubscriber(ctx, sub, events); err != nil {
			log.WithError(err).Warnf("Failed to notify %s.", id)
		}
	}

	return nil
}

// notifySubscriber notifies the specified subscriber with new events.
func (q *Queue) notifySubscriber(ctx context.Context, sub Subscriber, events []*pb.TimelineEvent) error {
	filtered := filterByTimestamp(events, sub.Timestamp())
	if err := sub.Notify(ctx, filtered); err != nil {
		return trace.Wrap(err)
	}
	return nil
}

// filterByTimestamp filters out events that occurred before the provided
// timestamp.
func filterByTimestamp(events []*pb.TimelineEvent, timestamp time.Time) (filtered []*pb.TimelineEvent) {
	for _, event := range events {
		if event.GetTimestamp().ToTime().After(timestamp) {
			filtered = append(filtered, event)
		}
	}
	return filtered
}
