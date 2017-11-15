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

import (
	"errors"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"
)

var errNoMaster = errors.New("master node unavailable")

// setSystemStatus combines the status of individual nodes into the status of the
// cluster as a whole.
// It additionally augments the cluster status with human-readable summary.
func setSystemStatus(status *pb.SystemStatus) {
	var foundMaster bool

	status.Status = pb.SystemStatus_Running
	for _, node := range status.Nodes {
		if !foundMaster && isMaster(node.MemberStatus) {
			foundMaster = true
		}
		if status.Status == pb.SystemStatus_Running {
			status.Status = nodeToSystemStatus(node.Status)
		}
		if node.MemberStatus.Status == pb.MemberStatus_Failed {
			status.Status = pb.SystemStatus_Degraded
		}
	}
	if !foundMaster {
		status.Status = pb.SystemStatus_Degraded
		status.Summary = errNoMaster.Error()
	}
}

func isMaster(member *pb.MemberStatus) bool {
	value, ok := member.Tags["role"]
	return ok && value == string(RoleMaster)
}

func nodeToSystemStatus(status pb.NodeStatus_Type) pb.SystemStatus_Type {
	switch status {
	case pb.NodeStatus_Running:
		return pb.SystemStatus_Running
	case pb.NodeStatus_Degraded:
		return pb.SystemStatus_Degraded
	default:
		return pb.SystemStatus_Unknown
	}
}

func isDegraded(status pb.SystemStatus) bool {
	switch status.Status {
	case pb.SystemStatus_Unknown, pb.SystemStatus_Degraded:
		return true
	}
	return false
}
