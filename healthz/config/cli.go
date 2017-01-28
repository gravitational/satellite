package config

import (
	"fmt"

	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/gravitational/satellite/healthz/defaults"
)

// ParseCLIFlags updates config with supplied command line flags values
func ParseCLIFlags(cfg *Config) {
	kingpin.Flag("debug", "Turn on debug mode").Envar("DEBUG").BoolVar(&cfg.Debug)
	kingpin.Flag("listen-addr", "Address to listen on.").Default("0.0.0.0:8080").Envar("HEALTH_LISTEN_ADDR").StringVar(&cfg.ListenAddr)
	kingpin.Flag("check-interval", "Interval between checks.").Default("1m").Envar("HEALTH_CHECK_INTERVAL").DurationVar(&cfg.CheckInterval)
	accessKeyDoc := fmt.Sprintf("Key to access health status. HTTP client must supply it as `%s` param.", defaults.AccessKeyParam)
	kingpin.Flag("access-key", accessKeyDoc).Required().Envar("HEALTH_ACCESS_KEY").StringVar(&cfg.AccessKey)
	kingpin.Flag("cert-file", "Path to TLS cert file.").Envar("HEALTH_CERT_FILE").StringVar(&cfg.CertFile)
	kingpin.Flag("key-file", "Path to TLS key file.").Envar("HEALTH_KEY_FILE").StringVar(&cfg.KeyFile)
	kingpin.Flag("ca-file", "Path to TLS CA file.").Envar("HEALTH_CA_FILE").StringVar(&cfg.CAFile)
	kingpin.Flag("kube-addr", "K8S apiserver address.").Default("http://localhost:8080").Envar("HEALTH_KUBE_ADDR").StringVar(&cfg.KubeAddr)
	etcdEndpoint := kingpin.Flag("etcd-endpoint", "Etcd machine address.").Default("http://localhost:4001").Envar("ETCDCTL_PEERS").String()
	kingpin.Flag("etcd-cert-file", "Path to etcd TLS cert file.").Envar("ETCDCTL_CERT_FILE").StringVar(&cfg.ETCDConfig.CertFile)
	kingpin.Flag("etcd-key-file", "Path to etcd TLS key file.").Envar("ETCDCTL_KEY_FILE").StringVar(&cfg.ETCDConfig.KeyFile)
	kingpin.Flag("etcd-ca-file", "Path to etcd TLS CA file.").Envar("ETCDCTL_CA_FILE").StringVar(&cfg.ETCDConfig.CAFile)
	kingpin.Parse()

	cfg.ETCDConfig.Endpoints = []string{*etcdEndpoint}
}