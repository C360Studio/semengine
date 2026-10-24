package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/types"
)

// gatewayComponent is a component that registers its own routes, as a gateway does.
type gatewayComponent struct{ baseDiscoverable }

func (gatewayComponent) RegisterHTTPHandlers(prefix string, mux *http.ServeMux) {
	mux.HandleFunc("GET "+prefix+"/hello", answer(http.StatusOK, "hello"))
}

// awaitStartupStatus requests /services until its startup status is status, failing the test
// once stopBound passes. It waits for what a goroutine the test does not hold records, such as a
// component's start outcome, which the component manager stores after the component's Start
// returned.
func awaitStartupStatus(t *testing.T, baseURL, status string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), stopBound)
	defer cancel()
	_, err := probe.Await(ctx,
		func(ctx context.Context) (string, error) {
			code, body, err := httpGet(ctx, baseURL+"/services")
			if err != nil {
				return "", err
			}
			if code != http.StatusOK {
				return "", errors.New(body)
			}
			var response startupCountsResponse
			if err := json.Unmarshal([]byte(body), &response); err != nil {
				return "", err
			}
			return response.Startup.Status, nil
		},
		func(got string) bool { return got == status })
	require.NoError(t, err, "/services never reported startup status %q", status)
}

// TestComponentStartupServesStartupRoutesCountsAndComponentRoutes: the component manager is
// handed the startup metric writer before its components start, serves its read-only startup
// routes under /components before the commit, reports its component counts in /services and in
// the startup gauge, and has the routes of components that register their own mounted with the
// full route set (design D3 and D6; pin service_manager.go:593, :1142, :1543).
//
// spec: operator-surface / The service manager serves only diagnostics until boot commits
func TestComponentStartupServesStartupRoutesCountsAndComponentRoutes(t *testing.T) {
	gated := newBarrierTestComponent("gated")
	gated.entered = make(chan struct{})
	release := newReleaser(t)
	gated.release = release.ch
	cm := newTestComponentManager(t,
		barrierComposed(gated), testComponent{name: "gateway", instance: gatewayComponent{}})
	deps := createTestServiceDependencies()
	manager := createTestServiceManager(ManagerConfig{}, deps)
	require.NoError(t, manager.RegisterInstance("component-manager", cm))

	start := beginObservedManagerStart(t, manager, release.release)
	start.requireSignal(t, gated.entered, "the gated component's Start entry")
	url := observedManagerHTTPURL(t, manager)
	requireAnswer(t, url+"/components/list", http.StatusOK, "")
	requireAnswer(t, url+"/gateway/hello", http.StatusServiceUnavailable, "NOT READY")

	response := getStartupCounts(t, url)
	require.Equal(t, 2, response.Startup.Components.Admitted)
	require.Equal(t, 1, response.Startup.Components.LifecycleParticipants)
	gauge := func(stage string) float64 {
		return requireGauge(t, deps.MetricsRegistry, "semengine_startup_units",
			map[string]string{"owner": "components", "stage": stage})
	}
	require.Equal(t, float64(2), gauge("admitted"))
	// The gated component's start was published on the gauge, so the component manager held the
	// startup metric writer before its components started.
	require.Equal(t, float64(1), gauge("starts_invoked"),
		"the component manager was not handed the startup metric writer before its components started")

	release.release()
	require.NoError(t, start.wait(t))
	requireAnswer(t, url+"/gateway/hello", http.StatusOK, "hello")
}

// TestComponentStartupGatesReadiness: with one lifecycle component and one plain component,
// /readyz says READY only while every admitted component reports healthy and every lifecycle
// participant has started, and not once the component manager's stop has begun; /services names
// the reason (ADR-109; design D3 and D6; pin service_manager.go:1792-1866).
//
// spec: operator-surface / /readyz says READY only after boot, while all is healthy
func TestComponentStartupGatesReadiness(t *testing.T) {
	// compose registers a component manager over components on a fresh service manager.
	compose := func(t *testing.T, components ...testComponent) (*Manager, *ComponentManager) {
		t.Helper()
		cm := newTestComponentManager(t, components...)
		manager := createTestServiceManager(ManagerConfig{}, createTestServiceDependencies())
		require.NoError(t, manager.RegisterInstance("component-manager", cm))
		return manager, cm
	}
	requireNotReady := func(t *testing.T, url, status string) {
		t.Helper()
		requireAnswer(t, url+"/readyz", http.StatusServiceUnavailable, "NOT READY")
		require.Equal(t, status, getStartupCounts(t, url).Startup.Status)
	}

	t.Run("healthy, then a plain component unhealthy", func(t *testing.T) {
		plain := newStartupDiscoverable()
		manager, _ := compose(t, barrierComposed(newBarrierTestComponent("lifecycle")),
			testComponent{name: "plain", instance: plain})
		startTestManager(t, manager)
		url := observedManagerHTTPURL(t, manager)
		awaitStartupAnswer(t, url+"/readyz", http.StatusOK, "READY")
		require.Equal(t, "ready", getStartupCounts(t, url).Startup.Status)

		plain.healthy.Store(false)
		requireNotReady(t, url, "not_ready")
	})

	t.Run("a lifecycle component's Start held", func(t *testing.T) {
		held := newBarrierTestComponent("lifecycle")
		held.entered = make(chan struct{})
		release := newReleaser(t)
		held.release = release.ch
		manager, _ := compose(t, barrierComposed(held),
			testComponent{name: "plain", instance: newStartupDiscoverable()})

		start := beginObservedManagerStart(t, manager, release.release)
		start.requireSignal(t, held.entered, "the held component's Start entry")
		requireNotReady(t, observedManagerHTTPURL(t, manager), "starting")
	})

	t.Run("a sibling's Start failed while the other is held", func(t *testing.T) {
		held := newBarrierTestComponent("lifecycle")
		held.entered = make(chan struct{})
		release := newReleaser(t)
		held.release = release.ch
		failing := newBarrierTestComponent("sibling")
		failing.startErr = errors.New("sibling start failed")
		manager, _ := compose(t, barrierComposed(held), barrierComposed(failing),
			testComponent{name: "plain", instance: newStartupDiscoverable()})

		start := beginObservedManagerStart(t, manager, release.release)
		start.requireSignal(t, held.entered, "the held component's Start entry")
		start.requireSignal(t, failing.startReturned, "the failing sibling's Start return")
		url := observedManagerHTTPURL(t, manager)
		// The component manager records the sibling's outcome after its Start returned.
		awaitStartupStatus(t, url, "failed")
		requireNotReady(t, url, "failed")
	})

	t.Run("the component manager's own Stop called", func(t *testing.T) {
		manager, cm := compose(t, barrierComposed(newBarrierTestComponent("lifecycle")),
			testComponent{name: "plain", instance: newStartupDiscoverable()})
		startTestManager(t, manager)
		url := observedManagerHTTPURL(t, manager)
		awaitStartupAnswer(t, url+"/readyz", http.StatusOK, "READY")

		require.NoError(t, cm.Stop(stopContext(t)))
		requireNotReady(t, url, "stopping")
	})
}

