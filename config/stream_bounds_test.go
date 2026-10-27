package config

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/types"
)

// boundedStreamConfig is a complete ordinary-stream declaration: finite MaxAge,
// finite MaxBytes, explicit discard policy. Tests that are about something OTHER
// than the bounds contract use it so their fixtures satisfy it.
func boundedStreamConfig(subjects ...string) StreamConfig {
	return StreamConfig{
		Subjects: subjects,
		MaxAge:   "24h",
		MaxBytes: 10 * 1024 * 1024,
		Discard:  StreamDiscardOld,
	}
}

// namedPortComponent is portComponent with control over the PORT name, so a
// test can distinguish attribution sorted by component from attribution sorted
// by port.
func namedPortComponent(t *testing.T, portName, streamName, subject string) types.ComponentConfig {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"ports": map[string]any{
			"outputs": []map[string]any{{
				"name": portName,
				"config": map[string]any{
					"kind":        "jetstream",
					"subjects":    []string{subject},
					"stream_name": streamName,
				},
			}},
		},
	})
	require.NoError(t, err)
	return types.ComponentConfig{
		Type:    types.ComponentTypeProcessor,
		Name:    "attribution-test",
		Enabled: true,
		Config:  raw,
	}
}

// boundedDeclaration builds a resolved declaration carrying complete bounds, for
// tests that call createStream/buildStreamConfig directly.
func boundedDeclaration(name string, subjects ...string) streamDeclaration {
	return streamDeclaration{
		name:   name,
		cfg:    boundedStreamConfig(subjects...),
		source: "test",
	}
}

// ---------------------------------------------------------------------------
// 5.1 — explicit finite bounds and discard policy on ordinary streams
// ---------------------------------------------------------------------------

// TestConfigValidate_OrdinaryStreamMustDeclareBounds is the core of the bounds
// requirement: each of the three fields is individually load-bearing, and the
// diagnostic must name the stream, its declaration source, and the field the
// operator has to write.
func TestConfigValidate_OrdinaryStreamMustDeclareBounds(t *testing.T) {
	tests := []struct {
		name        string
		stream      StreamConfig
		wantMissing []string
		wantOK      bool
	}{
		{
			name:        "nothing declared",
			stream:      StreamConfig{Subjects: []string{"agent.>"}},
			wantMissing: []string{"max_age", "max_bytes", "discard"},
		},
		{
			name: "max_age omitted — the field the framework used to default to 7d",
			stream: StreamConfig{
				Subjects: []string{"agent.>"}, MaxBytes: 1024, Discard: StreamDiscardOld,
			},
			wantMissing: []string{"max_age"},
		},
		{
			name: "max_bytes omitted — the field NATS reads as unlimited",
			stream: StreamConfig{
				Subjects: []string{"agent.>"}, MaxAge: "24h", Discard: StreamDiscardOld,
			},
			wantMissing: []string{"max_bytes"},
		},
		{
			name: "discard omitted — the field that used to be hardcoded",
			stream: StreamConfig{
				Subjects: []string{"agent.>"}, MaxAge: "24h", MaxBytes: 1024,
			},
			wantMissing: []string{"discard"},
		},
		{
			name: "max_bytes 0 is unlimited to NATS, not a declaration",
			stream: StreamConfig{
				Subjects: []string{"agent.>"}, MaxAge: "24h", MaxBytes: 0, Discard: StreamDiscardOld,
			},
			wantMissing: []string{"max_bytes"},
		},
		{
			name: "max_bytes -1 is unlimited to NATS, not a declaration",
			stream: StreamConfig{
				Subjects: []string{"agent.>"}, MaxAge: "24h", MaxBytes: -1, Discard: StreamDiscardOld,
			},
			wantMissing: []string{"max_bytes"},
		},
		{
			name: "max_age 0s is unlimited to NATS, not a finite bound",
			stream: StreamConfig{
				Subjects: []string{"agent.>"}, MaxAge: "0s", MaxBytes: 1024, Discard: StreamDiscardOld,
			},
			wantMissing: []string{"max_age"},
		},
		{
			name: "unparseable max_age is not a declaration",
			stream: StreamConfig{
				Subjects: []string{"agent.>"}, MaxAge: "forever", MaxBytes: 1024, Discard: StreamDiscardOld,
			},
			wantMissing: []string{"max_age"},
		},
		{
			name: "an unrecognised discard spelling is not a declaration",
			stream: StreamConfig{
				Subjects: []string{"agent.>"}, MaxAge: "24h", MaxBytes: 1024, Discard: "oldest",
			},
			wantMissing: []string{"discard"},
		},
		{
			name:   "all three declared",
			stream: boundedStreamConfig("agent.>"),
			wantOK: true,
		},
		{
			name: "discard new is an equally valid declaration",
			stream: StreamConfig{
				Subjects: []string{"agent.>"}, MaxAge: "24h", MaxBytes: 1024, Discard: StreamDiscardNew,
			},
			wantOK: true,
		},
		{
			name: "day-suffixed max_age parses",
			stream: StreamConfig{
				Subjects: []string{"agent.>"}, MaxAge: "7d", MaxBytes: 1024, Discard: StreamDiscardOld,
			},
			wantOK: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := guardTestConfig()
			cfg.Streams["AGENT"] = tt.stream

			err := cfg.Validate()

			if tt.wantOK {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.ErrorIs(t, err, ErrStreamBoundsUndeclared,
				"an undeclared bound must be classifiable, not just any error")
			assert.Contains(t, err.Error(), `"AGENT"`, "diagnostic must name the stream")
			assert.Contains(t, err.Error(), `config.streams["AGENT"]`,
				"diagnostic must name the declaration source")
			for _, field := range tt.wantMissing {
				assert.Contains(t, err.Error(), field, "diagnostic must name the missing field")
			}
			for _, field := range []string{"max_age", "max_bytes", "discard"} {
				if !contains(tt.wantMissing, field) {
					assert.NotContains(t, strings.SplitN(err.Error(), "\n\n", 2)[0], "missing "+field,
						"a declared field must not be reported missing")
				}
			}
		})
	}
}

