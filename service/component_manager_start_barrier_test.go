package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/storage"
)

// The phase tests, from the pin's component_manager_start_barrier_test.go:1-389. The pin built
// struct-literal managers and marked them initialized by hand; here every manager comes from
// NewComponentManager (newTestComponentManager), and the tests assert what Start, Stop and
// GetComponentStatus return and what the components and the store registry saw (design D2, D7).
// The pin's TestPerformDetailedHealthCheck_ReportsFailedComponent (:390-425) moved to
// component_manager_state_test.go as TestComponentManagerHealthNamesEveryFailedComponent.

// barrierTestComponent is a lifecycle component whose Start the phase tests script: it can fail
// with a fixed error, or block until the test releases a gate. startReturned is closed as the
// last action of Start, so a test that observes it knows Start really returned.
type barrierTestComponent struct {
	baseDiscoverable
	name     string
	startErr error

	// entered and release gate a blocking Start. Nil means Start returns at once. entered is
	// closed when Start begins; Start then blocks until release is closed.
	entered chan struct{}
	release chan struct{}

	startReturned chan struct{}
}

func newBarrierTestComponent(name string) *barrierTestComponent {
	return &barrierTestComponent{name: name, startReturned: make(chan struct{})}
}

func (c *barrierTestComponent) Initialize() error          { return nil }
func (c *barrierTestComponent) Stop(context.Context) error { return nil }

func (c *barrierTestComponent) Start(context.Context) error {
	defer close(c.startReturned)
	if c.entered != nil {
		close(c.entered)
		<-c.release
	}
	return c.startErr
}

var _ component.LifecycleComponent = (*barrierTestComponent)(nil)

// barrierStoreProvider is a barrierTestComponent that owns stores, so the manager starts it in
// the provider phase and registers its stores once its Start returns nil.
type barrierStoreProvider struct {
	*barrierTestComponent
	provided map[string]storage.StreamableStore
}

func (p *barrierStoreProvider) ProvidedStores() map[string]storage.StreamableStore {
	return p.provided
}

var _ component.StoreProvider = (*barrierStoreProvider)(nil)

// barrierComposed returns the testComponent that composes c under its own name.
func barrierComposed(c *barrierTestComponent) testComponent {
	return testComponent{name: c.name, instance: c}
}

// barrierComposedProvider returns the testComponent that composes p under its component's name.
func barrierComposedProvider(p *barrierStoreProvider) testComponent {
	return testComponent{name: p.name, instance: p}
}

// barrierStopOnCleanup stops cm when the test ends, under a fresh bounded context, and expects
// nil: Stop after a Start whose rollback succeeded, or after none, returns nil (design D2).
func barrierStopOnCleanup(t *testing.T, cm *ComponentManager) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), stopBound)
		defer cancel()
		assert.NoError(t, cm.Stop(ctx))
	})
}

// barrierStartResult waits for the result of a Start running on another goroutine, failing the
// test, naming description, once stopBound passes.
func barrierStartResult(t *testing.T, done <-chan error, description string) error {
	t.Helper()
	timer := time.NewTimer(stopBound)
	defer timer.Stop()
	select {
	case err := <-done:
		return err
	case <-timer.C:
		t.Fatalf("timed out waiting for %s", description)
		return nil
	}
}

// barrierNotEntered fails the test if entered closes within negativeWindow: a component of a
// later phase that starts while the earlier phase is held.
func barrierNotEntered(t *testing.T, entered <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-entered:
		t.Fatalf("%s", description)
	case <-time.After(negativeWindow):
	}
}

func TestComponentManagerStart_ProvidersRegisterBeforeConcurrentConsumers(t *testing.T) {
	providerRelease := make(chan struct{})
	providerA := newBarrierTestComponent("provider-a")
	providerA.entered = make(chan struct{})
	providerA.release = providerRelease
	providerB := newBarrierTestComponent("provider-b")
	providerB.entered = make(chan struct{})
	providerB.release = providerRelease

	consumerRelease := make(chan struct{})
	consumerA := newBarrierTestComponent("consumer-a")
	consumerA.entered = make(chan struct{})
	consumerA.release = consumerRelease
	consumerB := newBarrierTestComponent("consumer-b")
	consumerB.entered = make(chan struct{})
	consumerB.release = consumerRelease

	cm := newTestComponentManager(t,
		barrierComposedProvider(&barrierStoreProvider{
			barrierTestComponent: providerA,
			provided:             map[string]storage.StreamableStore{"store-a": &fakeStreamable{id: "a"}},
		}),
		barrierComposedProvider(&barrierStoreProvider{
			barrierTestComponent: providerB,
			provided:             map[string]storage.StreamableStore{"store-b": &fakeStreamable{id: "b"}},
		}),
		barrierComposed(consumerA),
		barrierComposed(consumerB),
	)
	barrierStopOnCleanup(t, cm)

	done := make(chan error, 1)
	go func() { done <- cm.Start(t.Context()) }()

	// Both providers are inside Start at once: the provider phase starts its components
	// concurrently.
	waitForSignal(t, providerA.entered, "provider-a to start in the provider phase")
	waitForSignal(t, providerB.entered, "provider-b to start in the provider phase")
	barrierNotEntered(t, consumerA.entered, "consumer-a started before the provider phase completed")
	barrierNotEntered(t, consumerB.entered, "consumer-b started before the provider phase completed")

	close(providerRelease)

	waitForSignal(t, consumerA.entered, "consumer-a to start in the consumer phase")
	waitForSignal(t, consumerB.entered, "consumer-b to start in the consumer phase")
	_, ok := cm.storeRegistry.Streamable("store-a")
	assert.True(t, ok, "provider-a's store was not registered before consumers started")
	_, ok = cm.storeRegistry.Streamable("store-b")
	assert.True(t, ok, "provider-b's store was not registered before consumers started")

	close(consumerRelease)
	require.NoError(t, barrierStartResult(t, done, "Start to return after both phases"))
}

