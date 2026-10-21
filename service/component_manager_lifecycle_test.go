package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/storage"
	"github.com/c360studio/semengine/storage/storeregistry"
	"github.com/c360studio/semengine/types"
)

// The component manager's Start and Stop, from the pin's lifecycle_context_contract_test.go:177-532
// and the tests of ADR-113's option A (design D2, "The component's own Start context"). The pin's
// tests built struct-literal managers and read their lifecycle fields; these build every manager
// through NewComponentManager and assert what Start and Stop return, what GetComponentStatus reports
// and what each component recorded (design D7). The pin's newControlledGenerationComponent,
// newStartedSupervisorManager and rollbackStoreComponent had no caller and are not carried.

// lifecycleCallLog records the name of each component whose Stop is called, in call order, across
// the components of one test.
type lifecycleCallLog struct {
	mu    sync.Mutex
	stops []string
}

func (l *lifecycleCallLog) stopped(name string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.stops = append(l.stops, name)
}

// stopOrder returns a copy of the names recorded so far.
func (l *lifecycleCallLog) stopOrder() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.stops...)
}

// lifecycleTestComponent is a lifecycle component a test scripts. Its Start records the context it
// was given, closes startEntered and waits for releaseStart when holdStart armed them, then returns
// startErr. Its Stop records the call (and on log, when set), runs onStop when set, whose non-nil
// error it returns, and otherwise returns stopErrs[n-1] on its n-th call, nil past the end.
type lifecycleTestComponent struct {
	baseDiscoverable
	name     string
	log      *lifecycleCallLog
	startErr error
	stopErrs []error
	onStop   func(ctx context.Context, call int64) error

	startEntered chan struct{}
	releaseStart chan struct{}

	starts atomic.Int64
	stops  atomic.Int64

	mu                sync.Mutex
	startCtx          context.Context
	startCtxErrAtStop error
}

func newLifecycleTestComponent(name string, log *lifecycleCallLog) *lifecycleTestComponent {
	return &lifecycleTestComponent{name: name, log: log}
}

// holdStart makes the component's Start close startEntered and wait until the test closes
// releaseStart.
func (c *lifecycleTestComponent) holdStart() {
	c.startEntered, c.releaseStart = make(chan struct{}), make(chan struct{})
}

func (c *lifecycleTestComponent) testComponent() testComponent {
	return testComponent{name: c.name, instance: c}
}

func (*lifecycleTestComponent) Initialize() error { return nil }

func (c *lifecycleTestComponent) Start(ctx context.Context) error {
	c.starts.Add(1)
	c.mu.Lock()
	c.startCtx = ctx
	c.mu.Unlock()
	if c.startEntered != nil {
		close(c.startEntered)
		<-c.releaseStart
	}
	return c.startErr
}

func (c *lifecycleTestComponent) Stop(ctx context.Context) error {
	call := c.stops.Add(1)
	c.mu.Lock()
	if c.startCtx != nil {
		c.startCtxErrAtStop = c.startCtx.Err()
	}
	c.mu.Unlock()
	if c.log != nil {
		c.log.stopped(c.name)
	}
	if c.onStop != nil {
		if err := c.onStop(ctx, call); err != nil {
			return err
		}
	}
	if int(call) <= len(c.stopErrs) {
		return c.stopErrs[call-1]
	}
	return nil
}

// startContext returns the context the component's last Start was given.
func (c *lifecycleTestComponent) startContext() context.Context {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.startCtx
}

// startContextErrAtStop returns what the Start context's Err reported when Stop was last called.
func (c *lifecycleTestComponent) startContextErrAtStop() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.startCtxErrAtStop
}

var _ component.LifecycleComponent = (*lifecycleTestComponent)(nil)

// lifecycleTestStore is a store that holds nothing; a provider hands it out so the store registry
// has an entry to hold.
type lifecycleTestStore struct{}

func (lifecycleTestStore) Put(context.Context, string, []byte) error      { return nil }
func (lifecycleTestStore) Get(context.Context, string) ([]byte, error)    { return nil, nil }
func (lifecycleTestStore) List(context.Context, string) ([]string, error) { return nil, nil }
func (lifecycleTestStore) Delete(context.Context, string) error           { return nil }
func (lifecycleTestStore) Open(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(nil)), nil
}

