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
	pb "github.com/gravitational/satellite/agent/proto/agentpb"

	"github.com/jonboulle/clockwork"
)

// ClusterStatus represents the overall status of a cluster.
type ClusterStatus struct {
	*pb.SystemStatus
}

// NewClusterStatus constructs a new ClusterStatus.
func NewClusterStatus(status *pb.SystemStatus) ClusterStatus {
	return ClusterStatus{SystemStatus: status}
}

// diffCluster calculates the differences between a previous cluster status and
// a new cluster status. The differences are returned as a list of Events.
func (s ClusterStatus) diffCluster(clock clockwork.Clock, other ClusterStatus) (events []Event) {
	oldNodes := s.nodeMap()
	newNodes := other.nodeMap()

	// Keep track of removed nodes
	removed := map[string]bool{}
	for name := range oldNodes {
		removed[name] = true
	}

	for name, newNode := range newNodes {
		// Nodes modified
		if oldNode, ok := oldNodes[name]; ok {
			events = append(events, oldNode.diffNode(clock, newNode)...)
			delete(removed, name)
			continue
		}

		// Nodes added to the cluster
		event := NewNodeAdded(clock.Now(), name)
		events = append(events, event)
		events = append(events, NewNodeStatus(nil).diffNode(clock, newNode)...)
	}

	// Nodes removed from the cluster
	for name := range removed {
		event := NewNodeRemoved(clock.Now(), name)
		events = append(events, event)
	}

	// Compare cluster status
	if s.GetStatus() == other.GetStatus() {
		return events
	}

	if other.isRunning() {
		events = append(events, NewClusterRecovered(clock.Now()))
		return events
	}

	events = append(events, NewClusterDegraded(clock.Now()))
	return events
}

// nodeMap returns the cluster's list of nodes as a map with each node mapped
// to its name.
func (s ClusterStatus) nodeMap() map[string]NodeStatus {
	nodes := make(map[string]NodeStatus, len(s.GetNodes()))
	for _, node := range s.GetNodes() {
		nodes[node.GetName()] = NewNodeStatus(node)
	}
	return nodes
}

// isRunning returns true if the cluster has a running status.
func (s ClusterStatus) isRunning() bool {
	return s.GetStatus() == pb.SystemStatus_Running
}

// NodeStatus represents the status of a node.
type NodeStatus struct {
	*pb.NodeStatus
}

// NewNodeStatus constructs a new NodeStatus.
func NewNodeStatus(status *pb.NodeStatus) NodeStatus {
	return NodeStatus{NodeStatus: status}
}

// diffNode calculates the differences between a previous node status and a new
// node status. The differences are returned as a list of Events.
func (s NodeStatus) diffNode(clock clockwork.Clock, other NodeStatus) (events []Event) {
	oldProbes := s.probeMap()
	newProbes := other.probeMap()

	for name, newProbe := range newProbes {
		if oldProbe, ok := oldProbes[name]; ok {
			events = append(events, oldProbe.diffProbe(clock, other.GetName(), newProbe)...)
		}
	}

	// Compare node status
	if s.GetStatus() == other.GetStatus() {
		return events
	}

	if other.isRunning() {
		return append(events, NewNodeRecovered(clock.Now(), other.GetName()))
	}

	return append(events, NewNodeDegraded(clock.Now(), other.GetName()))
}

// probeMap returns the node's list of probes as a map with each probe mapped
// to its name.
func (s NodeStatus) probeMap() map[string]ProbeStatus {
	probes := make(map[string]ProbeStatus, len(s.GetProbes()))
	for _, probe := range s.GetProbes() {
		probes[probe.GetChecker()] = NewProbeStatus(probe)
	}
	return probes
}

// isRunning returns true if the node has a running status.
func (s NodeStatus) isRunning() bool {
	return s.GetStatus() == pb.NodeStatus_Running
}

// ProbeStatus represents the result of a probe.
type ProbeStatus struct {
	*pb.Probe
}

// NewProbeStatus constructs a new ProbeStatus.
func NewProbeStatus(status *pb.Probe) ProbeStatus {
	return ProbeStatus{Probe: status}
}

// diffProbe calculates the differences between a previous probe and a new
// probe. The differences are returned as a list of Events. The provided
// nodeName is used to specify which node the probes belong to.
func (s ProbeStatus) diffProbe(clock clockwork.Clock, nodeName string, other ProbeStatus) (events []Event) {
	if s.GetStatus() == other.GetStatus() {
		return events
	}

	if other.isSuccessful() {
		return append(events, NewProbeSucceeded(clock.Now(), nodeName, other.GetChecker()))
	}

	return append(events, NewProbeFailed(clock.Now(), nodeName, other.GetChecker()))
}

// isSuccessful returns true if the probe has a running status.
func (s ProbeStatus) isSuccessful() bool {
	return s.GetStatus() == pb.Probe_Running
}
