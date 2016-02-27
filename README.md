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
Another prerequisite is Go version 1.5+.

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

  version
    Display version details


$ satellite agent help
usage: satellite agent [<flags>]

Start monitoring agent

Flags:
  --help                     Show context-sensitive help (also try --help-long and --help-man).
  --debug                    Enable verbose mode
  --role=ROLE                Agent role
  --rpc-addr=127.0.0.1:7575  Address to bind the RPC listener to. Can be specified multiple times
  --name=NAME                Agent name. Must be the same as the name of the local serf node
  --serf-rpc-addr="127.0.0.1:7373"  
                             RPC address of the local serf node
  --initial-cluster=INITIAL-CLUSTER  
                             Initial cluster configuration as a comma-separated list of peers
  --state-dir=STATE-DIR      Directory to store agent-specific state
  --tags=TAGS                Set initial tags as comma-separated list of key:value pairs

$ satellite agent --name=hostname --tags=label:value,another-label:another-value
```

Out of the box, the agent requires at least a single master node (agent with `role=master`). If no master is found during
the test, the cluster state will be marked as `degraded`.


[//]: # (Footnots and references)

[Kubernetes]: <https://github.com/kubernetes/kubernetes>
[serf]: <https://www.serfdom.io/downloads.html>
[Go]: <https://golang.org/doc/install>
[godep]: <https://github.com/tools/godep>
[Setup GOPATH]: <https://golang.org/doc/code.html#GOPATH>
[sqlite]: <https://www.sqlite.org/>
