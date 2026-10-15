// Package cache provides generic, thread-safe cache implementations with configurable eviction policies.
//
// Strategies supported through one parameterized implementation (memCache):
//   - StrategySimple: No eviction policy (stores items indefinitely)
//   - StrategyLRU: Least Recently Used eviction based on max_size
//   - StrategyTTL: Time-To-Live eviction based on ttl and cleanup_interval
//   - StrategyHybrid: Combined LRU and TTL eviction
//
// All stateful caches are thread-safe with built-in atomic statistics (always enabled for observability)
// and optional Prometheus metrics integration via functional options.
package cache

import (
	"container/list"
	"context"
	"errors"
	"sync"
	"time"

	"github.com/c360studio/semengine/pkg/errs"
)

// Cache represents a generic cache interface that all cache implementations must satisfy.
// The cache is parameterized by value type V for type safety.
type Cache[V any] interface {
	// Get retrieves a value by key. Returns the value and true if found, zero value and false otherwise.
	Get(key string) (V, bool)

	// Set stores a value with the given key. Returns true if a new entry was created, false if updated.
	// Returns an error if the operation fails (e.g., invalid key).
	Set(key string, value V) (bool, error)

	// Delete removes an entry by key. Returns true if the key existed and was deleted.
	// Returns an error if the operation fails.
	Delete(key string) (bool, error)

	// Clear removes all entries from the cache.
	Clear()

	// Size returns the current number of entries in the cache.
	Size() int

	// Keys returns a slice of all keys currently in the cache.
	Keys() []string

	// Stats returns cache statistics if enabled, nil otherwise.
	Stats() *Statistics

	// Close shuts down the cache and releases any resources (e.g., background goroutines).
	Close() error
}

// validateKey validates a cache key for basic requirements.
// Returns a classified error if the key is invalid.
func validateKey(key string) error {
	if key == "" {
		return errs.WrapInvalid(errs.ErrInvalidData, "cache", "validateKey", "key cannot be empty")
	}
	return nil
}

// cacheEntry represents an entry in a stateful in-memory cache.
type cacheEntry[V any] struct {
	key       string
	value     V
	expiresAt time.Time     // zero when TTL eviction is disabled
	element   *list.Element // non-nil when LRU ordering (maxSize > 0) is enabled
}

func (e *cacheEntry[V]) isExpired(now time.Time) bool {
	return !e.expiresAt.IsZero() && now.After(e.expiresAt)
}

// memCache is the unified stateful in-memory cache implementation supporting simple, LRU, TTL,
// and hybrid eviction policies.
type memCache[V any] struct {
	mu              sync.RWMutex
	maxSize         int
	ttl             time.Duration
	cleanupInterval time.Duration
	items           map[string]*cacheEntry[V]
	order           *list.List    // non-nil when maxSize > 0
	stats           *Statistics   // ALWAYS initialized
	metrics         *cacheMetrics // Optional, if metrics enabled

	// Background cleanup coordination (non-nil when ttl > 0)
	shutdown  chan struct{}
	closeOnce sync.Once
	done      chan struct{}
}

// newMemCache creates a stateful in-memory cache with optional LRU (maxSize > 0) and optional TTL
// (ttl > 0, cleanupInterval > 0) eviction.
func newMemCache[V any](
	ctx context.Context, maxSize int, ttl, cleanupInterval time.Duration, opts *cacheOptions[V],
) (*memCache[V], error) {
	if ttl > 0 && ctx == nil {
		return nil, errs.WrapInvalid(errors.New("nil context"), "cache", "newMemCache", "context is required")
	}

	stats := NewStatistics()

	var metrics *cacheMetrics
	if opts.metricsReg != nil && opts.metricsPrefix != "" {
		var err error
		metrics, err = newCacheMetrics(opts.metricsReg, opts.metricsPrefix)
		if err != nil {
			return nil, errs.WrapTransient(err, "cache", "newMemCache", "metrics registration")
		}
	}

	c := &memCache[V]{
		maxSize:         maxSize,
		ttl:             ttl,
		cleanupInterval: cleanupInterval,
		items:           make(map[string]*cacheEntry[V]),
		stats:           stats,
		metrics:         metrics,
	}
	if maxSize > 0 {
		c.order = list.New()
	}
	if ttl > 0 {
		c.shutdown = make(chan struct{})
		c.done = make(chan struct{})
		go c.cleanup(ctx)
	}

	return c, nil
}