// lifecycleStoreProvider is a lifecycle component that provides one store under its own name. Its
// factory keeps the store registry the manager hands every component (component.Dependencies), so
// the test reads the registry the manager fills.
type lifecycleStoreProvider struct {
	*lifecycleTestComponent
	storeRegistry *storeregistry.Registry
}

func (p *lifecycleStoreProvider) ProvidedStores() map[string]storage.StreamableStore {
	return map[string]storage.StreamableStore{p.name: lifecycleTestStore{}}
}

var _ component.StoreProvider = (*lifecycleStoreProvider)(nil)

// newLifecycleStoreProviderManager builds a component manager composing provider alone, through
// NewComponentManager, with a factory that keeps the manager's store registry on provider.
func newLifecycleStoreProviderManager(t *testing.T, provider *lifecycleStoreProvider) *ComponentManager {
	t.Helper()
	registry := component.NewRegistry()
	require.NoError(t, registry.Register(component.Registration{
		Name: provider.name, Type: types.ComponentTypeProcessor,
		Factory: func(_ json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
			provider.storeRegistry = deps.StoreRegistry
			return provider, nil
		},
		Ports: func(json.RawMessage, string) (component.PortConfig, error) {
			return component.PortConfigFrom(nil, nil), nil
		},
	}))
	built, err := NewComponentManager(nil, testComponentDependencies(t, registry, provider.name))
	require.NoError(t, err)
	cm, ok := built.(*ComponentManager)
	require.True(t, ok, "NewComponentManager returned %T, want *ComponentManager", built)
	require.NotNil(t, provider.storeRegistry, "the manager handed its components no store registry")
	return cm
}

// requireComponentStates fails unless GetComponentStatus reports want for each named component.
func requireComponentStates(t *testing.T, cm *ComponentManager, want component.State, names ...string) {
	t.Helper()
	statuses := cm.GetComponentStatus()
	for _, name := range names {
		require.Equal(t, want, statuses[name].State, "component %q", name)
	}
}

// TestComponentStartContextIsTheManagersStartContext: a component's Start gets the context the
// manager's Start got, which the manager never ends (design D2, Q-A option A; ADR-113). It replaces
// the pin's per-component child context, which the manager cancelled in Stop.
//
// spec: service-lifecycle / The component manager's Start bounds Start
func TestComponentStartContextIsTheManagersStartContext(t *testing.T) {
	t.Run("LiveAfterStopUntilTheCallerEndsIt", func(t *testing.T) {
		c := newLifecycleTestComponent("start-context", nil)
		cm := newTestComponentManager(t, c.testComponent())
		startCtx, cancelStart := context.WithCancel(t.Context())
		defer cancelStart()

		require.NoError(t, cm.Start(startCtx))
		recorded := c.startContext()
		require.NotNil(t, recorded, "the manager's Start did not start the component")
		require.NoError(t, recorded.Err(), "the component's Start context ended when the manager's Start returned")

		require.NoError(t, cm.Stop(stopContext(t)))
		require.Equal(t, int64(1), c.stops.Load())
		require.NoError(t, recorded.Err(), "the manager's Stop ended the component's Start context")

		cancelStart()
		waitForSignal(t, recorded.Done(), "the component's Start context to end with the caller's")
		require.ErrorIs(t, recorded.Err(), context.Canceled)
	})

	t.Run("EndingItStopsNothing", func(t *testing.T) {
		c := newLifecycleTestComponent("start-context", nil)
		cm := newTestComponentManager(t, c.testComponent())
		startCtx, cancelStart := context.WithCancel(t.Context())
		defer cancelStart()

		require.NoError(t, cm.Start(startCtx))
		cancelStart()
		waitForSignal(t, c.startContext().Done(), "the component's Start context to end with the caller's")

		require.Zero(t, c.stops.Load(), "ending the Start context stopped the component")
		requireComponentStates(t, cm, component.StateStarted, c.name)
		require.Equal(t, StatusRunning, cm.Status(), "ending the Start context stopped the manager")

		require.NoError(t, cm.Stop(stopContext(t)))
		require.Equal(t, int64(1), c.stops.Load())
		requireComponentStates(t, cm, component.StateStopped, c.name)
	})
}