// TestConfigValidate_MigrationOverrideKeyAdmitsNothing proves the removed
// stream_migration_overrides key is inert: a document that still sets it loads,
// and the stream it names is refused like any other stream without bounds.
func TestConfigValidate_MigrationOverrideKeyAdmitsNothing(t *testing.T) {
	document := `{
		"platform": {"org": "acme", "id": "p1"},
		"streams": {"LEGACY": {"subjects": ["legacy.>"]}},
		"stream_migration_overrides": {
			"LEGACY": {"owner": "ops", "expires": "2099-01-01", "reason": "sizing study"}
		}
	}`

	cfg, err := NewLoader().LoadFromBytes([]byte(document))
	require.NoError(t, err, "an old document that still sets the key must load")

	err = cfg.Validate()
	require.ErrorIs(t, err, ErrStreamBoundsUndeclared,
		"the key must not admit a stream that lacks bounds")
	assert.Contains(t, err.Error(), `"LEGACY"`, "diagnostic must name the stream")
	assert.Contains(t, err.Error(), "max_age", "diagnostic must name a missing field")
}

// TestConfigValidate_FrameworkStreamsSatisfyTheirOwnRequirement is the control
// that keeps the requirement honest: the five framework-guaranteed streams are
// declarations too, and a config declaring nothing at all must still boot.
func TestConfigValidate_FrameworkStreamsSatisfyTheirOwnRequirement(t *testing.T) {
	require.NoError(t, guardTestConfig().Validate(),
		"the framework's own streams must satisfy the contract they impose")

	decls, archival, err := planStreams(guardTestConfig(), nil)
	require.NoError(t, err)
	assert.Empty(t, archival, "no framework stream may need an archival exception")

	names := make([]string, 0, len(decls))
	for _, d := range decls {
		names = append(names, d.name)
		assert.Empty(t, missingBounds(d), "framework stream %q must declare complete bounds", d.name)
		assert.Equal(t, frameworkConstantSource, d.source,
			"a framework constant must report its SOURCE, not an invented owner")
	}
	assert.ElementsMatch(t,
		[]string{"LOGS", "HEALTH", "METRICS", "FLOWS", "GOVERNANCE_VERDICT_AUDIT", "MAX_DELIVERY_EVENTS"}, names)
}

