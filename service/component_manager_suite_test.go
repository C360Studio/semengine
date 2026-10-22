package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/internal/harness/lifecycletest"
	"github.com/c360studio/semengine/internal/lifecycleguard"
)

// errSuiteComponentStart is the error the must-fail composition's failing component returns from
// its Start.
var errSuiteComponentStart = errors.New("suite component Start failed")

// suiteComponent is a lifecycle component written to the carried contract
// (component.LifecycleComponent; ADR-113): its Start launches work under a run context of its own,
// which outlives Start's context, and its Stop ends that context and waits for the work to return.
// Its lifecycle state is a lifecycleguard.Guard, as a ported component's is. A suiteComponent with
// startErr set fails its Start before launching anything.
type suiteComponent struct {
	baseDiscoverable
	name     string
	startErr error

	guard  lifecycleguard.Guard
	starts atomic.Int64
	stops  atomic.Int64

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{} // closed when the work returns
}

func (c *suiteComponent) Initialize() error { return nil }

func (c *suiteComponent) Start(ctx context.Context) error {
	c.starts.Add(1)
	return c.guard.Start(ctx, c.start, c.cleanup)
}

func (c *suiteComponent) Stop(ctx context.Context) error {
	c.stops.Add(1)
	return c.guard.Stop(ctx)
}

func (c *suiteComponent) start(ctx context.Context) error {
	if c.startErr != nil {
		return c.startErr
	}
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	done := make(chan struct{})
	c.mu.Lock()
	c.cancel, c.done = cancel, done
	c.mu.Unlock()
	go func() {
		defer close(done)
		<-runCtx.Done()
	}()
	return nil
}

func (c *suiteComponent) cleanup(ctx context.Context) error {
	c.mu.Lock()
	cancel, done := c.cancel, c.done
	c.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// working reports whether the component's work runs.
func (c *suiteComponent) working() bool {
	c.mu.Lock()
	done := c.done
	c.mu.Unlock()
	return done != nil && !isClosed(done)
}

// componentManagerOwner adapts the component manager to the lifecycle suite (lifecycle-suite,
// "Observe adapter contract"; design D2). A started manager holds its base service, its
// supervisor (the health publisher and the override-expiry reporter, which return before the
// supervisor's done channel closes, design D4) and its components' work. Observe lists
// "base-status=<status>" while the base service is not stopped, "supervisor" while
// cm.supervisorDone is open, and "component-work:<name>" while that component's work runs; it
// reads the base service's status through its accessor and adds nothing to the ComponentManager
// (#186, #190).
//
// Calls counts each component's Start and Stop calls, "start:<name>" and "stop:<name>": the
// manager makes those calls, so the suite's no-op checks (a refused second Start, a repeated
// Stop, a Stop after a failed Start's rollback) hold the manager to calling no component.
type componentManagerOwner struct {
	cm         *ComponentManager
	components []*suiteComponent
}

func (o *componentManagerOwner) Start(ctx context.Context) error { return o.cm.Start(ctx) }
func (o *componentManagerOwner) Stop(ctx context.Context) error  { return o.cm.Stop(ctx) }

func (o *componentManagerOwner) Observe() lifecycletest.Observation {
	var held []string
	if status := o.cm.Status(); status != StatusStopped {
		held = append(held, "base-status="+status.String())
	}
	if done := o.cm.supervisorDone; done != nil && !isClosed(done) {
		held = append(held, "supervisor")
	}
	calls := make(map[string]int, 2*len(o.components))
	for _, c := range o.components {
		if c.working() {
			held = append(held, "component-work:"+c.name)
		}
		calls["start:"+c.name] = int(c.starts.Load())
		calls["stop:"+c.name] = int(c.stops.Load())
	}
	return lifecycletest.Observation{Unresolved: held, Calls: calls}
}

// newSuiteComponentManager composes one working component and, when failing is set, a second
// whose Start fails, so the failed Start must roll back the working one.
func newSuiteComponentManager(t *testing.T, failing bool) *componentManagerOwner {
	t.Helper()
	components := []*suiteComponent{{name: "suite-work"}}
	if failing {
		components = append(components, &suiteComponent{name: "suite-fails", startErr: errSuiteComponentStart})
	}
	composed := make([]testComponent, 0, len(components))
	for _, c := range components {
		composed = append(composed, testComponent{name: c.name, instance: c})
	}
	return &componentManagerOwner{cm: newTestComponentManager(t, composed...), components: components}
}

// TestComponentManagerPassesTheLifecycleSuite runs the lifecycle suite on the component manager
// (ADR-113; design D2; the pin failed StartContextEndStopsNothing, sweep § 3). The must-fail owner
// is a composition one of whose two components fails its Start. The manager is one-shot: no
// restart is promised.
//
// spec: service-lifecycle / The component manager's Start bounds Start
func TestComponentManagerPassesTheLifecycleSuite(t *testing.T) {
	factory := func() lifecycletest.Owner { return newSuiteComponentManager(t, false) }
	mustFail := func() lifecycletest.Owner { return newSuiteComponentManager(t, true) }
	lifecycletest.Run(t, factory, mustFail, lifecycletest.Promise{})

	// lifecycle-suite, "Started owner reports its handles": the suite judges only what is left
	// after a Stop, so an adapter that never reported a handle would pass it.
	t.Run("StartedOwnerReportsItsHandles", func(t *testing.T) {
		owner := newSuiteComponentManager(t, false)
		require.NoError(t, owner.Start(t.Context()))
		require.Equal(t, []string{"base-status=running", "supervisor", "component-work:suite-work"},
			owner.Observe().Unresolved)
		require.NoError(t, owner.Stop(stopContext(t)))
		require.Empty(t, owner.Observe().Unresolved, "a Stop that returned nil left a handle")
		require.Equal(t, map[string]int{"start:suite-work": 1, "stop:suite-work": 1}, owner.Observe().Calls)
	})

	// The must-fail composition's Start fails naming the failing component, and its rollback
	// stops both components, the failing one included (design D2): the suite's
	// FailedStartHoldsNothing reads only that nothing is held, not that the working component had
	// started and was stopped.
	t.Run("FailedStartRollsBackTheStartedComponent", func(t *testing.T) {
		owner := newSuiteComponentManager(t, true)
		err := owner.Start(t.Context())
		require.ErrorIs(t, err, errSuiteComponentStart)
		require.ErrorContains(t, err, `component "suite-fails"`)
		require.Empty(t, owner.Observe().Unresolved)
		calls := owner.Observe().Calls
		require.Equal(t, 1, calls["start:suite-work"], "the working component was not started")
		require.Equal(t, 1, calls["stop:suite-work"], "the rollback did not stop the started component")
		require.Equal(t, 1, calls["stop:suite-fails"], "the rollback did not stop the component whose Start failed")
	})
}
