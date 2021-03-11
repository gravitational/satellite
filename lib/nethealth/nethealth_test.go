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
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	testclient "k8s.io/client-go/kubernetes/fake"
)

const ns = "test-namespace"

func TestResyncPeerList(t *testing.T) {
	server := testServer(t)

	cases := []struct {
		add         []*v1.Node
		rm          []string
		expected    map[string]*peer
		description string
	}{
		{
			description: "skip our own server",
			add: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-1",
					},
				},
			},
			expected: map[string]*peer{},
		},
		{
			description: "add a peer to the cluster",
			add: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "peer-1",
					},
				},
			},
			expected: map[string]*peer{
				"peer-1": {
					name:             "peer-1",
					lastStatusChange: server.clock.Now(),
					addr:             &net.IPAddr{},
				},
			},
		},
		{
			description: "add multiple peers",
			add: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "peer-2",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "peer-3",
					},
				},
			},
			expected: map[string]*peer{
				"peer-1": {
					name:             "peer-1",
					lastStatusChange: server.clock.Now(),
					addr:             &net.IPAddr{},
				},
				"peer-2": {
					name:             "peer-2",
					lastStatusChange: server.clock.Now(),
					addr:             &net.IPAddr{},
				},
				"peer-3": {
					name:             "peer-3",
					lastStatusChange: server.clock.Now(),
					addr:             &net.IPAddr{},
				},
			},
		},
		{
			description: "remove peer 2 and 3 from the cluster",
			rm:          []string{"peer-2", "peer-3"},
			expected: map[string]*peer{
				"peer-1": {
					name:             "peer-1",
					lastStatusChange: server.clock.Now(),
					addr:             &net.IPAddr{},
				},
			},
		},
	}

	for _, tt := range cases {
		for _, node := range tt.add {
			_, err := server.client.CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
			assert.NoError(t, err, tt.description)
		}

		for _, node := range tt.rm {
			err := server.client.CoreV1().Nodes().Delete(context.TODO(), node, metav1.DeleteOptions{})
			assert.NoError(t, err, tt.description)
		}

		err := server.resyncPeerList()
		assert.NoError(t, err, tt.description)
		assert.Equal(t, tt.expected, server.peers, tt.description)
	}

}

func TestResyncPods(t *testing.T) {
	server := testServer(t)
	nodes := []*v1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-1",
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "peer-1",
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "peer-2",
			},
		},
	}

	cases := []struct {
		add            []*v1.Pod
		rm             []string
		expectedPeers  map[string]*peer
		expectedLookup map[string]string
		description    string
	}{
		{
			description: "ignore our own node",
			add: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-node-1",
						Labels: map[string]string{
							"k8s-app": "nethealth",
						},
					},
					Spec: v1.PodSpec{
						NodeName: "node-1",
					},
					Status: v1.PodStatus{
						PodIP: "1.1.1.1",
					},
				},
			},
			expectedPeers: map[string]*peer{
				"peer-1": {
					name:             "peer-1",
					lastStatusChange: server.clock.Now(),
					addr:             &net.IPAddr{},
				},
				"peer-2": {
					name:             "peer-2",
					lastStatusChange: server.clock.Now(),
					addr:             &net.IPAddr{},
				},
			},
			expectedLookup: map[string]string{},
		},
		{
			description: "new pod for peer-1",
			add: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-peer-1",
						Labels: map[string]string{
							"k8s-app": "nethealth",
						},
					},
					Spec: v1.PodSpec{
						NodeName: "peer-1",
					},
					Status: v1.PodStatus{
						PodIP: "1.1.1.1",
					},
				},
			},
			expectedPeers: map[string]*peer{
				"peer-1": {
					name:             "peer-1",
					lastStatusChange: server.clock.Now(),
					addr: &net.IPAddr{
						IP: net.ParseIP("1.1.1.1"),
					},
				},
				"peer-2": {
					name:             "peer-2",
					lastStatusChange: server.clock.Now(),
					addr:             &net.IPAddr{},
				},
			},
			expectedLookup: map[string]string{
				"1.1.1.1": "peer-1",
			},
		},
		{
			description: "pod that doesn't have a node set",
			add: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-unknown-peer-1",
						Labels: map[string]string{
							"k8s-app": "nethealth",
						},
					},
					Spec: v1.PodSpec{
						NodeName: "",
					},
					Status: v1.PodStatus{
						PodIP: "1.1.1.1",
					},
				},
			},
			expectedPeers: map[string]*peer{
				"peer-1": {
					name:             "peer-1",
					lastStatusChange: server.clock.Now(),
					addr: &net.IPAddr{
						IP: net.ParseIP("1.1.1.1"),
					},
				},
				"peer-2": {
					name:             "peer-2",
					lastStatusChange: server.clock.Now(),
					addr:             &net.IPAddr{},
				},
			},
			expectedLookup: map[string]string{
				"1.1.1.1": "peer-1",
			},
		},
		{
			description: "pod that doesn't have an equivelant node object",
			add: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-unknown-peer-2",
						Labels: map[string]string{
							"k8s-app": "nethealth",
						},
					},
					Spec: v1.PodSpec{
						NodeName: "non-existant-node",
					},
					Status: v1.PodStatus{
						PodIP: "1.1.1.1",
					},
				},
			},
			expectedPeers: map[string]*peer{
				"peer-1": {
					name:             "peer-1",
					lastStatusChange: server.clock.Now(),
					addr: &net.IPAddr{
						IP: net.ParseIP("1.1.1.1"),
					},
				},
				"peer-2": {
					name:             "peer-2",
					lastStatusChange: server.clock.Now(),
					addr:             &net.IPAddr{},
				},
			},
			expectedLookup: map[string]string{
				"1.1.1.1": "peer-1",
			},
		},
	}

	for _, node := range nodes {
		_, err := server.client.CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
		assert.NoError(t, err, node.Name)
	}
	err := server.resyncPeerList()
	assert.NoError(t, err, "error syncing peer list")

	for _, tt := range cases {
		for _, pod := range tt.add {
			_, err := server.client.CoreV1().Pods(ns).Create(context.TODO(), pod, metav1.CreateOptions{})
			assert.NoError(t, err, tt.description)
		}

		for _, node := range tt.rm {
			err := server.client.CoreV1().Pods(ns).Delete(context.TODO(), node, metav1.DeleteOptions{})
			assert.NoError(t, err, tt.description)
		}

		err = server.resyncNethealthPods()
		assert.NoError(t, err, tt.description)
		assert.Equal(t, tt.expectedPeers, server.peers, tt.description)
		assert.Equal(t, tt.expectedLookup, server.addrToPeer, tt.description)
	}
}

func testServer(t *testing.T) *Server {
	// reset the prometheus registerer between tests
	prometheus.DefaultRegisterer = prometheus.NewRegistry()

	config := Config{
		Namespace: ns,
		NodeName:  "node-1",
	}

	server, err := config.New()
	assert.NoError(t, err, "error creating test server")

	server.client = testclient.NewSimpleClientset()
	server.clock = clockwork.NewFakeClock()

	return server
}