// TestEnsureStreams_UnboundedStreamFailsBeforeAnyJetStreamCall pins that the
// bounds refusal is fail-closed at boot: nothing is provisioned, and the failure
// happens before NATS is touched, so a config missing one bound cannot leave a
// half-provisioned account behind.
func TestEnsureStreams_UnboundedStreamFailsBeforeAnyJetStreamCall(t *testing.T) {
	cfg := guardTestConfig()
	cfg.Streams["AGENT"] = StreamConfig{Subjects: []string{"agent.>"}}

	err := guardTestManager().EnsureStreams(context.Background(), cfg)

	require.ErrorIs(t, err, ErrStreamBoundsUndeclared)
	assert.True(t, errs.IsFatal(err), "an undeclared bound is unrecoverable, not retryable")
	assert.NotContains(t, err.Error(), jetStreamReached,
		"no stream may be created before the whole declaration set is validated")
}

// TestCreateStream_RefusesUndeclaredBoundsAtTheStampingSeam covers the second
// half, mirroring the prefix guard's structure: createStream is the seam that
// actually stamps a configuration onto NATS, so a caller path that skipped
// declaration validation must not be a supported route to an unbounded stream.
func TestCreateStream_RefusesUndeclaredBoundsAtTheStampingSeam(t *testing.T) {
	err := guardTestManager().createStream(context.Background(), streamDeclaration{
		name:   "AGENT",
		cfg:    StreamConfig{Subjects: []string{"agent.>"}},
		source: "test",
	})

	require.ErrorIs(t, err, ErrStreamBoundsUndeclared)
	assert.NotContains(t, err.Error(), jetStreamReached,
		"the bounds refusal must precede any JetStream access")
}

// TestPlanStreams_ReportsEveryOffenderDeterministically: an operator migrating a
// configuration should learn every field they owe in one boot, and see the same
// list every time. Fixing them one at a time across N boots is how a migration
// gets abandoned halfway.
func TestPlanStreams_ReportsEveryOffenderDeterministically(t *testing.T) {
	build := func() *Config {
		cfg := guardTestConfig()
		cfg.Streams["ZULU"] = StreamConfig{Subjects: []string{"zulu.>"}, MaxAge: "1h", MaxBytes: 1024}
		cfg.Streams["ALPHA"] = StreamConfig{Subjects: []string{"alpha.>"}}
		cfg.Components["mike"] = portComponent(t, "", "mike.out.thing")
		return cfg
	}

	_, _, err := planStreams(build(), nil)
	require.Error(t, err)
	first := err.Error()

	assert.Contains(t, first, `"ALPHA"`)
	assert.Contains(t, first, `"MIKE"`)
	assert.Contains(t, first, `"ZULU"`)
	assert.Less(t, strings.Index(first, `"ALPHA"`), strings.Index(first, `"MIKE"`),
		"offenders must be reported in stable name order")
	assert.Less(t, strings.Index(first, `"MIKE"`), strings.Index(first, `"ZULU"`))

	for range 20 {
		_, _, again := planStreams(build(), nil)
		require.Error(t, again)
		require.Equal(t, first, again.Error(),
			"the diagnostic must not depend on map iteration order")
	}
}

// ---------------------------------------------------------------------------
// 5.2 — name the owning component where the declaration source records one
// ---------------------------------------------------------------------------

