package main

import (
	"github.com/gravitational/satellite/agent"
	"github.com/gravitational/satellite/monitoring"
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
	// NettestImage is the image name to use for networking test
	NettestImage string
}

// addCheckers adds checkers to the agent.
func addCheckers(node agent.Agent, config *config) {
	switch config.Role {
	case agent.RoleMaster:
		addToMaster(node, config)
	case agent.RoleNode:
		addToNode(node, config)
	}
}

func addToMaster(node agent.Agent, config *config) {
	node.AddChecker(monitoring.KubeApiServerHealth(config.KubeAddr))
	node.AddChecker(monitoring.ComponentStatusHealth(config.KubeAddr))
	node.AddChecker(monitoring.DockerHealth(config.DockerAddr))
	node.AddChecker(monitoring.EtcdHealth(config.EtcdAddr))
	node.AddChecker(monitoring.SystemdHealth())
	node.AddChecker(monitoring.IntraPodCommunication(config.KubeAddr, config.NettestImage))
}

func addToNode(node agent.Agent, config *config) {
	node.AddChecker(monitoring.KubeletHealth(config.KubeletAddr))
	node.AddChecker(monitoring.DockerHealth(config.DockerAddr))
	node.AddChecker(monitoring.EtcdHealth(config.EtcdAddr))
	node.AddChecker(monitoring.SystemdHealth())
}
