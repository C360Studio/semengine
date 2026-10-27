package config

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/natsclient"
)

// Discard-policy declaration values for StreamConfig.Discard. These are the
// operator-facing spellings of jetstream.DiscardOld / jetstream.DiscardNew; the
// mapping happens once, in buildStreamConfig.
const (
	// StreamDiscardOld evicts the oldest messages once the stream reaches a
	// limit. The write always succeeds; the data loss is silent and at the tail.
	StreamDiscardOld = "old"
	// StreamDiscardNew refuses the write once the stream reaches a limit. Nothing
	// is evicted; producers see the rejection instead. See discardPolicyGuidance.
	StreamDiscardNew = "new"
)

// Declaration-source strings. The framework-constant and operator-map paths
// carry no component attribution, so they report the SOURCE rather than
// inventing an owner they do not know.
const (
	frameworkConstantSource = "framework constant (config/streams.go)"
)

// discardPolicyGuidance is the sentence every declaration diagnostic carries
// when discard policy is at stake. This change's own measurement characterized
// the DiscardNew failure mode against NATS 2.12 — at the ceiling a bucket
// refuses a REPLACEMENT of an existing key, not merely a new one — so handing
// operators the knob without the warning would be careless. Kept as one
// constant so the missing-field error and the declared-DiscardNew warning
// cannot drift apart.
const discardPolicyGuidance = `discard must be "old" or "new": "old" evicts the oldest messages once the ` +
	`stream reaches max_bytes or max_age (the write succeeds, the loss is silent); "new" refuses the write ` +
	`instead, which a producer at the ceiling sees as NATS 503 err_code=10077 "maximum bytes exceeded" on ` +
	`every publish — including a replacement of an existing message — until something is deleted`

// Bounds-contract sentinels. Classifiable so a boot path, an operator tool, or a
// test can tell an undeclared bound from a malformed archival declaration, which
// fail validation for different reasons and have different fixes.
var (
	// errFixedFrameworkStreamCollision is intentionally private: it classifies
	// an invalid configuration without creating a new adopter-facing API.
	errFixedFrameworkStreamCollision = errors.New("configuration collides with a fixed framework stream")

	// ErrStreamBoundsUndeclared is returned when an ordinary stream carries no
	// explicitly declared finite MaxAge, finite MaxBytes, or discard policy and
	// is not admitted by an archival declaration.
	//
	// It is natsclient's value rather than a second one of the same name. The same
	// requirement is enforced at two seams — this declarative path and
	// natsclient.Client.EnsureStream — and a caller testing for "bounds are not
	// declared" should not have to know which door refused it.
	ErrStreamBoundsUndeclared = natsclient.ErrStreamBoundsUndeclared

	// ErrArchivalStreamInvalid is returned when an archival declaration omits its
	// owner or its reason, so archival cannot become a silent way to opt out of
	// bounds.
	ErrArchivalStreamInvalid = errors.New("archival stream declaration is incomplete")
)

// ArchivalStream declares ONE ordinary stream whose contract is permanence:
// nothing may ever be evicted from it. It is exempt from the finite-bounds
// requirement by declaration, and validation reports it as a named PERMANENT
// exception.
//
// Both fields are required, so archival cannot become an unowned, unexplained
// way to opt out of bounds.
type ArchivalStream struct {
	// Owner is the team or component accountable for this stream's growth.
	// Required — an archival stream's only remaining lever is capacity, and
	// capacity questions need an addressee.
	Owner string `json:"owner"`

	// Reason states why permanence is the contract. Required — without it,
	// "archival" is just an unbounded stream with better vocabulary.
	Reason string `json:"reason"`
}

// ArchivalStreams maps ordinary stream name to its archival declaration.
type ArchivalStreams map[string]ArchivalStream

// ArchivalStreamStatus is one permanent exception as validation reports it. It
// has no expiry: archival is permanent by contract.
type ArchivalStreamStatus struct {
	Stream string // the permanently unbounded ordinary stream
	Owner  string // who is accountable for its growth
	Reason string // why permanence is the contract
}

// streamExemption records WHY a stream is admitted without finite bounds, or
// that it is not.
type streamExemption uint8

const (
	// exemptNone: the stream must declare finite bounds and a discard policy.
	exemptNone streamExemption = iota
	// exemptArchival: permanent by declaration.
	exemptArchival
)

// streamDeclaration is one ordinary stream as the provisioner resolved it: the
// declaration itself, where it came from, and whether an exception admits it
// without bounds. Resolution is pure and I/O-free so it can run at config
// validation, before any NATS connection exists.
type streamDeclaration struct {
	name string
	cfg  StreamConfig

	// source names where the declaration came from, for the diagnostic. Where
	// the source records an owning component it names that component and its
	// port; the framework-constant and operator-map paths carry no component
	// attribution, so they name the source instead of a guessed owner.
	source string

	exemption streamExemption
}

