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
	"net/http"
	"strings"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"

	"github.com/gravitational/roundtrip"
	"github.com/gravitational/trace"
	serf "github.com/hashicorp/serf/client"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
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
func newRPCServer(agent *agent, caFile, certFile, keyFile string, rpcAddrs []string) (*server, error) {
	creds, err := credentials.NewServerTLSFromFile(certFile, keyFile)
	if err != nil {
		return nil, trace.Wrap(err, "failed to read certificate/key from %v/%v", certFile, keyFile)
	}

	healthzHandler, err := newHealthHandler(caFile)
	if err != nil {
		return nil, trace.Wrap(err, "failed to read CA certificate from %v", caFile)
	}

	backend := grpc.NewServer(grpc.Creds(creds))
	server := &server{agent: agent, Server: backend}
	pb.RegisterAgentServer(backend, server)
	// handler is a multiplexer for both gRPC and HTTPS queries.
	// The HTTPS endpoint returns the cluster status as JSON
	handler := grpcHandlerFunc(server, healthzHandler)

	for _, addr := range rpcAddrs {
		go http.ListenAndServeTLS(addr, certFile, keyFile, handler)
	}

	return server, nil
}

// newHealthHandler creates a http.Handler that returns cluster status
// from an HTTPS endpoint listening on the same RPC port as the agent.
func newHealthHandler(certFile string) (http.HandlerFunc, error) {
	addr := fmt.Sprintf("127.0.0.1:%v", RPCPort)
	client, err := NewClient(addr, certFile)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		status, err := client.Status(context.TODO())
		if err != nil {
			roundtrip.ReplyJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
			return
		}

		roundtrip.ReplyJSON(w, http.StatusOK, status)
	}, nil
}

// defaultDialRPC is a default RPC client factory function.
// It creates a new client based on address details from the specific serf member.
func defaultDialRPC(certFile string) dialRPC {
	return func(member *serf.Member) (*client, error) {
		return NewClient(fmt.Sprintf("%s:%d", member.Addr.String(), RPCPort), certFile)
	}
}

// grpcHandlerFunc returns an http.Handler that delegates to
// rpcServer on incoming gRPC connections or other otherwise
func grpcHandlerFunc(rpcServer *server, other http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType := r.Header.Get("Content-Type")
		if r.ProtoMajor == 2 && strings.Contains(contentType, "application/grpc") {
			rpcServer.ServeHTTP(w, r)
		} else {
			other.ServeHTTP(w, r)
		}
	})
}
