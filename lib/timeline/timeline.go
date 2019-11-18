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

// Package timeline provides interfaces for keeping track of cluster status events.
package timeline

import (
	"fmt"
	"strings"
	"time"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"
)

// Stamp defines default timestamp format for events.
const Stamp = "Jan _2 15:04:05"

// Event represents a Timeline event. An event occurrs  whenever there is a
// change in the cluster status. An event could be triggered by a failed
// heartbeat or a failed health check.
type Event struct {
	// TimeStamp specifies when the event occurred.
	TimeStamp time.Time

	// TODO: can we rely on user CLI sessions to provide colored text?
	// If not, maybe we should indicate the different event types a different
	// way.

	// Color specifies the color of the event.
	// Red -> Indicates change has caused state to degraded.
	// Yellow -> Indicates change has not caused any state changes.
	// Green -> Indicates changes has caused state to recover.
	Color string

	// Description specifies a description of the event.
	Description string
}

// NewEvent initializes and returns a new Event with the current timestamp.
func NewEvent() Event {
	return Event{
		TimeStamp: time.Now(),
	}
}

// String returns a string representation of Event.
func (e *Event) String() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[%s] ", e.TimeStamp.Format(Stamp)))
	sb.WriteString(e.Description)
	return sb.String()
}

// Timeline represents a timeline of cluster status events. The Timeline
// can hold a specified amount of events and uses a FIFO eviction policy.
type Timeline struct {
	// Size specifies the max size of the timeline.
	Size int
	// Timeline holds the latest status events.
	Timeline []string
	// Cluster holds the latest cluster status.
	Cluster *Cluster
}

// NewTimeline initializes and returns a new StatusTimeline with the
// specified size.
func NewTimeline(size int) Timeline {
	return Timeline{
		Size:     size,
		Timeline: []string{},
		Cluster:  &Cluster{},
	}
}

// RecordStatus records differences of the previous status to the provided
// status into the Timeline.
func (t *Timeline) RecordStatus(status *pb.SystemStatus) {
	cluster := parseSystemStatus(status)
	events := t.Cluster.diffCluster(cluster)
	for _, event := range events {
		t.addEvent(event)
	}
	t.Cluster = cluster
}

// GetTimeline returns the current timeline.
func (t *Timeline) GetTimeline() []string {
	return t.Timeline
}

// addEvent appends the provided event to the timeline.
func (t *Timeline) addEvent(event Event) {
	if len(t.Timeline) > t.Size {
		t.Timeline = t.Timeline[1:]
	}
	t.Timeline = append(t.Timeline, event.String())
}

// Cluster represents the overall status of a cluster.
type Cluster struct {
	// Status specifies the cluster status.
	Status string
	// Nodes specify the individual node statuses.
	Nodes map[string]*Node
}

// difCluster calculates the differences from the provided cluster and returns
// the differences as a list of events.
func (c *Cluster) diffCluster(cluster *Cluster) []Event {
	events := []Event{}

	// Compare cluster status
	if c.Status != cluster.Status {
		event := NewEvent()
		event.Description = fmt.Sprintf("cluster status changed from [%s] to [%s]", c.Status, cluster.Status)
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
			event := NewEvent()
			event.Description = fmt.Sprintf("[%s] has been added to the cluster", name)
			events = append(events, event)
		} else {
			events = append(events, oldNode.diffNode(newNode)...)
			delete(removed, name)
		}
	}

	// Nodes removed from the cluster
	for name := range removed {
		event := NewEvent()
		event.Description = fmt.Sprintf("[%s] has been removed from the cluster", name)
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
		event := NewEvent()
		event.Description = fmt.Sprintf("[%s] status changed from [%s] to [%s]", n.Name, n.Status, node.Status)
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
			event := NewEvent()
			event.Description = fmt.Sprintf("[%s:%s] probe has been added to node [%s]", n.Name, name, n.Name)
			events = append(events, event)
		} else {
			events = append(events, oldProbe.diffProbe(n.Name, newProbe)...)
			delete(removed, name)
		}
	}

	// Probes removed from the node
	for name := range removed {
		event := NewEvent()
		event.Description = fmt.Sprintf("[%s:%s] probe has been removed from node [%s]", n.Name, name, n.Name)
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
		event := NewEvent()
		desc := fmt.Sprintf("[%s:%s] status changed from [%s] to [%s]", nodeName, p.Name, p.Status, probe.Status)
		event.Description = desc
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
