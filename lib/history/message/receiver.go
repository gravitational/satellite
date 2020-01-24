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
	"time"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/lib/client"

	"github.com/gravitational/trace"
)

// Receiver can be used to subscribe to a Messenger and be notified when new
// events are available.
// Implements Subscriber.
type Receiver struct {
	// rpcConfig specifies configuration used to create a new rpc client.
	rpcConfig client.Config
	// timestamp specifies the last timestamp seen by the receiver.
	timestamp time.Time
}

// NewReceiver constructs a new receiver with the provided configuration.
func NewReceiver(rpcConfig client.Config) *Receiver {
	return &Receiver{rpcConfig: rpcConfig}
}

// Timestamp returns the last seen timestamp.
func (r *Receiver) Timestamp() time.Time {
	return r.timestamp
}

// Notify notifies the receiver of new events that need to be sent to the
// receiving cluster member.
func (r *Receiver) Notify(ctx context.Context, events []*pb.TimelineEvent) error {
	return r.pushEvents(ctx, events)
}

// pushEvents sends the provided events to the receiving cluster member  and
// updates the last seen timestamp.
// The provided events must be sorted by timestamp to be able to retry failed
// updates.
func (r *Receiver) pushEvents(ctx context.Context, events []*pb.TimelineEvent) error {
	client, err := client.NewClient(ctx, r.rpcConfig)
	if err != nil {
		return trace.Wrap(err)
	}
	defer client.Close()

	for _, event := range events {
		// TODO: add retry strategy
		if _, err := client.UpdateTimeline(ctx, &pb.UpdateRequest{Event: event}); err != nil {
			return trace.Wrap(err)
		}
		// Subtract a second in case there are multiple events with the same timestamp.
		// Update timeline might fail on one of these events and needs to be pushed
		// again.
		r.timestamp = event.GetTimestamp().ToTime().Add(-time.Second)
	}
	return nil
}