func TestComponentManagerStart_ProviderFailurePreventsConsumerPhase(t *testing.T) {
	providerComp := newBarrierTestComponent("provider")
	providerComp.startErr = errors.New("provider unavailable")
	consumer := newBarrierTestComponent("consumer")
	consumer.entered = make(chan struct{})
	consumer.release = make(chan struct{})

	cm := newTestComponentManager(t,
		barrierComposedProvider(&barrierStoreProvider{
			barrierTestComponent: providerComp,
			provided:             map[string]storage.StreamableStore{"store": &fakeStreamable{id: "store"}},
		}),
		barrierComposed(consumer),
	)
	barrierStopOnCleanup(t, cm)

	err := cm.Start(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), `component "provider"`)
	select {
	case <-consumer.entered:
		t.Fatal("consumer started after the provider phase failed")
	default:
	}
	assert.Equal(t, component.StateInitialized, cm.GetComponentStatus()["consumer"].State)
}

func TestComponentManagerStopAfterProviderFailureDoesNotWaitForUnlaunchedSupervisor(t *testing.T) {
	providerComp := newBarrierTestComponent("provider")
	providerComp.startErr = errors.New("provider unavailable")
	cm := newTestComponentManager(t, barrierComposedProvider(&barrierStoreProvider{
		barrierTestComponent: providerComp,
		provided:             map[string]storage.StreamableStore{"store": &fakeStreamable{id: "store"}},
	}))

	require.Error(t, cm.Start(t.Context()))
	require.NoError(t, cm.Stop(stopContext(t)))
}

func TestComponentManagerStart_DuplicateProviderFailsWithoutStartingConsumers(t *testing.T) {
	providerA := newBarrierTestComponent("provider-a")
	providerB := newBarrierTestComponent("provider-b")
	consumer := newBarrierTestComponent("consumer")
	consumer.entered = make(chan struct{})
	consumer.release = make(chan struct{})

	cm := newTestComponentManager(t,
		barrierComposedProvider(&barrierStoreProvider{
			barrierTestComponent: providerA,
			provided:             map[string]storage.StreamableStore{"shared": &fakeStreamable{id: "a"}},
		}),
		barrierComposedProvider(&barrierStoreProvider{
			barrierTestComponent: providerB,
			provided:             map[string]storage.StreamableStore{"shared": &fakeStreamable{id: "b"}},
		}),
		barrierComposed(consumer),
	)
	barrierStopOnCleanup(t, cm)

	err := cm.Start(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate ownership")
	_, ok := cm.storeRegistry.Streamable("shared")
	assert.False(t, ok, "the failed Start's rollback must deregister the incumbent store")
	select {
	case <-consumer.entered:
		t.Fatal("consumer started after duplicate provider registration failed")
	default:
	}

	status := cm.GetComponentStatus()
	for _, name := range []string{"provider-a", "provider-b"} {
		assert.Equal(t, component.StateStopped, status[name].State,
			"the failed Start's rollback must stop every provider it launched, %s included", name)
	}
}

// TestComponentManagerStart_FailsClosedOnComponentStartError: a component whose Start returns an
// error fails ComponentManager.Start with an error naming the component and wrapping its error,
// and the rollback stops every component it launched.
func TestComponentManagerStart_FailsClosedOnComponentStartError(t *testing.T) {
	boom := errors.New("bucket carries a binding retention")
	failing := newBarrierTestComponent("failing-comp")
	failing.startErr = boom
	healthy := newBarrierTestComponent("healthy-comp")

	cm := newTestComponentManager(t, barrierComposed(failing), barrierComposed(healthy))
	barrierStopOnCleanup(t, cm)

	err := cm.Start(t.Context())
	require.Error(t, err, "a component Start failure must fail ComponentManager.Start")
	assert.Contains(t, err.Error(), `component "failing-comp"`, "the error must name the failed component")
	assert.ErrorIs(t, err, boom, "the component's own error must be wrapped, not replaced")

	status := cm.GetComponentStatus()
	require.Contains(t, status, "failing-comp")
	assert.Equal(t, component.StateStopped, status["failing-comp"].State)
	require.Contains(t, status, "healthy-comp")
	assert.Equal(t, component.StateStopped, status["healthy-comp"].State)
}

// TestComponentManagerStart_JoinsMultipleFailures: the joined error names every failed component,
// not only the first.
func TestComponentManagerStart_JoinsMultipleFailures(t *testing.T) {
	errA := errors.New("failure A")
	errB := errors.New("failure B")
	failA := newBarrierTestComponent("fail-a")
	failA.startErr = errA
	failB := newBarrierTestComponent("fail-b")
	failB.startErr = errB

	cm := newTestComponentManager(t, barrierComposed(failA), barrierComposed(failB))
	barrierStopOnCleanup(t, cm)

	err := cm.Start(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), `component "fail-a"`, "the first failed component must be named")
	assert.Contains(t, err.Error(), `component "fail-b"`, "the second failed component must be named")
	assert.ErrorIs(t, err, errA)
	assert.ErrorIs(t, err, errB)
}

