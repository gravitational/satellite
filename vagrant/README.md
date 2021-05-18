# Setup Local Satellite Cluster

The provided Vagrantfile and Ansible playbooks can be used to initialize a
local satellite cluster and quickly test changes in development environment.
The default configuration will setup a three node cluster 
(k8s-master, node-1, node-2). Kubernetes, and Satellite will be installed
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

To ssh into a vm (k8s-master, node-1, node-2):
```console
$ vagrant ssh [node-name]
```

## Scripts

A few scripts are provided once ssh'd into a node.

Get status:
```console
$ ./status.sh
{
   "status": "degraded",
   "nodes": [
       {
         "name": "node-1",
         "member_status": {
            "name": "node-1",
            "addr": "192.168.50.11:7946",
            "status": "alive",
            "tags": {
               "role": "node"
            }
         },
         "status": "degraded",
         "probes": [
            {
               "checker": "etcd-healthz",
               "status": "failed",
               "error": "healthz check failed: Get http://127.0.0.1:2379/health: dial tcp 127.0.0.1:2379: connect: connection refused"
            },
            {
               "checker": "kubelet",
               "status": "running"
            },
            {
               "checker": "docker",
               "status": "running"
            }
         ]
      },
      ...
   ]
}


```

Get history:
```console
$ ./history.sh
[Jan  9 19:14:50 UTC] Node Degraded [node-1]
[Jan  9 19:14:51 UTC] Node Degraded [node-2]
[Jan  9 19:14:52 UTC] Node Degraded [k8s-master]
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