# Satellite

Satellite is an agent written in Go for collecting health information in a [kubernetes] cluster.
It is both a library and an application. As a library, it can be used as a basis for a custom monitoring solution.
The health status information is collected in the form of a time series and persisted to the [sqlite] backend.
Additional backends are supported via an interface.

The original design goals are:

 - lightweight periodic testing
 - high availability and resilience to network partitions
 - no single point of failure
 - history of health data as a time series

The agents communicate over a [Gossip] protocol implemented by [serf].

## Dependencies
 - [serf], v0.7.0 or later
 - [godep]

## Installation

There are currently no binary distributions available.

### Building from source

Satellite manages dependencies via [godep] which is also a prerequisite for building.
Another prerequisite is Go version 1.7+.

 1. Install [Go]
 1. Install [godep]
 1. [Setup GOPATH]
 1. Run `go get github.com/gravitational/satellite`
 1. Run `cd $GOPATH/src/github.com/gravitational/satellite`
 1. Run `make`


## How to use it

```console
$ satellite help
usage: satellite [<flags>] <command> [<args> ...]

Cluster health monitoring agent

Flags:
  --help   Show help (also see --help-long and --help-man).
  --debug  Enable verbose mode

Commands:
  help [<command>...]
    Show help.

  agent [<flags>]
    Start monitoring agent

  status [<flags>]
    Query cluster status

  version
    Display version


$ satellite agent help
usage: satellite agent [<flags>]

Start monitoring agent

Flags:
  --help                         Show context-sensitive help (also try --help-long and --help-man).
  --debug                        Enable verbose mode
  --rpc-addr=127.0.0.1:7575      List of addresses to bind the RPC listener to (host:port), comma-separated
  --kube-addr="http://127.0.0.1:8080"  
                                 Address of the kubernetes API server
  --kubelet-addr="http://127.0.0.1:10248"  
                                 Address of the kubelet
  --docker-addr="/var/run/docker.sock"  
                                 Path to the docker daemon socket
  --nettest-image="gcr.io/google_containers/nettest:1.8"  
                                 Name of the image to use for networking test
  --name=NAME                    Agent name. Must be the same as the name of the local serf node
  --serf-rpc-addr="127.0.0.1:7373"  
                                 RPC address of the local serf node
  --initial-cluster=INITIAL-CLUSTER  
                                 Initial cluster configuration as a comma-separated list of peers
  --state-dir=STATE-DIR          Directory to store agent-specific state
  --tags=TAGS                    Define a tags as comma-separated list of key:value pairs
  --etcd-servers=http://127.0.0.1:2379  
                                 List of etcd endpoints (http://host:port), comma separated
  --etcd-cafile=ETCD-CAFILE      SSL Certificate Authority file used to secure etcd communication
  --etcd-certfile=ETCD-CERTFILE  SSL certificate file used to secure etcd communication
  --etcd-keyfile=ETCD-KEYFILE    SSL key file used to secure etcd communication
  --influxdb-database=INFLUXDB-DATABASE  
                                 Database to connect to
  --influxdb-user=INFLUXDB-USER  Username to use for connection
  --influxdb-password=INFLUXDB-PASSWORD  
                                 Password to use for connection
  --influxdb-url=INFLUXDB-URL    URL of the InfluxDB endpoint

$ satellite agent --name=my-host --tags=role:master

$ satellite help status
usage: satellite status [<flags>]

Query cluster status

Flags:
  --help           Show context-sensitive help (also try --help-long and --help-man).
  --debug          Enable verbose mode
  --rpc-port=7575  Local agent RPC port
  --pretty         Pretty-print the output
  --local          Query the status of the local node
```

You can then query the status of the cluster or that of the local node by issuing a status query:

```console
$ satellite status --pretty
```

resulting in:

```json
{
   "status": "degraded",
   "nodes": [
      {
         "name": "example.domain",
         "member_status": {
            "name": "example.domain",
            "addr": "192.168.178.32:7946",
            "status": "alive",
            "tags": {
               "role": "node"
            }
         },
         "status": "degraded",
         "probes": [
            {
               "checker": "docker",
               "status": "running"
            },
            ...
         ]
      }
   ],
   "timestamp": "2016-03-03T12:19:44.757110373Z",
   "summary": "master node unavailable"
}
```


Out of the box, the agent requires at least a single master node (agent with `role:master`). The test will mark the cluster as `degraded` if no master is available.

Connect the agent to InfluxDB database `monitoring`:

```console
$ satellite agent --tags=role:master \
	--state-dir=/var/run/satellite \
	--influxdb-database=monitoring \
	--influxdb-url=http://localhost:8086
```


[//]: # (Footnots and references)

[Kubernetes]: <https://github.com/kubernetes/kubernetes>
[serf]: <https://www.serfdom.io/downloads.html>
[Go]: <https://golang.org/doc/install>
[godep]: <https://github.com/tools/godep>
[Setup GOPATH]: <https://golang.org/doc/code.html#GOPATH>
[sqlite]: <https://www.sqlite.org/>
