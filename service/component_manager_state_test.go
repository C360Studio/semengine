package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/component"
)

// The component manager's state shape and its own health (design D2, "State shape" and "No wait
// for readers"; D4, "The manager's own health"; #360). The set of components is fixed at
// construction and each component's changing state is one value on its own record, so no read
// waits on a component's Start or Stop, and a read during or after Stop asks the component itself.
// The pin's borrow-fence tests (component_manager_owner_test.go:137-156, :180-311) are dropped with
// the fence; its TestComponentManagerTerminalHealthProjectionInvokesNoChild (:158-178) is flipped
// here as TestComponentHealthAfterStopAsksEachComponent, and its
// TestPerformDetailedHealthCheck_ReportsFailedComponent (component_manager_start_barrier_test.go:
// 390-425, a struct literal) is adapted as TestComponentManagerHealthNamesEveryFailedComponent.

// stateHealthComponent is a lifecycleTestComponent whose Health, DataFlow and DebugStatus answer
// from what the component knows, without waiting: healthy and "running" until its own Stop
// returned nil, then unhealthy and "stopped". It counts every Health call, so a test sees whether
// the manager asked it.
type stateHealthComponent struct {
	*lifecycleTestComponent
	stopped     atomic.Bool
	healthCalls atomic.Int64
}

func newStateHealthComponent(name string, log *lifecycleCallLog) *stateHealthComponent {
	return &stateHealthComponent{lifecycleTestComponent: newLifecycleTestComponent(name, log)}
}

func (c *stateHealthComponent) testComponent() testComponent {
	return testComponent{name: c.name, instance: c}
}

func (c *stateHealthComponent) Stop(ctx context.Context) error {
	err := c.lifecycleTestComponent.Stop(ctx)
	if err == nil {
		c.stopped.Store(true)
	}
	return err
}

func (c *stateHealthComponent) Health() component.HealthStatus {
	c.healthCalls.Add(1)
	if c.stopped.Load() {
		return component.HealthStatus{Healthy: false, Status: "stopped"}
	}
	return component.HealthStatus{Healthy: true, Status: "running"}
}

func (c *stateHealthComponent) DebugStatus() any {
	return map[string]any{"stops": c.stops.Load(), "stopped": c.stopped.Load()}
}

var (
	_ component.LifecycleComponent  = (*stateHealthComponent)(nil)
	_ component.DebugStatusProvider = (*stateHealthComponent)(nil)
)

// stateMux returns a mux serving cm's routes under /components/, as the service manager mounts
// them.
func stateMux(cm *ComponentManager) *http.ServeMux {
	mux := http.NewServeMux()
	cm.RegisterHTTPHandlers("/components/", mux)
	return mux
}

// stateGet serves a GET of path on mux and returns the recorded response.
func stateGet(mux *http.ServeMux, path string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder
}

// stateReadReturns runs read on its own goroutine and fails the test, naming description, unless
// it returns within stopBound. A read that waits on a held Start or Stop is reported, not hung.
func stateReadReturns(t *testing.T, description string, read func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		read()
	}()
	waitForSignal(t, done, description+" to return")
}

// stateStartOrders returns each component's StartOrder as withComponents hands it out.
func stateStartOrders(t *testing.T, cm *ComponentManager) map[string]int {
	t.Helper()
	orders := make(map[string]int)
	require.NoError(t, cm.withComponents(func(managed map[string]*component.ManagedComponent) error {
		for name, mc := range managed {
			orders[name] = mc.StartOrder
		}
		return nil
	}))
	return orders
}

