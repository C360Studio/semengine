package service

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/metric"
)

// This file carries the pin's startup_observability_amendment_test.go without its tests that need
// a real ComponentManager, which go to PR 3 with it (design D7): pin lines 130-150 and 269-368.
// TestReadinessRequiresCommitAndClearsBeforeChildStop (pin :151) is in service_manager_test.go
// (task 2.1, part 4). The two commit tests, which drove
// the pin's private startup steps and flags, now drive StartAll and read the main listener.

// foreignStartupGauge returns a collector under the startup gauge's name that the startup metric
// writer did not create.
func foreignStartupGauge(labels []string) *prometheus.GaugeVec {
	return prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "semengine", Subsystem: "startup", Name: "units",
		Help: "Process-local startup unit counts by manager owner and fixed lifecycle stage",
	}, labels)
}

// TestStartupMetricWriterInitializesExactlyNinePrivateSeries (pin :27).
func TestStartupMetricWriterInitializesExactlyNinePrivateSeries(t *testing.T) {
	registry := metric.NewMetricsRegistry()
	writer, err := newStartupMetricWriter(
		registry,
		func() serviceStartupCounts { return serviceStartupCounts{Admitted: 3} },
		func() startupUnitCounts {
			return startupUnitCounts{Admitted: 4, LifecycleParticipants: 2}
		},
	)
	require.NoError(t, err)
	require.NotNil(t, writer)

	family := requireMetricFamily(t, registry, "semengine_startup_units")
	require.Len(t, family.Metric, 9)
	require.Equal(t, float64(3), requireGauge(t, registry, "semengine_startup_units", map[string]string{
		"owner": "services", "stage": "admitted",
	}))
	require.Equal(t, float64(4), requireGauge(t, registry, "semengine_startup_units", map[string]string{
		"owner": "components", "stage": "admitted",
	}))
	require.Equal(t, float64(2), requireGauge(t, registry, "semengine_startup_units", map[string]string{
		"owner": "components", "stage": "lifecycle_participants",
	}))
}

// TestStartupMetricWriterRejectsForeignCollectorInsteadOfAdoptingIt (pin :52): the writer
// registers directly on the injected registry and refuses a gauge it did not create, which
// metric.RegisterOrGet would adopt (design D4, S13).
func TestStartupMetricWriterRejectsForeignCollectorInsteadOfAdoptingIt(t *testing.T) {
	for _, labels := range [][]string{{"owner", "stage"}, {"owner", "stage", "unit"}} {
		t.Run(labels[len(labels)-1], func(t *testing.T) {
			registry := metric.NewMetricsRegistry()
			require.NoError(t, registry.PrometheusRegistry().Register(foreignStartupGauge(labels)))

			writer, err := newStartupMetricWriter(
				registry,
				func() serviceStartupCounts { return serviceStartupCounts{} },
				func() startupUnitCounts { return startupUnitCounts{} },
			)
			require.Error(t, err)
			require.Nil(t, writer)
		})
	}
}

// TestMetricCoreExportsNoStartupLifecycleWriter (pin :73).
func TestMetricCoreExportsNoStartupLifecycleWriter(t *testing.T) {
	metricsType := reflect.TypeOf((*metric.Metrics)(nil))
	_, exists := metricsType.MethodByName("RecordStartupUnits")
	require.False(t, exists)
}

// TestStartupGaugeVecHasOnlyPrivateServicePackageOwner (pin :79). The pin matched the struct
// field with gofmt's exact padding; the field is matched with any spacing here, so a realigned
// struct cannot pass the holder check by no longer matching.
func TestStartupGaugeVecHasOnlyPrivateServicePackageOwner(t *testing.T) {
	for _, path := range []string{"../metric/core.go", "../metric/registry.go"} {
		source, err := os.ReadFile(path)
		require.NoError(t, err)
		require.NotContains(t, string(source), "RecordStartupUnits")
		require.NotContains(t, string(source), "startupUnits")
	}

	holder := regexp.MustCompile(`(?m)^\s*units\s+\*prometheus\.GaugeVec\s*$`)
	paths, err := filepath.Glob("*.go")
	require.NoError(t, err)
	holders := make([]string, 0, 1)
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		source, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		if holder.Match(source) {
			holders = append(holders, path)
		}
	}
	require.Equal(t, []string{"startup_metrics.go"}, holders)
}