// TestStopDuringStartWaitsForComponentStarts: a Stop that arrives while a component's Start is in
// flight waits for every Start to return, ends no Start context, and then stops each component once
// in reverse start order (design D2, Q-A option A). It replaces the pin's
// TestComponentManagerStopDuringStartCancelsAndPreventsLateStoreCommit, whose Stop cancelled the
// Starts in flight. The synctest bubble makes "Stop has not returned" exact: synctest.Wait returns
// only once the Stop goroutine is blocked.
//
// spec: service-lifecycle / The component manager's Start bounds Start
func TestStopDuringStartWaitsForComponentStarts(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := &lifecycleCallLog{}
		quick := newLifecycleTestComponent("stop-wait-a", log)
		held := newLifecycleTestComponent("stop-wait-b", log)
		held.holdStart()
		cm := newTestComponentManager(t, quick.testComponent(), held.testComponent())

		started := make(chan error, 1)
		go func() { started <- cm.Start(t.Context()) }()
		<-held.startEntered

		stopCtx := stopContext(t)
		stopped := make(chan error, 1)
		go func() { stopped <- cm.Stop(stopCtx) }()
		synctest.Wait()

		select {
		case err := <-stopped:
			t.Fatalf("Stop returned %v while a component's Start was still in flight", err)
		case err := <-started:
			t.Fatalf("Start returned %v while a component's Start was still in flight", err)
		default:
		}
		require.NoError(t, held.startContext().Err(), "Stop ended the context of a Start in flight")
		require.Empty(t, log.stopOrder(), "Stop stopped a component before every Start returned")

		close(held.releaseStart)
		require.NoError(t, <-started)
		require.NoError(t, <-stopped)
		require.Equal(t, []string{"stop-wait-b", "stop-wait-a"}, log.stopOrder(),
			"Stop did not stop each component once in reverse start order")
		requireComponentStates(t, cm, component.StateStopped, quick.name, held.name)

		require.NoError(t, cm.Stop(stopContext(t)))
		require.Equal(t, []string{"stop-wait-b", "stop-wait-a"}, log.stopOrder(),
			"a Stop after one that returned nil called a component")
	})
}

// TestStopDuringAFailingStartStopsWhatTheRollbackLeft: a Stop waits for a held Start that then
// fails; the failed Start rolls back first, inside Start, with one component's rollback Stop armed
// to fail once, and the waiting Stop then stops only what the rollback left (design D2, Q-A option
// A).
//
// spec: service-lifecycle / The component manager's Start bounds Start
func TestStopDuringAFailingStartStopsWhatTheRollbackLeft(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		errHeldStart := errors.New("held Start failed")
		errArmedStop := errors.New("rollback Stop failed")
		log := &lifecycleCallLog{}
		armed := newLifecycleTestComponent("rollback-a", log)
		armed.stopErrs = []error{errArmedStop}
		held := newLifecycleTestComponent("rollback-b", log)
		held.startErr = errHeldStart
		held.holdStart()
		cm := newTestComponentManager(t, armed.testComponent(), held.testComponent())

		started := make(chan error, 1)
		go func() { started <- cm.Start(t.Context()) }()
		<-held.startEntered

		stopCtx := stopContext(t)
		stopped := make(chan error, 1)
		go func() { stopped <- cm.Stop(stopCtx) }()
		synctest.Wait()

		select {
		case err := <-stopped:
			t.Fatalf("Stop returned %v while a component's Start was still in flight", err)
		default:
		}
		require.Empty(t, log.stopOrder(), "Stop stopped a component before every Start returned")

		close(held.releaseStart)
		err := <-started
		require.ErrorIs(t, err, errHeldStart)
		require.ErrorIs(t, err, errArmedStop, "Start did not report the rollback's failure")
		require.ErrorContains(t, err, `component "rollback-b"`)

		require.NoError(t, <-stopped)
		// The rollback stops both launched components in reverse launch order (by name within the
		// phase), the failing one included; the waiting Stop then stops only the one whose rollback
		// Stop failed.
		require.Equal(t, []string{"rollback-b", "rollback-a", "rollback-a"}, log.stopOrder())
		require.Equal(t, int64(1), held.stops.Load(), "the waiting Stop called a component the rollback had stopped")
		require.Equal(t, int64(2), armed.stops.Load(), "the waiting Stop did not stop what the rollback left")
		requireComponentStates(t, cm, component.StateStopped, armed.name, held.name)

		require.NoError(t, cm.Stop(stopContext(t)))
		require.Len(t, log.stopOrder(), 3, "a Stop after one that returned nil called a component")
	})
}

