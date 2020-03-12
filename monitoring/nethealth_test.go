/*
Copyright 2020 Gravitational, Inc.

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
	"github.com/gravitational/satellite/agent/health"
	"github.com/gravitational/satellite/lib/test"

	"github.com/mailgun/holster"
	. "gopkg.in/check.v1"
)

type NethealthSuite struct{}

var _ = Suite(&NethealthSuite{})

// TestUpdateTimeoutStats verifies the checker can properly record timeout
// data points from available metrics.
func (s *NethealthSuite) TestUpdateTimeoutStats(c *C) {
	var testCases = []struct {
		comment  CommentInterface
		expected []float64
		data     []float64
	}{
		{
			comment:  Commentf("Expected all data points to be recorded."),
			expected: []float64{0, 0, 0, 0, 0},
			data:     []float64{0, 0, 0, 0, 0},
		},
		{
			comment:  Commentf("Expected oldest data point to be removed"),
			expected: []float64{0, 0, 0, 0, 0},
			data:     []float64{1, 0, 0, 0, 0, 0},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		checker, err := s.newNethealthChecker()
		c.Assert(err, IsNil)

		for _, count := range testCase.data {
			c.Assert(checker.updateStats(s.newMetricsWithCount(count)), IsNil, testCase.comment)
		}

		series, err := checker.getNetStats(testNode)
		c.Assert(err, IsNil, testCase.comment)
		c.Assert(series, test.DeepCompare, testCase.expected, testCase.comment)
	}
}

// TestNethealthChecker verifies nethealth checker can properly detect
// healthy/unhealthy network.
func (s *NethealthSuite) TestNethealthVerification(c *C) {

	var testCases = []struct {
		comment  CommentInterface
		expected health.Reporter
		data     []float64
	}{
		{
			comment:  Commentf("Expected no failed probes. Not enough data points."),
			expected: new(health.Probes),
			data:     []float64{abovePLT, abovePLT, abovePLT},
		},
		{
			comment:  Commentf("Expected no failed probes. No timeouts."),
			expected: new(health.Probes),
			data:     []float64{0, 0, 0, 0, 0},
		},
		{
			comment:  Commentf("Expected no failed probes. Data points do not exceed threshold."),
			expected: new(health.Probes),
			data:     []float64{plt, plt, plt, plt, plt},
		},
		{
			comment:  Commentf("Expected no failed probes. Does not exceed threshold throughout entire interval."),
			expected: new(health.Probes),
			data:     []float64{abovePLT, abovePLT, abovePLT, abovePLT, plt},
		},
		{
			comment:  Commentf("Expected failed probe. Timeouts increase at each interval."),
			expected: &health.Probes{NewProbeFromErr(nethealthCheckerID, nethealthDetail(testNode), nil)},
			data:     []float64{abovePLT, abovePLT, abovePLT, abovePLT, abovePLT},
		},
	}

	checker, err := s.newNethealthChecker()
	c.Assert(err, IsNil)

	for _, testCase := range testCases {
		testCase := testCase

		c.Assert(checker.setNetStats(testNode, testCase.data), IsNil, testCase.comment)

		reporter, err := checker.verifyNethealth([]string{testNode})
		c.Assert(err, IsNil, testCase.comment)
		c.Assert(reporter, test.DeepCompare, testCase.expected, testCase.comment)
	}
}

// newNethealthChecker returns a new nethealth checker to be used for testing.
func (s *NethealthSuite) newNethealthChecker() (*nethealthChecker, error) {
	return &nethealthChecker{
		netStats:       holster.NewTTLMap(netStatsCapacity),
		seriesCapacity: testCapacity,
	}, nil
}

// newMetricsWithCount creates new metrics with the provided val.
func (s *NethealthSuite) newMetricsWithCount(val float64) (packetLossPercentages nethealthData) {
	return s.newMetrics(testNode, val)
}

// newMetrics creates new metrics with the provided values.
func (s *NethealthSuite) newMetrics(peer string, val float64) (packetLossPercentages nethealthData) {
	packetLossPercentages = make(nethealthData)
	packetLossPercentages[peer] = val
	return packetLossPercentages
}

const (
	// testNode is used for 'node_name' or 'peer_name' value in test cases.
	testNode = "test-node"
	// testCapacity specifies the time series capacity used for test cases.
	testCapacity = 5
	// plt is packetLossThreshold alias
	plt = packetLossThreshold
	// abovePLT is arbitrary percentage above the packetLossThreshold
	abovePLT = packetLossThreshold + 0.01
)
