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
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/trace"
)

// NewBootConfigParamChecker returns a new checker that verifies
// kernel configuration options
func NewBootConfigParamChecker(params ...BootConfigParam) health.Checker {
	return &bootConfigParamChecker{
		Params:              params,
		kernelVersionReader: realKernelVersionReader,
		bootConfigReader:    realBootConfigReader,
	}
}

// bootConfigParamChecker checks whether parameters provided are specified in linux boot configuration file
type bootConfigParamChecker struct {
	// Params is array of parameters to check for
	Params []BootConfigParam
	kernelVersionReader
	bootConfigReader
}

// BootConfigParam defines parameter name (without CONFIG_ prefix) to check
type BootConfigParam struct {
	// Param is boot config parameter to check
	Name string
	// KernelConstraint specifies an optional kernel version constraint
	KernelConstraint KernelConstraintFunc
}

// Name returns name of the checker
func (c *bootConfigParamChecker) Name() string {
	return bootConfigParamID
}

// Check parses boot config files and validates whether parameters provided are set
func (c *bootConfigParamChecker) Check(ctx context.Context, reporter health.Reporter) {
	release, err := c.kernelVersionReader()
	if err != nil {
		reporter.Add(NewProbeFromErr(bootConfigParamID,
			"failed to determine kernel version",
			trace.Wrap(err)))
		return
	}

	kernelVersion, err := parseKernelVersion(release)
	if err != nil {
		reporter.Add(NewProbeFromErr(bootConfigParamID,
			"failed to determine kernel version",
			trace.Wrap(err)))
		return
	}

	r, err := c.bootConfigReader(release)
	if trace.IsNotFound(err) {
		reporter.Add(NewProbeFromErr(bootConfigParamID,
			"boot config unavailable, checks skipped", err))
		return
	}
	if err != nil {
		reporter.Add(NewProbeFromErr(bootConfigParamID,
			"failed to parse boot config file",
			trace.Wrap(err)))
		return
	}

	cfg, err := parseBootConfig(r)
	if err != nil {
		reporter.Add(NewProbeFromErr(bootConfigParamID,
			"failed to parse boot config file",
			trace.Wrap(err)))
		return
	}

	failed := false
	for _, param := range c.Params {
		if param.KernelConstraint != nil &&
			!param.KernelConstraint(*kernelVersion) {
			// Skip if the kernel condition is not satisfied
			continue
		}
		if _, ok := cfg[param.Name]; ok {
			continue
		}

		reporter.Add(&pb.Probe{
			Checker: bootConfigParamID,
			Detail: fmt.Sprintf("required kernel boot config parameter %s missing",
				param.Name),
			Status: pb.Probe_Failed,
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
func DefaultBootConfigParams() health.Checker {
	return NewBootConfigParamChecker(
		BootConfigParam{Name: "CONFIG_NET_NS"},
		BootConfigParam{Name: "CONFIG_PID_NS"},
		BootConfigParam{Name: "CONFIG_IPC_NS"},
		BootConfigParam{Name: "CONFIG_UTS_NS"},
		BootConfigParam{Name: "CONFIG_CGROUPS"},
		BootConfigParam{Name: "CONFIG_CGROUP_CPUACCT"},
		BootConfigParam{Name: "CONFIG_CGROUP_DEVICE"},
		BootConfigParam{Name: "CONFIG_CGROUP_FREEZER"},
		BootConfigParam{Name: "CONFIG_CGROUP_SCHED"},
		BootConfigParam{Name: "CONFIG_CPUSETS"},
		BootConfigParam{Name: "CONFIG_MEMCG"},
		BootConfigParam{Name: "CONFIG_KEYS"},
		BootConfigParam{Name: "CONFIG_VETH"},
		BootConfigParam{Name: "CONFIG_BRIDGE"},
		BootConfigParam{Name: "CONFIG_BRIDGE_NETFILTER"},
		BootConfigParam{Name: "CONFIG_NF_NAT_IPV4"},
		BootConfigParam{Name: "CONFIG_IP_NF_FILTER"},
		BootConfigParam{Name: "CONFIG_IP_NF_TARGET_MASQUERADE"},
		BootConfigParam{Name: "CONFIG_NETFILTER_XT_MATCH_ADDRTYPE"},
		BootConfigParam{Name: "CONFIG_NETFILTER_XT_MATCH_CONNTRACK"},
		BootConfigParam{Name: "CONFIG_NETFILTER_XT_MATCH_IPVS"},
		BootConfigParam{Name: "CONFIG_IP_NF_NAT"},
		BootConfigParam{Name: "CONFIG_NF_NAT"},
		BootConfigParam{Name: "CONFIG_NF_NAT_NEEDED"},
		BootConfigParam{Name: "CONFIG_POSIX_MQUEUE"},
		BootConfigParam{
			// See: https://lists.gt.net/linux/kernel/2465684#2465684
			//  and https://github.com/lxc/lxc/pull/1217
			// CONFIG_DEVPTS_MULTIPLE_INSTANCES has been removed as of kernel 4.7
			Name:             "CONFIG_DEVPTS_MULTIPLE_INSTANCES",
			KernelConstraint: KernelVersionLessThan(KernelVersion{Release: 4, Major: 7}),
		},
	)
}

// GetStorageDriverBootConfigParams returns config params required for a given filesystem
func GetStorageDriverBootConfigParams(drv string) health.Checker {
	var params []BootConfigParam

	switch drv {
	case "devicemapper":
		params = append(params,
			BootConfigParam{Name: "CONFIG_BLK_DEV_DM"},
			BootConfigParam{Name: "CONFIG_DM_THIN_PROVISIONING"},
		)
	case "overlay", "overlay2":
		params = append(params, BootConfigParam{Name: "CONFIG_OVERLAY_FS"})
	}

	return NewBootConfigParamChecker(params...)
}

// KernelConstraintFunc is a function to determine if the kernel version
// satisfies a particular condition
type KernelConstraintFunc func(KernelVersion) bool

// KernelVersionLessThan is a kernel constraint checker
// that determines if the specified testVersion is less than
// the actual version
func KernelVersionLessThan(version KernelVersion) KernelConstraintFunc {
	return func(testVersion KernelVersion) bool {
		return testVersion.Release < version.Release ||
			(testVersion.Release == version.Release &&
				testVersion.Major < version.Major) ||
			(testVersion.Major == version.Major &&
				testVersion.Minor < version.Minor)
	}
}

// KernelVersion describes an abbreviated version of a Linux kernel.
// It contains only the kernel version (including major/minor components)
// skips irrelevant details like patch or build number.
//
// Example:
//  $ uname -r
//  $ 4.4.9-112-generic
//
// The result will be:
//  KernelVersion{Release: 4, Major: 4, Minor: 9}
type KernelVersion struct {
	// Release specifies the release of the kernel
	Release int
	// Major specifies the major version component
	Major int
	// Minor specifies the minor version component
	Minor int
}

func realBootConfigReader(release string) (io.ReadCloser, error) {
	file, err := os.Open(fmt.Sprintf("/boot/config-%s", release))
	if err != nil {
		return nil, trace.ConvertSystemError(err)
	}

	return file, nil
}

func parseBootConfig(r io.ReadCloser) (config map[string]string, err error) {
	config = map[string]string{}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		if reBootConfigComment.Match(scanner.Bytes()) || scanner.Text() == "" {
			continue
		}

		parsed := reBootConfigParam.FindStringSubmatch(scanner.Text())
		if len(parsed) != 3 {
			continue
		}

		config[parsed[1]] = parsed[2]
	}

	err = scanner.Err()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return config, nil
}

func realKernelVersionReader() (version string, err error) {
	var uname syscall.Utsname
	err = syscall.Uname(&uname)
	if err != nil {
		return "", trace.Wrap(err)
	}

	return string(int8string(uname.Release[:])), nil
}

// kernelVersionReader returns the textual kernel version
type kernelVersionReader func() (version string, err error)

// bootConfigReader reads the kernel boot configuration file
// based on the specified kernel release version
type bootConfigReader func(release string) (io.ReadCloser, error)

func parseKernelVersion(input string) (*KernelVersion, error) {
	parts := strings.Split(input, "-")
	parts = strings.Split(parts[0], ".")
	if len(parts) != 3 {
		return nil, trace.BadParameter("invalid kernel version input: %q", input)
	}
	version, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, trace.BadParameter(
			"invalid kernel version: %v, expected a number",
			parts[0])
	}
	major, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, trace.BadParameter(
			"invalid kernel version major: %v, expected a number",
			parts[1])
	}
	minor, err := strconv.Atoi(parts[2])
	if err != nil {
		return nil, trace.BadParameter(
			"invalid kernel version minor: %v, expected a number",
			parts[2])
	}
	return &KernelVersion{
		Release: version,
		Major:   major,
		Minor:   minor,
	}, nil
}

func int8string(bytes []int8) (result []byte) {
	result = make([]byte, 0, len(bytes))
	for _, b := range bytes {
		if b == 0 {
			break
		}
		result = append(result, byte(b))
	}
	return result
}

const bootConfigParamID = "boot-config"

var reBootConfigParam = regexp.MustCompile(`(\S+)\=([ym])`)
var reBootConfigComment = regexp.MustCompile(`#.*`)
