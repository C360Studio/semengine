package objectstore

import (
	"context"

	"github.com/c360studio/semengine/internal/harness/lifecycletest"
)

// suiteOwner adapts Component to the lifecycle suite (lifecycle-suite, "Observe adapter
// contract"). Unresolved lists, by a stable name each, what a started Component holds and what a
// cleanup that succeeded clears: the run context's cancel, the Store, the core-NATS
// subscriptions, the JetStream consumers, and the input check while its goroutine runs.
//
// Calls is empty: the Component has no acquisition seam to count through (design D8), so the
// suite's "a no-op made no call" checks compare empty maps, and what a call left held is read
// from Unresolved.
type suiteOwner struct{ c *Component }

func (o suiteOwner) Start(ctx context.Context) error { return o.c.Start(ctx) }
func (o suiteOwner) Stop(ctx context.Context) error  { return o.c.Stop(ctx) }

// Observe reads the handles start records. The suite observes after a Start or Stop returned to
// it, and the input check touches none of these fields, so the reads need no lock.
func (o suiteOwner) Observe() lifecycletest.Observation {
	c := o.c
	var held []string
	if c.cancel != nil {
		held = append(held, "run cancel")
	}
	if c.store.Load() != nil {
		held = append(held, "store")
	}
	if len(c.subscriptions) > 0 {
		held = append(held, "core subscriptions")
	}
	if len(c.consumers) > 0 {
		held = append(held, "consumers")
	}
	// Listed while its goroutine runs: a closed checkDone means the check has returned, which
	// is not held work, so a check that ended with the Start context is reported gone.
	if c.checkDone != nil && !isClosed(c.checkDone) {
		held = append(held, "input check")
	}
	return lifecycletest.Observation{Unresolved: held}
}
