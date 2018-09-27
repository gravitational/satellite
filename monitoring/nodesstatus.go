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
	"fmt"

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/trace"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
)

// NewNodesStatusChecker returns a Checker that tests kubernetes nodes availability
func NewNodesStatusChecker(kubeAddr string, nodesReadyThreshold int) health.Checker {
	return &nodesStatusChecker{
		kubeAddr:            kubeAddr,
		nodesReadyThreshold: nodesReadyThreshold,
	}
}

// nodesStatusChecker tests and reports health failures in kubernetes
// nodes availability
type nodesStatusChecker struct {
	kubeAddr            string
	nodesReadyThreshold int
}

// Name returns the name of this checker
func (r *nodesStatusChecker) Name() string { return NodesStatusCheckerID }

// Check validates the status of kubernetes components
func (r *nodesStatusChecker) Check(ctx context.Context, reporter health.Reporter) {
	client, err := ConnectToKube(r.kubeAddr, "")
	if err != nil {
		reason := "failed to connect to kubernetes apiserver"
		reporter.Add(NewProbeFromErr(r.Name(), reason, trace.Wrap(err)))
		return
	}
	listOptions := metav1.ListOptions{
		LabelSelector: labels.Everything().String(),
		FieldSelector: fields.Everything().String(),
	}
	statuses, err := client.Core().Nodes().List(listOptions)
	if err != nil {
		reason := "failed to query nodes"
		reporter.Add(NewProbeFromErr(r.Name(), reason, trace.Wrap(err)))
		return
	}
	var nodesReady int
	for _, item := range statuses.Items {
		for _, condition := range item.Status.Conditions {
			if condition.Type != v1.NodeReady {
				continue
			}
			if condition.Status == v1.ConditionTrue {
				nodesReady++
				continue
			}
		}
	}

	if nodesReady < r.nodesReadyThreshold {
		reporter.Add(&pb.Probe{
			Checker: r.Name(),
			Status:  pb.Probe_Failed,
			Error: fmt.Sprintf("Not enough ready nodes: %v (threshold %v)",
				nodesReady, r.nodesReadyThreshold),
		})
	} else {
		reporter.Add(&pb.Probe{
			Checker: r.Name(),
			Status:  pb.Probe_Running,
		})
	}
}

// NewNodeStatusChecker returns a Checker that validates availability
// of a single kubernetes node
func NewNodeStatusChecker(kubeAddr, nodeName string) *nodeStatusChecker {
	nodeLister := kubeNodeLister{kubeAddr: kubeAddr}
	return &nodeStatusChecker{
		nodeLister: nodeLister,
		nodeName:   nodeName,
	}
}

// NewNodeStatusChecker returns a Checker that validates availability
// of a single kubernetes node
type nodeStatusChecker struct {
	nodeLister
	nodeName string
}

// Name returns the name of this checker
func (r *nodeStatusChecker) Name() string { return NodeStatusCheckerID }

// Check validates the status of kubernetes components
func (r *nodeStatusChecker) Check(ctx context.Context, reporter health.Reporter) {
	nodes, err := r.nodeLister.Nodes()
	if err != nil {
		reporter.Add(NewProbeFromErr(r.Name(), trace.UserMessage(err), trace.Wrap(err)))
		return
	}

	var unavailableNode *v1.Node
L:
	for _, node := range nodes.Items {
		for _, condition := range node.Status.Conditions {
			if condition.Type != v1.NodeReady {
				continue
			}
			if condition.Status != v1.ConditionTrue && node.Name == r.nodeName {
				unavailableNode = &node
				break L
			}
		}
	}

	if unavailableNode == nil {
		reporter.Add(&pb.Probe{
			Checker: r.Name(),
			Status:  pb.Probe_Running,
			Detail:  r.nodeName,
		})
		return
	}

	reporter.Add(&pb.Probe{
		Checker: r.Name(),
		Status:  pb.Probe_Temporary,
		Detail:  unavailableNode.Name,
		Error:   "Node is not ready",
	})
}

type nodeLister interface {
	Nodes() (*v1.NodeList, error)
}

func (r kubeNodeLister) Nodes() (*v1.NodeList, error) {
	client, err := ConnectToKube(r.kubeAddr, "")
	if err != nil {
		return nil, trace.Wrap(err, "failed to connect to kubernetes apiserver")
	}

	options := metav1.ListOptions{
		LabelSelector: labels.Everything().String(),
		FieldSelector: fields.Everything().String(),
	}
	nodes, err := client.CoreV1().Nodes().List(options)
	if err != nil {
		return nil, trace.Wrap(err, "failed to query nodes")
	}
	return nodes, nil
}

type kubeNodeLister struct {
	kubeAddr string
}

const (
	// NodeStatusCheckerID identifies the checker that detects whether a node is not ready
	NodeStatusCheckerID = "nodestatus"
	// NodesStatusCheckerID identifies the checker that validates node availability in a cluster
	NodesStatusCheckerID = "nodesstatus"
)
