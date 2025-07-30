// Package state provides an in-memory cache for tracking processed files
// to avoid redundant extended attribute operations.
package state

import (
	"os"
	"sync"
	"time"
)

// Entry represents a cached file state
type Entry struct {
	Inode uint64
	Mtime time.Time
	Added time.Time
}

// Cache manages the state of processed files with TTL
type Cache struct {
	mu      sync.RWMutex
	entries map[string]Entry // Changed from *Entry to Entry (value type)
	ttl     time.Duration
}

// NewCache creates a new state cache with the specified TTL
func NewCache(ttl time.Duration) *Cache {
	return &Cache{
		entries: make(map[string]Entry),
		ttl:     ttl,
	}
}

// Has checks if a file is in the cache and not expired
// Now includes lazy eviction of expired entries
func (c *Cache) Has(path string, info os.FileInfo) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	entry, exists := c.entries[path]
	if !exists {
		return false
	}
	
	// Check if entry is expired
	if time.Since(entry.Added) > c.ttl {
		// Lazy eviction: remove expired entry
		delete(c.entries, path)
		return false
	}
	
	// Check if file has been modified
	return entry.Inode == getInode(info) && entry.Mtime.Equal(info.ModTime())
}

// Add adds or updates a file entry in the cache
func (c *Cache) Add(path string, info os.FileInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.entries[path] = Entry{ // No pointer, direct value assignment
		Inode: getInode(info),
		Mtime: info.ModTime(),
		Added: time.Now(),
	}
}

// Remove removes a file entry from the cache
func (c *Cache) Remove(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	delete(c.entries, path)
}

// Clean removes expired entries from the cache
// This is now optional and can be called manually if needed
func (c *Cache) Clean() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	now := time.Now()
	for path, entry := range c.entries {
		if now.Sub(entry.Added) > c.ttl {
			delete(c.entries, path)
		}
	}
}

// Size returns the number of entries in the cache
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	return len(c.entries)
}

// StartCleaner is deprecated - we now use lazy eviction in Has()
// This method is kept for backward compatibility but returns a no-op function
func (c *Cache) StartCleaner(interval time.Duration) func() {
	// Return a no-op cleanup function
	return func() {}
}