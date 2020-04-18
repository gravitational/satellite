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
	"fmt"
	"net"
	"strings"

	"github.com/gravitational/satellite/agent"
	"github.com/gravitational/satellite/agent/health"
	"github.com/gravitational/satellite/lib/test"

	serf "github.com/hashicorp/serf/client"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
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
				packetLoss: s.newPacketLoss(),
			},
			incomingData: []networkData{
				{},
				{},
				{},
				{},
				{},
			},
		},
		{
			comment: Commentf("Expected no data points to be recorded. Timeout inc > request inc"),
			expected: peerData{
				prevTimeoutTotal: 5,
				packetLoss:       s.newPacketLoss(),
			},
			incomingData: []networkData{
				{timeoutTotal: 1},
				{timeoutTotal: 2},
				{timeoutTotal: 3},
				{timeoutTotal: 4},
				{timeoutTotal: 5},
			},
		},
		{
			comment: Commentf("Expected zero packet loss to be recorded."),
			expected: peerData{
				prevRequestTotal: 5,
				packetLoss:       s.newPacketLoss(0, 0, 0, 0, 0),
			},
			incomingData: []networkData{
				{requestTotal: 1},
				{requestTotal: 2},
				{requestTotal: 3},
				{requestTotal: 4},
				{requestTotal: 5},
			},
		},
		{
			comment: Commentf("Expected 100%% packet loss to be recorded."),
			expected: peerData{
				prevRequestTotal: 5,
				prevTimeoutTotal: 5,
				packetLoss:       s.newPacketLoss(1, 1, 1, 1, 1),
			},
			incomingData: []networkData{
				{requestTotal: 1, timeoutTotal: 1},
				{requestTotal: 2, timeoutTotal: 2},
				{requestTotal: 3, timeoutTotal: 3},
				{requestTotal: 4, timeoutTotal: 4},
				{requestTotal: 5, timeoutTotal: 5},
			},
		},
		{
			comment: Commentf("Expected oldest data point to be removed"),
			expected: peerData{
				prevRequestTotal: 6,
				prevTimeoutTotal: 1,
				packetLoss:       s.newPacketLoss(0, 0, 0, 0, 0),
			},
			incomingData: []networkData{
				{requestTotal: 1, timeoutTotal: 1},
				{requestTotal: 2, timeoutTotal: 1},
				{requestTotal: 3, timeoutTotal: 1},
				{requestTotal: 4, timeoutTotal: 1},
				{requestTotal: 5, timeoutTotal: 1},
				{requestTotal: 6, timeoutTotal: 1},
			},
		},
	}

	for _, testCase := range testCases {
		checker := s.newNethealthChecker()

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
				packetLoss: s.newPacketLoss(1, 1, 1),
			},
		},
		{
			comment:  Commentf("Expected no failed probes. No timeouts."),
			expected: new(health.Probes),
			storedData: peerData{
				packetLoss: s.newPacketLoss(0, 0, 0, 0, 0),
			},
		},
		{
			comment:  Commentf("Expected no failed probes. Data points do not exceed threshold."),
			expected: new(health.Probes),
			storedData: peerData{
				packetLoss: s.newPacketLoss(plt, plt, plt, plt, plt),
			},
		},
		{
			comment:  Commentf("Expected no failed probes. Does not exceed threshold throughout entire interval."),
			expected: new(health.Probes),
			storedData: peerData{
				packetLoss: s.newPacketLoss(abovePLT, abovePLT, abovePLT, abovePLT, 0),
			},
		},
		{
			comment:  Commentf("Expected failed probe. Timeouts increase at each interval."),
			expected: &health.Probes{nethealthFailureProbe(nethealthCheckerID, testNode, abovePLT)},
			storedData: peerData{
				packetLoss: s.newPacketLoss(abovePLT, abovePLT, abovePLT, abovePLT, abovePLT),
			},
		},
	}

	checker := s.newNethealthChecker()

	for _, testCase := range testCases {
		reporter := new(health.Probes)
		c.Assert(checker.peerStats.Set(testNode, testCase.storedData), IsNil, testCase.comment)
		c.Assert(checker.verifyNethealth([]string{testNode}, reporter), IsNil, testCase.comment)
		c.Assert(reporter, test.DeepCompare, testCase.expected, testCase.comment)
	}
}

