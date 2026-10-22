package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"maps"
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/health"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/types"
)

// componentStartupDouble is design D3's second stand-in: a Service named component-manager that
// implements componentStartup, so the manager's readiness, startup counts, startup routes and
// component routes can be tested before PR 3 brings the real ComponentManager. Its Start blocks
// until release is closed (a nil release does not block). Every field it answers from is guarded,
// since the manager's handlers read it while the test changes it.
type componentStartupDouble struct {
	release <-chan struct{}
	entered chan struct{}

	mu                sync.Mutex
	status            Status
	counts            startupUnitCounts
	stopBegun         bool
	componentHealth   map[string]component.HealthStatus
	components        map[string]*component.ManagedComponent
	writer            *startupMetricWriter
	writerBeforeStart bool
	startupPrefix     string
}

var _ componentStartup = (*componentStartupDouble)(nil)

func newComponentStartupDouble(release <-chan struct{}) *componentStartupDouble {
	return &componentStartupDouble{release: release, entered: make(chan struct{}), status: StatusStopped}
}

// set changes the double's answers under its lock.
func (d *componentStartupDouble) set(change func(*componentStartupDouble)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	change(d)
}

func (d *componentStartupDouble) Name() string { return "component-manager" }

func (d *componentStartupDouble) Start(ctx context.Context) error {
	d.set(func(d *componentStartupDouble) { d.status = StatusStarting })
	close(d.entered)
	if d.release != nil {
		select {
		case <-d.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	d.set(func(d *componentStartupDouble) { d.status = StatusRunning })
	return nil
}

func (d *componentStartupDouble) Stop(context.Context) error {
	d.set(func(d *componentStartupDouble) { d.status = StatusStopped })
	return nil
}

func (d *componentStartupDouble) Status() Status {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.status
}

func (d *componentStartupDouble) IsHealthy() bool { return true }
func (d *componentStartupDouble) GetStatus() Info { return Info{Name: d.Name(), Status: d.Status()} }

func (d *componentStartupDouble) Health() health.Status {
	return health.NewHealthy(d.Name(), "component manager double")
}

func (d *componentStartupDouble) startupSnapshot() startupUnitCounts {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.counts
}

func (d *componentStartupDouble) startupStopBegun() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.stopBegun
}

func (d *componentStartupDouble) setStartupMetricWriter(writer *startupMetricWriter) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.writer = writer
	d.writerBeforeStart = d.status == StatusStopped
}

func (d *componentStartupDouble) registerStartupHTTPHandlers(prefix string, mux *http.ServeMux) {
	d.mu.Lock()
	d.startupPrefix = prefix
	d.mu.Unlock()
	mux.HandleFunc("GET "+prefix+"/startup-probe", answer(http.StatusOK, "startup"))
}

func (d *componentStartupDouble) withComponents(fn func(map[string]*component.ManagedComponent) error) error {
	d.mu.Lock()
	components := maps.Clone(d.components)
	d.mu.Unlock()
	return fn(components)
}

func (d *componentStartupDouble) GetComponentHealth() map[string]component.HealthStatus {
	d.mu.Lock()
	defer d.mu.Unlock()
	return maps.Clone(d.componentHealth)
}

// gatewayComponent is a component that registers its own routes, as a gateway does.
type gatewayComponent struct{}

func (gatewayComponent) InputPorts() []component.Port    { return nil }
func (gatewayComponent) OutputPorts() []component.Port   { return nil }
func (gatewayComponent) Health() component.HealthStatus  { return component.HealthStatus{Healthy: true} }
func (gatewayComponent) DataFlow() component.FlowMetrics { return component.FlowMetrics{} }
func (gatewayComponent) RegisterHTTPHandlers(prefix string, mux *http.ServeMux) {
	mux.HandleFunc("GET "+prefix+"/hello", answer(http.StatusOK, "hello"))
}