// TestComponentManagerStart_WaitsForAllComponentStarts: ComponentManager.Start does not return
// until every component Start it launched has returned. Start returning while the gate is held is
// the fire-and-forget bug.
func TestComponentManagerStart_WaitsForAllComponentStarts(t *testing.T) {
	gated := newBarrierTestComponent("gated-comp")
	gated.entered = make(chan struct{})
	gated.release = make(chan struct{})

	cm := newTestComponentManager(t, barrierComposed(gated))
	barrierStopOnCleanup(t, cm)

	done := make(chan error, 1)
	go func() { done <- cm.Start(t.Context()) }()

	waitForSignal(t, gated.entered, "the gated component to enter Start")
	select {
	case err := <-done:
		t.Fatalf("ComponentManager.Start returned (err=%v) while a component Start was in flight", err)
	case <-time.After(negativeWindow):
	}

	close(gated.release)
	require.NoError(t, barrierStartResult(t, done, "Start to return after the gate opened"))
	select {
	case <-gated.startReturned:
	default:
		t.Fatal("ComponentManager.Start returned before the component's Start had returned")
	}
	assert.Equal(t, component.StateStarted, cm.GetComponentStatus()["gated-comp"].State)
}

// stopOrderLog records, in order, the names of the components whose Stop was called.
type stopOrderLog struct {
	mu    sync.Mutex
	names []string
}

func (l *stopOrderLog) record(name string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.names = append(l.names, name)
}

func (l *stopOrderLog) recorded() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.names...)
}

// stopOrderComponent is a lifecycle component that records its Stop in a shared log.
type stopOrderComponent struct {
	baseDiscoverable
	name string
	log  *stopOrderLog
}

func (c *stopOrderComponent) Initialize() error           { return nil }
func (c *stopOrderComponent) Start(context.Context) error { return nil }
func (c *stopOrderComponent) Stop(context.Context) error {
	c.log.record(c.name)
	return nil
}

// stopOrderProvider is a stopOrderComponent that owns one store, so it starts in the provider
// phase.
type stopOrderProvider struct {
	*stopOrderComponent
}

func (p *stopOrderProvider) ProvidedStores() map[string]storage.StreamableStore {
	return map[string]storage.StreamableStore{p.name: &fakeStreamable{id: p.name}}
}

// TestComponentManagerStopsInReverseStartOrderByPhaseThenName: within a phase the start order is
// recorded by name (sweep C4), providers before the rest, so Stop visits the consumers by name
// descending and then the providers by name descending, whatever order the configuration's map
// yields (design D2). The provider names sort after the consumer names, so an order by name alone,
// ignoring the phases, fails too.
func TestComponentManagerStopsInReverseStartOrderByPhaseThenName(t *testing.T) {
	log := &stopOrderLog{}
	composed := make([]testComponent, 0, 5)
	for _, name := range []string{"store-beta", "store-alpha"} {
		composed = append(composed, testComponent{name: name, instance: &stopOrderProvider{
			stopOrderComponent: &stopOrderComponent{name: name, log: log},
		}})
	}
	for _, name := range []string{"reader-b", "reader-c", "reader-a"} {
		composed = append(composed, testComponent{name: name, instance: &stopOrderComponent{name: name, log: log}})
	}
	cm := newTestComponentManager(t, composed...)

	require.NoError(t, cm.Start(t.Context()))
	require.NoError(t, cm.Stop(stopContext(t)))
	assert.Equal(t, []string{"reader-c", "reader-b", "reader-a", "store-beta", "store-alpha"}, log.recorded())
}
