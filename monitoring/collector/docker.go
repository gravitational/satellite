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
	"net"
	"net/http"
	"net/url"

	"github.com/gravitational/roundtrip"
	"github.com/gravitational/trace"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	dockerSocketPath = "/var/run/docker.sock"
	dockerURL        = "http://docker/"
)

// DockerCollector collect metrics about docker service status
type DockerCollector struct {
	client   *roundtrip.Client
	dockerUp typedDesc
}

// NewDockerCollector returns initialized DockerCollector
func NewDockerCollector() (*DockerCollector, error) {
	transport := &http.Transport{
		Dial: func(network, addr string) (net.Conn, error) {
			return net.Dial("unix", dockerSocketPath)
		},
	}
	client, err := NewRoundtripClient(dockerURL, roundtrip.HTTPClient(&http.Client{
		Transport: transport,
		Timeout:   collectMetricsTimeout,
	}))
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return &DockerCollector{
		client: client,
		dockerUp: typedDesc{prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "docker", "health"),
			"Status of Docker daemon",
			nil, nil,
		), prometheus.GaugeValue},
	}, nil
}

// Collect implements the prometheus.Collector interface
func (d *DockerCollector) Collect(ch chan<- prometheus.Metric) error {
	healthy, err := d.healtStatus()
	if err != nil {
		return trace.Wrap(err)
	}
	if healthy {
		ch <- d.dockerUp.mustNewConstMetric(1.0)
		return nil
	}

	ch <- d.dockerUp.mustNewConstMetric(0.0)
	return nil
}

// healthStatus determines status of docker service
// by checking url http://docker/version from daemon socket
func (d *DockerCollector) healtStatus() (bool, error) {
	resp, err := d.client.Get(d.client.Endpoint("version"), url.Values{})
	if err != nil {
		return false, trace.Wrap(err, "HTTP request failed: %v", err)
	}

	if resp.Code() == 200 {
		return true, nil
	}

	return false, nil
}
