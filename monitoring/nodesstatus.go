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
	"fmt"

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/trace"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/fields"
	"k8s.io/kubernetes/pkg/labels"
)

// NewNodesStatusChecker returns a Checker that tests kubernetes nodes availability
func NewNodesStatusChecker(hostPort string, nodesReadyThreshold int) health.Checker {
	return &nodesStatusChecker{
		hostPort:            hostPort,
		nodesReadyThreshold: nodesReadyThreshold,
	}
}

// nodesStatusChecker tests and reports health failures in kubernetes
// nodes availability
type nodesStatusChecker struct {
	name                string
	hostPort            string
	nodesReadyThreshold int
}

// Name returns the name of this checker
func (r *nodesStatusChecker) Name() string { return "nodesstatuses" }

// Check validates the status of kubernetes components
func (r *nodesStatusChecker) Check(reporter health.Reporter) {
	client, err := ConnectToKube(r.hostPort)
	if err != nil {
		reporter.Add(NewProbeFromErr(r.Name(), trace.Errorf("failed to connect to kube: %v", err)))
		return
	}
	listOptions := api.ListOptions{
		LabelSelector: labels.Everything(),
		FieldSelector: fields.Everything(),
	}
	statuses, err := client.Nodes().List(listOptions)
	if err != nil {
		reporter.Add(NewProbeFromErr(r.Name(), trace.Errorf("failed to query nodes: %v", err)))
		return
	}
	var nodesCount int
	var nodesReady int
	for _, item := range statuses.Items {
		nodesCount++
		for _, condition := range item.Status.Conditions {
			if condition.Type != api.NodeReady {
				continue
			}
			if condition.Status == api.ConditionTrue {
				nodesReady++
			}
		}
	}

	percentNodesReady := 0
	if nodesCount > 0 {
		percentNodesReady = int(100. * float32(nodesReady) / float32(nodesCount))
	}

	if percentNodesReady < r.nodesReadyThreshold {
		reporter.Add(&pb.Probe{
			Checker: r.Name(),
			Status:  pb.Probe_Failed,
			Error: fmt.Sprintf("Not enough ready nodes: %v%% (threshold %v%%)",
				percentNodesReady, r.nodesReadyThreshold),
		})
	} else {
		reporter.Add(&pb.Probe{
			Checker: r.Name(),
			Status:  pb.Probe_Running,
		})
	}
}
