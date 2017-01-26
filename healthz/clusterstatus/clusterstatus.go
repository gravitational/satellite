package clusterstatus

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
	"sync"

	log "github.com/Sirupsen/logrus"
	"github.com/gravitational/satellite/healthz/config"
	"github.com/gravitational/satellite/healthz/utils"
)

type ClusterStatus struct {
	K8S struct {
		Status       bool
		NodesTotal   uint
		NodesHealthy uint
	}
	Etcd struct {
		Status bool
	}
	Mux sync.Mutex
}

const ContextKey = "clusterstatus"

func (c *ClusterStatus) String() string {
	c.Mux.Lock()
	defer c.Mux.Unlock()
	return fmt.Sprintf("{\"K8S\": {\"Status\": %v, \"NodesTotal\": %v "+
		"\"NodesHealthy\": %v}, \"Etcd\": {\"Status\": %v}}", c.K8S.Status,
		c.K8S.NodesTotal, c.K8S.NodesHealthy, c.Etcd.Status)
}

func (c *ClusterStatus) updateK8sClusterStatus() {
	_, err := utils.ExecCommand("kubectl", "cluster-info")
	c.Mux.Lock()
	defer c.Mux.Unlock()
	c.K8S.Status = (err == nil) // No errors -- healthy
}

func (c *ClusterStatus) updateEtcdStatus() {
	_, err := utils.ExecCommand("etcdctl", "cluster-health")
	c.Mux.Lock()
	defer c.Mux.Unlock()
	c.Etcd.Status = (err == nil) // No errors -- healthy
}

func (c *ClusterStatus) updateK8SNodesStatus() {
	out, err := utils.ExecCommand("kubectl", "get", "nodes", "--no-headers", "-o=wide")
	c.Mux.Lock()
	defer c.Mux.Unlock()
	if err != nil {
		c.K8S.Status = false
	}
	var nodesTotal uint = 0
	var nodesReady uint = 0
	outReader := bytes.NewReader(out)
	scanner := bufio.NewScanner(outReader)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		status := fields[1]
		nodesTotal++
		if status == "Ready" {
			nodesReady++
		}
	}
	c.K8S.NodesTotal = nodesTotal
	c.K8S.NodesHealthy = nodesReady
}

func (c *ClusterStatus) DoChecks() {
	c.updateK8sClusterStatus()
	c.updateEtcdStatus()
	c.updateK8SNodesStatus()
	log.Debugf("Cluster status: %v", c.String())
}

func (c *ClusterStatus) EvaluateStatus(t *config.Thresholds) (bool, string) {
	c.Mux.Lock()
	defer c.Mux.Unlock()

	// Report K8S cluster general status
	if !c.K8S.Status {
		return false, "K8S cluster is unhealthy"
	}

	// Report Etcd cluster general status
	if !c.Etcd.Status {
		return false, "Etcd cluster is unhealthy"
	}

	// Report if a lot of nodes is dead
	currentThreshold := t.GetByNodeCount(c.K8S.NodesTotal)
	k8sDeadNodesPct := uint8(float32(c.K8S.NodesTotal - c.K8S.NodesHealthy) / float32(c.K8S.NodesTotal) * 100.)
	if k8sDeadNodesPct >= currentThreshold {
		msg := fmt.Sprintf("A lot of nodes is dead: %d%% > %d%%", k8sDeadNodesPct, currentThreshold)
		return false, msg
	}

	return true, ""
}