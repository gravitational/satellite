// Package clusterstatus implements checks and status of
// monitored K8S cluster
package clusterstatus

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	log "github.com/Sirupsen/logrus"
	"github.com/gravitational/satellite/healthz/config"
	"github.com/gravitational/satellite/healthz/utils"
	"github.com/gravitational/trace"
)

// ClusterStatus represent current status of monitored K8S cluster
type ClusterStatus struct {
	K8S struct {
		Healthy      bool
		NodesTotal   int
		NodesHealthy int
	}
	Etcd struct {
		Healthy bool
	}
	Mux sync.Mutex `json:"-"`
}

type key int

const (
	contextKey                 key = iota
	statusK8SClusterUnhealthy      = "K8S cluster is unhealthy"
	statusEtcdClusterUnhealthy     = "Etcd cluster is unhealthy"
)

// AttachToContext adds *ClusterStatus to context
func AttachToContext(ctx context.Context, status *ClusterStatus) context.Context {
	return context.WithValue(ctx, contextKey, status)
}

// FromContext tries to get *ClusterStatus from context
func FromContext(ctx context.Context) (*ClusterStatus, bool) {
	status, ok := ctx.Value(contextKey).(*ClusterStatus)
	return status, ok
}

// NewClusterStatus initializes state as healthy cluster
func NewClusterStatus() *ClusterStatus {
	c := &ClusterStatus{}
	c.K8S.Healthy = true
	c.Etcd.Healthy = true
	return c
}

func (c *ClusterStatus) String() string {
	c.Mux.Lock()
	defer c.Mux.Unlock()
	s, err := json.Marshal(c)
	if err != nil {
		log.Error(trace.Wrap(err).Error())
	}
	return string(s)
}

func (c *ClusterStatus) updateK8sClusterStatus() error {
	_, err := utils.ExecCommand("kubectl", "cluster-info")
	c.Mux.Lock()
	defer c.Mux.Unlock()
	c.K8S.Healthy = (err == nil) // No errors -- healthy
	return nil
}

func (c *ClusterStatus) updateEtcdStatus() error {
	_, err := utils.ExecCommand("etcdctl", "cluster-health")
	c.Mux.Lock()
	defer c.Mux.Unlock()
	c.Etcd.Healthy = (err == nil) // No errors -- healthy
	return nil
}

func (c *ClusterStatus) updateK8SNodesStatus() error {
	out, err := utils.ExecCommand("kubectl", "get", "nodes", "--no-headers", "-o=wide")
	c.Mux.Lock()
	defer c.Mux.Unlock()
	if err != nil {
		c.K8S.Healthy = false
	}
	var nodesTotal int
	var nodesReady int
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
	if err := scanner.Err(); err != nil {
		return trace.Wrap(err)
	}
	c.K8S.NodesTotal = nodesTotal
	c.K8S.NodesHealthy = nodesReady
	return nil
}

// DoChecks sequentially makes all present checks
func (c *ClusterStatus) DoChecks() error {
	if err := c.updateK8sClusterStatus(); err != nil {
		return trace.Wrap(err)
	}
	if err := c.updateEtcdStatus(); err != nil {
		return trace.Wrap(err)
	}
	if err := c.updateK8SNodesStatus(); err != nil {
		return trace.Wrap(err)
	}
	log.Debugf("Cluster status: %v", c)
	return nil
}

// EvaluateStatus returns first found problem after reading and evaluating ClusterStatus
func (c *ClusterStatus) EvaluateStatus(t *config.Thresholds) (healthy bool, msg string) {
	c.Mux.Lock()
	defer c.Mux.Unlock()

	// Report K8S cluster general status
	if !c.K8S.Healthy {
		return false, statusK8SClusterUnhealthy
	}

	// Report Etcd cluster general status
	if !c.Etcd.Healthy {
		return false, statusEtcdClusterUnhealthy
	}

	// Report if a lot of nodes are unavailable
	currentThreshold := t.GetByNodeCount(c.K8S.NodesTotal)
	// If no nodes present assume cluster is totally unavailable
	k8sDeadNodesPercent := 100
	if c.K8S.NodesTotal != 0 {
		k8sDeadNodesPercent = 100. * (c.K8S.NodesTotal - c.K8S.NodesHealthy) / c.K8S.NodesTotal
	}
	if k8sDeadNodesPercent >= currentThreshold {
		msg := fmt.Sprintf("%d%% of nodes are unavailable (threshold %d%%)", k8sDeadNodesPercent, currentThreshold)
		return false, msg
	}

	return true, ""
}
