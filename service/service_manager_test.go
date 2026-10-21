package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/health"
	"github.com/c360studio/semengine/internal/harness/lifecycletest"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

// MockService is a Service whose status and health a test sets (pin service_manager_test.go:149).
// Its Start honours an ended context and its Stop does nothing.
type MockService struct {
	name    string
	status  Status
	healthy bool
}

func (m *MockService) Name() string { return m.name }

func (m *MockService) Start(ctx context.Context) error { return ctx.Err() }

func (m *MockService) Stop(context.Context) error { return nil }
func (m *MockService) Status() Status             { return m.status }
func (m *MockService) IsHealthy() bool            { return m.healthy }
func (m *MockService) GetStatus() Info            { return Info{Name: m.name, Status: m.status} }

func (m *MockService) Health() health.Status {
	if !m.healthy {
		return health.NewUnhealthy(m.name, "Mock service unhealthy")
	}
	switch m.status {
	case StatusRunning:
		return health.NewHealthy(m.name, "Mock service running")
	case StatusStarting:
		return health.NewDegraded(m.name, "Mock service starting")
	case StatusStopping:
		return health.NewDegraded(m.name, "Mock service stopping")
	default:
		return health.NewUnhealthy(m.name, "Mock service stopped")
	}
}

// newStandInComponentManager returns the plain Service named component-manager that every
// composition here holds until PR 3 registers the real constructor (design D3, "Tests"). It does
// not implement componentStartup, so the manager takes the pin's not-concrete path: no component
// routes, zero component counts, readiness from services.
func newStandInComponentManager() *MockService {
	return &MockService{name: "component-manager", status: StatusRunning, healthy: true}
}

// createTestServiceDependencies returns Dependencies with an Error-level logger and a fresh metrics
// registry. The pin's mockNATSClient argument is not carried: the pin set an empty
// natsclient.Client whatever the mock said, so no test read the mock.
func createTestServiceDependencies() *Dependencies {
	return &Dependencies{
		Logger:          slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		MetricsRegistry: metric.NewMetricsRegistry(),
	}
}

// createTestServiceManager returns a Manager whose configuration the test sets, as the pin's helper
// does (pin :607): port 0 binds a kernel-picked main port, which no configuration path offers.
// The Manager is not a Service (design D2, S11), so the pin's embedded BaseService goes; logger and
// natsClient are set from deps as ConfigureFromServices sets them.
func createTestServiceManager(config ManagerConfig, deps *Dependencies) *Manager {
	m := NewServiceManager(NewServiceRegistry())
	m.config = config
	m.dependencies = deps
	if deps != nil {
		m.logger = deps.Logger
		m.natsClient = deps.NATSClient
	}
	return m
}

// observedManagerHTTPURL returns the loopback URL of a manager's bound main listener (pin
// startup_observability_test.go:37). The listener field is set from the bind until it is released.
func observedManagerHTTPURL(t *testing.T, manager *Manager) string {
	t.Helper()
	manager.mu.RLock()
	listener := manager.httpListener
	manager.mu.RUnlock()
	require.NotNil(t, listener, "main HTTP listener not bound")
	addr, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok, "main HTTP listener must have a TCP endpoint")
	return "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(addr.Port))
}

// startTestManager starts manager and stops it when the test ends under a bounded detached
// context.
func startTestManager(t *testing.T, manager *Manager) {
	t.Helper()
	require.NoError(t, manager.StartAll(t.Context()))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), stopBound)
		defer cancel()
		require.NoError(t, manager.StopAll(ctx))
	})
}

// negativeWindow is how long a test waits to see that a call has not returned while something it
// must wait for is still held. It can only miss a call that returns early, never fail one that
// waits: a correct call is still waiting when the window ends, however slow the machine.
const negativeWindow = 100 * time.Millisecond

// failedStartRollbackService is a Service whose Start and Stop run the test's functions (pin
// service_manager_test.go:162).
type failedStartRollbackService struct {
	MockService
	start func(context.Context) error
	stop  func(context.Context) error
}

func (s *failedStartRollbackService) Start(ctx context.Context) error { return s.start(ctx) }
func (s *failedStartRollbackService) Stop(ctx context.Context) error  { return s.stop(ctx) }

type rollbackContextObservation struct {
	err         error
	hasDeadline bool
}

// managerAddr returns the host:port of a manager's bound main listener.
func managerAddr(t *testing.T, manager *Manager) string {
	t.Helper()
	return strings.TrimPrefix(observedManagerHTTPURL(t, manager), "http://")
}

// httpGet requests url once, on a connection it does not keep, and returns the status and body.
// It reports instead of failing, so a goroutine may call it.
func httpGet(ctx context.Context, url string) (int, string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, "", err
	}
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	response, err := client.Do(request)
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	return response.StatusCode, string(body), err
}

type getResult struct {
	status int
	body   string
	err    error
}

// getAsync starts httpGet in a goroutine and returns the channel its result arrives on.
func getAsync(ctx context.Context, url string) <-chan getResult {
	result := make(chan getResult, 1)
	go func() {
		status, body, err := httpGet(ctx, url)
		result <- getResult{status: status, body: body, err: err}
	}()
	return result
}