// portAttribution is one component/port pair that derived a stream name.
type portAttribution struct {
	component string
	port      string
}

// resolveStreamDeclarations returns every ordinary stream this configuration
// would provision — framework constants, operator-map entries, and port-derived
// streams — in deterministic name order, each carrying its declaration source
// and any bounds exemption.
//
// The backing-stream prefix guard runs here in the same order EnsureStreams
// applied it before this function existed (operator map first, sorted, then
// ports) so a config with several offenders always names the same one.
//
// Pure and I/O-free.
func resolveStreamDeclarations(cfg *Config, _ *slog.Logger) ([]streamDeclaration, error) {
	type resolved struct {
		cfg      StreamConfig
		source   string
		derived  []portAttribution
		explicit bool // declared by a framework constant or the operator map
	}
	decls := make(map[string]*resolved)
	fixed := make(map[string]struct{})

	// 1. Framework constants. These are explicit declarations at a named source
	//    in framework code, not silent defaults: the values are written down,
	//    and readiness reports the source when one of them is incomplete.
	for _, fc := range frameworkStreams() {
		decls[fc.name] = &resolved{cfg: fc.cfg, source: frameworkConstantSource, explicit: true}
		if fc.fixed {
			fixed[fc.name] = struct{}{}
		}
	}

	// 2. Operator map. Highest priority for ordinary framework constants. Fixed
	//    framework declarations reject even byte-for-byte identical duplicates:
	//    accepting one would imply that this is an operator-owned policy surface.
	for _, name := range slices.Sorted(maps.Keys(cfg.Streams)) {
		source := fmt.Sprintf("config.streams[%q]", name)
		if _, isFixed := fixed[name]; isFixed {
			return nil, fmt.Errorf("%w: %s must not declare %q; remove the declaration and allow SemStreams to provision its fixed policy",
				errFixedFrameworkStreamCollision, source, name)
		}
		if err := checkOrdinaryStream(name, source); err != nil {
			return nil, fmt.Errorf("validate stream declaration %q: %w", name, err)
		}
		if existing, ok := decls[name]; ok {
			existing.cfg = cfg.Streams[name]
			existing.source = source
			existing.explicit = true
			continue
		}
		decls[name] = &resolved{cfg: cfg.Streams[name], source: source, explicit: true}
	}

	// 3. Streams derived from component JetStream output ports. Component names
	//    are sorted so the attribution in a diagnostic does not depend on map
	//    iteration order.
	for _, compName := range slices.Sorted(maps.Keys(cfg.Components)) {
		compCfg := cfg.Components[compName]
		if !compCfg.Enabled {
			continue
		}

		ports, err := extractPortsFromConfig(compCfg.Config)
		if err != nil {
			return nil, fmt.Errorf("decode component %q ports: %w", compName, err)
		}

		for _, definition := range ports.Outputs {
			port, err := definition.Resolve(component.DirectionOutput)
			if err != nil {
				return nil, fmt.Errorf("resolve component %q output port %q: %w", compName, definition.Name, err)
			}
			facts, err := port.Facts()
			if err != nil {
				return nil, fmt.Errorf("inspect component %q output port %q: %w", compName, definition.Name, err)
			}
			if facts.Kind() != component.PortKindJetStream {
				continue
			}
			stream, ok := facts.Stream()
			if !ok {
				return nil, fmt.Errorf("component %q output port %q has no JetStream facts", compName, definition.Name)
			}
			streamName, subjects, err := derivePortStream(stream)
			if err != nil {
				return nil, fmt.Errorf("derive component %q output port %q stream: %w", compName, definition.Name, err)
			}
			streamConfig, err := streamConfigFromFacts(stream, subjects)
			if err != nil {
				return nil, fmt.Errorf("project component %q output port %q stream policy: %w", compName, definition.Name, err)
			}

			source := fmt.Sprintf("component %q port %q", compName, definition.Name)
			if _, isFixed := fixed[streamName]; isFixed {
				return nil, fmt.Errorf("%w: %s must not declare %q; choose an application stream name",
					errFixedFrameworkStreamCollision, source, streamName)
			}
			if err := checkOrdinaryStream(streamName, source); err != nil {
				return nil, fmt.Errorf("validate stream declaration %q: %w", streamName, err)
			}

			attribution := portAttribution{component: compName, port: definition.Name}
			if existing, ok := decls[streamName]; ok {
				existing.derived = append(existing.derived, attribution)
				continue
			}
			decls[streamName] = &resolved{
				cfg:     streamConfig,
				derived: []portAttribution{attribution},
			}
		}
	}

	out := make([]streamDeclaration, 0, len(decls))
	for _, name := range slices.Sorted(maps.Keys(decls)) {
		r := decls[name]
		out = append(out, streamDeclaration{
			name:   name,
			cfg:    r.cfg,
			source: describeSource(r.source, r.explicit, r.derived),
		})
	}
	return out, nil
}

