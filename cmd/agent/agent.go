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
	"os"
	"os/signal"
	"syscall"

	"github.com/gravitational/satellite/agent"
	"github.com/gravitational/trace"

	log "github.com/sirupsen/logrus"
)

// runAgent starts the monitoring process and blocks waiting for a signal.
func runAgent(config *agent.Config, monitoringConfig *config, peers []string) error {
	if len(peers) > 0 {
		log.Infof("initial cluster=%v", peers)
	}
	log.Infof("starting with config=%v", config)
	monitoringAgent, err := agent.New(config)
	if err != nil {
		return trace.Wrap(err)
	}
	defer monitoringAgent.Close()

	addCheckers(monitoringAgent, monitoringConfig)
	if err = monitoringAgent.Start(); err != nil {
		return trace.Wrap(err)
	}
	if len(peers) > 0 {
		if err = monitoringAgent.Join(peers); err != nil {
			return trace.Wrap(err, "failed to join serf cluster")
		}
	}

	signalc := make(chan os.Signal, 2)
	signal.Notify(signalc, os.Interrupt, syscall.SIGTERM)

	select {
	case <-signalc:
	}

	log.Infof("shutting down agent")
	return nil
}
