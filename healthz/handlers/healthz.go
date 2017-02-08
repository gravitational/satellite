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

package handlers

import (
	"fmt"
	"net/http"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/healthz/defaults"
)

// Auth checks if client supplied proper access key and responds with HTTP 401
// if authorization fails. Returns true if authorization was successful
func Auth(accessKey string, w http.ResponseWriter, req *http.Request) (passed bool) {
	if req.FormValue(defaults.AccessKeyParam) != accessKey {
		httpStatus := http.StatusUnauthorized
		http.Error(w, http.StatusText(httpStatus), httpStatus)
		return false
	}
	return true
}

// Healthz sends health status to client
func Healthz(clusterHealth pb.Probe, w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	switch clusterHealth.Status {
	case pb.Probe_Running:
		fmt.Fprint(w, clusterHealth.Error)
	default:
		http.Error(w, clusterHealth.Error, http.StatusBadGateway)
	}
}
