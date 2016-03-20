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
	// Role is the current agent's role
	Role agent.Role
	// KubeAddr is the address of the kubernetes API server
	KubeAddr string
	// KubeletAddr is the address of the kubelet
	KubeletAddr string
	// DockerAddr is the endpoint of the docker daemon
	DockerAddr string
	// EtcdAddr is the address of the etcd endpoint
	EtcdAddr string
	// NettestContainerImage is the image name to use for networking test
	NettestContainerImage string
	// TLSConfig defines configuration to secure HTTP communication
	TLSConfig *monitoring.TLSConfig
}

// addCheckers adds checkers to the agent.
func addCheckers(node agent.Agent, config *config) (err error) {
	switch config.Role {
	case agent.RoleMaster:
		err = addToMaster(node, config)
	case agent.RoleNode:
		err = addToNode(node, config)
	}
	return trace.Wrap(err)
}

func addToMaster(node agent.Agent, config *config) error {
	etcdChecker, err := monitoring.EtcdHealth(config.EtcdAddr, config.TLSConfig)
	if err != nil {
		return trace.Wrap(err)
	}
	node.AddChecker(monitoring.KubeApiServerHealth(config.KubeAddr))
	node.AddChecker(monitoring.ComponentStatusHealth(config.KubeAddr))
	node.AddChecker(monitoring.DockerHealth(config.DockerAddr))
	node.AddChecker(etcdChecker)
	node.AddChecker(monitoring.SystemdHealth())
	node.AddChecker(monitoring.IntraPodCommunication(config.KubeAddr, config.NettestContainerImage))
	return nil
}

func addToNode(node agent.Agent, config *config) error {
	etcdChecker, err := monitoring.EtcdHealth(config.EtcdAddr, config.TLSConfig)
	if err != nil {
		return trace.Wrap(err)
	}
	node.AddChecker(monitoring.KubeletHealth(config.KubeletAddr))
	node.AddChecker(monitoring.DockerHealth(config.DockerAddr))
	node.AddChecker(etcdChecker)
	node.AddChecker(monitoring.SystemdHealth())
	return nil
}
