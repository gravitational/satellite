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
	"github.com/gravitational/satellite/lib/test"

	"github.com/mailgun/holster"
	dto "github.com/prometheus/client_model/go"
	. "gopkg.in/check.v1"
)

type NethealthSuite struct{}

var _ = Suite(&NethealthSuite{})

// TestUpdateTimeoutStats verifies the checker can properly record timeout
// data points from available metrics.
func (s *NethealthSuite) TestUpdateTimeoutStats(c *C) {
	var testCases = []struct {
		comment  CommentInterface
		expected []int64
		data     []float64
	}{
		{
			comment:  Commentf("Expected all data points to be recorded."),
			expected: []int64{0, 0, 0, 0, 0},
			data:     []float64{0, 0, 0, 0, 0},
		},
		{
			comment:  Commentf("Expected oldest data point to be removed"),
			expected: []int64{0, 0, 0, 0, 0},
			data:     []float64{1, 0, 0, 0, 0, 0},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		checker, err := s.newNethealthChecker()
		c.Assert(err, IsNil)

		for _, count := range testCase.data {
			updated, err := checker.updateStats(s.newMetricsWithCount(count))
			c.Assert(err, IsNil, testCase.comment)
			c.Assert(updated, test.DeepCompare, []string{testNode}, testCase.comment)
		}

		series, err := checker.getTimeoutSeries(testNode)
		c.Assert(err, IsNil, testCase.comment)
		c.Assert(series, test.DeepCompare, testCase.expected, testCase.comment)
	}
}

// TestNethealthChecker verifies nethealth checker can properly detect
// healthy/unhealthy network.
func (s *NethealthSuite) TestNethealthVerification(c *C) {
	var testCases = []struct {
		comment  CommentInterface
		expected Checker
		data     []int64
	}{
		{
			comment:  Commentf("Expected healthy network check. Not enough data points."),
			expected: IsNil,
			data:     []int64{0, 1, 2},
		},
		{
			comment:  Commentf("Expected healthy network check. No timeouts."),
			expected: IsNil,
			data:     []int64{0, 0, 0, 0, 0},
		},
		{
			comment:  Commentf("Expected healthy network check. Timeouts do not increase for a long enough duration"),
			expected: IsNil,
			data:     []int64{0, 1, 2, 3, 3},
		},
		{
			comment:  Commentf("Expected unhealthy network check. Timeouts increase at each interval."),
			expected: Not(IsNil),
			data:     []int64{0, 1, 2, 3, 4},
		},
	}

	checker, err := s.newNethealthChecker()
	c.Assert(err, IsNil)

	for _, testCase := range testCases {
		testCase := testCase
		c.Assert(checker.setTimeoutSeries(testNode, testCase.data), IsNil, testCase.comment)
		c.Assert(checker.verifyNethealth([]string{testNode}), testCase.expected, testCase.comment)
	}
}

// newNethealthChecker returns a new nethealth checker to be used for testing.
func (s *NethealthSuite) newNethealthChecker() (*nethealthChecker, error) {
	config := NethealthConfig{
		SeriesCapacity: testCapacity,
	}

	return &nethealthChecker{
		timeoutStats:    holster.NewTTLMap(testCapacity),
		NethealthConfig: config,
	}, nil
}

// newMetricsWithCount creates new metrics with the provided count.
func (s *NethealthSuite) newMetricsWithCount(count float64) []*dto.Metric {
	return s.newMetrics(testNode, count)
}

// newMetrics creates new metrics with the provided values.
func (s *NethealthSuite) newMetrics(peerValue string, count float64) []*dto.Metric {
	peer := peerLabel

	return []*dto.Metric{
		&dto.Metric{
			Label: []*dto.LabelPair{
				&dto.LabelPair{Name: &peer, Value: &peerValue},
			},
			Counter: &dto.Counter{Value: &count},
		},
	}
}

const (
	// testNode is used for 'node_name' or 'peer_name' value in test cases.
	testNode = "test-node"
	// testCapacity specifies the time series capacity used for test cases.
	testCapacity = 5
)
