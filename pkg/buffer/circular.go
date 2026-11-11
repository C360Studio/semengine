package buffer

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/c360studio/semengine/pkg/errs"
)

// circularBuffer is a thread-safe circular buffer with configurable overflow policies.
type circularBuffer[T any] struct {
	mu       sync.RWMutex
	items    []T
	capacity int
	size     int
	head     int            // Points to the next write position
	tail     int            // Points to the next read position
	stats    *Statistics    // ALWAYS initialized for observability
	metrics  *bufferMetrics // Optional Prometheus metrics
	opts     *bufferOptions[T]

	// For Block policy. spaceFreed is nil until a writer waits for space; a read, Clear or
	// Close closes it and clears the field, waking every waiting writer to look again.
	notEmpty   *sync.Cond
	spaceFreed chan struct{}
	closed     bool
}

// newCircularBuffer creates a new circular buffer instance.
// Returns an error if metrics registration fails when requested.
func newCircularBuffer[T any](capacity int, opts *bufferOptions[T]) (*circularBuffer[T], error) {
	if capacity <= 0 {
		capacity = 1 // Minimum capacity; declared on NewCircularBuffer
	}

	// Stats are ALWAYS initialized - observability is not optional
	stats := NewStatistics()

	var metrics *bufferMetrics
	// Optionally expose stats as Prometheus metrics
	if opts.metricsReg != nil && opts.metricsPrefix != "" {
		var err error
		metrics, err = newBufferMetrics(opts.metricsReg, opts.metricsPrefix)
		if err != nil {
			// Return classified error instead of silently ignoring
			return nil, errs.WrapTransient(err, "buffer", "newCircularBuffer", "metrics registration")
		}
	}

	cb := &circularBuffer[T]{
		items:    make([]T, capacity),
		capacity: capacity,
		stats:    stats,   // ALWAYS present
		metrics:  metrics, // Optional
		opts:     opts,
	}

	cb.notEmpty = sync.NewCond(&cb.mu)

	return cb, nil
}

// Write adds an item to the buffer according to the overflow policy.
func (cb *circularBuffer[T]) Write(item T) error {
	dropped, hasDropped, err := cb.write(item)
	if hasDropped {
		// After the lock is released, so the callback may call any buffer method.
		cb.opts.dropCallback(dropped)
	}
	return err
}

// write does Write's work under the lock and returns the item a drop callback must receive.
func (cb *circularBuffer[T]) write(item T) (dropped T, hasDropped bool, err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.closed {
		return dropped, false, errs.WrapInvalid(errs.ErrAlreadyStopped, "Buffer", "Write", "buffer closed")
	}

	// Handle different overflow policies when buffer is full
	if cb.size == cb.capacity {
		switch cb.opts.overflowPolicy {
		case DropOldest:
			// Remove oldest item to make room
			dropped = cb.items[cb.tail]
			cb.tail = (cb.tail + 1) % cb.capacity
			cb.size--
			cb.recordDrop()
			hasDropped = cb.opts.dropCallback != nil

		case DropNewest:
			// Drop the new item
			cb.recordDrop()
			return item, cb.opts.dropCallback != nil, nil

		case Block:
			// Wait for space to become available
			for cb.size == cb.capacity && !cb.closed {
				space := cb.spaceChan()
				cb.mu.Unlock()
				<-space
				cb.mu.Lock()
			}

			if cb.closed {
				return dropped, false, errs.WrapInvalid(errs.ErrAlreadyStopped, "Buffer", "Write",
					"buffer closed during blocking wait")
			}
		}
	}

	cb.push(item)
	return dropped, hasDropped, nil
}

// recordDrop counts one overflow and one drop. The caller holds the lock.
func (cb *circularBuffer[T]) recordDrop() {
	// ALWAYS track in stats
	cb.stats.Overflow()
	cb.stats.Drop()

	// ALSO track in metrics if enabled
	if cb.metrics != nil {
		cb.metrics.recordOverflow()
		cb.metrics.recordDrop()
	}
}