// TestStartupMetricRegistrationConflictFailsBeforeListenersOrChildren (pin :103): a foreign
// startup gauge fails StartAll before any listener is bound or service started.
func TestStartupMetricRegistrationConflictFailsBeforeListenersOrChildren(t *testing.T) {
	for _, labels := range [][]string{{"owner", "stage"}, {"owner", "stage", "unit"}} {
		t.Run(labels[len(labels)-1], func(t *testing.T) {
			deps := createTestServiceDependencies()
			require.NoError(t, deps.MetricsRegistry.PrometheusRegistry().Register(foreignStartupGauge(labels)))
			manager := createTestServiceManager(ManagerConfig{}, deps)
			componentManager := newGatedStartupService("component-manager", nil)
			require.NoError(t, manager.RegisterInstance("component-manager", componentManager))
			stopManagerAtEnd(t, manager)

			err := manager.StartAll(t.Context())
			require.ErrorContains(t, err, "register fresh startup metric collector")
			require.Zero(t, componentManager.starts.Load())
			manager.mu.RLock()
			require.Nil(t, manager.httpListener)
			manager.mu.RUnlock()
		})
	}
}

// assertStartupOnly requires that the main listener at baseURL serves only the startup routes:
// /readyz and the widgets service's route both answer 503 NOT READY (pin :480).
func assertStartupOnly(t *testing.T, baseURL string) {
	t.Helper()
	requireAnswer(t, baseURL+"/readyz", http.StatusServiceUnavailable, "NOT READY")
	requireAnswer(t, baseURL+"/widgets/committed", http.StatusServiceUnavailable, "NOT READY")
}

// TestPreparedFullMuxStaysInvisibleUntilCommit: while the full route set is being prepared (a
// service's RegisterHTTPHandlers blocks, a call production makes), the main listener serves only
// the startup routes; after StartAll returned, the service's route answers and /readyz answers
// READY (design D2, "Test-only code"; pin :197). The pin also looked between the end of the
// preparation and the commit through the dropped testCommitPrepared hooks; no production call
// pauses there.
//
// spec: operator-surface / The service manager serves only diagnostics until boot commits
func TestPreparedFullMuxStaysInvisibleUntilCommit(t *testing.T) {
	release := newReleaser(t)
	registered := make(chan struct{})
	var once sync.Once
	widgets := newRoutedService("widgets", func(prefix string, mux *http.ServeMux) {
		once.Do(func() { close(registered) })
		<-release.ch
		mux.HandleFunc(prefix+"/committed", answer(http.StatusNoContent, ""))
	})
	manager := createTestServiceManager(ManagerConfig{}, nil)
	require.NoError(t, manager.RegisterInstance("component-manager", newStandInComponentManager()))
	require.NoError(t, manager.RegisterInstance("widgets", widgets))

	start := beginObservedManagerStart(t, manager, release.release)
	start.requireSignal(t, registered, "widgets RegisterHTTPHandlers entry")
	url := observedManagerHTTPURL(t, manager)
	assertStartupOnly(t, url)

	release.release()
	require.NoError(t, start.wait(t))
	requireAnswer(t, url+"/readyz", http.StatusOK, "READY")
	requireAnswer(t, url+"/widgets/committed", http.StatusNoContent, "")
}

// stopTimeAnswers is what a rollback Stop saw on the main listener.
type stopTimeAnswers struct {
	listenerBound bool
	ready         getResult
	route         getResult
}

// TestHealthPublisherFailureNeverCommitsPreparedRoutes: when StartAll fails after preparing the
// full route set and before committing it, no request is served by the prepared routes, even
// while the rollback stops the services and the main listener still serves (design D6; pin :241,
// which forced the publisher's failure through the dropped healthPublisherUsed flag). Here the
// widgets service ends StartAll's context while its routes are prepared, so the publisher, started
// under that context, or the commit is refused; services stop before the listeners, so the
// rollback's Stop of widgets can request both routes.
//
// spec: operator-surface / The service manager serves only diagnostics until boot commits
func TestHealthPublisherFailureNeverCommitsPreparedRoutes(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	manager := createTestServiceManager(ManagerConfig{}, nil)
	var served atomic.Bool
	var answers stopTimeAnswers
	widgets := newRoutedService("widgets", func(prefix string, mux *http.ServeMux) {
		mux.HandleFunc(prefix+"/committed", func(w http.ResponseWriter, _ *http.Request) {
			served.Store(true)
			w.WriteHeader(http.StatusNoContent)
		})
		cancel()
	})
	widgets.stop = func(context.Context) error {
		manager.mu.RLock()
		listener := manager.httpListener
		manager.mu.RUnlock()
		if listener == nil {
			return nil
		}
		answers.listenerBound = true
		url := "http://" + listener.Addr().String()
		answers.ready = <-getAsync(t.Context(), url+"/readyz")
		answers.route = <-getAsync(t.Context(), url+"/widgets/committed")
		return nil
	}
	require.NoError(t, manager.RegisterInstance("component-manager", newStandInComponentManager()))
	require.NoError(t, manager.RegisterInstance("widgets", widgets))
	stopManagerAtEnd(t, manager)

	err := manager.StartAll(ctx)
	require.ErrorIs(t, err, context.Canceled)
	require.True(t, answers.listenerBound, "the main listener was released before the services stopped")
	require.NoError(t, answers.ready.err)
	require.Equal(t, http.StatusServiceUnavailable, answers.ready.status)
	require.Equal(t, "NOT READY", answers.ready.body)
	require.NoError(t, answers.route.err)
	require.Equal(t, http.StatusServiceUnavailable, answers.route.status, "a prepared route was served")
	require.Equal(t, "NOT READY", answers.route.body)
	require.False(t, served.Load(), "a prepared route's handler ran")
}

