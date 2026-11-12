package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/internal/harness/lifecycletest"
	"github.com/c360studio/semengine/pkg/errs"
)

// The log forwarder's configuration, record, status, registration, composition and lifecycle.
// Accepted configurations are checked by TestLogForwarderSaysItForwardsNothing, which reads the
// applied values from the record.

// forwardsNothingMessage is the record's message, written out by hand as the oracle.
const forwardsNothingMessage = "log-forwarder forwards nothing: no log handler in this module publishes logs to NATS"

// newTestLogForwarder builds a log forwarder the one production way, through
// NewLogForwarderService, from config over createTestServiceDependencies with logger as
// Dependencies.Logger. It returns the service and the dependencies, whose MetricsRegistry the
// status test reads.
func newTestLogForwarder(t *testing.T, config string, logger *slog.Logger) (*LogForwarder, *Dependencies) {
	t.Helper()
	deps := createTestServiceDependencies()
	deps.Logger = logger
	built, err := NewLogForwarderService(json.RawMessage(config), deps)
	require.NoError(t, err)
	lf, ok := built.(*LogForwarder)
	require.True(t, ok, "NewLogForwarderService returned %T, want *LogForwarder", built)
	stop := stopContext(t)
	t.Cleanup(func() { _ = lf.Stop(stop) })
	return lf, deps
}

// jsonRecords decodes every record a slog.NewJSONHandler wrote into logs, one object per line.
func jsonRecords(t *testing.T, logs *syncBuffer) []map[string]any {
	t.Helper()
	var records []map[string]any
	for line := range strings.SplitSeq(logs.String(), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record), "record %q", line)
		records = append(records, record)
	}
	return records
}

// TestLogForwarderConstructionRefusals: each refusal is invalid data (errs.IsInvalid) and names
// the input it refuses. An empty level is not refused: it takes INFO
// (TestLogForwarderSaysItForwardsNothing).
//
// spec: service-lifecycle / The log forwarder refuses an invalid configuration
func TestLogForwarderConstructionRefusals(t *testing.T) {
	withDeps := func() *Dependencies { return createTestServiceDependencies() }
	refusals := map[string]struct {
		config string
		deps   func() *Dependencies
		names  string
	}{
		"nil dependencies":    {config: `{}`, deps: func() *Dependencies { return nil }, names: "dependencies"},
		"unparseable":         {config: `{"min_level":`, deps: withDeps, names: "log-forwarder"},
		"second JSON value":   {config: `{"min_level":"info"} {}`, deps: withDeps, names: "log-forwarder"},
		"level TRACE":         {config: `{"min_level":"TRACE"}`, deps: withDeps, names: "invalid log level: TRACE"},
		"level INVALID":       {config: `{"min_level":"INVALID"}`, deps: withDeps, names: "invalid log level: INVALID"},
		"unknown key":         {config: `{"min_levl":"debug"}`, deps: withDeps, names: "min_levl"},
		"miscased min_level":  {config: `{"Min_Level":"debug"}`, deps: withDeps, names: "config key Min_Level must be written min_level"},
		"miscased exclusions": {config: `{"Exclude_Sources":["a"]}`, deps: withDeps, names: "config key Exclude_Sources must be written exclude_sources"},
	}
	for name, tc := range refusals {
		t.Run(name, func(t *testing.T) {
			built, err := NewLogForwarderService(json.RawMessage(tc.config), tc.deps())
			require.Error(t, err, "construction must refuse, not default")
			require.Nil(t, built)
			assert.True(t, errs.IsInvalid(err), "want an invalid-data refusal, got %v", err)
			assert.Contains(t, err.Error(), tc.names, "the refusal names what it refuses")
		})
	}
}

