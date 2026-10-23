package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/pkg/errs"
)

// The heartbeat service's configuration, registration, composition, lifecycle edges and record
// (design D2-D5, D6 U-b). The pin's config and constructor tests (heartbeat_test.go:59-157) become
// TestHeartbeatConstructionRefusals; its Start refusals (:258-345) become
// TestHeartbeatStartRefusesWithoutTheComponentManager; its record tests (:379-470) read the record
// from Dependencies.Logger, never a swapped field. The pin's mock health getter (:17-26) and its
// test-only constructor are not carried (S10): every heartbeat is built through its registered
// constructor (newTestHeartbeat).

// heartbeatRecordMessage is the message of the heartbeat's one record (pin heartbeat.go:244).
const heartbeatRecordMessage = "System heartbeat"

// heartbeatRecord is one "System heartbeat" record: its level and its attributes, each as its
// value's string form.
type heartbeatRecord struct {
	level slog.Level
	attrs map[string]string
}

// heartbeatRecorder is an slog.Handler, enabled at every level, that keeps every "System
// heartbeat" record a heartbeat logs. It is the composition's logger (Dependencies.Logger), so a
// heartbeat that logs anywhere else leaves it empty.
type heartbeatRecorder struct {
	mu      sync.Mutex
	records []heartbeatRecord
}

func (r *heartbeatRecorder) Enabled(context.Context, slog.Level) bool { return true }

func (r *heartbeatRecorder) Handle(_ context.Context, record slog.Record) error {
	if record.Message != heartbeatRecordMessage {
		return nil
	}
	attrs := make(map[string]string, record.NumAttrs())
	record.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Resolve().String()
		return true
	})
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, heartbeatRecord{level: record.Level, attrs: attrs})
	return nil
}

// WithAttrs and WithGroup return the recorder itself: every logger derived from the composition's
// must reach the same records.
func (r *heartbeatRecorder) WithAttrs([]slog.Attr) slog.Handler { return r }
func (r *heartbeatRecorder) WithGroup(string) slog.Handler      { return r }

// recorded returns a copy of the records kept so far.
func (r *heartbeatRecorder) recorded() []heartbeatRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]heartbeatRecord(nil), r.records...)
}

// healthComponent is a composed component whose own health answer is healthy.
type healthComponent struct {
	baseDiscoverable
	healthy bool
}

func (c *healthComponent) Health() component.HealthStatus {
	return component.HealthStatus{Healthy: c.healthy}
}

// heartbeatTestManager returns a service manager with no services, the Dependencies.ServiceManager
// a direct construction needs.
func heartbeatTestManager() *Manager { return NewServiceManager(NewServiceRegistry()) }

// --- configuration and construction (design D3) ----------------------------

// TestHeartbeatConstructionRefusals is design D3's heartbeat list, replacing pin
// heartbeat_test.go:59-157. Each refusal is invalid data (errs.IsInvalid) and names the input it
// refuses; each accepted block builds a *HeartbeatService. The pin's empty-interval row now takes
// the default (its cadence is TestHeartbeatCadence's "default" rows).
func TestHeartbeatConstructionRefusals(t *testing.T) {
	withManager := func() *Dependencies { return &Dependencies{ServiceManager: heartbeatTestManager()} }
	noDeps := func() *Dependencies { return nil }
	noManager := func() *Dependencies { return &Dependencies{} }

	refusals := map[string]struct {
		config string
		deps   func() *Dependencies
		names  string
	}{
		"nil dependencies":         {config: `{}`, deps: noDeps, names: "dependencies"},
		"nil service manager":      {config: `{}`, deps: noManager, names: "service manager"},
		"unparseable document":     {config: `{invalid`, deps: withManager, names: "heartbeat"},
		"unparseable interval":     {config: `{"interval":"invalid"}`, deps: withManager, names: "interval"},
		"zero interval":            {config: `{"interval":"0s"}`, deps: withManager, names: "interval"},
		"negative interval":        {config: `{"interval":"-1s"}`, deps: withManager, names: "interval"},
		"interval under 1s":        {config: `{"interval":"500ms"}`, deps: withManager, names: "interval"},
		"interval just under 1s":   {config: `{"interval":"999ms"}`, deps: withManager, names: "interval"},
		"unknown key":              {config: `{"intervall":"30s"}`, deps: withManager, names: "intervall"},
		"key in other letter case": {config: `{"Interval":"30s"}`, deps: withManager, names: "Interval"},
	}
	for name, tc := range refusals {
		t.Run(name, func(t *testing.T) {
			built, err := NewHeartbeatService(json.RawMessage(tc.config), tc.deps())
			require.Error(t, err, "construction must refuse, not default")
			require.Nil(t, built)
			assert.True(t, errs.IsInvalid(err), "want an invalid-data refusal, got %v", err)
			assert.Contains(t, err.Error(), tc.names, "the refusal names what it refuses")
		})
	}

	accepted := map[string]string{
		"no document":     ``,
		"empty block":     `{}`,
		"empty interval":  `{"interval":""}`,
		"10s interval":    `{"interval":"10s"}`,
		"1m interval":     `{"interval":"1m"}`,
		"the 1s boundary": `{"interval":"1s"}`,
	}
	for name, config := range accepted {
		t.Run("accepts "+name, func(t *testing.T) {
			built, err := NewHeartbeatService(json.RawMessage(config), withManager())
			require.NoError(t, err)
			_, ok := built.(*HeartbeatService)
			require.True(t, ok, "NewHeartbeatService returned %T, want *HeartbeatService", built)
		})
	}
}

