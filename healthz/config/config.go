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

package config

import (
	"time"

	"github.com/gravitational/satellite/monitoring"
)

// Config stores application configuration
type Config struct {
	// Debug specifies whether to log in verbose mode
	Debug bool
	// ListenAddr specifies the listen address
	ListenAddr string
	// CheckInterval is how often to execute checks
	CheckInterval time.Duration
	// AccessKey to protect the healthz endpoint
	AccessKey string
	// KubeconfigPath specifies the absolute path to kubeconfig
	KubeconfigPath string
	// KubeCertFile is the path to a Kubernetes API server certificate file
	KubeCertFile string
	// KubeNodesThreshold defines the threshold when the node availability
	// test considers the cluster degraded
	KubeNodesThreshold int
	// ETCDConfig specifies the configuration for etcd
	ETCDConfig monitoring.ETCDConfig
	// CertFile is the path to a TLS certificate file
	CertFile string
	// KeyFile is the path to a TLS certificate key file
	KeyFile string
	// CAFile is the path to a TLS CA certificate file
	CAFile string
}
