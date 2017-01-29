/*
Copyright 2017 Gravitational, Inc.

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

package checks

import (
	"fmt"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/gravitational/trace"

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/monitoring"
)

// Runner stores configures checkers and provides interface to configure
// and run them
type Runner struct {
	health.Checkers
}

// NewRunner creates Runner with checks configured using provided options
func NewRunner(kubeAddr string, etcdConfig monitoring.ETCDConfig) (*Runner, error) {
	runner := &Runner{}
	runner.AddChecker(monitoring.KubeAPIServerHealth(kubeAddr))
	runner.AddChecker(monitoring.NodesStatusHealth(kubeAddr))
	etcdChecker, err := monitoring.EtcdHealth(&etcdConfig)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	runner.AddChecker(etcdChecker)
	return runner, nil
}

// Run runs all checks successively and reports general cluster status
func (c *Runner) Run() *pb.Probe {
	var probes health.Probes

	for _, c := range c.Checkers {
		log.Infof("running checker %s", c.Name())
		c.Check(&probes)
	}

	return finalHealth(probes)
}

// finalHealth aggregates statuses from all probes into one summarized health status
func finalHealth(probes health.Probes) *pb.Probe {
	var errors []string
	status := pb.Probe_Running

	for _, probe := range probes {
		switch probe.Status {
		case pb.Probe_Running:
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
