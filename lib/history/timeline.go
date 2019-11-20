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

// Package history provides interfaces for keeping track of cluster status timeline.
package history

import (
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
)

// Timeline can be used to record changes in the system status and retrieve them
// as a list of Events.
type Timeline interface {
	// RecordStatus records any changes that have occurred since the previous
	// recorded status.
	RecordStatus(status *pb.SystemStatus)
	// GetEvents returns the currently stored list of events.
	GetEvents() []*Event
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
func (c *Cluster) diffCluster(cluster *Cluster) []*Event {
	events := []*Event{}

	// Compare cluster status
	if c.Status != cluster.Status {
		var event *Event
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

	for name, newNode := range cluster.Nodes {
		// Nodes modified
		if oldNode, ok := c.Nodes[name]; ok {
			events = append(events, oldNode.diffNode(newNode)...)
			delete(removed, name)
			continue
		}

		// Nodes added to the cluster
		event := NewNodeAddedEvent()
		event.SetMetadata("node", name)
		event.SetMetadata("new", newNode.Status)
		events = append(events, event)

		// Add new probes as well.
		events = append(events, (&Node{}).diffNode(newNode)...)
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
func (n *Node) diffNode(node *Node) []*Event {
	events := []*Event{}

	// Compare node status
	if n.Status != node.Status {
		var event *Event
		if node.Status == pb.NodeStatus_Running.String() {
			event = NewNodeRecoveredEvent()
		} else {
			event = NewNodeDegradedEvent()
		}
		event.SetMetadata("node", node.Name)
		event.SetMetadata("old", n.Status)
		event.SetMetadata("new", node.Status)
		events = append(events, event)
	}

	// Keep track of removed probes
	removed := map[string]bool{}
	for name := range n.Probes {
		removed[name] = true
	}

	for name, newProbe := range node.Probes {
		// Probes modified
		if oldProbe, ok := n.Probes[name]; ok {
			events = append(events, oldProbe.diffProbe(node.Name, newProbe)...)
			delete(removed, name)
			continue
		}

		// Probes added to the node
		event := NewProbeAddedEvent()
		event.SetMetadata("node", node.Name)
		event.SetMetadata("probe", name)
		event.SetMetadata("new", newProbe.Status)
		event.SetMetadata("detail", newProbe.Detail)
		events = append(events, event)
	}

	// Probes removed from the node
	for name := range removed {
		event := NewProbeRemovedEvent()
		event.SetMetadata("node", node.Name)
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
func (p *Probe) diffProbe(nodeName string, probe *Probe) []*Event {
	events := []*Event{}
	if p.Status != probe.Status {
		var event *Event
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