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

package sqlite

import (
	"database/sql/driver"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"
)

// This file implements various mapping and conversion helpers to
// exchange values with the database backend.

// probeType represents value of type pb.Probe_Type in the backend
type probeType string

func protoToProbe(status pb.Probe_Type) probeType {
	switch status {
	case pb.Probe_Running:
		return probeType("H")
	case pb.Probe_Terminated:
		return probeType("T")
	default:
		return probeType("F")
	}
}

func (r probeType) toProto() pb.Probe_Type {
	switch r {
	case "H":
		return pb.Probe_Running
	case "T":
		return pb.Probe_Terminated
	default:
		return pb.Probe_Failed
	}
}

// Value implements driver.Valuer
func (r probeType) Value() (value driver.Value, err error) {
	return string(r), nil
}

// Scan implements sql.Scanner
func (r *probeType) Scan(src interface{}) error {
	if src != nil {
		*r = probeType(src.([]byte))
	}
	return nil
}

// timestamp represents value of type pb.Timestamp in the backend
type timestamp pb.Timestamp

// Scan implements sql.Scanner
func (ts *timestamp) Scan(src interface{}) error {
	return (*pb.Timestamp)(ts).UnmarshalText(src.([]byte))
}

// Value implements driver.Valuer
func (ts timestamp) Value() (value driver.Value, err error) {
	return pb.Timestamp(ts).MarshalText()
}

// memberStatusType represents value of type pb.MemberStatus_Type in the backend
type memberStatusType string

func protoToMemberStatus(status pb.MemberStatus_Type) memberStatusType {
	switch status {
	case pb.MemberStatus_Alive:
		return memberStatusType("A")
	case pb.MemberStatus_Leaving:
		return memberStatusType("G")
	case pb.MemberStatus_Left:
		return memberStatusType("L")
	default:
		return memberStatusType("F")
	}
}

func (r memberStatusType) toProto() pb.MemberStatus_Type {
	switch r {
	case "A":
		return pb.MemberStatus_Alive
	case "G":
		return pb.MemberStatus_Leaving
	case "L":
		return pb.MemberStatus_Left
	case "F":
		return pb.MemberStatus_Failed
	default:
		return pb.MemberStatus_None
	}
}

// Value implements driver.Valuer
func (r memberStatusType) Value() (value driver.Value, err error) {
	return string(r), nil
}

// Scan implements sql.Scanner
func (r *memberStatusType) Scan(src interface{}) error {
	*r = memberStatusType(src.([]byte))
	return nil
}

func (r memberStatusType) String() string { return string(r) }

// systemStatusType represents value of type pb.SystemStatus_Type in the backend
type systemStatusType string

func protoToSystemStatus(status pb.SystemStatus_Type) systemStatusType {
	switch status {
	case pb.SystemStatus_Running:
		return systemStatusType("H")
	default:
		return systemStatusType("F")
	}
}

func (r systemStatusType) toProto() pb.SystemStatus_Type {
	switch r {
	case "H":
		return pb.SystemStatus_Running
	case "F":
		return pb.SystemStatus_Degraded
	default:
		return pb.SystemStatus_Unknown
	}
}

// Value implements driver.Valuer
func (r systemStatusType) Value() (value driver.Value, err error) {
	return string(r), nil
}

// Scan implements sql.Scanner
func (r *systemStatusType) Scan(src interface{}) error {
	*r = systemStatusType(src.([]byte))
	return nil
}

func (r systemStatusType) String() string { return string(r) }

// nodeStatusType represents value of type pb.NodeStatus_Type in the backend
type nodeStatusType string

func protoToNodeStatus(status pb.NodeStatus_Type) nodeStatusType {
	switch status {
	case pb.NodeStatus_Running:
		return nodeStatusType("H")
	default:
		return nodeStatusType("F")
	}
}

func (r nodeStatusType) toProto() pb.NodeStatus_Type {
	switch r {
	case "H":
		return pb.NodeStatus_Running
	case "F":
		return pb.NodeStatus_Degraded
	default:
		return pb.NodeStatus_Unknown
	}
}

// Value implements driver.Valuer
func (r nodeStatusType) Value() (value driver.Value, err error) {
	return string(r), nil
}

// Scan implements sql.Scanner
func (r *nodeStatusType) Scan(src interface{}) error {
	*r = nodeStatusType(src.([]byte))
	return nil
}

func (r nodeStatusType) String() string { return string(r) }
