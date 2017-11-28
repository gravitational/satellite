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

package monitoring

import (
	"github.com/gravitational/satellite/agent/health"
)

// BasicCheckers will try to detect most common problems preventing k8s cluster to function properly
func BasicCheckers() *compositeChecker {
	return &compositeChecker{
		name: "local",
		checkers: []health.Checker{
			DefaultOSChecker(),
			NewIPForwardChecker(),
			NewBridgeNetfilterChecker(),
			NewMayDetachMountsChecker(),
			DefaultProcessChecker(),
			DefaultPortChecker(),
			DefaultBootConfigParams(),
		},
	}
}

// PreInstallCheckers are designed to run on a node before installing telekube
func PreInstallCheckers() health.Checker {
	base := BasicCheckers()
	base.checkers = append(base.checkers, PreInstallPortChecker())
	return base
}
