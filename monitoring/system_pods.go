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
	"github.com/gravitational/satellite/utils"

	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SystemPodsConfig specifies configuration for a system pods checkers.
type SystemPodsConfig struct {
	// AdvertiseIP specifies the advertised ip address of the host running this checker.
	AdvertiseIP string
	// KubeConfig specifies kubernetes access information.
	*KubeConfig
}

// CheckAndSetDefaults validates that this configuration is correct and sets
// value defaults where necessary.
func (r *SystemPodsConfig) CheckAndSetDefaults() error {
	var errors []error
	if r.AdvertiseIP == "" {
		errors = append(errors, trace.BadParameter("host advertise ip must be provided"))
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
	if err := config.CheckAndSetDefaults(); err != nil {
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
		log.WithError(err).Warn("Failed to verify system pods")
		reporter.Add(NewProbeFromErr(r.Name(), "failed to verify system pods", err))
		return
	}
	if reporter.NumProbes() == 0 {
		reporter.Add(NewSuccessProbe(r.Name()))
	}
}

func (r *systemPodsChecker) check(ctx context.Context, reporter health.Reporter) error {
	pods, err := r.getPods()
	if trace.IsNotFound(err) {
		log.Debug("Failed to get system pods.")
		return nil // system pods were not found, log and treat gracefully
	}
	if err != nil {
		return trace.Wrap(err)
	}

	r.verifyPods(pods, reporter)
	return nil
}

// getPods returns a list of the local pods that exist in the
// systemPodsNamespace.
func (r *systemPodsChecker) getPods() ([]corev1.Pod, error) {
	opts := metav1.ListOptions{}
	pods, err := r.Client.CoreV1().Pods(systemPodsNamespace).List(opts)
	if err != nil {
		return nil, utils.ConvertError(err) // this will convert error to a proper trace error, e.g. trace.NotFound
	}

	var localPods []corev1.Pod
	for _, pod := range pods.Items {
		if pod.Status.HostIP == r.AdvertiseIP {
			localPods = append(localPods, pod)
		}
	}
	return localPods, nil
}

// verifyPods verifies the pods are in a valid state. Reports a failed probe for
// any pods that are in an invalid state.
func (r *systemPodsChecker) verifyPods(pods []corev1.Pod, reporter health.Reporter) {
	for _, pod := range pods {
		if err := verifyPodStatus(pod.Status); err != nil {
			reporter.Add(NewProbeFromErr(r.Name(), fmt.Sprintf("%s is in an invalid state", pod.Name), err))
		}
	}
}

// verifyPodStatus verifies the status phase and conditions.
func verifyPodStatus(status corev1.PodStatus) error {
	// https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#pod-phase
	switch status.Phase {
	case corev1.PodPending, corev1.PodSucceeded:
		return nil
	case corev1.PodFailed:
		return trace.BadParameter("pod has failed")
	case corev1.PodUnknown:
		log.Warn("Pod is in unknown state.")
		return nil
	}

	// https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#pod-conditions
	// If the pod phase is `Running`, all conditions are expected to be true?
	for _, condition := range status.Conditions {
		if condition.Status == corev1.ConditionFalse {
			return trace.BadParameter("%s condition is false; reason: %s", condition.Type, condition.Reason)
		}
		if condition.Status == corev1.ConditionUnknown {
			log.Warnf("%s condition is in an unknown state.", condition.Type)
		}
	}

	return nil
}

const systemPodsCheckerID = "system-pods-checker"
const systemPodsNamespace = "kube-system"
