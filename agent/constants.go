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

package agent

import "time"

// MemberStatus describes the state of a serf node.
type MemberStatus string

const (
	MemberAlive   MemberStatus = "alive"
	MemberLeaving              = "leaving"
	MemberLeft                 = "left"
	MemberFailed               = "failed"
)

// Role describes the agent's server role.
type Role string

const (
	RoleMaster Role = "master"
	RoleNode        = "node"
)

// Timeout values
const (
	// timelineInitTimeout specifies the amount of time to wait for the
	// timeline to initialize.
	timelineInitTimeout = 5 * time.Second

	// statusUpdateTimeout is the amount of time to wait between status update collections.
	statusUpdateTimeout = 30 * time.Second

	// recycleTimeout is the amount of time to wait between recycle attempts.
	// Recycle is a request to clean up / remove stale data that backends can choose to
	// implement.
	recycleTimeout = 10 * time.Minute

	// statusQueryReplyTimeout specifies the amount of time to wait for the cluster
	// status query reply.
	statusQueryReplyTimeout = 30 * time.Second

	// nodeTimeout specifies the amount of time to wait for a node status query reply.
	// The nodeTimeout is smaller than the statusQueryReplyTimeout so that node
	// status collection step can return results before the deadline.
	nodeTimeout = 25 * time.Second

	// checksTimeout specifies the amount of time to wait for a check to complete.
	// The checksTimeout is smaller than the nodeTimeout so that the checks
	// can return results before the deadline.
	checksTimeout = 20 * time.Second

	// probeTimeout specifies the amount of time to wait for a probe to complete.
	// The probeTimeout is smaller than the checksTimeout so that the probe
	// collection step can return results before the deadline.
	probeTimeout = 15 * time.Second
)

// maxConcurrentCheckers specifies the maximum number of checkers active at
// any given time.
const maxConcurrentCheckers = 10
