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
	"bytes"
	"context"

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/lib/test"

	"github.com/gravitational/trace"
	. "gopkg.in/check.v1"
)

type MonitoringSuite struct{}

var _ = Suite(&MonitoringSuite{})

func (_ *MonitoringSuite) TestLoadsModules(c *C) {
	// setup
	var testCases = []struct {
		err        error
		getModules moduleGetterFunc
		modules    Modules
		comment    string
	}{
		{
			modules: moduleMap(
				Module{Name: "br_netfilter", ModuleState: ModuleStateLive},
				Module{Name: "nf_conntrack_netlink", ModuleState: ModuleStateLive},
				Module{Name: "ebtable_filter", ModuleState: ModuleStateLive, Instances: 1},
				Module{Name: "ebtables", ModuleState: ModuleStateLive, Instances: 3},
				Module{Name: "nfsd", ModuleState: ModuleStateLive, Instances: 1},
				Module{Name: "ebtable_nat", ModuleState: ModuleStateLive, Instances: 1},
				Module{Name: "ebtable_broute", ModuleState: ModuleStateLive, Instances: 1},
			),
			getModules: moduleReader(modulesData),
			comment:    "loades modules",
		},
		{
			comment:    "broken input",
			getModules: moduleReader([]byte(`module foo bar`)),
			err:        trace.BadParameter(`invalid input: expected six whitespace-separated columns, but got "module foo bar"`),
		},
		{
			comment:    "broken input: invalid instance count",
			getModules: moduleReader([]byte(`module foo bar - Live qux`)),
			err:        trace.BadParameter(`invalid instances field: expected integer, but got "bar"`),
		},
		{
			comment:    "empty input",
			getModules: moduleReader(nil),
			modules:    Modules{},
		},
	}

	// exercise / verify
	for _, testCase := range testCases {
		modules, err := testCase.getModules()
		if testCase.err != nil {
			c.Assert(err, ErrorMatches, testCase.err.Error())
		} else {
			c.Assert(err, IsNil)
		}
		c.Assert(modules, test.DeepCompare, testCase.modules, Commentf(testCase.comment))
	}
}

func (_ *MonitoringSuite) TestHasModules(c *C) {
	// exercise
	modules, err := ReadModulesFrom(bytes.NewReader(modulesData))

	// verify
	c.Assert(err, IsNil)
	for _, module := range []string{"ebtables", "br_netfilter"} {
		c.Assert(modules.WasLoaded(module), Equals, true)
	}
}

func (_ *MonitoringSuite) TestValidatesModules(c *C) {
	var testCases = []struct {
		modules []string
		reader  moduleGetterFunc
		probes  health.Probes
		comment string
	}{
		{
			modules: []string{"ebtables", "br_netfilter"},
			reader:  moduleReader(modulesData),
			probes:  health.Probes{&pb.Probe{Checker: kernelModuleCheckerID, Status: pb.Probe_Running}},
			comment: "running",
		},
		{
			modules: []string{"required"},
			reader:  moduleReader(modulesData),
			probes: health.Probes{
				&pb.Probe{
					Checker: kernelModuleCheckerID,
					Detail:  `kernel module "required" not loaded`,
					Status:  pb.Probe_Failed,
				},
			},
			comment: "missing module",
		},
		{
			modules: nil,
			reader:  moduleReader(modulesData),
			probes: health.Probes{
				&pb.Probe{
					Checker: kernelModuleCheckerID,
					Status:  pb.Probe_Running,
				},
			},
			comment: "skip test for empty requirements",
		},
		{
			modules: []string{"required"},
			reader:  testFailingModuleReader(trace.NotFound("file or directory not found")),
			probes: health.Probes{
				&pb.Probe{
					Checker: kernelModuleCheckerID,
					Status:  pb.Probe_Running,
				},
			},
			comment: "skip test if no modules file available",
		},
		{
			modules: []string{"required"},
			reader:  testFailingModuleReader(trace.AccessDenied("permission denied")),
			probes: health.Probes{
				&pb.Probe{
					Checker: kernelModuleCheckerID,
					Detail:  "failed to validate kernel modules",
					Error:   "permission denied",
					Status:  pb.Probe_Failed,
				},
			},
			comment: "fail if error prevents from reading the file (other than not found)",
		},
	}

	// exercise / verify
	for _, testCase := range testCases {
		checker := kernelModuleChecker{
			Modules:    testCase.modules,
			getModules: testCase.reader,
		}
		var reporter health.Probes
		checker.Check(context.TODO(), &reporter)
		c.Assert(reporter, test.DeepCompare, testCase.probes, Commentf(testCase.comment))
	}
}

func moduleReader(data []byte) func() (Modules, error) {
	return func() (Modules, error) {
		return ReadModulesFrom(bytes.NewReader(data))
	}
}

func testFailingModuleReader(err error) func() (Modules, error) {
	return func() (Modules, error) {
		return nil, err
	}
}

func moduleMap(modules ...Module) Modules {
	result := make(map[string]Module)
	for _, module := range modules {
		result[module.Name] = module
	}
	return result
}

var modulesData = []byte(`br_netfilter 22209 0 - Live 0xffffffffc063f000
nf_conntrack_netlink 40449 0 - Live 0xffffffffc0659000
ebtable_filter 12827 1 - Live 0xffffffffc0415000
ebtables 35009 3 ebtable_nat,ebtable_broute,ebtable_filter, Live 0xffffffffc0407000
nfsd 342857 1 - Live 0xffffffffc033f000
ebtable_nat 12807 1 - Live 0xffffffffc058c000
ebtable_broute 12731 1 - Live 0xffffffffc0597000`)
