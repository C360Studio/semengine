package service

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/metric"
)

// The override-expiry reporter's tests, from the pin's stream_override_expiry_test.go (design D4,
// D7). The pin composed its managers over an embedded NATS server (:146-170), which is not carried
// (sweep C14): a manager here is built through NewComponentManager over an unconnected client, which
// is all construction and the supervisor need. The pin's tests that waited on wall-clock windows
// run inside a testing/synctest bubble, where synctest.Wait settles every goroutine the manager
// started and <-time.After advances the bubble's fake clock; no nats.go connection runs in a bubble
// (docs/testing.md), since the client never connects.

// overrideExpiryMetric is the gauge the reporter registers.
const overrideExpiryMetric = "semengine_streams_migration_override_expired"

func expiryReporter(t *testing.T, cfg *config.Config) (*streamOverrideExpiryReporter, *strings.Builder, *metric.MetricsRegistry) {
	t.Helper()
	logs := &strings.Builder{}
	r := newStreamOverrideExpiryReporter(
		func() *config.Config { return cfg },
		slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
	)
	registry := metric.NewMetricsRegistry()
	require.NoError(t, r.register(registry))
	return r, logs, registry
}

func overrideConfig(expires string) *config.Config {
	return &config.Config{
		StreamMigrationOverrides: config.StreamMigrationOverrides{
			"LEGACY": {Owner: "team-legacy", Expires: expires, Reason: "sizing study"},
		},
	}
}

// TestOverrideExpiry_CrossesTheDeadlineWithoutRestart: the same configuration, evaluated either
// side of the deadline, reports the lapse without a restart. Boot-time validation cannot see this
// transition at all.
//
// spec: service-lifecycle / A lapsed stream migration override is reported while running
func TestOverrideExpiry_CrossesTheDeadlineWithoutRestart(t *testing.T) {
	r, logs, registry := expiryReporter(t, overrideConfig("2026-09-30"))
	labels := map[string]string{"stream": "LEGACY", "owner": "team-legacy"}

	r.evaluate(time.Date(2026, 9, 30, 23, 59, 59, 0, time.UTC))
	require.Equal(t, 0.0, requireGauge(t, registry, overrideExpiryMetric, labels),
		"an open bridge reports zero: the series must exist before it matters, or the alert cannot be tested")
	require.NotContains(t, logs.String(), "EXPIRED")

	logs.Reset()
	r.evaluate(time.Date(2026, 10, 1, 4, 0, 0, 0, time.UTC))

	require.Equal(t, 1.0, requireGauge(t, registry, overrideExpiryMetric, labels),
		"one process, one configuration, no restart: the same evaluation now reports the lapse")
	logged := logs.String()
	require.Contains(t, logged, "EXPIRED")
	require.Contains(t, logged, "LEGACY")
	require.Contains(t, logged, "team-legacy", "the remedy needs an addressee")
	require.Contains(t, logged, "next boot will refuse to start",
		"the operator must be told where enforcement actually lands")
	require.Contains(t, logged, "archival_streams", "and be given the escape if permanence is the contract")
}

// TestOverrideExpiry_ReportsOnEveryTick: a lapse is reported on every evaluation, not once. The
// gauge is what an alert reads, but the log is what someone searches hours later.
func TestOverrideExpiry_ReportsOnEveryTick(t *testing.T) {
	r, logs, _ := expiryReporter(t, overrideConfig("2026-09-30"))

	for i := range 3 {
		logs.Reset()
		r.evaluate(time.Date(2026, 10, 1, 4+i, 0, 0, 0, time.UTC))
		require.Contains(t, logs.String(), "EXPIRED", "tick %d must report", i)
	}
}

