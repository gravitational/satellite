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

package collector

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/gravitational/roundtrip"
	"github.com/gravitational/satellite/monitoring"
	"github.com/gravitational/trace"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	isLeader = typedDesc{prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "etcd", "leader"),
		"Endpoint is the leader of etcd cluster.",
		nil, nil), prometheus.GaugeValue}

	isRunning = typedDesc{prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "etcd", "up"),
		"Whether scraping etcd metrics was successful.",
		nil, nil), prometheus.GaugeValue}

	health = typedDesc{prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "etcd", "health"),
		"Health status of etcd.",
		nil, nil), prometheus.GaugeValue}

	runningOn = isRunning.mustNewConstMetric(1.0)

	healthyOn  = health.mustNewConstMetric(1.0)
	healthyOff = health.mustNewConstMetric(0.0)

	leaderOn  = isLeader.mustNewConstMetric(1.0)
	leaderOff = isLeader.mustNewConstMetric(0.0)
)

// EtcdCollector collects etcd stats from the given server and exports them using
// the prometheus metrics package.
type EtcdCollector struct {
	client *roundtrip.Client
}

// NewEtcdCollector returns an initialized EtcdCollector.
func NewEtcdCollector(config *monitoring.ETCDConfig) (*EtcdCollector, error) {
	if len(config.Endpoints) == 0 {
		return nil, trace.BadParameter("no etcd endpoints configured")
	}

	transport, err := config.NewHTTPTransport()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	client, err := NewRoundtripClient(config.Endpoints[0], roundtrip.HTTPClient(&http.Client{
		Transport: transport,
		Timeout:   collectMetricsTimeout,
	}))
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return &EtcdCollector{
		client: client,
	}, nil
}

// Collect implements the prometheus.Collector interface
func (e *EtcdCollector) Collect(ch chan<- prometheus.Metric) error {
	resp, err := e.client.Get(e.client.Endpoint("v2", "stats", "leader"), url.Values{})
	if err != nil {
		return trace.Wrap(err)
	}

	if isFollowerResp(resp) {
		ch <- leaderOff
		ch <- runningOn
		return nil
	}

	ch <- leaderOn
	healthy, err := e.healthStatus()
	if err != nil {
		return trace.Wrap(err, "failed to get health status of etcd cluster")
	}
	if healthy {
		ch <- healthyOn
	} else {
		ch <- healthyOff
	}

	ch <- runningOn
	return nil
}

// healthStatus determines status of etcd member
func (e *EtcdCollector) healthStatus() (healthy bool, err error) {
	result := struct{ Health string }{}
	nresult := struct{ Health bool }{}
	resp, err := e.client.Get(e.client.Endpoint("health"), url.Values{})
	if err != nil {
		return false, trace.Wrap(err)
	}

	err = json.Unmarshal(resp.Bytes(), &result)
	if err != nil {
		err = json.Unmarshal(resp.Bytes(), &nresult)
	}
	if err != nil {
		return false, trace.Wrap(err, "unable to parse JSON output: %s", string(resp.Bytes()))
	}

	return (result.Health == "true" || nresult.Health == true), nil
}

// isFollowerRes determines follower type of etcd member
func isFollowerResp(resp *roundtrip.Response) bool {
	return bytes.Contains(resp.Bytes(), []byte("not current leader"))
}