// publisherRunning reports whether the manager's health publisher goroutine is still running.
func publisherRunning(manager *Manager) bool {
	manager.mu.RLock()
	done := manager.healthPublisherDone
	manager.mu.RUnlock()
	return done != nil && !isClosed(done)
}

// stopManagerAtEnd stops manager when the test ends under a bounded detached context.
func stopManagerAtEnd(t *testing.T, manager *Manager) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), stopBound)
		defer cancel()
		require.NoError(t, manager.StopAll(ctx))
	})
}

// releaser closes a channel once, from the test body or a cleanup, whichever runs first.
type releaser struct {
	ch   chan struct{}
	once sync.Once
}

func newReleaser(t *testing.T) *releaser {
	r := &releaser{ch: make(chan struct{})}
	t.Cleanup(r.release)
	return r
}

func (r *releaser) release() { r.once.Do(func() { close(r.ch) }) }

// TestStartAllRollsBackFailedChildStartWithDetachedBoundedContext: the second of two services fails
// its Start; StartAll stops both under a live, bounded context derived from its own (the pass
// token below is the pin's observation of that derivation, no production state), does not return
// before the rollback joined, returns the Start error, and leaves the main listener refusing
// connections. A rollback that fails is run again by StopAll (design D2; pin :176).
//
// spec: service-lifecycle / The service manager's StartAll bounds StartAll
func TestStartAllRollsBackFailedChildStartWithDetachedBoundedContext(t *testing.T) {
	tests := []struct {
		name               string
		firstRollbackFails bool
	}{
		{name: "successful rollback makes later StopAll a no-op"},
		{name: "failed rollback retains authority for later StopAll", firstRollbackFails: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			startErr := errors.New("later Start failed")
			startOrderErr := errors.New("later service started before earlier service completed Start")
			rollbackErr := errors.New("later rollback failed")
			type rollbackContextKey string
			const key rollbackContextKey = "failed-start"
			parent, cancelParent := context.WithCancel(context.WithValue(t.Context(), key, "preserved"))
			defer cancelParent()

			earlierStarted := make(chan struct{})
			laterStopped := make(chan context.Context, 2)
			earlierStopped := make(chan context.Context, 2)
			releaseEarlierStop := newReleaser(t)
			var laterStopCalls atomic.Int64
			var earlierStopCalls atomic.Int64

			earlier := &failedStartRollbackService{
				MockService: MockService{name: "component-manager", status: StatusRunning, healthy: true},
				start: func(context.Context) error {
					close(earlierStarted)
					return nil
				},
				stop: func(ctx context.Context) error {
					call := earlierStopCalls.Add(1)
					earlierStopped <- ctx
					if call == 1 {
						<-releaseEarlierStop.ch
					}
					return nil
				},
			}
			later := &failedStartRollbackService{
				MockService: MockService{name: "later", status: StatusStarting, healthy: false},
				start: func(context.Context) error {
					select {
					case <-earlierStarted:
					default:
						return startOrderErr
					}
					cancelParent()
					return startErr
				},
				stop: func(ctx context.Context) error {
					call := laterStopCalls.Add(1)
					laterStopped <- ctx
					if test.firstRollbackFails && call == 1 {
						return rollbackErr
					}
					return nil
				},
			}

			manager := createTestServiceManager(ManagerConfig{}, createTestServiceDependencies())
			require.NoError(t, manager.RegisterInstance("component-manager", earlier))
			require.NoError(t, manager.RegisterInstance("later", later))

			startResult := make(chan error, 1)
			go func() { startResult <- manager.StartAll(parent) }()

			var laterRollbackCtx context.Context
			select {
			case laterRollbackCtx = <-laterStopped:
			case err := <-startResult:
				t.Fatalf("StartAll returned without rolling back the failed child Start: %v", err)
			}
			require.NoError(t, laterRollbackCtx.Err())
			_, hasDeadline := laterRollbackCtx.Deadline()
			require.True(t, hasDeadline)
			require.Equal(t, "preserved", laterRollbackCtx.Value(key))

			earlierRollbackCtx := <-earlierStopped
			require.NoError(t, earlierRollbackCtx.Err())
			_, hasDeadline = earlierRollbackCtx.Deadline()
			require.True(t, hasDeadline)
			require.Equal(t, "preserved", earlierRollbackCtx.Value(key))
			// The listeners stop after the services, and earlier's Stop still blocks.
			mainAddr := managerAddr(t, manager)
			select {
			case err := <-startResult:
				t.Fatalf("StartAll returned before rollback joined: %v", err)
			default:
			}

			releaseEarlierStop.release()
			err := <-startResult
			require.ErrorIs(t, err, startErr)
			// The rollback goes on past a service that fails to stop, as StopAll does.
			requireListenerReleased(t, mainAddr)
			if test.firstRollbackFails {
				require.ErrorIs(t, err, rollbackErr)
				_, retained := manager.GetService("later")
				require.True(t, retained)
				require.ErrorIs(t, manager.StartAll(t.Context()), errs.ErrAlreadyStarted)
				require.NoError(t, manager.StopAll(stopContext(t)))
				require.EqualValues(t, 2, laterStopCalls.Load())
				require.EqualValues(t, 2, earlierStopCalls.Load())
			} else {
				require.NotErrorIs(t, err, rollbackErr)
				require.NoError(t, manager.StopAll(stopContext(t)))
				require.EqualValues(t, 1, laterStopCalls.Load())
				require.EqualValues(t, 1, earlierStopCalls.Load())
			}
		})
	}
}

