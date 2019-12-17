# Setup Local Satellite Cluster

The provided Vagrantfile and Ansible playbooks can be used to initialize a
local satellite cluster and quickly test changes in development environment.
The default configuration will setup a three node cluster 
(k8s-master, node-1, node-2). Kubernetes, Serf, and Satellite will be installed
on each node.

## Dependencies
*   [vagrant](https://www.vagrantup.com/)
*   [ansible](https://www.ansible.com/)

## Ansible

*   The ansible directory contains a default ansible.cfg file and a few play books
    used to initialize the local satellite cluster.

    -   **credentials.yaml** initializes server & client credentials used by satellite agents.

    -   **k8s.yaml** initializes a kubernetes cluster with 1 master and 2 nodes.

    -   **satellite.yaml** installs satellite on each node as a service.

    -   **scripts.yaml** adds a few helpful scripts to run client commands.

    -   **serf.yaml** installs and starts serf on each node.

    -   **update-satellite.yaml** updates the satellite binary on each node and 
        restarts the service.


## Instructions

Initialize virtual machines, install dependencies, and start satellite:
```console
$ make all
```

To run an individual playbook:
```console
$ make ansible-[playbook-name]
```

To update satellite binary:
```console
$ make ansible-update-satellite
```

To destroy cluster:
```console
$ make destroy
```

To ssh into a cluster (k8s-master, node-1, node-2):
```console
$ vagrant ssh k8s-master
```

## Scripts

A few scripts are provided once ssh'd into a node.

Get status:
```console
$ ./status.sh
```

Get history:
```console
$ ./history.sh
```

Drop packets from other nodes to test the behavior under network partition:
```console
$ ./drop-input.sh
```

Reset iptable rules. No-op if drop-input has not already been invoked:
```console
$ ./reset-network.sh
```

## Todo

Configure etcd. etcd health checks are always failing and can't test health
cluster state.