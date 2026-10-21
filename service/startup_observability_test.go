package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/health"
	"github.com/c360studio/semengine/metric"
)

// This file carries the pin's startup_observability_test.go without its tests that need a real
// ComponentManager, which go to PR 3 with it (design D7): pin lines 148-223, 264-395 and 418-449.
// TestStartupMuxCommitIsCausallyAtomic (pin :509-537) is not carried: it drove the dropped
// testCommitPrepared hooks (design D2, "Test-only code"). The tests that called the pin's private
// startup steps (sealComposition, initializeHTTPInfrastructure, commitStartup and the readiness
// handler on a recorder) now drive StartAll and read the main listener, as the manager's other
// tests do.

// gatedStartupService is a Service whose Start blocks until release is closed (a nil release
// does not block) and whose health a test sets (pin :27).
type gatedStartupService struct {
	name    string
	entered chan struct{}
	release <-chan struct{}
	status  atomic.Int32
	healthy atomic.Bool
	starts  atomic.Int64
	stops   atomic.Int64
}

func newGatedStartupService(name string, release <-chan struct{}) *gatedStartupService {
	s := &gatedStartupService{name: name, entered: make(chan struct{}), release: release}
	s.status.Store(int32(StatusStopped))
	s.healthy.Store(true)
	return s
}

func (s *gatedStartupService) Name() string { return s.name }

func (s *gatedStartupService) Start(ctx context.Context) error {
	s.starts.Add(1)
	s.status.Store(int32(StatusStarting))
	close(s.entered)
	if s.release != nil {
		select {
		case <-s.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	s.status.Store(int32(StatusRunning))
	return nil
}

func (s *gatedStartupService) Stop(context.Context) error {
	s.stops.Add(1)
	s.status.Store(int32(StatusStopped))
	return nil
}

func (s *gatedStartupService) Status() Status  { return Status(s.status.Load()) }
func (s *gatedStartupService) IsHealthy() bool { return s.healthy.Load() }
func (s *gatedStartupService) GetStatus() Info { return Info{Name: s.name, Status: s.Status()} }

func (s *gatedStartupService) Health() health.Status {
	if s.IsHealthy() {
		return health.NewHealthy(s.name, "healthy")
	}
	return health.NewUnhealthy(s.name, "unhealthy")
}

// observedManagerMetricsURL returns the scrape URL of the metrics server the manager claimed
// (pin :48). metricsServer is set from the claim until the manager released it.
func observedManagerMetricsURL(t *testing.T, manager *Manager) string {
	t.Helper()
	manager.mu.RLock()
	server := manager.metricsServer
	manager.mu.RUnlock()
	require.NotNil(t, server, "claimed metrics server not set")
	return server.Address()
}

// observedManagerStart is a StartAll running in its own goroutine (pin :57).
type observedManagerStart struct {
	done chan struct{}
	err  error
}

// beginObservedManagerStart runs manager.StartAll in a goroutine. When the test ends it calls
// release, waits for StartAll, and stops the manager under a bounded detached context.
func beginObservedManagerStart(t *testing.T, manager *Manager, release func()) *observedManagerStart {
	t.Helper()
	result := &observedManagerStart{done: make(chan struct{})}
	t.Cleanup(func() {
		if release != nil {
			release()
		}
		select {
		case <-result.done:
		case <-time.After(stopBound):
			t.Error("Manager.StartAll did not finish after the test's gates were released")
			return
		}
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), stopBound)
		defer cancel()
		if err := manager.StopAll(ctx); err != nil {
			t.Errorf("stop the manager after StartAll: %v", err)
		}
	})
	go func() {
		result.err = manager.StartAll(t.Context())
		close(result.done)
	}()
	return result
}

// requireSignal waits for signal while StartAll is still running.
func (result *observedManagerStart) requireSignal(t *testing.T, signal <-chan struct{}, owner string) {
	t.Helper()
	select {
	case <-signal:
	case <-result.done:
		t.Fatalf("Manager.StartAll returned before %s: %v", owner, result.err)
	case <-time.After(stopBound):
		t.Fatalf("%s was not observed while Manager.StartAll was running", owner)
	}
}

// wait returns StartAll's result.
func (result *observedManagerStart) wait(t *testing.T) error {
	t.Helper()
	select {
	case <-result.done:
		return result.err
	case <-time.After(stopBound):
		t.Fatal("Manager.StartAll did not return")
		return nil
	}
}