// push adds item at the head and records the write. The caller holds the lock and has made room.
func (cb *circularBuffer[T]) push(item T) {
	cb.items[cb.head] = item
	cb.head = (cb.head + 1) % cb.capacity
	cb.size++

	// ALWAYS track in stats
	cb.stats.Write()
	cb.stats.UpdateSize(int64(cb.size))

	// ALSO track in metrics if enabled
	if cb.metrics != nil {
		cb.metrics.recordWrite(cb.size, cb.capacity)
	}

	// Signal waiting readers
	cb.notEmpty.Signal()
}

// spaceChan returns the channel the next freed space closes. The caller holds the lock.
func (cb *circularBuffer[T]) spaceChan() <-chan struct{} {
	if cb.spaceFreed == nil {
		cb.spaceFreed = make(chan struct{})
	}
	return cb.spaceFreed
}

// signalSpace wakes every writer waiting for space. The caller holds the lock.
func (cb *circularBuffer[T]) signalSpace() {
	if cb.spaceFreed != nil {
		close(cb.spaceFreed)
		cb.spaceFreed = nil
	}
}

// Read retrieves and removes one item from the buffer.
func (cb *circularBuffer[T]) Read() (T, bool) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	var zero T

	if cb.size == 0 {
		// Don't record a miss for empty reads, just return
		return zero, false
	}

	// Get the item
	item := cb.items[cb.tail]
	cb.items[cb.tail] = zero // Clear for GC
	cb.tail = (cb.tail + 1) % cb.capacity
	cb.size--

	// ALWAYS track in stats
	cb.stats.Read()
	cb.stats.UpdateSize(int64(cb.size))

	// ALSO track in metrics if enabled
	if cb.metrics != nil {
		cb.metrics.recordRead(cb.size, cb.capacity)
	}

	// Signal waiting writers
	cb.signalSpace()

	return item, true
}

// ReadBatch retrieves and removes up to maxN items from the buffer.
func (cb *circularBuffer[T]) ReadBatch(maxN int) []T {
	if maxN <= 0 {
		return nil
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.size == 0 {
		return nil
	}

	// Determine how many items to read
	readCount := maxN
	if readCount > cb.size {
		readCount = cb.size
	}

	result := make([]T, readCount)
	var zero T

	for i := 0; i < readCount; i++ {
		result[i] = cb.items[cb.tail]
		cb.items[cb.tail] = zero // Clear for GC
		cb.tail = (cb.tail + 1) % cb.capacity
		cb.size--

		// ALWAYS track in stats
		cb.stats.Read()
	}

	// ALWAYS track in stats
	cb.stats.UpdateSize(int64(cb.size))

	// ALSO track in metrics if enabled - count every item removed, then set the final size once
	if cb.metrics != nil {
		cb.metrics.recordReads(readCount, cb.size, cb.capacity)
	}

	// Signal waiting writers: readCount > 0 here, so space was freed
	cb.signalSpace()

	return result
}

// Peek retrieves one item without removing it from the buffer.
func (cb *circularBuffer[T]) Peek() (T, bool) {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	var zero T

	if cb.size == 0 {
		return zero, false
	}

	item := cb.items[cb.tail]

	// ALWAYS track in stats
	cb.stats.Peek()

	// ALSO track in metrics if enabled
	if cb.metrics != nil {
		cb.metrics.recordPeek()
	}

	return item, true
}

// Size returns the current number of items in the buffer.
func (cb *circularBuffer[T]) Size() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.size
}

// Capacity returns the maximum number of items the buffer can hold.
func (cb *circularBuffer[T]) Capacity() int {
	return cb.capacity // This is immutable, so no lock needed
}

// IsFull returns true if the buffer is at maximum capacity.
func (cb *circularBuffer[T]) IsFull() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.size == cb.capacity
}

// IsEmpty returns true if the buffer contains no items.
func (cb *circularBuffer[T]) IsEmpty() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.size == 0
}

