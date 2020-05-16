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
	"github.com/gravitational/satellite/agent/health"
	"github.com/gravitational/satellite/lib/test"

	"github.com/gravitational/trace"
	. "gopkg.in/check.v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type SystemPodsSuite struct {
	systemPodsChecker
}

var _ = Suite(&SystemPodsSuite{
	systemPodsChecker: systemPodsChecker{
		SystemPodsConfig: SystemPodsConfig{
			NodeName: "test-node",
		},
	},
})

// TestValidPodStatus verifies that the checker can identify healthy pods.
func (r *SystemPodsSuite) TestValidPodStatus(c *C) {
	var testCases = []struct {
		comment CommentInterface
		pod     corev1.Pod
	}{
		{
			comment: Commentf("Pod Succeeded."),
			pod: corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodSucceeded,
				},
			},
		},
		{
			comment: Commentf("Pod Unknown."),
			pod: corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodUnknown,
				},
			},
		},
		{
			comment: Commentf("Pod Running && Containers Completed/Running."),
			pod: corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{
							State: corev1.ContainerState{
								Terminated: &corev1.ContainerStateTerminated{
									Reason: containerCompleted,
								},
							},
						},
						{
							State: corev1.ContainerState{
								Running: &corev1.ContainerStateRunning{},
							},
						},
					},
				},
			},
		},
		{
			comment: Commentf("Pod Pending && Uninitialized && InitContainer Running/PodInitializing."),
			pod: corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodInitialized,
							Status: corev1.ConditionFalse,
						},
					},
					InitContainerStatuses: []corev1.ContainerStatus{
						{
							State: corev1.ContainerState{
								Running: &corev1.ContainerStateRunning{},
							},
						},
						{
							State: corev1.ContainerState{
								Waiting: &corev1.ContainerStateWaiting{
									Reason: podInitializing,
								},
							},
						},
					},
				},
			},
		},
		{
			comment: Commentf("Pod Pending && Uninitialized && InitContainer Running && Container PodInitializing."),
			pod: corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodInitialized,
							Status: corev1.ConditionFalse,
						},
					},
					InitContainerStatuses: []corev1.ContainerStatus{
						{
							State: corev1.ContainerState{
								Running: &corev1.ContainerStateRunning{},
							},
						},
					},
					ContainerStatuses: []corev1.ContainerStatus{
						{
							State: corev1.ContainerState{
								Waiting: &corev1.ContainerStateWaiting{
									Reason: podInitializing,
								},
							},
						},
					},
				},
			},
		},
		{
			comment: Commentf("Pod Pending && Initialized && Container ContainerCreating"),
			pod: corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodInitialized,
							Status: corev1.ConditionTrue,
						},
					},
					ContainerStatuses: []corev1.ContainerStatus{
						{
							State: corev1.ContainerState{
								Waiting: &corev1.ContainerStateWaiting{
									Reason: "ContainerCreating",
								},
							},
						},
					},
				},
			},
		},
		{
			comment: Commentf("Pod Pending && Initialized && Containers Running."),
			pod: corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodInitialized,
							Status: corev1.ConditionTrue,
						},
					},
					ContainerStatuses: []corev1.ContainerStatus{
						{
							State: corev1.ContainerState{
								Running: &corev1.ContainerStateRunning{},
							},
						},
					},
				},
			},
		},
	}

	for _, testCase := range testCases {
		reporter := &health.Probes{}
		r.verifyPods([]corev1.Pod{testCase.pod}, reporter)
		c.Assert(reporter, test.DeepCompare, &health.Probes{}, testCase.comment)
	}
}

// TestInvalidPodStatus verifies that the checker can identify unhealthy pods.
func (r *SystemPodsSuite) TestInvalidPodStatus(c *C) {
	var testCases = []struct {
		comment  CommentInterface
		pod      corev1.Pod
		expected health.Reporter
	}{
		{
			comment: Commentf("Pod Failed."),
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod-failed",
					Namespace: "test",
				},
				Status: corev1.PodStatus{
					Phase:  corev1.PodFailed,
					Reason: "TestReason",
				},
			},
			expected: &health.Probes{
				systemPodsFailureProbe(r.Name(), "test", "pod-failed", trace.BadParameter("pod failed: TestReason")),
			},
		},
		{
			comment: Commentf("Pod Pending && Uninitialized && InitContainers Waiting"),
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod-pending",
					Namespace: "test",
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodInitialized,
							Status: corev1.ConditionFalse,
						},
					},
					InitContainerStatuses: []corev1.ContainerStatus{
						{
							Name: "init-waiting",
							State: corev1.ContainerState{
								Waiting: &corev1.ContainerStateWaiting{
									Reason: imagePullBackOff,
								},
							},
						},
					},
				},
			},
			expected: &health.Probes{
				systemPodsFailureProbe(r.Name(), "test", "pod-pending",
					trace.BadParameter("init-waiting waiting: ImagePullBackOff")),
			},
		},
		{
			comment: Commentf("Pod Running && Container Copmleted/CrashLoopBackOff"),
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod-running",
					Namespace: "test",
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name: "cont-terminated",
							State: corev1.ContainerState{
								Terminated: &corev1.ContainerStateTerminated{
									Reason: containerCompleted,
								},
							},
						},
						{
							Name: "cont-waiting",
							State: corev1.ContainerState{
								Waiting: &corev1.ContainerStateWaiting{
									Reason: crashLoopBackOff,
								},
							},
						},
					},
				},
			},
			expected: &health.Probes{
				systemPodsFailureProbe(r.Name(), "test", "pod-running",
					trace.BadParameter("cont-waiting waiting: CrashLoopBackOff")),
			},
		},
	}

	for _, testCase := range testCases {
		reporter := &health.Probes{}
		r.verifyPods([]corev1.Pod{testCase.pod}, reporter)
		c.Assert(reporter, test.DeepCompare, testCase.expected, testCase.comment)
	}
}
