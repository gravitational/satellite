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
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/utils"

	"github.com/gravitational/trace"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// DefaultAdvertiseIPKey defines the default gravitational advertise IP key.
const DefaultAdvertiseIPKey = "gravitational.io/advertise-ip"

// NodesConfig contains information needed to identify other members deployed in
// the kubernetes cluster.
type NodesConfig struct {
	// AdvertiseIPKey specifies the label key that holds the advertised IP address.
	AdvertiseIPKey string
	// KubeClient specifies the Kubernetes API interface.
	KubeClient kubernetes.Interface
}

// checkAndSetDefaults validates configuration and sets default values.
func (r *NodesConfig) checkAndSetDefaults() error {
	var errors []error
	if r.KubeClient == nil {
		errors = append(errors, trace.BadParameter("KubeClient must be provided"))
	}
	if len(errors) > 0 {
		return trace.NewAggregate(errors...)
	}
	if r.AdvertiseIPKey == "" {
		r.AdvertiseIPKey = DefaultAdvertiseIPKey
	}
	return trace.NewAggregate(errors...)
}

// ClusterNodes can poll the members that are deployed in the kubernetes cluster.
//
// Implements membership.Cluster
type ClusterNodes struct {
	*NodesConfig
}

// NewClusterNodes returns a new cluster. All nodes in the kubernetes cluster
// are members of the cluster.
func NewClusterNodes(config *NodesConfig) (*ClusterNodes, error) {
	if err := config.checkAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err, "invalid configuration")
	}

	return &ClusterNodes{NodesConfig: config}, nil
}

// Members lists the members deployed in the kubernetes cluster.
func (r *ClusterNodes) Members() (members []*pb.MemberStatus, err error) {
	list, err := r.KubeClient.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return members, utils.ConvertError(err)
	}

	members = make([]*pb.MemberStatus, len(list.Items))
	for i, node := range list.Items {
		members[i] = pb.NewMemberStatus(node.Name, node.Labels[r.AdvertiseIPKey], node.Annotations)
	}
	return members, nil
}

// Member returns the member with the specified name.
// Returns NotFound if the specified member is not deployed in the kubernetes
// cluster.
func (r *ClusterNodes) Member(name string) (member *pb.MemberStatus, err error) {
	node, err := r.KubeClient.CoreV1().Nodes().Get(name, metav1.GetOptions{})
	if err != nil {
		return member, utils.ConvertError(err)
	}
	return pb.NewMemberStatus(node.Name, node.Labels[r.AdvertiseIPKey], node.Annotations), nil
}