// TestOverrideExpiry_ClearsWhenTheBridgeIsRenewed: an operator may extend an override without
// restarting, and the gauge then stops reporting the lapse.
func TestOverrideExpiry_ClearsWhenTheBridgeIsRenewed(t *testing.T) {
	cfg := overrideConfig("2026-09-30")
	r, _, registry := expiryReporter(t, cfg)
	labels := map[string]string{"stream": "LEGACY", "owner": "team-legacy"}
	now := time.Date(2026, 10, 1, 4, 0, 0, 0, time.UTC)

	r.evaluate(now)
	require.Equal(t, 1.0, requireGauge(t, registry, overrideExpiryMetric, labels))

	// The operator extends it, live.
	cfg.StreamMigrationOverrides["LEGACY"] = config.StreamMigrationOverride{
		Owner: "team-legacy", Expires: "2027-03-01", Reason: "sizing study",
	}
	r.evaluate(now)

	require.Equal(t, 0.0, requireGauge(t, registry, overrideExpiryMetric, labels),
		"a renewed bridge must stop reporting without a restart")
}

// TestOverrideExpiry_RemovedOverrideStopsReporting: an override the operator deleted leaves no
// series behind.
func TestOverrideExpiry_RemovedOverrideStopsReporting(t *testing.T) {
	cfg := overrideConfig("2026-09-30")
	r, _, registry := expiryReporter(t, cfg)
	labels := map[string]string{"stream": "LEGACY", "owner": "team-legacy"}
	now := time.Date(2026, 10, 1, 4, 0, 0, 0, time.UTC)

	r.evaluate(now)
	require.Equal(t, 1.0, requireGauge(t, registry, overrideExpiryMetric, labels))

	cfg.StreamMigrationOverrides = nil
	r.evaluate(now)

	_, ok := gaugeValue(t, registry, overrideExpiryMetric, labels)
	require.False(t, ok, "an override the operator deleted must not keep a series standing")
}

// composeComponentManagerWithOverride builds a component manager through NewComponentManager,
// the production composition path, over a configuration that declares overrides and enables
// components, logging to logger and registering its metrics on metrics. The property under test
// is the wiring, not the reporter: a reporter-only assertion would keep passing after the wiring
// was dropped. Called inside a synctest bubble it builds everything in the bubble.
func composeComponentManagerWithOverride(
	t *testing.T, overrides config.StreamMigrationOverrides, logger *slog.Logger, metrics *metric.MetricsRegistry,
	components ...testComponent,
) *ComponentManager {
	t.Helper()
	registry := component.NewRegistry()
	names := make([]string, 0, len(components))
	for _, c := range components {
		admitTestRegistryComponent(t, registry, c.name, c.instance)
		names = append(names, c.name)
	}
	deps := testComponentDependencies(t, registry, names...)

	// testComponentDependencies' configuration, with the overrides declared.
	bootConfig := deps.Manager.GetConfig().Get()
	bootConfig.StreamMigrationOverrides = overrides
	configManager, err := config.NewConfigManager(bootConfig, deps.NATSClient, logger)
	require.NoError(t, err)
	deps.Manager = configManager
	deps.Logger = logger
	deps.MetricsRegistry = metrics

	built, err := NewComponentManager(nil, deps)
	require.NoError(t, err)
	cm, ok := built.(*ComponentManager)
	require.True(t, ok, "NewComponentManager returned %T, want *ComponentManager", built)
	return cm
}

