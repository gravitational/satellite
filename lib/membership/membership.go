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

// Package membership provides interface for cluster membership management.
package membership

import (
	pb "github.com/gravitational/satellite/agent/proto/agentpb"

	"github.com/hashicorp/serf/coordinate"
)

// ClusterMembership is interface to interact with a cluster membership service.
type ClusterMembership interface {
	// Members returns the list of cluster members.
	Members() ([]*pb.MemberStatus, error)
	// FindMember finds the member with the specified name.
	FindMember(name string) (*pb.MemberStatus, error)
	// Close closes the client.
	Close() error
	// Join attempts to join an existing cluster identified by peers.
	// Replay controls if previous user events are replayed once this node has joined the cluster.
	// Returns the number of nodes joined.
	Join(peers []string, replay bool) (int, error)
	// UpdateTags will modify the tags on a running member.
	UpdateTags(tags map[string]string, delTags []string) error
	// GetCoordinate returns the Serf Coordinate for a specific node
	GetCoordinate(node string) (*coordinate.Coordinate, error)
}
