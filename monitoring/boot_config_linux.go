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

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"regexp"
	"syscall"

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/trace"
)

func int8string(bs []int8) string {
	b := make([]byte, 0, len(bs))
	for _, v := range bs {
		if v == 0 {
			break
		}
		b = append(b, byte(v))
	}
	return string(b)
}

var (
	reBootConfigParam   = regexp.MustCompile(`(\S+)\=([ym])`)
	reBootConfigComment = regexp.MustCompile(`#.*`)
)

func parseBootConfig() (map[string]string, error) {
	var uname syscall.Utsname
	err := syscall.Uname(&uname)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	file, err := os.Open(fmt.Sprintf("/boot/config-%s", int8string(uname.Release[:])))
	if err != nil {
		return nil, trace.ConvertSystemError(err)
	}
	defer file.Close()

	params := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if reBootConfigComment.Match(scanner.Bytes()) || scanner.Text() == "" {
			continue
		}

		parsed := reBootConfigParam.FindStringSubmatch(scanner.Text())
		if len(parsed) != 3 {
			continue
		}

		params[parsed[1]] = parsed[2]
	}

	err = scanner.Err()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return params, nil
}

// BootConfigParam defines parameter name (without CONFIG_ prefix) to check
type BootConfigParam struct {
	// Param is boot config parameter to check
	Name string
}

// BootConfigParamChecker checks whether parameters provided are specified in linux boot configuration file
type BootConfigParamChecker struct {
	// Params is array of parameters to check for
	Params []BootConfigParam
}

const (
	bootConfigParamID = "boot-config"
)

// Name returns name of the checker
func (c *BootConfigParamChecker) Name() string {
	return bootConfigParamID
}

// Check parses boot config files and validates whether parameters provided are set
func (c *BootConfigParamChecker) Check(ctx context.Context, reporter health.Reporter) {
	cfg, err := parseBootConfig()
	if trace.IsNotFound(err) {
		reporter.Add(NewProbeFromErr(bootConfigParamID,
			"boot config unavailable, checks skipped", err))
		return
	}
	if err != nil {
		reporter.Add(NewProbeFromErr(bootConfigParamID, "failed to parse boot config file", trace.Wrap(err)))
		return
	}

	failed := false
	for _, param := range c.Params {
		if _, ok := cfg[param.Name]; ok {
			continue
		}

		reporter.Add(&pb.Probe{
			Checker: bootConfigParamID,
			Detail:  fmt.Sprintf("required kernel boot config parameter %s missing", param.Name),
			Status:  pb.Probe_Failed,
		})
		failed = true
	}

	if failed {
		return
	}
	reporter.Add(&pb.Probe{
		Checker: bootConfigParamID,
		Status:  pb.Probe_Running,
	})
}

// DefaultBootConfigParams returns standard kernel configs required for running kubernetes
func DefaultBootConfigParams() *BootConfigParamChecker {
	return &BootConfigParamChecker{
		[]BootConfigParam{
			BootConfigParam{"CONFIG_NET_NS"},
			BootConfigParam{"CONFIG_PID_NS"},
			BootConfigParam{"CONFIG_IPC_NS"},
			BootConfigParam{"CONFIG_UTS_NS"},
			BootConfigParam{"CONFIG_CGROUPS"},
			BootConfigParam{"CONFIG_CGROUP_CPUACCT"},
			BootConfigParam{"CONFIG_CGROUP_DEVICE"},
			BootConfigParam{"CONFIG_CGROUP_FREEZER"},
			BootConfigParam{"CONFIG_CGROUP_SCHED"},
			BootConfigParam{"CONFIG_CPUSETS"},
			BootConfigParam{"CONFIG_MEMCG"},
			BootConfigParam{"CONFIG_KEYS"},
			BootConfigParam{"CONFIG_VETH"},
			BootConfigParam{"CONFIG_BRIDGE"},
			BootConfigParam{"CONFIG_BRIDGE_NETFILTER"},
			BootConfigParam{"CONFIG_NF_NAT_IPV4"},
			BootConfigParam{"CONFIG_IP_NF_FILTER"},
			BootConfigParam{"CONFIG_IP_NF_TARGET_MASQUERADE"},
			BootConfigParam{"CONFIG_NETFILTER_XT_MATCH_ADDRTYPE"},
			BootConfigParam{"CONFIG_NETFILTER_XT_MATCH_CONNTRACK"},
			BootConfigParam{"CONFIG_NETFILTER_XT_MATCH_IPVS"},
			BootConfigParam{"CONFIG_IP_NF_NAT"},
			BootConfigParam{"CONFIG_NF_NAT"},
			BootConfigParam{"CONFIG_NF_NAT_NEEDED"},
			BootConfigParam{"CONFIG_POSIX_MQUEUE"},
			BootConfigParam{"CONFIG_DEVPTS_MULTIPLE_INSTANCES"},
		}}
}

// GetStorageDriverBootConfigParams returns config params required for a given filesystem
func GetStorageDriverBootConfigParams(drv string) *BootConfigParamChecker {
	var param []BootConfigParam

	switch drv {
	case "devicemapper":
		param = append(param, BootConfigParam{"CONFIG_BLK_DEV_DM"}, BootConfigParam{"CONFIG_DM_THIN_PROVISIONING"})
	case "overlay", "overlay2":
		param = append(param, BootConfigParam{"CONFIG_OVERLAY_FS"})
	}

	return &BootConfigParamChecker{param}
}
