/*
Copyright 2019 Gravitational, Inc.

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
	"context"

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"

	"gopkg.in/check.v1"
)

type DNSSuite struct{}

var _ = check.Suite(&DNSSuite{})

func (s *DNSSuite) TestDnsChecker(c *check.C) {
	testCases := []struct {
		checker DNSChecker
		probes  health.Probes
		comment string
	}{
		{
			checker: DNSChecker{
				QuestionNS:  []string{"."},
				Nameservers: []string{"1.1.1.1"},
			},
			probes: health.Probes{
				{
					Checker: "dns",
					Status:  pb.Probe_Running,
				},
			},
			comment: "test root system query (requires internet access)",
		},
		{
			checker: DNSChecker{
				QuestionA:   []string{"google.com."},
				Nameservers: []string{"1.1.1.1"},
			},
			probes: health.Probes{
				{
					Checker: "dns",
					Status:  pb.Probe_Running,
				},
			},
			comment: "test known name (requires internet access)",
		},
		{
			checker: DNSChecker{
				QuestionA:   []string{"test.invalid."},
				Nameservers: []string{"1.1.1.1"},
			},
			probes: health.Probes{
				{
					Checker: "dns",
					Status:  pb.Probe_Failed,
					Error:   "NXDOMAIN",
					Detail:  "failed to resolve 'test.invalid.' (A) nameserver 1.1.1.1",
				},
			},
			comment: "test non existant name (requires internet access)",
		},
	}

	for _, testCase := range testCases {
		var reporter health.Probes
		testCase.checker.Check(context.TODO(), &reporter)
		c.Assert(reporter, check.DeepEquals, testCase.probes, check.Commentf(testCase.comment))
	}
}