// TestLogForwarderSaysItForwardsNothing: after Start the service has written exactly one record, at
// WARN, saying it forwards nothing and stating the configuration it resolved; the accepted config
// rows and exclude_sources are read from that record. The oracle is the production
// slog.NewJSONHandler at Debug, so a record at any level counts. The exclusion list is exactly what
// was configured, normalised: nothing is added.
//
// A refused Start writes no record: neither one under an ended context nor one after
// Stop.
//
// spec: service-lifecycle / The log forwarder forwards nothing and says so
func TestLogForwarderSaysItForwardsNothing(t *testing.T) {
	rows := map[string]struct {
		config     string
		minLevel   string
		exclusions []any
	}{
		"empty block":        {config: `{}`, minLevel: "INFO", exclusions: []any{}},
		"null":               {config: `null`, minLevel: "INFO", exclusions: []any{}},
		"lower-case level":   {config: `{"min_level":"debug"}`, minLevel: "DEBUG", exclusions: []any{}},
		"padded level":       {config: `{"min_level":" warn "}`, minLevel: "WARN", exclusions: []any{}},
		"exclusions trimmed": {config: `{"min_level":"warn","exclude_sources":[" a ","a","","b"]}`, minLevel: "WARN", exclusions: []any{"a", "b"}},
	}
	for name, tc := range rows {
		t.Run(name, func(t *testing.T) {
			logs := &syncBuffer{}
			lf, _ := newTestLogForwarder(t, tc.config,
				slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})))

			require.NoError(t, lf.Start(t.Context()))
			records := jsonRecords(t, logs)
			require.Len(t, records, 1, "Start writes exactly one record")
			record := records[0]
			assert.Equal(t, "WARN", record["level"])
			assert.Equal(t, forwardsNothingMessage, record["msg"])
			assert.Equal(t, "log-forwarder", record["service"])
			assert.Equal(t, tc.minLevel, record["min_level"])
			assert.Equal(t, tc.exclusions, record["exclude_sources"])

			require.ErrorIs(t, lf.Start(t.Context()), errs.ErrAlreadyStarted)
			require.Len(t, jsonRecords(t, logs), 1, "a second Start while running writes no record")

			require.NoError(t, lf.Stop(stopContext(t)))
			require.Len(t, jsonRecords(t, logs), 1, "Stop writes no record")
			require.ErrorIs(t, lf.Start(t.Context()), errs.ErrAlreadyStarted)
			require.Len(t, jsonRecords(t, logs), 1, "a refused Start writes no record")
		})
	}

	t.Run("refused Start writes no record", func(t *testing.T) {
		logs := &syncBuffer{}
		lf, _ := newTestLogForwarder(t, `{}`,
			slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})))

		ended, cancel := context.WithCancel(t.Context())
		cancel()
		require.Error(t, lf.Start(ended), "Start refuses an ended context")
		require.Empty(t, jsonRecords(t, logs), "a Start refused for its context writes no record")

		require.NoError(t, lf.Stop(stopContext(t)))
		require.ErrorIs(t, lf.Start(t.Context()), errs.ErrAlreadyStarted)
		require.Empty(t, jsonRecords(t, logs), "a Start refused after Stop writes no record")
	})
}

// TestLogForwarderReportsItsStatus: the service's status is recorded on the injected registry as
// semengine_service_status, stopped (0) once built, running (2) after Start and stopped (0) after
// Stop, as TestMetricsForwarderReportsItsStatus does.
//
// spec: service-lifecycle / The log forwarder forwards nothing and says so
func TestLogForwarderReportsItsStatus(t *testing.T) {
	lf, deps := newTestLogForwarder(t, `{}`, discardLogger)
	registry := deps.MetricsRegistry
	labels := map[string]string{"service": "log-forwarder"}

	require.Equal(t, 0.0, requireGauge(t, registry, "semengine_service_status", labels), "built")
	require.NoError(t, lf.Start(t.Context()))
	require.Equal(t, 2.0, requireGauge(t, registry, "semengine_service_status", labels), "started")
	require.NoError(t, lf.Stop(stopContext(t)))
	require.Equal(t, 0.0, requireGauge(t, registry, "semengine_service_status", labels), "stopped")
}

// TestLogForwarderIsRegistered: RegisterAll offers the service under its config-block name
// and that name's constructor builds a *LogForwarder from {}.
//
// spec: service-lifecycle / The log forwarder refuses an invalid configuration
func TestLogForwarderIsRegistered(t *testing.T) {
	registry := NewServiceRegistry()
	require.NoError(t, RegisterAll(registry))

	constructor, ok := registry.Constructor("log-forwarder")
	require.True(t, ok, "log-forwarder must be registered under its config-block name")
	built, err := constructor(json.RawMessage(`{}`), createTestServiceDependencies())
	require.NoError(t, err)
	_, ok = built.(*LogForwarder)
	require.True(t, ok, "the log-forwarder constructor returned %T, want *LogForwarder", built)
}

