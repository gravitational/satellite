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
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"

	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

// NewModuleChecker creates a new kernel module checker
func NewModuleChecker(names ...string) ModuleChecker {
	return ModuleChecker{
		Modules:      names,
		moduleGetter: moduleGetterFunc(LoadModules),
	}
}

// Name returns name of the checker
func (r ModuleChecker) Name() string {
	return "module-check"
}

// Check determines if the modules specified with r.Modules have been loaded
func (r ModuleChecker) Check(ctx context.Context, reporter health.Reporter) {
	err := r.check(ctx, reporter)
	if err != nil {
		reporter.Add(NewProbeFromErr(r.Name(), "", trace.Wrap(err)))
		return
	}
	reporter.Add(&pb.Probe{Checker: r.Name(), Status: pb.Probe_Running})
}

func (r ModuleChecker) check(ctx context.Context, reporter health.Reporter) error {
	modules, err := r.moduleGetter.Modules()
	if err != nil {
		return trace.Wrap(err)
	}

	var errors []error
	for _, module := range r.Modules {
		if !modules.HasLoaded(module) {
			errors = append(errors, trace.NotFound("module %q not loaded", module))
		}
	}

	if len(errors) == 1 {
		return trace.Wrap(errors[0])
	}
	return trace.NewAggregate(errors...)
}

// ModuleChecker checks if the specified set of kernel modules ara loaded
type ModuleChecker struct {
	// Modules lists required kernel modules
	Modules []string
	moduleGetter
}

// LoadModules loads list of kernel modules from /proc/modules
func LoadModules() (Modules, error) {
	f, err := os.Open("/proc/modules")
	if err != nil {
		return nil, trace.ConvertSystemError(err)
	}
	defer f.Close()

	modules, err := LoadModulesFrom(f)
	if err != nil {
		return nil, trace.ConvertSystemError(err)
	}

	return modules, nil
}

// LoadModulesFrom loads list of kernel modules from the specified reader
func LoadModulesFrom(r io.Reader) (modules Modules, err error) {
	s := bufio.NewScanner(r)

	for s.Scan() {
		line := s.Text()
		module, err := parseModule(line)
		if err != nil {
			log.Warnf("failed to parse module, skip line %q", line)
			continue
		}
		modules = append(modules, *module)
	}

	if s.Err() != nil {
		return nil, trace.ConvertSystemError(err)
	}

	return modules, nil
}

// HasLoaded determines whether module names are loaded.
// names should not have duplicates
func (r Modules) HasLoaded(names ...string) bool {
L:
	for _, module := range r {
		for i, name := range names {
			if module.Name == name && module.IsLoaded() {
				names = append(names[:i], names[i+1:]...)
				continue L
			}
		}
	}
	return len(names) == 0
}

// Modules lists kernel modules
type Modules []Module

// IsLoaded determines if this module is loaded
func (r Module) IsLoaded() bool {
	return r.ModuleState == ModuleStateLive
}

// Module describes a kernel module
type Module struct {
	// ModuleState specifies the state of the module: live, loading/unloading
	ModuleState
	// Name identifies the module
	Name string
	// Instances specifies the number of instances this module has loaded
	Instances int
	// DependsOn lists module's dependencies on other modules
	DependsOn []string
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

	var dependencies []string
	if columns[3] != "-" {
		dependencies = strings.Split(columns[3], ",")
		// Last item is empty after the first
		if numDeps := len(dependencies); numDeps != 0 && dependencies[numDeps-1] == "" {
			dependencies = dependencies[:numDeps-1]
		}
	}

	return &Module{
		ModuleState: ModuleState(columns[4]),
		Name:        columns[0],
		Instances:   int(instances),
		DependsOn:   dependencies,
	}, nil
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
)

func (r moduleGetterFunc) Modules() (Modules, error) {
	return r()
}

type moduleGetterFunc func() (Modules, error)

type moduleGetter interface {
	Modules() (Modules, error)
}

var moduleColumns = []string{"name", "memory_size", "instances", "dependencies", "state", "memory_offset"}