// requireAnswer requests url and requires status and, when body is not empty, that body.
func requireAnswer(t *testing.T, url string, status int, body string) {
	t.Helper()
	gotStatus, gotBody, err := httpGet(t.Context(), url)
	require.NoError(t, err)
	require.Equal(t, status, gotStatus, "GET %s answered %q", url, gotBody)
	if body != "" {
		require.Equal(t, body, gotBody, "GET %s", url)
	}
}

// startupCountsResponse is the startup part of the /services answer, decoded by its wire names.
type startupCountsResponse struct {
	Startup struct {
		Status   string `json:"status"`
		Services struct {
			Admitted        int `json:"admitted"`
			StartsInvoked   int `json:"starts_invoked"`
			StartsCompleted int `json:"starts_completed"`
			StartsFailed    int `json:"starts_failed"`
		} `json:"services"`
		Components struct {
			Admitted              int `json:"admitted"`
			LifecycleParticipants int `json:"lifecycle_participants"`
			StartsInvoked         int `json:"starts_invoked"`
			StartsCompleted       int `json:"starts_completed"`
			StartsFailed          int `json:"starts_failed"`
		} `json:"components"`
	} `json:"startup"`
}

// getStartupCounts requests /services and decodes its startup part.
func getStartupCounts(t *testing.T, baseURL string) startupCountsResponse {
	t.Helper()
	status, body, err := httpGet(t.Context(), baseURL+"/services")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status, body)
	var response startupCountsResponse
	require.NoError(t, json.Unmarshal([]byte(body), &response), body)
	return response
}

// TestStartupReadinessAndAtomicPromotion: while a service's Start blocks, the main listener
// serves only the startup routes, so /readyz and /graph/triples answer 503 NOT READY; once
// StartAll committed, /graph/triples answers (an empty array with no NATS client) and /readyz
// answers READY, until the component-manager service turns unhealthy (ADR-109; design D6; pin
// :224, which drove the private startup steps on a recorder).
//
// spec: operator-surface / The service manager serves only diagnostics until boot commits
// spec: operator-surface / /readyz says READY only after boot, while all is healthy
func TestStartupReadinessAndAtomicPromotion(t *testing.T) {
	release := newReleaser(t)
	componentManager := newGatedStartupService("component-manager", release.ch)
	manager := createTestServiceManager(ManagerConfig{}, nil)
	require.NoError(t, manager.RegisterInstance("component-manager", componentManager))

	start := beginObservedManagerStart(t, manager, release.release)
	start.requireSignal(t, componentManager.entered, "component-manager Start entry")
	url := observedManagerHTTPURL(t, manager)
	requireAnswer(t, url+"/readyz", http.StatusServiceUnavailable, "NOT READY")
	requireAnswer(t, url+"/graph/triples", http.StatusServiceUnavailable, "NOT READY")

	release.release()
	require.NoError(t, start.wait(t))
	requireAnswer(t, url+"/graph/triples", http.StatusOK, "[]")
	requireAnswer(t, url+"/readyz", http.StatusOK, "READY")

	componentManager.healthy.Store(false)
	requireAnswer(t, url+"/readyz", http.StatusServiceUnavailable, "NOT READY")
}

// TestServicesResponseIncludesStartupCounts: /services, a startup route, reports the startup
// counts while the only service's Start blocks (pin :396, which called the private recorders).
func TestServicesResponseIncludesStartupCounts(t *testing.T) {
	release := newReleaser(t)
	componentManager := newGatedStartupService("component-manager", release.ch)
	manager := createTestServiceManager(ManagerConfig{}, nil)
	require.NoError(t, manager.RegisterInstance("component-manager", componentManager))

	start := beginObservedManagerStart(t, manager, release.release)
	start.requireSignal(t, componentManager.entered, "component-manager Start entry")
	response := getStartupCounts(t, observedManagerHTTPURL(t, manager))
	require.Equal(t, "starting", response.Startup.Status)
	require.Equal(t, 1, response.Startup.Services.Admitted)
	require.Equal(t, 1, response.Startup.Services.StartsInvoked)
	require.Equal(t, 0, response.Startup.Services.StartsCompleted)

	release.release()
	require.NoError(t, start.wait(t))
}

