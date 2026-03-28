// Package cache provides an interface for cache implementations
package cache

import (
	"context"
	"time"
)

// Cache is the contract that all cache implementations must follow
type Cache interface {
	Get(ctx context.Context, key string) (*Entry, bool)

	Set(ctx context.Context, key string, entry *Entry) error

	Delete(ctx context.Context, key string) error

	Clear(ctx context.Context) error

	Size() int64
}

// Entry represents a single cached HTTP response
type Entry struct {
	StatusCode  int
	Headers     map[string][]string
	Body        []byte
	ContentType string

	// Cache metadata
	CachedAt  time.Time
	ExpiresAt time.Time
	Size      int64
}
