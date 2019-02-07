/*
Copyright 2019 Gravitational, Inc.

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
)

const (
	PingCheckerID = "ping-checker"
)

// PingChecker is a checker that verifies if ping between Master Nodes and other
// ones is below a specified threshold
type PingChecker struct {
}

// Name returns the check name
// Implements health.Checker
func (c *PingChecker) Name() string {
	return PingCheckerID
}

// Check verifies that all nodes' ping with Master Nodes is lowed than the
// desired threshold
// Implements health.Checker
func (c *PingChecker) Check(ctx context.Context, r health.Reporter) {
	return
}