// Clear removes all items from the buffer.
func (cb *circularBuffer[T]) Clear() {
	dropped := cb.clear()
	// After the lock is released, so the callback may call any buffer method.
	for _, item := range dropped {
		cb.opts.dropCallback(item)
	}
}

// clear does Clear's work under the lock and returns the items a drop callback must receive.
func (cb *circularBuffer[T]) clear() []T {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	var zero T

	var dropped []T
	if cb.opts.dropCallback != nil {
		dropped = make([]T, cb.size)
		for i := 0; i < cb.size; i++ {
			idx := (cb.tail + i) % cb.capacity
			dropped[i] = cb.items[idx]
		}
	}

	// Clear all items
	for i := 0; i < cb.capacity; i++ {
		cb.items[i] = zero
	}

	cb.head = 0
	cb.tail = 0
	cb.size = 0

	// ALWAYS track in stats
	cb.stats.UpdateSize(0)

	// ALSO track in metrics if enabled
	if cb.metrics != nil {
		cb.metrics.updateSize(0, cb.capacity)
	}

	// Signal all waiting writers
	cb.signalSpace()

	return dropped
}

// Stats returns buffer statistics (always available for observability).
func (cb *circularBuffer[T]) Stats() *Statistics {
	return cb.stats
}

// Close shuts down the buffer and releases resources.
func (cb *circularBuffer[T]) Close() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.closed {
		return nil
	}

	cb.closed = true

	// Wake up all waiting goroutines
	cb.notEmpty.Broadcast()
	cb.signalSpace()

	return nil
}

// WriteWithTimeout attempts to write an item with a timeout when using Block policy.
func (cb *circularBuffer[T]) WriteWithTimeout(item T, timeout time.Duration) error {
	if cb.opts.overflowPolicy != Block {
		return cb.Write(item)
	}
	if timeout <= 0 {
		// As at the pin: a bound already passed refuses, even when there is space.
		return cb.writeBounded("WriteWithTimeout", item, expired, deadlineExceeded, nil)
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	return cb.writeBounded("WriteWithTimeout", item, nil, nil, timer.C)
}

// WriteWithContext attempts to write an item with context cancellation when using Block policy.
func (cb *circularBuffer[T]) WriteWithContext(ctx context.Context, item T) error {
	if ctx == nil {
		return errs.WrapInvalid(errNilContext, "Buffer", "WriteWithContext", "nil context")
	}
	if cb.opts.overflowPolicy != Block {
		return cb.Write(item)
	}
	return cb.writeBounded("WriteWithContext", item, ctx.Done(), ctx.Err, nil)
}

var (
	errNilContext = errors.New("nil context")

	// expired is a closed channel: the bound of a write whose timeout has already passed.
	expired = func() chan struct{} { c := make(chan struct{}); close(c); return c }()
)

func deadlineExceeded() error { return context.DeadlineExceeded }

// writeBounded writes item under the Block policy, waiting for space until done is closed
// (returning doneErr()) or expiry fires (returning context.DeadlineExceeded); either may be nil.
// A bound that has already ended refuses before any wait, even when there is space. It starts
// no goroutine: the wait is one select on the space, done and expiry channels.
func (cb *circularBuffer[T]) writeBounded(op string, item T, done <-chan struct{}, doneErr func() error,
	expiry <-chan time.Time,
) error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.closed {
		return errs.WrapInvalid(errs.ErrAlreadyStopped, "Buffer", op, "buffer closed")
	}

	select {
	case <-done:
		return doneErr()
	default:
	}

	for cb.size == cb.capacity && !cb.closed {
		space := cb.spaceChan()
		cb.mu.Unlock()
		select {
		case <-space:
		case <-done:
			cb.mu.Lock()
			return doneErr()
		case <-expiry:
			cb.mu.Lock()
			return context.DeadlineExceeded
		}
		cb.mu.Lock()
	}

	if cb.closed {
		return errs.WrapInvalid(errs.ErrAlreadyStopped, "Buffer", op, "buffer closed during wait")
	}

	cb.push(item)
	return nil
}