// describeSource builds the declaration-source string for a diagnostic.
//
// A port-derived stream records an owning component, so the diagnostic names it
// (and every other component publishing to the same stream, ordered by
// component — that is what an operator looks up). A stream declared in the
// operator map or by a framework constant records no component, so the
// diagnostic names the SOURCE: reporting an owner we do not know would be a
// guess, and a guessed owner in a boot failure sends someone to the wrong team.
func describeSource(explicitSource string, explicit bool, derived []portAttribution) string {
	slices.SortFunc(derived, func(a, b portAttribution) int {
		if c := cmp.Compare(a.component, b.component); c != 0 {
			return c
		}
		return cmp.Compare(a.port, b.port)
	})

	parts := make([]string, 0, len(derived)+1)
	if explicit {
		parts = append(parts, explicitSource)
	}
	for _, d := range derived {
		parts = append(parts, fmt.Sprintf("component %q port %q", d.component, d.port))
	}
	return strings.Join(parts, "; ")
}

// derivePortStream returns the stream name and subjects a JetStream output port
// contributes.
//
// An explicit stream_name from the port definition wins over subject-derived
// naming. The canonical port type carries stream_name (e.g. agentic-tools'
// tool.result port declares stream_name: "AGENT" so its publishes land on the
// existing AGENT stream rather than spawning a derived TOOL stream). The
// historical failure involved broad AGENT "tool.>" coverage; current shipped
// guidance uses the explicit "tool.execute.>" and "tool.result.>" families.
// This relies on the shadow struct
// having been retired in favour of component.PortDefinition; a previous shadow
// stripped this field on JSON unmarshal and silently swallowed every tool.result
// publish in semspec (project_open_work_2026_05_08.md, bug class 3).
func derivePortStream(stream component.StreamFacts) (string, []string, error) {
	subjects := stream.Subjects()
	if len(subjects) == 0 {
		return "", nil, fmt.Errorf("JetStream output requires at least one subject")
	}
	if stream.Name() != "" {
		return stream.Name(), subjects, nil
	}
	name := DeriveStreamName(subjects[0])
	if name == "" {
		return "", nil, fmt.Errorf("cannot derive stream name from subject %q", subjects[0])
	}
	return name, subjects, nil
}

func streamConfigFromFacts(stream component.StreamFacts, subjects []string) (StreamConfig, error) {
	config := StreamConfig{
		Subjects: append([]string(nil), subjects...),
		Storage:  strings.TrimSpace(stream.Storage()),
		Replicas: stream.Replicas(),
	}
	if config.Storage != "" && config.Storage != "file" && config.Storage != "memory" {
		return StreamConfig{}, fmt.Errorf("unknown storage %q", config.Storage)
	}

	switch retention := strings.TrimSpace(stream.RetentionPolicy()); retention {
	case "":
	case "limits", "interest":
		config.Retention = retention
	case "work_queue":
		config.Retention = "workqueue"
	default:
		return StreamConfig{}, fmt.Errorf("unknown retention policy %q", retention)
	}

	days := stream.RetentionDays()
	if days < 0 || int64(days) > math.MaxInt64/int64(24*time.Hour) {
		return StreamConfig{}, fmt.Errorf("retention_days %d is outside the supported duration range", days)
	}
	if days > 0 {
		config.MaxAge = fmt.Sprintf("%dd", days)
	}

	maxSizeGB := stream.MaxSizeGB()
	const bytesPerGiB int64 = 1024 * 1024 * 1024
	if maxSizeGB < 0 || int64(maxSizeGB) > math.MaxInt64/bytesPerGiB {
		return StreamConfig{}, fmt.Errorf("max_size_gb %d is outside the supported byte range", maxSizeGB)
	}
	if maxSizeGB > 0 {
		config.MaxBytes = int64(maxSizeGB) * bytesPerGiB
	}
	if config.Replicas < 0 {
		return StreamConfig{}, fmt.Errorf("replicas %d must not be negative", config.Replicas)
	}
	return config, nil
}

// logAt logs only when a logger was supplied. Config validation runs with none.
func logAt(logger *slog.Logger, level slog.Level, msg string, args ...any) {
	if logger == nil {
		return
	}
	logger.Log(context.Background(), level, msg, args...)
}

