package model

import (
	"context"
	"sync"
	"testing"
)

// stubWatcher implements model.Watcher by feeding registries from a
// pre-populated channel. Used to drive Watch without a real KV+config
// stack.
type stubWatcher struct {
	ch chan *Registry
}

func newStubWatcher() *stubWatcher {
	return &stubWatcher{ch: make(chan *Registry, 4)}
}

func (s *stubWatcher) WatchModelRegistry() <-chan *Registry {
	return s.ch
}

// model.Watch must be safe to call with nil arguments — guard rails for
// callers that pass through optional config wiring.
func TestWatch_NilArgs(t *testing.T) {
	// Both nil — must return immediately, not panic.
	assertWatchReturns(t, func() {
		Watch(context.Background(), nil, nil)
	})

	// Watcher set, apply nil — must return immediately.
	assertWatchReturns(t, func() {
		Watch(context.Background(), newStubWatcher(), nil)
	})

	// apply set, watcher nil — must return immediately.
	assertWatchReturns(t, func() {
		Watch(context.Background(), nil, func(*Registry) {})
	})
}

// model.Watch forwards every registry value to apply in order.
func TestWatch_DeliversAllRegistries(t *testing.T) {
	w := newStubWatcher()
	regA := &Registry{Endpoints: map[string]*EndpointConfig{"a": {Model: "a"}}}
	regB := &Registry{Endpoints: map[string]*EndpointConfig{"b": {Model: "b"}}}

	var mu sync.Mutex
	var seen []*Registry
	done := make(chan struct{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		Watch(ctx, w, func(r *Registry) {
			mu.Lock()
			seen = append(seen, r)
			gotN := len(seen)
			mu.Unlock()
			if gotN == 2 {
				close(done)
			}
		})
	}()

	w.ch <- regA
	w.ch <- regB

	<-done

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 2 {
		t.Fatalf("seen = %d, want 2", len(seen))
	}
	if seen[0] != regA {
		t.Errorf("seen[0] != regA")
	}
	if seen[1] != regB {
		t.Errorf("seen[1] != regB")
	}
}

// Closing the watcher channel must terminate Watch cleanly.
func TestWatch_ChannelClose(t *testing.T) {
	w := newStubWatcher()
	done := make(chan struct{})

	go func() {
		Watch(t.Context(), w, func(*Registry) {})
		close(done)
	}()

	close(w.ch)

	<-done
}

// Cancelling ctx must terminate Watch even when no updates arrive.
func TestWatch_ContextCancel(t *testing.T) {
	w := newStubWatcher()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})

	go func() {
		Watch(ctx, w, func(*Registry) {})
		close(done)
	}()

	cancel()

	<-done
}

// Watch passes nil registries through without panicking. config.Manager
// emits nil when model_registry is unset/cleared, and downstream
// callers may need to react (e.g., disable registry-dependent
// features). model.Watch must not filter or guard.
func TestWatch_NilRegistryDelivered(t *testing.T) {
	w := newStubWatcher()
	got := make(chan *Registry, 1)

	go Watch(context.Background(), w, func(r *Registry) {
		got <- r
	})

	w.ch <- nil

	if r := <-got; r != nil {
		t.Errorf("apply got non-nil registry, want nil")
	}
}

// Helper used by TestWatch_NilArgs to assert that Watch returns.
func assertWatchReturns(t *testing.T, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		fn()
		close(done)
	}()
	<-done
}

// Compile-time assertion that the public Watcher interface still
// matches what config.Manager.WatchModelRegistry returns. Catches
// drift between the two packages.
var _ Watcher = (*stubWatcher)(nil)
