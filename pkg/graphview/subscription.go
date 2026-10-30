package graphview

import (
	"cmp"
	"context"
	"slices"
	"sync"
)

// Subscription is one subscriber's attachment to a View. Deltas() delivers
// coalesced batches; channel close is the explicit terminal signal and Err()
// reports why (nil after Unsubscribe, the subscription context's error after
// its cancellation, ErrViewStopped after a stop, a wrapped ErrWatcherLost
// after a watcher loss). A subscription that detached first keeps its own
// reason. Slowness never disconnects a subscription: an
// undrained subscriber's pending deltas coalesce last-writer-wins per key
// until it resumes.
type Subscription[T any] struct {
	view      *View[T]
	id        uint64
	attachSeq uint64

	mu      sync.Mutex
	pending map[string]Delta[T] // LWW per key; nil after terminate
	err     error

	notify chan struct{}
	deltas chan []Delta[T]
	done   chan struct{}
	closed chan struct{} // closed once deltas has closed
	term   sync.Once
}

// Deltas returns the delivery channel. Each batch holds at most one delta
// per key (the greatest-revision operation), sorted by revision ascending.
// The channel closes on Unsubscribe, subscription-context cancellation, a view
// stop, or watcher loss — check Err() after close. After Unsubscribe or the
// subscription's context ends, a batch already taken may still be delivered
// before the close.
func (s *Subscription[T]) Deltas() <-chan []Delta[T] { return s.deltas }

// Err reports the terminal reason once Deltas has closed: nil for a clean
// Unsubscribe, the context error for a context detach, ErrViewStopped for
// view shutdown, or a wrapped ErrWatcherLost staleness signal.
func (s *Subscription[T]) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Unsubscribe detaches the subscription and releases its buffered state.
// Idempotent; safe to call concurrently with delivery. It does not wait for
// the delivery goroutine; Run does.
func (s *Subscription[T]) Unsubscribe() { s.detach(nil) }

func (s *Subscription[T]) detach(cause error) {
	s.view.removeSub(s.id)
	s.terminate(cause)
}

// terminate records the terminal reason, releases the pending buffer, and
// wakes the delivery goroutine (which closes the deltas channel — the
// explicit terminal close every consumer observes).
func (s *Subscription[T]) terminate(cause error) {
	s.term.Do(func() {
		s.mu.Lock()
		s.err = cause
		s.pending = nil
		s.mu.Unlock()
		close(s.done)
	})
}

// run is the per-subscriber delivery goroutine: it drains the pending buffer
// into the deltas channel OUTSIDE the projection mutex, so a slow consumer
// blocks only its own delivery — never the watcher, the apply, or peers.
//
// Every wait here also selects on s.done, so once terminate has closed it the
// goroutine returns without waiting on a reader: a consumer that never reads
// cannot hold the close of deltas, nor a caller waiting on s.closed.
func (s *Subscription[T]) run(ctx context.Context) {
	defer s.view.wg.Done()
	defer close(s.closed)
	defer close(s.deltas)
	for {
		select {
		case <-s.done:
			return
		case <-ctx.Done():
			s.detach(ctx.Err())
			return
		case <-s.notify:
			batch := s.take()
			if batch == nil {
				continue
			}
			select {
			case s.deltas <- batch:
			case <-s.done:
				return
			case <-ctx.Done():
				s.detach(ctx.Err())
				return
			}
		}
	}
}

// take swaps out the pending buffer and orders it by revision ascending
// (deterministic batches; G4 — a batch never carries an older op after a
// newer one for the same key because the buffer is LWW per key).
func (s *Subscription[T]) take() []Delta[T] {
	s.mu.Lock()
	if len(s.pending) == 0 {
		s.mu.Unlock()
		return nil
	}
	m := s.pending
	s.pending = make(map[string]Delta[T])
	s.mu.Unlock()
	batch := make([]Delta[T], 0, len(m))
	for _, d := range m {
		batch = append(batch, d)
	}
	slices.SortFunc(batch, func(a, b Delta[T]) int { return cmp.Compare(a.Revision, b.Revision) })
	return batch
}
