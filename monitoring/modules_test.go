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
	"bytes"
	"context"

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/lib/test"

	. "gopkg.in/check.v1"
)

type MonitoringSuite struct{}

var _ = Suite(&MonitoringSuite{})

func (_ *MonitoringSuite) TestLoadsModules(c *C) {
	// exercise
	modules, err := LoadModulesFrom(bytes.NewReader(modulesData))

	// verify
	c.Assert(err, IsNil)
	c.Assert(modules, test.DeepCompare, Modules{
		Module{Name: "br_netfilter", ModuleState: ModuleStateLive},
		Module{Name: "nf_conntrack_netlink", ModuleState: ModuleStateLive},
		Module{Name: "ebtable_filter", ModuleState: ModuleStateLive, Instances: 1},
		Module{Name: "ebtables", ModuleState: ModuleStateLive, Instances: 3,
			DependsOn: []string{"ebtable_nat", "ebtable_broute", "ebtable_filter"}},
		Module{Name: "nfsd", ModuleState: ModuleStateLive, Instances: 1},
		Module{Name: "ebtable_nat", ModuleState: ModuleStateLive, Instances: 1},
		Module{Name: "ebtable_broute", ModuleState: ModuleStateLive, Instances: 1},
	})
}

func (_ *MonitoringSuite) TestHasModules(c *C) {
	// exercise
	modules, err := LoadModulesFrom(bytes.NewReader(modulesData))

	// verify
	c.Assert(err, IsNil)
	c.Assert(modules.HasLoaded("ebtables", "br_netfilter"), Equals, true)
}

func (_ *MonitoringSuite) TestVerifiesModules(c *C) {
	var testCases = []struct {
		modules []string
		probes  health.Probes
		comment string
	}{
		{
			modules: []string{"ebtables", "br_netfilter"},
			probes:  health.Probes{&pb.Probe{Checker: "module-check", Status: pb.Probe_Running}},
			comment: "running",
		},
		{
			modules: []string{"required"},
			probes: health.Probes{
				&pb.Probe{
					Checker: "module-check",
					Error:   `module "required" not loaded`,
					Status:  pb.Probe_Failed,
				},
			},
			comment: "missing module",
		},
	}

	// exercise / verify
	for _, testCase := range testCases {
		checker := NewModuleChecker(testCase.modules...)
		checker.moduleGetter = moduleGetterFunc(moduleReader)
		var reporter health.Probes
		checker.Check(context.TODO(), &reporter)
		c.Assert(reporter, test.DeepCompare, testCase.probes, Commentf(testCase.comment))
	}
}

func moduleReader() (Modules, error) {
	return LoadModulesFrom(bytes.NewReader(modulesData))
}

var modulesData = []byte(`br_netfilter 22209 0 - Live 0xffffffffc063f000
nf_conntrack_netlink 40449 0 - Live 0xffffffffc0659000
ebtable_filter 12827 1 - Live 0xffffffffc0415000
ebtables 35009 3 ebtable_nat,ebtable_broute,ebtable_filter, Live 0xffffffffc0407000
nfsd 342857 1 - Live 0xffffffffc033f000
ebtable_nat 12807 1 - Live 0xffffffffc058c000
ebtable_broute 12731 1 - Live 0xffffffffc0597000`)
