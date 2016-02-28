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
	statuses, err := client.ComponentStatuses().List(labels.Everything(), fields.Everything())
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
