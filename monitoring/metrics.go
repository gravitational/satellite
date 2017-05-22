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

package monitoring

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/gravitational/trace"
	"github.com/prometheus/client_golang/prometheus"
)

const collectETCDMetricsTimeout = 5 * time.Second

// EtcdLeaderStats is used by the leader in an etcd cluster, and encapsulates
// statistics about communication with its followers
type EtcdLeaderStats struct {
	// Leader is the ID of the leader in the etcd cluster.
	Leader    string                        `json:"leader"`
	Followers map[string]*EtcdFollowerStats `json:"followers"`
}

// FollowerStats encapsulates various statistics about a follower in an etcd cluster
type EtcdFollowerStats struct {
	Latency EtcdLatencyStats `json:"latency"`
	Counts  EtcdCountsStats  `json:"counts"`
}

// LatencyStats encapsulates latency statistics.
type EtcdLatencyStats struct {
	Current float64 `json:"current"`
}

// CountsStats encapsulates raft statistics.
type EtcdCountsStats struct {
	Fail    uint64 `json:"fail"`
	Success uint64 `json:"success"`
}

var (
	etcdFollowersLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "etcd",
			Name:      "etcd_followers_latency",
			Help:      "Bucketed histogram of latency time (s) between ETCD leader and follower",
			Buckets:   prometheus.ExponentialBuckets(0.0005, 2, 13), // buckets from 0.0001 till 4.096 sec
		}, []string{"followerName"})

	etcdFollowersCountFails = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "etcd",
			Name:      "etcd_followers_counts_fails",
			Help:      "Counter of Raft RPC failed requests between ETCD leader and follower",
		}, []string{"followerName"})

	etcdFollowersCountSuccess = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "etcd",
			Name:      "etcd_followers_counts_success",
			Help:      "Counter of Raft RPC successful requests between ETCD leader and follower",
		}, []string{"followerName"})
)

func init() {
	prometheus.MustRegister(etcdFollowersLatency)
	prometheus.MustRegister(etcdFollowersCountFails)
	prometheus.MustRegister(etcdFollowersCountSuccess)
}

func collectETCDMetrics(config *ETCDConfig) error {
	transport, err := config.newHTTPTransport()
	if err != nil {
		return trace.Wrap(err)
	}

	for _, endpoint := range config.Endpoints {
		var etcdLeaderStats EtcdLeaderStats
		url := fmt.Sprintf("%v/v2/stats/leader", endpoint)
		client := &http.Client{
			Transport: transport,
			Timeout:   collectETCDMetricsTimeout,
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

		err = json.Unmarshal(payload, &etcdLeaderStats)
		if err != nil {
			return trace.Wrap(err)
		}

		for id, follower := range etcdLeaderStats.Followers {
			etcdFollowersCountSuccess.WithLabelValues(id).Set(float64(follower.Counts.Success))
			etcdFollowersCountFails.WithLabelValues(id).Set(float64(follower.Counts.Fail))
			etcdFollowersLatency.WithLabelValues(id).Observe(follower.Latency.Current)
		}
	}

	return nil
}
