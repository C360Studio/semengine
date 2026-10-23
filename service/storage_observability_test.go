package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/health"
	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient/storagereport"
	"github.com/c360studio/semengine/pkg/errs"
)

// The storage observability service's configuration, registration and lifecycle edges (design D2,
// D3, D7 U-b). Configuration and registration tests are carried from the pin
// (storage_observability_test.go:128-235); the pin's refusal tests on the applied form
// (:193-224, :237-241) become TestStorageObservabilityConstructionRefusals, which drives every
// refusal through the one constructor.

// The two warnings the loops log when the account cannot be read: the inventory collector's
// (natsclient/storagereport/storage_inventory.go:329) and the report consumer's
// (storage_report_consumer.go:215). The tests wait on these records, never on a clock.
const (
	storageCollectionFailedLog = "storage inventory collection failed; serving last known result"
	storageWatchFailedLog      = "could not watch the storage report; operator surfaces are serving their last values"
)

// storageLoopBound bounds a wait for a loop's first warning: the collector collects at once and
// the consumer's first watch is attempted at once, so 10 s is ten one-second intervals of slack
// (design D7, the real-clock windows).
const storageLoopBound = 10 * time.Second

// storageLoopsConfig collects every second and gives a collection one second, so a loop's retry
// is due well inside storageLoopBound.
const storageLoopsConfig = `{"interval":"1s","timeout":"1s"}`

// awaitStorageLog fails the test unless ch closes within storageLoopBound.
func awaitStorageLog(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	timer := time.NewTimer(storageLoopBound)
	defer timer.Stop()
	select {
	case <-ch:
	case <-timer.C:
		t.Fatalf("%s: not within %s", what, storageLoopBound)
	case <-t.Context().Done():
		t.Fatalf("%s: test context ended", what)
	}
}

// stopStorageObservabilityAtCleanup stops s when the test ends, under a bounded context detached
// from the test's, which has ended by then.
func stopStorageObservabilityAtCleanup(t *testing.T, s *StorageObservabilityService) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), stopBound)
		defer cancel()
		if err := s.Stop(ctx); err != nil {
			t.Errorf("cleanup Stop: %v", err)
		}
	})
}

// --- configuration (pin :128-224) -------------------------------------------

// TestStorageObservabilityConfig_JSONRoundTrip carries pin :128-176: the collection interval and
// every threshold are reachable from a config block, survive a re-encode, and reach the applied
// form the pressure evaluation reads.
func TestStorageObservabilityConfig_JSONRoundTrip(t *testing.T) {
	raw := []byte(`{
		"interval": "30s",
		"timeout": "9s",
		"pressure_thresholds": {
			"warning_headroom": 0.4,
			"high_headroom": 0.3,
			"critical_headroom": 0.2,
			"warning_horizon": "48h",
			"high_horizon": "12h",
			"critical_horizon": "1h"
		}
	}`)

	var decoded StorageObservabilityConfig
	require.NoError(t, json.Unmarshal(raw, &decoded))

	assert.Equal(t, "30s", decoded.Interval)
	assert.Equal(t, "9s", decoded.Timeout)
	assert.InDelta(t, 0.4, decoded.PressureThresholds.WarningHeadroom, 1e-9)
	assert.InDelta(t, 0.3, decoded.PressureThresholds.HighHeadroom, 1e-9)
	assert.InDelta(t, 0.2, decoded.PressureThresholds.CriticalHeadroom, 1e-9)
	assert.Equal(t, "48h", decoded.PressureThresholds.WarningHorizon)
	assert.Equal(t, "12h", decoded.PressureThresholds.HighHorizon)
	assert.Equal(t, "1h", decoded.PressureThresholds.CriticalHorizon)

	// A field that survives one direction but not the other is still unreachable to the operator
	// who edits it back.
	encoded, err := json.Marshal(decoded)
	require.NoError(t, err)

	var again StorageObservabilityConfig
	require.NoError(t, json.Unmarshal(encoded, &again))
	assert.Equal(t, decoded, again)

	resolved, err := again.PressureThresholds.Resolve()
	require.NoError(t, err)
	assert.InDelta(t, 0.4, resolved.WarningHeadroom, 1e-9)
	assert.Equal(t, 48*time.Hour, resolved.WarningHorizon)
	assert.Equal(t, time.Hour, resolved.CriticalHorizon)

	// The constructor accepts the same block: every key in it is one the service defines.
	_, err = NewStorageObservabilityService(raw, &Dependencies{
		NATSClient:      newUnconnectedClient(t),
		MetricsRegistry: metric.NewMetricsRegistry(),
	})
	require.NoError(t, err)
}

