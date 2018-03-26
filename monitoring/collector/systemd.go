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

var listOfServices = map[string]int{
	"flanneld.service":                1,
	"serf.service":                    1,
	"planet-agent.service":            1,
	"dnsmasq.service":                 1,
	"registry.service":                1,
	"docker.service":                  1,
	"etcd.service":                    1,
	"kube-apiserver.service":          1,
	"kube-scheduler.service":          1,
	"kube-kubelet.service":            1,
	"kube-proxy.service":              1,
	"kube-controller-manager.service": 1,
}

type SystemdCollector struct {
	dbusConn         *dbus.Conn
	systemdState     typedDesc
	systemdUnitState typedDesc
}

func NewSystemdCollector() (*SystemdCollector, error) {
	conn, err := dbus.New()
	if err != nil {
		return nil, trace.Wrap(err, "failed to connect to dbus")
	}

	return &SystemdCollector{
		dbusConn: conn,
		systemdState: typedDesc{prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "systemd", "health"),
			"State of systemd service",
			nil, nil,
		), prometheus.GaugeValue},
		systemdUnitState: typedDesc{prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "systemd", "unit_health"),
			"State of systemd unit",
			[]string{"unit_name"}, nil,
		), prometheus.GaugeValue},
	}, nil
}

func (s *SystemdCollector) Collect(ch chan<- prometheus.Metric) error {
	systemdStatus, err := monitoring.IsSystemRunning()
	if err != nil {
		return trace.Wrap(err, "failed to query systemd status")
	}

	var systemdHealthMetric prometheus.Metric
	if systemdStatus == monitoring.SystemStatusRunning {
		if systemdHealthMetric, err = s.systemdState.newConstMetric(1.0); err != nil {
			return trace.Wrap(err, "failed to create prometheus metric: %v", err)
		}
	} else {
		if systemdHealthMetric, err = s.systemdState.newConstMetric(0.0); err != nil {
			return trace.Wrap(err, "failed to create prometheus metric: %v", err)
		}
	}

	ch <- systemdHealthMetric

	units, err := s.dbusConn.ListUnits()
	if err != nil {
		return trace.Wrap(err, "failed to query systemd units: %v", err)
	}

	var systemdUnitHealthMetric prometheus.Metric
	for _, unit := range units {
		if _, ok := listOfServices[unit.Name]; ok {
			if unit.ActiveState == string(activeStateActive) && unit.LoadState == string(loadStateLoaded) {
				if systemdUnitHealthMetric, err = s.systemdUnitState.newConstMetric(1.0, unit.Name); err != nil {
					return trace.Wrap(err, "failed to create prometheus metric: %v", err)
				}
			} else {
				if systemdUnitHealthMetric, err = s.systemdUnitState.newConstMetric(0.0, unit.Name); err != nil {
					return trace.Wrap(err, "failed to create prometheus metric: %v", err)
				}
			}
			ch <- systemdUnitHealthMetric
		}
	}

	return nil
}

type loadState string

const (
	loadStateLoaded   loadState = "loaded"
	loadStateError              = "error"
	loadStateMasked             = "masked"
	loadStateNotFound           = "not-found"
)

type activeState string

const (
	activeStateActive       activeState = "active"
	activeStateReloading                = "reloading"
	activeStateInactive                 = "inactive"
	activeStateFailed                   = "failed"
	activeStateActivating               = "activating"
	activeStateDeactivating             = "deactivating"
)
