/*
Copyright 2016 Gravitational, Inc.

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
	"path/filepath"

	"github.com/gravitational/satellite/agent"
	"github.com/gravitational/satellite/agent/backend"
	"github.com/gravitational/satellite/agent/backend/influxdb"
	"github.com/gravitational/satellite/agent/backend/sqlite"
	"github.com/gravitational/satellite/agent/cache"
	"github.com/gravitational/satellite/agent/cache/multiplex"
	"github.com/gravitational/trace"
	"github.com/gravitational/version"

	log "github.com/Sirupsen/logrus"
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
		cagent               = app.Command("agent", "Start monitoring agent")
		cagentRPCAddrs       = ListFlag(cagent.Flag("rpc-addr", "Address to bind the RPC listener to.  Can be specified multiple times").Default("127.0.0.1:7575"))
		cagentKubeAddr       = cagent.Flag("kube-addr", "Address of the kubernetes API server").Default("http://127.0.0.1:8080").String()
		cagentKubeletAddr    = cagent.Flag("kubelet-addr", "Address of the kubelet").Default("http://127.0.0.1:10248").String()
		cagentDockerAddr     = cagent.Flag("docker-addr", "Address of the docker daemon endpoint").Default("/var/run/docker.sock").String()
		cagentEtcdAddr       = cagent.Flag("etcd-addr", "Address of the etcd endpoint").Default("http://127.0.0.1:2379").String()
		cagentNettestImage   = cagent.Flag("nettest-image", "Name of the image to use for networking test").Default("gcr.io/google_containers/nettest:1.6").String()
		cagentName           = cagent.Flag("name", "Agent name.  Must be the same as the name of the local serf node").OverrideDefaultFromEnvar(EnvAgentName).String()
		cagentSerfRPCAddr    = cagent.Flag("serf-rpc-addr", "RPC address of the local serf node").Default("127.0.0.1:7373").String()
		cagentInitialCluster = KeyValueListFlag(cagent.Flag("initial-cluster", "Initial cluster configuration as a comma-separated list of peers").OverrideDefaultFromEnvar(EnvInitialCluster))
		cagentStateDir       = cagent.Flag("state-dir", "Directory to store agent-specific state").OverrideDefaultFromEnvar(EnvStateDir).String()
		cagentTags           = KeyValueListFlag(cagent.Flag("tags", "Define a tags as comma-separated list of key:value pairs").OverrideDefaultFromEnvar(EnvTags))
		// InfluxDB backend configuration
		cagentInfluxDatabase = cagent.Flag("influxdb-database", "Database to connect to").OverrideDefaultFromEnvar(EnvInfluxDatabase).String()
		cagentInfluxUsername = cagent.Flag("influxdb-user", "Username to use for connection").OverrideDefaultFromEnvar(EnvInfluxUser).String()
		cagentInfluxPassword = cagent.Flag("influxdb-password", "Password to use for connection").OverrideDefaultFromEnvar(EnvInfluxPassword).String()
		cagentInfluxURL      = cagent.Flag("influxdb-url", "URL of the InfluxDB endpoint").OverrideDefaultFromEnvar(EnvInfluxURL).String()

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
			return trace.Errorf("agent name not set")
		}
		agentRole, ok := (*cagentTags)["role"]
		if !ok {
			return trace.Errorf("agent role not set")
		}
		path := filepath.Join(*cagentStateDir, monitoringDbFile)
		log.Infof("saving health history to %v", path)
		var cache cache.Cache
		cache, err = sqlite.New(path)
		if err != nil {
			err = trace.Wrap(err, "failed to create cache")
			break
		}
		var backends []backend.Backend
		if *cagentInfluxDatabase != "" {
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
			SerfRPCAddr: *cagentSerfRPCAddr,
			Tags:        *cagentTags,
			Cache:       multiplex.New(cache, backends...),
		}
		monitoringConfig := &config{
			Role:         agent.Role(agentRole),
			KubeAddr:     *cagentKubeAddr,
			KubeletAddr:  *cagentKubeletAddr,
			DockerAddr:   *cagentDockerAddr,
			EtcdAddr:     *cagentEtcdAddr,
			NettestImage: *cagentNettestImage,
		}
		err = runAgent(agentConfig, monitoringConfig, toAddrList(*cagentInitialCluster))
	case cversion.FullCommand():
		version.Print()
	}

	return trace.Wrap(err)
}

// monitoringDbFile names the file where agent persists health status history.
const monitoringDbFile = "monitoring.db"
