package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/pkg/errs"
)

// The component manager as a lifecycle owner, from the pin's component_manager_owner_test.go. The
// borrow-fence tests (pin :137-156, :180-311) and their helpers are not carried: the fence is gone
// (design D2, "No wait for readers"; D7). The pin's :158-178 is flipped into
// TestComponentHealthAfterStopAsksEachComponent (component_manager_state_test.go). Components are
// lifecycleTestComponent (component_manager_lifecycle_test.go), which stands in for the pin's
// orderedManagerComponent.

// TestComponentManagerNilContextsPreserveLifecycleState: Start and Stop refuse a nil context as
// invalid, and a refusal leaves the manager as it was (design D2). The pin read lifecycleUsed,
// cleanupPending, lifecycleTerminal, startDone and cancel; this reads what a later Start and Stop
// do.
//
// spec: service-lifecycle / The component manager's Start bounds Start
func TestComponentManagerNilContextsPreserveLifecycleState(t *testing.T) {
	var nilCtx context.Context // the nil context is the input under test

	t.Run("RefusalsLeaveTheManagerUnused", func(t *testing.T) {
		c := newLifecycleTestComponent("nil-context", nil)
		cm := newTestComponentManager(t, c.testComponent())

		require.True(t, errs.IsInvalid(cm.Start(nilCtx)), "Start(nil) was not refused as invalid")
		require.True(t, errs.IsInvalid(cm.Stop(nilCtx)), "Stop(nil) was not refused as invalid")
		require.False(t, cm.startupStopBegun(), "a refused Stop began the stop")
		require.Zero(t, c.starts.Load(), "a refused Start started the component")
		require.Zero(t, c.stops.Load(), "a refused Stop stopped the component")

		// The refusals used nothing: the manager still admits its one Start.
		require.NoError(t, cm.Start(t.Context()))
		require.Equal(t, int64(1), c.starts.Load())
		require.NoError(t, cm.Stop(stopContext(t)))
		require.True(t, cm.startupStopBegun(), "Stop did not mark the stop begun")
		require.Equal(t, int64(1), c.stops.Load())
	})

	t.Run("StopBeforeStartUsesTheManager", func(t *testing.T) {
		c := newLifecycleTestComponent("nil-context", nil)
		cm := newTestComponentManager(t, c.testComponent())

		require.NoError(t, cm.Stop(stopContext(t)))
		require.True(t, errs.IsInvalid(cm.Stop(nilCtx)), "Stop(nil) after a Stop was not refused as invalid")
		require.ErrorIs(t, cm.Start(t.Context()), errs.ErrAlreadyStarted, "a Start after Stop was not refused")
		require.Zero(t, c.starts.Load(), "a refused Start started the component")
		require.Zero(t, c.stops.Load(), "a Stop before any Start stopped the component")
	})
}

// TestComponentManagerStopUsesReverseStartOrderAndAggregates: within one phase the start order is
// recorded by name (sweep C4), Stop stops in the reverse of it and joins every component's failure
// (design D2). The pin's second Stop called nothing, a failed Stop being final; here it calls again,
// in the same reverse order, only the components whose Stop failed (sweep C6), and returns nil.
//
// spec: service-lifecycle / The component manager's Start bounds Start
func TestComponentManagerStopUsesReverseStartOrderAndAggregates(t *testing.T) {
	errA := errors.New("stop a")
	errC := errors.New("stop c")
	log := &lifecycleCallLog{}
	a := newLifecycleTestComponent("a", log)
	a.stopErrs = []error{errA}
	b := newLifecycleTestComponent("b", log)
	c := newLifecycleTestComponent("c", log)
	c.stopErrs = []error{errC}
	cm := newTestComponentManager(t, c.testComponent(), a.testComponent(), b.testComponent())
	require.NoError(t, cm.Start(t.Context()))

	order := make(map[string]int)
	require.NoError(t, cm.withComponents(func(managed map[string]*component.ManagedComponent) error {
		for name, mc := range managed {
			order[name] = mc.StartOrder
		}
		return nil
	}))
	require.Len(t, order, 3)
	require.True(t, order["a"] < order["b"] && order["b"] < order["c"],
		"the start order is not recorded by name: %v", order)

	err := cm.Stop(stopContext(t))
	require.ErrorIs(t, err, errA)
	require.ErrorIs(t, err, errC)
	require.Equal(t, []string{"c", "b", "a"}, log.stopOrder(), "Stop did not use the reverse start order")

	require.NoError(t, cm.Stop(stopContext(t)))
	require.Equal(t, []string{"c", "b", "a", "c", "a"}, log.stopOrder(),
		"the retry did not stop exactly the failed components, in reverse start order")

	require.NoError(t, cm.Stop(stopContext(t)))
	require.Len(t, log.stopOrder(), 5, "a Stop after one that returned nil called a component")
}
