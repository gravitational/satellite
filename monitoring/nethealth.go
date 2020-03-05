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
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"sync"

	"github.com/gravitational/satellite/agent/health"

	"github.com/gravitational/roundtrip"
	"github.com/gravitational/trace"
	"github.com/mailgun/holster"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NethealthConfig specifies configuration for a nethealth checker.
type NethealthConfig struct {
	// AdvertiseIP specifies the advertised ip address of the host running this checker.
	AdvertiseIP string
	// NethealthPort specifies the port that nethealth is listening on.
	NethealthPort int
	// SeriesCapacity specifies the max number of data points to store in a time
	// series interval.
	SeriesCapacity int
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
	if c.NethealthPort == 0 {
		c.NethealthPort = defaultNethealthPort
	}
	if c.SeriesCapacity < 0 {
		errors = append(errors, trace.BadParameter("timeout series capacity cannot be < 0"))
	}
	if c.SeriesCapacity == 0 {
		c.SeriesCapacity = defaultSeriesCapacity
	}
	if c.KubeConfig == nil {
		errors = append(errors, trace.BadParameter("kubernetes access config must be provided"))
	}
	return trace.NewAggregate(errors...)
}

// nethealthChecker checks network communication between peers.
type nethealthChecker struct {
	// NethealthConfig contains caller specified nethealth checker configuration
	// values.
	NethealthConfig
	// lock access to timeoutStats
	sync.Mutex
	// timeoutStats maps a peer to a time series data interval containing the
	// number of echo timeouts received by the specific peer for the set interval.
	timeoutStats *holster.TTLMap
}

// NewNethealthChecker returns a new nethealth checker.
func NewNethealthChecker(config NethealthConfig) (*nethealthChecker, error) {
	if err := config.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}

	return &nethealthChecker{
		timeoutStats:    holster.NewTTLMap(timeoutStatsCapacity),
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
	if err := c.check(ctx, reporter); err != nil {
		log.WithError(err).Error("Unable to verify nethealth.")
		return
	}
	reporter.Add(NewSuccessProbe(c.Name()))
}

func (c *nethealthChecker) check(ctx context.Context, reporter health.Reporter) error {
	metrics, err := c.getMetrics(ctx)
	if err != nil {
		return trace.Wrap(err, "failed to get nethealth metrics")
	}

	updated, err := c.updateStats(metrics)
	if err != nil {
		return trace.Wrap(err, "failed to update nethealth timeout stats")
	}

	return c.verifyNethealth(updated, reporter)
}

// getMetrics returns the network metrics from the local nethealth pod.
func (c *nethealthChecker) getMetrics(ctx context.Context) ([]nethealthMetric, error) {
	addr, err := c.getNethealthAddr()
	if err != nil {
		return nil, trace.Wrap(err, "failed to get local nethealth address")
	}

	b, err := fetchMetrics(ctx, addr)
	if err != nil {
		log.WithError(err).Warnf("Failed to fetch metrics from %s", addr)
		return nil, trace.Wrap(err, "failed to fetch metrics")
	}

	metrics, err := parseMetrics(bytes.NewReader(b))
	if err != nil {
		return nil, trace.Wrap(err, "failed to parse metrics")
	}

	return metrics, nil
}

// updateStats updates the timeout data with new data points collected in
// the provided metrics. Returns a list containing the updated keys.
func (c *nethealthChecker) updateStats(metrics []nethealthMetric) (updated []string, err error) {
	for _, metric := range metrics {
		series, err := c.getTimeoutSeries(metric.peerName)
		if err != nil {
			return updated, trace.Wrap(err)
		}

		// Keep only the last `seriesCapacity` number of data points.
		if len(series) >= c.SeriesCapacity {
			series = series[1:]
		}
		series = append(series, metric.totalTimeout)

		if err := c.setTimeoutSeries(metric.peerName, series); err != nil {
			return updated, trace.Wrap(err)
		}

		// Record updated nodes to be returned for use in later nethealth verification step.
		updated = append(updated, metric.peerName)
	}
	return updated, nil
}

// verifyNethealth verifies that the network communication is healthy for the
// nodes specified by the provided list of names.
func (c *nethealthChecker) verifyNethealth(names []string, reporter health.Reporter) error {
	for _, name := range names {
		series, err := c.getTimeoutSeries(name)
		if err != nil {
			return trace.Wrap(err)
		}

		if !c.isHealthy(series) {
			reporter.Add(NewProbeFromErr(c.Name(), nethealthDetail(name), nil))
		}
	}
	return nil
}

