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
	"context"
	"fmt"
	"math"
	"net/url"
	"sync"
	"time"

	"github.com/gravitational/satellite/agent"
	"github.com/gravitational/satellite/agent/health"

	"github.com/gravitational/roundtrip"
	"github.com/gravitational/trace"
	"github.com/mailgun/holster"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NethealthConfig specifies configuration for a nethealth checker.
type NethealthConfig struct {
	// AdvertiseIP specifies the advertised ip address of the host running this checker.
	AdvertiseIP string
	// NethealthPort specifies the port that nethealth is listening on.
	NethealthPort int
	// NetStatsInterval specifies the duration to store net stats.
	NetStatsInterval time.Duration
	// KubeConfig specifies kubernetes access information.
	*KubeConfig
}

// CheckAndSetDefaults validates that this configuration is correct and sets
// value defaults where necessary.
func (c *NethealthConfig) CheckAndSetDefaults() error {
	var errors []error
	if c.AdvertiseIP == "" {
		errors = append(errors, trace.BadParameter("host advertise ip must be provided"))
	}
	if c.KubeConfig == nil {
		errors = append(errors, trace.BadParameter("kubernetes access config must be provided"))
	}
	if c.NethealthPort == 0 {
		c.NethealthPort = defaultNethealthPort
	}
	if c.NetStatsInterval == time.Duration(0) {
		c.NetStatsInterval = defaultNetStatsInterval
	}
	return trace.NewAggregate(errors...)
}

// nethealthChecker checks network communication between peers.
type nethealthChecker struct {
	// NethealthConfig contains caller specified nethealth checker configuration
	// values.
	NethealthConfig
	// seriesCapacity specifies the number of data points to store in a series.
	seriesCapacity int
	// Mutex locks access to peerStats
	sync.Mutex
	// peerStats maps a peer to its recorded nethealth stats.
	peerStats netStats
}

// NewNethealthChecker returns a new nethealth checker.
func NewNethealthChecker(config NethealthConfig) (*nethealthChecker, error) {
	if err := config.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}

	// seriesCapacity is determined by the NetStatsInterval / StatusUpdateTimeout
	// rounded up to the nearest integer
	seriesCapacity := math.Ceil(float64(config.NetStatsInterval) / float64(agent.StatusUpdateTimeout))

	return &nethealthChecker{
		peerStats:       newNetStats(netStatsCapacity),
		seriesCapacity:  int(seriesCapacity),
		NethealthConfig: config,
	}, nil
}

// Name returns this checker name
// Implements health.Checker
func (c *nethealthChecker) Name() string {
	return nethealthCheckerID
}

// Check verifies the network is healthy.
// Implements health.Checker
func (c *nethealthChecker) Check(ctx context.Context, reporter health.Reporter) {
	probes, err := c.check(ctx)
	if err != nil {
		log.WithError(err).Error("Unable to verify nethealth.")
	}

	if probes.NumProbes() != 0 {
		health.AddFrom(reporter, probes)
		return
	}
	reporter.Add(NewSuccessProbe(c.Name()))
}

func (c *nethealthChecker) check(ctx context.Context) (health.Reporter, error) {
	reporter := new(health.Probes)
	pods, err := c.Client.CoreV1().Pods(nethealthNamespace).List(metav1.ListOptions{})
	if err != nil {
		reporter.Add(NewProbeFromErr(c.Name(), "failed to retrieve k8s pods", err))
		return reporter, trace.Wrap(err)
	}

	addr, err := c.getNethealthAddr(pods)
	if err != nil {
		return reporter, trace.Wrap(err, "failed to get local nethealth address")
	}

	metricFamilies, err := fetchMetrics(ctx, addr)
	if err != nil {
		reporter.Add(NewProbeFromErr(c.Name(), "unable to nethealth metrics", err))
		return reporter, trace.Wrap(err, "failed to fetch metrics from %s", addr)
	}

	echoRequests, err := parseCounter(metricFamilies, echoRequestLabel)
	if err != nil {
		return reporter, trace.Wrap(err, "failed to parse echo requests")
	}

	echoTimeouts, err := parseCounter(metricFamilies, echoTimeoutLabel)
	if err != nil {
		return reporter, trace.Wrap(err, "failed to parse echo timeouts")
	}

	updated, err := c.updateStats(echoRequests, echoTimeouts)
	if err != nil {
		return reporter, trace.Wrap(err, "failed to update nethealth stats")
	}

	return c.verifyNethealth(updated)
}