// TestStartAllMainBindFailureStartsNoService: StartAll on the port of a listener the test holds
// returns the bind error before any service's Start is called (design D2; pin :286).
//
// spec: service-lifecycle / The service manager's StartAll bounds StartAll
func TestStartAllMainBindFailureStartsNoService(t *testing.T) {
	var starts atomic.Int64
	var stops atomic.Int64
	service := &failedStartRollbackService{
		MockService: MockService{name: "component-manager", status: StatusRunning, healthy: true},
		start: func(context.Context) error {
			starts.Add(1)
			return nil
		},
		stop: func(context.Context) error {
			stops.Add(1)
			return nil
		},
	}
	manager := createTestServiceManager(ManagerConfig{HTTPPort: heldPort(t)}, createTestServiceDependencies())
	require.NoError(t, manager.RegisterInstance("component-manager", service))

	err := manager.StartAll(t.Context())
	require.Error(t, err)
	require.Contains(t, err.Error(), "bind HTTP listener")
	require.Zero(t, starts.Load(), "shared listener bind failure must start no child")
	require.Zero(t, stops.Load(), "no acquired child requires rollback")
	require.NoError(t, manager.StopAll(stopContext(t)))
}

// TestStartAllRollsBackMainAndPublisherAfterPublisherStartFailure: the last service's Start ends
// the StartAll context and returns nil, so the health publisher, started after every service under
// that context, is refused. StartAll returns that error, stops the service under a live bounded
// context, and leaves neither the main listener nor the publisher running (design D2; pin :314,
// which forced the failure by starting the publisher through a private call and read the private
// httpTerminal and healthPublisherTerminal flags).
//
// spec: service-lifecycle / The service manager's StartAll bounds StartAll
func TestStartAllRollsBackMainAndPublisherAfterPublisherStartFailure(t *testing.T) {
	startCtx, cancelStart := context.WithCancel(t.Context())
	defer cancelStart()
	stopObservation := make(chan rollbackContextObservation, 1)
	service := &failedStartRollbackService{
		MockService: MockService{name: "component-manager", status: StatusRunning, healthy: true},
		start: func(context.Context) error {
			cancelStart()
			return nil
		},
		stop: func(ctx context.Context) error {
			_, hasDeadline := ctx.Deadline()
			stopObservation <- rollbackContextObservation{err: ctx.Err(), hasDeadline: hasDeadline}
			return nil
		},
	}
	manager := createTestServiceManager(ManagerConfig{}, createTestServiceDependencies())
	require.NoError(t, manager.RegisterInstance("component-manager", service))

	err := manager.StartAll(startCtx)
	require.ErrorIs(t, err, context.Canceled)
	observation := <-stopObservation
	require.NoError(t, observation.err)
	require.True(t, observation.hasDeadline)
	manager.mu.RLock()
	listener := manager.httpListener
	manager.mu.RUnlock()
	require.Nil(t, listener, "the rollback left the main listener bound")
	require.False(t, publisherRunning(manager), "the rollback left the health publisher running")
	require.NoError(t, manager.StopAll(stopContext(t)))
}

// TestStopAllRetainsServiceAuthorityAfterFailure: a service's Stop fails; StopAll returns an error
// naming it, the service stays composed, and the next StopAll calls its Stop again (design D2; pin
// :340, which wrote the service map directly).
//
// spec: service-lifecycle / StopAll counts a service already stopped as stopped
func TestStopAllRetainsServiceAuthorityAfterFailure(t *testing.T) {
	wantErr := errors.New("stop failed")
	failing := &stopSpyService{
		MockService: MockService{name: "failing", status: StatusRunning, healthy: true},
		stopErr:     wantErr,
	}
	manager := startedManagerWith(t, failing)

	err := manager.StopAll(stopContext(t))
	require.ErrorIs(t, err, wantErr)
	require.Contains(t, err.Error(), "failing")
	_, retained := manager.GetService("failing")
	require.True(t, retained)
	require.EqualValues(t, 1, failing.stopCalls.Load())

	require.ErrorIs(t, manager.StopAll(stopContext(t)), wantErr)
	require.EqualValues(t, 2, failing.stopCalls.Load(), "the next StopAll calls the failing Stop again")

	failing.stopErr = nil
	require.NoError(t, manager.StopAll(stopContext(t)))
	require.EqualValues(t, 3, failing.stopCalls.Load())
}

