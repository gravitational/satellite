/*
Copyright 2016 Gravitational, Inc.

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

package agent

import (
	"github.com/gravitational/trace"
	serf "github.com/hashicorp/serf/client"
)

// serfClient is the minimal interface to the serf cluster.
// It enables mocking access to serf network in tests.
type serfClient interface {
	// Members lists members of the serf cluster.
	Members() ([]serf.Member, error)
	// Stop cancels the serf event delivery and removes the subscription.
	Stop(serf.StreamHandle) error
	// Close closes the client.
	Close() error
	// Join attempts to join an existing serf cluster identified by peers.
	// Replay controls if previous user events are replayed once this node has joined the cluster.
	// Returns the number of nodes joined
	Join(peers []string, replay bool) (int, error)
	// UpdateTags will modify the tags on a running serf agent
	UpdateTags(tags map[string]string, delTags []string) error
}

func newRetryingClient(clientConfig serf.Config) (*retryingClient, error) {
	client, err := reinit(clientConfig)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &retryingClient{
		client: client,
		config: clientConfig,
	}, nil
}

// Members lists members of the serf cluster.
func (r *retryingClient) Members() ([]serf.Member, error) {
	if err := r.reinit(); err != nil {
		return nil, trace.Wrap(err)
	}
	return r.client.Members()
}

// Stop cancels the serf event delivery and removes the subscription.
func (r *retryingClient) Stop(handle serf.StreamHandle) error {
	if err := r.reinit(); err != nil {
		return trace.Wrap(err)
	}
	return r.client.Stop(handle)
}

// Join attempts to join an existing serf cluster identified by peers.
// Replay controls if previous user events are replayed once this node has joined the cluster.
// Returns the number of nodes joined
func (r *retryingClient) Join(peers []string, replay bool) (int, error) {
	if err := r.reinit(); err != nil {
		return 0, trace.Wrap(err)
	}
	return r.client.Join(peers, replay)
}

// UpdateTags will modify the tags on a running serf agent
func (r *retryingClient) UpdateTags(tags map[string]string, delTags []string) error {
	if err := r.reinit(); err != nil {
		return trace.Wrap(err)
	}
	return r.client.UpdateTags(tags, delTags)
}

// Close closes the client
func (r *retryingClient) Close() error {
	if r.client.IsClosed() {
		return nil
	}
	return r.client.Close()
}

func (r *retryingClient) reinit() error {
	if !r.client.IsClosed() {
		return nil
	}
	client, err := reinit(r.config)
	if err != nil {
		return trace.Wrap(err)
	}
	r.client = client
	return nil
}

func reinit(clientConfig serf.Config) (*serf.RPCClient, error) {
	client, err := serf.ClientFromConfig(&clientConfig)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return client, nil
}

type retryingClient struct {
	client *serf.RPCClient
	config serf.Config
}