// getNethealthAddr returns the address of the local nethealth pod.
func (c *nethealthChecker) getNethealthAddr(pods *v1.PodList) (string, error) {
	// Find nethealth pod with matching host ip address.
	for _, pod := range pods.Items {
		if pod.GetLabels()[nethealthLabel] != nethealthValue {
			continue
		}
		if pod.Status.HostIP == c.AdvertiseIP {
			return fmt.Sprintf("http://%s:%d", pod.Status.PodIP, c.NethealthPort), nil
		}
	}
	return "", trace.NotFound("unable to find local nethealth pod")
}

// fetchMetrics collects the network metrics from the nethealth pod.
func fetchMetrics(ctx context.Context, addr string) (metricFamilies map[string]*dto.MetricFamily, err error) {
	client, err := roundtrip.NewClient(addr, "")
	if err != nil {
		return nil, trace.Wrap(err, "failed to connect to nethealth service at %s.", addr)
	}

	resp, err := client.Get(ctx, client.Endpoint("metrics"), url.Values{})
	if err != nil {
		return nil, trace.Wrap(err)
	}

	var parser expfmt.TextParser
	metricsFamilies, err := parser.TextToMetricFamilies(resp.Reader())
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return metricsFamilies, nil
}

// updateStats updates netStats with new data collected in the provided
// MetricFamily.
func (c *nethealthChecker) updateStats(echoRequests, echoTimeouts map[string]float64) (updated []string, err error) {
	if len(echoRequests) != len(echoTimeouts) {
		return updated, trace.BadParameter("recieved %d timeout counters and %d request counters",
			len(echoRequests), len(echoTimeouts))
	}

	for peer, requests := range echoRequests {
		timeouts, ok := echoTimeouts[peer]
		if !ok {
			return updated, trace.BadParameter("echo timeout data not available for %s", peer)
		}

		data, err := c.peerStats.Get(peer)
		if err != nil {
			return updated, trace.Wrap(err)
		}

		// Calcuate counter increase since last check and replace previous total
		requestInc := requests - data.prevRequestTotal
		data.prevRequestTotal = requests
		timeoutInc := timeouts - data.prevTimeoutTotal
		data.prevTimeoutTotal = timeouts

		// Request counter should be constantly increasing, if this is not the
		// case, nethealth pod most likely reset the counter. Do no update
		// packet loss percentages this round.
		if requestInc <= 0 {
			log.Warn("Request counter did not increase. Nethealth pod may have restarted.")
			continue
		}

		// It should not be possible for the timeout counter to have increased
		// more than the request counter. Log and ignore this situation.
		if timeoutInc > requestInc {
			log.WithField("request-inc", requestInc).
				WithField("timeout-inc", timeoutInc).
				Warn("Timeout counter increased more than request counter.")
			continue
		}

		// Record the new packet loss percentage, but only keep the last `seriesCapacity` number of data points.
		if len(data.packetLossPercentages) >= c.seriesCapacity {
			data.packetLossPercentages = data.packetLossPercentages[1:]
		}
		packetLoss := timeoutInc / requestInc
		data.packetLossPercentages = append(data.packetLossPercentages, packetLoss)

		if err := c.peerStats.Set(peer, data); err != nil {
			return updated, trace.Wrap(err)
		}

		// Record updated peers to be returned for later use in network verification step.
		updated = append(updated, peer)
	}
	return updated, nil
}

// verifyNethealth verifies that the overlay network communication is healthy
// for the nodes specified by the provided list of peers. Failed probes will be
// reported for unhealthy peers.
func (c *nethealthChecker) verifyNethealth(peers []string) (health.Reporter, error) {
	reporter := new(health.Probes)
	for _, peer := range peers {
		healthy, err := c.isHealthy(peer)
		if err != nil {
			return reporter, trace.Wrap(err)
		}
		if !healthy {
			reporter.Add(NewProbeFromErr(c.Name(), nethealthDetail(peer), nil))
		}
	}
	return reporter, nil
}

// isHealthy returns true if the overlay network is healthy for the specified
// peer.
func (c *nethealthChecker) isHealthy(peer string) (healthy bool, err error) {
	packetLossPercentages, err := c.peerStats.GetPacketLoss(peer)
	if err != nil {
		return false, trace.Wrap(err)
	}

	// Checker has not collected enough data yet to check network health.
	if len(packetLossPercentages) < c.seriesCapacity {
		return true, nil
	}

	// If the percentage is above `packetLossTreshhold` throughout the entire
	// interval, the overlay network communication to that peer will be
	// considered unhealthy.
	for i := 0; i < c.seriesCapacity; i++ {
		if packetLossPercentages[i] <= packetLossThreshold {
			return true, nil
		}
	}
	return false, nil
}

