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
	"time"

	"github.com/gravitational/satellite/agent"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/trace"

	"golang.org/x/net/context"
)

// statusTimeout is the maximum time status query is blocked.
const statusTimeout = 5 * time.Second

// status queries the status of the local satellite agent on the port
// specified with rpcPort and outputs the results to stderr.
// If local is true, only the local node status is returned, otherwise
// the status of the cluster.
// Returns true if the status query was successful, false - otherwise.
// The output is prettified if prettyPrint is true.
func status(RPCPort int, local, prettyPrint bool) (ok bool, err error) {
	RPCAddr := fmt.Sprintf("127.0.0.1:%d", RPCPort)
	client, err := agent.NewClient(RPCAddr)
	if err != nil {
		return false, trace.Wrap(err)
	}
	var statusJson []byte
	var statusBlob interface{}
	ctx, cancel := context.WithTimeout(context.Background(), statusTimeout)
	defer cancel()
	if local {
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
	if prettyPrint {
		statusJson, err = json.MarshalIndent(statusBlob, "", "   ")
	} else {
		statusJson, err = json.Marshal(statusBlob)
	}
	if err != nil {
		return ok, trace.Wrap(err, "failed to marshal status data")
	}
	if _, err = os.Stderr.Write(statusJson); err != nil {
		return ok, trace.Wrap(err, "failed to output status")
	}
	return ok, nil
}
