package main

import (
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/gravitational/trace"
	"github.com/gravitational/satellite/healthz/utils"
	"github.com/gravitational/satellite/healthz/config"
	"github.com/gravitational/satellite/healthz/clusterstatus"
	"github.com/gravitational/satellite/healthz/service"
)

func main() {
	status := clusterstatus.ClusterStatus{}
	c := config.Config{}

	config.ParseCLIFlags(&c)

	if err := utils.SetupLogging(c.LogLevel); err != nil {
		log.Fatal(trace.Wrap(err))
	}

	// Ensure we fired checks at least once
	status.DoChecks()

	go func() {
		for {
			status.DoChecks()
			time.Sleep(c.Interval)
		}
	}()

	server := service.NewServer(c, status)
	if err := server.Start(); err != nil {
		log.Fatalf("Error starting server: %s", err.Error())
	}
}
