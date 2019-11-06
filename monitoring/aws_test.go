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

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/lib/test"

	. "gopkg.in/check.v1"
)

// TestComplyWithContext verifies that the checker complies with the provided
// context.
func (*MonitoringSuite) TestComplyWithContext(c *C) {
	// setup
	var testCase = struct {
		probes  health.Probes
		comment string
	}{
		probes: health.Probes{
			&pb.Probe{
				Checker: awsHasProfileCheckerID,
				Detail:  "failed to validate IAM profile",
				Error:   "context canceled",
				Status:  pb.Probe_Failed,
			},
		},
		comment: "contexted canceled before all checks completed",
	}

	// exercise / verify
	ctx, cancel := context.WithCancel(context.TODO())
	cancel()
	checker := awsHasProfileChecker{}
	var reporter health.Probes
	checker.Check(ctx, &reporter)
	c.Assert(reporter, test.DeepCompare, testCase.probes, Commentf(testCase.comment))
}
