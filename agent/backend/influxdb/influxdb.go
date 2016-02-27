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
		pt, err := nodeToPoint(nodeStatus, status.Timestamp.ToTime())
		if err != nil {
			return trace.Wrap(err)
		}
		batch.AddPoint(pt)
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
	if int64(config.Timeout) == 0 {
		config.Timeout = defaultTimeout
	}
	if config.Precision == "" {
		config.Precision = "s"
	}
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

// nodeToPoint creates a new time-series from the specified node and a timestamp
func nodeToPoint(status *pb.NodeStatus, timestamp time.Time) (*influx.Point, error) {
	return influx.NewPoint("node", nil, nil, timestamp)
}

// defaultTimeout is the default timeout for InfluxDB writes
const defaultTimeout = 5 * time.Second