// TestStopRetriesOnlyComponentsThatDidNotStop: a Stop that fails leaves for the next Stop every
// component whose Stop failed or was not reached because the Stop context ended, and never calls
// again a component whose Stop returned nil (design D2; sweep C6). Start order is by name, so the
// Stop order is retry-c, retry-b, retry-a; retry-b's first Stop ends the Stop context and fails.
//
// spec: service-lifecycle / The component manager's Start bounds Start
func TestStopRetriesOnlyComponentsThatDidNotStop(t *testing.T) {
	errRetryB := errors.New("retry-b Stop failed")
	first, endFirst := context.WithCancel(stopContext(t))
	defer endFirst()

	a := newLifecycleTestComponent("retry-a", nil)
	a.onStop = func(ctx context.Context, _ int64) error { return ctx.Err() } // honours its Stop context
	b := newLifecycleTestComponent("retry-b", nil)
	b.stopErrs = []error{errRetryB}
	b.onStop = func(_ context.Context, call int64) error {
		if call == 1 {
			endFirst()
		}
		return nil
	}
	c := newLifecycleTestComponent("retry-c", nil)
	cm := newTestComponentManager(t, a.testComponent(), b.testComponent(), c.testComponent())
	startTestComponentManager(t, cm)

	err := cm.Stop(first)
	require.ErrorIs(t, err, errRetryB, "the failed component Stop was not reported")
	require.ErrorIs(t, err, context.Canceled, "the ended Stop context was not reported")
	statuses := cm.GetComponentStatus()
	require.Equal(t, component.StateStopped, statuses[c.name].State)
	require.NoError(t, statuses[c.name].LastError)
	require.Equal(t, component.StateFailed, statuses[b.name].State)
	require.ErrorIs(t, statuses[b.name].LastError, errRetryB)
	require.NotEqual(t, component.StateStopped, statuses[a.name].State,
		"a component the ended Stop context did not stop reads stopped")
	require.Equal(t, int64(1), c.stops.Load())
	require.Equal(t, int64(1), b.stops.Load())
	aStopsBeforeRetry := a.stops.Load()

	require.NoError(t, cm.Stop(stopContext(t)))
	require.Equal(t, int64(1), c.stops.Load(), "the retry called a component whose Stop had returned nil")
	require.Equal(t, int64(2), b.stops.Load(), "the retry did not stop the component whose Stop failed")
	require.Equal(t, aStopsBeforeRetry+1, a.stops.Load(),
		"the retry did not stop the component the ended Stop context left")
	statuses = cm.GetComponentStatus()
	for _, name := range []string{a.name, b.name, c.name} {
		require.Equal(t, component.StateStopped, statuses[name].State, "component %q", name)
		require.NoError(t, statuses[name].LastError, "component %q", name)
	}

	require.NoError(t, cm.Stop(stopContext(t)))
	require.Equal(t, []int64{aStopsBeforeRetry + 1, 2, 1}, []int64{a.stops.Load(), b.stops.Load(), c.stops.Load()},
		"a Stop after one that returned nil called a component")
}

// TestComponentManagerCallsComponentStopBeforeRuntimeCancellation, flipped by Q-A option A (design
// D7): the component's Stop runs while its Start context is live, as at the pin, and the manager's
// Stop no longer ends that context afterwards. A repeated Stop calls nothing, and a Start after Stop
// is refused.
//
// spec: service-lifecycle / The component manager's Start bounds Start
func TestComponentManagerCallsComponentStopBeforeRuntimeCancellation(t *testing.T) {
	c := newLifecycleTestComponent("live-stop", nil)
	cm := newTestComponentManager(t, c.testComponent())
	require.NoError(t, cm.Start(t.Context()))

	require.NoError(t, cm.Stop(stopContext(t)))
	require.Equal(t, int64(1), c.stops.Load())
	require.NoError(t, c.startContextErrAtStop(), "the component's Stop ran after its Start context ended")
	require.NoError(t, c.startContext().Err(), "the manager's Stop ended the component's Start context")

	require.NoError(t, cm.Stop(stopContext(t)))
	require.Equal(t, int64(1), c.stops.Load(), "a repeated Stop called the component")
	require.ErrorIs(t, cm.Start(t.Context()), errs.ErrAlreadyStarted)
	require.Equal(t, int64(1), c.starts.Load(), "a refused Start started the component")
}

