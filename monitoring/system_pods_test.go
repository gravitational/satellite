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
	. "gopkg.in/check.v1"
	corev1 "k8s.io/api/core/v1"
)

type SystemPodsSuite struct{}

var _ = Suite(&SystemPodsSuite{})

// TestVerifyPodStatus verifies that the checker can correctly identify valid
// pod status.
func (r *SystemPodsSuite) TestVerifyPodStatus(c *C) {
	var testCases = []struct {
		comment CommentInterface
		status  corev1.PodStatus
	}{
		{
			comment: Commentf("Pod is pending. Expected no errors."),
			status:  corev1.PodStatus{Phase: corev1.PodPending},
		},
		{
			comment: Commentf("Pod has succeeded. Expected no errors."),
			status:  corev1.PodStatus{Phase: corev1.PodSucceeded},
		},
		{
			comment: Commentf("Pod is running. Expected no errors."),
			status:  corev1.PodStatus{Phase: corev1.PodRunning},
		},
		{
			comment: Commentf("Pod is in unknown state. Expected no errors."),
			status:  corev1.PodStatus{Phase: corev1.PodUnknown},
		},
	}

	for _, testCase := range testCases {
		err := verifyPodStatus("", testCase.status)
		c.Assert(err, IsNil, testCase.comment)
	}
}
