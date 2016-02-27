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
package agent

import (
	"fmt"
	"net"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/trace"

	serf "github.com/hashicorp/serf/client"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

// Default RPC port.
const RPCPort = 7575 // FIXME: use serf to discover agents

// RPCServer is the interface that defines the interaction with an agent via RPC.
type RPCServer interface {
	Status(context.Context, *pb.StatusRequest) (*pb.StatusResponse, error)
	LocalStatus(context.Context, *pb.LocalStatusRequest) (*pb.LocalStatusResponse, error)
	Stop()
}

// server implements RPCServer for an agent.
type server struct {
	*grpc.Server
	agent *agent
}

// Status reports the health status of a serf cluster by iterating over the list
// of currently active cluster members and collecting their respective health statuses.
func (r *server) Status(ctx context.Context, req *pb.StatusRequest) (resp *pb.StatusResponse, err error) {
	resp = &pb.StatusResponse{}

	resp.Status, err = r.agent.recentStatus()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return resp, nil
}

// LocalStatus reports the health status of the local serf node.
func (r *server) LocalStatus(ctx context.Context, req *pb.LocalStatusRequest) (resp *pb.LocalStatusResponse, err error) {
	resp = &pb.LocalStatusResponse{}

	resp.Status = r.agent.recentLocalStatus()

	return resp, nil
}

// newRPCServer creates an agent RPC endpoint for each provided listener.
func newRPCServer(agent *agent, listeners []net.Listener) *server {
	backend := grpc.NewServer()
	server := &server{agent: agent, Server: backend}
	pb.RegisterAgentServer(backend, server)
	for _, listener := range listeners {
		go backend.Serve(listener)
	}
	return server
}

// defaultDialRPC is a default RPC client factory function.
// It creates a new client based on address details from the specific serf member.
func defaultDialRPC(member *serf.Member) (*client, error) {
	return NewClient(fmt.Sprintf("%s:%d", member.Addr.String(), RPCPort))
}