// parseCounter parses the provided metricFamilies and returns a map of counters
// for the desired metrics specified by label.
func parseCounter(metricFamilies map[string]*dto.MetricFamily, label string) (counters map[string]float64, err error) {
	metricFamily, ok := metricFamilies[label]
	if !ok {
		return nil, trace.NotFound("%s metrics not found", label)
	}

	counters = make(map[string]float64)
	for _, m := range metricFamily.GetMetric() {
		peerName, err := getPeerName(m.GetLabel())
		if err != nil {
			return nil, trace.Wrap(err)
		}
		counters[peerName] = m.GetCounter().GetValue()
	}
	return counters, nil
}

// getPeerName extracts the 'peer_name' value from the provided labels.
func getPeerName(labels []*dto.LabelPair) (peer string, err error) {
	for _, label := range labels {
		if peerLabel == label.GetName() {
			return label.GetValue(), nil
		}
	}
	return "", trace.NotFound("unable to find %s label", peerLabel)
}

// nethealthDetail returns a failed probe detail message.
func nethealthDetail(name string) string {
	return fmt.Sprintf("overlay network communication failure with %s", name)
}

// netStats holds nethealth data for a peer.
type netStats struct {
	sync.Mutex
	*holster.TTLMap
}

// newNetStats constructs a new netStats.
func newNetStats(capacity int) netStats {
	return netStats{TTLMap: holster.NewTTLMap(capacity)}
}

// Get returns the peerData for the specified peer.
func (r netStats) Get(peer string) (data peerData, err error) {
	r.Lock()
	defer r.Unlock()
	if value, ok := r.TTLMap.Get(peer); ok {
		if data, ok = value.(peerData); !ok {
			return data, trace.BadParameter("expected %T, got %T", data, value)
		}
	}
	return data, nil
}

// Set maps the specified peer and data.
func (r netStats) Set(peer string, data peerData) error {
	r.Lock()
	defer r.Unlock()
	return r.TTLMap.Set(peer, data, netStatsTTLSeconds)
}

// GetPacketLoss returns the packet loss percentages for the specified peer.
func (r netStats) GetPacketLoss(peer string) (packetLoss []float64, err error) {
	data, err := r.Get(peer)
	if err != nil {
		return packetLoss, trace.Wrap(err)
	}
	return data.packetLossPercentages, nil
}

// peerData keeps trock of relevant nethealth data for a node.
type peerData struct {
	// prevRequestTotal keeps track of the previously recorded total number of
	// requests for each peer. This is necessary to calculate the number of new
	// requests received since the last check.
	prevRequestTotal float64
	// prevTimeoutTotal keeps track of the previously recorded total number of
	// timeouts for each peer. This is necessary to calculate the number of new
	// timeouts received since the last check.
	prevTimeoutTotal float64
	// packetLoss is time series data containing the packet loss percentage for
	// a set interval.
	packetLossPercentages []float64
}

const (
	nethealthCheckerID = "nethealth-checker"
	peerLabel          = "peer_name"
	nethealthNamespace = "monitoring"
	nethealthLabel     = "k8s-app"
	nethealthValue     = "nethealth"
	// echoRequestLabel defines the metric family label for the cho request counter
	echoRequestLabel = "nethealth_echo_request_total"
	// echoTimeoutLabel defines the metric family label for the echo timeout counter
	echoTimeoutLabel = "nethealth_echo_timeout_total"

	// defaultNethealthPort defines the default nethealth port.
	defaultNethealthPort = 9801

	// defaultNetStatsInterval defines the default interval duration for the netStats.
	defaultNetStatsInterval = 5 * time.Minute

	// netStatsCapacity sets the number of TTLMaps that can be stored.
	// This will be the size of the cluster -1.
	netStatsCapacity = 1000

	// netStatsTTLSeconds defines the time to live in seconds for the stored
	// netStats. This ensure the checker does not hold on to unused data when
	// a member leaves the cluster.
	netStatsTTLSeconds = 60 * 60 * 24 // 1 day

	// packetLossThreshold defines the packet loss percentage used to determine
	// if overlay network communication with a peer is unhealthy.
	packetLossThreshold = 0.20 // packet loss > 20% is unhealthy
)
