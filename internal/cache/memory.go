package cache

import (
	"context"
	"sync"
	"time"
)

// MemoryCache implements the Cache interface using in-memory storage
type MemoryCache struct {
	mu          sync.RWMutex
	entries     map[string]*Entry
	accessOrder []string

	ttl     time.Duration
	maxSize int64

	currentSize int64
}

// NewMemoryCache creates a new in-memory cache
func NewMemoryCache(ttl time.Duration, maxSize int64) *MemoryCache {
	c := &MemoryCache{
		entries:     make(map[string]*Entry),
		ttl:         ttl,
		maxSize:     maxSize,
		accessOrder: make([]string, 0),
	}

	// Start a background goroutine to clean up expired entries
	go c.cleanup()

	return c
}

// Get retrieves an entry from the cache
func (c *MemoryCache) Get(ctx context.Context, key string) (*Entry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Look for the entry
	entry, exists := c.entries[key]
	if !exists {
		return nil, false
	}

	// Check if it's expired
	if time.Now().After(entry.ExpiresAt) {
		// Remove expired entry
		delete(c.entries, key)
		c.currentSize -= entry.Size
		c.removeFromAccessOrder(key)
		return nil, false
	}

	// Mark this entry as recently used (move to end of access order)
	c.updateAccessOrder(key)

	return entry, true
}

// Set stores an entry in the cache
func (c *MemoryCache) Set(ctx context.Context, key string, entry *Entry) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Set expiration time
	entry.ExpiresAt = time.Now().Add(c.ttl)
	entry.Size = int64(len(entry.Body))

	// Make room if needed
	for c.currentSize+entry.Size > c.maxSize && len(c.entries) > 0 {
		c.evictLRU() // Remove least recently used entry
	}

	// If there was an old entry with this key, remove its size
	if old, exists := c.entries[key]; exists {
		c.currentSize -= old.Size
		c.removeFromAccessOrder(key)
	}

	// Store the new entry
	c.entries[key] = entry
	c.currentSize += entry.Size
	c.updateAccessOrder(key)

	return nil
}

// Delete removes an entry from the cache
func (c *MemoryCache) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, exists := c.entries[key]; exists {
		delete(c.entries, key)
		c.currentSize -= entry.Size
		c.removeFromAccessOrder(key)
	}

	return nil
}

// Clear removes all entries from the cache
func (c *MemoryCache) Clear(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*Entry)
	c.currentSize = 0
	c.accessOrder = make([]string, 0)

	return nil
}

// Size returns the current cache size in bytes
func (c *MemoryCache) Size() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.currentSize
}

// evictLRU removes the least recently used entry
func (c *MemoryCache) evictLRU() {
	if len(c.accessOrder) == 0 {
		return
	}

	// Get the least recently used key (first in the list)
	key := c.accessOrder[0]

	// Remove it from cache
	if entry, exists := c.entries[key]; exists {
		delete(c.entries, key)
		c.currentSize -= entry.Size
	}

	// Remove from access order list
	c.accessOrder = c.accessOrder[1:]
}

// updateAccessOrder marks a key as recently used
func (c *MemoryCache) updateAccessOrder(key string) {
	// First remove it from wherever it is
	c.removeFromAccessOrder(key)
	// Then add it to the end (most recent)
	c.accessOrder = append(c.accessOrder, key)
}

// removeFromAccessOrder removes a key from the access order list
func (c *MemoryCache) removeFromAccessOrder(key string) {
	for i, k := range c.accessOrder {
		if k == key {
			// Remove this element
			c.accessOrder = append(c.accessOrder[:i], c.accessOrder[i+1:]...)
			break
		}
	}
}

// cleanup runs in the background and periodically removes expired entries
// Runs every 1 minute to keep the cache clean
func (c *MemoryCache) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		now := time.Now()

		// Check all entries for expiration
		for key, entry := range c.entries {
			if now.After(entry.ExpiresAt) {
				delete(c.entries, key)
				c.currentSize -= entry.Size
				c.removeFromAccessOrder(key)
			}
		}

		c.mu.Unlock()
	}
}
