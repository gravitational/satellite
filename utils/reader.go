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

package utils

import (
	"context"
	"io"
)

// ReaderWithContext wraps a reader with a context.
type ReaderWithContext struct {
	ctx    context.Context
	reader io.Reader
}

// NewReaderWithContext intializes and returns a new ReaderWithContext.
func NewReaderWithContext(ctx context.Context, reader io.Reader) *ReaderWithContext {
	return &ReaderWithContext{ctx: ctx, reader: reader}
}

// Read reads from the underlying reader. Return without reading if context is
// done.
//
// Implements io.Reader
func (r *ReaderWithContext) Read(p []byte) (n int, err error) {
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	default:
		return r.reader.Read(p)
	}
}
