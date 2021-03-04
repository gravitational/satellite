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
	"sort"
	"testing"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/lib/test"

	. "gopkg.in/check.v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
)

func TestNodes(t *testing.T) { TestingT(t) }

type KubernetesSuite struct{}

var _ = Suite(&KubernetesSuite{})

// TestMembers verfies members can be queried.
func (r *KubernetesSuite) TestMembers(c *C) {

	comment := Commentf("List all nodes")
	nodes := []v1.Node{
		r.newNode("satellite-1", "192.168.1.101", "master"),
		r.newNode("satellite-2", "192.168.1.102", "master"),
	}
	expected := []*pb.MemberStatus{
		pb.NewMemberStatus("satellite-1", "192.168.1.101",
			map[string]string{
				"role":     "master",
				"publicip": "192.168.1.101",
			}),
		pb.NewMemberStatus("satellite-2", "192.168.1.102",
			map[string]string{
				"role":     "master",
				"publicip": "192.168.1.102",
			}),
	}

	factory := informers.NewSharedInformerFactory(fake.NewSimpleClientset(&v1.NodeList{Items: nodes}), 0)
	informer := factory.Core().V1().Nodes().Informer()

	stop := make(chan struct{})
	defer close(stop)
	go informer.Run(stop)

	c.Assert(cache.WaitForCacheSync(stop, informer.HasSynced), Equals, true, comment)

	cluster, err := NewCluster(&Config{
		Informer: informer,
	})
	c.Assert(err, IsNil, comment)

	members, err := cluster.Members()
	c.Assert(err, IsNil, comment)

	sort.Sort(pb.ByName(members))
	c.Assert(members, test.DeepCompare, expected, comment)
}

// TestMember verifies single member can be queried.
func (r *KubernetesSuite) TestMember(c *C) {
	comment := Commentf("Query satellite-1")
	nodes := []v1.Node{
		r.newNode("satellite-1", "192.168.1.101", "master"),
	}
	expected := pb.NewMemberStatus("satellite-1", "192.168.1.101",
		map[string]string{
			"role":     "master",
			"publicip": "192.168.1.101",
		})

	factory := informers.NewSharedInformerFactory(fake.NewSimpleClientset(&v1.NodeList{Items: nodes}), 0)
	informer := factory.Core().V1().Nodes().Informer()

	stop := make(chan struct{})
	defer close(stop)
	go informer.Run(stop)

	c.Assert(cache.WaitForCacheSync(stop, informer.HasSynced), Equals, true, comment)

	cluster, err := NewCluster(&Config{
		Informer: informer,
	})
	c.Assert(err, IsNil, comment)

	member, err := cluster.Member("satellite-1")
	c.Assert(err, IsNil, comment)
	c.Assert(member, test.DeepCompare, expected, comment)
}

// newNode constructs a new pod.
func (r *KubernetesSuite) newNode(name, addr, role string) v1.Node {
	return v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				AdvertiseIPKey: addr,
				RoleKey:        role,
			},
		},
	}
}