// TestServiceManager_RegisterInstance verifies that RegisterInstance places the
// service into the map AND the order slice so StartAll and StopAll treat it
// identically to a config-driven CreateService registration.
func TestServiceManager_RegisterInstance(t *testing.T) {
	t.Parallel()
	manager := createTestServiceManager(ManagerConfig{}, nil)

	svc := &MockService{
		name:    "instance-svc",
		status:  StatusStopped,
		healthy: true,
	}

	if err := manager.RegisterInstance("instance-svc", svc); err != nil {
		t.Fatalf("RegisterInstance: %v", err)
	}

	// GetService must find it.
	got, ok := manager.GetService("instance-svc")
	if !ok {
		t.Fatal("RegisterInstance: GetService returned not-found")
	}
	if got != svc {
		t.Error("RegisterInstance: GetService returned wrong instance")
	}

	// order slice must include it so StopAll visits it in reverse order.
	manager.mu.RLock()
	found := false
	for _, name := range manager.order {
		if name == "instance-svc" {
			found = true
			break
		}
	}
	manager.mu.RUnlock()

	if !found {
		t.Error("RegisterInstance: service name not found in order slice")
	}
}

func TestServiceManager_RegisterInstance_DuplicateRejected(t *testing.T) {
	t.Parallel()
	manager := createTestServiceManager(ManagerConfig{}, nil)

	first := &MockService{name: "dup-svc", status: StatusStopped, healthy: true}
	second := &MockService{name: "dup-svc", status: StatusStopped, healthy: true}

	if err := manager.RegisterInstance("dup-svc", first); err != nil {
		t.Fatal(err)
	}
	err := manager.RegisterInstance("dup-svc", second)
	var duplicate *DuplicateServiceError
	if !errors.As(err, &duplicate) {
		t.Fatalf("duplicate error = %v", err)
	}

	// The original instance and registration order remain unchanged.
	got, ok := manager.GetService("dup-svc")
	if !ok || got != first {
		t.Errorf("duplicate RegisterInstance: GetService = %v,%v, want the first instance", got, ok)
	}

	// order must contain exactly ONE entry for the name (no double-Stop).
	manager.mu.RLock()
	count := 0
	for _, name := range manager.order {
		if name == "dup-svc" {
			count++
		}
	}
	manager.mu.RUnlock()
	if count != 1 {
		t.Errorf("duplicate RegisterInstance: order has %d entries for dup-svc, want exactly 1 (double entry → double Stop)", count)
	}
}

// TestStopAllWithAnEndedContextStopsNothing: a started manager's StopAll given a cancelled context
// returns context.Canceled, calls no service's Stop and leaves the main listener serving; a
// following StopAll with a live context returns nil. It replaces the pin's
// TestRuntimeServerCanceledStopIsTerminalWithoutResultReplay and
// TestHealthPublisherCanceledStopIsHonestAndTerminal, which called such a stop final (design D2).
//
// spec: service-lifecycle / The service manager's StartAll bounds StartAll
func TestStopAllWithAnEndedContextStopsNothing(t *testing.T) {
	spy := &stopSpyService{MockService: MockService{name: "spy", status: StatusRunning, healthy: true}}
	manager := startedManagerWith(t, spy)
	url := observedManagerHTTPURL(t, manager)

	ended, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, manager.StopAll(ended), context.Canceled)
	require.Zero(t, spy.stopCalls.Load(), "a StopAll with an ended context stopped a service")
	require.True(t, publisherRunning(manager), "a StopAll with an ended context stopped the health publisher")
	status, body, err := httpGet(t.Context(), url+"/healthz")
	require.NoError(t, err, "a StopAll with an ended context stopped the main listener")
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "OK", body)

	require.NoError(t, manager.StopAll(stopContext(t)))
	require.EqualValues(t, 1, spy.stopCalls.Load())
	require.False(t, publisherRunning(manager))
	requireListenerReleased(t, strings.TrimPrefix(url, "http://"))
}

// blockingHealthService is a Service whose Health blocks, once armed, until released, so a /health
// request stays in flight on whichever listener serves it.
type blockingHealthService struct {
	MockService
	armed     atomic.Bool
	entered   chan struct{}
	enterOnce sync.Once
	release   *releaser
}

func (s *blockingHealthService) Health() health.Status {
	if s.armed.Load() {
		s.enterOnce.Do(func() { close(s.entered) })
		<-s.release.ch
	}
	return s.MockService.Health()
}

// TestStopAllWaitsForInFlightRequests: StopAll does not return while a request on the main or the
// health listener is in flight, and that request is answered once its handler returns. It
// replaces the pin's TestRuntimeServerShutdownKeepsHandlerAuthorityLiveUntilHandlerReturns and
// TestHealthListenerShutdownKeepsHandlerAuthorityLiveUntilHandlerReturns, which installed a
// private runtime (design D2: a listener stops accepting and StopAll waits for its in-flight
// requests).
//
// spec: service-lifecycle / The service manager's StartAll bounds StartAll
func TestStopAllWaitsForInFlightRequests(t *testing.T) {
	for _, listenerName := range []string{"main", "health"} {
		t.Run(listenerName, func(t *testing.T) {
			blocking := &blockingHealthService{
				MockService: MockService{name: "blocking", status: StatusRunning, healthy: true},
				entered:     make(chan struct{}),
				release:     newReleaser(t),
			}
			manager := createTestServiceManager(ManagerConfig{}, createTestServiceDependencies())
			require.NoError(t, manager.RegisterInstance("component-manager", newStandInComponentManager()))
			require.NoError(t, manager.RegisterInstance("blocking", blocking))
			healthListener := startHealthOnBoundListener(t.Context(), t, manager)
			require.NoError(t, manager.StartAll(t.Context()))
			stopManagerAtEnd(t, manager)
			t.Cleanup(blocking.release.release) // runs before the stop above

			url := observedManagerHTTPURL(t, manager)
			if listenerName == "health" {
				url = "http://" + healthListener.Addr().String()
			}
			blocking.armed.Store(true)
			request := getAsync(t.Context(), url+"/health")
			<-blocking.entered

			stopResult := make(chan error, 1)
			go func() { stopResult <- manager.StopAll(stopContext(t)) }()
			select {
			case err := <-stopResult:
				t.Fatalf("StopAll returned while a request was in flight: %v", err)
			case <-time.After(negativeWindow):
			}

			blocking.release.release()
			require.NoError(t, <-stopResult)
			answer := <-request
			require.NoError(t, answer.err, "the in-flight request was cut")
			require.Equal(t, http.StatusOK, answer.status)
		})
	}
}

