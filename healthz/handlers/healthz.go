package handlers

import (
	"net/http"
	"io"
	"github.com/gravitational/satellite/healthz/clusterstatus"
	"github.com/gravitational/satellite/healthz/config"
)

func HandlerHealthz(w http.ResponseWriter, req *http.Request) {
	config := req.Context().Value(config.ContextKey).(config.Config)
	clusterStatus := req.Context().Value(clusterstatus.ContextKey).(clusterstatus.ClusterStatus)

	if req.FormValue("accesskey") != config.AccessKey {
		http.Error(w, "401 Unauthorized", http.StatusUnauthorized)
		return
	}

	if healthy, msg := clusterStatus.EvaluateStatus(config.Thresholds); healthy {
		http.Error(w, msg, http.StatusBadGateway)
		return
	} else {
		io.WriteString(w, "Cluster is healthy")
	}
}
