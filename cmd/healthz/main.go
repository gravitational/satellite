package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/gravitational/satellite/healthz/clusterstatus"
	"github.com/gravitational/satellite/healthz/config"
	"github.com/gravitational/satellite/healthz/service"
	"github.com/gravitational/satellite/healthz/utils"
	"github.com/gravitational/trace"
)

func run() error {
	status := clusterstatus.NewClusterStatus()
	cfg := &config.Config{}

	if err := config.ParseCLIFlags(cfg); err != nil {
		return trace.Wrap(err)
	}

	if err := utils.SetupLogging(cfg.LogLevel); err != nil {
		return trace.Wrap(err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		exitSignals := make(chan os.Signal, 1)
		signal.Notify(exitSignals, syscall.SIGTERM, syscall.SIGINT)

		select {
		case sig := <-exitSignals:
			log.Infof("signal: %v", sig)
			cancel()
		}
	}()

	go func() {
		ticker := time.NewTimer(cfg.Interval)
		defer ticker.Stop()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := status.DoChecks(); err != nil {
				log.Errorf(err.Error())
			}
		}
	}()

	server, err := service.NewServer(cfg, status)
	if err != nil {
		return trace.Wrap(err)
	}

	go func() {
		if err := server.Start(); err != nil {
			log.Fatal(trace.Wrap(err))
		}
	}()

	<-ctx.Done()

	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
