package service

import (
	"crypto/tls"
	"net"
	"net/http"

	log "github.com/Sirupsen/logrus"
	"github.com/gravitational/satellite/healthz/clusterstatus"
	"github.com/gravitational/satellite/healthz/config"
	"github.com/gravitational/satellite/healthz/handlers"
	"github.com/gravitational/trace"
)

// HealthzServer works as router and listener (tcp and tls)
type HealthzServer struct {
	http.Server
	Listener net.Listener
}

// NewServer creates server with configured routes, tls and general configuration
func NewServer(cfg *config.Config, status *clusterstatus.ClusterStatus) (*HealthzServer, error) {
	mux := http.NewServeMux()
	handlerHealthz := func(w http.ResponseWriter, r *http.Request) {
		ctx := config.AttachToContext(r.Context(), cfg)
		handlers.HandlerHealthz(w, r.WithContext(ctx))
	}
	handlerHealthz = func(w http.ResponseWriter, r *http.Request) {
		ctx := clusterstatus.AttachToContext(r.Context(), status)
		handlerHealthz(w, r.WithContext(ctx))
	}
	handlerHealthz = func(w http.ResponseWriter, r *http.Request) {
		log.Infof("%s %s %s %s", r.RemoteAddr, r.Host, r.RequestURI, r.UserAgent())
		handlerHealthz(w, r)
	}
	mux.HandleFunc("/healthz", handlerHealthz)

	tcpListener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	var server HealthzServer

	if cfg.CertFile != "" && cfg.KeyFile != "" && cfg.CAFile != "" {
		tc, err := NewServerTLS(cfg.CertFile, cfg.KeyFile, cfg.CAFile)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		tlsListener := tls.NewListener(tcpListener, tc)
		server = HealthzServer{
			http.Server{
				Handler:   mux,
				TLSConfig: tc,
			},
			tlsListener,
		}
	} else {
		server = HealthzServer{
			http.Server{
				Handler: mux,
			},
			tcpListener,
		}
	}

	return &server, nil
}

// Start starts preconfigured server
func (s *HealthzServer) Start() error {
	return s.Serve(s.Listener)
}