// isHealthy returns false if the number of timeouts increases at each data point.
func (c *nethealthChecker) isHealthy(series []int64) bool {
	// Checker has not collected enough data yet to check network health.
	if len(series) < c.SeriesCapacity {
		return true
	}

	// series contains time series data of the running total of timeouts for a peer.
	// If the counter is increasing at each data point, that means requests to
	// this peer have been timing consistently throughout this interval. This
	// should indicate that there is a network issue.
	for i := 0; i < c.SeriesCapacity-1; i++ {
		if series[i] >= series[i+1] {
			return true
		}
	}

	return false
}

// getTimeoutSeries returns the time series data mapped to the specified name.
// Returns an empty slice if name was not previously mapped.
func (c *nethealthChecker) getTimeoutSeries(name string) (series []int64, err error) {
	c.Lock()
	defer c.Unlock()
	if value, ok := c.timeoutStats.Get(name); ok {
		if series, ok = value.([]int64); !ok {
			return series, trace.BadParameter("couldn't parse time series as []int64; got type %T", value)
		}
	}
	return series, nil
}

// setTimeoutSeries maps the name to the timeout series.
func (c *nethealthChecker) setTimeoutSeries(name string, series []int64) error {
	c.Lock()
	defer c.Unlock()
	return c.timeoutStats.Set(name, series, timeoutStatsTTLSeconds)
}

// getNethealthAddr returns the address of the local nethealth pod.
func (c *nethealthChecker) getNethealthAddr() (string, error) {
	pods, err := c.Client.CoreV1().Pods(nethealthNamespace).List(metav1.ListOptions{})
	if err != nil {
		return "", trace.Wrap(err)
	}

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
// Metrics are returned as an array of bytes.
func fetchMetrics(ctx context.Context, addr string) ([]byte, error) {
	client, err := roundtrip.NewClient(addr, "")
	if err != nil {
		return nil, trace.Wrap(err, "failed to connect to nethealth service at %s.", addr)
	}

	resp, err := client.Get(ctx, client.Endpoint("metrics"), url.Values{})
	if err != nil {
		return nil, trace.ConvertSystemError(err)
	}

	return resp.Bytes(), nil
}

// parseMetrics parses input from the provided reader and returns the metrics
// for the 'nethealth_echo_timeout_total' counter.
func parseMetrics(input io.Reader) ([]nethealthMetric, error) {
	// requestTimeoutName defines the metric family name of the relevant nethealth counter
	const requestTimeoutName = "nethealth_echo_timeout_total"

	var parser expfmt.TextParser
	metricsFamilies, err := parser.TextToMetricFamilies(input)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	mf, ok := metricsFamilies[requestTimeoutName]
	if !ok {
		return nil, trace.NotFound("%s metrics not found", requestTimeoutName)
	}

	metrics := make([]nethealthMetric, 0, len(mf.GetMetric()))
	for _, m := range mf.GetMetric() {
		peerName, err := getPeerName(m.GetLabel())
		if err != nil {
			return nil, trace.Wrap(err)
		}

		metrics = append(metrics, nethealthMetric{
			peerName:     peerName,
			totalTimeout: int64(m.GetCounter().GetValue()),
		})
	}
	return metrics, nil
}

// getPeerName extracts the 'peer_name' value from the provided labels.
func getPeerName(labels []*dto.LabelPair) (peer string, err error) {
	for _, label := range labels {
		if peerLabel == label.GetName() {
			return label.GetValue(), nil
		}
	}
	return "", trace.NotFound("unable to find required peer label")
}

// nethealthDetail returns a failed probe detail message.
func nethealthDetail(name string) string {
	return fmt.Sprintf("overlay network communication failure with %s", name)
}

type nethealthMetric struct {
	// peerName is the node name of the peer.
	peerName string
	// totalTimeout is the running total of requests to the peer that have timed out.
	totalTimeout int64
}

const (
	nethealthCheckerID = "nethealth-checker"
	peerLabel          = "peer_name"
	nethealthNamespace = "monitoring"
	nethealthLabel     = "k8s-app"
	nethealthValue     = "nethealth"

	// defaultSeriesCapacity defines the default capacity of a time series
	// interval.
	defaultSeriesCapacity = 10

	// defaultNethealthPort defines the default nethealth port.
	defaultNethealthPort = 9801

	// timeoutStatsCapacity sets the number of TTLMaps that can be stored.
	// This will be the size of the cluster -1.
	timeoutStatsCapacity = 1000

	// timeoutStatsTTLSeconds defines the time to live in seconds for the
	// stored timeout stats. This ensures the checker does not hold on to
	// unsed information when a member leaves the cluster.
	timeoutStatsTTLSeconds = 5 * 60 // 5 minutes
)
