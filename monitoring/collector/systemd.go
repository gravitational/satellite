/*
Copyright (C) 2018 Gravitational, Inc.

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
	"github.com/gravitational/satellite/monitoring"

	"github.com/coreos/go-systemd/dbus"
	"github.com/gravitational/trace"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	listOfServices = serviceMap(
		"flanneld.service",
		"serf.service",
		"planet-agent.service",
		"dnsmasq.service",
		"registry.service",
		"docker.service",
		"etcd.service",
		"kube-apiserver.service",
		"kube-scheduler.service",
		"kube-kubelet.service",
		"kube-proxy.service",
		"kube-controller-manager.service",
	)

	systemdState = typedDesc{prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "systemd", "health"),
		"State of systemd service",
		nil, nil,
	), prometheus.GaugeValue}

	systemdHealthy   = systemdState.mustNewConstMetric(1.0)
	systemdUnhealthy = systemdState.mustNewConstMetric(0.0)
)

// SystemdCollector collects metrics about systemd health
type SystemdCollector struct {
	dbusConn         *dbus.Conn
	systemdUnitState typedDesc
}

// NewSystemdCollector returns initialized SystemdCollector
func NewSystemdCollector() (*SystemdCollector, error) {
	conn, err := dbus.New()
	if err != nil {
		return nil, trace.Wrap(err, "failed to connect to dbus")
	}

	return &SystemdCollector{
		dbusConn: conn,
		systemdUnitState: typedDesc{prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "systemd", "unit_health"),
			"State of systemd unit",
			[]string{"unit_name"}, nil,
		), prometheus.GaugeValue},
	}, nil
}

// Collect is called by the Prometheus registry when collecting metrics.
func (s *SystemdCollector) Collect(ch chan<- prometheus.Metric) error {
	systemdStatus, err := monitoring.IsSystemRunning()
	if err != nil {
		return trace.Wrap(err, "failed to query systemd status")
	}

	if systemdStatus == monitoring.SystemStatusRunning {
		ch <- systemdHealthy
	} else {
		ch <- systemdUnhealthy
	}

	units, err := s.dbusConn.ListUnits()
	if err != nil {
		return trace.Wrap(err, "failed to query systemd units")
	}

	for _, unit := range units {
		if _, ok := listOfServices[unit.Name]; ok {
			var systemdUnitHealthMetric prometheus.Metric

			if unit.ActiveState == string(activeStateActive) && unit.LoadState == string(loadStateLoaded) {
				if systemdUnitHealthMetric, err = s.systemdUnitState.newConstMetric(1.0, unit.Name); err != nil {
					return trace.Wrap(err, "failed to create prometheus metric")
				}
			} else {
				if systemdUnitHealthMetric, err = s.systemdUnitState.newConstMetric(0.0, unit.Name); err != nil {
					return trace.Wrap(err, "failed to create prometheus metric")
				}
			}
			ch <- systemdUnitHealthMetric
		}
	}

	return nil
}

func serviceMap(services ...string) map[string]struct{} {
	result := make(map[string]struct{})
	for _, service := range services {
		result[service] = struct{}{}
	}
	return result
}

type loadState string

const (
	loadStateLoaded   loadState = "loaded"
	loadStateError    loadState = "error"
	loadStateMasked   loadState = "masked"
	loadStateNotFound loadState = "not-found"
)

type activeState string

const (
	activeStateActive       activeState = "active"
	activeStateReloading    activeState = "reloading"
	activeStateInactive     activeState = "inactive"
	activeStateFailed       activeState = "failed"
	activeStateActivating   activeState = "activating"
	activeStateDeactivating activeState = "deactivating"
)
