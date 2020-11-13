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

package membership

import (
	pb "github.com/gravitational/satellite/agent/proto/agentpb"

	"github.com/gravitational/trace"
	serf "github.com/hashicorp/serf/client"
	"github.com/hashicorp/serf/coordinate"
)

// Client is an rpc client used to make requests to a serf agent.
type Client struct {
	client *serf.RPCClient
}

// NewSerfClient returns a new serf client for the specified configuration.
func NewSerfClient(config serf.Config) (*Client, error) {
	client, err := serf.ClientFromConfig(&config)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &Client{
		client: client,
	}, nil
}

// Members lists members of the serf cluster.
func (r *Client) Members() ([]*pb.MemberStatus, error) {
	members, err := r.client.Members()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	// NOTE: is it okay to filter out inactive nodes in this function?
	// When do we want to use a list of members including inactive nodes?
	members = filterLeft(members)

	clusterMembers := make([]*pb.MemberStatus, 0, len(members))
	for _, member := range members {
		status := pb.NewMemberStatus(member.Name, member.Addr.String(), member.Tags)
		clusterMembers = append(clusterMembers, status)
	}
	return clusterMembers, nil
}

// FindMember finds serf member with the specified name.
func (r *Client) FindMember(name string) (member *pb.MemberStatus, err error) {
	members, err := r.Members()
	if err != nil {
		return member, trace.Wrap(err)
	}
	for _, member := range members {
		if member.Name == name {
			return member, nil
		}
	}
	return member, trace.NotFound("serf member %q not found", name)
}

// Stop cancels the serf event delivery and removes the subscription.
func (r *Client) Stop(handle serf.StreamHandle) error {
	return r.client.Stop(handle)
}

// Join attempts to join an existing serf cluster identified by peers.
// Replay controls if previous user events are replayed once this node has joined the cluster.
// Returns the number of nodes joined
func (r *Client) Join(peers []string, replay bool) (int, error) {
	return r.client.Join(peers, replay)
}

// UpdateTags will modify the tags on a running serf agent
func (r *Client) UpdateTags(tags map[string]string, delTags []string) error {
	return r.client.UpdateTags(tags, delTags)
}

// Close closes the client
func (r *Client) Close() error {
	if r.client.IsClosed() {
		return nil
	}
	return r.client.Close()
}

// GetCoordinate returns the Serf Coordinate for a specific node
func (r *Client) GetCoordinate(node string) (*coordinate.Coordinate, error) {
	return r.client.GetCoordinate(node)
}

// filterLeft filters out members that have left the serf cluster
func filterLeft(members []serf.Member) (result []serf.Member) {
	result = make([]serf.Member, 0, len(members))
	for _, member := range members {
		if MemberStatus(member.Status) == MemberLeft {
			// Skip
			continue
		}
		result = append(result, member)
	}
	return result
}
