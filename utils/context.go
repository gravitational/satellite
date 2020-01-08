package utils

import "context"

// IsContextDone returns true if context is done.
func IsContextDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}
