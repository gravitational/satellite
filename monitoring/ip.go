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

// NewIPForwardChecker returns new IP forward checker
func NewIPForwardChecker() *SysctlChecker {
	return &SysctlChecker{
		CheckerName:     "ip-forward",
		Param:           "net.ipv4.ip_forward",
		Expected:        "1",
		OnMissing:       "ipv4 forwarding status unknown",
		OnValueMismatch: "ipv4 forwarding off",
	}
}

// NewBrNetfilterChecker will check if br_netfilter module has established bridge networking
func NewBrNetfilterChecker() *SysctlChecker {
	return &SysctlChecker{
		CheckerName:     "br-netfilter",
		Param:           "net.bridge.bridge-nf-call-iptables",
		Expected:        "1",
		OnMissing:       "br_netfilter module is either not loaded, or sysctl net.bridge.bridge-nf-call-iptables is not set",
		OnValueMismatch: "kubernetes requires net.bridge.bridge-nf-call-iptables sysctl set to 1",
	}
}