// TestComponentManagerRegistersOverrideExpiryGaugeAtConstruction: NewComponentManager registers
// the gauge on the registry the /metrics endpoint scrapes and evaluates once, so every declared
// bridge has a series before Start: 1 and a Warn line for a lapsed one, 0 for an open one (design
// D4). The component manager hosts the reporter because it is the one service every composition
// has (service-lifecycle, "Every composition has a component manager"). The pin named this test
// TestStreamOverrideExpiryReporterRegistersWithoutFlowService, after the SemStreams service that
// hosted the reporter before.
//
// spec: service-lifecycle / A lapsed stream migration override is reported while running
func TestComponentManagerRegistersOverrideExpiryGaugeAtConstruction(t *testing.T) {
	logs := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	metrics := metric.NewMetricsRegistry()

	composeComponentManagerWithOverride(t, config.StreamMigrationOverrides{
		"LAPSED": {Owner: "team-legacy", Expires: "2020-01-01", Reason: "sizing study"},
		"OPEN":   {Owner: "team-current", Expires: "2999-01-01", Reason: "sizing study"},
	}, logger, metrics)

	require.Equal(t, 1.0,
		requireGauge(t, metrics, overrideExpiryMetric, map[string]string{"stream": "LAPSED", "owner": "team-legacy"}),
		"a lapsed bridge reports 1 from construction")
	require.Equal(t, 0.0,
		requireGauge(t, metrics, overrideExpiryMetric, map[string]string{"stream": "OPEN", "owner": "team-current"}),
		"an open bridge reports zero so the alert series exists before it matters")
	require.Contains(t, logs.String(), "EXPIRED", "construction's evaluation logs the lapse")
}

// syncBuffer is a log sink the manager's goroutines write while the test reads.
type syncBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestComponentManagerStartRunsOverrideExpiryReporter: registration alone would leave a series
// frozen at its construction value. A bridge that is open at construction and at Start lapses
// half an interval later; one interval after Start the running loop has re-evaluated the live
// configuration and reports it, without a restart (design D4). The pin polled the log for up to 5
// seconds; the bubble's fake clock makes the interval exact.
//
// spec: service-lifecycle / A lapsed stream migration override is reported while running
func TestComponentManagerStartRunsOverrideExpiryReporter(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		logs := &syncBuffer{}
		logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
		metrics := metric.NewMetricsRegistry()
		labels := map[string]string{"stream": "LAPSING", "owner": "team-legacy"}

		expires := time.Now().Add(overrideExpiryCheckInterval / 2).UTC().Format(time.RFC3339)
		cm := composeComponentManagerWithOverride(t, config.StreamMigrationOverrides{
			"LAPSING": {Owner: "team-legacy", Expires: expires, Reason: "sizing study"},
		}, logger, metrics)
		require.Equal(t, 0.0, requireGauge(t, metrics, overrideExpiryMetric, labels),
			"construction did not report the open bridge")

		require.NoError(t, cm.Start(t.Context()))
		synctest.Wait()
		require.Equal(t, 0.0, requireGauge(t, metrics, overrideExpiryMetric, labels),
			"the bridge is still open at Start")
		require.NotContains(t, logs.String(), "EXPIRED")

		<-time.After(overrideExpiryCheckInterval)
		synctest.Wait()
		require.Equal(t, 1.0, requireGauge(t, metrics, overrideExpiryMetric, labels),
			"one interval after Start the running loop did not report the lapse")
		require.Contains(t, logs.String(), "EXPIRED")
		require.Contains(t, logs.String(), "LAPSING")

		require.NoError(t, cm.Stop(stopContext(t)))
	})
}

// heldConfigSource blocks the reporter inside its first evaluation until the test releases it,
// and reports when the loop got there. The real reporter returns on cancellation at once, so a
// supervisor that released its done channel early and one that released it correctly look the
// same; only a loop the test holds open, through the reporter's own configuration source, proves
// the join. The pin's lever, kept: same reporter type, same launch path.
type heldConfigSource struct {
	cfg      *config.Config
	entered  chan struct{}
	release  chan struct{}
	enterOne sync.Once
	calls    atomic.Int64
}

