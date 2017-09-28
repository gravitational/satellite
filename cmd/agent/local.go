package main

import (
	"context"
	"fmt"

	"github.com/gravitational/satellite/agent/health"
	"github.com/gravitational/satellite/monitoring"
)

func localChecks() {
	ch := monitoring.BasicCheckers()
	var r health.Probes

	ch.Check(context.TODO(), &r)

	fmt.Printf("%+v\n", r)
}
