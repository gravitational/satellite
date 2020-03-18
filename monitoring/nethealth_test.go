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
		comment      CommentInterface
		expected     peerData
		incomingData []networkData
	}{
		{
			comment: Commentf("Expected no data points to be recorded. Requests did not increase."),
			expected: peerData{
				prevRequestTotal: 0,
				prevTimeoutTotal: 0,
				requestInc:       nil,
				timeoutInc:       nil,
			},
			incomingData: []networkData{
				networkData{requestTotal: 0, timeoutTotal: 0},
				networkData{requestTotal: 0, timeoutTotal: 0},
				networkData{requestTotal: 0, timeoutTotal: 0},
				networkData{requestTotal: 0, timeoutTotal: 0},
				networkData{requestTotal: 0, timeoutTotal: 0},
			},
		},
		{
			comment: Commentf("Expected no data points to be recorded. Timeout inc > request inc"),
			expected: peerData{
				prevRequestTotal: 0,
				prevTimeoutTotal: 5,
				requestInc:       nil,
				timeoutInc:       nil,
			},
			incomingData: []networkData{
				networkData{requestTotal: 0, timeoutTotal: 1},
				networkData{requestTotal: 0, timeoutTotal: 2},
				networkData{requestTotal: 0, timeoutTotal: 3},
				networkData{requestTotal: 0, timeoutTotal: 4},
				networkData{requestTotal: 0, timeoutTotal: 5},
			},
		},
		{
			comment: Commentf("Expected zero packet loss to be recorded."),
			expected: peerData{
				prevRequestTotal: 5,
				prevTimeoutTotal: 0,
				requestInc:       []float64{1, 1, 1, 1, 1},
				timeoutInc:       []float64{0, 0, 0, 0, 0},
			},
			incomingData: []networkData{
				networkData{requestTotal: 1, timeoutTotal: 0},
				networkData{requestTotal: 2, timeoutTotal: 0},
				networkData{requestTotal: 3, timeoutTotal: 0},
				networkData{requestTotal: 4, timeoutTotal: 0},
				networkData{requestTotal: 5, timeoutTotal: 0},
			},
		},
		{
			comment: Commentf("Expected 100% packet loss to be recorded."),
			expected: peerData{
				prevRequestTotal: 5,
				prevTimeoutTotal: 5,
				requestInc:       []float64{1, 1, 1, 1, 1},
				timeoutInc:       []float64{1, 1, 1, 1, 1},
			},
			incomingData: []networkData{
				networkData{requestTotal: 1, timeoutTotal: 1},
				networkData{requestTotal: 2, timeoutTotal: 2},
				networkData{requestTotal: 3, timeoutTotal: 3},
				networkData{requestTotal: 4, timeoutTotal: 4},
				networkData{requestTotal: 5, timeoutTotal: 5},
			},
		},
		{
			comment: Commentf("Expected oldest data point to be removed"),
			expected: peerData{
				prevRequestTotal: 7,
				prevTimeoutTotal: 2,
				requestInc:       []float64{1, 1, 1, 1, 2},
				timeoutInc:       []float64{0, 0, 0, 0, 1},
			},
			incomingData: []networkData{
				networkData{requestTotal: 1, timeoutTotal: 1},
				networkData{requestTotal: 2, timeoutTotal: 1},
				networkData{requestTotal: 3, timeoutTotal: 1},
				networkData{requestTotal: 4, timeoutTotal: 1},
				networkData{requestTotal: 5, timeoutTotal: 1},
				networkData{requestTotal: 7, timeoutTotal: 2},
			},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		checker, err := s.newNethealthChecker()
		c.Assert(err, IsNil)

		for _, data := range testCase.incomingData {
			c.Assert(checker.updatePeer(testNode, data), IsNil, testCase.comment)
		}

		data, err := checker.peerStats.Get(testNode)
		c.Assert(err, IsNil, testCase.comment)
		c.Assert(data, test.DeepCompare, testCase.expected, testCase.comment)
	}
}

// TestNethealthChecker verifies nethealth checker can properly detect
// healthy/unhealthy network.
func (s *NethealthSuite) TestNethealthVerification(c *C) {
	var testCases = []struct {
		comment    CommentInterface
		expected   health.Reporter
		packetLoss []float64
		storedData peerData
	}{
		{
			comment:  Commentf("Expected no failed probes. Not enough data points."),
			expected: new(health.Probes),
			storedData: peerData{
				requestInc: []float64{10, 20, 30},
				timeoutInc: []float64{5, 10, 15},
			},
		},
		{
			comment:  Commentf("Expected no failed probes. No timeouts."),
			expected: new(health.Probes),
			storedData: peerData{
				requestInc: []float64{10, 20, 30, 40, 50},
				timeoutInc: []float64{0, 0, 0, 0, 0},
			},
		},
		{
			comment:  Commentf("Expected no failed probes. Data points do not exceed threshold."),
			expected: new(health.Probes),
			storedData: peerData{
				requestInc: []float64{10, 20, 30, 40, 50},
				timeoutInc: []float64{2, 4, 6, 8, 10},
			},
		},
		{
			comment:  Commentf("Expected no failed probes. Does not exceed threshold throughout entire interval."),
			expected: new(health.Probes),
			storedData: peerData{
				requestInc: []float64{10, 20, 30, 40, 50},
				timeoutInc: []float64{10, 0, 0, 0, 0},
			},
		},
		{
			comment:  Commentf("Expected failed probe. Timeouts increase at each interval."),
			expected: &health.Probes{NewProbeFromErr(nethealthCheckerID, nethealthDetail(testNode), nil)},
			storedData: peerData{
				requestInc: []float64{10, 20, 30, 40, 50},
				timeoutInc: []float64{10, 20, 30, 40, 50},
			},
		},
	}

	checker, err := s.newNethealthChecker()
	c.Assert(err, IsNil)

	for _, testCase := range testCases {
		testCase := testCase
		reporter := new(health.Probes)

		c.Assert(checker.peerStats.Set(testNode, testCase.storedData), IsNil, testCase.comment)

		c.Assert(checker.verifyNethealth([]string{testNode}, reporter), IsNil, testCase.comment)
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

// newNetData wraps the counters into networkData and maps it to the test node.
func (s *NethealthSuite) newNetData(requestTotal, timeoutTotal float64) map[string]networkData {
	data := make(map[string]networkData)
	data[testNode] = networkData{
		requestTotal: requestTotal,
		timeoutTotal: timeoutTotal,
	}
	return data
}

const (
	// testNode is used for 'node_name' or 'peer_name' value in test cases.
	testNode = "test-node"
	// testCapacity specifies the time series capacity used for test cases.
	testCapacity = 5
)
