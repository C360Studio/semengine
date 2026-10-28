package graphingest

import (
	"context"

	"github.com/c360studio/semengine/internal/harness/lifecycletest"
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
// Calls counts the consumers a started Component holds ("consumer bind") and the Drain
// calls cleanup has issued on them ("consumer drain", each binding's drainIssued): after a
// Start that returned nil, suiteOwner.Start keeps the Component's bindings, which cleanup marks in
// place
// (storage/objectstore reads drainIssued the same way). The handles are not wrapped:
// cleanup waits for them through natsclient's WaitConsumerHandlers, which refuses a handle
// its client did not return (#441). A failed Start's binds and its rollback's drains are
// not counted. No production code calls Stop on a consumer handle, so none is counted.
// Request subscriptions, the pool's Shutdown and the cache's Close are not counted; their
// handles are listed under Unresolved until a cleanup succeeds.
type suiteOwner struct {
	c     *Component
	bound []graphIngestConsumerBinding
}

func newSuiteOwner(c *Component) *suiteOwner {
	return &suiteOwner{c: c}
}

// Start starts the Component and, when Start returned nil, keeps the consumer bindings it
// holds. Start has returned, so nothing else writes c.consumers until Stop, which marks
// these bindings in place and clears c.consumers only once its cleanup succeeded.
func (o *suiteOwner) Start(ctx context.Context) error {
	if err := o.c.Start(ctx); err != nil {
		return err
	}
	o.bound = o.c.consumers
	return nil
}

func (o *suiteOwner) Stop(ctx context.Context) error { return o.c.Stop(ctx) }

func (o *suiteOwner) Observe() lifecycletest.Observation {
	calls := map[string]int{}
	if len(o.bound) > 0 {
		calls["consumer bind"] = len(o.bound)
	}
	for _, binding := range o.bound {
		if binding.drainIssued {
			calls["consumer drain"]++
		}
	}

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