// TestHeartbeatRefusesAMiscasedKeyBesideTheDefault is the #344 proof (design D3). The loader merges
// an operator's {"Interval":"5s"} with the default {"interval":"30s"}; the pin's decoder then took
// 30s and dropped the operator's value without a word. The strict decoder refuses the document,
// naming both spellings, so composition fails instead of running at a value nobody wrote.
func TestHeartbeatRefusesAMiscasedKeyBesideTheDefault(t *testing.T) {
	loaded, err := config.NewLoader().LoadFromBytes([]byte(`{"platform":{"org":"acme","id":"p1"},` +
		`"services":{"heartbeat":{"enabled":true,"config":{"Interval":"5s"}}}}`))
	require.NoError(t, err, "the loader accepts the document; the heartbeat is the owner of its payload")

	registry := NewServiceRegistry()
	require.NoError(t, RegisterAll(registry))
	manager := NewServiceManager(registry)
	err = manager.ConfigureFromServices(loaded.Services, testComponentDependencies(t, component.NewRegistry()))
	require.Error(t, err, "a miscased key beside the default must not compose")
	assert.Contains(t, err.Error(), "config key Interval must be written interval")
	_, composed := manager.GetService("heartbeat")
	assert.False(t, composed, "a refused composition holds no heartbeat")
}

// TestHeartbeatAndMetricsForwarderAreRegistered: RegisterAll offers both services under their
// config-block names (design D5), and each name's constructor builds that service.
func TestHeartbeatAndMetricsForwarderAreRegistered(t *testing.T) {
	registry := NewServiceRegistry()
	require.NoError(t, RegisterAll(registry))

	heartbeat, ok := registry.Constructor("heartbeat")
	require.True(t, ok, "heartbeat must be registered under its config-block name")
	built, err := heartbeat(json.RawMessage(`{}`), &Dependencies{ServiceManager: heartbeatTestManager()})
	require.NoError(t, err)
	assert.IsType(t, &HeartbeatService{}, built)

	forwarder, ok := registry.Constructor("metrics-forwarder")
	require.True(t, ok, "metrics-forwarder must be registered under its config-block name")
	deps := createTestServiceDependencies()
	deps.NATSClient = newUnconnectedClient(t)
	built, err = forwarder(json.RawMessage(`{}`), deps)
	require.NoError(t, err)
	assert.IsType(t, &MetricsForwarder{}, built)
}

