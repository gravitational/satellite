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

package monitoring

import (
	"context"
	"fmt"

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/lib/kubernetes"
	"github.com/gravitational/satellite/utils"

	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
)

// SystemPodsConfig specifies configuration for a system pods checker.
type SystemPodsConfig struct {
	// NodeName specifies the kubernetes name of this node.
	NodeName string
	// KubeConfig specifies kubernetes access configuration.
	*KubeConfig
}

// checkAndSetDefaults validates that this configuration is correct and sets
// value defaults where necessary.
func (r *SystemPodsConfig) checkAndSetDefaults() error {
	var errors []error
	if r.NodeName == "" {
		errors = append(errors, trace.BadParameter("node name must be provided"))

	}
	if r.KubeConfig == nil {
		errors = append(errors, trace.BadParameter("kubernetes access config must be provided"))
	}
	return trace.NewAggregate(errors...)
}

// systemPodsChecker verifies system pods are operational.
type systemPodsChecker struct {
	// SystemPodsConfig specifies checker configuration values.
	SystemPodsConfig
}

// NewSystemPodsChecker returns a new system pods checker.
func NewSystemPodsChecker(config SystemPodsConfig) (*systemPodsChecker, error) {
	if err := config.checkAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}

	return &systemPodsChecker{
		SystemPodsConfig: config,
	}, nil
}

// Name returns this checker name
// Implements health.Checker
func (r *systemPodsChecker) Name() string {
	return systemPodsCheckerID
}

// Check verifies that all system pods are operational.
// Implements health.Checker
func (r *systemPodsChecker) Check(ctx context.Context, reporter health.Reporter) {
	err := r.check(ctx, reporter)
	if err != nil {
		log.WithError(err).Warn("Failed to verify critical system pods")
		// TODO: set probe to critical
		reporter.Add(&pb.Probe{
			Checker:  r.Name(),
			Detail:   "failed to verify critical system pods",
			Error:    trace.UserMessage(err),
			Status:   pb.Probe_Failed,
			Severity: pb.Probe_Warning,
		})
		return
	}
	if reporter.NumProbes() == 0 {
		reporter.Add(NewSuccessProbe(r.Name()))
	}
}

func (r *systemPodsChecker) check(ctx context.Context, reporter health.Reporter) error {
	pods, err := r.getPods()
	if trace.IsNotFound(err) {
		log.Debug("No critical system pods found.")
		return nil // system pods were not found, log and treat gracefully
	}
	if err != nil {
		return trace.Wrap(err)
	}

	r.verifyPods(pods, reporter)
	return nil
}

// getPods returns a list of the local pods that have the `critical` label.
func (r *systemPodsChecker) getPods() ([]corev1.Pod, error) {
	opts := metav1.ListOptions{
		LabelSelector: systemPodsSelector.String(),
		FieldSelector: fields.OneTermEqualSelector("spec.nodeName", r.NodeName).String(),
	}
	pods, err := r.Client.CoreV1().Pods(kubernetes.AllNamespaces).List(opts)
	if err != nil {
		return nil, utils.ConvertError(err)
	}

	return pods.Items, nil
}

// verifyPods verifies the pods are in a valid state. Reports a failed probe for
// each failed pod.
func (r *systemPodsChecker) verifyPods(pods []corev1.Pod, reporter health.Reporter) {
	for _, pod := range pods {
		switch pod.Status.Phase {
		case corev1.PodSucceeded:
			continue
		case corev1.PodRunning:
			if err := verifyContainers(pod.Status.ContainerStatuses); err != nil {
				reporter.Add(systemPodsFailureProbe(r.Name(), pod.Namespace, pod.Name, err))
			}
		case corev1.PodPending:
			var err error
			if isInitialized(pod.Status.Conditions) {
				err = verifyContainers(pod.Status.ContainerStatuses)
			} else {
				err = verifyContainers(pod.Status.InitContainerStatuses)
			}
			if err != nil {
				reporter.Add(systemPodsFailureProbe(r.Name(), pod.Namespace, pod.Name, err))
			}
		case corev1.PodFailed:
			err := trace.BadParameter("pod failed: %v", pod.Status.Reason)
			reporter.Add(systemPodsFailureProbe(r.Name(), pod.Namespace, pod.Name, err))
			continue
		default:
			log.WithField("phase", pod.Status.Phase).Warn("Pod is in an unknown phase.")
		}
	}
}

// isInitialized returns true if the pod's `Initialized` condition is `True`.
func isInitialized(conditions []corev1.PodCondition) bool {
	for _, condition := range conditions {
		if condition.Type == corev1.PodInitialized && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// verifyContainers verifies all containers are either Running or Terminated.
// Report error and reason if a container is Waiting.
func verifyContainers(containerStatuses []corev1.ContainerStatus) error {
	for _, status := range containerStatuses {
		if status.State.Waiting != nil {
			return trace.BadParameter("%v waiting: %v", status.Name, status.State.Waiting.Reason)
		}
	}
	return nil
}

// systemPodsFailureProbe constructs a probe that represents a failed system pods
// check for the pod specified by podName and namespace.
func systemPodsFailureProbe(checkerName, namespace, podName string, err error) *pb.Probe {
	return &pb.Probe{
		Checker:  checkerName,
		Detail:   fmt.Sprintf("pod %v/%v is not running", namespace, podName),
		Error:    trace.UserMessage(err),
		Status:   pb.Probe_Failed,
		Severity: pb.Probe_Warning, // TODO: set probe to critical
	}
}

const systemPodsCheckerID = "system-pods-checker"
const systemPodKey = "gravitational.io/critical-pod"

// systemPodsSelector defines a label selector used to query critical system pods.
var systemPodsSelector = utils.MustLabelSelector(
	metav1.LabelSelectorAsSelector(
		&metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{Key: systemPodKey, Operator: metav1.LabelSelectorOpExists},
			}}))