// TestStartupMetricsExposeFixedOwnerStagePairs (pin :450; the gauge is semengine_startup_units,
// design D1).
func TestStartupMetricsExposeFixedOwnerStagePairs(t *testing.T) {
	registry := metric.NewMetricsRegistry()
	_, err := newStartupMetricWriter(
		registry,
		func() serviceStartupCounts { return serviceStartupCounts{Admitted: 3} },
		func() startupUnitCounts { return startupUnitCounts{LifecycleParticipants: 7} },
	)
	require.NoError(t, err)

	families, err := registry.PrometheusRegistry().Gather()
	require.NoError(t, err)
	pairs := map[string]float64{}
	for _, family := range families {
		if family.GetName() != "semengine_startup_units" {
			continue
		}
		for _, sample := range family.Metric {
			labels := map[string]string{}
			for _, label := range sample.Label {
				labels[label.GetName()] = label.GetValue()
			}
			pairs[labels["owner"]+"/"+labels["stage"]] = sample.GetGauge().GetValue()
		}
	}
	require.Equal(t, float64(3), pairs["services/admitted"])
	require.Equal(t, float64(7), pairs["components/lifecycle_participants"])
}

// startOrderService is a running, healthy Service that records its name when it starts.
type startOrderService struct {
	MockService
	mu    *sync.Mutex
	order *[]string
}

func (s *startOrderService) Start(context.Context) error {
	s.mu.Lock()
	*s.order = append(*s.order, s.name)
	s.mu.Unlock()
	return nil
}

// TestBuiltinMetricsRetainsRegistrationOrder: the metrics service starts in its registration
// place, whether it is the real Metrics or any service named metrics (pin :478, which read the
// private sealed order; here the starts are observed).
func TestBuiltinMetricsRetainsRegistrationOrder(t *testing.T) {
	t.Run("the real Metrics", func(t *testing.T) {
		deps := createTestServiceDependencies()
		manager := createTestServiceManager(ManagerConfig{}, deps)
		metrics := newListenerMetrics(t, deps.MetricsRegistry)
		var metricsRunningAtLater atomic.Bool
		later := &failedStartRollbackService{
			MockService: MockService{name: "later", status: StatusRunning, healthy: true},
			start: func(context.Context) error {
				metricsRunningAtLater.Store(metrics.Status() == StatusRunning)
				return nil
			},
			stop: func(context.Context) error { return nil },
		}
		require.NoError(t, manager.RegisterInstance("component-manager", newStandInComponentManager()))
		require.NoError(t, manager.RegisterInstance("later", later))
		require.NoError(t, manager.RegisterInstance("metrics", metrics))
		startTestManager(t, manager)
		require.False(t, metricsRunningAtLater.Load(), "metrics started before a service registered ahead of it")
		require.Equal(t, StatusRunning, metrics.Status())
	})

	t.Run("a service named metrics", func(t *testing.T) {
		manager := createTestServiceManager(ManagerConfig{}, nil)
		var mu sync.Mutex
		var started []string
		for _, name := range []string{"component-manager", "later", "metrics"} {
			require.NoError(t, manager.RegisterInstance(name, &startOrderService{
				MockService: MockService{name: name, status: StatusRunning, healthy: true},
				mu:          &mu,
				order:       &started,
			}))
		}
		startTestManager(t, manager)
		mu.Lock()
		defer mu.Unlock()
		require.Equal(t, []string{"component-manager", "later", "metrics"}, started)
	})
}