// TestParseMetricsSuccess verifies parseMetrics can successfully parse
// nethealth metrics.
func (s *NethealthSuite) TestParseMetricsSuccess(c *C) {
	var testCases = []struct {
		comment  CommentInterface
		expected map[string]networkData
		metrics  string
	}{

		{
			comment: Commentf("Expected networkData for peers 10.128.0.70 and 10.128.0.97."),
			expected: map[string]networkData{
				"10.128.0.70": {
					requestTotal: 236,
					timeoutTotal: 37,
				},
				"10.128.0.97": {
					requestTotal: 273,
					timeoutTotal: 0,
				},
			},
			metrics: `# HELP nethealth_echo_request_total The number of echo requests that have been sent
	      # TYPE nethealth_echo_request_total counter
	      nethealth_echo_request_total{node_name="10.128.0.96",peer_name="10.128.0.70"} 236
	      nethealth_echo_request_total{node_name="10.128.0.96",peer_name="10.128.0.97"} 273
	      # HELP nethealth_echo_timeout_total The number of echo requests that have timed out
	      # TYPE nethealth_echo_timeout_total counter
	      nethealth_echo_timeout_total{node_name="10.128.0.96",peer_name="10.128.0.70"} 37
		  nethealth_echo_timeout_total{node_name="10.128.0.96",peer_name="10.128.0.97"} 0
		  `,
		},
	}

	for _, testCase := range testCases {
		netData, err := parseMetrics([]byte(testCase.metrics))
		c.Assert(err, IsNil, testCase.comment)
		c.Assert(netData, test.DeepCompare, testCase.expected)
	}
}

// TestParseMetricsFailure verifies parseMetrics correctly handles invalid
// metrics input.
func (s *NethealthSuite) TestParseMetricsFailure(c *C) {
	var testCases = []struct {
		comment  CommentInterface
		expected string
		metrics  string
	}{
		{
			comment:  Commentf("Expected missing request metrics error."),
			expected: fmt.Sprintf("failed to parse echo requests\n\t%s metrics not found", echoRequestLabel),
			metrics: `# HELP nethealth_echo_timeout_total The number of echo requests that have timed out
	      # TYPE nethealth_echo_timeout_total counter
	      nethealth_echo_timeout_total{node_name="10.128.0.96",peer_name="10.128.0.70"} 37
		  nethealth_echo_timeout_total{node_name="10.128.0.96",peer_name="10.128.0.97"} 0
		  `,
		},
		{
			comment:  Commentf("Expected missing timeout metrics error."),
			expected: fmt.Sprintf("failed to parse echo timeouts\n\t%s metrics not found", echoTimeoutLabel),
			metrics: `# HELP nethealth_echo_request_total The number of echo requests that have been sent
	      # TYPE nethealth_echo_request_total counter
	      nethealth_echo_request_total{node_name="10.128.0.96",peer_name="10.128.0.70"} 236
	      nethealth_echo_request_total{node_name="10.128.0.96",peer_name="10.128.0.97"} 273
		  `,
		},
		{
			comment: Commentf("Expected error about unequal counters."),
			expected: "received incomplete pair(s) of nethealth metrics. " +
				"This may be due to a bug in prometheus or in the way nethealth is exposing its metrics",
			metrics: `# HELP nethealth_echo_request_total The number of echo requests that have been sent
	      # TYPE nethealth_echo_request_total counter
	      nethealth_echo_request_total{node_name="10.128.0.96",peer_name="10.128.0.70"} 236
	      nethealth_echo_request_total{node_name="10.128.0.96",peer_name="10.128.0.97"} 273
		  # HELP nethealth_echo_timeout_total The number of echo requests that have timed out
	      # TYPE nethealth_echo_timeout_total counter
	      nethealth_echo_timeout_total{node_name="10.128.0.96",peer_name="10.128.0.70"} 37
		  `,
		},
		{
			comment:  Commentf("Expected error about missing timeout counter."),
			expected: "echo timeout data not available for 10.128.0.97",
			metrics: `# HELP nethealth_echo_request_total The number of echo requests that have been sent
	      # TYPE nethealth_echo_request_total counter
	      nethealth_echo_request_total{node_name="10.128.0.96",peer_name="10.128.0.97"} 273
		  # HELP nethealth_echo_timeout_total The number of echo requests that have timed out
	      # TYPE nethealth_echo_timeout_total counter
	      nethealth_echo_timeout_total{node_name="10.128.0.96",peer_name="10.128.0.70"} 37
		  `,
		},
	}

	for _, testCase := range testCases {
		_, err := parseMetrics([]byte(testCase.metrics))
		c.Assert(err.Error(), Equals, testCase.expected, testCase.comment)
	}
}