// TestConfigureRefusesACompositionWithoutAComponentManager: a registry offering metrics but no
// component-manager constructor (a composition that registers its services by hand instead of
// calling RegisterAll) cannot compose the mandatory component-manager, so ConfigureFromServices
// refuses naming the missing constructor and StartAll refuses likewise, binding nothing
// (setup-04a-03-boot-service-manager design D3). With a component-manager constructor
// registered, here one returning a stand-in component manager, the default composition holds
// component-manager and metrics.
//
// spec: service-lifecycle / Every composition has a component manager
func TestConfigureRefusesACompositionWithoutAComponentManager(t *testing.T) {
	registryWithoutComponentManager := func(t *testing.T) *Registry {
		t.Helper()
		registry := NewServiceRegistry()
		require.NoError(t, registry.Register("metrics", NewMetrics))
		return registry
	}

	t.Run("ConfigureFromServices", func(t *testing.T) {
		manager := NewServiceManager(registryWithoutComponentManager(t))
		err := manager.ConfigureFromServices(nil, createTestServiceDependencies())
		require.ErrorContains(t, err,
			"create configured service component-manager: no constructor registered for service component-manager")
	})

	t.Run("StartAll", func(t *testing.T) {
		manager := NewServiceManager(registryWithoutComponentManager(t))
		stopManagerAtEnd(t, manager)
		err := manager.StartAll(t.Context())
		require.ErrorContains(t, err, "no constructor registered for service component-manager")
		manager.mu.RLock()
		require.Nil(t, manager.httpListener, "a StartAll without a component manager bound the main listener")
		manager.mu.RUnlock()
	})

	t.Run("the default composition with a component-manager constructor", func(t *testing.T) {
		registry := registryWithoutComponentManager(t)
		standIn := newStandInComponentManager()
		require.NoError(t, registry.Register("component-manager", func(json.RawMessage, *Dependencies) (Service, error) {
			return standIn, nil
		}))
		manager := NewServiceManager(registry)
		require.NoError(t, manager.ConfigureFromServices(nil, createTestServiceDependencies()))
		got, ok := manager.GetService("component-manager")
		require.True(t, ok)
		require.Same(t, standIn, got)
		metrics, ok := manager.GetService("metrics")
		require.True(t, ok, "the default composition holds no metrics service")
		require.IsType(t, &Metrics{}, metrics)
	})
}

// TestManagerLogsThroughTheInjectedLogger: the manager logs through deps.Logger, which
// ConfigureFromServices sets, and never through slog's default logger (design D2, P4; the pin fell
// back to slog.Default). It replaces the process-wide default logger, so it must not run in
// parallel with another test.
func TestManagerLogsThroughTheInjectedLogger(t *testing.T) {
	fallback := &recordingHandler{}
	previous := slog.Default()
	slog.SetDefault(slog.New(fallback))
	t.Cleanup(func() { slog.SetDefault(previous) })

	// RegisterAll's component manager needs a configuration manager this test does not build, so
	// the registry offers metrics and a stand-in component manager instead.
	registry := NewServiceRegistry()
	require.NoError(t, registry.Register("metrics", NewMetrics))
	require.NoError(t, registry.Register("component-manager", func(json.RawMessage, *Dependencies) (Service, error) {
		return newStandInComponentManager(), nil
	}))
	injected := &recordingHandler{}
	deps := &Dependencies{Logger: slog.New(injected), MetricsRegistry: metric.NewMetricsRegistry()}
	manager := NewServiceManager(registry)
	// metrics is disabled: its default port is fixed.
	require.NoError(t, manager.ConfigureFromServices(types.ServiceConfigs{
		"metrics": {Enabled: false, Config: json.RawMessage(`{}`)},
	}, deps))
	// ConfigureFromServices defaults the main port to 8080; a kernel-picked port is set as
	// createTestServiceManager sets it.
	manager.config.HTTPPort = 0
	require.NoError(t, manager.StartAll(t.Context()))
	require.NoError(t, manager.StopAll(stopContext(t)))

	require.Positive(t, injected.count(), "the manager logged nothing through the injected logger")
	require.Zero(t, fallback.count(), "the manager logged through slog's default logger")
}