// TestStartAllBindsSharedAndMetricsBeforeBlockedService: while the first service's Start blocks,
// the main listener serves the startup routes through the middleware, the claimed metrics server
// already serves the startup gauge (three admitted, one start invoked, none completed), and the
// later service has not started; after release the full route set is served (pin :538). The pin's
// wait for Metrics' first health observation went with the dropped OnHealthChange seam;
// readiness after boot is TestStartupReadinessAndAtomicPromotion's and
// TestPreparedFullMuxStaysInvisibleUntilCommit's.
func TestStartAllBindsSharedAndMetricsBeforeBlockedService(t *testing.T) {
	deps := createTestServiceDependencies()
	manager := createTestServiceManager(ManagerConfig{}, deps)

	release := newReleaser(t)
	componentManager := newGatedStartupService("component-manager", release.ch)
	later := newGatedStartupService("later", nil)
	metrics := newListenerMetrics(t, deps.MetricsRegistry)
	metricsAddr := metrics.testListener.Addr().String()
	require.NoError(t, manager.RegisterInstance("component-manager", componentManager))
	require.NoError(t, manager.RegisterInstance("later", later))
	require.NoError(t, manager.RegisterInstance("metrics", metrics))

	var middlewareCalls atomic.Int64
	manager.UseHTTPMiddleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			middlewareCalls.Add(1)
			next.ServeHTTP(w, r)
		})
	})

	start := beginObservedManagerStart(t, manager, release.release)
	start.requireSignal(t, componentManager.entered, "component-manager Start entry")
	select {
	case <-later.entered:
		t.Fatal("later service started while component-manager Start was blocked")
	default:
	}

	url := observedManagerHTTPURL(t, manager)
	require.Equal(t, "http://"+metricsAddr+"/metrics", observedManagerMetricsURL(t, manager))
	requireAnswer(t, url+"/readyz", http.StatusServiceUnavailable, "NOT READY")
	requireAnswer(t, url+"/openapi.json", http.StatusServiceUnavailable, "NOT READY")
	require.EqualValues(t, 2, middlewareCalls.Load())

	status, metricsText, err := httpGet(t.Context(), observedManagerMetricsURL(t, manager))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)
	require.Contains(t, metricsText, `semengine_startup_units{owner="services",stage="admitted"} 3`)
	require.Contains(t, metricsText, `semengine_startup_units{owner="services",stage="starts_completed"} 0`)
	require.Contains(t, metricsText, `semengine_startup_units{owner="services",stage="starts_invoked"} 1`)

	release.release()
	require.NoError(t, start.wait(t))
	require.EqualValues(t, 1, later.starts.Load(), "later service did not start after the release")
	require.Equal(t, StatusRunning, later.Status())
	requireAnswer(t, url+"/openapi.json", http.StatusOK, "")
	require.NoError(t, manager.StopAll(stopContext(t)))
}

// addrPausingListener stands in for the metrics service's testListener. metric.Server reads a
// supplied listener's Addr before it serves and refuses one with no TCP endpoint
// (metric/handler.go:80-83), so the manager's bind of the claimed server stops in Addr until the
// test releases it, and then fails.
type addrPausingListener struct {
	net.Listener
	entered chan struct{}
	once    sync.Once
	release <-chan struct{}
}

func (l *addrPausingListener) Addr() net.Addr {
	l.once.Do(func() { close(l.entered) })
	<-l.release
	return &net.UnixAddr{Name: "not-a-tcp-endpoint", Net: "unix"}
}

// TestMetricsBindFailureClosesSharedAndStartsNoLaterService: when the claimed metrics server
// fails to bind, StartAll fails naming the metrics bind, starts no service, and releases the main
// listener, which was bound before the metrics bind began (design D2 and D4; pin :660). The pin
// paused StartAll through the dropped testSharedHTTPBound and testMetricsBindRelease fields; here
// the metrics service's test listener pauses the bind, a call production makes. The pin's
// drained-request leg is TestStartAllRollbackWaitsForAnInFlightRequest's (design D2).
//
// spec: service-lifecycle / The service manager's StartAll bounds StartAll
func TestMetricsBindFailureClosesSharedAndStartsNoLaterService(t *testing.T) {
	t.Run("the claimed server refuses its listener", func(t *testing.T) {
		deps := createTestServiceDependencies()
		manager := createTestServiceManager(ManagerConfig{}, deps)
		componentManager := newGatedStartupService("component-manager", nil)
		later := newGatedStartupService("later", nil)
		metrics := newListenerMetrics(t, deps.MetricsRegistry)
		release := newReleaser(t)
		paused := &addrPausingListener{Listener: metrics.testListener, entered: make(chan struct{}), release: release.ch}
		metrics.testListener = paused
		require.NoError(t, manager.RegisterInstance("component-manager", componentManager))
		require.NoError(t, manager.RegisterInstance("later", later))
		require.NoError(t, manager.RegisterInstance("metrics", metrics))

		start := beginObservedManagerStart(t, manager, release.release)
		start.requireSignal(t, paused.entered, "the metrics bind")
		// The main listener is bound before the metrics bind begins.
		mainAddr := managerAddr(t, manager)
		requireAnswer(t, "http://"+mainAddr+"/readyz", http.StatusServiceUnavailable, "NOT READY")

		release.release()
		err := start.wait(t)
		require.ErrorContains(t, err, "start metrics diagnostics")
		require.Zero(t, componentManager.starts.Load())
		require.Zero(t, later.starts.Load())
		require.NotEqual(t, StatusRunning, metrics.Status(), "the metrics service started")
		manager.mu.RLock()
		require.Nil(t, manager.httpListener)
		manager.mu.RUnlock()
		requireListenerReleased(t, mainAddr)
	})

	t.Run("the metrics port is taken", func(t *testing.T) {
		deps := createTestServiceDependencies()
		manager := createTestServiceManager(ManagerConfig{}, deps)
		componentManager := newGatedStartupService("component-manager", nil)
		later := newGatedStartupService("later", nil)
		metrics, err := NewMetrics(json.RawMessage(fmt.Sprintf(`{"port":%d,"path":"/metrics"}`, heldPort(t))), deps)
		require.NoError(t, err)
		require.NoError(t, manager.RegisterInstance("component-manager", componentManager))
		require.NoError(t, manager.RegisterInstance("later", later))
		require.NoError(t, manager.RegisterInstance("metrics", metrics))
		stopManagerAtEnd(t, manager)

		err = manager.StartAll(t.Context())
		require.ErrorContains(t, err, "start metrics diagnostics")
		require.Zero(t, componentManager.starts.Load())
		require.Zero(t, later.starts.Load())
		manager.mu.RLock()
		require.Nil(t, manager.httpListener)
		manager.mu.RUnlock()
	})
}

