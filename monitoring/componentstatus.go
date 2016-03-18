/*
Copyright 2016 Gravitational, Inc.

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

// NewComponentStatusChecker returns a Checker that tests kubernetes component statuses
func NewComponentStatusChecker(hostPort string) health.Checker {
	return &componentStatusChecker{
		hostPort: hostPort,
	}
}

// componentStatusChecker tests and reports health failures in kubernetes
// components (controller-manager, scheduler, etc.)
type componentStatusChecker struct {
	name     string
	hostPort string
}

// Name returns the name of this checker
func (r *componentStatusChecker) Name() string { return "componentstatuses" }

// Check validates the status of kubernetes components
func (r *componentStatusChecker) Check(reporter health.Reporter) {
	client, err := ConnectToKube(r.hostPort)
	if err != nil {
		reporter.Add(NewProbeFromErr(r.Name(), trace.Errorf("failed to connect to kube: %v", err)))
		return
	}
	listOptions := api.ListOptions{
		LabelSelector: labels.Everything(),
		FieldSelector: fields.Everything(),
	}
	statuses, err := client.ComponentStatuses().List(listOptions)
	if err != nil {
		reporter.Add(NewProbeFromErr(r.Name(), trace.Errorf("failed to query component statuses: %v", err)))
		return
	}
	for _, item := range statuses.Items {
		for _, condition := range item.Conditions {
			if condition.Type != api.ComponentHealthy || condition.Status != api.ConditionTrue {
				reporter.Add(&pb.Probe{
					Checker: r.Name(),
					Detail:  item.Name,
					Status:  pb.Probe_Failed,
					Error:   fmt.Sprintf("%s (%s)", condition.Message, condition.Error),
				})
			} else {
				reporter.Add(&pb.Probe{
					Checker: r.Name(),
					Detail:  item.Name,
					Status:  pb.Probe_Running,
				})
			}
		}
	}
}
