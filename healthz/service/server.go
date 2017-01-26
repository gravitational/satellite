package service

import (
	"context"
	"net/http"
	"crypto/tls"
	"net"

	"github.com/gravitational/satellite/healthz/config"
	"github.com/gravitational/satellite/healthz/clusterstatus"
	"github.com/gravitational/satellite/healthz/handlers"
	log "github.com/Sirupsen/logrus"
	"github.com/gravitational/trace"
)

type HealthzServer struct {
	Server 	 http.Server
	Listener net.Listener
}

func HandlerWithContext(key, value interface{}, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), key, value)
		next(w, r.WithContext(ctx))
	}
}

func HandlerWithLogs(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("%s %s %s %s", r.RemoteAddr, r.Host, r.RequestURI, r.UserAgent())
		next(w, r)
	}
}

func NewServer(c config.Config, status clusterstatus.ClusterStatus) *HealthzServer {
	mux := http.NewServeMux()
	handlerHealthz := HandlerWithContext(config.ContextKey, c, handlers.HandlerHealthz)
	handlerHealthz = HandlerWithContext(clusterstatus.ContextKey, status, handlerHealthz)
	handlerHealthz = HandlerWithLogs(handlerHealthz)
	mux.HandleFunc("/healthz", handlerHealthz)

	tcpListener, err := net.Listen("tcp", c.Listen)
	if err != nil {
		trace.Fatalf("Error creating listener: %s", err.Error())
	}

	var server HealthzServer

	if c.CertFile != "" && c.KeyFile != "" && c.CAFile != "" {
		tc, err := NewServerTLS(c.CertFile, c.KeyFile, c.CAFile)
		if err != nil {
			log.Fatal(trace.Wrap(err))
		}
		tlsListener := tls.NewListener(tcpListener, tc)
		server = HealthzServer{
			Server: http.Server{
				Handler: mux,
				TLSConfig: tc,
			},
			Listener: tlsListener,
		}
	} else {
		server = HealthzServer{
			Server: http.Server{
				Handler: mux,
			},
			Listener: tcpListener,
		}
	}

	return &server
}

func (s *HealthzServer) Start() error {
	return s.Server.Serve(s.Listener)
}