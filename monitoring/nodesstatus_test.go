/*
Copyright 2017 Gravitational, Inc.

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

package monitoring

import (
	"context"

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"

	. "gopkg.in/check.v1"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (_ *MonitoringSuite) TestDetectsNodeStatus(c *C) {
	var testCases = []struct {
		nodes    nodeList
		nodeName string
		comment  string
		probes   health.Probes
	}{
		{
			nodes: nodeList{
				Items: []v1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "foo"},
						Status: v1.NodeStatus{
							Conditions: []v1.NodeCondition{
								{Type: v1.NodeReady, Status: v1.ConditionFalse},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "bar"},
						Status: v1.NodeStatus{
							Conditions: []v1.NodeCondition{
								{Type: v1.NodeReady, Status: v1.ConditionTrue},
							},
						},
					},
				},
			},
			nodeName: "foo",
			probes: health.Probes{
				&pb.Probe{
					Checker: NodeStatusCheckerID,
					Status:  pb.Probe_Temporary,
					Error:   "Node is not ready",
				},
			},
			comment: "detects a not ready node",
		},
		{
			nodes: nodeList{
				Items: []v1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "foo"},
						Status: v1.NodeStatus{
							Conditions: []v1.NodeCondition{
								{Type: v1.NodeReady, Status: v1.ConditionTrue},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "bar"},
						Status: v1.NodeStatus{
							Conditions: []v1.NodeCondition{
								{Type: v1.NodeReady, Status: v1.ConditionTrue},
							},
						},
					},
				},
			},
			nodeName: "foo",
			probes: health.Probes{
				&pb.Probe{
					Checker: NodeStatusCheckerID,
					Status:  pb.Probe_Running,
				},
			},
			comment: "detects a healthy node",
		},
	}

	for _, testCase := range testCases {
		comment := Commentf(testCase.comment)
		checker := nodeStatusChecker{
			nodeLister: testCase.nodes,
			nodeName:   testCase.nodeName,
		}
		var probes health.Probes
		checker.Check(context.TODO(), &probes)
		c.Assert(probes, DeepEquals, testCase.probes, comment)
	}
}

func (r nodeList) Nodes() (*v1.NodeList, error) {
	return (*v1.NodeList)(&r), nil
}

type nodeList v1.NodeList
