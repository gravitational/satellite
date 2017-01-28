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

// package influxdb implements a backend to InfluxDB (https://influxdata.com)
package influxdb

import (
	"fmt"
	"time"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/trace"

	influx "github.com/influxdata/influxdb/client/v2"
)

// Config defines the configuration for setting up InfluxDB backend
type Config struct {
	// URL defines the InfluxDB endpoint
	URL string
	// Database for health metrics.  Will be created if not yet existing
	Database string
	// Timeout is the write timeout for the InfluxDB client
	Timeout time.Duration
	// Username for the database
	Username string
	// Password for the database
	Password string
	// Precision defines the timestamp format for metric points
	// (rfc3339: one of `h, m, s, ms, u, ns`)
	Precision string
}

// New creates an instance of the InfluxDB backend using specified configuration
func New(config *Config) (*backend, error) {
	client, err := connect(config)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &backend{
		Config: config,
		client: client,
	}, nil
}

// UpdateStatus writes the specified system status to the InfluxDB as a series
// of probe measurements for each node in the cluster
func (r *backend) UpdateStatus(status *pb.SystemStatus) error {
	batch, err := influx.NewBatchPoints(influx.BatchPointsConfig{
		Database:  r.Database,
		Precision: r.Precision,
	})
	if err != nil {
		return trace.Wrap(err)
	}
	for _, nodeStatus := range status.Nodes {
		if err := addNode(batch, nodeStatus, status.Timestamp.ToTime()); err != nil {
			return trace.Wrap(err)
		}
	}
	return r.client.Write(batch)
}

// Close is a no-op for a InfluxDB backend
func (r *backend) Close() error {
	return nil
}

type backend struct {
	*Config
	client influx.Client
}

func connect(config *Config) (client influx.Client, err error) {
	client, err = influx.NewHTTPClient(influx.HTTPConfig{
		Addr:     config.URL,
		Username: config.Username,
		Password: config.Password,
		Timeout:  config.Timeout,
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}

	// create database if it doesn't exist
	if _, err = client.Query(influx.Query{
		Command: fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", config.Database),
	}); err != nil {
		return nil, trace.Wrap(err)
	}
	return client, nil
}

// addNode adds specified node status to the given batch
//
// Following schema is used for encoding:
//
// Nodes:
//  tags: {"name": "worker", serf_tags}
//  fields: {"addr": "192.168.172.1", "status": "running"}
//
// Probes:
//  tags: {"checker": "systemd", "node": "worker", "state": "failed"}
//  fields: {"error": "storage unavailable", "detail": "service", "code": "500"}
func addNode(batch influx.BatchPoints, status *pb.NodeStatus, timestamp time.Time) error {
	tags := map[string]string{
		"name": status.MemberStatus.Name,
	}
	for key, value := range status.MemberStatus.Tags {
		tags[key] = value
	}
	fields := map[string]interface{}{
		"addr":          status.MemberStatus.Addr,
		"member_status": status.MemberStatus.Status,
		"status":        status.Status,
	}
	pt, err := influx.NewPoint("node", tags, fields, timestamp)
	if err != nil {
		return trace.Wrap(err)
	}
	batch.AddPoint(pt)

	for _, probe := range status.Probes {
		probeStatus, _ := probe.Status.MarshalText()
		tags = map[string]string{
			"checker": probe.Checker,
			"node":    status.Name,
			"status":  string(probeStatus),
		}
		fields = map[string]interface{}{
			"error": probe.Error,
			"code":  probe.Code,
		}
		if probe.Detail != "" {
			fields["detail"] = probe.Detail
		}
		pt, err = influx.NewPoint("probe", tags, fields, timestamp)
		if err != nil {
			return trace.Wrap(err)
		}
		batch.AddPoint(pt)
	}

	return nil
}
