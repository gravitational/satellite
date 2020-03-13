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

	. "gopkg.in/check.v1"
)

type NethealthSuite struct{}

var _ = Suite(&NethealthSuite{})

// TestUpdateTimeoutStats verifies the checker can properly record timeout
// data points from available metrics.
func (s *NethealthSuite) TestUpdateTimeoutStats(c *C) {
	var testCases = []struct {
		comment            CommentInterface
		expectedPacketLoss []float64
		expectedUpdated    []string
		requests           []float64
		timeouts           []float64
	}{
		{
			comment:            Commentf("Expected no data points to be recorded. Requests did not increase."),
			expectedPacketLoss: nil,
			expectedUpdated:    nil,
			requests:           []float64{0, 0, 0, 0, 0},
			timeouts:           []float64{0, 0, 0, 0, 0},
		},
		{
			comment:            Commentf("Expected no data points to be recorded. Timeout inc > request inc"),
			expectedPacketLoss: nil,
			expectedUpdated:    nil,
			requests:           []float64{0, 0, 0, 0, 0},
			timeouts:           []float64{1, 2, 3, 4, 5},
		},
		{
			comment:            Commentf("Expected zero packet loss to be recorded."),
			expectedPacketLoss: []float64{0, 0, 0, 0, 0},
			expectedUpdated:    []string{testNode},
			requests:           []float64{1, 2, 3, 4, 5},
			timeouts:           []float64{0, 0, 0, 0, 0},
		},
		{
			comment:            Commentf("Expected 100% packet loss to be recorded."),
			expectedPacketLoss: []float64{1, 1, 1, 1, 1},
			expectedUpdated:    []string{testNode},
			requests:           []float64{1, 2, 3, 4, 5},
			timeouts:           []float64{1, 2, 3, 4, 5},
		},
		{
			comment:            Commentf("Expected oldest data point to be removed"),
			expectedPacketLoss: []float64{0, 0, 0, 0, 0},
			expectedUpdated:    []string{testNode},
			requests:           []float64{1, 2, 3, 4, 5, 6},
			timeouts:           []float64{1, 1, 1, 1, 1, 1},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		checker, err := s.newNethealthChecker()
		c.Assert(err, IsNil)

		for i, requestCount := range testCase.requests {
			timeoutCount := testCase.timeouts[i]

			requestData := s.newDataWithCount(requestCount)
			timeoutData := s.newDataWithCount(timeoutCount)

			updated, err := checker.updateStats(requestData, timeoutData)
			c.Assert(err, IsNil, testCase.comment)
			c.Assert(updated, test.DeepCompare, testCase.expectedUpdated, testCase.comment)
		}

		packetLoss, err := checker.peerStats.GetPacketLoss(testNode)
		c.Assert(err, IsNil, testCase.comment)
		c.Assert(packetLoss, test.DeepCompare, testCase.expectedPacketLoss, testCase.comment)
	}
}

// TestNethealthChecker verifies nethealth checker can properly detect
// healthy/unhealthy network.
func (s *NethealthSuite) TestNethealthVerification(c *C) {
	var testCases = []struct {
		comment    CommentInterface
		expected   health.Reporter
		packetLoss []float64
	}{
		{
			comment:    Commentf("Expected no failed probes. Not enough data points."),
			expected:   new(health.Probes),
			packetLoss: []float64{abovePLT, abovePLT, abovePLT},
		},
		{
			comment:    Commentf("Expected no failed probes. No timeouts."),
			expected:   new(health.Probes),
			packetLoss: []float64{0, 0, 0, 0, 0},
		},
		{
			comment:    Commentf("Expected no failed probes. Data points do not exceed threshold."),
			expected:   new(health.Probes),
			packetLoss: []float64{plt, plt, plt, plt, plt},
		},
		{
			comment:    Commentf("Expected no failed probes. Does not exceed threshold throughout entire interval."),
			expected:   new(health.Probes),
			packetLoss: []float64{abovePLT, abovePLT, abovePLT, abovePLT, plt},
		},
		{
			comment:    Commentf("Expected failed probe. Timeouts increase at each interval."),
			expected:   &health.Probes{NewProbeFromErr(nethealthCheckerID, nethealthDetail(testNode), nil)},
			packetLoss: []float64{abovePLT, abovePLT, abovePLT, abovePLT, abovePLT},
		},
	}

	checker, err := s.newNethealthChecker()
	c.Assert(err, IsNil)

	for _, testCase := range testCases {
		testCase := testCase

		data := peerData{packetLossPercentages: testCase.packetLoss}
		c.Assert(checker.peerStats.Set(testNode, data), IsNil, testCase.comment)

		reporter, err := checker.verifyNethealth([]string{testNode})
		c.Assert(err, IsNil, testCase.comment)
		c.Assert(reporter, test.DeepCompare, testCase.expected, testCase.comment)
	}
}

// newNethealthChecker returns a new nethealth checker to be used for testing.
func (s *NethealthSuite) newNethealthChecker() (*nethealthChecker, error) {
	return &nethealthChecker{
		peerStats:      newNetStats(netStatsCapacity),
		seriesCapacity: testCapacity,
	}, nil
}

// newDataWithCount returns a map with the testNode mapped to the count.
func (s *NethealthSuite) newDataWithCount(count float64) map[string]float64 {
	return s.newData(testNode, count)
}

// newData returns a map with the peer mapped to the count.
func (s *NethealthSuite) newData(peer string, count float64) map[string]float64 {
	data := make(map[string]float64)
	data[peer] = count
	return data
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
