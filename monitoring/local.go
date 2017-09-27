package monitoring

import (
	"github.com/gravitational/satellite/agent/health"
)

// LocalCheckers
func LocalCheckers() health.Checker {
	return &compositeChecker{
		name: "local",
		checkers: []health.Checker{
			NewIPForwardChecker(),
			NewBrNetfilterChecker(),
			DefaultProcessChecker(),
			DefaultPortChecker(),
		},
	}
}
