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

package etcd

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/gravitational/satellite/monitoring"
	"github.com/gravitational/trace"
	"github.com/prometheus/client_golang/prometheus"
)

const collectMetricsTimeout = 5 * time.Second

// LeaderStats is used by the leader in an etcd cluster, and encapsulates
// statistics about communication with its followers
// reference documentation https://github.com/coreos/etcd/blob/master/etcdserver/stats/leader.go
type LeaderStats struct {
	// Leader is the ID of the leader in the etcd cluster.
	Leader    string                    `json:"leader"`
	Followers map[string]*FollowerStats `json:"followers"`
}

// FollowerStats encapsulates various statistics about a follower in an etcd cluster
type FollowerStats struct {
	Latency LatencyStats `json:"latency"`
	Counts  RaftStats    `json:"counts"`
}

// LatencyStats encapsulates latency statistics.
type LatencyStats struct {
	Current float64 `json:"current"`
}

// RaftStats encapsulates raft statistics.
type RaftStats struct {
	Fail    uint64 `json:"fail"`
	Success uint64 `json:"success"`
}

var (
	followersLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "etcd",
			Name:      "etcd_followers_latency",
			Help:      "Bucketed histogram of latency time (s) between ETCD leader and follower",
			Buckets:   prometheus.ExponentialBuckets(0.0005, 2, 13), // buckets from 0.0001 till 4.096 sec
		}, []string{"followerName"})

	followersRaftFail = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "etcd",
			Name:      "etcd_followers_raft_fail",
			Help:      "Counter of Raft RPC failed requests between ETCD leader and follower",
		}, []string{"followerName"})

	followersRaftSuccess = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "etcd",
			Name:      "etcd_followers_raft_success",
			Help:      "Counter of Raft RPC successful requests between ETCD leader and follower",
		}, []string{"followerName"})
)

func init() {
	prometheus.MustRegister(followersLatency)
	prometheus.MustRegister(followersRaftFail)
	prometheus.MustRegister(followersRaftSuccess)
}

func collectMetrics(config *monitoring.ETCDConfig) error {
	transport, err := config.newHTTPTransport()
	if err != nil {
		return trace.Wrap(err)
	}

	for _, endpoint := range config.Endpoints {
		var leaderStats LeaderStats
		url := fmt.Sprintf("%v/v2/stats/leader", endpoint)
		client := &http.Client{
			Transport: transport,
			Timeout:   collectMetricsTimeout,
		}

		resp, err := client.Get(url)
		if err != nil {
			return trace.Wrap(err)
		}
		defer resp.Body.Close()

		payload, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return trace.Wrap(err)
		}

		err = json.Unmarshal(payload, &leaderStats)
		if err != nil {
			return trace.Wrap(err)
		}

		for id, follower := range leaderStats.Followers {
			followersRaftSuccess.WithLabelValues(id).Set(float64(follower.RaftStats.Success))
			followersRaftFail.WithLabelValues(id).Set(float64(follower.RaftStats.Fail))
			followersLatency.WithLabelValues(id).Observe(follower.Latency.Current)
		}
	}

	return nil
}
