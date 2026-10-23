package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/internal/harness/lifecycletest"
	"github.com/c360studio/semengine/pkg/errs"
)

// suiteStorageObservabilityConfig is the default configuration: the suite judges the lifecycle,
// which no setting changes.
const suiteStorageObservabilityConfig = `{}`

// storageObservabilityOwner adapts the storage observability service to the lifecycle suite
// (lifecycle-suite, "Observe adapter contract"; design D2, D7 U-a). A started service holds its base
// service and its two loops, the inventory collector's and the report consumer's. Observe lists
// "base-status=<status>" while the base service is neither stopped nor starting, and "loops" while a
// loop goroutine has not returned, read from s.loopsDone, the channel Stop's join waits on. It reads
// the base service's status through its accessor and adds nothing to the service (#186, #190).
type storageObservabilityOwner struct {
	s *StorageObservabilityService
}

func (o *storageObservabilityOwner) Start(ctx context.Context) error { return o.s.Start(ctx) }
func (o *storageObservabilityOwner) Stop(ctx context.Context) error  { return o.s.Stop(ctx) }

func (o *storageObservabilityOwner) Observe() lifecycletest.Observation {
	var held []string
	if status := o.s.Status(); status != StatusStopped && status != StatusStarting {
		held = append(held, "base-status="+status.String())
	}
	if done := o.s.loopsDone; done != nil && !isClosed(done) {
		held = append(held, "loops")
	}
	return lifecycletest.Observation{Unresolved: held}
}

// TestStorageObservabilityPassesTheLifecycleSuite runs the lifecycle suite on the storage
// observability service (ADR-113; design D2; the pin failed StartContextEndStopsNothing, probe § 3).
// Every service is built through NewStorageObservabilityService over a client that never connects:
// Start does no I/O, and the loops retry an unreadable account on their own schedule (design D2,
// "Startup work"). The must-fail owner is a service stopped before Start, whose Start is refused
// (base_lifecycle_test.go's precedent). The service is one-shot: no restart is promised.
//
// spec: service-lifecycle / Storage observability's Start bounds Start; Stop is the only stop
func TestStorageObservabilityPassesTheLifecycleSuite(t *testing.T) {
	newOwner := func() *storageObservabilityOwner {
		s, _ := newTestStorageObservability(t, newUnconnectedClient(t), suiteStorageObservabilityConfig, nil)
		return &storageObservabilityOwner{s: s}
	}
	factory := func() lifecycletest.Owner { return newOwner() }
	mustFail := func() lifecycletest.Owner {
		owner := newOwner()
		require.NoError(t, owner.Stop(stopContext(t)))
		return owner
	}
	lifecycletest.Run(t, factory, mustFail, lifecycletest.Promise{})

	// lifecycle-suite, "Started owner reports its handles": the suite judges only what is left
	// after a Stop, so an adapter that never reported the loops would pass it.
	t.Run("StartedOwnerReportsItsHandles", func(t *testing.T) {
		owner := newOwner()
		require.NoError(t, owner.Start(t.Context()))
		require.Equal(t, []string{"base-status=running", "loops"}, owner.Observe().Unresolved)
		require.NoError(t, owner.Stop(stopContext(t)))
		require.Empty(t, owner.Observe().Unresolved, "a Stop that returned nil left a handle")
	})

	// design D2, "Refusals": a Start after any Stop is refused with errs.ErrAlreadyStarted and
	// holds nothing. The suite's FailedStartHoldsNothing accepts any refusal.
	t.Run("StartAfterStopIsRefused", func(t *testing.T) {
		owner := newOwner()
		require.NoError(t, owner.Stop(stopContext(t)))
		require.ErrorIs(t, owner.Start(t.Context()), errs.ErrAlreadyStarted)
		require.Empty(t, owner.Observe().Unresolved)
	})
}
