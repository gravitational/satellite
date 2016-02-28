package multiplex

import (
	"github.com/gravitational/satellite/agent/backend"
	"github.com/gravitational/satellite/agent/cache"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/trace"
)

// New creates a new multiplexer from cache and the list of backends
func New(cache cache.Cache, backends ...backend.Backend) *multiplexer {
	return &multiplexer{
		cache:    cache,
		backends: backends,
	}
}

// Update updates system status from status
func (r *multiplexer) UpdateStatus(status *pb.SystemStatus) (err error) {
	if err = r.cache.UpdateStatus(status); err != nil {
		return trace.Wrap(err)
	}

	// TODO: run the actual updates in background to avoid blocking
	// the agent loop
	for _, backend := range r.backends {
		if err = backend.UpdateStatus(status); err != nil {
			return trace.Wrap(err)
		}
	}
	return nil
}

// Read obtains last known system status
func (r *multiplexer) RecentStatus() (*pb.SystemStatus, error) {
	return r.cache.RecentStatus()
}

// Close resets the cache and closes any resources
func (r *multiplexer) Close() (err error) {
	err = r.cache.Close()
	for _, backend := range r.backends {
		err = backend.Close()
	}
	return trace.Wrap(err)
}

// multiplexer implements cache.Cache by delegating to the wrapped cache
// and replicating the status information to the list of backends
type multiplexer struct {
	// cache is the actual cache implementation
	cache cache.Cache

	// backends lists all backends multiplexer replicates into
	backends []backend.Backend
}