func newHeldConfigSource(cfg *config.Config) *heldConfigSource {
	return &heldConfigSource{
		cfg:     cfg,
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (h *heldConfigSource) get() *config.Config {
	h.calls.Add(1)
	h.enterOne.Do(func() { close(h.entered) })
	<-h.release
	return h.cfg
}

// TestSuperviseHoldsDoneUntilTheOverrideExpiryLoopReturns asserts on the channel Stop joins: done
// stays open while a launched loop still runs (design D4). The pin gave the health loop 50 ms to
// return before looking; synctest.Wait returns only once the health loop has returned and supervise
// is blocked, so the look is exact.
//
// spec: service-lifecycle / A lapsed stream migration override is reported while running
func TestSuperviseHoldsDoneUntilTheOverrideExpiryLoopReturns(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		source := newHeldConfigSource(&config.Config{})
		cm := composeComponentManagerWithOverride(t, nil, discardLogger, metric.NewMetricsRegistry())
		cm.overrideExpiry = newStreamOverrideExpiryReporter(source.get, discardLogger)

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan struct{})
		go cm.supervise(ctx, done)

		<-source.entered // the loop is inside evaluate, held
		cancel()         // the health loop returns; the held loop cannot
		synctest.Wait()

		select {
		case <-done:
			close(source.release)
			t.Fatal("supervise released done while the override-expiry loop was still running: " +
				"Stop would return with a live goroutine behind it")
		default:
		}

		close(source.release)
		waitForSignal(t, done, "supervise to release done after the loop returned")
	})
}

// TestComponentManagerStopWaitsForTheOverrideExpiryLoop drives Start and Stop rather than
// supervise alone: Stop does not return while the override-expiry loop is inside an evaluation,
// and returns nil once it is released (design D4). The pin's 250 ms window becomes synctest.Wait,
// which returns only once Stop is blocked.
//
// spec: service-lifecycle / A lapsed stream migration override is reported while running
func TestComponentManagerStopWaitsForTheOverrideExpiryLoop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		source := newHeldConfigSource(&config.Config{})
		cm := composeComponentManagerWithOverride(t, nil, discardLogger, metric.NewMetricsRegistry())
		// The reporter's configuration source swapped for one this test holds: same reporter type
		// and same launch path, only the source is under control.
		cm.overrideExpiry = newStreamOverrideExpiryReporter(source.get, discardLogger)

		require.NoError(t, cm.Start(t.Context()))
		<-source.entered

		stopped := make(chan error, 1)
		go func() { stopped <- cm.Stop(stopContext(t)) }()
		synctest.Wait()

		select {
		case err := <-stopped:
			close(source.release)
			t.Fatalf("Stop returned (%v) while the override-expiry loop was still inside evaluate", err)
		default:
		}

		close(source.release)
		require.NoError(t, <-stopped)
	})
}

// TestComponentManagerFailedStartDoesNotLaunchOverrideExpiryLoop: a Start that fails, here because
// a component's Start fails, leaves no supervisor behind: the reporter never read the
// configuration, no health was published, and the done channel is nil or closed (design D4). The
// pin failed Start on a struct-literal manager it had not initialized; the bubble would also fail
// the test if any goroutine the Start launched were left running.
//
// spec: service-lifecycle / Component health is published and its drops counted
func TestComponentManagerFailedStartDoesNotLaunchOverrideExpiryLoop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		source := newHeldConfigSource(&config.Config{})
		close(source.release) // never block; this test counts calls, it does not hold
		metrics := metric.NewMetricsRegistry()
		failing := &suiteComponent{name: "supervisor-fails", startErr: errSuiteComponentStart}
		cm := composeComponentManagerWithOverride(t, nil, discardLogger, metrics,
			testComponent{name: failing.name, instance: failing})
		cm.overrideExpiry = newStreamOverrideExpiryReporter(source.get, discardLogger)

		require.ErrorContains(t, cm.Start(t.Context()), `component "supervisor-fails"`)
		synctest.Wait()

		require.Zero(t, source.calls.Load(),
			"a failed Start launched the override-expiry loop anyway: it read the configuration %d time(s)",
			source.calls.Load())
		done := cm.supervisorDone
		require.True(t, done == nil || isClosed(done), "a failed Start left a supervisor to join")
		published, _ := gaugeValue(t, metrics, "semengine_errors_total",
			map[string]string{"service": "component-manager", "type": "health_publish"})
		require.Zero(t, published, "a failed Start launched the health publisher anyway")
	})
}
