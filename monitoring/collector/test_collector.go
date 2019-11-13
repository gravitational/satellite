package collector

import "github.com/prometheus/client_golang/prometheus"

// TestCollector TESTING METRICS COLLECTOR
type TestCollector struct {
	val bool
}

// NewTestCollector initializes and returns a new TestCollector
func NewTestCollector() *TestCollector {
	return &TestCollector{}
}

// Collect TESTING METRICS COLLECTOR
func (c *TestCollector) Collect(ch chan<- prometheus.Metric) error {
	metrics := typedDesc{prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "metrics", "test"),
		"Satellite metrics collection test",
		[]string{"label1", "label2", "label3"},
		nil,
	), prometheus.GaugeValue}

	if c.val {
		ch <- metrics.mustNewConstMetric(1.0, "test1", "test2", "test3")
	} else {
		ch <- metrics.mustNewConstMetric(0.0, "test1", "test2", "test3")
	}
	c.val = !c.val
	return nil
}
