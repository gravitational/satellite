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

// Package test provides helpful gocheck checkers and testing functions.
package test

import (
	"context"
	"time"
)

// WithTimeout runs the provided test case with the default test timeout.
func WithTimeout(fn func(ctx context.Context)) {
	// testTimeout specifies the overall time limit for a test.
	const testTimeout = 5 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	fn(ctx)
}
