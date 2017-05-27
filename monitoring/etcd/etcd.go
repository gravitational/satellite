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
	"sync"
	"time"

	"github.com/gravitational/satellite/monitoring"
	"github.com/gravitational/trace"
	"github.com/prometheus/client_golang/prometheus"

	log "github.com/Sirupsen/logrus"
)

const (
	namespace             = "etcd"
	collectMetricsTimeout = 5 * time.Second
)

var (
	followersLatency = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "etcd_followers_latency"),
		"Bucketed histogram of latency time (s) between ETCD leader and follower",
		//			Buckets:   prometheus.ExponentialBuckets(0.0005, 2, 13), // buckets from 0.0001 till 4.096 sec
		[]string{"followerName"}, nil,
	)

	followersRaftFail = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "etcd_followers_raft_fail"),
		"Counter of Raft RPC failed requests between ETCD leader and follower",
		[]string{"followerName"}, nil,
	)

	followersRaftSuccess = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "etcd_followers_raft_success"),
		"Counter of Raft RPC successful requests between ETCD leader and follower",
		[]string{"followerName"}, nil,
	)
)

// LeaderStats is used by the leader in an etcd cluster, and encapsulates
// statistics about communication with its followers
// reference documentation https://github.com/coreos/etcd/blob/master/etcdserver/stats/leader.go
type LeaderStats struct {
	// Leader is the ID of the leader in the etcd cluster.
	Leader    string                    `json:"leader"`
	Followers map[string]*FollowerStats `json:"followers"`
	Message   string                    `json:"message"`
}

// FollowerStats encapsulates various statistics about a follower in an etcd cluster
type FollowerStats struct {
	Latency   LatencyStats `json:"latency"`
	RaftStats RaftStats    `json:"counts"`
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

// Exporter collects ETCD stats from the given server and exports them using
// the prometheus metrics package.
type Exporter struct {
	client *http.Client
	config monitoring.ETCDConfig
	mutex  sync.RWMutex

	followersLatency     *prometheus.GaugeVec
	followersRaftFail    *prometheus.GaugeVec
	followersRaftSuccess *prometheus.GaugeVec
}

// NewExporter returns an initialized ETCDExporter.
func NewExporter(config *monitoring.ETCDConfig) (*Exporter, error) {
	transport, err := config.NewHTTPTransport()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   collectMetricsTimeout,
	}

	return &Exporter{
		client: client,
		config: *config,
		followersLatency: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "followers_latency",
			Help:      "Latency time (s) between ETCD leader and follower",
		}, []string{"followerName"}),
		followersRaftFail: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "followers_raft_fail",
			Help:      "Counter of Raft RPC failed requests between ETCD leader and follower",
		}, []string{"followerName"}),
		followersRaftSuccess: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "followers_raft_success",
			Help:      "Counter of Raft RPC successful requests between ETCD leader and follower",
		}, []string{"followerName"}),
	}, nil
}

// Describe implements prometheus.Collector.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	e.followersLatency.Describe(ch)
	e.followersRaftFail.Describe(ch)
	e.followersRaftSuccess.Describe(ch)
}

func (e *Exporter) collect(ch chan<- prometheus.Metric) error {
	for _, endpoint := range e.config.Endpoints {

		var leaderStats LeaderStats
		url := fmt.Sprintf("%v/v2/stats/leader", endpoint)

		resp, err := e.client.Get(url)
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

		if leaderStats.Message != "" {
			// Endpoint is not a leader of ETCD cluster
			continue
		}
	membersMap, err := e.getMembers()
	if err != nil {
		return trace.Wrap(err)
	}

		for id, follower := range leaderStats.Followers {
			e.followersRaftSuccess.WithLabelValues(id).Set(float64(follower.RaftStats.Success))
			e.followersRaftFail.WithLabelValues(id).Set(float64(follower.RaftStats.Fail))
			e.followersLatency.WithLabelValues(id).Set(follower.Latency.Current)
		}

		e.followersLatency.Collect(ch)
		e.followersRaftFail.Collect(ch)
		e.followersRaftSuccess.Collect(ch)
		return nil
	}
	return trace.Errorf("ETCD cluster has no leader")
}

// Collect fetches the stats from configured ETCD endpoint and delivers them
// as Prometheus metrics. It implements prometheus.Collector.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	e.mutex.Lock() // To protect metrics from concurrent collects.
	defer e.mutex.Unlock()
	if err := e.collect(ch); err != nil {
		log.Errorf("error collecting stats from ETCD: %v", err)
	}
}

// Member represents simplified ETCD member struct
type Member struct {
	// ID of etcd cluster member
	ID string `json:"id"`
	// Name of etcd cluster member
	Name string `json:"name,omitempty"`
}

type Members struct {
	// List of etcd cluster members
	Members []Member `json:"members"`
}

func (e *Exporter) getMembers() (map[string]string, error) {
	var members Members
	resp, err := e.client.Get(e.client.Endpoint("stats", "leader"), url.Values{})
	if err != nil {
		return nil, trace.Wrap(err)
	}

	err = json.Unmarshal(resp.Bytes(), &members)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	membersMap := make(map[string]string)
	for _, member := range members.Members {
		membersMap[member.ID] = member.Name
	}
	return membersMap, nil
}

	}
	return
}
