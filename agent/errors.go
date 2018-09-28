/*
Copyright 2016 Gravitational, Inc.

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
package agent

import (
	"github.com/gravitational/trace"
	"google.golang.org/grpc"
	grpcerrors "google.golang.org/grpc/codes"
)

// ConvertGRPCError maps grpc error to one of trace type classes.
// Returns original error if no mapping is possible
func ConvertGRPCError(err error) error {
	if err == nil {
		return nil
	}

	switch grpc.Code(trace.Unwrap(err)) {
	case grpcerrors.InvalidArgument, grpcerrors.OutOfRange:
		return trace.BadParameter(err.Error())
	case grpcerrors.DeadlineExceeded:
		return trace.LimitExceeded(err.Error())
	case grpcerrors.AlreadyExists:
		return trace.AlreadyExists(err.Error())
	case grpcerrors.NotFound:
		return trace.NotFound(err.Error())
	case grpcerrors.PermissionDenied, grpcerrors.Unauthenticated:
		return trace.AccessDenied(err.Error())
	}
	return trace.Wrap(err)
}

// IsUnavailableError determines if the specified error
// is a temporary agent availability error
func IsUnavailableError(err error) bool {
	err = ConvertGRPCError(err)
	switch {
	case grpc.Code(trace.Unwrap(err)) == grpcerrors.Unavailable:
		return true
	case trace.IsLimitExceeded(err):
		return true
	}
	return false
}