// TestStorageObservabilityConfig_OmittedFieldsTakeDocumentedDefaults carries pin :178-191: an
// empty block applies a one-minute interval, a 15 s timeout and the shipped thresholds (design D3,
// "Defaults have one home"). The oracle is the literal durations and storagereport's own defaults,
// resolved, in place of the pin's two constants that are unexported there now.
func TestStorageObservabilityConfig_OmittedFieldsTakeDocumentedDefaults(t *testing.T) {
	var cfg StorageObservabilityConfig
	require.NoError(t, json.Unmarshal([]byte(`{}`), &cfg))

	applied, err := cfg.applied()
	require.NoError(t, err)
	assert.Equal(t, time.Minute, applied.interval)
	assert.Equal(t, 15*time.Second, applied.timeout)

	want, err := storagereport.DefaultStoragePressureThresholds().Resolve()
	require.NoError(t, err)
	got, err := applied.thresholds.Resolve()
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

// TestStorageObservabilityService_IsRegistered carries pin :226-235: RegisterAll offers the
// service under its config-block name, the only way an operator turns it on.
func TestStorageObservabilityService_IsRegistered(t *testing.T) {
	registry := NewServiceRegistry()
	require.NoError(t, RegisterAll(registry))

	constructor, ok := registry.Constructor(StorageObservabilityServiceName)
	require.True(t, ok, "the service must be registered under its config-block name")
	require.NotNil(t, constructor)
}

// TestStorageObservabilityConstructionRefusals is design D3's refusal list, replacing pin
// :193-224 and :237-241. Each refusal is invalid data (errs.IsInvalid) and names the input it
// refuses. The pin accepted the three key cases (probe § 3) and refused the rest unclassified.
func TestStorageObservabilityConstructionRefusals(t *testing.T) {
	type refusal struct {
		config string
		deps   func(t *testing.T) *Dependencies
		names  string
	}
	full := func(t *testing.T) *Dependencies {
		return &Dependencies{NATSClient: newUnconnectedClient(t), MetricsRegistry: metric.NewMetricsRegistry()}
	}
	withConfig := func(config, names string) refusal { return refusal{config: config, deps: full, names: names} }

	cases := map[string]refusal{
		"nil dependencies": {deps: func(*testing.T) *Dependencies { return nil }, names: "dependencies"},
		"nil NATS client": {deps: func(*testing.T) *Dependencies {
			return &Dependencies{MetricsRegistry: metric.NewMetricsRegistry()}
		}, names: "NATS client"},
		"nil metrics registry": {deps: func(t *testing.T) *Dependencies {
			return &Dependencies{NATSClient: newUnconnectedClient(t)}
		}, names: "metrics registry"},
		"unparseable interval":     withConfig(`{"interval":"soon"}`, "interval"),
		"non-positive interval":    withConfig(`{"interval":"0s"}`, "interval"),
		"unparseable timeout":      withConfig(`{"timeout":"later"}`, "timeout"),
		"non-positive timeout":     withConfig(`{"timeout":"-1s"}`, "timeout"),
		"timeout above interval":   withConfig(`{"interval":"5s","timeout":"30s"}`, "timeout"),
		"inverted headroom":        withConfig(`{"pressure_thresholds":{"warning_headroom":0.1,"high_headroom":0.5}}`, "high_headroom"),
		"headroom out of range":    withConfig(`{"pressure_thresholds":{"warning_headroom":1.5}}`, "warning_headroom"),
		"unparseable horizon":      withConfig(`{"pressure_thresholds":{"warning_horizon":"soon"}}`, "warning_horizon"),
		"unknown key":              withConfig(`{"intervall":"1m"}`, "intervall"),
		"unknown nested key":       withConfig(`{"pressure_thresholds":{"warning_headrom":0.3}}`, "warning_headrom"),
		"key in other letter case": withConfig(`{"Interval":"1m"}`, "Interval"),
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			built, err := NewStorageObservabilityService(json.RawMessage(tc.config), tc.deps(t))
			require.Error(t, err, "construction must refuse, not default")
			require.Nil(t, built)
			assert.True(t, errs.IsInvalid(err), "want an invalid-data refusal, got %v", err)
			assert.Contains(t, err.Error(), tc.names, "the refusal names what it refuses")
		})
	}
}

