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
	"fmt"
	"strings"

	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/gravitational/satellite/healthz/defaults"
)

// ParseCLIFlags updates config with supplied command line flags values
func ParseCLIFlags(cfg *Config) {
	kingpin.Flag("debug", "Turn on debug mode.").Envar("DEBUG").BoolVar(&cfg.Debug)
	kingpin.Flag("listen-addr", "Address to listen on.").Default("0.0.0.0:8080").Envar("HEALTH_LISTEN_ADDR").StringVar(&cfg.ListenAddr)
	kingpin.Flag("check-interval", "Interval between checks.").Default("1m").Envar("HEALTH_CHECK_INTERVAL").DurationVar(&cfg.CheckInterval)
	accessKeyDoc := fmt.Sprintf("Key to access health status. HTTP client must supply it as `%q` param.", defaults.AccessKeyParam)
	kingpin.Flag("access-key", accessKeyDoc).Required().Envar("HEALTH_ACCESS_KEY").StringVar(&cfg.AccessKey)
	kingpin.Flag("cert-file", "Path to TLS cert file.").Envar("HEALTH_CERT_FILE").StringVar(&cfg.CertFile)
	kingpin.Flag("key-file", "Path to TLS key file.").Envar("HEALTH_KEY_FILE").StringVar(&cfg.KeyFile)
	kingpin.Flag("ca-file", "Path to TLS CA file.").Envar("HEALTH_CA_FILE").StringVar(&cfg.CAFile)
	kingpin.Flag("kube-addr", "K8S apiserver address.").Default("http://localhost:8080").Envar("HEALTH_KUBE_ADDR").StringVar(&cfg.KubeAddr)
	kingpin.Flag("kube-cert-file", "K8S apiserver TLS cert file.").Envar("HEALTH_KUBE_CERT_FILE").StringVar(&cfg.KubeCertFile)
	kingpin.Flag("kube-nodes-threshold", "Minimal limit of K8S nodes must be ready to assume cluster is healthy").Required().Envar("HEALTH_KUBE_NODES_THRESHOLD").IntVar(&cfg.KubeNodesThreshold)
	etcdEndpoints := kingpin.Flag("etcd-addr", "Etcd machine address.").Default("http://localhost:4001,http://localhost:2380").Envar("ETCDCTL_PEERS").String()
	kingpin.Flag("etcd-cert-file", "Path to etcd TLS cert file.").Envar("ETCDCTL_CERT_FILE").StringVar(&cfg.ETCDConfig.CertFile)
	kingpin.Flag("etcd-key-file", "Path to etcd TLS key file.").Envar("ETCDCTL_KEY_FILE").StringVar(&cfg.ETCDConfig.KeyFile)
	kingpin.Flag("etcd-ca-file", "Path to etcd TLS CA file.").Envar("ETCDCTL_CA_FILE").StringVar(&cfg.ETCDConfig.CAFile)
	kingpin.Flag("etcd-skip-verify", "Skip etcd keys verification.").Envar("ETCDCTL_SKIP_VERIFY").BoolVar(&cfg.ETCDConfig.InsecureSkipVerify)
	kingpin.Parse()

	cfg.ETCDConfig.Endpoints = strings.Split(*etcdEndpoints, ",")
}
