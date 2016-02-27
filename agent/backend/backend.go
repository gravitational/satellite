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
package backend

import pb "github.com/gravitational/satellite/agent/proto/agentpb"

// Backend is an interface that allows to persist health status results
// after a monitoring test run.
type Backend interface {
	// Update updates status for the cluster.
	UpdateStatus(status *pb.SystemStatus) error

	// Close resets the backend and releases any resources.
	Close() error
}