// stopOrderService records its name when it stops (pin :773).
type stopOrderService struct {
	MockService
	order *[]string
	mu    *sync.Mutex
}

func (s *stopOrderService) Stop(context.Context) error {
	s.mu.Lock()
	*s.order = append(*s.order, s.name)
	s.mu.Unlock()
	return nil
}

// TestStopAllReversesRegistrationOrder (pin :786). The pin called StopAll without StartAll; a
// StopAll before any StartAll stops no service here (design D2), so StartAll runs first.
func TestStopAllReversesRegistrationOrder(t *testing.T) {
	manager := createTestServiceManager(ManagerConfig{}, nil)
	var mu sync.Mutex
	var stopped []string
	for _, name := range []string{"component-manager", "later", "metrics"} {
		require.NoError(t, manager.RegisterInstance(name, &stopOrderService{
			MockService: MockService{name: name, status: StatusRunning, healthy: true},
			order:       &stopped,
			mu:          &mu,
		}))
	}
	require.NoError(t, manager.StartAll(t.Context()))
	require.NoError(t, manager.StopAll(stopContext(t)))
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []string{"metrics", "later", "component-manager"}, stopped)
}

// TestStartupMetricHasNoUnitIdentityLabel (pin :801).
func TestStartupMetricHasNoUnitIdentityLabel(t *testing.T) {
	registry := metric.NewMetricsRegistry()
	_, err := newStartupMetricWriter(
		registry,
		func() serviceStartupCounts { return serviceStartupCounts{Admitted: 1} },
		func() startupUnitCounts { return startupUnitCounts{} },
	)
	require.NoError(t, err)
	families, err := registry.PrometheusRegistry().Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() != "semengine_startup_units" {
			continue
		}
		for _, sample := range family.Metric {
			for _, label := range sample.Label {
				require.NotContains(t, strings.ToLower(label.GetName()), "name")
				require.NotContains(t, strings.ToLower(label.GetName()), "unit")
			}
		}
		return
	}
	t.Fatal("startup metric family missing")
}

// TestStartAllRejectsEndedContextBeforeCompositionOrListenerMutation: a StartAll refused for a
// nil or ended context binds nothing, creates no component-manager and leaves the composition
// open (design D2; pin :826, which read the dropped sealed and httpUsed flags and named a fixed
// port it never bound).
func TestStartAllRejectsEndedContextBeforeCompositionOrListenerMutation(t *testing.T) {
	for _, test := range []struct {
		name string
		ctx  func() context.Context
	}{
		{name: "nil", ctx: func() context.Context { return nil }},
		{name: "pre-canceled", ctx: func() context.Context {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			return ctx
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			registry := NewServiceRegistry()
			require.NoError(t, registry.Register("component-manager", func(json.RawMessage, *Dependencies) (Service, error) {
				return newStandInComponentManager(), nil
			}))
			manager := NewServiceManager(registry)
			require.Error(t, manager.StartAll(test.ctx()))
			manager.mu.RLock()
			listener := manager.httpListener
			manager.mu.RUnlock()
			require.Nil(t, listener, "a refused StartAll bound the main listener")
			_, created := manager.GetService("component-manager")
			require.False(t, created, "a refused StartAll created component-manager")
			require.NoError(t, manager.RegisterInstance("late", &MockService{name: "late"}),
				"a refused StartAll sealed the composition")
		})
	}
}