// TestBoundsDiagnostic_NamesComponentWhereTheSourceRecordsOne is the whole of
// 5.2: a port-derived stream HAS an owning component and the diagnostic names it
// (and its port); the framework-constant and operator-map paths do not, so they
// name the SOURCE rather than a guessed owner. A guessed owner in a boot failure
// sends someone to the wrong team.
func TestBoundsDiagnostic_NamesComponentWhereTheSourceRecordsOne(t *testing.T) {
	t.Run("port-derived stream names component and port", func(t *testing.T) {
		cfg := guardTestConfig()
		cfg.Components["iot_sensor"] = portComponent(t, "", "sensor.processed.entity")

		_, _, err := planStreams(cfg, nil)

		require.ErrorIs(t, err, ErrStreamBoundsUndeclared)
		assert.Contains(t, err.Error(), `"SENSOR"`, "diagnostic must name the derived stream")
		assert.Contains(t, err.Error(), `component "iot_sensor" port "out"`,
			"diagnostic must name the owning component and the port that derived the stream")
	})

	t.Run("explicit stream_name still attributes the component", func(t *testing.T) {
		cfg := guardTestConfig()
		cfg.Components["agentic-tools"] = portComponent(t, "AGENT", "tool.result.*")

		_, _, err := planStreams(cfg, nil)

		require.ErrorIs(t, err, ErrStreamBoundsUndeclared)
		assert.Contains(t, err.Error(), `component "agentic-tools" port "out"`)
	})

	// Port names are deliberately ordered OPPOSITE to component names, so the
	// assertion discriminates "sorted by component" from "sorted by port". With
	// both ports named the same, either sort key produces the same output and
	// the ordering guarantee would be untested.
	t.Run("several components deriving one stream are all named, sorted by component", func(t *testing.T) {
		cfg := guardTestConfig()
		cfg.Components["agentic-tools"] = namedPortComponent(t, "aaa-result", "AGENT", "tool.result.*")
		cfg.Components["agentic-loop"] = namedPortComponent(t, "zzz-request", "AGENT", "agent.request.*")

		_, _, err := planStreams(cfg, nil)

		require.ErrorIs(t, err, ErrStreamBoundsUndeclared)
		msg := err.Error()
		assert.Contains(t, msg, `component "agentic-loop" port "zzz-request"`)
		assert.Contains(t, msg, `component "agentic-tools" port "aaa-result"`)
		assert.Less(t, strings.Index(msg, "agentic-loop"), strings.Index(msg, "agentic-tools"),
			"attribution is ordered by COMPONENT — that is what an operator looks up — "+
				"and must not depend on map iteration order")
	})

	t.Run("operator-map stream reports its source and invents no owner", func(t *testing.T) {
		cfg := guardTestConfig()
		cfg.Streams["AGENT"] = StreamConfig{Subjects: []string{"agent.>"}}

		_, _, err := planStreams(cfg, nil)

		require.ErrorIs(t, err, ErrStreamBoundsUndeclared)
		assert.Contains(t, err.Error(), `config.streams["AGENT"]`)
		assert.NotContains(t, err.Error(), "component ",
			"a declaration carrying no component attribution must not report one")
	})

	t.Run("framework constant reports its source", func(t *testing.T) {
		cfg := guardTestConfig()
		// Override a framework constant with an incomplete declaration; the
		// operator map is then the source, which is the honest answer.
		cfg.Streams["LOGS"] = StreamConfig{Subjects: []string{"logs.>"}}

		_, _, err := planStreams(cfg, nil)

		require.ErrorIs(t, err, ErrStreamBoundsUndeclared)
		assert.Contains(t, err.Error(), `config.streams["LOGS"]`)
	})

	t.Run("a stream both declared and port-derived names both", func(t *testing.T) {
		cfg := guardTestConfig()
		cfg.Streams["AGENT"] = StreamConfig{Subjects: []string{"agent.>"}}
		cfg.Components["agentic-loop"] = portComponent(t, "AGENT", "agent.request.*")

		_, _, err := planStreams(cfg, nil)

		require.ErrorIs(t, err, ErrStreamBoundsUndeclared)
		assert.Contains(t, err.Error(), `config.streams["AGENT"]`)
		assert.Contains(t, err.Error(), `component "agentic-loop" port "out"`,
			"knowing which component publishes here is what tells the operator how to size it")
	})
}

// ---------------------------------------------------------------------------
// 5.3 — discard policy is an explicit declaration, with the DiscardNew warning
// ---------------------------------------------------------------------------

// TestDeclaredDiscardPolicyIsTheOneApplied pins that the operator's choice
// reaches the stamped JetStream configuration. It replaced a hardcoded
// jetstream.DiscardOld, so "the declared value is the applied value" is the
// entire point of the change.
func TestDeclaredDiscardPolicyIsTheOneApplied(t *testing.T) {
	tests := []struct {
		declared string
		want     jetstream.DiscardPolicy
	}{
		{StreamDiscardOld, jetstream.DiscardOld},
		{StreamDiscardNew, jetstream.DiscardNew},
	}
	for _, tt := range tests {
		t.Run(tt.declared, func(t *testing.T) {
			decl := boundedDeclaration("AGENT", "agent.>")
			decl.cfg.Discard = tt.declared

			got, err := buildStreamConfig(decl, nil)

			require.NoError(t, err)
			assert.Equal(t, tt.want, got.Discard)
			assert.Equal(t, 24*time.Hour, got.MaxAge)
			assert.Equal(t, int64(10*1024*1024), got.MaxBytes)
		})
	}
}

