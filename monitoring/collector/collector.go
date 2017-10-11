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
	"sync"
	"time"

	"github.com/gravitational/satellite/monitoring"
	"github.com/gravitational/trace"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

const (
	namespace             = "planet"
	collectMetricsTimeout = 5 * time.Second
)

var (
	subsystem          = "exporter"
	scrapeDurationDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, subsystem, "collector_duration_seconds"),
		"Duration of a collector scrape.",
		[]string{"collector"},
		nil,
	)
	scrapeSuccessDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, subsystem, "collector_success"),
		"Whether a collector succeeded.",
		[]string{"collector"},
		nil,
	)
)

// PlanetCollector implements the prometheus.Collector interface.
type PlanetCollector struct {
	configEtcd monitoring.ETCDConfig
	collectors map[string]Collector
}

// NewPlanetCollector creates a new PlanetCollector
func NewPlanetCollector(configEtcd *monitoring.ETCDConfig, kubeAddr string) (*PlanetCollector, error) {
	collectorEtcd, err := NewEtcdCollector(configEtcd)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	collectorDocker, err := NewDockerCollector()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	collectors := make(map[string]Collector)
	collectors["etcd"] = collectorEtcd
	collectors["sysctl"] = NewSysctlCollector()
	collectors["docker"] = collectorDocker
	return &PlanetCollector{collectors: collectors}, nil
}

// Describe implements the prometheus.Collector interface.
func (pc *PlanetCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- scrapeDurationDesc
	ch <- scrapeSuccessDesc
}

// Collect implements the prometheus.Collector interface.
func (pc *PlanetCollector) Collect(ch chan<- prometheus.Metric) {
	wg := sync.WaitGroup{}
	wg.Add(len(pc.collectors))
	for name, c := range pc.collectors {
		go func(name string, c Collector) {
			defer wg.Done()
			execute(name, c, ch)
		}(name, c)
	}
	wg.Wait()
}

func execute(name string, c Collector, ch chan<- prometheus.Metric) {
	begin := time.Now()
	err := c.Collect(ch)
	duration := time.Since(begin)
	var success float64

	if err != nil {
		log.Warnf("%s collector failed after %v: %s", name, duration, err)
		success = 0
	} else {
		log.Debugf("%s collector succeeded after %v.", name, duration)
		success = 1
	}
	metric, err := prometheus.NewConstMetric(scrapeDurationDesc, prometheus.GaugeValue, duration.Seconds(), name)
	if err != nil {
		log.Warnf("failed to create metric for duration of scrape: %s", err)
	} else {
		ch <- metric
	}

	metric, err = prometheus.NewConstMetric(scrapeSuccessDesc, prometheus.GaugeValue, success, name)
	if err != nil {
		log.Warnf("failed to create metric for status of scrape: %s", err)
	} else {
		ch <- metric
	}
}

type Collector interface {
	// Collect collects metrics and exposes them to the prometheus registry
	// on the specified channel. Returns an error if collection fails
	Collect(ch chan<- prometheus.Metric) error
}

type typedDesc struct {
	desc      *prometheus.Desc
	valueType prometheus.ValueType
}

func (d *typedDesc) newConstMetric(value float64, labels ...string) (prometheus.Metric, error) {
	return prometheus.NewConstMetric(d.desc, d.valueType, value, labels...)
}

func (d *typedDesc) mustNewConstMetric(value float64, labels ...string) prometheus.Metric {
	return prometheus.MustNewConstMetric(d.desc, d.valueType, value, labels...)
}
