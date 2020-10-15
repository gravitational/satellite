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

package kubernetes

import (
	"testing"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/lib/test"

	. "gopkg.in/check.v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNodes(t *testing.T) { TestingT(t) }

type NodesSuite struct{}

var _ = Suite(&NodesSuite{})

// TestMembers verfies members can be queried.
func (r *NodesSuite) TestMembers(c *C) {
	tests := []struct {
		comment  string
		nodes    []v1.Node
		expected []*pb.MemberStatus
	}{
		{
			comment: "List all nodes",
			nodes: []v1.Node{
				r.newNode("satellite-1", "192.168.1.101", map[string]string{"role": "master"}),
				r.newNode("satellite-2", "192.168.1.102", map[string]string{"role": "master"}),
			},
			expected: []*pb.MemberStatus{
				pb.NewMemberStatus("satellite-1", "192.168.1.101", map[string]string{"role": "master"}),
				pb.NewMemberStatus("satellite-2", "192.168.1.102", map[string]string{"role": "master"}),
			},
		},
	}

	for _, tc := range tests {
		comment := Commentf(tc.comment)

		cluster, err := NewClusterNodes(&NodesConfig{
			KubeClient: fake.NewSimpleClientset(
				&v1.NodeList{Items: tc.nodes},
			),
		})
		c.Assert(err, IsNil, comment)

		members, err := cluster.Members()
		c.Assert(err, IsNil, comment)
		c.Assert(members, test.DeepCompare, tc.expected, comment)
	}
}

// TestMember verifies single member can be queried.
func (r *NodesSuite) TestMember(c *C) {
	tests := []struct {
		comment  string
		name     string
		nodes    []v1.Node
		expected *pb.MemberStatus
	}{
		{
			comment: "Query satellite-1",
			name:    "satellite-1",
			nodes: []v1.Node{
				r.newNode("satellite-1", "192.168.1.101", map[string]string{"role": "master"}),
			},
			expected: pb.NewMemberStatus("satellite-1", "192.168.1.101", map[string]string{"role": "master"}),
		},
	}
	for _, tc := range tests {
		comment := Commentf(tc.comment)

		cluster, err := NewClusterNodes(&NodesConfig{
			KubeClient: fake.NewSimpleClientset(
				&v1.NodeList{Items: tc.nodes},
			),
		})
		c.Assert(err, IsNil, comment)

		member, err := cluster.Member(tc.name)
		c.Assert(err, IsNil, comment)
		c.Assert(member, test.DeepCompare, tc.expected, comment)
	}
}

// newNode constructs a new pod.
func (r *NodesSuite) newNode(name, addr string, annotations map[string]string) v1.Node {
	return v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Annotations: annotations,
			Labels:      map[string]string{DefaultAdvertiseIPKey: addr},
		},
	}
}
