package cache

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/prometheus/client_golang/prometheus"
)

// CoalescingSet collects keys over a time window and fires a callback with the batch.
// It deduplicates keys automatically using a map-based set structure.
// Its background goroutine runs the caller's callback, so it stops through Shutdown(ctx), which
// bounds the wait for that callback by the caller's context (background-work shape 3).
type CoalescingSet struct {
	pending   map[string]struct{}
	mu        sync.Mutex
	window    time.Duration
	callback  func(keys []string)
	ticker    *time.Ticker
	shutdown  chan struct{}
	done      chan struct{}
	closeOnce sync.Once

	logger       *slog.Logger
	panicCounter prometheus.Counter // nil: panics are logged, not counted
}

// CoalescingOption configures a CoalescingSet.
type CoalescingOption func(*coalescingOptions)

type coalescingOptions struct {
	logger        *slog.Logger
	metricsReg    *metric.MetricsRegistry
	metricsPrefix string
}

// WithPanicLogger sets the logger that records a recovered callback panic. A nil logger keeps
// the default, slog.Default(). The package has no other logger path.
func WithPanicLogger(logger *slog.Logger) CoalescingOption {
	return func(o *coalescingOptions) {
		if logger != nil {
			o.logger = logger
		}
	}
}

// WithCoalescingMetrics counts recovered callback panics on
// semengine_cache_coalescing_callback_panics_total{component=prefix}, registered on registry the
// way WithMetrics registers a cache's collectors. A nil registry or an empty prefix is ignored, as
// WithMetrics ignores them: a panic is then logged but not counted.
func WithCoalescingMetrics(registry *metric.MetricsRegistry, prefix string) CoalescingOption {
	return func(o *coalescingOptions) {
		if registry != nil && prefix != "" {
			o.metricsReg = registry
			o.metricsPrefix = prefix
		}
	}
}

// NewCoalescingSet creates a new CoalescingSet that fires the callback every window duration
// with the collected (deduplicated) keys. The background goroutine stops when ctx is cancelled
// or when Shutdown is called. A nil ctx or a nil callback is refused with an invalid-argument
// error, and a panic counter the registry refuses with a transient error, as the cache
// constructors return a metrics-registration error; in each case no goroutine starts. A panic in
// the callback is recovered: the set is a helper inside a service, not a root process (owner
// ruling, #9 comment 5994720412 item 3). The batch the callback was handed is dropped, the panic
// is logged at error level with the batch size and counted when WithCoalescingMetrics names a
// registry, and later batches still fire.
func NewCoalescingSet(
	ctx context.Context, window time.Duration, callback func([]string), opts ...CoalescingOption,
) (*CoalescingSet, error) {
	if ctx == nil {
		return nil, errs.WrapInvalid(errors.New("nil context"), "cache", "NewCoalescingSet", "context is required")
	}
	if callback == nil {
		return nil, errs.WrapInvalid(errors.New("nil callback"), "cache", "NewCoalescingSet", "callback is required")
	}
	c := &CoalescingSet{
		pending:  make(map[string]struct{}),
		window:   window,
		callback: callback,
		shutdown: make(chan struct{}),
		done:     make(chan struct{}),
	}
	o := coalescingOptions{logger: slog.Default()}
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	c.logger = o.logger
	if o.metricsReg != nil {
		counter, err := metric.RegisterOrGet(o.metricsReg, o.metricsPrefix, "cache_coalescing_callback_panics",
			prometheus.NewCounter(prometheus.CounterOpts{
				Namespace:   "semengine",
				Subsystem:   "cache",
				Name:        "coalescing_callback_panics_total",
				ConstLabels: prometheus.Labels{"component": o.metricsPrefix},
				Help:        "Total number of CoalescingSet callback panics recovered; each dropped its batch",
			}))
		if err != nil {
			return nil, errs.WrapTransient(err, "cache", "NewCoalescingSet", "metrics registration")
		}
		c.panicCounter = counter
	}

	// Handle zero or negative window by using minimum ticker duration
	tickerDuration := window
	if tickerDuration <= 0 {
		tickerDuration = 1 * time.Nanosecond
	}

	c.ticker = time.NewTicker(tickerDuration)

	// Start background goroutine
	go c.run(ctx)

	return c, nil
}