// TestComponentStartupServesStartupRoutesCountsAndComponentRoutes: a component-manager that
// implements componentStartup is handed the startup metric writer before its Start, serves its
// read-only startup routes under /components before the commit, reports its component counts in
// /services and in the startup gauge, and has the routes of components that register their own
// mounted with the full route set (design D3 and D6; pin service_manager.go:593, :1142, :1543).
//
// spec: operator-surface / The service manager serves only diagnostics until boot commits
func TestComponentStartupServesStartupRoutesCountsAndComponentRoutes(t *testing.T) {
	deps := createTestServiceDependencies()
	manager := createTestServiceManager(ManagerConfig{}, deps)
	release := newReleaser(t)
	double := newComponentStartupDouble(release.ch)
	double.set(func(d *componentStartupDouble) {
		d.counts = startupUnitCounts{Admitted: 2, LifecycleParticipants: 1}
		d.components = map[string]*component.ManagedComponent{"gateway": {Component: gatewayComponent{}}}
		d.componentHealth = map[string]component.HealthStatus{"gateway": {Healthy: true}, "store": {Healthy: true}}
	})
	require.NoError(t, manager.RegisterInstance("component-manager", double))

	start := beginObservedManagerStart(t, manager, release.release)
	start.requireSignal(t, double.entered, "component-manager Start entry")
	url := observedManagerHTTPURL(t, manager)
	requireAnswer(t, url+"/components/startup-probe", http.StatusOK, "startup")
	requireAnswer(t, url+"/gateway/hello", http.StatusServiceUnavailable, "NOT READY")

	response := getStartupCounts(t, url)
	require.Equal(t, 2, response.Startup.Components.Admitted)
	require.Equal(t, 1, response.Startup.Components.LifecycleParticipants)
	require.Equal(t, float64(2), requireGauge(t, deps.MetricsRegistry, "semengine_startup_units",
		map[string]string{"owner": "components", "stage": "admitted"}))
	double.mu.Lock()
	writer, beforeStart, prefix := double.writer, double.writerBeforeStart, double.startupPrefix
	double.mu.Unlock()
	require.NotNil(t, writer, "the component manager was not handed the startup metric writer")
	require.True(t, beforeStart, "the startup metric writer was handed after the component manager started")
	require.Equal(t, "/components", prefix)

	release.release()
	require.NoError(t, start.wait(t))
	requireAnswer(t, url+"/gateway/hello", http.StatusOK, "hello")
}

// TestComponentStartupGatesReadiness: with a componentStartup component-manager, /readyz says
// READY only while every admitted component reports healthy and every lifecycle participant has
// started, and not once the component manager's stop has begun; /services names the reason
// (ADR-109; design D3 and D6; pin service_manager.go:1792-1866).
//
// spec: operator-surface / /readyz says READY only after boot, while all is healthy
func TestComponentStartupGatesReadiness(t *testing.T) {
	ready := func(d *componentStartupDouble) {
		d.counts = startupUnitCounts{Admitted: 1, LifecycleParticipants: 1, StartsInvoked: 1, StartsCompleted: 1}
		d.componentHealth = map[string]component.HealthStatus{"store": {Healthy: true}}
		d.stopBegun = false
	}
	double := newComponentStartupDouble(nil)
	double.set(ready)
	manager := createTestServiceManager(ManagerConfig{}, createTestServiceDependencies())
	require.NoError(t, manager.RegisterInstance("component-manager", double))
	startTestManager(t, manager)
	url := observedManagerHTTPURL(t, manager)
	requireAnswer(t, url+"/readyz", http.StatusOK, "READY")

	for _, tt := range []struct {
		name   string
		change func(*componentStartupDouble)
		status string
	}{
		{name: "a component unhealthy", status: "not_ready", change: func(d *componentStartupDouble) {
			d.componentHealth = map[string]component.HealthStatus{"store": {Healthy: false}}
		}},
		{name: "an admitted component missing from health", status: "not_ready", change: func(d *componentStartupDouble) {
			d.componentHealth = map[string]component.HealthStatus{}
		}},
		{name: "a lifecycle participant not yet started", status: "starting", change: func(d *componentStartupDouble) {
			d.counts.StartsCompleted = 0
		}},
		{name: "a component start failed", status: "failed", change: func(d *componentStartupDouble) {
			d.counts.StartsFailed = 1
		}},
		{name: "the component manager's stop begun", status: "stopping", change: func(d *componentStartupDouble) {
			d.stopBegun = true
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			double.set(tt.change)
			requireAnswer(t, url+"/readyz", http.StatusServiceUnavailable, "NOT READY")
			require.Equal(t, tt.status, getStartupCounts(t, url).Startup.Status)
			double.set(ready)
			requireAnswer(t, url+"/readyz", http.StatusOK, "READY")
		})
	}
}

// TestConfigureRefusesACompositionWithoutAComponentManager: a registry offering metrics but no
// component-manager constructor (a composition that registers its services by hand instead of
// calling RegisterAll) cannot compose the mandatory component-manager, so ConfigureFromServices
// refuses naming the missing constructor and StartAll refuses likewise, binding nothing
// (setup-04a-03-boot-service-manager design D3). With a component-manager constructor
// registered, here one returning the componentStartup double, the default composition holds
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
		double := newComponentStartupDouble(nil)
		require.NoError(t, registry.Register("component-manager", func(json.RawMessage, *Dependencies) (Service, error) {
			return double, nil
		}))
		manager := NewServiceManager(registry)
		require.NoError(t, manager.ConfigureFromServices(nil, createTestServiceDependencies()))
		got, ok := manager.GetService("component-manager")
		require.True(t, ok)
		require.Same(t, double, got)
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
