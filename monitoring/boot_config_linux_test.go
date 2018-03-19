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
	"fmt"
	"io"
	"io/ioutil"

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/lib/test"
	"github.com/gravitational/trace"

	. "gopkg.in/check.v1"
)

func (*MonitoringSuite) TestReadsBootConfig(c *C) {
	// exercise
	config, err := parseBootConfig(ioutil.NopCloser(bytes.NewReader(testBootConfig)))

	// verify
	c.Assert(err, IsNil)
	c.Assert(config, test.DeepCompare, map[string]string{
		"CONFIG_PARAM2": "m",
		"CONFIG_PARAM3": "y",
	})
}

func (*MonitoringSuite) TestParsesKernelVersion(c *C) {
	// setup
	var testCases = []struct {
		release  string
		expected KernelVersion
		comment  string
	}{
		{
			release:  "4.4.0-112-generic",
			expected: KernelVersion{4, 4, 0},
			comment:  "ubuntu 16.04",
		},
		{
			release:  "3.10.0-514.16.1.el7.x86_64",
			expected: KernelVersion{3, 10, 0},
			comment:  "centos 7.4",
		},
		{
			release:  "4.9.0-4-amd64",
			expected: KernelVersion{4, 9, 0},
			comment:  "debian 9.3",
		},
	}

	// exercise / verify
	for _, testCase := range testCases {
		version, err := parseKernelVersion(testCase.release)
		c.Assert(err, IsNil)
		c.Assert(version, test.DeepCompare, &testCase.expected, Commentf(testCase.comment))
	}
}

func (*MonitoringSuite) TestValidatesBootConfig(c *C) {
	// setup
	var testCases = []struct {
		params []BootConfigParam
		kernelVersionReader
		bootConfigReader
		probes  health.Probes
		comment string
	}{
		{
			params:              staticParams("CONFIG_PARAM2", "CONFIG_PARAM3"),
			kernelVersionReader: staticKernelVersion("4.4.0"),
			bootConfigReader:    testBootConfigReader(testBootConfig),
			probes:              health.Probes{&pb.Probe{Checker: bootConfigParamID, Status: pb.Probe_Running}},
			comment:             "all parameters available",
		},
		{
			params:              staticParams("CONFIG_PARAM1", "CONFIG_PARAM2"),
			kernelVersionReader: staticKernelVersion("4.4.0"),
			bootConfigReader:    testBootConfigReader(testBootConfig),
			probes: health.Probes{&pb.Probe{
				Checker: bootConfigParamID,
				Status:  pb.Probe_Failed,
				Detail:  "required kernel boot config parameter CONFIG_PARAM1 missing",
			}},
			comment: "parameter missing",
		},
		{
			params: []BootConfigParam{
				BootConfigParam{
					Name:             "CONFIG_PARAM4",
					KernelConstraint: KernelVersionLessThan(KernelVersion{Release: 4, Major: 4}),
				}},
			kernelVersionReader: staticKernelVersion("4.5.0"),
			bootConfigReader:    testBootConfigReader(testBootConfig),
			probes:              health.Probes{&pb.Probe{Checker: bootConfigParamID, Status: pb.Probe_Running}},
			comment:             "parameter should be skipped due to kernel version",
		},
		{
			params: []BootConfigParam{
				BootConfigParam{
					Name:             "CONFIG_PARAM4",
					KernelConstraint: KernelVersionLessThan(KernelVersion{Release: 4, Major: 4}),
				}},
			kernelVersionReader: staticKernelVersion("4.5.0"),
			bootConfigReader:    testBootConfigReader(testBootConfig),
			probes:              health.Probes{&pb.Probe{Checker: bootConfigParamID, Status: pb.Probe_Running}},
			comment:             "parameter should be skipped due to kernel version",
		},
		{
			params:              staticParams("CONFIG_PARAM"),
			kernelVersionReader: staticKernelVersion("4.5.0"),
			bootConfigReader:    testBootConfigFailingReader(trace.NotFound("file or directory not found")),
			probes:              health.Probes{&pb.Probe{Checker: bootConfigParamID, Status: pb.Probe_Running}},
			comment:             "skip test if boot configuration is unavailable",
		},
		{
			params:              staticParams("CONFIG_PARAM"),
			kernelVersionReader: testFailingKernelVersion(fmt.Errorf("unknown error")),
			probes: health.Probes{
				&pb.Probe{
					Checker: bootConfigParamID,
					Detail:  "failed to validate boot configuration",
					Error:   "failed to read kernel version",
					Status:  pb.Probe_Failed,
				}},
			comment: "fails if kernel version cannot be determined",
		},
		{
			params:              staticParams("CONFIG_PARAM4", "CONFIG_PARAM5"),
			kernelVersionReader: staticKernelVersion("4.5.0"),
			bootConfigReader:    testBootConfigReader(testBootConfig),
			probes: health.Probes{
				&pb.Probe{
					Checker: bootConfigParamID,
					Status:  pb.Probe_Failed,
					Detail:  "required kernel boot config parameter CONFIG_PARAM4 missing",
				},
				&pb.Probe{
					Checker: bootConfigParamID,
					Status:  pb.Probe_Failed,
					Detail:  "required kernel boot config parameter CONFIG_PARAM5 missing",
				},
			},
			comment: "collect all failed probes",
		},
	}

	// exercise / verify
	for _, testCase := range testCases {
		checker := &bootConfigParamChecker{
			Params:              testCase.params,
			kernelVersionReader: testCase.kernelVersionReader,
			bootConfigReader:    testCase.bootConfigReader,
		}
		var reporter health.Probes
		checker.Check(context.TODO(), &reporter)
		c.Assert(reporter, test.DeepCompare, testCase.probes, Commentf(testCase.comment))
	}
}

func testBootConfigReader(config []byte) bootConfigReader {
	return func(string) (io.ReadCloser, error) {
		return ioutil.NopCloser(bytes.NewReader(config)), nil
	}
}

func testBootConfigFailingReader(err error) bootConfigReader {
	return func(string) (io.ReadCloser, error) {
		return nil, err
	}
}

func staticKernelVersion(version string) kernelVersionReader {
	return func() (string, error) {
		return version, nil
	}
}

func testFailingKernelVersion(err error) kernelVersionReader {
	return func() (string, error) {
		return "", err
	}
}

func staticParams(params ...string) []BootConfigParam {
	result := make([]BootConfigParam, 0, len(params))
	for _, param := range params {
		result = append(result, BootConfigParam{Name: param})
	}
	return result
}

var testBootConfig = []byte(`
# this configuration parameter is disabled
CONFIG_PARAM1=n
CONFIG_PARAM2=m
CONFIG_PARAM3=y
`)