// TestDiscardDiagnostic_StatesWhatDiscardNewDoesAtTheCeiling: this change's own
// measurement characterized the DiscardNew failure — at the ceiling NATS refuses
// even a REPLACEMENT with 503 err_code=10077 — so handing operators the knob
// without the warning would be careless. Both the missing-field diagnostic and
// the declared-DiscardNew warning must carry it.
func TestDiscardDiagnostic_StatesWhatDiscardNewDoesAtTheCeiling(t *testing.T) {
	t.Run("the missing-field diagnostic explains both choices", func(t *testing.T) {
		cfg := guardTestConfig()
		cfg.Streams["AGENT"] = StreamConfig{
			Subjects: []string{"agent.>"}, MaxAge: "24h", MaxBytes: 1024,
		}

		_, _, err := planStreams(cfg, nil)

		require.ErrorIs(t, err, ErrStreamBoundsUndeclared)
		msg := err.Error()
		assert.Contains(t, msg, "503", "the operator must be told the producer-side failure code")
		assert.Contains(t, msg, "10077", "err_code=10077 is what they will actually see in a log")
		assert.Contains(t, msg, "evicts the oldest",
			"the operator must be told what the other choice does too")
		assert.Contains(t, msg, "replacement",
			"replacement failing first is the non-obvious half of the DiscardNew behavior")
	})

	// An operator who cannot see the two escapes will reach for the nearest
	// plausible bound instead. That is how a stream whose contract is permanence
	// ends up on a 7-day MaxAge, and how a legacy stream ends up bounded by a
	// number nobody chose — both failures this contract exists to prevent.
	// The offender is PORT-DERIVED on purpose. That is the lane with no
	// declaration site at all — its source names a component, not a config key —
	// so it is the only case where "where do I write this?" is a real question,
	// and the only case where an operator-map offender would mask the answer by
	// happening to contain "config.streams" in its source string.
	t.Run("the bounds diagnostic names the archival escape and where to declare", func(t *testing.T) {
		cfg := guardTestConfig()
		cfg.Components["iot_sensor"] = portComponent(t, "", "sensor.processed.entity")

		_, _, err := planStreams(cfg, nil)

		require.ErrorIs(t, err, ErrStreamBoundsUndeclared)
		msg := err.Error()
		require.NotContains(t, strings.SplitN(msg, "\n\n", 2)[0], "config.streams",
			"test premise: a port-derived offender's SOURCE must not already name the config key")
		assert.Contains(t, msg, "config.streams",
			"a port-derived stream has no declaration site, so the operator must be told to make one")
		assert.Contains(t, msg, "archival_streams",
			"a stream whose contract is permanence must be pointed at archival, not at a wrong bound")
		assert.NotContains(t, msg, "stream_migration_overrides",
			"the removed key must not be offered as an escape")
	})

	// slog.SetDefault is not touched here, but the handler is process-shared
	// state in spirit; no t.Parallel anywhere in this file.
	t.Run("declaring discard=new warns with the same statement", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

		decl := boundedDeclaration("AGENT", "agent.>")
		decl.cfg.Discard = StreamDiscardNew
		_, err := buildStreamConfig(decl, logger)
		require.NoError(t, err)

		logged := buf.String()
		assert.Contains(t, logged, "AGENT")
		assert.Contains(t, logged, "10077",
			"an operator who selects discard=new must be told at boot what they selected")
	})

	t.Run("declaring discard=old warns about nothing", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

		_, err := buildStreamConfig(boundedDeclaration("AGENT", "agent.>"), logger)
		require.NoError(t, err)

		assert.Empty(t, buf.String(), "the safe choice must not produce noise")
	})
}

// ---------------------------------------------------------------------------
// 5.8 — the ARCHIVAL classification
// ---------------------------------------------------------------------------

