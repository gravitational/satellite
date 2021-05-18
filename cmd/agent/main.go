/*
Copyright 2016-2020 Gravitational, Inc.

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

package main

import (
	"os"

	"github.com/gravitational/satellite/agent"
	"github.com/gravitational/satellite/agent/backend"
	"github.com/gravitational/satellite/agent/backend/influxdb"
	"github.com/gravitational/satellite/agent/backend/inmemory"
	"github.com/gravitational/satellite/agent/cache/multiplex"
	"github.com/gravitational/satellite/cmd"
	"github.com/gravitational/satellite/lib/history/sqlite"
	"github.com/gravitational/satellite/monitoring"
	"github.com/gravitational/trace"
	"github.com/gravitational/version"

	log "github.com/sirupsen/logrus"
	"gopkg.in/alecthomas/kingpin.v2"
)

func init() {
	version.Init("v0.0.1-master+$Format:%h$")
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var (
		app   = kingpin.New("satellite", "Cluster health monitoring agent")
		debug = app.Flag("debug", "Enable verbose mode").Bool()

		// `agent` command
		cagent                      = app.Command("agent", "Start monitoring agent")
		cagentRPCAddrs              = ListFlag(cagent.Flag("rpc-addr", "List of addresses to bind the RPC listener to (host:port), comma-separated").Default("127.0.0.1:7575"))
		cagentKubeconfig            = cagent.Flag("kubeconfig", "Absolute path to the kubeconfig file").OverrideDefaultFromEnvar(cmd.EnvKubeconfigFile).Required().String()
		cagentKubeletAddr           = cagent.Flag("kubelet-addr", "Address of the kubelet").Default("http://127.0.0.1:10248").String()
		cagentDockerAddr            = cagent.Flag("docker-addr", "Path to the docker daemon socket").Default("/var/run/docker.sock").String()
		cagentNettestContainerImage = cagent.Flag("nettest-image", "Name of the image to use for networking test").Default("gcr.io/google_containers/nettest:1.8").String()
		cagentName                  = cagent.Flag("name", "Agent name.  Must be the same as the name of the local node").OverrideDefaultFromEnvar(EnvAgentName).String()
		cagentMetricsAddr           = cagent.Flag("metrics-addr", "Address to listen on for web interface and telemetry for Prometheus metrics").Default("127.0.0.1:7580").String()
		cagentInitialCluster        = KeyValueListFlag(cagent.Flag("initial-cluster", "Initial cluster configuration as a comma-separated list of peers").OverrideDefaultFromEnvar(EnvInitialCluster))
		cagentTags                  = KeyValueListFlag(cagent.Flag("tags", "Define a tags as comma-separated list of key:value pairs").OverrideDefaultFromEnvar(EnvTags))
		disableInterPodCheck        = cagent.Flag("disable-interpod-check", "Disable inter-pod check for single node cluster").Bool()
		// etcd configuration
		cagentEtcdServers  = ListFlag(cagent.Flag("etcd-servers", "List of etcd endpoints (http://host:port), comma separated").Default("http://127.0.0.1:2379"))
		cagentEtcdCAFile   = cagent.Flag("etcd-cafile", "SSL Certificate Authority file used to secure etcd communication").String()
		cagentEtcdCertFile = cagent.Flag("etcd-certfile", "SSL certificate file used to secure etcd communication").String()
		cagentEtcdKeyFile  = cagent.Flag("etcd-keyfile", "SSL key file used to secure etcd communication").String()
		// InfluxDB backend configuration
		cagentInfluxDatabase = cagent.Flag("influxdb-database", "Database to connect to").OverrideDefaultFromEnvar(EnvInfluxDatabase).String()
		cagentInfluxUsername = cagent.Flag("influxdb-user", "Username to use for connection").OverrideDefaultFromEnvar(EnvInfluxUser).String()
		cagentInfluxPassword = cagent.Flag("influxdb-password", "Password to use for connection").OverrideDefaultFromEnvar(EnvInfluxPassword).String()
		cagentInfluxURL      = cagent.Flag("influxdb-url", "URL of the InfluxDB endpoint").OverrideDefaultFromEnvar(EnvInfluxURL).String()
		cagentCAFile         = cagent.Flag("ca-file", "SSL CA certificate for verifying server certificates").ExistingFile()
		cagentCertFile       = cagent.Flag("cert-file", "SSL certificate for server RPC").ExistingFile()
		cagentKeyFile        = cagent.Flag("key-file", "SSL certificate key for server RPC").ExistingFile()
		// sqlite config
		cagentTimelineDir = cagent.Flag("timeline", "Directory to be used for timeline storage").Default("/tmp/timeline").String()
		cagentRetention   = cagent.Flag("retention", "Window to retain timeline as a Go duration").Duration()

		// `status` command
		cstatus            = app.Command("status", "Query cluster status")
		cstatusRPCPort     = cstatus.Flag("rpc-port", "Local agent RPC port").Default("7575").Int()
		cstatusPrettyPrint = cstatus.Flag("pretty", "Pretty-print the output").Bool()
		cstatusLocal       = cstatus.Flag("local", "Query the status of the local node").Bool()
		cstatusCAFile      = cstatus.Flag("ca-file", "CA certificate for verifying server certificates").ExistingFile()
		cstatusCertFile    = cstatus.Flag("client-cert-file", "mTLS client certificate file").ExistingFile()
		cstatusKeyFile     = cstatus.Flag("client-key-file", "mTLS client key file").ExistingFile()

		// `history` command
		chistory         = app.Command("history", "Query cluster status history")
		chistoryRPCPort  = chistory.Flag("rpc-port", "Local agent RPC port").Default("7575").Int()
		chistoryCAFile   = chistory.Flag("ca-file", "CA certificate for verifying server certificates").ExistingFile()
		chistoryCertFile = chistory.Flag("client-cert-file", "mTLS client certificate file").ExistingFile()
		chistoryKeyFile  = chistory.Flag("client-key-file", "mTLS client key file").ExistingFile()

		// checks command
		cchecks = app.Command("checks", "Run local compatibility checks")

		// `version` command
		cversion = app.Command("version", "Display version")
	)

	var cmd string
	var err error

	cmd, err = app.Parse(os.Args[1:])
	if err != nil {
		return trace.Errorf("unable to parse command line.\nUse agent --help for help.")
	}

	log.SetOutput(os.Stderr)
	if *debug == true {
		log.SetLevel(log.InfoLevel)
	} else {
		log.SetLevel(log.WarnLevel)
	}

	switch cmd {
	case cagent.FullCommand():
		if *cagentName == "" {
			*cagentName, err = os.Hostname()
			if err != nil {
				return trace.Wrap(err, "agent name not set, failed to set it to hostname")
			}
			log.Infof("using hostname `%v` as agent name", *cagentName)
		}
		agentRole, ok := (*cagentTags)["role"]
		if !ok {
			return trace.Errorf("agent role not set")
		}
		cache := inmemory.New()
		var backends []backend.Backend
		if *cagentInfluxDatabase != "" {
			log.Infof("connecting to influxdb database `%v` on %v", *cagentInfluxDatabase,
				*cagentInfluxURL)
			influxdb, err := influxdb.New(&influxdb.Config{
				Database: *cagentInfluxDatabase,
				Username: *cagentInfluxUsername,
				Password: *cagentInfluxPassword,
				URL:      *cagentInfluxURL,
			})
			if err != nil {
				return trace.Wrap(err, "failed to create influxdb backend")
			}
			backends = append(backends, influxdb)
		}

		agentConfig := &agent.Config{
			Name:        *cagentName,
			RPCAddrs:    *cagentRPCAddrs,
			MetricsAddr: *cagentMetricsAddr,
			Tags:        *cagentTags,
			Cache:       multiplex.New(cache, backends...),
			CAFile:      *cagentCAFile,
			CertFile:    *cagentCertFile,
			KeyFile:     *cagentKeyFile,
			TimelineConfig: sqlite.Config{
				DBPath:            *cagentTimelineDir,
				RetentionDuration: *cagentRetention,
			},
		}
		monitoringConfig := &config{
			role:                 agent.Role(agentRole),
			kubeconfigPath:       *cagentKubeconfig,
			kubeletAddr:          *cagentKubeletAddr,
			dockerAddr:           *cagentDockerAddr,
			disableInterPodCheck: *disableInterPodCheck,
			etcd: &monitoring.ETCDConfig{
				Endpoints: *cagentEtcdServers,
				CAFile:    *cagentEtcdCAFile,
				CertFile:  *cagentEtcdCertFile,
				KeyFile:   *cagentEtcdKeyFile,
			},
			nettestContainerImage: *cagentNettestContainerImage,
		}
		err = runAgent(agentConfig, monitoringConfig, toAddrList(*cagentInitialCluster))
	case cstatus.FullCommand():
		config := statusConfig{
			rpcConfig: rpcConfig{
				rpcPort:  *cstatusRPCPort,
				caFile:   *cstatusCAFile,
				certFile: *cstatusCertFile,
				keyFile:  *cstatusKeyFile,
			},
			local:       *cstatusLocal,
			prettyPrint: *cstatusPrettyPrint,
		}
		_, err = status(config)
	case chistory.FullCommand():
		config := rpcConfig{
			rpcPort:  *chistoryRPCPort,
			caFile:   *chistoryCAFile,
			certFile: *chistoryCertFile,
			keyFile:  *chistoryKeyFile,
		}
		_, err = history(config)
	case cchecks.FullCommand():
		err = localChecks()
	case cversion.FullCommand():
		version.Print()
		err = nil
	}

	return trace.Wrap(err)
}