// stateListedNames returns the sorted names /components/list answers with.
func stateListedNames(t *testing.T, mux *http.ServeMux) []string {
	t.Helper()
	response := stateGet(mux, "/components/list")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var listed []map[string]any
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &listed))
	names := make([]string, 0, len(listed))
	for _, entry := range listed {
		name, ok := entry["name"].(string)
		require.True(t, ok, "a /components/list entry has no name: %v", entry)
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// TestComponentListIsFixedAtConstruction: the names and start positions withComponents gives, and
// the names /components/list gives, are the same before Start, after it, after Stop and, on a
// second manager, after a failed Start; the start positions are by name within the one phase. A
// withComponents callback that writes a ManagedComponent's State, LastError and StartOrder, or
// deletes an entry, changes nothing the manager reports (design D2, "State shape").
//
// spec: service-lifecycle / The component manager's reads wait on no lock and not on Stop
func TestComponentListIsFixedAtConstruction(t *testing.T) {
	names := []string{"list-a", "list-b", "list-c"}
	a := newLifecycleTestComponent("list-a", nil)
	b := newLifecycleTestComponent("list-b", nil)
	c := newLifecycleTestComponent("list-c", nil)
	cm := newTestComponentManager(t, c.testComponent(), a.testComponent(), b.testComponent())
	mux := stateMux(cm)

	constructed := stateStartOrders(t, cm)
	require.Len(t, constructed, 3)
	require.True(t, constructed["list-a"] < constructed["list-b"] && constructed["list-b"] < constructed["list-c"],
		"the start positions are not by name: %v", constructed)
	require.Equal(t, names, stateListedNames(t, mux), "before Start")

	require.NoError(t, cm.Start(t.Context()))
	require.Equal(t, constructed, stateStartOrders(t, cm), "after Start")
	require.Equal(t, names, stateListedNames(t, mux), "after Start")

	errWritten := errors.New("written by a withComponents callback")
	require.NoError(t, cm.withComponents(func(managed map[string]*component.ManagedComponent) error {
		for _, mc := range managed {
			mc.State = component.StateFailed
			mc.LastError = errWritten
			mc.StartOrder = 99
		}
		delete(managed, "list-b")
		return nil
	}))
	requireComponentStates(t, cm, component.StateStarted, names...)
	for name, status := range cm.GetComponentStatus() {
		require.NoError(t, status.LastError, "component %q carries the callback's write", name)
	}
	require.NoError(t, cm.healthCheck(), "the manager's health reads the callback's write")
	require.Equal(t, constructed, stateStartOrders(t, cm), "after a callback wrote its copies")
	require.Equal(t, names, stateListedNames(t, mux), "after a callback wrote its copies")

	require.NoError(t, cm.Stop(stopContext(t)))
	require.Equal(t, constructed, stateStartOrders(t, cm), "after Stop")
	require.Equal(t, names, stateListedNames(t, mux), "after Stop")

	errFailedStart := errors.New("list-y Start failed")
	x := newLifecycleTestComponent("list-x", nil)
	y := newLifecycleTestComponent("list-y", nil)
	y.startErr = errFailedStart
	failed := newTestComponentManager(t, y.testComponent(), x.testComponent())
	failedMux := stateMux(failed)
	failedConstructed := stateStartOrders(t, failed)
	require.Len(t, failedConstructed, 2)
	require.Less(t, failedConstructed["list-x"], failedConstructed["list-y"], "the start positions are not by name")

	require.ErrorIs(t, failed.Start(t.Context()), errFailedStart)
	require.Equal(t, failedConstructed, stateStartOrders(t, failed), "after a failed Start")
	require.Equal(t, []string{"list-x", "list-y"}, stateListedNames(t, failedMux), "after a failed Start")
	require.NoError(t, failed.Stop(stopContext(t)))
}

// TestComponentReadsReturnWhileAComponentStartIsBlocked: while one component's Start is held open
// by the test, GetComponentStatus, GetComponentHealth and /components/status/{name} return, and a
// sibling whose Start returned reads StateStarted (design D2, "State shape"). The sibling is a store
// provider, so it starts in the first phase, which every Start of joins before the held component's
// phase begins: its state is written before the held Start is entered, with no wait in the test.
//
// spec: service-lifecycle / The component manager's reads wait on no lock and not on Stop
func TestComponentReadsReturnWhileAComponentStartIsBlocked(t *testing.T) {
	provider := &lifecycleStoreProvider{lifecycleTestComponent: newLifecycleTestComponent("start-provider", nil)}
	held := newStateHealthComponent("start-held", nil)
	held.holdStart()
	cm := newTestComponentManager(t,
		held.testComponent(), testComponent{name: provider.name, instance: provider})
	mux := stateMux(cm)

	release := sync.OnceFunc(func() { close(held.releaseStart) })
	defer release()
	started := make(chan error, 1)
	go func() { started <- cm.Start(t.Context()) }()
	waitForSignal(t, held.startEntered, "the held component's Start")

	var statuses map[string]ComponentStatus
	stateReadReturns(t, "GetComponentStatus during a held Start", func() { statuses = cm.GetComponentStatus() })
	require.Equal(t, component.StateStarted, statuses[provider.name].State,
		"a sibling whose Start returned does not read started")
	require.NotEqual(t, component.StateStarted, statuses[held.name].State,
		"a component whose Start has not returned reads started")

	var health map[string]component.HealthStatus
	stateReadReturns(t, "GetComponentHealth during a held Start", func() { health = cm.GetComponentHealth() })
	require.Contains(t, health, held.name, "GetComponentHealth did not ask the component whose Start is held")
	require.Contains(t, health, provider.name)

	var response *httptest.ResponseRecorder
	stateReadReturns(t, "/components/status/{name} during a held Start", func() {
		response = stateGet(mux, "/components/status/"+held.name)
	})
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	release()
	require.NoError(t, <-started)
	requireComponentStates(t, cm, component.StateStarted, provider.name, held.name)
	require.NoError(t, cm.Stop(stopContext(t)))
}

// TestComponentReadsReturnWhileAComponentStopIsBlocked: while one component's Stop is held open by
// the test, GetComponentStatus, GetComponentHealth and /components/status/{name} return, and the
// sibling Stop stopped first reads StateStopped (design D2, "No wait for readers"). Within the one
// phase the Stop order is the reverse of the order by name, and the stop sequence calls each Stop
// on one stack, so stop-b's state is written before stop-a's Stop is entered.
//
// spec: service-lifecycle / The component manager's reads wait on no lock and not on Stop
func TestComponentReadsReturnWhileAComponentStopIsBlocked(t *testing.T) {
	stopEntered, releaseStop := make(chan struct{}), make(chan struct{})
	held := newStateHealthComponent("stop-a", nil)
	held.onStop = func(ctx context.Context, call int64) error {
		if call != 1 {
			return nil
		}
		close(stopEntered)
		select {
		case <-releaseStop:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	sibling := newStateHealthComponent("stop-b", nil)
	cm := newTestComponentManager(t, held.testComponent(), sibling.testComponent())
	mux := stateMux(cm)
	require.NoError(t, cm.Start(t.Context()))

	release := sync.OnceFunc(func() { close(releaseStop) })
	defer release()
	stopCtx := stopContext(t)
	stopped := make(chan error, 1)
	go func() { stopped <- cm.Stop(stopCtx) }()
	waitForSignal(t, stopEntered, "the held component's Stop")

	var statuses map[string]ComponentStatus
	stateReadReturns(t, "GetComponentStatus during a held Stop", func() { statuses = cm.GetComponentStatus() })
	require.Equal(t, component.StateStopped, statuses[sibling.name].State,
		"a sibling whose Stop returned does not read stopped")
	require.NotEqual(t, component.StateStopped, statuses[held.name].State,
		"a component whose Stop has not returned reads stopped")

	heldCalls := held.healthCalls.Load()
	var health map[string]component.HealthStatus
	stateReadReturns(t, "GetComponentHealth during a held Stop", func() { health = cm.GetComponentHealth() })
	require.Greater(t, held.healthCalls.Load(), heldCalls, "GetComponentHealth did not ask the component whose Stop is held")
	require.Equal(t, component.HealthStatus{Healthy: true, Status: "running"}, health[held.name])
	require.Equal(t, component.HealthStatus{Healthy: false, Status: "stopped"}, health[sibling.name])

	var response *httptest.ResponseRecorder
	stateReadReturns(t, "/components/status/{name} during a held Stop", func() {
		response = stateGet(mux, "/components/status/"+held.name)
	})
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	release()
	require.NoError(t, <-stopped)
	requireComponentStates(t, cm, component.StateStopped, held.name, sibling.name)
}

// TestComponentHealthAfterStopAsksEachComponent, flipped from the pin's
// TestComponentManagerTerminalHealthProjectionInvokesNoChild (component_manager_owner_test.go:
// 158-178), which found no health and no call into a component after Stop: here each component is
// asked, and what it answers after its own Stop is what GetComponentHealth and /components/health
// return (design D2, "No wait for readers"; D7).
//
// spec: service-lifecycle / The component manager's reads wait on no lock and not on Stop
func TestComponentHealthAfterStopAsksEachComponent(t *testing.T) {
	a := newStateHealthComponent("after-a", nil)
	b := newStateHealthComponent("after-b", nil)
	cm := newTestComponentManager(t, a.testComponent(), b.testComponent())
	mux := stateMux(cm)
	require.NoError(t, cm.Start(t.Context()))
	require.NoError(t, cm.Stop(stopContext(t)))

	stoppedHealth := component.HealthStatus{Healthy: false, Status: "stopped"}
	callsA, callsB := a.healthCalls.Load(), b.healthCalls.Load()
	health := cm.GetComponentHealth()
	require.Equal(t, map[string]component.HealthStatus{a.name: stoppedHealth, b.name: stoppedHealth}, health,
		"GetComponentHealth after Stop is not each component's own answer")
	require.Greater(t, a.healthCalls.Load(), callsA, "GetComponentHealth after Stop did not ask %q", a.name)
	require.Greater(t, b.healthCalls.Load(), callsB, "GetComponentHealth after Stop did not ask %q", b.name)

	callsA, callsB = a.healthCalls.Load(), b.healthCalls.Load()
	response := stateGet(mux, "/components/health")
	require.Equal(t, http.StatusServiceUnavailable, response.Code, response.Body.String())
	var body struct {
		Components []struct {
			Component string `json:"component"`
			Healthy   bool   `json:"healthy"`
		} `json:"components"`
		Total int `json:"total"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.Equal(t, 2, body.Total)
	answered := make(map[string]bool, len(body.Components))
	for _, entry := range body.Components {
		answered[entry.Component] = entry.Healthy
	}
	require.Equal(t, map[string]bool{a.name: false, b.name: false}, answered,
		"/components/health after Stop is not each component's own answer")
	require.Greater(t, a.healthCalls.Load(), callsA, "/components/health after Stop did not ask %q", a.name)
	require.Greater(t, b.healthCalls.Load(), callsB, "/components/health after Stop did not ask %q", b.name)
}

// TestComponentManagerHealthNamesAComponentLeftFailedByARollback: before Start the check looks and
// answers healthy; after a Start that fails and whose rollback Stop fails too, the component is
// left in StateFailed and the check reports it, naming it and carrying its last error; once a later
// Stop stops it, the check is healthy (design D4). The pin answered healthy after the failed Start,
// since its started flag was never set (:1156-1159). The check is called directly: BaseService
// starts only after every component started, so production never runs it after a failed Start.
//
// spec: service-lifecycle / The component manager's health reads every component
func TestComponentManagerHealthNamesAComponentLeftFailedByARollback(t *testing.T) {
	errStart := errors.New("rollback-health Start failed")
	errStop := errors.New("rollback-health Stop failed")
	c := newLifecycleTestComponent("rollback-health", nil)
	c.startErr = errStart
	c.stopErrs = []error{errStop}
	cm := newTestComponentManager(t, c.testComponent())
	require.NoError(t, cm.healthCheck(), "the check before Start found a failed component")

	err := cm.Start(t.Context())
	require.ErrorIs(t, err, errStart)
	require.ErrorIs(t, err, errStop)
	status := cm.GetComponentStatus()[c.name]
	require.Equal(t, component.StateFailed, status.State)
	require.Error(t, status.LastError, "the component left failed carries no last error")

	health := cm.healthCheck()
	require.Error(t, health, "the check answered healthy with a component left failed by the rollback")
	require.ErrorContains(t, health, `component "rollback-health" failed`)
	require.ErrorIs(t, health, status.LastError, "the check does not carry the component's last error")

	require.NoError(t, cm.Stop(stopContext(t)))
	requireComponentStates(t, cm, component.StateStopped, c.name)
	require.NoError(t, cm.healthCheck(), "the check still reports the component a later Stop stopped")
}

// TestComponentManagerHealthNamesEveryFailedComponent, adapted from the pin's
// TestPerformDetailedHealthCheck_ReportsFailedComponent (component_manager_start_barrier_test.go:
// 390-425), which built a struct-literal manager and set its state by hand: a Stop that leaves two
// components failed makes the check name both, in name order, each carrying its Stop error, and a
// retried Stop that succeeds clears it (design D4).
//
// spec: service-lifecycle / The component manager's health reads every component
func TestComponentManagerHealthNamesEveryFailedComponent(t *testing.T) {
	errA := errors.New("failed-a Stop failed")
	errC := errors.New("failed-c Stop failed")
	a := newLifecycleTestComponent("failed-a", nil)
	a.stopErrs = []error{errA}
	b := newLifecycleTestComponent("failed-b", nil)
	c := newLifecycleTestComponent("failed-c", nil)
	c.stopErrs = []error{errC}
	cm := newTestComponentManager(t, c.testComponent(), b.testComponent(), a.testComponent())
	require.NoError(t, cm.Start(t.Context()))
	require.NoError(t, cm.healthCheck())

	stopErr := cm.Stop(stopContext(t))
	require.ErrorIs(t, stopErr, errA)
	require.ErrorIs(t, stopErr, errC)

	health := cm.healthCheck()
	require.Error(t, health, "the check answered healthy with two components left failed")
	require.ErrorIs(t, health, errA)
	require.ErrorIs(t, health, errC)
	message := health.Error()
	first := strings.Index(message, `component "failed-a" failed`)
	second := strings.Index(message, `component "failed-c" failed`)
	require.GreaterOrEqual(t, first, 0, "the check does not name failed-a: %s", message)
	require.GreaterOrEqual(t, second, 0, "the check does not name failed-c: %s", message)
	require.Less(t, first, second, "the check does not name the failed components in name order: %s", message)
	require.NotContains(t, message, `"failed-b"`, "the check names a component that stopped")

	require.NoError(t, cm.Stop(stopContext(t)))
	require.NoError(t, cm.healthCheck(), "a retried Stop that succeeded did not clear the check")
}

// TestComponentReadsRaceStartAndStop: readers of GetComponentStatus, GetComponentHealth, the
// manager's health check, /components/list, /components/status/{name}?debug=true and
// /components/config/{name} run while the test runs Start, a Stop in which one component's Stop
// fails, and a retried Stop; the supervisor's health publish when Start returns (over the
// unconnected client) runs among them. Under -race this is the state shape's race check (design
// D2). The readers end on a closed channel and are joined, never slept. Beyond the race detector,
// a health check a reader began after the failing Stop returned, and ended before the retry began,
// names the failed component; the test waits for one such check before it retries, so the
// assertion runs.
//
// spec: service-lifecycle / The component manager's reads wait on no lock and not on Stop
func TestComponentReadsRaceStartAndStop(t *testing.T) {
	errRaceStop := errors.New("race-b Stop failed")
	a := newStateHealthComponent("race-a", nil)
	b := newStateHealthComponent("race-b", nil)
	b.stopErrs = []error{errRaceStop}
	cm := newTestComponentManager(t, a.testComponent(), b.testComponent())
	mux := stateMux(cm)

	var failedStopReturned, retryBegun atomic.Bool
	checkedBetween := make(chan struct{})
	signalChecked := sync.OnceFunc(func() { close(checkedBetween) })
	readersDone := make(chan struct{})
	endReaders := sync.OnceFunc(func() { close(readersDone) })
	var readers sync.WaitGroup
	defer readers.Wait()
	defer endReaders()

	reads := []func(){
		func() { _ = cm.GetComponentStatus() },
		func() { _ = cm.GetComponentHealth() },
		func() {
			began := failedStopReturned.Load()
			err := cm.healthCheck()
			if began && !retryBegun.Load() {
				if err == nil || !strings.Contains(err.Error(), `component "race-b" failed`) ||
					!errors.Is(err, errRaceStop) {
					t.Errorf("a health check between the failing Stop and its retry answered %v", err)
				}
				signalChecked()
			}
		},
		func() { stateRequireRoute(t, mux, "/components/list") },
		func() { stateRequireRoute(t, mux, "/components/status/race-a?debug=true") },
		func() { stateRequireRoute(t, mux, "/components/config/race-b") },
	}
	for _, read := range reads {
		readers.Go(func() {
			for {
				select {
				case <-readersDone:
					return
				default:
					read()
				}
			}
		})
	}

	require.NoError(t, cm.Start(t.Context()))
	require.ErrorIs(t, cm.Stop(stopContext(t)), errRaceStop)
	failedStopReturned.Store(true)
	waitForSignal(t, checkedBetween, "a health check between the failing Stop and its retry")
	retryBegun.Store(true)
	require.NoError(t, cm.Stop(stopContext(t)))
	endReaders()
	readers.Wait()

	requireComponentStates(t, cm, component.StateStopped, a.name, b.name)
	require.NoError(t, cm.healthCheck())
}

// stateRequireRoute serves a GET of path on mux and reports, from any goroutine, an answer other
// than 200.
func stateRequireRoute(t *testing.T, mux *http.ServeMux, path string) {
	if response := stateGet(mux, path); response.Code != http.StatusOK {
		t.Errorf("GET %s answered %d: %s", path, response.Code, response.Body.String())
	}
}
