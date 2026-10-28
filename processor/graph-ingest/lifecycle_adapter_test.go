package graphingest

import (
	"context"
	"maps"
	"sync"

	"github.com/c360studio/semengine/internal/harness/lifecycletest"
	"github.com/nats-io/nats.go/jetstream"
)

// suiteOwner adapts Component to the lifecycle suite (lifecycle-suite, "Observe adapter
// contract"). Unresolved lists, by a stable name each, what a started Component holds and
// what clearLifecycleHandles clears once a cleanup has succeeded: the runtime cancel, the
// two ingest cancels, the keyed ingest pool (whose lanes run until its Shutdown returns),
// the JetStream consumers, the request subscriptions, the status loop, the status
// publisher, the entity cache and the consumers bound for readiness.
//
// The guard's own state is not listed: the guard has no read-out (design D22), and every
// call on it either changes its state or returns before reading it. The suite's checks
// observe it through Start and Stop, as design D13 states it.
//
// Calls counts the consumers a started Component holds and the Drain and Stop calls it
// makes on them: after a Start that returned nil, Start wraps each handle in c.consumers,
// which only release reads (fix-301 design D2, "Suite adapter"). A failed Start's binds
// and its rollback's drains are not counted. Request subscriptions, the pool's Shutdown
// and the cache's Close are not counted; their handles are listed under Unresolved until
// a cleanup succeeds.
type suiteOwner struct {
	c     *Component
	mu    sync.Mutex
	calls map[string]int
}

func newSuiteOwner(c *Component) *suiteOwner {
	return &suiteOwner{c: c, calls: map[string]int{}}
}

func (o *suiteOwner) count(op string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.calls[op]++
}

// Start starts the Component and, when Start returned nil, wraps each consumer handle it
// holds. Start has returned, so nothing else reads or writes c.consumers until Stop.
func (o *suiteOwner) Start(ctx context.Context) error {
	if err := o.c.Start(ctx); err != nil {
		return err
	}
	for index := range o.c.consumers {
		binding := &o.c.consumers[index]
		binding.handle = &countedConsumeContext{ConsumeContext: binding.handle, count: o.count}
		o.count("consumer bind")
	}
	return nil
}

func (o *suiteOwner) Stop(ctx context.Context) error { return o.c.Stop(ctx) }

func (o *suiteOwner) Observe() lifecycletest.Observation {
	o.mu.Lock()
	calls := maps.Clone(o.calls)
	o.mu.Unlock()

	// The suite observes after a Start or Stop has returned to it, so these reads need no
	// lock; only a call that overran its bound is observed while it still runs, and that
	// check has already failed.
	c := o.c
	var held []string
	if c.cancel != nil {
		held = append(held, "runtime cancel")
	}
	if c.ingestSubmitCancel != nil {
		held = append(held, "ingest submit cancel")
	}
	if c.ingestPoolCancel != nil {
		held = append(held, "ingest pool cancel")
	}
	if c.ingestPool != nil {
		held = append(held, "ingest pool lanes")
	}
	if len(c.consumers) > 0 {
		held = append(held, "consumers")
	}
	if len(c.subscriptions) > 0 {
		held = append(held, "request subscriptions")
	}
	// The status loop is listed while its goroutine runs: statusDone is set and not yet
	// closed. A set but closed channel means the loop has returned, which is not held
	// work, so a loop that ends with the Start context is not reported as running.
	if c.statusDone != nil && !isClosed(c.statusDone) {
		held = append(held, "status loop")
	}
	if c.statusPublisher != nil {
		held = append(held, "status publisher")
	}
	if c.entityCache != nil {
		held = append(held, "entity cache")
	}
	if len(c.boundConsumerSnapshot()) > 0 {
		held = append(held, "readiness bound consumers")
	}
	return lifecycletest.Observation{Unresolved: held, Calls: calls}
}

// isClosed reports whether done has been closed, without waiting.
func isClosed(done <-chan struct{}) bool {
	select {
	case <-done:
		return true
	default:
		return false
	}
}

// countedConsumeContext counts the cleanup calls the Component makes on one consumer.
type countedConsumeContext struct {
	jetstream.ConsumeContext
	count func(string)
}

func (h *countedConsumeContext) Drain() {
	h.count("consumer drain")
	h.ConsumeContext.Drain()
}

func (h *countedConsumeContext) Stop() {
	h.count("consumer stop")
	h.ConsumeContext.Stop()
}
