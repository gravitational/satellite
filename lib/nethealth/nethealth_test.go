/*
Copyright 2019 Gravitational, Inc.

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
package nethealth

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	v1 "k8s.io/api/core/v1"
	testclient "k8s.io/client-go/kubernetes/fake"
)

const ns = "test-namespace"

func TestResyncPeerList(t *testing.T) {
	server := testServer(t)
	cases := []struct {
		pods           []v1.Pod
		expectedPeers  map[string]*peer
		expectedLookup map[string]string
		description    string
	}{
		{
			description: "skip our own server",
			pods: []v1.Pod{
				newTestPod(testHostIP, testPodIP),
			},
			expectedPeers:  map[string]*peer{},
			expectedLookup: map[string]string{},
		},
		{
			description: "add a peer to the cluster",
			pods: []v1.Pod{
				newTestPod("172.28.128.102", "10.128.0.2"),
			},
			expectedPeers: map[string]*peer{
				"172.28.128.102": newTestPeer("172.28.128.102", "10.128.0.2", server.clock.Now()),
			},
			expectedLookup: map[string]string{
				"10.128.0.2": "172.28.128.102",
			},
		},
		{
			description: "add multiple peers",
			pods: []v1.Pod{
				newTestPod("172.28.128.102", "10.128.0.2"),
				newTestPod("172.28.128.103", "10.128.0.3"),
				newTestPod("172.28.128.104", "10.128.0.4"),
			},
			expectedPeers: map[string]*peer{
				"172.28.128.102": newTestPeer("172.28.128.102", "10.128.0.2", server.clock.Now()),
				"172.28.128.103": newTestPeer("172.28.128.103", "10.128.0.3", server.clock.Now()),
				"172.28.128.104": newTestPeer("172.28.128.104", "10.128.0.4", server.clock.Now()),
			},
			expectedLookup: map[string]string{
				"10.128.0.2": "172.28.128.102",
				"10.128.0.3": "172.28.128.103",
				"10.128.0.4": "172.28.128.104",
			},
		},
	}

	for _, tt := range cases {
		server.resyncNethealth(tt.pods)
		assert.Equal(t, tt.expectedPeers, server.peers, tt.description)
		assert.Equal(t, tt.expectedLookup, server.podToHost, tt.description)
	}
}

const testHostIP = "172.28.128.101"
const testPodIP = "10.128.0.1"

func testServer(t *testing.T) *Server {
	// reset the prometheus registerer between tests
	prometheus.DefaultRegisterer = prometheus.NewRegistry()

	config := Config{
		Namespace: ns,
		HostIP:    testHostIP,
	}

	server, err := config.New()
	assert.NoError(t, err, "error creating test server")

	server.client = testclient.NewSimpleClientset()
	server.clock = clockwork.NewFakeClock()
	return server
}

// newTestPod constructs a new pods with the provided hostIP and podIP
func newTestPod(hostIP, podIP string) v1.Pod {
	return v1.Pod{
		Status: v1.PodStatus{
			HostIP: hostIP,
			PodIP:  podIP,
		},
	}
}

// newTestPeer constructs a new peer with the provided config values.
func newTestPeer(hostIP, podAddr string, ts time.Time) *peer {
	return &peer{
		hostIP:           hostIP,
		podAddr:          &net.IPAddr{IP: net.ParseIP(podAddr)},
		lastStatusChange: ts,
	}
}