// TestStopAllCutsOpenConnectionsAtItsDeadline: a handler on the main listener waits for its
// request context to end and then for a release. A StopAll whose 100 ms context ends while it
// waits returns context.DeadlineExceeded, the handler has seen its request context end, and a new
// dial is refused; a second StopAll does not return before the release and returns nil after it
// (design D2, Q-B; the pin's TestRuntimeServerCanceledStopIsTerminalWithoutResultReplay flipped).
//
// spec: service-lifecycle / The service manager's StartAll bounds StartAll
func TestStopAllCutsOpenConnectionsAtItsDeadline(t *testing.T) {
	entered := make(chan struct{})
	sawEnd := make(chan struct{})
	release := newReleaser(t)
	manager := createTestServiceManager(ManagerConfig{}, createTestServiceDependencies())
	require.NoError(t, manager.RegisterInstance("component-manager", newStandInComponentManager()))
	manager.UseHTTPMiddleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/block" {
				next.ServeHTTP(w, r)
				return
			}
			close(entered)
			<-r.Context().Done()
			close(sawEnd)
			<-release.ch
		})
	})
	require.NoError(t, manager.StartAll(t.Context()))
	stopManagerAtEnd(t, manager)
	t.Cleanup(release.release) // runs before the stop above
	url := observedManagerHTTPURL(t, manager)

	request := getAsync(t.Context(), url+"/block")
	<-entered

	deadline, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 100*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, manager.StopAll(deadline), context.DeadlineExceeded)
	select {
	case <-sawEnd:
	case <-time.After(stopBound):
		t.Fatal("the open request's context did not end at StopAll's deadline")
	}
	requireListenerReleased(t, strings.TrimPrefix(url, "http://"))
	require.Error(t, (<-request).err, "the cut connection answered")

	second := make(chan error, 1)
	go func() { second <- manager.StopAll(stopContext(t)) }()
	select {
	case err := <-second:
		t.Fatalf("a second StopAll returned while the handler still runs: %v", err)
	case <-time.After(negativeWindow):
	}
	release.release()
	require.NoError(t, <-second)
}

// TestStartAllRollbackWaitsForAnInFlightRequest: a service whose Start blocks until released and
// then fails holds StartAll open while a /services request blocks in a middleware. StartAll does
// not return before that request's handler returns, and the request answers "status":"failed", the
// startup status a failed service start takes over a begun stop (pin service_manager.go:1824)
// (design D2, the drained-request leg of the pin's
// TestMetricsBindFailureClosesSharedAndStartsNoLaterService).
//
// spec: service-lifecycle / The service manager's StartAll bounds StartAll
func TestStartAllRollbackWaitsForAnInFlightRequest(t *testing.T) {
	startErr := errors.New("slow Start failed")
	startEntered := make(chan struct{})
	releaseStart := newReleaser(t)
	requestEntered := make(chan struct{})
	releaseRequest := newReleaser(t)
	var handlerReturned atomic.Bool

	slow := &failedStartRollbackService{
		MockService: MockService{name: "slow", status: StatusStarting},
		start: func(context.Context) error {
			close(startEntered)
			<-releaseStart.ch
			return startErr
		},
		stop: func(context.Context) error { return nil },
	}
	manager := createTestServiceManager(ManagerConfig{}, createTestServiceDependencies())
	require.NoError(t, manager.RegisterInstance("component-manager", newStandInComponentManager()))
	require.NoError(t, manager.RegisterInstance("slow", slow))
	manager.UseHTTPMiddleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/services" {
				defer handlerReturned.Store(true)
				close(requestEntered)
				<-releaseRequest.ch
			}
			next.ServeHTTP(w, r)
		})
	})

	startResult := make(chan error, 1)
	go func() { startResult <- manager.StartAll(t.Context()) }()
	<-startEntered
	request := getAsync(t.Context(), observedManagerHTTPURL(t, manager)+"/services")
	<-requestEntered

	releaseStart.release()
	select {
	case err := <-startResult:
		t.Fatalf("StartAll returned while a request was in flight: %v", err)
	case <-time.After(negativeWindow):
	}
	releaseRequest.release()
	require.ErrorIs(t, <-startResult, startErr)
	require.True(t, handlerReturned.Load(), "StartAll returned before the request's handler returned")

	answer := <-request
	require.NoError(t, answer.err)
	require.Equal(t, http.StatusOK, answer.status)
	require.Contains(t, answer.body, `"status":"failed"`)
	require.NoError(t, manager.StopAll(stopContext(t)))
}

