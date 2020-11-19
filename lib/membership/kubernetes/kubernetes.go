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

// Package kubernetes provides an implementation of membership.Cluster that
// relies on a Kubernetes cluster.
package kubernetes

import (
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/utils"

	"github.com/gravitational/trace"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	// AdvertiseIPKey defines the advertise IP key.
	AdvertiseIPKey = "gravitational.io/advertise-ip"
	// RoleKey defines the k8s role key.
	RoleKey = "gravitational.io/k8s-role"
)

// Config contains information needed to identify other members deployed in
// the Kubernetes cluster.
type Config struct {
	// Informer specifies a Node Infomer.
	Informer cache.SharedIndexInformer
	// Stop specifies a stop channel that indicates that the informer has been
	// stopped when it is closed.
	Stop <-chan struct{}
}

// checkAndSetDefaults validates configuration and sets default values.
func (r *Config) checkAndSetDefaults() error {
	var errs []error
	if r.Informer == nil {
		errs = append(errs, trace.BadParameter("Informer must be provided"))
	}
	if r.Stop == nil {
		errs = append(errs, trace.BadParameter("Stop channel must be provided"))
	}
	if len(errs) > 0 {
		return trace.NewAggregate(errs...)
	}
	return nil
}

// Cluster queries the members that are deployed in the Kubernetes cluster. All
// Nodes in the cluster are considered members of the Satellite cluster.
//
// Implements membership.Cluster.
type Cluster struct {
	// Config specifies information needed to query Kubernetes Nodes.
	*Config
}

// NewCluster constructs a new cluster.
func NewCluster(config *Config) (*Cluster, error) {
	if err := config.checkAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err, "invalid configuration")
	}

	return &Cluster{Config: config}, nil
}

// Members lists the members deployed in the Kubernetes cluster.
func (r *Cluster) Members() (members []*pb.MemberStatus, err error) {
	for _, obj := range r.Informer.GetStore().List() {
		node, ok := obj.(*v1.Node)
		if !ok {
			logrus.WithField("obj", obj).Warn("Infomer returned a non node object.")
			continue
		}

		advertiseIP, exists := node.Labels[AdvertiseIPKey]
		if !exists {
			logrus.WithField("node", node.Name).Warnf("Node does not have the %s key", AdvertiseIPKey)
			continue
		}

		members = append(members, pb.NewMemberStatus(
			node.Name,
			advertiseIP,
			map[string]string{
				"role":     node.Labels[RoleKey],
				"publicip": advertiseIP, // Read by Gravity
			},
		))
	}
	return members, nil
}

// Member returns the member with the specified name.
// Returns NotFound if the specified member is not deployed in the Kubernetes
// cluster.
func (r *Cluster) Member(name string) (member *pb.MemberStatus, err error) {
	obj, exists, err := r.Informer.GetStore().Get(&v1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}})
	if err != nil {
		return member, utils.ConvertError(err)
	}

	if !exists {
		return member, trace.NotFound("%s is not a member of the cluster", name)
	}

	node, ok := obj.(*v1.Node)
	if !ok {
		logrus.WithField("obj", obj).Warn("Informer returned a non node object.")
		return member, trace.BadParameter("informer returned a non node object")
	}

	advertiseIP, exists := node.Labels[AdvertiseIPKey]
	if !exists {
		logrus.WithField("node", node.Name).Warnf("Node does not have the %s key", AdvertiseIPKey)
		return member, trace.BadParameter("node %s does not have the %s key", node.Name, AdvertiseIPKey)
	}

	return pb.NewMemberStatus(node.Name,
		advertiseIP,
		map[string]string{
			"role":     node.Labels[RoleKey],
			"publicip": advertiseIP, // Read by Gravity
		},
	), nil
}
