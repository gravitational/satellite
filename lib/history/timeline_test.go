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

	. "gopkg.in/check.v1"
)

func (s *HistorySuite) TestClusterStatusDiff(c *C) {
	clusterEvents := []struct {
		oldCluster *Cluster
		newCluster *Cluster
		diff       []*Event
		comment    string
	}{
		{
			oldCluster: &Cluster{Status: pb.SystemStatus_Degraded.String()},
			newCluster: &Cluster{Status: pb.SystemStatus_Running.String()},
			diff: []*Event{
				{
					eventType: ClusterRecovered,
					metadata: map[string]string{
						"old": pb.SystemStatus_Degraded.String(),
						"new": pb.SystemStatus_Running.String(),
					},
				},
			},
			comment: "Test cluster recovered",
		},
		{
			oldCluster: &Cluster{Status: pb.SystemStatus_Running.String()},
			newCluster: &Cluster{Status: pb.SystemStatus_Degraded.String()},
			diff: []*Event{
				{
					eventType: ClusterDegraded,
					metadata: map[string]string{
						"old": pb.SystemStatus_Running.String(),
						"new": pb.SystemStatus_Degraded.String(),
					},
				},
			},
			comment: "Test cluster degraded",
		},
	}

	for _, event := range clusterEvents {
		actual := event.oldCluster.diffCluster(event.newCluster)
		removeTimestamps(actual)
		c.Assert(actual, DeepEquals, event.diff, Commentf(event.comment))
	}
}

func (s *HistorySuite) TestAddOrRemoveNode(c *C) {
	clusterEvents := []struct {
		oldCluster *Cluster
		newCluster *Cluster
		diff       []*Event
		comment    string
	}{
		{
			oldCluster: &Cluster{Status: pb.SystemStatus_Running.String()},
			newCluster: &Cluster{
				Status: pb.SystemStatus_Running.String(),
				Nodes:  map[string]*Node{"node-added": {Name: "node-added"}},
			},
			diff: []*Event{
				{
					eventType: NodeAdded,
					metadata: map[string]string{
						"node": "node-added",
					},
				},
			},
			comment: "Test node added",
		},
		{
			oldCluster: &Cluster{
				Status: pb.SystemStatus_Running.String(),
				Nodes:  map[string]*Node{"node-removed": {Name: "node-removed"}},
			},
			newCluster: &Cluster{Status: pb.SystemStatus_Running.String()},
			diff: []*Event{
				{
					eventType: NodeRemoved,
					metadata: map[string]string{
						"node": "node-removed",
					},
				},
			},
			comment: "Test node removed",
		},
	}

	for _, event := range clusterEvents {
		actual := event.oldCluster.diffCluster(event.newCluster)
		removeTimestamps(actual)
		c.Assert(actual, DeepEquals, event.diff, Commentf(event.comment))
	}
}

func (s *HistorySuite) TestNodeStatusDiff(c *C) {
	nodeEvents := []struct {
		oldNode *Node
		newNode *Node
		diff    []*Event
		comment string
	}{
		{
			oldNode: &Node{Name: "node-recovered", Status: pb.NodeStatus_Degraded.String()},
			newNode: &Node{Name: "node-recovered", Status: pb.NodeStatus_Running.String()},
			diff: []*Event{
				{
					eventType: NodeRecovered,
					metadata: map[string]string{
						"node": "node-recovered",
						"old":  pb.NodeStatus_Degraded.String(),
						"new":  pb.NodeStatus_Running.String(),
					},
				},
			},
			comment: "Test node recovered",
		},
		{
			oldNode: &Node{Name: "node-degraded", Status: pb.NodeStatus_Running.String()},
			newNode: &Node{Name: "node-degraded", Status: pb.NodeStatus_Degraded.String()},
			diff: []*Event{
				{
					eventType: NodeDegraded,
					metadata: map[string]string{
						"node": "node-degraded",
						"old":  pb.NodeStatus_Running.String(),
						"new":  pb.NodeStatus_Degraded.String(),
					},
				},
			},
			comment: "Test node degraded",
		},
	}

	for _, event := range nodeEvents {
		actual := event.oldNode.diffNode(event.newNode)
		removeTimestamps(actual)
		c.Assert(actual, DeepEquals, event.diff, Commentf(event.comment))
	}
}

