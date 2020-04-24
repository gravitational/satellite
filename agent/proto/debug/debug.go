package debug

import (
	"runtime/pprof"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// NewServer creates a new instance of the Debug service
func NewServer() *Server {
	return &Server{}
}

// Profile executes the specifies debug profile and streams the results to the caller
func (r *Server) Profile(req *ProfileRequest, stream Debug_ProfileServer) error {
	profile := pprof.Lookup(req.Profile)
	if profile == nil {
		return status.Errorf(codes.NotFound, "invalid profile: %v", req.Profile)
	}
	if err := profile.WriteTo(&byteWriter{stream: stream}, 0); err != nil {
		return status.Errorf(codes.Internal, "failed to stream profile: %v", err)
	}
	return nil
}

// Server encapsulates a Debug service
type Server struct {
}

// Write writes the specified byte slice into the underlying stream
func (r *byteWriter) Write(p []byte) (n int, err error) {
	if err := r.stream.Send(&ProfileResponse{
		Output: p[:],
	}); err != nil {
		return 0, err
	}
	return len(p), err
}

type byteWriter struct {
	stream Debug_ProfileServer
}
