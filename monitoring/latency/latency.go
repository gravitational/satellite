/*
Copyright 2019-2020 Gravitational, Inc.

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

// Package latency implements a latency checker that verifies that latency (RTT)
// between nodes in the cluster remain within a specified threshold.
package latency

import (
	"context"
	"fmt"
	"time"

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/lib/membership"

	"github.com/gravitational/trace"
	dto "github.com/prometheus/client_model/go"
	"github.com/sirupsen/logrus"
)

const (
	// checkerID specifies the check name.
	checkerID = "latency"
	// latencyQuantile specifies the default quantile to use when comparing
	// latency metrics to the threshold.
	latencyQuantile = 0.95
	// latencyThreshold is the default latency threshold value.
	latencyThreshold = 15 * time.Millisecond

	// LabelSelectorNethealth specifies the nethealth k8s label selector.
	LabelSelectorNethealth = "k8s-app=nethealth"
	// NamespaceMonitoring specifies the monitoring namespace.
	NamespaceMonitoring = "monitoring"
)

// latencyClient interface provides latency summaries.
type latencyClient interface {
	// LatencySummariesMilli returns the latency summaries for each peer. The
	// latency values represent milliseconds.
	LatencySummariesMilli(ctx context.Context) (map[string]*dto.Summary, error)
}

// Config specifies latency checker config.
type Config struct {
	// NodeName specifies the name of the node that is running the check.
	NodeName string
	// LatencyQuantile specifies the latency quantile.
	LatencyQuantile float64
	// LatencyThreshold specifies the latency threshold.
	LatencyThreshold time.Duration
	// LatencyClient specifies nethealth client that provides latency metrics.
	LatencyClient latencyClient
	// Cluster specifies cluster membership interface.
	membership.Cluster
}

// checkAndSetDefaults validates the config and sets default values.
func (r *Config) checkAndSetDefaults() error {
	var errors []error
	if r.NodeName == "" {
		errors = append(errors, trace.BadParameter("NodeName must be provided"))
	}
	if r.Cluster == nil {
		errors = append(errors, trace.BadParameter("Cluster must be provided"))
	}
	if r.LatencyClient == nil {
		errors = append(errors, trace.BadParameter("LatencyClient must be provided"))
	}
	if len(errors) > 0 {
		return trace.NewAggregate(errors...)
	}
	if r.LatencyQuantile == 0 {
		r.LatencyQuantile = latencyQuantile
	}
	if r.LatencyThreshold == 0 {
		r.LatencyThreshold = latencyThreshold
	}
	return nil
}

// checker verifies that latency (RTT) between nodes in the cluster remain
// within a specified threshold.
//
// Implements health.Checker
type checker struct {
	// Config contains checker configuration.
	*Config
	// FieldLogger is used for logging.
	logrus.FieldLogger
}

// NewChecker constructs a new latency checker.
func NewChecker(config *Config) (health.Checker, error) {
	if err := config.checkAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}

	return &checker{
		Config:      config,
		FieldLogger: logrus.WithField(trace.Component, checkerID),
	}, nil
}

// Name returns the checker name
func (r *checker) Name() string {
	return checkerID
}

// Check executes checks and reports results to the reporter.
func (r *checker) Check(ctx context.Context, reporter health.Reporter) {
	if err := r.check(ctx, reporter); err != nil {
		r.WithError(err).Debug("Failed to verify latency.")
		return
	}
	if reporter.NumProbes() == 0 {
		reporter.Add(successProbe(r.NodeName, r.LatencyThreshold))
	}
}

// check checks the latency between this and other nodes in the cluster.
func (r *checker) check(ctx context.Context, reporter health.Reporter) error {
	members, err := r.Cluster.Members()
	if err != nil {
		return trace.Wrap(err, "failed to query cluster members")
	}

	// Ignore checks if this node is the only member of the cluster.
	if len(members) <= 1 {
		return nil
	}

	summaries, err := r.LatencyClient.LatencySummariesMilli(ctx)
	if err != nil {
		return trace.Wrap(err, "failed to get latency summaries")
	}

	for _, member := range members {
		r.verifyLatency(summaries, member.Name, reporter)
	}

	return nil
}

// verifyLatency verifies the latency for the specified node. Reports a failed
// probe if the latency at the configured quantile is higher than the configured
// threshold.
func (r *checker) verifyLatency(summaries map[string]*dto.Summary, node string, reporter health.Reporter) {
	// Skip self
	if r.NodeName == node {
		return
	}

	summary, exists := summaries[node]
	if !exists {
		r.WithField("node", node).Warn("Missing latency metrics for node.")
		return
	}

	latency, err := latencyAtQuantile(summary, r.LatencyQuantile)
	if err != nil {
		r.WithError(err).
			WithField("node", node).
			WithField("summary", summary).
			Warn("Failed to verify latency.")
		return
	}

	if latency > r.LatencyThreshold {
		reporter.Add(failureProbe(r.NodeName, node, latency, r.LatencyThreshold))
	}
}

// latencyAtQuantile returns the latency at the specified quantile.
// Returns NotFound if latency is not available at the specified quantile.
func latencyAtQuantile(summary *dto.Summary, quantile float64) (latency time.Duration, err error) {
	for _, q := range summary.GetQuantile() {
		if *q.Quantile == quantile {
			return time.Duration(int64(*q.Value)) * time.Millisecond, nil
		}
	}
	return latency, trace.NotFound("latency for quantile %v not available", quantile)
}

// successProbe constructs a probe that represents a successful latency check.
func successProbe(node string, threshold time.Duration) *pb.Probe {
	return &pb.Probe{
		Checker: checkerID,
		Detail: fmt.Sprintf("latency between %s and other nodes is within the allowed threshold of %s",
			node, threshold),
		Status: pb.Probe_Running,
	}
}

// failureProbe constructs a new probe that represents a failed latency check
// between the two nodes.
func failureProbe(node1, node2 string, latency, threshold time.Duration) *pb.Probe {
	return &pb.Probe{
		Checker:  checkerID,
		Detail:   fmt.Sprintf("latency between %s and %s is at %s", node1, node2, latency),
		Error:    fmt.Sprintf("latency is higher than the allowed threshold of %s", threshold),
		Status:   pb.Probe_Failed,
		Severity: pb.Probe_Warning,
	}
}
