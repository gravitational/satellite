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

package monitoring

import (
	"context"
	"net/http"

	"github.com/docker/docker/client"
	"github.com/gravitational/satellite/agent/health"
	"github.com/gravitational/trace"
)

// NewDockerChecker creates DockerChecker using specified host url, read
// https://docs.docker.com/engine/reference/commandline/dockerd/#daemon-socket-option for details
func NewDockerChecker(host string) health.Checker {
	return &DockerChecker{
		Host: host,
	}
}

// DockerChecker is a health.Checker that can validate dockerd daemon health using `docker info` command
type DockerChecker struct {
	Host       string
	HTTPClient *http.Client
}

// Name returns the name of this checker
func (c *DockerChecker) Name() string {
	return "docker"
}

// Check runs a dockerd daemon check via executing `docker info` command and reports errors to the specified Reporter
func (c *DockerChecker) Check(ctx context.Context, reporter health.Reporter) {
	dockerClient, err := client.NewClient(c.Host, "", nil, nil)
	if err != nil {
		reporter.Add(NewProbeFromErr(c.Name(), trace.Errorf("failed to connect: %v", err)))
		return
	}
	_, err = dockerClient.Info(ctx)
	if err != nil {
		reporter.Add(NewProbeFromErr(c.Name(), trace.Errorf("failed to get info: %v", err)))
		return
	}
}
