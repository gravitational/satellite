package monitoring

import (
	"fmt"

	"github.com/gravitational/satellite/agent/health"
)

// KubeApiServerHealth creates a checker for the kubernetes API server
func KubeApiServerHealth(kubeAddr string) health.Checker {
	return NewHTTPHealthzChecker("kube-apiserver", fmt.Sprintf("%v/healthz", kubeAddr), kubeHealthz)
}

// KubeletHealth creates a checker for the kubernetes kubelet component
func KubeletHealth(addr string) health.Checker {
	return NewHTTPHealthzChecker("kubelet", fmt.Sprintf("%v/healthz", addr), kubeHealthz)
}

// ComponentStatusHealth creates a checker of the kubernetes component statuses
func ComponentStatusHealth(kubeAddr string) health.Checker {
	return NewComponentStatusChecker(kubeAddr)
}

// EtcdHealth creates a checker that checks health of etcd
func EtcdHealth(addr string) health.Checker {
	return NewHTTPHealthzChecker("etcd-healthz", fmt.Sprintf("%v/health", addr), etcdChecker)
}

// DockerHealth creates a checker that checks health of the docker daemon
func DockerHealth(dockerAddr string) health.Checker {
	return NewUnixSocketHealthzChecker("docker", "http://docker/version", dockerAddr,
		dockerChecker)
}

// SystemdHealth creates a checker that reports the status of systemd units
func SystemdHealth() health.Checker {
	return NewSystemdChecker()
}

// IntraPodCommunication creates a checker that runs a network test in the cluster
// by scheduling pods and verifying the communication
func IntraPodCommunication(kubeAddr, nettestImage string) health.Checker {
	return NewIntraPodChecker(kubeAddr, nettestImage)
}