// Add adds a key to the pending set and reports whether it was newly inserted.
// If the key already exists, it is deduplicated. Thread-safe.
func (c *CoalescingSet) Add(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.pending[key]; exists {
		return false
	}
	c.pending[key] = struct{}{}
	return true
}

// Remove removes a key from the pending set and reports whether it existed.
// This is useful when an entity is deleted before the window expires.
// Thread-safe.
func (c *CoalescingSet) Remove(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.pending[key]; !exists {
		return false
	}
	delete(c.pending, key)
	return true
}

// RemovePrefix removes every pending key with the supplied prefix and returns
// the number removed.
// This is useful when several provenance-bearing work items belong to the
// same logical entity and a delete must retire all of them atomically.
func (c *CoalescingSet) RemovePrefix(prefix string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	removed := 0
	for key := range c.pending {
		if strings.HasPrefix(key, prefix) {
			delete(c.pending, key)
			removed++
		}
	}
	return removed
}

// PendingCount returns the number of keys currently pending in the set.
// Thread-safe.
func (c *CoalescingSet) PendingCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.pending)
}

// Drain removes and returns every pending key without invoking the callback.
// Callers use this after Shutdown when queued work owns external resources that
// must be released during shutdown.
func (c *CoalescingSet) Drain() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	keys := make([]string, 0, len(c.pending))
	for key := range c.pending {
		keys = append(keys, key)
	}
	c.pending = make(map[string]struct{})
	return keys
}

// Shutdown stops the background goroutine and waits for it to exit, including a callback it is
// running. It returns nil once the goroutine has exited, and ctx.Err() if ctx ends first: the
// goroutine then exits when the callback returns, and a later Shutdown returns nil. It is safe to
// call more than once and from several goroutines. A nil ctx is refused with an error; the
// goroutine is not stopped.
func (c *CoalescingSet) Shutdown(ctx context.Context) error {
	if ctx == nil {
		return errs.WrapInvalid(errors.New("nil context"), "CoalescingSet", "Shutdown", "context is required")
	}
	c.closeOnce.Do(func() {
		close(c.shutdown)
	})

	// The goroutine having exited is the answer even when ctx has also ended.
	select {
	case <-c.done:
		return nil
	default:
	}
	select {
	case <-c.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// run is the background goroutine that fires the callback on each tick.
// Follows the pattern from ttl.go cleanup method.
func (c *CoalescingSet) run(ctx context.Context) {
	defer close(c.done)
	defer c.ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.shutdown:
			return
		case <-c.ticker.C:
			c.fireBatch()
		}
	}
}

// fireBatch collects the current pending keys, clears the set, and calls the callback.
// The callback is invoked OUTSIDE the lock to prevent deadlocks.
func (c *CoalescingSet) fireBatch() {
	// Lock to read and clear pending keys
	c.mu.Lock()
	if len(c.pending) == 0 {
		c.mu.Unlock()
		// Skip callback if no keys pending
		return
	}

	// Copy keys to slice
	keys := make([]string, 0, len(c.pending))
	for k := range c.pending {
		keys = append(keys, k)
	}

	// Clear the pending set for next window
	c.pending = make(map[string]struct{})
	c.mu.Unlock()

	// Call callback OUTSIDE the lock to prevent deadlock.
	c.runCallback(keys)
}

// runCallback calls the callback and recovers a panic in it (owner ruling, #9 comment 5994720412
// item 3). The batch has already left the pending set, so it is dropped: the drop is declared by
// the error log, which names the batch size, and by the panic counter.
func (c *CoalescingSet) runCallback(keys []string) {
	defer func() {
		if r := recover(); r != nil {
			if c.panicCounter != nil {
				c.panicCounter.Inc()
			}
			c.logger.Error("panic in CoalescingSet callback; batch dropped",
				slog.Any("panic", r), slog.Int("batch_size", len(keys)))
		}
	}()
	c.callback(keys)
}
