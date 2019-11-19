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
	Events []Event
	// LastStatus holds the last recorded cluster status.
	LastStatus *Cluster
}

// NewMemTimeline initializes and returns a new MemTimeline with the specified
// size.
func NewMemTimeline(size int) Timeline {
	return &MemTimeline{
		Size:       size,
		Events:     make([]Event, 0, size),
		LastStatus: &Cluster{},
	}
}

// RecordStatus records differences of the previous status to the provided
// status into the Timeline.
func (t *MemTimeline) RecordStatus(status *pb.SystemStatus) {
	cluster := parseSystemStatus(status)
	events := t.LastStatus.diffCluster(cluster)
	for _, event := range events {
		t.addEvent(event)
	}
	t.LastStatus = cluster
}

// GetEvents returns the current timeline.
func (t *MemTimeline) GetEvents() []Event {
	return t.Events
}

// addEvent appends the provided event to the timeline.
func (t *MemTimeline) addEvent(event Event) {
	if len(t.Events) > t.Size {
		t.Events = t.Events[1:]
	}
	t.Events = append(t.Events, event)
}

// Cluster represents the overall status of a cluster.
type Cluster struct {
	// Status specifies the cluster status.
	Status string
	// Nodes specify the individual node statuses.
	Nodes map[string]*Node
}

// diffCluster calculates the differences from the provided cluster and returns
// the differences as a list of events.
func (c *Cluster) diffCluster(cluster *Cluster) []Event {
	events := []Event{}

	// Compare cluster status
	if c.Status != cluster.Status {
		var event Event
		if cluster.Status == pb.SystemStatus_Running.String() {
			event = NewClusterRecoveredEvent()
		} else {
			event = NewClusterDegradedEvent()
		}
		event.SetMetadata("old", c.Status)
		event.SetMetadata("new", cluster.Status)
		events = append(events, event)
	}

	// Keep track of removed nodes
	removed := map[string]bool{}
	for name := range c.Nodes {
		removed[name] = true
	}

	// Nodes added or modified
	for name, newNode := range cluster.Nodes {
		if oldNode, ok := c.Nodes[name]; !ok {
			event := NewNodeAddedEvent()
			event.SetMetadata("node", name)
			event.SetMetadata("new", newNode.Status)
			events = append(events, event)

			// Add new probes as well.
			events = append(events, (&Node{}).diffNode(newNode)...)
		} else {
			events = append(events, oldNode.diffNode(newNode)...)
			delete(removed, name)
		}
	}

	// Nodes removed from the cluster
	for name := range removed {
		event := NewNodeRemovedEvent()
		event.SetMetadata("node", name)
		event.SetMetadata("old", c.Nodes[name].Status)
		events = append(events, event)
	}

	return events
}

// Node represents the status of a node.
type Node struct {
	// Name specifies the name of the node.
	Name string
	// Status specifies the status of the node.
	Status string
	// MemberStatus specifies the node serf membership status.
	MemberStatus string
	// Probes specify the individual probe results.
	Probes map[string]*Probe
}

// diffNode calculates the differences from the provided node and returns the
// differences as a list of events.
func (n *Node) diffNode(node *Node) []Event {
	events := []Event{}

	// Compare node status
	if n.Status != node.Status {
		var event Event
		if node.Status == pb.NodeStatus_Running.String() {
			event = NewNodeRecoveredEvent()
		} else {
			event = NewNodeDegradedEvent()
		}
		event.SetMetadata("node", n.Name)
		event.SetMetadata("old", n.Status)
		event.SetMetadata("new", node.Status)
		events = append(events, event)
	}

	// Keep track of removed probes
	removed := map[string]bool{}
	for name := range n.Probes {
		removed[name] = true
	}

	// Probes added or modified
	for name, newProbe := range node.Probes {
		if oldProbe, ok := n.Probes[name]; !ok {
			event := NewProbeAddedEvent()
			event.SetMetadata("node", n.Name)
			event.SetMetadata("probe", name)
			event.SetMetadata("new", newProbe.Status)
			event.SetMetadata("detail", newProbe.Detail)
			events = append(events, event)
		} else {
			events = append(events, oldProbe.diffProbe(n.Name, newProbe)...)
			delete(removed, name)
		}
	}

	// Probes removed from the node
	for name := range removed {
		event := NewProbeRemovedEvent()
		event.SetMetadata("node", n.Name)
		event.SetMetadata("old", n.Probes[name].Status)
		events = append(events, event)
	}

	return events
}

// Probe represents the result of a probe.
// TODO: What fields do we need to store?
// Available fields:
// - Checker -> Name
// - CheckerData
// - Code
// - Detail -> Detail
// - Error
// - Severity
// - Status -> Status
type Probe struct {
	// Name specifies the type of probe.
	Name string
	// Status specifies the result of the probe.
	Status string
	// Detail specifies any specific details attached to the probe.
	Detail string
}

// diffProbe calculates the differences from the provided probe and returns the
// differences as a list of events.
func (p *Probe) diffProbe(nodeName string, probe *Probe) []Event {
	events := []Event{}
	if p.Status != probe.Status {
		var event Event
		if probe.Status == pb.Probe_Running.String() {
			event = NewProbePassedEvent()
		} else {
			event = NewProbeFailedEvent()
		}
		event.SetMetadata("node", nodeName)
		event.SetMetadata("probe", p.Name)
		event.SetMetadata("old", p.Status)
		event.SetMetadata("new", probe.Status)
		event.SetMetadata("detail", probe.Detail)
		events = append(events, event)
	}
	return events
}

// parseSystemStatus parses and returns the systemStatus as a Cluster.
func parseSystemStatus(status *pb.SystemStatus) *Cluster {
	cluster := &Cluster{
		Status: status.GetStatus().String(),
	}

	nodes := map[string]*Node{}
	for _, node := range status.GetNodes() {
		nodes[node.GetName()] = parseNodeStatus(node)
	}

	cluster.Nodes = nodes
	return cluster
}

// parseNodeStatus parses and returns the nodeStatus as a Node.
func parseNodeStatus(nodeStatus *pb.NodeStatus) *Node {
	node := &Node{
		Name:   nodeStatus.GetName(),
		Status: nodeStatus.GetMemberStatus().GetStatus().String(),
	}

	probes := map[string]*Probe{}
	for _, probe := range nodeStatus.GetProbes() {
		probes[probe.GetChecker()] = parseProbeStatus(probe)
	}

	node.Probes = probes
	return node
}

// parseProbeStatus parses and returns the probeStatus as a Probe.
func parseProbeStatus(probeStatus *pb.Probe) *Probe {
	return &Probe{
		Name:   probeStatus.GetChecker(),
		Status: probeStatus.GetStatus().String(),
		Detail: probeStatus.GetDetail(),
	}
}
