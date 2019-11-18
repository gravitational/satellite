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

package timeline

import (
	"fmt"
	"strings"
	"time"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"
)

// Stamp defines timestamp format
const Stamp = "Jan _2 15:04:05"

// Entry represents a Timeline entry.
type Entry struct {
	TimeStamp time.Time
	Instance  string
	Task      string
	Check     string
	Old       string
	New       string
	Msg       string
}

// Labels represents an Entry's labels
// type Labels map[string]string

// NewEntry initializes and returns a new Entry with a timestamp.
func NewEntry() *Entry {
	return &Entry{TimeStamp: time.Now()}
}

// String returns string representation of Entry.
func (e *Entry) String() string {
	var sb strings.Builder
	sb.WriteString("{")
	sb.WriteString(fmt.Sprintf("timestamp=%s", e.TimeStamp.Format(Stamp)))

	if e.Instance != "" {
		sb.WriteString(fmt.Sprintf(", instance=%s", e.Instance))
	}

	if e.Task != "" {
		sb.WriteString(fmt.Sprintf(", task=%s", e.Task))
	}

	if e.Check != "" {
		sb.WriteString(fmt.Sprintf(", check=%s", e.Check))
	}

	if e.Old != "" {
		sb.WriteString(fmt.Sprintf(", old=%s", e.Old))
	}

	if e.New != "" {
		sb.WriteString(fmt.Sprintf(", new=%s", e.New))
	}

	if e.Msg != "" {
		sb.WriteString(fmt.Sprintf(", msg=%s", e.Msg))
	}

	sb.WriteString("}")
	return sb.String()
}

// Timeline represents a timeline of gravity statuses. The Timeline
// can hold a specified amount of entries and uses a FIFO eviction policy.
type Timeline struct {
	// size specifies the max size of the timeline
	size int
	// timeline holds the latest status entries
	timeline []string
	// cluster holds the latest cluster status
	cluster *Cluster
}

// NewTimeline initializes and returns a new StatusTimeline with the
// specified size.
func NewTimeline(size int) *Timeline {
	return &Timeline{
		size:     size,
		timeline: []string{},
		cluster:  &Cluster{},
	}
}

// RecordStatus records differences of the previous status to the provided
// status into the Timeline.
func (t *Timeline) RecordStatus(status *pb.SystemStatus) {
	cluster := parseSystemStatus(status)
	entries := t.cluster.diffCluster(cluster)
	for _, entry := range entries {
		t.addEntry(entry)
	}
	t.cluster = cluster
}

// GetTimeline returns the current timeline.
func (t *Timeline) GetTimeline() []string {
	return t.timeline
}

// addEntry appends the provided entry to the timeline.
func (t *Timeline) addEntry(entry string) {
	if len(t.timeline) > t.size {
		t.timeline = t.timeline[1:]
	}
	t.timeline = append(t.timeline, entry)
}

// Cluster represents the overall status of a cluster.
type Cluster struct {
	Status string
	Nodes  map[string]*Node
}

func (c *Cluster) diffCluster(cluster *Cluster) []string {
	entries := []string{}

	// Compare cluster status
	if c.Status != cluster.Status {
		entry := NewEntry()
		entry.Task = "cluster_status"
		entry.Old = c.Status
		entry.New = cluster.Status
		entries = append(entries, entry.String())
	}

	// Keep track of removed nodes
	removed := map[string]bool{}
	for name := range c.Nodes {
		removed[name] = true
	}

	// Nodes added or modified
	for name, newNode := range cluster.Nodes {
		if oldNode, ok := c.Nodes[name]; !ok {
			entry := NewEntry()
			entry.Instance = name
			entry.Task = "heartbeat"
			entry.New = "success"
			entries = append(entries, entry.String())
		} else {
			entries = append(entries, oldNode.diffNode(newNode)...)
			delete(removed, name)
		}
	}

	// Nodes removed from the cluster
	for name := range removed {
		entry := NewEntry()
		entry.Instance = name
		entry.Task = "heartbeat"
		entry.Old = "success"
		entries = append(entries, entry.String())
	}

	return entries
}

// Node represents the status of a node.
type Node struct {
	Name   string
	Status string
	Probes map[string]*Probe
}

func (n *Node) diffNode(node *Node) []string {
	entries := []string{}

	// Compare node status
	if n.Status != node.Status {
		entry := NewEntry()
		entry.Instance = n.Name
		entry.Task = "node_status"
		entry.Old = n.Status
		entry.New = node.Status
		entries = append(entries, entry.String())
	}

	// Keep track of removed probes
	removed := map[string]bool{}
	for name := range n.Probes {
		removed[name] = true
	}

	// Probes added or modified
	for name, newProbe := range node.Probes {
		if oldProbe, ok := n.Probes[name]; !ok {
			entry := NewEntry()
			entry.Instance = n.Name
			entry.Task = "health_check"
			entry.Check = name
			entry.New = newProbe.Status
			entry.Msg = newProbe.Message
			entries = append(entries, entry.String())
		} else {
			entries = append(entries, oldProbe.diffProbe(n.Name, newProbe)...)
			delete(removed, name)
		}
	}

	// Probes removed from the node
	for name := range removed {
		entry := NewEntry()
		entry.Instance = n.Name
		entry.Task = "health_check"
		entry.Check = name
		entry.Old = n.Probes[name].Status
		entries = append(entries, entry.String())
	}

	return entries
}

// Probe represents the result of a probe.
type Probe struct {
	Name    string
	Status  string
	Message string
}

func (p *Probe) diffProbe(nodeName string, probe *Probe) []string {
	entries := []string{}
	if p.Status != probe.Status {
		entry := NewEntry()
		entry.Instance = nodeName
		entry.Task = "health_check"
		entry.Check = p.Name
		entry.Old = p.Status
		entry.New = probe.Status
		entry.Msg = probe.Message
		entries = append(entries, entry.String())
	}
	return entries
}

//// UTIL ////

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

func parseNodeStatus(nodeStatus *pb.NodeStatus) *Node {
	node := &Node{
		Name:   nodeStatus.GetName(),
		Status: nodeStatus.GetStatus().String(),
	}

	probes := map[string]*Probe{}
	for _, probe := range nodeStatus.GetProbes() {
		probes[probe.GetChecker()] = parseProbeStatus(probe)
	}

	node.Probes = probes
	return node
}

func parseProbeStatus(probeStatus *pb.Probe) *Probe {
	return &Probe{
		Name:    probeStatus.GetChecker(),
		Status:  probeStatus.GetStatus().String(),
		Message: probeStatus.GetDetail(),
	}
}