// TestReadinessRequiresCommitAndClearsBeforeChildStop: /readyz answers NOT READY while a service's
// Start blocks, READY once StartAll returned nil, and NOT READY again while a service's Stop
// blocks inside StopAll, which returns nil once that Stop returns (design D2, D6).
//
// spec: operator-surface / /readyz says READY only after boot, while all is healthy
func TestReadinessRequiresCommitAndClearsBeforeChildStop(t *testing.T) {
	startEntered := make(chan struct{})
	releaseStart := newReleaser(t)
	stopEntered := make(chan struct{})
	releaseStop := newReleaser(t)
	gated := &failedStartRollbackService{
		MockService: MockService{name: "gated", status: StatusRunning, healthy: true},
		start: func(context.Context) error {
			close(startEntered)
			<-releaseStart.ch
			return nil
		},
		stop: func(context.Context) error {
			close(stopEntered)
			<-releaseStop.ch
			return nil
		},
	}
	manager := createTestServiceManager(ManagerConfig{}, createTestServiceDependencies())
	require.NoError(t, manager.RegisterInstance("component-manager", newStandInComponentManager()))
	require.NoError(t, manager.RegisterInstance("gated", gated))

	requireReadyz := func(url string, wantStatus int, wantBody string) {
		t.Helper()
		status, body, err := httpGet(t.Context(), url+"/readyz")
		require.NoError(t, err)
		require.Equal(t, wantStatus, status)
		require.Equal(t, wantBody, body)
	}

	startResult := make(chan error, 1)
	go func() { startResult <- manager.StartAll(t.Context()) }()
	<-startEntered
	url := observedManagerHTTPURL(t, manager)
	requireReadyz(url, http.StatusServiceUnavailable, "NOT READY")

	releaseStart.release()
	require.NoError(t, <-startResult)
	requireReadyz(url, http.StatusOK, "READY")

	stopResult := make(chan error, 1)
	go func() { stopResult <- manager.StopAll(stopContext(t)) }()
	<-stopEntered
	requireReadyz(url, http.StatusServiceUnavailable, "NOT READY")

	releaseStop.release()
	require.NoError(t, <-stopResult)
}

// TestReadinessStaysClearedWhenStopAllExpiresDuringStart: a StopAll given a live context is a
// stop request from its entry, even when its deadline ends while a service is still starting and
// before any service is stopped. Once that start completes, StartAll commits, but /readyz answers
// NOT READY, never READY (design D2; Codex review of PR #353, issue #356: SIGTERM during a slow
// start).
//
// spec: operator-surface / /readyz says READY only after boot, while all is healthy
func TestReadinessStaysClearedWhenStopAllExpiresDuringStart(t *testing.T) {
	entered := make(chan struct{})
	release := newReleaser(t)
	slow := &failedStartRollbackService{
		MockService: MockService{name: "slow", status: StatusRunning, healthy: true},
		start: func(context.Context) error {
			close(entered)
			<-release.ch
			return nil
		},
		stop: func(context.Context) error { return nil },
	}
	manager := createTestServiceManager(ManagerConfig{}, createTestServiceDependencies())
	require.NoError(t, manager.RegisterInstance("component-manager", newStandInComponentManager()))
	require.NoError(t, manager.RegisterInstance("slow", slow))

	startResult := make(chan error, 1)
	go func() { startResult <- manager.StartAll(t.Context()) }()
	<-entered
	stopManagerAtEnd(t, manager)

	// The deadline ends while the guard waits for the start; the start is still blocked, so no
	// service has been stopped when StopAll returns.
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, manager.StopAll(ctx), context.DeadlineExceeded)

	release.release()
	require.NoError(t, <-startResult)
	status, body, err := httpGet(t.Context(), observedManagerHTTPURL(t, manager)+"/readyz")
	require.NoError(t, err)
	require.Equal(t, http.StatusServiceUnavailable, status, "StopAll began, but /readyz answers %q", body)
	require.Equal(t, "NOT READY", body)
}

// newUnconnectedClient returns a NATS client that never connects; the URL is never dialled.
func newUnconnectedClient(t *testing.T) *natsclient.Client {
	t.Helper()
	client, err := natsclient.NewClient("nats://unconnected.invalid")
	require.NoError(t, err)
	return client
}

// TestHealthPublishFailureIsCounted: a manager whose NATS client is not connected completes
// StartAll, and the failed publish of each composed service's health is counted on
// semengine_errors_total{service="service-manager",type="health_publish"}. The count is read after
// StopAll, which joins the publisher, so the first publish has run whether StartAll made it or the
// publisher goroutine did (design D2, S8).
//
// spec: service-lifecycle / The service manager publishes health and counts its drops
func TestHealthPublishFailureIsCounted(t *testing.T) {
	deps := createTestServiceDependencies()
	deps.NATSClient = newUnconnectedClient(t)
	manager := createTestServiceManager(ManagerConfig{}, deps)
	require.NoError(t, manager.RegisterInstance("component-manager", newStandInComponentManager()))
	require.NoError(t, manager.RegisterInstance("probe", &MockService{name: "probe", status: StatusRunning, healthy: true}))

	require.NoError(t, manager.StartAll(t.Context()))
	require.NoError(t, manager.StopAll(stopContext(t)))

	count := requireGauge(t, deps.MetricsRegistry, "semengine_errors_total",
		map[string]string{"service": "service-manager", "type": "health_publish"})
	require.GreaterOrEqual(t, count, 1.0)
}