// TestLogForwarderComposedWhenEnabled: a document enabling log-forwarder composes one, and the
// same document naming no services composes none, since config's defaults do not enable it.
//
// spec: service-lifecycle / The log forwarder refuses an invalid configuration
func TestLogForwarderComposedWhenEnabled(t *testing.T) {
	compose := func(t *testing.T, document string) *Manager {
		t.Helper()
		loaded, err := config.NewLoader().LoadFromBytes([]byte(document))
		require.NoError(t, err)
		registry := NewServiceRegistry()
		require.NoError(t, RegisterAll(registry))
		manager := NewServiceManager(registry)
		require.NoError(t, manager.ConfigureFromServices(loaded.Services,
			testComponentDependencies(t, component.NewRegistry())))
		return manager
	}

	t.Run("enabled", func(t *testing.T) {
		manager := compose(t, `{"platform":{"org":"acme","id":"p1"},`+
			`"services":{"log-forwarder":{"enabled":true,"config":{"min_level":"warn"}}}}`)
		got, found := manager.GetService("log-forwarder")
		require.True(t, found, "an enabled log-forwarder is composed")
		_, ok := got.(*LogForwarder)
		require.True(t, ok, "GetService(log-forwarder) returned %T, want *LogForwarder", got)
	})

	t.Run("not named", func(t *testing.T) {
		manager := compose(t, `{"platform":{"org":"acme","id":"p1"}}`)
		_, found := manager.GetService("log-forwarder")
		require.False(t, found, "a document naming no services composes no log forwarder")
	})
}

// logForwarderOwner adapts the log forwarder to the lifecycle suite (lifecycle-suite, "Observe
// adapter contract"). The service holds nothing of its own: a started log forwarder
// holds its base service's health goroutine, which Observe reports as baseServiceOwner does over
// the embedded BaseService. Nothing is added to the service for this adapter.
type logForwarderOwner struct {
	lf *LogForwarder
}

func (o *logForwarderOwner) Start(ctx context.Context) error { return o.lf.Start(ctx) }
func (o *logForwarderOwner) Stop(ctx context.Context) error  { return o.lf.Stop(ctx) }

func (o *logForwarderOwner) Observe() lifecycletest.Observation {
	return (&baseServiceOwner{s: o.lf.BaseService}).Observe()
}

// TestLogForwarderPassesTheLifecycleSuite runs the lifecycle suite on the log forwarder built
// through NewLogForwarderService (ADR-113). The must-fail owner is a log forwarder stopped before
// Start, whose Start is refused (base_lifecycle_test.go's precedent). The service is one-shot: no
// restart is promised.
//
// spec: service-lifecycle / The log forwarder's Start bounds Start and Stop is the only stop
func TestLogForwarderPassesTheLifecycleSuite(t *testing.T) {
	newOwner := func() lifecycletest.Owner {
		lf, _ := newTestLogForwarder(t, `{}`, discardLogger)
		return &logForwarderOwner{lf: lf}
	}
	mustFail := func() lifecycletest.Owner {
		owner := newOwner()
		require.NoError(t, owner.Stop(stopContext(t)))
		return owner
	}
	lifecycletest.Run(t, newOwner, mustFail, lifecycletest.Promise{})

	// lifecycle-suite, "Started owner reports its handles": the suite judges only what is left
	// after a Stop, so an adapter that never reported the health loop would pass it.
	t.Run("StartedOwnerReportsTheHealthLoop", func(t *testing.T) {
		owner := newOwner()
		require.NoError(t, owner.Start(t.Context()))
		require.Equal(t, []string{"health-loop"}, owner.Observe().Unresolved)
		require.NoError(t, owner.Stop(stopContext(t)))
		require.Empty(t, owner.Observe().Unresolved, "a Stop that returned nil left a handle")
	})
}
