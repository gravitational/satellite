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
	"encoding/json"
	"fmt"
	"os"

	"github.com/gravitational/satellite/agent"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/trace"

	"golang.org/x/net/context"
)

// rpcConfig defines configuration for an RPC.
type rpcConfig struct {
	// rpcPort specifies local agent rpc port.
	rpcPort int
	// caFile specifies CA used to verify server certificates.
	caFile string
	// certFile specifies client certificate file.
	certFile string
	// keyFile specifies client key file.
	keyFile string
}

// statusConfig defines configuration for a status RPC.
type statusConfig struct {
	rpcConfig
	// local configures status to only collect local node status.
	local bool
	// prettyPrint configures status to print output in a prettified format.
	prettyPrint bool
}

// status queries the status of the local satellite agent on the port
// specified with rpcPort and outputs the results to stderr.
// If local is true, only the local node status is returned, otherwise
// the status of the cluster.
// Returns true if the status query was successful, false - otherwise.
// The output is prettified if prettyPrint is true.
func status(config statusConfig) (ok bool, err error) {
	RPCAddr := fmt.Sprintf("127.0.0.1:%d", config.rpcPort)

	ctx, cancel := context.WithTimeout(context.Background(), statusTimeout)
	defer cancel()

	client, err := agent.NewClient(ctx, RPCAddr, config.caFile, config.certFile, config.keyFile)
	if err != nil {
		return false, trace.Wrap(err)
	}

	var statusJSON []byte
	var statusBlob interface{}
	if config.local {
		status, err := client.LocalStatus(ctx)
		if err != nil {
			return false, trace.Wrap(err)
		}
		ok = status.Status == pb.NodeStatus_Running
		statusBlob = status
	} else {
		status, err := client.Status(ctx)
		if err != nil {
			return false, trace.Wrap(err)
		}
		ok = status.Status == pb.SystemStatus_Running
		statusBlob = status
	}
	if config.prettyPrint {
		statusJSON, err = json.MarshalIndent(statusBlob, "", "   ")
	} else {
		statusJSON, err = json.Marshal(statusBlob)
	}
	if err != nil {
		return ok, trace.Wrap(err, "failed to marshal status data")
	}
	if _, err = os.Stderr.Write(statusJSON); err != nil {
		return ok, trace.Wrap(err, "failed to output status")
	}
	return ok, nil
}

// history queries the status history of the cluster. RPC configuration
// specified by provided config.
// Returns trie if successfully queried the status history, false - otherwise.
func history(config rpcConfig) (ok bool, err error) {
	RPCAddr := fmt.Sprintf("127.0.0.1:%d", config.rpcPort)

	ctx, cancel := context.WithTimeout(context.Background(), statusTimeout)
	defer cancel()

	client, err := agent.NewClient(ctx, RPCAddr, config.caFile, config.certFile, config.keyFile)
	if err != nil {
		return false, trace.Wrap(err)
	}
	timeline, err := client.Timeline(ctx, &pb.TimelineRequest{})
	if err != nil {
		return false, trace.Wrap(err)
	}
	events := timeline.GetEvents()
	for _, event := range events {
		printEvent(event)
	}
	return true, nil
}

// printEvent prints the provided event.
func printEvent(event *pb.TimelineEvent) {
	timestamp := event.GetTimestamp().ToTime()
	fmt.Printf("[%s] ", timestamp.Format(stamp))

	switch event.GetData().(type) {
	case *pb.TimelineEvent_ClusterDegraded:
		fmt.Println("Cluster Degraded")
	case *pb.TimelineEvent_ClusterRecovered:
		fmt.Println("Cluster Recovered")
	case *pb.TimelineEvent_NodeAdded:
		fmt.Printf("Node Added [%s]\n", event.GetNodeAdded().GetNode())
	case *pb.TimelineEvent_NodeRemoved:
		fmt.Printf("Node Removed [%s]\n", event.GetNodeRemoved().GetNode())
	case *pb.TimelineEvent_NodeDegraded:
		fmt.Printf("Node Degraded [%s]\n", event.GetNodeDegraded().GetNode())
	case *pb.TimelineEvent_NodeRecovered:
		fmt.Printf("Node Recovered [%s]\n", event.GetNodeRecovered().GetNode())
	case *pb.TimelineEvent_ProbeFailed:
		e := event.GetProbeFailed()
		fmt.Printf("Probe Failed [%s] [%s]\n", e.GetNode(), e.GetProbe())
	case *pb.TimelineEvent_ProbeSucceeded:
		e := event.GetProbeSucceeded()
		fmt.Printf("Probe Succeeded [%s] [%s]\n", e.GetNode(), e.GetProbe())
	default:
		fmt.Println("Unknown Event")
	}
}
