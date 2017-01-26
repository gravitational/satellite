// Package handlers implements http handlers for healthz server
package handlers

import (
	"io"
	"net/http"

	log "github.com/Sirupsen/logrus"
	"github.com/gravitational/satellite/healthz/clusterstatus"
	"github.com/gravitational/satellite/healthz/config"
	"github.com/gravitational/trace"
)

// HandlerHealthz checks access key and responds with evaluated cluster health
func HandlerHealthz(w http.ResponseWriter, req *http.Request) {
	config, ok := config.FromContext(req.Context())
	if !ok {
		http.Error(w, "Bad gateway", http.StatusBadGateway)
		return
	}

	clusterStatus, ok := clusterstatus.FromContext(req.Context())
	if !ok {
		http.Error(w, "Bad gateway", http.StatusBadGateway)
		return
	}

	if req.FormValue("accesskey") != config.AccessKey {
		http.Error(w, "401 Unauthorized", http.StatusUnauthorized)
		return
	}

	if healthy, msg := clusterStatus.EvaluateStatus(config.Thresholds); healthy {
		http.Error(w, msg, http.StatusServiceUnavailable)
		return
	}

	if _, err := io.WriteString(w, "Cluster is healthy"); err != nil {
		log.Errorf("error writing response %s", trace.Wrap(err).Error())
	}
}
