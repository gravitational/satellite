package checks

import (
	"fmt"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/gravitational/trace"

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/healthz/config"
	"github.com/gravitational/satellite/monitoring"
)

// RunAll runs all checks successively and reports general cluster status
func RunAll(cfg config.Config) (*pb.Probe, error) {
	var probes health.Probes
	var checkers health.Checkers

	checkers.AddChecker(monitoring.KubeAPIServerHealth(cfg.KubeAddr))
	checkers.AddChecker(monitoring.NodesStatusHealth(cfg.KubeAddr))
	etcdChecker, err := monitoring.EtcdHealth(&cfg.ETCDConfig)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	checkers.AddChecker(etcdChecker)

	for _, c := range checkers {
		log.Infof("running checker %s", c.Name())
		c.Check(&probes)
	}

	return finalHealth(probes), nil
}

// finalHealth aggregates statuses from all probes into one summarized health status
func finalHealth(probes health.Probes) *pb.Probe {
	var errors []string
	status := pb.Probe_Running

	for _, probe := range probes {
		switch probe.Status {
		case pb.Probe_Running:
			status = pb.Probe_Running
			errors = append(errors, fmt.Sprintf("Check %s: OK", probe.Checker))
		default:
			status = pb.Probe_Failed
			errors = append(errors, fmt.Sprintf("Check %s: %s", probe.Checker, probe.Error))
		}
	}

	clusterHealth := pb.Probe{
		Status: status,
		Error:  strings.Join(errors, "\n"),
	}
	log.Debug(trace.Errorf("cluster new health: %#v", clusterHealth))
	return &clusterHealth
}
