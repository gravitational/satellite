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

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"

	"github.com/gravitational/trace"
)

// NewKernelModuleChecker creates a new kernel module checker
func NewKernelModuleChecker(modules ...ModuleRequest) health.Checker {
	return kernelModuleChecker{
		Modules:    modules,
		getModules: ReadModules,
	}
}

// Name returns name of the checker
func (r kernelModuleChecker) Name() string {
	return KernelModuleCheckerID
}

// Check determines if the modules specified with r.Modules have been loaded
func (r kernelModuleChecker) Check(ctx context.Context, reporter health.Reporter) {
	var probes health.Probes
	err := r.check(ctx, &probes)
	if err != nil && !trace.IsNotFound(err) {
		reporter.Add(NewProbeFromErr(r.Name(), "failed to validate kernel modules", trace.Wrap(err)))
		return
	}

	health.AddFrom(reporter, &probes)
	if probes.NumProbes() != 0 {
		return
	}

	reporter.Add(NewSuccessProbe(r.Name()))
}

func (r kernelModuleChecker) check(ctx context.Context, reporter health.Reporter) error {
	modules, err := r.getModules()
	if err != nil {
		return trace.Wrap(err)
	}

	for _, module := range r.Modules {
		if modules.IsLoaded(module) {
			continue
		}

		data, err := json.Marshal(KernelModuleCheckerData{Module: module})
		if err != nil {
			return trace.Wrap(err)
		}

		reporter.Add(&pb.Probe{
			Checker:     r.Name(),
			Detail:      fmt.Sprintf("%v not loaded", module),
			Status:      pb.Probe_Failed,
			CheckerData: data,
		})
	}

	return nil
}

// KernelModuleCheckerData gets attached to the kernel module check probes
type KernelModuleCheckerData struct {
	// Module is the probed kernel module
	Module ModuleRequest `json:"module"`
}

// kernelModuleChecker checks if the specified set of kernel modules are loaded
type kernelModuleChecker struct {
	// Modules lists required kernel modules
	Modules    []ModuleRequest
	getModules moduleGetterFunc
}

// ReadModules reads list of kernel modules from /proc/modules
func ReadModules() (modules Modules, err error) {
	dynamic, err := readProcModules()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	// Read the list of builtin modules
	release, err := realKernelVersionReader()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	builtin, err := readBuiltinModules(release)
	if err != nil && !trace.IsNotFound(err) {
		return nil, trace.Wrap(err)
	}

	return NewModules(append(dynamic, builtin...)...), nil
}

// IsLoaded determines whether module name is loaded.
func (r Modules) IsLoaded(module ModuleRequest) bool {
	_, loaded := r[module.Name]
	if loaded {
		return true
	}
	// Check alternative module names
	for _, name := range module.Names {
		if _, loaded = r[name]; loaded {
			return true
		}
	}
	return false
}

// String returns a text representation of this kernel module request
func (r ModuleRequest) String() string {
	if len(r.Names) == 0 {
		return fmt.Sprintf("kernel module %q", r.Name)
	}
	return fmt.Sprintf("kernel module %q (%q)", r.Name, r.Names)
}

// ModuleRequest describes a kernel module
type ModuleRequest struct {
	// Name names the kernel module
	Name string `json:"name"`
	// Names lists alternative names for the module if any.
	// For example, on CentOS 7.2 bridge netfilter module is called "bridge"
	// instead of "br_netfilter".
	Names []string `json:"names,omitempty"`
}

func NewModules(modules ...Module) (result Modules) {
	result = Modules{}
	for _, m := range modules {
		result[m.Name] = m
	}
	return result
}

// Modules lists kernel modules
type Modules map[string]Module

// IsLoaded determines if this module is loaded
func (r Module) IsLoaded() bool {
	return r.ModuleState == ModuleStateLive
}

// String returns a text representation of this kernel module
func (r Module) String() string {
	return fmt.Sprintf("kernel module %q", r.Name)
}

// Module describes a kernel module
type Module struct {
	// ModuleState specifies the state of the module: live, loading/unloading
	ModuleState
	// Name identifies the module
	Name string
	// Instances specifies the number of instances this module has loaded
	Instances int
}

func readBuiltinModules(kernelVersion string) ([]Module, error) {
	f, err := os.Open(fmt.Sprintf("/lib/modules/%v/modules.builtin", kernelVersion))
	if err != nil {
		return nil, trace.ConvertSystemError(err)
	}
	defer f.Close()

	modules, err := readModulesFrom(f, parseBuiltinModule)
	if err != nil {
		return nil, trace.ConvertSystemError(err)
	}
	return modules, nil
}

func readProcModules() ([]Module, error) {
	f, err := os.Open("/proc/modules")
	if err != nil {
		return nil, trace.ConvertSystemError(err)
	}
	defer f.Close()

	modules, err := readModulesFrom(f, parseModule)
	if err != nil {
		return nil, trace.ConvertSystemError(err)
	}
	return modules, nil
}

func readModulesFrom(r io.Reader, moduleParser moduleParseFunc) (modules []Module, err error) {
	s := bufio.NewScanner(r)

	for s.Scan() {
		line := s.Text()
		module, err := moduleParser(line)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		modules = append(modules, *module)
	}

	if s.Err() != nil {
		return nil, trace.ConvertSystemError(err)
	}

	return modules, nil
}

// parseModule parses module information from a single line of /proc/modules
// https://www.centos.org/docs/5/html/Deployment_Guide-en-US/s1-proc-topfiles.html#s2-proc-modules
func parseModule(moduleS string) (*Module, error) {
	columns := strings.SplitN(moduleS, " ", len(moduleColumns))
	if len(columns) != len(moduleColumns) {
		return nil, trace.BadParameter("invalid input: expected six whitespace-separated columns, but got %q",
			moduleS)
	}

	instanceS := columns[2]
	instances, err := strconv.ParseInt(instanceS, 10, 32)
	if err != nil {
		return nil, trace.BadParameter("invalid instances field: expected integer, but got %q", instanceS)
	}

	return &Module{
		ModuleState: ModuleState(columns[4]),
		Name:        columns[0],
		Instances:   int(instances),
	}, nil
}

// /lib/modules/$(uname -r)/modules.builtin lists modules as file paths
func parseBuiltinModule(path string) (*Module, error) {
	module := filepath.Base(path)
	i := strings.Index(module, ".")
	if i >= 0 {
		// Strip the extension
		module = module[:i]
	}
	return &Module{Name: module, ModuleState: ModuleBuiltin}, nil
}

// ModuleState describes the state of a kernel module
type ModuleState string

const (
	// ModuleStateLive defines a live (loaded) module
	ModuleStateLive ModuleState = "Live"
	// ModuleStateLoading defines a loading module
	ModuleStateLoading = "Loading"
	// ModuleStateUnloading defines an unloading module
	ModuleStateUnloading = "Unloading"
	// ModuleBuiltin defines a built-in module
	ModuleBuiltin = "Builtin"
)

type moduleGetterFunc func() (Modules, error)

type moduleParseFunc func(line string) (*Module, error)

// KernelModuleCheckerID is the ID of the checker of kernel modules
const KernelModuleCheckerID = "kernel-module"

var moduleColumns = []string{"name", "memory_size", "instances", "dependencies", "state", "memory_offset"}