// Get retrieves a value by key, checking for expiration and updating LRU order when configured.
func (c *memCache[V]) Get(key string) (V, bool) {
	if c.order == nil {
		c.mu.RLock()
		entry, exists := c.items[key]
		var (
			val       V
			expiresAt time.Time
		)
		if exists {
			val = entry.value
			expiresAt = entry.expiresAt
		}
		c.mu.RUnlock()

		if !exists {
			var zero V
			c.recordMiss()
			return zero, false
		}

		if c.ttl > 0 && !expiresAt.IsZero() && time.Now().After(expiresAt) {
			c.mu.Lock()
			if cur, stillExists := c.items[key]; stillExists && cur.isExpired(time.Now()) {
				c.removeEntry(cur)
				size := len(c.items)
				c.stats.Eviction()
				c.stats.UpdateSize(int64(size))
				if c.metrics != nil {
					c.metrics.recordEviction()
					c.metrics.updateSize(size)
				}
			}
			c.mu.Unlock()

			var zero V
			c.recordMiss()
			return zero, false
		}

		c.recordHit()
		return val, true
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.items[key]
	if !exists {
		var zero V
		c.recordMiss()
		return zero, false
	}

	if c.ttl > 0 && entry.isExpired(time.Now()) {
		c.removeEntry(entry)
		size := len(c.items)
		c.stats.Eviction()
		c.stats.Miss()
		c.stats.UpdateSize(int64(size))
		if c.metrics != nil {
			c.metrics.recordEviction()
			c.metrics.recordMiss()
			c.metrics.updateSize(size)
		}
		var zero V
		return zero, false
	}

	c.order.MoveToFront(entry.element)
	c.recordHit()
	return entry.value, true
}

// Set stores a value with the given key, applying TTL and LRU eviction when configured.
func (c *memCache[V]) Set(key string, value V) (bool, error) {
	if err := validateKey(key); err != nil {
		return false, err
	}
	var expiresAt time.Time
	if c.ttl > 0 {
		expiresAt = time.Now().Add(c.ttl)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, exists := c.items[key]; exists {
		entry.value = value
		entry.expiresAt = expiresAt
		if c.order != nil {
			c.order.MoveToFront(entry.element)
		}
		size := len(c.items)
		c.stats.Set()
		c.stats.UpdateSize(int64(size))
		if c.metrics != nil {
			c.metrics.recordSet()
			c.metrics.updateSize(size)
		}
		return false, nil
	}

	entry := &cacheEntry[V]{
		key:       key,
		value:     value,
		expiresAt: expiresAt,
	}
	if c.order != nil {
		entry.element = c.order.PushFront(entry)
	}
	c.items[key] = entry

	if c.maxSize > 0 && len(c.items) > c.maxSize {
		c.evictLRU()
	}

	size := len(c.items)
	c.stats.Set()
	c.stats.UpdateSize(int64(size))
	if c.metrics != nil {
		c.metrics.recordSet()
		c.metrics.updateSize(size)
	}

	return true, nil
}

// Delete removes an entry by key.
func (c *memCache[V]) Delete(key string) (bool, error) {
	if err := validateKey(key); err != nil {
		return false, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.items[key]
	if !exists {
		return false, nil
	}

	c.removeEntry(entry)
	size := len(c.items)
	c.stats.Delete()
	c.stats.UpdateSize(int64(size))
	if c.metrics != nil {
		c.metrics.recordDelete()
		c.metrics.updateSize(size)
	}

	return true, nil
}

// Clear removes all entries from the cache.
func (c *memCache[V]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*cacheEntry[V])
	if c.order != nil {
		c.order.Init()
	}

	c.stats.UpdateSize(0)
	if c.metrics != nil {
		c.metrics.updateSize(0)
	}
}

// Size returns the current number of entries in the cache.
func (c *memCache[V]) Size() int {
	c.mu.RLock()
	size := len(c.items)
	c.mu.RUnlock()
	return size
}

// Keys returns a slice of all live keys currently in the cache.
// When LRU ordering is enabled (maxSize > 0), keys are returned most-recently-used first.
func (c *memCache[V]) Keys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	keys := make([]string, 0, len(c.items))
	var now time.Time
	if c.ttl > 0 {
		now = time.Now()
	}

	if c.order != nil {
		for element := c.order.Front(); element != nil; element = element.Next() {
			entry := element.Value.(*cacheEntry[V])
			if !entry.isExpired(now) {
				keys = append(keys, entry.key)
			}
		}
		return keys
	}

	for key, entry := range c.items {
		if !entry.isExpired(now) {
			keys = append(keys, key)
		}
	}
	return keys
}

// Stats returns cache statistics.
func (c *memCache[V]) Stats() *Statistics {
	return c.stats
}

// Close shuts down the cache and waits for any background cleanup goroutine to exit.
func (c *memCache[V]) Close() error {
	if c.done == nil {
		return nil
	}
	c.closeOnce.Do(func() { close(c.shutdown) })
	<-c.done
	return nil
}

func (c *memCache[V]) recordHit() {
	c.stats.Hit()
	if c.metrics != nil {
		c.metrics.recordHit()
	}
}

func (c *memCache[V]) recordMiss() {
	c.stats.Miss()
	if c.metrics != nil {
		c.metrics.recordMiss()
	}
}

// evictLRU removes the least recently used item from the cache. Must be called with c.mu held.
func (c *memCache[V]) evictLRU() {
	element := c.order.Back()
	if element == nil {
		return
	}
	entry := element.Value.(*cacheEntry[V])
	c.removeEntry(entry)
	c.stats.Eviction()
	if c.metrics != nil {
		c.metrics.recordEviction()
	}
}

// removeEntry removes entry from c.items and (when c.order != nil) from c.order. Must be called with c.mu held.
func (c *memCache[V]) removeEntry(entry *cacheEntry[V]) {
	delete(c.items, entry.key)
	if c.order != nil && entry.element != nil {
		c.order.Remove(entry.element)
		entry.element = nil
	}
}

// cleanup runs in a background goroutine and periodically removes expired entries.
func (c *memCache[V]) cleanup(ctx context.Context) {
	defer close(c.done)

	ticker := time.NewTicker(c.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.shutdown:
			return
		case <-ticker.C:
			c.removeExpired()
		}
	}
}

// removeExpired removes all expired entries from the cache under c.mu.
func (c *memCache[V]) removeExpired() {
	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()

	expired := 0
	for _, entry := range c.items {
		if entry.isExpired(now) {
			expired++
			c.removeEntry(entry)
		}
	}

	if expired > 0 {
		size := len(c.items)
		for range expired {
			c.stats.Eviction()
		}
		c.stats.UpdateSize(int64(size))
		if c.metrics != nil {
			for range expired {
				c.metrics.recordEviction()
			}
			c.metrics.updateSize(size)
		}
	}
}
