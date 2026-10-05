// Package cache provides high-performance, thread-safe caching implementations with
// multiple eviction policies, built-in statistics tracking, and optional Prometheus
// metrics integration.
//
// # Overview
//
// The cache package offers four cache implementations with different eviction strategies:
//   - Simple: No eviction (manual cleanup only)
//   - LRU: Least Recently Used eviction
//   - TTL: Time-To-Live expiration
//   - Hybrid: Combines LRU and TTL policies
//
// All implementations are generic, thread-safe, and provide comprehensive observability
// through always-on statistics and optional metrics.
//
// # Quick Start
//
// Every constructor except NewNoop returns (Cache[V], error), and the cache is nil when
// the error is not: check the error before using the cache.
//
// Simple cache creation:
//
//	names, err := cache.NewSimple[string]()
//	if err != nil {
//		return err
//	}
//	if _, err := names.Set("key", "value"); err != nil {
//		return err
//	}
//	value, ok := names.Get("key")
//
// LRU cache with capacity limit:
//
//	users, err := cache.NewLRU[*User](1000)
//	if err != nil {
//		return err
//	}
//
// TTL cache with expiration:
//
//	sessions, err := cache.NewTTL[*Session](ctx, 30*time.Minute, 5*time.Minute)
//	if err != nil {
//		return err
//	}
//
// Hybrid cache with both LRU and TTL, built from a Config (there is no direct hybrid constructor):
//
//	responses, err := cache.NewFromConfig[[]byte](ctx, cache.Config{
//		Enabled: true, Strategy: cache.StrategyHybrid,
//		MaxSize: 5000, TTL: 10 * time.Minute, CleanupInterval: 1 * time.Minute,
//	}, cache.WithMetrics[[]byte](registry, "api_cache"))
//	if err != nil {
//		return err
//	}
//
// # Cache Types and Eviction Policies
//
// Simple Cache (No Eviction):
//
// Items remain in cache until explicitly deleted or cache is cleared. Best for
// small, stable datasets where manual control is desired.
//
//	c, err := cache.NewSimple[V]()
//
// LRU Cache (Capacity-Based):
//
// Evicts least recently used items when maximum capacity is reached. Best for
// fixed-size caches where recent access patterns indicate importance.
//
//	c, err := cache.NewLRU[V](maxSize)
//
// TTL Cache (Time-Based):
//
// Items expire after a time-to-live period. Background cleanup goroutine removes
// expired items. Best for time-sensitive data like sessions or tokens.
//
//	c, err := cache.NewTTL[V](ctx, ttl, cleanupInterval)
//
// Hybrid Cache (Capacity + Time):
//
// Combines LRU and TTL - items are evicted if they're either least recently used
// OR expired. Best for production caches requiring both size and time limits.
//
//	c, err := cache.NewFromConfig[V](ctx, cache.Config{
//		Enabled: true, Strategy: cache.StrategyHybrid,
//		MaxSize: maxSize, TTL: ttl, CleanupInterval: cleanupInterval,
//	})
//
// In each case check err before using c, as in Quick Start. NewFromConfig with Enabled
// false returns the no-op cache from NewNoop, which stores nothing and whose Stats is nil.
//
// # Observability Architecture
//
// The cache package implements a dual-tracking pattern for comprehensive observability:
//
// Statistics (Always On):
//   - Tracks all operations using atomic counters
//   - Zero configuration required
//   - Available via cache.Stats()
//   - Provides computed metrics (hit ratio, requests/sec)
//   - No external dependencies
//
// Prometheus Metrics (Optional):
//   - Enabled via WithMetrics() option
//   - Exports to Prometheus for time-series monitoring
//   - Includes component labels for instance identification
//   - Standard metric types (Counter, Gauge)
//
// # Design Decision: Dual Tracking Pattern
//
// Both Statistics and Metrics track operations independently, which appears redundant
// but serves distinct operational purposes:
//
// Why Track Twice?
//
// 1. Independence: Statistics work without Prometheus dependency
//   - Always available for debugging, even in minimal deployments
//   - No external infrastructure required for basic observability
//   - Critical for tests and local development
//
// 2. Computed Metrics: Statistics provide derived values not available in raw Prometheus
//   - Hit ratio (hits / total requests)
//   - Requests per second with built-in timing
//   - Miss ratio (misses / total requests)
//
// 3. Different Use Cases:
//   - Statistics: Programmatic access, debugging, tests, runtime inspection
//   - Metrics: Time-series analysis, Grafana dashboards, alerting, production monitoring
//
// 4. Performance Trade-off:
//   - Overhead: ~50-100ns per operation for dual tracking
//   - At 1M ops/sec: ~0.5-1% total overhead
//   - Cost is negligible compared to observability value
//
// Alternative Considered: Metrics-Based Statistics
//
// We considered reading Statistics from Prometheus metrics to avoid duplication:
//
//	func (s *Statistics) Hits() int64 {
//		dto := &dto.Metric{}
//		s.metrics.hits.Write(dto)
//		return int64(dto.Counter.GetValue())
//	}
//
// Rejected because:
//   - Creates Prometheus dependency for basic stats
//   - Reading from Prometheus is significantly slower (~10x) than atomic operations
//   - Breaks Statistics when metrics are disabled
//   - Violates separation of concerns (stats vs monitoring)
//   - Makes testing more complex (requires mock metrics)
//
// # Performance Impact
//
// Dual tracking overhead per operation:
//   - 1x atomic increment (Statistics)
//   - 1x atomic increment (Prometheus counter) if enabled
//   - 1x gauge set (Prometheus) if enabled
//
// Benchmarks (M1 MacBook Pro):
//   - LRU Get with stats only: ~226ns/op
//   - LRU Get with stats + metrics: ~238ns/op (~5% overhead)
//   - LRU Set with stats only: ~361ns/op
//   - LRU Set with stats + metrics: ~379ns/op (~5% overhead)
//
// At high throughput (1M ops/sec), dual tracking adds ~50-100ms/sec of overhead,
// which is acceptable for the operational visibility gained.
//
// # Functional Options Pattern
//
// The package uses functional options for clean, composable configuration:
//
//	c, err := cache.NewLRU[V](capacity,
//		cache.WithMetrics[V](registry, "component"),
//	)
//	if err != nil {
//		return err
//	}
//
// Available options:
//   - WithMetrics: Enable Prometheus metrics export
//
// This pattern provides:
//   - Clear intent with named functions
//   - Easy composition of features
//   - Backward compatibility when adding options
//   - Type-safe configuration with generics
//
// # Thread Safety
//
// All cache operations are thread-safe for concurrent use:
//   - Multiple goroutines can read concurrently (RWMutex for reads)
//   - Writes are serialized with mutex protection
//   - Statistics use atomic operations (lock-free)
//   - Metrics use Prometheus atomic types
//   - TTL cleanup runs in background goroutine
//
// # Performance Characteristics
//
// Simple Cache:
//   - Get: O(1) map lookup
//   - Set: O(1) map insert
//   - Delete: O(1) map delete
//   - Memory: O(n) where n is number of items
//
// LRU Cache:
//   - Get: O(1) map lookup + list move
//   - Set: O(1) map insert + list append/evict
//   - Delete: O(1) map delete + list remove
//   - Memory: O(n) map + list overhead
//
// TTL Cache:
//   - Get: O(1) map lookup + expiry check
//   - Set: O(1) map insert
//   - Delete: O(1) map delete
//   - Cleanup: O(n) periodic scan (background)
//   - Memory: O(n) map + expiry tracking
//
// Hybrid Cache:
//   - Get: O(1) map lookup + list move + expiry check
//   - Set: O(1) map insert + list append/evict
//   - Delete: O(1) map delete + list remove
//   - Cleanup: O(n) periodic scan (background)
//   - Memory: O(n) map + list + expiry tracking
//
// # Generic Type Support
//
// Caches are fully generic and work with any Go type: the type parameter names the
// value type, as in cache.NewSimple[string](), cache.NewLRU[int](100),
// cache.NewTTL[*User](ctx, 5*time.Minute, 1*time.Minute) or cache.NewFromConfig[[]byte](ctx, cfg).
// Each returns an error to check, as in Quick Start.
//
// Type constraints:
//   - Keys are always strings (for consistent hashing and comparison)
//   - Values can be any type V
//   - No serialization required - stores values directly in memory
//
// # Common Use Cases
//
// Every example below continues with `if err != nil { return err }` before the cache is used.
//
// API Response Caching:
//
//	responses, err := cache.NewFromConfig[*Response](ctx, cache.Config{
//		Enabled: true, Strategy: cache.StrategyHybrid,
//		MaxSize: 5000, TTL: 30 * time.Minute, CleanupInterval: 5 * time.Minute,
//	}, cache.WithMetrics[*Response](registry, "api_cache"))
//
// Session Storage:
//
//	sessions, err := cache.NewTTL[*Session](ctx, 2*time.Hour, 10*time.Minute)
//
// Entity Caching (Two-Level), one error check per constructor:
//
//	hot, err := cache.NewLRU[*Entity](1000) // Hot entities
//	if err != nil {
//		return err
//	}
//	recent, err := cache.NewTTL[*Entity](ctx, 1*time.Hour, 5*time.Minute) // All recent entities
//	if err != nil {
//		return err
//	}
//
// Computed Results:
//
//	results, err := cache.NewLRU[*Result](500,
//		cache.WithMetrics[*Result](registry, "computation_cache"),
//	)
//
// # Context and Cleanup
//
// TTL and Hybrid caches run background cleanup goroutines. Always pass a context
// that will be canceled when cleanup should stop:
//
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//
//	c, err := cache.NewTTL[V](ctx, ttl, cleanupInterval)
//	if err != nil {
//		return err
//	}
//	// Cleanup goroutine stops when ctx is canceled
//
// Close also stops the cleanup goroutine, and returns once it has exited. NewTTL and
// NewFromConfig refuse a nil context with an error.
//
// For Simple and LRU caches, no background goroutines are created.
//
// CoalescingSet's goroutine runs the caller's callback, so it stops through Shutdown(ctx):
// Shutdown returns once the goroutine has exited, or ctx.Err() if ctx ends while the callback is
// still running; the goroutine then exits when the callback returns. NewCoalescingSet refuses a
// nil context, a nil callback, or a panic counter the registry refuses with an error, before it
// starts the goroutine. A panic in the callback is
// recovered: the batch is dropped, the panic is logged with the batch size (WithPanicLogger) and
// counted when WithCoalescingMetrics names a registry, and later batches still fire.
//
// # Testing
//
// The package includes comprehensive tests with race detection:
//
//	go test -race ./internal/cache
//
// Benchmarks are available to validate performance:
//
//	go test -bench=. ./internal/cache
//
// Statistics make testing cache behavior easy:
//
//	c, err := cache.NewSimple[int]()
//	require.NoError(t, err)
//	_, err = c.Set("key", 42)
//	require.NoError(t, err)
//	_, _ = c.Get("key")
//	_, _ = c.Get("missing")
//
//	assert.Equal(t, int64(1), c.Stats().Hits())
//	assert.Equal(t, int64(1), c.Stats().Misses())
//	assert.Equal(t, 0.5, c.Stats().HitRatio())
//
// # Examples
//
// The tests in cache_test.go show each cache in use; the package has no Example functions.
package cache
