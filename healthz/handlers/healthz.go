package handlers

import (
	"fmt"
	"net/http"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/healthz/defaults"
)

// Auth checks if client supplied proper access key, if not sends
// HTTP 401 Unauthorized and return false, otherwise returns true
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