func TestArchivalStream(t *testing.T) {
	t.Run("an archival stream satisfies readiness without finite bounds", func(t *testing.T) {
		cfg := guardTestConfig()
		cfg.Streams["CAMPAIGN_LEDGER"] = StreamConfig{Subjects: []string{"campaign.ledger.>"}}
		cfg.ArchivalStreams = ArchivalStreams{
			"CAMPAIGN_LEDGER": {
				Owner:  "semmachina",
				Reason: "the campaign ledger is the permanent record a campaign is replayed from",
			},
		}

		_, archival, err := planStreams(cfg, nil)

		require.NoError(t, err)
		require.Len(t, archival, 1)
		assert.Equal(t, "CAMPAIGN_LEDGER", archival[0].Stream)
		assert.Equal(t, "semmachina", archival[0].Owner)
		assert.Contains(t, archival[0].Reason, "permanent record")
	})

	t.Run("an archival declaration with no owner is rejected", func(t *testing.T) {
		cfg := guardTestConfig()
		cfg.Streams["CAMPAIGN_LEDGER"] = StreamConfig{Subjects: []string{"campaign.ledger.>"}}
		cfg.ArchivalStreams = ArchivalStreams{"CAMPAIGN_LEDGER": {Reason: "permanent"}}

		_, _, err := planStreams(cfg, nil)

		require.ErrorIs(t, err, ErrArchivalStreamInvalid)
		assert.Contains(t, err.Error(), "owner")
	})

	t.Run("an archival declaration with no reason is rejected", func(t *testing.T) {
		cfg := guardTestConfig()
		cfg.Streams["CAMPAIGN_LEDGER"] = StreamConfig{Subjects: []string{"campaign.ledger.>"}}
		cfg.ArchivalStreams = ArchivalStreams{"CAMPAIGN_LEDGER": {Owner: "semmachina"}}

		_, _, err := planStreams(cfg, nil)

		require.ErrorIs(t, err, ErrArchivalStreamInvalid,
			"without a stated reason, archival is just an unbounded stream with better vocabulary")
		assert.Contains(t, err.Error(), "reason")
	})

	t.Run("whitespace is not a reason", func(t *testing.T) {
		cfg := guardTestConfig()
		cfg.Streams["CAMPAIGN_LEDGER"] = StreamConfig{Subjects: []string{"campaign.ledger.>"}}
		cfg.ArchivalStreams = ArchivalStreams{"CAMPAIGN_LEDGER": {Owner: " ", Reason: "\t"}}

		_, _, err := planStreams(cfg, nil)

		require.ErrorIs(t, err, ErrArchivalStreamInvalid)
	})

	t.Run("an archival stream is stamped with no limit and refuses rather than evicts", func(t *testing.T) {
		cfg := guardTestConfig()
		cfg.Streams["CAMPAIGN_LEDGER"] = StreamConfig{
			Subjects: []string{"campaign.ledger.>"}, Storage: "file",
		}
		cfg.ArchivalStreams = ArchivalStreams{
			"CAMPAIGN_LEDGER": {Owner: "semmachina", Reason: "permanent by contract"},
		}

		decls, _, err := planStreams(cfg, nil)
		require.NoError(t, err)

		stamped, err := buildStreamConfig(declarationNamed(t, decls, "CAMPAIGN_LEDGER"), nil)
		require.NoError(t, err)
		assert.Zero(t, stamped.MaxAge, "nothing may ever age out of an archive")
		assert.Zero(t, stamped.MaxBytes, "nothing may ever be evicted for size")
		assert.Equal(t, jetstream.DiscardNew, stamped.Discard,
			`nats stream info on an archive must not read "discard: old"`)
	})

	t.Run("the boot log reports an archival stream at Info and names the size report", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

		logStreamExceptions(logger, []ArchivalStreamStatus{{
			Stream: "CAMPAIGN_LEDGER", Owner: "semmachina", Reason: "permanent by contract",
		}})

		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		require.Len(t, lines, 1)
		assert.Contains(t, lines[0], "CAMPAIGN_LEDGER")
		assert.Contains(t, lines[0], "level=INFO")
		assert.NotContains(t, lines[0], "expires",
			"a permanent exception must never render as a countdown")
		assert.Contains(t, lines[0], "storage-observability",
			"the hint must name the service that reports the stream's size")
	})
}

// ---------------------------------------------------------------------------
// Operator surface: JSON round trip
// ---------------------------------------------------------------------------