// TestComponentManagerFailedStartRollsBackSynchronouslyAndDoesNotReplay, flipped by Q-A option A
// (design D7): the failed Start's rollback has stopped the component by the time Start returns, its
// Stop sees the Start context still live (the pin's saw it cancelled), and a later Stop does not run
// the rollback again.
//
// spec: service-lifecycle / The component manager's Start bounds Start
func TestComponentManagerFailedStartRollsBackSynchronouslyAndDoesNotReplay(t *testing.T) {
	errPartialStart := errors.New("partial start failed")
	c := newLifecycleTestComponent("partial", nil)
	c.startErr = errPartialStart
	cm := newTestComponentManager(t, c.testComponent())

	require.ErrorIs(t, cm.Start(t.Context()), errPartialStart)
	require.Equal(t, int64(1), c.stops.Load(), "the rollback had not stopped the component when Start returned")
	require.NoError(t, c.startContextErrAtStop(), "the rollback's Stop saw the Start context ended")

	require.NoError(t, cm.Stop(stopContext(t)))
	require.Equal(t, int64(1), c.stops.Load(), "Stop ran the succeeded rollback again")
}

// TestComponentManagerFailedStartCleanupFailureRetainsAuthorityForLaterStop: a failed Start whose
// rollback Stop fails returns both errors and leaves that component for the next Stop, which stops
// it and returns nil; a Start in between is refused (design D2; lifecycle-suite, "Failed start whose
// own cleanup fails"). The pin read cleanupPending and lifecycleTerminal; this reads outcomes.
//
// spec: service-lifecycle / The component manager's Start bounds Start
func TestComponentManagerFailedStartCleanupFailureRetainsAuthorityForLaterStop(t *testing.T) {
	errPartialStart := errors.New("partial start failed")
	errPartialCleanup := errors.New("partial cleanup failed")
	c := newLifecycleTestComponent("partial", nil)
	c.startErr = errPartialStart
	c.stopErrs = []error{errPartialCleanup}
	cm := newTestComponentManager(t, c.testComponent())

	err := cm.Start(t.Context())
	require.ErrorIs(t, err, errPartialStart)
	require.ErrorIs(t, err, errPartialCleanup)
	require.Equal(t, int64(1), c.stops.Load(), "the rollback did not stop the component")
	requireComponentStates(t, cm, component.StateFailed, c.name)

	require.ErrorIs(t, cm.Start(t.Context()), errs.ErrAlreadyStarted)
	require.Equal(t, int64(1), c.starts.Load(), "a refused Start started the component")
	require.Equal(t, int64(1), c.stops.Load(), "a refused Start stopped the component")

	require.NoError(t, cm.Stop(stopContext(t)))
	require.Equal(t, int64(2), c.stops.Load(), "Stop did not stop what the failed rollback left")
	statuses := cm.GetComponentStatus()
	require.Equal(t, component.StateStopped, statuses[c.name].State)
	require.NoError(t, statuses[c.name].LastError)

	require.NoError(t, cm.Stop(stopContext(t)))
	require.Equal(t, int64(2), c.stops.Load(), "a Stop after one that returned nil called the component")
}

// TestComponentManagerRunningStopDeregistersStoreBeforeLiveChildStop: a running provider's stores
// leave the store registry before its Stop is called, and its Start context is still live then
// (design D2-D3; Q-A option A: the pin cancelled it after the Stop).
//
// spec: service-lifecycle / Providers start before consumers and own their stores
func TestComponentManagerRunningStopDeregistersStoreBeforeLiveChildStop(t *testing.T) {
	errStoreStillRegistered := errors.New("store still registered when the provider's Stop was called")
	provider := &lifecycleStoreProvider{lifecycleTestComponent: newLifecycleTestComponent("arbitrated", nil)}
	provider.onStop = func(context.Context, int64) error {
		if _, registered := provider.storeRegistry.Streamable(provider.name); registered {
			return errStoreStillRegistered
		}
		return nil
	}
	cm := newLifecycleStoreProviderManager(t, provider)

	require.NoError(t, cm.Start(t.Context()))
	_, registered := provider.storeRegistry.Streamable(provider.name)
	require.True(t, registered, "the started provider's store is not registered")

	require.NoError(t, cm.Stop(stopContext(t)))
	require.Equal(t, int64(1), provider.stops.Load())
	require.NoError(t, provider.startContextErrAtStop(), "the provider's Stop ran after its Start context ended")
	_, registered = provider.storeRegistry.Streamable(provider.name)
	require.False(t, registered, "the stopped provider's store is still registered")

	require.NoError(t, cm.Stop(stopContext(t)))
	require.Equal(t, int64(1), provider.stops.Load(), "a Stop after one that returned nil called the provider")
}
