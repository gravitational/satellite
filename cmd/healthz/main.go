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

package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gravitational/satellite/healthz/checks"
	"github.com/gravitational/satellite/healthz/config"
	"github.com/gravitational/satellite/healthz/handlers"
	"github.com/gravitational/satellite/healthz/utils"

	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(trace.DebugReport(err))
		fmt.Printf("ERROR: %v\n", err.Error())
		os.Exit(255)
	}
}

func run() error {
	cfg := config.Config{}
	config.ParseCLIFlags(&cfg)

	trace.SetDebug(cfg.Debug)
	if cfg.Debug {
		log.SetLevel(log.DebugLevel)
	}
	log.SetFormatter(&log.TextFormatter{})
	log.SetOutput(os.Stderr)

	log.Debugf("[%q] starting using config: %#v", utils.SourceFileAndLine(), cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		exitSignals := make(chan os.Signal, 1)
		signal.Ignore(syscall.SIGHUP)
		signal.Notify(exitSignals, syscall.SIGTERM, syscall.SIGINT)

		select {
		case sig := <-exitSignals:
			log.Infof("signal: %v", sig)
			cancel()
		}
	}()

	errChan := make(chan error, 10)
	runner, err := checks.NewRunner(cfg.KubeAddr, cfg.KubeNodesThreshold, cfg.ETCDConfig)
	if err != nil {
		return trace.Wrap(err)
	}

	clusterHealth := runner.Run(context.TODO())
	clusterHealthMu := sync.Mutex{}

	http.HandleFunc("/healthz", func(w http.ResponseWriter, req *http.Request) {
		log.Infof("%s %s %s %s", req.RemoteAddr, req.Host, req.RequestURI, req.UserAgent())
		if !handlers.Auth(cfg.AccessKey, w, req) {
			return
		}
		clusterHealthMu.Lock()
		status := *clusterHealth
		clusterHealthMu.Unlock()
		handlers.Healthz(status, w, req)
	})

	listener, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return trace.Wrap(err)
	}

	if cfg.CAFile != "" && cfg.KeyFile != "" && cfg.CertFile != "" {
		tlsConfig, err := utils.NewServerTLS(cfg.CertFile, cfg.KeyFile, cfg.CAFile)
		if err != nil {
			return trace.Wrap(err)
		}
		listener = tls.NewListener(listener, tlsConfig)
	}

	if len(cfg.KubeCertFile) > 0 {
		if err := utils.AddCertToDefaultPool(cfg.KubeCertFile); err != nil {
			return trace.Wrap(err)
		}
	}

	go func() {
		if err := http.Serve(listener, nil); err != nil {
			errChan <- trace.Wrap(err)
			return
		}
	}()

	go func() {
		for {
			ticker := time.NewTimer(cfg.CheckInterval)
			defer ticker.Stop()
			select {
			case <-ticker.C:
				status := runner.Run(context.TODO())
				clusterHealthMu.Lock()
				clusterHealth = status
				clusterHealthMu.Unlock()
			case <-ctx.Done():
				return
			}
		}
	}()

	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return nil
	}
}
