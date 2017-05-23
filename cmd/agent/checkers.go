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
	"github.com/gravitational/satellite/agent"
	"github.com/gravitational/satellite/monitoring"
	"github.com/gravitational/trace"
)

// config represents configuration for setting up monitoring checkers.
type config struct {
	// role is the current agent's role
	role agent.Role
	// kubeAddr is the address of the kubernetes API server
	kubeAddr string
	// kubeletAddr is the address of the kubelet
	kubeletAddr string
	// dockerAddr is the endpoint of the docker daemon
	dockerAddr string
	// nettestContainerImage is the image name to use for networking test
	nettestContainerImage string
	// disableInterPodCheck disables inter-pod communication tests
	disableInterPodCheck bool
	// etcd defines etcd-specific configuration
	etcd *monitoring.ETCDConfig
}

// addCheckers adds checkers to the agent.
func addCheckers(node agent.Agent, config *config) (err error) {
	switch config.role {
	case agent.RoleMaster:
		err = addToMaster(node, config)
	case agent.RoleNode:
		err = addToNode(node, config)
	}
	return trace.Wrap(err)
}

func addToMaster(node agent.Agent, config *config) error {
	etcdChecker, err := monitoring.EtcdHealth(config.etcd)
	if err != nil {
		return trace.Wrap(err)
	}
	node.AddChecker(monitoring.KubeAPIServerHealth(config.kubeAddr, ""))
	node.AddChecker(monitoring.DockerHealth(config.dockerAddr))
	node.AddChecker(etcdChecker)
	node.AddChecker(monitoring.SystemdHealth())
	node.AddChecker(monitoring.KubeMasterProcessesHealth())
	node.AddChecker(monitoring.KubeMasterSocketsHealth())

	if !config.disableInterPodCheck {
		node.AddChecker(monitoring.InterPodCommunication(config.kubeAddr, config.nettestContainerImage))
	}
	return nil
}

func addToNode(node agent.Agent, config *config) error {
	etcdChecker, err := monitoring.EtcdHealth(config.etcd)
	if err != nil {
		return trace.Wrap(err)
	}
	node.AddChecker(monitoring.KubeletHealth(config.kubeletAddr))
	node.AddChecker(monitoring.DockerHealth(config.dockerAddr))
	node.AddChecker(etcdChecker)
	node.AddChecker(monitoring.SystemdHealth())
	node.AddChecker(monitoring.KubeNodeProcessesHealth())
	node.AddChecker(monitoring.KubeNodeSocketsHealth())
	return nil
}
