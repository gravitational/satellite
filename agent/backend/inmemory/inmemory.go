package inmemory

import (
	"sync"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"
)

// New creates a new instance of cache
func New() *cache {
	return &cache{}
}

// Update persists the specified cluster status.
func (r *cache) UpdateStatus(status *pb.SystemStatus) error {
	r.Lock()
	defer r.Unlock()
	r.SystemStatus = status.Clone()
	return nil
}

// RecentStatus returns the contents of the last persisted cluster state.
func (r *cache) RecentStatus() (status *pb.SystemStatus, err error) {
	r.RLock()
	defer r.RUnlock()
	return r.SystemStatus, nil
}

// Recycle is a no-op for inmemory cache
func (r *cache) Recycle() error {
	return nil
}

// Close is a no-op for inmemory cache
func (r *cache) Close() error {
	return nil
}

// cache implements agent/cache.Cache interface
type cache struct {
	sync.RWMutex
	*pb.SystemStatus
}
