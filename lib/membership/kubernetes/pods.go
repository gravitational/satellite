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

// PodsConfig contains information needed to identify other members deployed in
// the kubernetes cluster.
type PodsConfig struct {
	// Namespace specifies the kubernetes namespace.
	Namespace string
	// LabelSelector specifies the kubernetes label that identifies cluster
	// members.
	LabelSelector string
	// KubeClient specifies the Kubernetes API interface.
	KubeClient kubernetes.Interface
}

// checkAndSetDefaults validates configuration and sets default values.
func (r *PodsConfig) checkAndSetDefaults() error {
	var errors []error
	if r.Namespace == "" {
		errors = append(errors, trace.BadParameter("Namespace must be provided"))
	}
	if r.LabelSelector == "" {
		errors = append(errors, trace.BadParameter("LabelSelector must be provided"))
	}
	if r.KubeClient == nil {
		errors = append(errors, trace.BadParameter("KubeClient must be provided"))
	}
	return trace.NewAggregate(errors...)
}

// ClusterPods can poll the members that are deployed in the kubernetes cluster.
//
// Implements membership.Cluster
type ClusterPods struct {
	*PodsConfig
}

// NewClusterPods returns a new cluster. Configured kubernetes namespace and
//label selector are used to determine cluster membership.
func NewClusterPods(config *PodsConfig) (*ClusterPods, error) {
	if err := config.checkAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err, "invalid configuration")
	}

	return &ClusterPods{PodsConfig: config}, nil
}

// Members lists the members deployed in the kubernetes cluster.
func (r *ClusterPods) Members() (members []*pb.MemberStatus, err error) {
	list, err := r.KubeClient.CoreV1().Pods(r.Namespace).List(metav1.ListOptions{
		LabelSelector: r.LabelSelector,
	})
	if err != nil {
		return members, utils.ConvertError(err)
	}

	members = make([]*pb.MemberStatus, len(list.Items))
	for i, pod := range list.Items {
		members[i] = pb.NewMemberStatus(pod.Spec.NodeName, pod.Status.PodIP, pod.Annotations)
	}

	return members, nil
}

// Member returns the member with the specified name.
// Returns NotFound if the specified member is not deployed in the kubernetes
// cluster.
func (r *ClusterPods) Member(name string) (member *pb.MemberStatus, err error) {
	pod, err := r.KubeClient.CoreV1().Pods(r.Namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return member, utils.ConvertError(err)
	}
	return pb.NewMemberStatus(pod.Spec.NodeName, pod.Status.PodIP, pod.Annotations), nil
}
