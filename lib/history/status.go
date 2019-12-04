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
	"time"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"
)

// ClusterStatus represents the overall status of a cluster.
type ClusterStatus struct {
	*pb.SystemStatus
}

func (s *ClusterStatus) diffCluster(other *ClusterStatus) (events []Event) {
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
			events = append(events, oldNode.diffNode(newNode)...)
			delete(removed, name)
			continue
		}

		// Nodes added to the cluster
		event := NewNodeAdded(time.Now(), name)
		events = append(events, event)
		events = append(events, (&NodeStatus{}).diffNode(newNode)...)
	}

	// Nodes removed from the cluster
	for name := range removed {
		event := NewNodeRemoved(time.Now(), name)
		events = append(events, event)
	}

	// Compare cluster status
	if s.GetStatus() == other.GetStatus() {
		return events
	}

	if other.isRunning() {
		events = append(events, NewClusterRecovered(time.Now()))
		return events
	}

	events = append(events, NewClusterDegraded(time.Now()))
	return events
}

func (s *ClusterStatus) nodeMap() map[string]NodeStatus {
	nodes := make(map[string]NodeStatus, len(s.GetNodes()))
	for _, node := range s.GetNodes() {
		nodes[node.GetName()] = NodeStatus{node}
	}
	return nodes
}

func (s *ClusterStatus) isRunning() bool {
	return s.GetStatus() == pb.SystemStatus_Running
}

// NodeStatus represents the status of a node.
type NodeStatus struct {
	*pb.NodeStatus
}

func (s *NodeStatus) diffNode(other NodeStatus) (events []Event) {
	oldProbes := s.probeMap()
	newProbes := other.probeMap()

	for name, newProbe := range newProbes {
		if oldProbe, ok := oldProbes[name]; ok {
			events = append(events, oldProbe.diffProbe(other.GetName(), newProbe)...)
		}
	}

	// Compare node status
	if s.GetStatus() == other.GetStatus() {
		return events
	}

	if other.isRunning() {
		events = append(events, NewNodeRecovered(time.Now(), other.GetName()))
		return events
	}

	events = append(events, NewNodeRecovered(time.Now(), other.GetName()))
	return events
}

func (s *NodeStatus) probeMap() map[string]ProbeStatus {
	probes := make(map[string]ProbeStatus, len(s.GetProbes()))
	for _, probe := range s.GetProbes() {
		probes[probe.GetChecker()] = ProbeStatus{probe}
	}
	return probes
}

func (s *NodeStatus) isRunning() bool {
	return s.GetStatus() == pb.NodeStatus_Running
}

// ProbeStatus represents the result of a probe.
type ProbeStatus struct {
	*pb.Probe
}

func (s *ProbeStatus) diffProbe(nodeName string, other ProbeStatus) (events []Event) {
	if s.GetStatus() == other.GetStatus() {
		return events
	}

	if other.isPassing() {
		events = append(events, NewProbePassed(time.Now(), nodeName, other.GetChecker()))
		return events
	}

	events = append(events, NewProbeFailed(time.Now(), nodeName, other.GetChecker()))
	return events
}

func (s *ProbeStatus) isPassing() bool {
	return s.GetStatus() == pb.Probe_Running
}