func (s *HistorySuite) TestProbeDiff(c *C) {
	probeEvents := []struct {
		nodeName string
		oldProbe *Probe
		newProbe *Probe
		diff     []*Event
		comment  string
	}{
		{
			nodeName: "test-node",
			oldProbe: &Probe{Name: "test-probe", Status: pb.Probe_Running.String()},
			newProbe: &Probe{Name: "test-probe", Status: pb.Probe_Failed.String()},
			diff: []*Event{
				{
					eventType: ProbeFailed,
					metadata: map[string]string{
						"node":  "test-node",
						"probe": "test-probe",
						"old":   pb.Probe_Running.String(),
						"new":   pb.Probe_Failed.String(),
					},
				},
			},
			comment: "Test failed probe event",
		},
		{
			nodeName: "test-node",
			oldProbe: &Probe{Name: "test-probe", Status: pb.Probe_Failed.String()},
			newProbe: &Probe{Name: "test-probe", Status: pb.Probe_Running.String()},
			diff: []*Event{
				{
					eventType: ProbePassed,
					metadata: map[string]string{
						"node":  "test-node",
						"probe": "test-probe",
						"old":   pb.Probe_Failed.String(),
						"new":   pb.Probe_Running.String(),
					},
				},
			},
			comment: "Test passed probe event",
		},
	}

	for _, event := range probeEvents {
		actual := event.oldProbe.diffProbe(event.nodeName, event.newProbe)
		removeTimestamps(actual)
		c.Assert(actual, DeepEquals, event.diff, Commentf(event.comment))
	}
}

func (s *HistorySuite) TestParseCluster(c *C) {
	actual := parseSystemStatus(&pb.SystemStatus{
		Status: pb.SystemStatus_Running,
		Nodes: []*pb.NodeStatus{
			{
				Name:   "test-node1",
				Status: pb.NodeStatus_Running,
				Probes: []*pb.Probe{
					{Checker: "test-probe1", Status: pb.Probe_Running},
					{Checker: "test-probe2", Status: pb.Probe_Failed},
				},
			},
			{
				Name:   "test-node2",
				Status: pb.NodeStatus_Degraded,
				Probes: []*pb.Probe{
					{Checker: "test-probe1", Status: pb.Probe_Running},
					{Checker: "test-probe2", Status: pb.Probe_Failed},
				},
			},
		},
	})

	expected := &Cluster{
		Status: pb.SystemStatus_Running.String(),
		Nodes: map[string]*Node{
			"test-node1": {
				Name:   "test-node1",
				Status: pb.NodeStatus_Running.String(),
				Probes: map[string]*Probe{
					"test-probe1": {Name: "test-probe1", Status: pb.Probe_Running.String()},
					"test-probe2": {Name: "test-probe2", Status: pb.Probe_Failed.String()},
				},
			},
			"test-node2": {
				Name:   "test-node2",
				Status: pb.NodeStatus_Degraded.String(),
				Probes: map[string]*Probe{
					"test-probe1": {Name: "test-probe1", Status: pb.Probe_Running.String()},
					"test-probe2": {Name: "test-probe2", Status: pb.Probe_Failed.String()},
				},
			},
		},
	}

	c.Assert(actual, DeepEquals, expected)
}

func (s *HistorySuite) TestParseNode(c *C) {
	actual := parseNodeStatus(&pb.NodeStatus{
		Name:   "test-node",
		Status: pb.NodeStatus_Running,
		Probes: []*pb.Probe{
			{Checker: "test-probe1", Status: pb.Probe_Running},
			{Checker: "test-probe2", Status: pb.Probe_Failed},
		},
	})

	expected := &Node{
		Name:   "test-node",
		Status: pb.NodeStatus_Running.String(),
		Probes: map[string]*Probe{
			"test-probe1": {Name: "test-probe1", Status: pb.Probe_Running.String()},
			"test-probe2": {Name: "test-probe2", Status: pb.Probe_Failed.String()},
		},
	}

	c.Assert(actual, DeepEquals, expected)
}

func (s *HistorySuite) TestParseProbe(c *C) {
	actual := parseProbeStatus(&pb.Probe{
		Checker: "test-probe",
		Status:  pb.Probe_Running,
	})

	expected := &Probe{Name: "test-probe", Status: pb.Probe_Running.String()}

	c.Assert(actual, DeepEquals, expected)
}
