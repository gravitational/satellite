/*
Copyright 2019 Gravitational, Inc.

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

// Package httplib provides HTTP implementations.
package httplib

import (
	"context"
	"net/http"
)

// TransportWithContext wraps a transport with a context.
//
// Implements http.RoundTripper
type TransportWithContext struct {
	ctx       context.Context
	transport http.RoundTripper
}

// NewTransportWithContext initializes and returns a new TransportWithContext.
// http.DefaultTransport is used as underlying transport.
func NewTransportWithContext(ctx context.Context) *TransportWithContext {
	return &TransportWithContext{
		ctx:       ctx,
		transport: http.DefaultTransport,
	}
}

// RoundTrip executes a single HTTP transaction, returning a Response for the
// provided Request.
func (t *TransportWithContext) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.WithContext(t.ctx)
	return t.transport.RoundTrip(req)
}