// TestHeartbeatComposedFromTheConfigDefaults is design D5: a loaded document that names no
// service composes the heartbeat, which config's defaults turn on, and no forwarder, which is
// opt-in. The heartbeat needs the component manager composed, not started.
func TestHeartbeatComposedFromTheConfigDefaults(t *testing.T) {
	loaded, err := config.NewLoader().LoadFromBytes([]byte(`{"platform":{"org":"acme","id":"p1"}}`))
	require.NoError(t, err)

	registry := NewServiceRegistry()
	require.NoError(t, RegisterAll(registry))
	manager := NewServiceManager(registry)
	require.NoError(t, manager.ConfigureFromServices(loaded.Services,
		testComponentDependencies(t, component.NewRegistry())))

	hb, ok := manager.GetService("heartbeat")
	require.True(t, ok, "the defaults turn the heartbeat on")
	assert.IsType(t, &HeartbeatService{}, hb)
	_, ok = manager.GetService("metrics-forwarder")
	assert.False(t, ok, "the forwarder is opt-in")
}

// --- lifecycle edges (design D2) -------------------------------------------

// TestHeartbeatStartRefusesWithoutTheComponentManager is design D2's "The heartbeat's Start needs
// the component manager", carrying pin heartbeat_test.go:282-345: Start refuses, naming
// component-manager, when no service has that name or the service is not the framework's
// *ComponentManager, and the refused Start leaves the service stopped and holding nothing. The
// lookup is at Start, so the constructor accepts both compositions.
func TestHeartbeatStartRefusesWithoutTheComponentManager(t *testing.T) {
	cases := map[string]func(t *testing.T, manager *Manager){
		"no service named component-manager": func(*testing.T, *Manager) {},
		"a stand-in component manager": func(t *testing.T, manager *Manager) {
			require.NoError(t, manager.RegisterInstance(componentManagerName, newStandInComponentManager()))
		},
	}
	for name, compose := range cases {
		t.Run(name, func(t *testing.T) {
			registry := NewServiceRegistry()
			require.NoError(t, registry.Register("heartbeat", NewHeartbeatService))
			manager := NewServiceManager(registry)
			compose(t, manager)
			built, err := manager.CreateService("heartbeat", json.RawMessage(`{}`),
				&Dependencies{Logger: discardLogger})
			require.NoError(t, err, "the constructor does no lookup")
			hb, ok := built.(*HeartbeatService)
			require.True(t, ok, "NewHeartbeatService returned %T, want *HeartbeatService", built)

			err = hb.Start(t.Context())
			require.Error(t, err, "Start must refuse without the framework's component manager")
			assert.Contains(t, err.Error(), componentManagerName)
			assert.Equal(t, StatusStopped, hb.Status(), "a refused Start leaves the service stopped")
			assert.Empty(t, observeOneLoop(hb.Status(), hb.loopDone).Unresolved, "a refused Start holds nothing")
		})
	}
}

// TestHeartbeatFailedStopIsRetried is design D2's "Stop": a Stop whose context ends before the
// loop returns reports that context's error, and the next Stop waits for the loop again. The
// composition's logger holds the loop's first record ("System heartbeat"), so the loop is inside
// its log call and cannot return until released.
//
// The two 200 ms windows are real-clock windows (design, "Harness rules"). The first Stop's
// deadline cannot fail a correct service: the held loop keeps the join from finishing, so it can
// only end at the deadline. The second window is an observation: a service that forgets its loop
// after a failed Stop returns nil from the second Stop at once; a correct one waits for the held
// loop.
func TestHeartbeatFailedStopIsRetried(t *testing.T) {
	const window = 200 * time.Millisecond
	handler := newHoldingHandler(t, heartbeatRecordMessage)
	hb := newTestHeartbeat(t, `{}`, slog.New(handler))
	require.NoError(t, hb.Start(t.Context()))
	t.Cleanup(func() {
		if err := hb.Stop(stopContext(t)); err != nil {
			t.Errorf("cleanup Stop: %v", err)
		}
	})
	t.Cleanup(handler.releaseHeld)

	held := time.NewTimer(stopBound)
	defer held.Stop()
	select {
	case <-handler.heldRecord():
	case <-held.C:
		t.Fatalf("the loop's first record was not held within %s", stopBound)
	}

	firstCtx, cancelFirst := context.WithTimeout(t.Context(), window)
	defer cancelFirst()
	require.ErrorIs(t, hb.Stop(firstCtx), context.DeadlineExceeded,
		"a Stop whose loop cannot return reports its context's error")

	secondCtx, cancelSecond := context.WithTimeout(t.Context(), stopBound)
	defer cancelSecond()
	secondResult := make(chan error, 1)
	go func() { secondResult <- hb.Stop(secondCtx) }()
	observation := time.NewTimer(window)
	defer observation.Stop()
	select {
	case err := <-secondResult:
		t.Fatalf("the second Stop returned (%v) while the loop was held: the failed Stop forgot its loop", err)
	case <-observation.C:
	}

	handler.releaseHeld()
	require.NoError(t, <-secondResult, "the second Stop finishes the join the first left")
	require.NoError(t, hb.Stop(stopContext(t)), "a Stop after one that returned nil returns nil")
	assert.Empty(t, observeOneLoop(hb.Status(), hb.loopDone).Unresolved,
		"a stopped heartbeat holds no loop and no base status")
}

