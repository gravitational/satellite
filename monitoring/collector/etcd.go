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
	"encoding/json"
	"net/http"
	"net/url"
	"sync"

	"github.com/gravitational/roundtrip"
	"github.com/gravitational/satellite/monitoring"
	"github.com/gravitational/trace"
	"github.com/prometheus/client_golang/prometheus"
)

// ETCDCollector collects ETCD stats from the given server and exports them using
// the prometheus metrics package.
type ETCDCollector struct {
	client *roundtrip.Client
	config monitoring.ETCDConfig
	mutex  sync.RWMutex

	isRunning typedDesc
	health    typedDesc
}

// NewETCDCollector returns an initialized ETCDCollector.
func NewETCDCollector(config *monitoring.ETCDConfig) (Collector, error) {
	transport, err := config.NewHTTPTransport()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	if len(config.Endpoints) == 0 {
		return nil, trace.BadParameter("no ETCD endpoints configured")
	}

	client, err := newClient(config.Endpoints[0], roundtrip.HTTPClient(&http.Client{
		Transport: transport,
		Timeout:   collectMetricsTimeout,
	}))

	return &ETCDCollector{
		client: client,
		config: *config,
		isRunning: typedDesc{prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "etcd", "up"),
			"Whether scraping ETCD metrics was successful.",
			nil, nil,
		), prometheus.GaugeValue},
		health: typedDesc{prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "etcd", "health"),
			"Health status of ETCD.",
			nil, nil,
		), prometheus.GaugeValue},
	}, nil
}

// Update implements prometheus.Collector.
func (e *ETCDCollector) Update(ch chan<- prometheus.Metric) error {
	e.mutex.Lock() // To protect metrics from concurrent collects.
	defer e.mutex.Unlock()

	sendStatus := func() {
		ch <- e.isRunning.mustNewConstMetric(1.0)
	}
	defer sendStatus()

	ch <- e.isRunning.mustNewConstMetric(0.0)

	health, err := e.healthStatus()
	if err != nil {
		return trace.Wrap(err)
	}
	if health {
		ch <- e.health.mustNewConstMetric(1.0)
	} else {
		ch <- e.health.mustNewConstMetric(0.0)
	}

	return nil
}

// healthStatus determines status of etcd member
func (e *ETCDCollector) healthStatus() (healthy bool, err error) {
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
		return false, trace.Wrap(err, "unable to parse JSON output: %s", resp.Bytes())
	}

	return (result.Health == "true" || nresult.Health == true), nil
}

func newClient(url string, opts ...roundtrip.ClientParam) (*roundtrip.Client, error) {
	clt, err := roundtrip.NewClient(url, "", opts...)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return clt, nil
}