// TestStreamBoundsConfig_JSONRoundTrip loads the whole new operator surface from
// JSON (not Go-constructed) and asserts it reaches its consumer. A Go struct
// field reachable only from a composition root is not operator configuration.
func TestStreamBoundsConfig_JSONRoundTrip(t *testing.T) {
	raw := []byte(`{
		"version": "1.0.0",
		"platform": {"org": "acme", "id": "test"},
		"streams": {
			"AGENT": {
				"subjects": ["agent.>"],
				"storage": "file",
				"max_age": "24h",
				"max_bytes": 104857600,
				"discard": "new"
			},
			"LEDGER": {"subjects": ["ledger.>"]}
		},
		"archival_streams": {
			"LEDGER": {
				"owner": "semmachina",
				"reason": "the campaign ledger is replayed from, so nothing may be evicted"
			}
		}
	}`)

	var cfg Config
	require.NoError(t, json.Unmarshal(raw, &cfg))

	require.Equal(t, StreamDiscardNew, cfg.Streams["AGENT"].Discard,
		"discard must survive the JSON round trip")
	require.Equal(t, int64(104857600), cfg.Streams["AGENT"].MaxBytes)
	require.Equal(t, "semmachina", cfg.ArchivalStreams["LEDGER"].Owner)

	// ... and reaches its consumer.
	require.NoError(t, cfg.Validate())
	archival, err := ValidateStreamDeclarations(&cfg)
	require.NoError(t, err)
	require.Len(t, archival, 1)

	decls, _, err := planStreams(&cfg, nil)
	require.NoError(t, err)
	stamped, err := buildStreamConfig(declarationNamed(t, decls, "AGENT"), nil)
	require.NoError(t, err)
	assert.Equal(t, jetstream.DiscardNew, stamped.Discard,
		"the JSON-declared discard policy must reach the stamped JetStream config")

	// And survives a Clone, which is how SafeConfig hands the config around.
	clone := cfg.Clone()
	assert.Equal(t, StreamDiscardNew, clone.Streams["AGENT"].Discard)
	assert.Equal(t, "semmachina", clone.ArchivalStreams["LEDGER"].Owner)
}

// TestExceptionBlocks_RefuseBackingStreamNames keeps the section-1 prohibition
// from being routed around by the archival block: it may not name a KV or
// ObjectStore backing stream, because doing so would tell an operator the
// provisioner governs a resource it must never touch.
func TestExceptionBlocks_RefuseBackingStreamNames(t *testing.T) {
	kvBackingStream := natsclient.KVStreamPrefix + "ENTITY_STATES"

	t.Run("archival cannot name a backing stream", func(t *testing.T) {
		cfg := guardTestConfig()
		cfg.ArchivalStreams = ArchivalStreams{kvBackingStream: {Owner: "x", Reason: "y"}}

		err := cfg.Validate()

		require.ErrorIs(t, err, ErrBackingStreamNotProvisionable)
		assert.Contains(t, err.Error(), kvBackingStream)
	})
}

// TestPlanStreams_PrefixRefusalPrecedesBoundsCheck pins the ordering the section
// 1 guard depends on: a backing-stream declaration is refused on its name before
// anything asks it for bounds, so the operator gets the refusal — which tells
// them to DELETE the declaration — rather than a bounds diagnostic telling them
// to bound a stream they must not touch.
func TestPlanStreams_PrefixRefusalPrecedesBoundsCheck(t *testing.T) {
	cfg := guardTestConfig()
	cfg.Streams[natsclient.KVStreamPrefix+"ENTITY_STATES"] = StreamConfig{Subjects: []string{"x.>"}}

	_, _, err := planStreams(cfg, nil)

	require.ErrorIs(t, err, ErrBackingStreamNotProvisionable)
	assert.False(t, errors.Is(err, ErrStreamBoundsUndeclared),
		"the operator must be told to delete the declaration, not to bound it")
}

// --- helpers ---------------------------------------------------------------

func declarationNamed(t *testing.T, decls []streamDeclaration, name string) streamDeclaration {
	t.Helper()
	for _, d := range decls {
		if d.name == name {
			return d
		}
	}
	t.Fatalf("no declaration named %q in %d declarations", name, len(decls))
	return streamDeclaration{}
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

// TestStreamBoundsSentinel_IsOneIdentityAcrossBothSeams pins that the
// declarative path and natsclient.Client.EnsureStream refuse with the SAME error
// value. The requirement is one requirement; a caller testing for "bounds are not
// declared" — including a sister repo — must not have to know which door caught
// it. Two errors.New calls with identical text would read the same and fail
// errors.Is across the boundary.
func TestStreamBoundsSentinel_IsOneIdentityAcrossBothSeams(t *testing.T) {
	require.Same(t, natsclient.ErrStreamBoundsUndeclared, ErrStreamBoundsUndeclared,
		"config must alias natsclient's sentinel rather than defining a second one")

	// And it holds through the errors.Is path both seams' callers actually use.
	seamErr := natsclient.CheckStreamBounds(jetstream.StreamConfig{Name: "EVENTS"}, "test")
	require.ErrorIs(t, seamErr, ErrStreamBoundsUndeclared,
		"a programmatic refusal must satisfy a config-side errors.Is check")
}