// planStreams resolves every ordinary stream this configuration would provision
// and enforces the bounds contract on it, returning the declarations to create
// and validation's list of the archival streams, sorted by stream name.
func planStreams(cfg *Config, logger *slog.Logger) ([]streamDeclaration, []ArchivalStreamStatus, error) {
	decls, err := resolveStreamDeclarations(cfg, logger)
	if err != nil {
		return nil, nil, err
	}

	archival, exemptions, err := validateExceptionDeclarations(cfg)
	if err != nil {
		return nil, nil, err
	}

	var undeclared []string
	for i := range decls {
		decls[i].exemption = exemptions[decls[i].name]
		if miss := missingBounds(decls[i]); len(miss) > 0 {
			undeclared = append(undeclared, fmt.Sprintf("  - stream %q (declared by %s): missing %s",
				decls[i].name, decls[i].source, strings.Join(miss, ", ")))
		}
	}
	if len(undeclared) > 0 {
		return nil, nil, fmt.Errorf("%w:\n%s\n\n%s",
			ErrStreamBoundsUndeclared, strings.Join(undeclared, "\n"), boundsRemedy)
	}

	return decls, archival, nil
}

// boundsRemedy tells the operator exactly what to write and what each choice
// costs. It names the one escape, because an operator who cannot see it will
// reach for the nearest plausible bound instead and pick a wrong one.
var boundsRemedy = strings.Join([]string{
	`declare each missing field in config.streams["<STREAM>"]: ` +
		`max_age (a positive duration, e.g. "24h" or "7d"), ` +
		`max_bytes (a positive byte count — 0 and -1 mean unlimited to NATS and are NOT a declaration), and ` +
		discardPolicyGuidance + ".",
	`a stream whose contract is permanence belongs in archival_streams (owner + reason), not in a bound.`,
}, "\n")

// missingBounds returns the declaration fields this stream still owes, in a
// stable order. An exempt stream owes nothing.
func missingBounds(decl streamDeclaration) []string {
	if decl.exemption != exemptNone {
		return nil
	}
	var missing []string
	if d, err := parseDurationWithDays(decl.cfg.MaxAge); decl.cfg.MaxAge == "" || err != nil || d <= 0 {
		missing = append(missing, "max_age")
	}
	if decl.cfg.MaxBytes <= 0 {
		missing = append(missing, "max_bytes")
	}
	if decl.cfg.Discard != StreamDiscardOld && decl.cfg.Discard != StreamDiscardNew {
		missing = append(missing, "discard")
	}
	return missing
}

// validateExceptionDeclarations validates the archival block and builds
// validation's list of it.
//
// Shape validation runs for EVERY entry, including one naming a stream this
// configuration does not provision.
func validateExceptionDeclarations(cfg *Config) ([]ArchivalStreamStatus, map[string]streamExemption, error) {
	exemptions := make(map[string]streamExemption)
	var archival []ArchivalStreamStatus

	for _, name := range slices.Sorted(maps.Keys(cfg.ArchivalStreams)) {
		source := fmt.Sprintf("archival_streams[%q]", name)
		if err := checkOrdinaryStream(name, source); err != nil {
			return nil, nil, fmt.Errorf("validate stream declaration %q: %w", name, err)
		}
		a := cfg.ArchivalStreams[name]
		var missing []string
		if strings.TrimSpace(a.Owner) == "" {
			missing = append(missing, "owner")
		}
		if strings.TrimSpace(a.Reason) == "" {
			missing = append(missing, "reason")
		}
		if len(missing) > 0 {
			return nil, nil, fmt.Errorf(
				"%w: %s is missing %s — an archival declaration states WHO owns the stream and WHY permanence "+
					"is its contract, so archival cannot become a silent way to opt out of bounds",
				ErrArchivalStreamInvalid, source, strings.Join(missing, " and "))
		}
		exemptions[name] = exemptArchival
		archival = append(archival, ArchivalStreamStatus{
			Stream: name, Owner: a.Owner, Reason: a.Reason,
		})
	}

	return archival, exemptions, nil
}

// ValidateStreamDeclarations resolves every ordinary stream this configuration
// would provision and enforces the bounds contract on it, returning one entry per
// archival_streams entry, sorted by stream name, nil when there is none.
//
// It is pure and I/O-free, which is what lets Config.Validate run it: without
// that, `semstreams --validate` would print "✓ Configuration is valid" for a
// configuration that hard-fails at boot.
func ValidateStreamDeclarations(cfg *Config) ([]ArchivalStreamStatus, error) {
	_, archival, err := planStreams(cfg, nil)
	return archival, err
}

// logStreamExceptions logs one Info line per archival stream at boot.
func logStreamExceptions(logger *slog.Logger, archival []ArchivalStreamStatus) {
	if logger == nil {
		return
	}
	for _, a := range archival {
		logger.Info("Ordinary stream is permanently unbounded by archival declaration",
			"stream", a.Stream,
			"owner", a.Owner,
			"reason", a.Reason,
			"hint", "permanent by contract, not a countdown; its only ceiling is the account tier limit; "+
				"the storage-observability service reports the stream's size")
	}
}
