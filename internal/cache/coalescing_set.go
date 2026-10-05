package cache

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

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
	panicCounter prometheus.Counter
}

// CoalescingOption configures a CoalescingSet.
type CoalescingOption func(*CoalescingSet)

// WithPanicLogger sets the logger that records a recovered callback panic. A nil logger keeps
// the default, slog.Default().
func WithPanicLogger(logger *slog.Logger) CoalescingOption {
	return func(c *CoalescingSet) {
		if logger != nil {
			c.logger = logger
		}
	}
}

// WithPanicCounter sets the counter a recovered callback panic increments. Register it through
// metric.RegisterOrGet and pass the collector that call returns. Without one a panic is logged
// but not counted.
func WithPanicCounter(counter prometheus.Counter) CoalescingOption {
	return func(c *CoalescingSet) {
		c.panicCounter = counter
	}
}

// NewCoalescingSet creates a new CoalescingSet that fires the callback every window duration
// with the collected (deduplicated) keys. The background goroutine stops when ctx is cancelled
// or when Shutdown is called. A nil ctx or a nil callback panics here, before any goroutine
// starts. A panic in the callback is recovered: the set is a helper inside a service, not a root
// process (owner ruling, #9 comment 5994720412 item 3). The batch the callback was handed is
// dropped, the panic is logged at error level with the batch size and counted on the
// WithPanicCounter counter, and later batches still fire.
func NewCoalescingSet(
	ctx context.Context, window time.Duration, callback func([]string), opts ...CoalescingOption,
) *CoalescingSet {
	if ctx == nil {
		panic("cache: NewCoalescingSet called with a nil context")
	}
	if callback == nil {
		panic("cache: NewCoalescingSet called with a nil callback")
	}
	c := &CoalescingSet{
		pending:  make(map[string]struct{}),
		window:   window,
		callback: callback,
		shutdown: make(chan struct{}),
		done:     make(chan struct{}),
		logger:   slog.Default(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}

	// Handle zero or negative window by using minimum ticker duration
	tickerDuration := window
	if tickerDuration <= 0 {
		tickerDuration = 1 * time.Nanosecond
	}

	c.ticker = time.NewTicker(tickerDuration)

	// Start background goroutine
	go c.run(ctx)

	return c
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