// orderGateService is a running, healthy Service whose Start and Stop each signal entry and
// block until released (pin :440).
type orderGateService struct {
	MockService
	startEntered chan struct{}
	startRelease chan struct{}
	stopEntered  chan struct{}
	stopRelease  chan struct{}
}

func newOrderGateService(name string) *orderGateService {
	return &orderGateService{
		MockService:  MockService{name: name, status: StatusRunning, healthy: true},
		startEntered: make(chan struct{}),
		startRelease: make(chan struct{}),
		stopEntered:  make(chan struct{}),
		stopRelease:  make(chan struct{}),
	}
}

func (s *orderGateService) Start(context.Context) error {
	close(s.startEntered)
	<-s.startRelease
	return nil
}

func (s *orderGateService) Stop(context.Context) error {
	close(s.stopEntered)
	<-s.stopRelease
	return nil
}

// requireClosed waits for signal.
func requireClosed(t *testing.T, signal <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(stopBound):
		t.Fatalf("%s was not observed", what)
	}
}

// TestConcreteMetricsLifecycleRetainsRegistrationAndReverseStopOrder: the real Metrics,
// registered between two services, starts after the first and before the second and stops
// after the second and before the first, while its claimed server already serves when the first
// starts (pin :369, which read Metrics' private lifecycle flags; here its Status).
func TestConcreteMetricsLifecycleRetainsRegistrationAndReverseStopOrder(t *testing.T) {
	deps := createTestServiceDependencies()
	manager := createTestServiceManager(ManagerConfig{}, deps)
	componentManager := newOrderGateService("component-manager")
	later := newOrderGateService("later")
	metrics := newListenerMetrics(t, deps.MetricsRegistry)
	metricsAddr := metrics.testListener.Addr().String()
	require.NoError(t, manager.RegisterInstance("component-manager", componentManager))
	require.NoError(t, manager.RegisterInstance("metrics", metrics))
	require.NoError(t, manager.RegisterInstance("later", later))

	var startFirstOnce, startLaterOnce, stopFirstOnce, stopLaterOnce sync.Once
	releaseStartFirst := func() { startFirstOnce.Do(func() { close(componentManager.startRelease) }) }
	releaseStartLater := func() { startLaterOnce.Do(func() { close(later.startRelease) }) }
	releaseStopFirst := func() { stopFirstOnce.Do(func() { close(componentManager.stopRelease) }) }
	releaseStopLater := func() { stopLaterOnce.Do(func() { close(later.stopRelease) }) }
	start := beginObservedManagerStart(t, manager, func() {
		releaseStartFirst()
		releaseStartLater()
		releaseStopFirst()
		releaseStopLater()
	})
	start.requireSignal(t, componentManager.startEntered, "component-manager Start entry")
	require.Equal(t, "http://"+metricsAddr+"/metrics", observedManagerMetricsURL(t, manager))
	require.NotEqual(t, StatusRunning, metrics.Status(), "metrics started before the service registered ahead of it")
	select {
	case <-later.startEntered:
		t.Fatal("later service started before the first registered service was released")
	default:
	}
	releaseStartFirst()
	start.requireSignal(t, later.startEntered, "later service Start entry")
	require.Equal(t, StatusRunning, metrics.Status(), "metrics had not started when the service after it started")
	releaseStartLater()
	require.NoError(t, start.wait(t))

	stopCtx := stopContext(t)
	stopDone := make(chan error, 1)
	go func() { stopDone <- manager.StopAll(stopCtx) }()
	requireClosed(t, later.stopEntered, "later service Stop entry")
	require.Equal(t, StatusRunning, metrics.Status(), "metrics stopped before the service registered after it")
	select {
	case <-componentManager.stopEntered:
		t.Fatal("component-manager stopped before the later service was released")
	default:
	}
	releaseStopLater()
	requireClosed(t, componentManager.stopEntered, "component-manager Stop entry")
	require.NotEqual(t, StatusRunning, metrics.Status(), "metrics had not stopped when the service before it stopped")
	releaseStopFirst()
	require.NoError(t, <-stopDone)
}

// requireMetricFamily returns the gathered family named name (pin :498).
func requireMetricFamily(t *testing.T, registry *metric.MetricsRegistry, name string) *dto.MetricFamily {
	t.Helper()
	families, err := registry.PrometheusRegistry().Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() == name {
			return family
		}
	}
	t.Fatalf("metric family %s not found", name)
	return nil
}
