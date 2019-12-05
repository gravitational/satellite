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

	. "gopkg.in/check.v1"
)

type StatusSuite struct {
	clock clockwork.FakeClock
}

var _ = Suite(&StatusSuite{})

func (s *StatusSuite) SetUpSuite(c *C) {
	s.clock = clockwork.NewFakeClock()
}

func (s *StatusSuite) TestClusterStatusDiff(c *C) {
	clusterEvents := []struct {
		oldCluster *pb.SystemStatus
		newCluster *pb.SystemStatus
		diff       []*pb.TimelineEvent
		comment    string
	}{
		{
			oldCluster: &pb.SystemStatus{Status: pb.SystemStatus_Degraded},
			newCluster: &pb.SystemStatus{Status: pb.SystemStatus_Running},
			diff:       []*pb.TimelineEvent{NewClusterRecovered(s.clock.Now())},
			comment:    "Test cluster recovered",
		},
		{
			oldCluster: &pb.SystemStatus{Status: pb.SystemStatus_Running},
			newCluster: &pb.SystemStatus{Status: pb.SystemStatus_Degraded},
			diff:       []*pb.TimelineEvent{NewClusterDegraded(s.clock.Now())},
			comment:    "Test cluster degraded",
		},
	}

	for _, event := range clusterEvents {
		actual := diffCluster(s.clock, event.oldCluster, event.newCluster)
		c.Assert(actual, DeepEquals, event.diff, Commentf(event.comment))
	}
}

func (s *StatusSuite) TestAddOrRemoveNode(c *C) {
	clusterEvents := []struct {
		oldCluster *pb.SystemStatus
		newCluster *pb.SystemStatus
		diff       []*pb.TimelineEvent
		comment    string
	}{
		{
			oldCluster: &pb.SystemStatus{},
			newCluster: &pb.SystemStatus{
				Nodes: []*pb.NodeStatus{&pb.NodeStatus{Name: "node-added"}},
			},
			diff:    []*pb.TimelineEvent{NewNodeAdded(s.clock.Now(), "node-added")},
			comment: "Test node added",
		},
		{

			oldCluster: &pb.SystemStatus{
				Nodes: []*pb.NodeStatus{&pb.NodeStatus{Name: "node-removed"}},
			},
			newCluster: &pb.SystemStatus{},
			diff:       []*pb.TimelineEvent{NewNodeRemoved(s.clock.Now(), "node-removed")},
			comment:    "Test node removed",
		},
	}

	for _, event := range clusterEvents {
		actual := diffCluster(s.clock, event.oldCluster, event.newCluster)
		c.Assert(actual, DeepEquals, event.diff, Commentf(event.comment))
	}
}

func (s *StatusSuite) TestNodeStatusDiff(c *C) {
	nodeEvents := []struct {
		oldNode *pb.NodeStatus
		newNode *pb.NodeStatus
		diff    []*pb.TimelineEvent
		comment string
	}{
		{
			oldNode: &pb.NodeStatus{Name: "node-recovered", Status: pb.NodeStatus_Degraded},
			newNode: &pb.NodeStatus{Name: "node-recovered", Status: pb.NodeStatus_Running},
			diff:    []*pb.TimelineEvent{NewNodeRecovered(s.clock.Now(), "node-recovered")},
			comment: "Test node recovered",
		},
		{
			oldNode: &pb.NodeStatus{Name: "node-degraded", Status: pb.NodeStatus_Running},
			newNode: &pb.NodeStatus{Name: "node-degraded", Status: pb.NodeStatus_Degraded},
			diff:    []*pb.TimelineEvent{NewNodeDegraded(s.clock.Now(), "node-degraded")},
			comment: "Test node degraded",
		},
	}

	for _, event := range nodeEvents {
		actual := diffNode(s.clock, event.oldNode, event.newNode)
		c.Assert(actual, DeepEquals, event.diff, Commentf(event.comment))
	}
}

func (s *StatusSuite) TestProbeDiff(c *C) {
	probeEvents := []struct {
		nodeName string
		oldProbe *pb.Probe
		newProbe *pb.Probe
		diff     []*pb.TimelineEvent
		comment  string
	}{
		{
			nodeName: "test-node",
			oldProbe: &pb.Probe{Checker: "probe-successful", Status: pb.Probe_Failed},
			newProbe: &pb.Probe{Checker: "probe-successful", Status: pb.Probe_Running},
			diff:     []*pb.TimelineEvent{NewProbeSucceeded(s.clock.Now(), "test-node", "probe-successful")},
			comment:  "Test successful probe event",
		},
		{
			nodeName: "test-node",
			oldProbe: &pb.Probe{Checker: "probe-failure", Status: pb.Probe_Running},
			newProbe: &pb.Probe{Checker: "probe-failure", Status: pb.Probe_Failed},
			diff:     []*pb.TimelineEvent{NewProbeFailed(s.clock.Now(), "test-node", "probe-failure")},
			comment:  "Test failure probe event",
		},
	}

	for _, event := range probeEvents {
		actual := diffProbe(s.clock, event.nodeName, event.oldProbe, event.newProbe)
		c.Assert(actual, DeepEquals, event.diff, Commentf(event.comment))
	}
}
