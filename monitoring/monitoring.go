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

import pb "github.com/gravitational/satellite/agent/proto/agentpb"

// NewProbeFromErr creates a new Probe given an error and a checker name
func NewProbeFromErr(name string, err error) *pb.Probe {
	return &pb.Probe{
		Checker: name,
		Error:   err.Error(),
		Status:  pb.Probe_Failed,
	}
}
