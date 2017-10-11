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
	"strconv"

	"github.com/gravitational/satellite/monitoring"
	"github.com/gravitational/trace"
	"github.com/prometheus/client_golang/prometheus"
)

// Name of sysctl parameters to scrape
const (
	IPv4Forwarding  = "net.ipv4.ip_forward"
	BridgeNetfilter = "net.bridge.bridge-nf-call-iptables"
)

// SysctlCollector converts kernel system parameters to prometheus metrics
type SysctlCollector struct {
	ipv4Forwarding typedDesc
	brNetfilter    typedDesc
}

// NewSysctlCollector returns an initialized SysctlCollector.
func NewSysctlCollector() *SysctlCollector {
	return &SysctlCollector{
		ipv4Forwarding: typedDesc{prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "sysctl", "ipv4_forwarding"),
			"Value of IPv4 forwarding kernel parameter",
			nil, nil,
		), prometheus.GaugeValue},
		brNetfilter: typedDesc{prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "sysctl", "br_netfilter"),
			"Value of bridge netfilter module parameter",
			nil, nil,
		), prometheus.GaugeValue},
	}
}

// Collect implements prometheus.Collector.
func (s *SysctlCollector) Collect(ch chan<- prometheus.Metric) error {
	metric, err := sysctlMetric(IPv4Forwarding, s.ipv4Forwarding)
	if err != nil {
		return trace.Wrap(err)
	}
	ch <- metric

	metric, err = sysctlMetric(BridgeNetfilter, s.brNetfilter)
	if err != nil {
		return trace.Wrap(err)
	}
	ch <- metric

	return nil
}

func sysctlMetric(paramName string, desc typedDesc) (prometheus.Metric, error) {
	var (
		parsedMetric float64
		metric       prometheus.Metric
	)

	param, err := monitoring.Sysctl(paramName)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	if parsedMetric, err = strconv.ParseFloat(param, 64); err != nil {
		return nil, trace.Wrap(err)
	}
	if metric, err = desc.newConstMetric(parsedMetric); err != nil {
		return nil, trace.Wrap(err)
	}
	return metric, nil
}