// --- lifecycle edges (design D2) ---------------------------------------------

// TestStorageObservabilityFailedStopIsRetried is D2's "Stop": a Stop whose context ends before
// the loops return reports that context's error, and the next Stop waits for them again. The
// composition's logger holds the collector's first failure record, so the collector's goroutine
// is inside its log call and cannot return until released.
//
// The two 200 ms windows are this file's only real-clock windows besides the loop bound (design
// D7). The first Stop's deadline cannot fail a correct service: the held loop keeps the join from
// finishing, so it can only end at the deadline. The second window is an observation: a service
// that forgets its loops after a failed Stop returns nil from the second Stop at once; a correct
// one waits for the held loop.
func TestStorageObservabilityFailedStopIsRetried(t *testing.T) {
	const window = 200 * time.Millisecond
	handler := newHoldingHandler(t, storageCollectionFailedLog)
	s, _ := newTestStorageObservability(t, newUnconnectedClient(t), storageLoopsConfig, slog.New(handler))
	require.NoError(t, s.Start(t.Context()))
	stopStorageObservabilityAtCleanup(t, s)
	t.Cleanup(handler.releaseHeld)

	awaitStorageLog(t, handler.heldRecord(), "the collector's failure record held")

	firstCtx, cancelFirst := context.WithTimeout(t.Context(), window)
	defer cancelFirst()
	require.ErrorIs(t, s.Stop(firstCtx), context.DeadlineExceeded,
		"a Stop whose loops cannot return reports its context's error")

	secondCtx, cancelSecond := context.WithTimeout(t.Context(), stopBound)
	defer cancelSecond()
	secondResult := make(chan error, 1)
	go func() { secondResult <- s.Stop(secondCtx) }()
	observation := time.NewTimer(window)
	defer observation.Stop()
	select {
	case err := <-secondResult:
		t.Fatalf("the second Stop returned (%v) while the collector was held: the failed Stop forgot its loops", err)
	case <-observation.C:
	}

	handler.releaseHeld()
	require.NoError(t, <-secondResult, "the second Stop finishes the join the first left")
	require.NoError(t, s.Stop(stopContext(t)), "a Stop after one that returned nil returns nil")
	owner := &storageObservabilityOwner{s: s}
	assert.Empty(t, owner.Observe().Unresolved, "a stopped service holds no loop and no base status")
}

// TestStorageObservabilityBootsWithoutAReachableAccount is D2's "Startup work", carrying pin
// storage_observability_integration_test.go:318-346 onto a client that never connects: Start does
// no I/O and returns nil, both loops fail and log, and the service stays healthy and says it has
// read nothing. The pin's three-second sleep is replaced by the two warnings themselves, and its
// Snapshot accessor by the operator's route.
//
// The base service is unhealthy until its first health check, which runs on its own goroutine and
// is not ordered before the warnings (#385), so the test waits for a healthy verdict. A service an
// unreadable account made unhealthy never gives one, and the wait fails with the last status seen.
func TestStorageObservabilityBootsWithoutAReachableAccount(t *testing.T) {
	handler := newHoldingHandler(t, "")
	s, _ := newTestStorageObservability(t, newUnconnectedClient(t), storageLoopsConfig, slog.New(handler))
	collectorFailed := handler.arrived(storageCollectionFailedLog)
	watchFailed := handler.arrived(storageWatchFailedLog)

	require.NoError(t, s.Start(t.Context()), "Start must not depend on account state")
	stopStorageObservabilityAtCleanup(t, s)

	awaitStorageLog(t, collectorFailed, "the collector's failure warning")
	awaitStorageLog(t, watchFailed, "the consumer's watch warning")

	ctx, cancel := context.WithTimeout(t.Context(), storageLoopBound)
	defer cancel()
	status, err := probe.Await(ctx,
		func(context.Context) (health.Status, error) { return s.Health(), nil },
		func(status health.Status) bool { return status.Healthy })
	require.NoError(t, err, "an unreadable account must not make the service unhealthy")
	assert.Contains(t, status.Message, "storage report has not been read yet")

	mux := http.NewServeMux()
	s.RegisterHTTPHandlers("/storage-observability/", mux)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/storage-observability/report", nil))
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	synced, ok := body["synced"]
	require.True(t, ok, "the report states whether it was read: %s", recorder.Body.String())
	assert.JSONEq(t, `false`, string(synced), "nothing was read, and nothing claims otherwise")
}
