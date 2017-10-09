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
	"io/ioutil"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/gravitational/trace"
	"github.com/prometheus/client_golang/prometheus"
)

// SysctlCollector collects metrics from kernel system params
type SysctlCollector struct {
	ipv4Forwarding typedDesc
	brNetfilter    typedDesc
	mutex          sync.RWMutex
}

// NewSysctlCollector returns an initialized SysctlCollector.
func NewSysctlCollector() Collector {
	return &SysctlCollector{
		ipv4Forwarding: typedDesc{prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "sysctl", "ipv4_forwarding"),
			"Status of IPv4 forwarding kernel param",
			nil, nil,
		), prometheus.GaugeValue},
		brNetfilter: typedDesc{prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "sysctl", "br_netfilter"),
			"Status of bridge netfilter kernel param",
			nil, nil,
		), prometheus.GaugeValue},
	}
}

// Collect implements prometheus.Collector.
func (s *SysctlCollector) Collect(ch chan<- prometheus.Metric) error {
	s.mutex.Lock() // To protect metrics from concurrent collects.
	defer s.mutex.Unlock()

	var (
		metric float64
		m      prometheus.Metric
	)

	param, err := sysctl("net.ipv4.ip_forward")
	if err != nil {
		return trace.Wrap(err)
	}
	if metric, err = strconv.ParseFloat(param, 64); err != nil {
		return trace.Wrap(err)
	}
	if m, err = s.ipv4Forwarding.newConstMetric(metric); err != nil {
		return trace.Wrap(err)
	}
	ch <- m

	param, err = sysctl("net.bridge.bridge-nf-call-iptables")
	if err != nil {
		return trace.Wrap(err)
	}
	if metric, err = strconv.ParseFloat(param, 64); err != nil {
		return trace.Wrap(err)
	}
	if m, err = s.brNetfilter.newConstMetric(metric); err != nil {
		return trace.Wrap(err)
	}
	ch <- m

	return nil
}

// sysctl returns kernel parameter by reading /proc/sys directory
func sysctl(name string) (string, error) {
	path := filepath.Clean(filepath.Join("/proc", "sys", strings.Replace(name, ".", "/", -1)))
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return "", trace.ConvertSystemError(err)
	}
	if len(data) == 0 {
		return "", trace.BadParameter("empty output from sysctl")
	}
	return string(data[:len(data)-1]), nil
}
