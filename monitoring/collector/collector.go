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
	factories          = make(map[string]func() (Collector, error))
	collectorState     = make(map[string]*bool)
	subsystem          = "exporter"
	scrapeDurationDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, subsystem, "collector_duration_seconds"),
		"planet_exporter: Duration of a collector scrape.",
		[]string{"collector"},
		nil,
	)
	scrapeSuccessDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, subsystem, "collector_success"),
		"planet_exporter: Whether a collector succeeded.",
		[]string{"collector"},
		nil,
	)
)

// PlanetCollector implements the prometheus.Collector interface.
type PlanetCollector struct {
	configETCD monitoring.ETCDConfig
	Collectors map[string]Collector
}

// NewPlanetCollector creates a new PlanetCollector
func NewPlanetCollector(configETCD *monitoring.ETCDConfig) (*PlanetCollector, error) {
	collectors := make(map[string]Collector)
	collectorETCD, err := NewETCDCollector(configETCD)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	collectorSysctl := NewSysctlCollector()

	collectors["etcd"] = collectorETCD
	collectors["sysctl"] = collectorSysctl
	return &PlanetCollector{Collectors: collectors}, nil
}

// Describe implements the prometheus.Collector interface.
func (pc *PlanetCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- scrapeDurationDesc
	ch <- scrapeSuccessDesc
}

// Collect implements the prometheus.Collector interface.
func (pc *PlanetCollector) Collect(ch chan<- prometheus.Metric) {
	wg := sync.WaitGroup{}
	wg.Add(len(pc.Collectors))
	for name, c := range pc.Collectors {
		go func(name string, c Collector) {
			execute(name, c, ch)
			wg.Done()
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
		log.Errorf("%s collector failed after %fs: %s", name, duration.Seconds(), err)
		success = 0
	} else {
		log.Debugf("%s collector succeeded after %fs.", name, duration.Seconds())
		success = 1
	}
	m, err := prometheus.NewConstMetric(scrapeDurationDesc, prometheus.GaugeValue, duration.Seconds(), name)
	if err != nil {
		log.Errorf("failed to collect of collector_duration_seconds metric: %s", err)
	}
	ch <- m

	m, err = prometheus.NewConstMetric(scrapeSuccessDesc, prometheus.GaugeValue, success, name)
	if err != nil {
		log.Errorf("failed to collect of collector_success metric: %s", err)
	}
	ch <- m
}

// Collector is the interface a collector has to implement.
type Collector interface {
	// Get new metrics and expose them via prometheus registry.
	Collect(ch chan<- prometheus.Metric) error
}

type typedDesc struct {
	desc      *prometheus.Desc
	valueType prometheus.ValueType
}

func (d *typedDesc) newConstMetric(value float64, labels ...string) (prometheus.Metric, error) {
	return prometheus.NewConstMetric(d.desc, d.valueType, value, labels...)
}
