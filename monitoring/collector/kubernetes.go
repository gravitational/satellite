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
	"github.com/gravitational/satellite/monitoring"
	"github.com/gravitational/trace"
	"github.com/prometheus/client_golang/prometheus"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	kube "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
)

// KubernetesCollector collects metrics about Kubernetes state
type KubernetesCollector struct {
	client      *kube.Clientset
	nodeIsReady typedDesc
}

// NewKubernetesCollector returns initialized KubernetesCollector
func NewKubernetesCollector(kubeAddr string) (*KubernetesCollector, error) {
	client, err := monitoring.ConnectToKube(kubeAddr, schedulerConfigPath)
	if err != nil {
		return nil, trace.Wrap(err, "failed to connect to kubernetes apiserver: %s", kubeAddr)
	}

	return &KubernetesCollector{
		client: client,
		nodeIsReady: typedDesc{prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "k8s", "node_ready"),
			"Status of Kubernetes node",
			[]string{"node"}, nil,
		), prometheus.GaugeValue},
	}, nil
}

// Collect implements the prometheus.Collector interface
func (k *KubernetesCollector) Collect(ch chan<- prometheus.Metric) error {
	var metric prometheus.Metric

	listOptions := metav1.ListOptions{
		LabelSelector: labels.Everything().String(),
		FieldSelector: fields.Everything().String(),
	}
	nodes, err := k.client.Nodes().List(listOptions)
	if err != nil {
		return trace.Wrap(err, "failed to query nodes: %v", err)
	}

	for _, item := range nodes.Items {
		for _, condition := range item.Status.Conditions {
			if condition.Type != v1.NodeReady {
				continue
			}
			if condition.Status == v1.ConditionTrue {
				if metric, err = k.nodeIsReady.newConstMetric(1.0, item.Name); err != nil {
					return trace.Wrap(err, "failed to create prometheus metric: %v", err)
				}
			} else {
				if metric, err = k.nodeIsReady.newConstMetric(0.0, item.Name); err != nil {
					return trace.Wrap(err, "failed to create prometheus metric: %v", err)
				}
			}
			ch <- metric
		}
	}

	return nil
}