// --- what the heartbeat emits (design D4) ----------------------------------

// TestHeartbeatRecordsComponentHealth carries pin heartbeat_test.go:379-459 onto the production
// path: the heartbeat's component manager composes two healthy components and one unhealthy one,
// and the record written at Start, read from Dependencies.Logger, counts 2 of 3 at Info (owner,
// Q1; the pin's was Debug). The oracle is the three health answers the test composes. The manager
// is not started: its health read waits on nothing.
func TestHeartbeatRecordsComponentHealth(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		recorder := &heartbeatRecorder{}
		hb := newTestHeartbeat(t, `{}`, slog.New(recorder),
			testComponent{name: "healthy-a", instance: &healthComponent{healthy: true}},
			testComponent{name: "healthy-b", instance: &healthComponent{healthy: true}},
			testComponent{name: "unhealthy", instance: &healthComponent{healthy: false}},
		)
		require.NoError(t, hb.Start(t.Context()))
		synctest.Wait()

		records := recorder.recorded()
		require.Len(t, records, 1, "one record at Start, read from Dependencies.Logger")
		record := records[0]
		assert.Equal(t, slog.LevelInfo, record.level, "the heartbeat is written at Info (owner, Q1)")
		assert.Equal(t, "2", record.attrs["components_healthy"])
		assert.Equal(t, "3", record.attrs["components_total"])
		assert.Equal(t, "0s", record.attrs["uptime"], "the base service's uptime, rounded to the second")
		assert.Contains(t, record.attrs, "goroutines")

		require.NoError(t, hb.Stop(stopContext(t)))
	})
}

// TestHeartbeatCadence is design D4's "at once and then every interval", on synctest's clock: one
// record at Start, none more before the interval has passed, and one per interval, each carrying
// the base service's uptime at that round. The default rows show an empty block and an empty
// interval both take 30s (design D3; the pin's empty-interval refusal row).
func TestHeartbeatCadence(t *testing.T) {
	cases := map[string]struct {
		config   string
		interval time.Duration
	}{
		"1s interval":    {config: `{"interval":"1s"}`, interval: time.Second},
		"3s interval":    {config: `{"interval":"3s"}`, interval: 3 * time.Second},
		"default":        {config: `{}`, interval: 30 * time.Second},
		"empty interval": {config: `{"interval":""}`, interval: 30 * time.Second},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				recorder := &heartbeatRecorder{}
				hb := newTestHeartbeat(t, tc.config, slog.New(recorder))
				require.NoError(t, hb.Start(t.Context()))
				synctest.Wait()
				require.Len(t, recorder.recorded(), 1, "the first round runs at Start")

				<-time.After(tc.interval - time.Millisecond)
				synctest.Wait()
				require.Len(t, recorder.recorded(), 1, "no round before the interval has passed")

				for round := 2; round <= 4; round++ {
					<-time.After(time.Millisecond)
					synctest.Wait()
					records := recorder.recorded()
					require.Len(t, records, round, "one round per interval")
					assert.Equal(t, (time.Duration(round-1) * tc.interval).String(),
						records[round-1].attrs["uptime"])
					<-time.After(tc.interval - time.Millisecond)
				}

				require.NoError(t, hb.Stop(stopContext(t)))
			})
		})
	}
}