// TestFilterNetData verifies filterNetData correctly filters irrelevant data.
func (s *NethealthSuite) TestFilterNetData(c *C) {
	var testCases = []struct {
		comment  CommentInterface
		expected map[string]networkData
		netData  map[string]networkData
		members  []serf.Member
	}{
		{
			comment: Commentf("Expected netData present for all members. No members have left the cluster."),
			expected: map[string]networkData{
				"172.28.128.101": {},
			},
			netData: map[string]networkData{
				"172.28.128.101": {},
			},
			members: []serf.Member{
				s.newSerfMember("172.28.128.101", agent.MemberAlive),
			},
		},
		{
			comment: Commentf("Expected netData only for the single member that is `Alive`"),
			expected: map[string]networkData{
				"172.28.128.101": {},
			},
			netData: map[string]networkData{
				"172.28.128.101": {},
				"172.28.128.102": {},
				"172.28.128.103": {},
				"172.28.128.104": {},
				"172.28.128.105": {},
			},
			members: []serf.Member{
				s.newSerfMember("172.28.128.101", agent.MemberAlive),
				s.newSerfMember("172.28.128.102", agent.MemberLeft),
				s.newSerfMember("172.28.128.103", agent.MemberLeaving),
				s.newSerfMember("172.28.128.104", agent.MemberFailed),
			},
		},
	}

	for _, testCase := range testCases {
		filtered, err := filterBySerf(testCase.netData, testCase.members)
		c.Assert(err, IsNil, testCase.comment)
		c.Assert(filtered, test.DeepCompare, testCase.expected, testCase.comment)
	}
}

// newNethealthChecker returns a new nethealth checker to be used for testing.
func (s *NethealthSuite) newNethealthChecker() *nethealthChecker {
	return &nethealthChecker{
		peerStats: newNetStats(netStatsCapacity, testCapacity),
	}
}

// newPacketLoss constructs a new slice with capacity equal to testCapacity.
// Provided packetLoss values are appended to the new slice.
func (s *NethealthSuite) newPacketLoss(values ...float64) []float64 {
	packetLoss := make([]float64, 0, testCapacity)
	packetLoss = append(packetLoss, values...)
	return packetLoss
}

// textToMetrics converts the string into a map of MetricFamilies.
func (s *NethealthSuite) textToMetrics(metrics string) (map[string]*dto.MetricFamily, error) {
	var parser expfmt.TextParser
	return parser.TextToMetricFamilies(strings.NewReader(metrics))
}

// newSerfMember constructs a new serf member with the provided IP and status.
func (s *NethealthSuite) newSerfMember(ip string, status agent.MemberStatus) serf.Member {
	return serf.Member{
		Addr:   net.ParseIP(ip),
		Status: string(status),
	}
}

const (
	// testNode is used for 'node_name' or 'peer_name' value in test cases.
	testNode = "test-node"
	// testCapacity specifies the packetLoss capacity to use in test cases.
	testCapacity = 5
	// plt is alias for packetLossThreshold
	plt = packetLossThreshold
	// abovePLT is arbitrary value above packetLossThreshold
	abovePLT = packetLossThreshold + .01
)
