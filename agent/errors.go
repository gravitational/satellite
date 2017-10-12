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
