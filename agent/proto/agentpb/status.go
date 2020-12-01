/*
Copyright 2016-2020 Gravitational, Inc.

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

// This file implements JSON encoding/decoding for status types
package agentpb

import "github.com/gravitational/trace"

// EmptyStatus returns an empty system status
func EmptyStatus() *SystemStatus {
	return &SystemStatus{Status: SystemStatus_Unknown}
}

// NewMemberStatus constructs a new MemberStatus.
func NewMemberStatus(name, addr string, tags map[string]string) *MemberStatus {
	return &MemberStatus{
		Name:   name,
		Addr:   addr,
		Status: MemberStatus_Alive,
		Tags:   tags,
	}
}

// encoding.TextMarshaler
func (s SystemStatus_Type) MarshalText() (text []byte, err error) {
	switch s {
	case SystemStatus_Running:
		return []byte("running"), nil
	case SystemStatus_Degraded:
		return []byte("degraded"), nil
	default:
		return nil, nil
	}
}

// encoding.TextUnmarshaler
func (s *SystemStatus_Type) UnmarshalText(text []byte) error {
	switch string(text) {
	case "running":
		*s = SystemStatus_Running
	case "degraded":
		*s = SystemStatus_Degraded
	default:
		*s = SystemStatus_Unknown
	}
	return nil
}

// encoding.TextMarshaler
func (s NodeStatus_Type) MarshalText() (text []byte, err error) {
	switch s {
	case NodeStatus_Running:
		return []byte("running"), nil
	case NodeStatus_Degraded:
		return []byte("degraded"), nil
	default:
		return nil, nil
	}
}

// encoding.TextUnmarshaler
func (s *NodeStatus_Type) UnmarshalText(text []byte) error {
	switch string(text) {
	case "running":
		*s = NodeStatus_Running
	case "degraded":
		*s = NodeStatus_Degraded
	default:
		*s = NodeStatus_Unknown
	}
	return nil
}

// encoding.TextMarshaler
func (s Probe_Type) MarshalText() (text []byte, err error) {
	switch s {
	case Probe_Running:
		return []byte("running"), nil
	case Probe_Failed:
		return []byte("failed"), nil
	case Probe_Terminated:
		return []byte("terminated"), nil
	default:
		return nil, nil
	}
}

// encoding.TextUnmarshaler
func (s *Probe_Type) UnmarshalText(text []byte) error {
	switch string(text) {
	case "running":
		*s = Probe_Running
	case "failed":
		*s = Probe_Failed
	case "terminated":
		*s = Probe_Terminated
	default:
		*s = Probe_Unknown
	}
	return nil
}

// encoding.TextMarshaler
func (s Probe_Severity) MarshalText() (text []byte, err error) {
	switch s {
	case Probe_None:
		return []byte("none"), nil
	case Probe_Critical:
		return []byte("critical"), nil
	case Probe_Warning:
		return []byte("warning"), nil
	default:
		return nil, trace.BadParameter("unknown severity %s", s)
	}
}

// encoding.TextUnmarshaler
func (s *Probe_Severity) UnmarshalText(text []byte) error {
	switch string(text) {
	case "none":
		*s = Probe_None
	case "critical":
		*s = Probe_Critical
	case "warning":
		*s = Probe_Warning
	default:
		return trace.BadParameter("unknown severity %s", text)
	}
	return nil
}

// encoding.TextMarshaler
func (s MemberStatus_Type) MarshalText() (text []byte, err error) {
	switch s {
	case MemberStatus_Alive:
		return []byte("alive"), nil
	case MemberStatus_Leaving:
		return []byte("leaving"), nil
	case MemberStatus_Left:
		return []byte("left"), nil
	case MemberStatus_Failed:
		return []byte("failed"), nil
	case MemberStatus_None:
	default:
		return []byte("none"), nil
	}
	return nil, nil
}

// encoding.TextUnmarshaler
func (s *MemberStatus_Type) UnmarshalText(text []byte) error {
	switch string(text) {
	case "alive":
		*s = MemberStatus_Alive
	case "leaving":
		*s = MemberStatus_Leaving
	case "left":
		*s = MemberStatus_Left
	case "failed":
		*s = MemberStatus_Failed
	default:
		*s = MemberStatus_None
	}
	return nil
}

// ByName implements sort.Interface.
// Enables MemberStatus to be sorted by name.
type ByName []*MemberStatus

func (r ByName) Len() int           { return len(r) }
func (r ByName) Swap(i, j int)      { r[i], r[j] = r[j], r[i] }
func (r ByName) Less(i, j int) bool { return r[i].Name < r[j].Name }