// TestCreateServiceRefusedAfterStartAll: once StartAll sealed the composition, CreateService and
// RegisterInstance return a *CompositionSealedError, the constructor is not called, and the
// service is not added (design D2).
//
// spec: service-lifecycle / Every composition has a component manager
func TestCreateServiceRefusedAfterStartAll(t *testing.T) {
	var constructed atomic.Int64
	registry := NewServiceRegistry()
	require.NoError(t, registry.Register("late", func(json.RawMessage, *Dependencies) (Service, error) {
		constructed.Add(1)
		return &MockService{name: "late", status: StatusRunning, healthy: true}, nil
	}))
	deps := createTestServiceDependencies()
	// createTestServiceManager's shape, on a registry holding the late constructor.
	manager := NewServiceManager(registry)
	manager.dependencies = deps
	manager.logger = deps.Logger
	require.NoError(t, manager.RegisterInstance("component-manager", newStandInComponentManager()))
	startTestManager(t, manager)

	_, err := manager.CreateService("late", json.RawMessage(`{}`), deps)
	var sealed *CompositionSealedError
	require.ErrorAs(t, err, &sealed)
	require.Zero(t, constructed.Load(), "a sealed composition called a constructor")
	_, found := manager.GetService("late")
	require.False(t, found)

	err = manager.RegisterInstance("instance", &MockService{name: "instance"})
	require.ErrorAs(t, err, &sealed)
	_, found = manager.GetService("instance")
	require.False(t, found)
}

// TestServicesListsComposedServicesWithHealth: /services lists component-manager and probe, each
// with its own status and health, with restart_required false, the startup snapshot and the
// pending changes (design D6).
//
// spec: operator-surface / Health and service listings are served as at the pin
func TestServicesListsComposedServicesWithHealth(t *testing.T) {
	manager := startedManagerWith(t, &MockService{name: "probe", status: StatusRunning, healthy: false})
	stopManagerAtEnd(t, manager)

	status, body, err := httpGet(t.Context(), observedManagerHTTPURL(t, manager)+"/services")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)

	type listed struct {
		Name    string `json:"name"`
		Status  string `json:"status"`
		Healthy bool   `json:"healthy"`
	}
	var response struct {
		Services        []listed `json:"services"`
		RestartRequired bool     `json:"restart_required"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &response))
	require.ElementsMatch(t, []listed{
		{Name: "component-manager", Status: "running", Healthy: true},
		{Name: "probe", Status: "running", Healthy: false},
	}, response.Services)
	require.False(t, response.RestartRequired)

	var keys map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(body), &keys))
	require.Contains(t, keys, "startup")
	require.Contains(t, keys, "pending_service_changes")
}

// TestSystemHealthReportsNATSDisconnected: /health from a manager whose NATS client is not
// connected answers 503, and its nats sub-status is unhealthy with a message beginning
// "Disconnected:" (design D2; the pin's failure count is gone with natsStatus.FailureCount).
//
// spec: operator-surface / Health and service listings are served as at the pin
func TestSystemHealthReportsNATSDisconnected(t *testing.T) {
	deps := createTestServiceDependencies()
	deps.NATSClient = newUnconnectedClient(t)
	manager := createTestServiceManager(ManagerConfig{}, deps)
	require.NoError(t, manager.RegisterInstance("component-manager", newStandInComponentManager()))
	startTestManager(t, manager)

	status, body, err := httpGet(t.Context(), observedManagerHTTPURL(t, manager)+"/health")
	require.NoError(t, err)
	require.Equal(t, http.StatusServiceUnavailable, status)

	var system health.Status
	require.NoError(t, json.Unmarshal([]byte(body), &system))
	var nats *health.Status
	for i := range system.SubStatuses {
		if system.SubStatuses[i].Component == "nats" {
			nats = &system.SubStatuses[i]
		}
	}
	require.NotNil(t, nats, "no nats sub-status in %s", body)
	require.False(t, nats.Healthy)
	require.True(t, strings.HasPrefix(nats.Message, "Disconnected:"), "nats message %q", nats.Message)
}

// managerOwner adapts the Manager to the lifecycle suite (lifecycle-suite, "Observe adapter
// contract"; design D2). A started manager holds its main listener, its health publisher, the
// metrics server it claimed from the metrics service, and its services; the child BaseService
// stands for those, through its health goroutine. Observe reads the manager's handles under its
// lock and adds nothing to the Manager (#186, #190). Calls is empty for BaseService's reason
// (base_lifecycle_test.go).
type managerOwner struct {
	m     *Manager
	child *BaseService
}

func (o *managerOwner) Start(ctx context.Context) error { return o.m.StartAll(ctx) }
func (o *managerOwner) Stop(ctx context.Context) error  { return o.m.StopAll(ctx) }

func (o *managerOwner) Observe() lifecycletest.Observation {
	o.m.mu.RLock()
	listener := o.m.httpListener
	publisherDone := o.m.healthPublisherDone
	metricsServer := o.m.metricsServer
	o.m.mu.RUnlock()
	var held []string
	if listener != nil {
		held = append(held, "main-listener")
	}
	if publisherDone != nil && !isClosed(publisherDone) {
		held = append(held, "health-publisher")
	}
	if metricsServer != nil {
		held = append(held, "metrics-server")
	}
	o.child.mu.RLock()
	childDone := o.child.healthDone
	o.child.mu.RUnlock()
	if childDone != nil && !isClosed(childDone) {
		held = append(held, "child-health-loop")
	}
	return lifecycletest.Observation{Unresolved: held}
}

// newSuiteManager composes the stand-in component-manager, then, when failing is set, a service
// whose Start fails (the second service), then the metrics service on a test listener and a child
// BaseService.
func newSuiteManager(t *testing.T, failing bool) *managerOwner {
	t.Helper()
	deps := createTestServiceDependencies()
	manager := createTestServiceManager(ManagerConfig{}, deps)
	require.NoError(t, manager.RegisterInstance("component-manager", newStandInComponentManager()))
	if failing {
		require.NoError(t, manager.RegisterInstance("failing", &failedStartRollbackService{
			MockService: MockService{name: "failing"},
			start:       func(context.Context) error { return errors.New("second service Start failed") },
			stop:        func(context.Context) error { return nil },
		}))
	}
	require.NoError(t, manager.RegisterInstance("metrics", newListenerMetrics(t, deps.MetricsRegistry)))
	child := NewBaseServiceWithOptions("child")
	require.NoError(t, manager.RegisterInstance("child", child))
	return &managerOwner{m: manager, child: child}
}

// TestManagerPassesTheLifecycleSuite runs the lifecycle suite on the Manager (ADR-113; design D2;
// the pin failed StartContextEndStopsNothing). The must-fail owner is a composition whose second
// service's Start fails, so StartAll rolls back the main listener and the claimed metrics server.
// The manager is one-shot: no restart is promised.
//
// spec: service-lifecycle / The service manager's StartAll bounds StartAll
func TestManagerPassesTheLifecycleSuite(t *testing.T) {
	factory := func() lifecycletest.Owner { return newSuiteManager(t, false) }
	mustFail := func() lifecycletest.Owner { return newSuiteManager(t, true) }
	lifecycletest.Run(t, factory, mustFail, lifecycletest.Promise{})

	// lifecycle-suite, "Started owner reports its handles": the suite judges only what is left
	// after a Stop, so an adapter that never reported a handle would pass it.
	t.Run("StartedOwnerReportsItsHandles", func(t *testing.T) {
		owner := newSuiteManager(t, false)
		require.NoError(t, owner.Start(t.Context()))
		require.Equal(t, []string{"main-listener", "health-publisher", "metrics-server", "child-health-loop"},
			owner.Observe().Unresolved)
		require.NoError(t, owner.Stop(stopContext(t)))
		require.Empty(t, owner.Observe().Unresolved, "a StopAll that returned nil left a handle")
	})
}

// TestManagerServesTheClaimedMetricsServerAroundItsServices is D4's manager-driven half: the
// metrics server the manager claimed from the metrics service already serves when the first
// service starts and still serves when the last service stops (after the metrics service's own
// Stop), is released by StopAll, and the metrics service is healthy while it serves.
//
// spec: service-lifecycle / The metrics service serves on its own or through the manager
func TestManagerServesTheClaimedMetricsServerAroundItsServices(t *testing.T) {
	deps := createTestServiceDependencies()
	metrics := newListenerMetrics(t, deps.MetricsRegistry)
	metricsAddr := metrics.testListener.Addr().String()
	startScrape := make(chan error, 1)
	stopScrape := make(chan error, 1)
	// Registered first: started first and stopped last.
	first := &failedStartRollbackService{
		MockService: MockService{name: "component-manager", status: StatusRunning, healthy: true},
		start: func(ctx context.Context) error {
			startScrape <- scrape(ctx, metricsAddr)
			return nil
		},
		stop: func(ctx context.Context) error {
			stopScrape <- scrape(ctx, metricsAddr)
			return nil
		},
	}
	manager := createTestServiceManager(ManagerConfig{}, deps)
	require.NoError(t, manager.RegisterInstance("component-manager", first))
	require.NoError(t, manager.RegisterInstance("metrics", metrics))
	require.False(t, metrics.IsHealthy(), "the metrics service is healthy before its server serves")

	require.NoError(t, manager.StartAll(t.Context()))
	require.NoError(t, <-startScrape, "the claimed metrics server did not serve when the first service started")
	require.True(t, metrics.IsHealthy(), "the metrics service is not healthy while its server serves")
	require.NoError(t, scrape(t.Context(), metricsAddr))

	require.NoError(t, manager.StopAll(stopContext(t)))
	require.NoError(t, <-stopScrape, "the claimed metrics server stopped before the last service")
	requireListenerReleased(t, metricsAddr)
	require.False(t, metrics.IsHealthy(), "the metrics service is healthy after its server stopped")
}
