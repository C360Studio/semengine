package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/natsclient/storagereport"
)

// The storage observability metrics surface (design D4, U-c), carried from the pin
// (storage_observability_test.go:485-839) with the series named semengine_storage_* and the report
// types in storagereport, plus the two tests D4 adds: the second registration and ForgetAccount.

// newTestStorageMetrics builds the metrics surface and registers it on a fresh registry, which it
// returns so a test reads the series a scrape would.
func newTestStorageMetrics(t *testing.T) (*storageObservabilityMetrics, *metric.MetricsRegistry) {
	t.Helper()
	registry := metric.NewMetricsRegistry()
	metrics := newStorageObservabilityMetrics()
	require.NoError(t, metrics.register(registry))
	return metrics, registry
}

// seriesCount is the number of series the registry serves under the metric name.
func seriesCount(t *testing.T, registry *metric.MetricsRegistry, name string) int {
	t.Helper()
	families, err := registry.PrometheusRegistry().Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() == name {
			return len(family.GetMetric())
		}
	}
	return 0
}

// unixSeconds is the freshness gauge's encoding of a collection time.
func unixSeconds(at time.Time) float64 {
	return float64(at.UnixNano()) / float64(time.Second)
}

func TestStorageMetrics_PublishesPerResourceSeries(t *testing.T) {
	metrics, registry := newTestStorageMetrics(t)

	row := withGrowth(withPressure(resourceRow("LOGS", storagereport.TierFile), storagereport.PressureWarning),
		1024, 90*time.Minute)
	row.Resource.Kind = natsclient.ResourceKeyValue
	row.Resource.Bucket = "ENTITY_STATES"
	row.Resource.Attribution = storagereport.AttributionAttributed
	row.Resource.Owner = "graph-ingest"
	metrics.ObserveResource(row)

	labels := map[string]string{
		"resource": "LOGS", "owner": "graph-ingest", "kind": "kv", "tier": "file",
	}
	assert.InDelta(t, 100, requireGauge(t, registry, "semengine_storage_resource_used_bytes", labels), 1e-9)
	assert.InDelta(t, 1000, requireGauge(t, registry, "semengine_storage_resource_limit_bytes", labels), 1e-9)
	assert.InDelta(t, 900, requireGauge(t, registry, "semengine_storage_resource_headroom_bytes", labels), 1e-9)
	assert.InDelta(t, 0.9, requireGauge(t, registry, "semengine_storage_resource_headroom_ratio", labels), 1e-9)
	assert.InDelta(t, 1024, requireGauge(t, registry,
		"semengine_storage_resource_growth_bytes_per_second", labels), 1e-9)
	assert.InDelta(t, 5400, requireGauge(t, registry,
		"semengine_storage_resource_time_to_threshold_seconds", labels), 1e-9)
	assert.InDelta(t, 1, requireGauge(t, registry, "semengine_storage_resource_pressure", labels), 1e-9,
		"warning is severity 1")
}

// TestStorageMetrics_PressureSeverityEncoding pins the numbers an alert reads (`>= 2` is high or
// worse) and the label value a resource with no pressure state is counted under.
func TestStorageMetrics_PressureSeverityEncoding(t *testing.T) {
	for state, want := range map[storagereport.PressureState]int{
		storagereport.PressureNormal:   0,
		storagereport.PressureWarning:  1,
		storagereport.PressureHigh:     2,
		storagereport.PressureCritical: 3,
		"":                             0,
	} {
		assert.Equal(t, want, pressureSeverityValue(state), "severity of %q", state)
	}
	assert.Equal(t, "not-evaluated", pressureStateNotEvaluated)
}

// TestStorageMetrics_UnattributedOwnerIsNamedNotBlank keeps a bucket that escaped the catalog
// from arriving as an empty label an operator cannot group on, and keeps it distinct from a
// resource kind that has no owner concept.
func TestStorageMetrics_UnattributedOwnerIsNamedNotBlank(t *testing.T) {
	metrics, registry := newTestStorageMetrics(t)

	escaped := withPressure(resourceRow("KV_STRAY", storagereport.TierFile), storagereport.PressureNormal)
	escaped.Resource.Kind = natsclient.ResourceKeyValue
	escaped.Resource.Attribution = storagereport.AttributionUnattributed
	metrics.ObserveResource(escaped)

	ordinary := withPressure(resourceRow("EVENTS", storagereport.TierFile), storagereport.PressureNormal)
	metrics.ObserveResource(ordinary)

	_, ok := gaugeValue(t, registry, "semengine_storage_resource_used_bytes",
		map[string]string{"resource": "KV_STRAY", "owner": "unattributed"})
	assert.True(t, ok, "an escaped bucket reports its finding state as its owner")

	_, ok = gaugeValue(t, registry, "semengine_storage_resource_used_bytes",
		map[string]string{"resource": "EVENTS", "owner": "not-applicable"})
	assert.True(t, ok, "a kind with no owner concept is distinct from an escaped bucket")
}

// TestStorageMetrics_AbsentMeasurementsPublishNoSeries: a zero is a measurement, and a resource
// with no measured rate must not report a rate of zero.
func TestStorageMetrics_AbsentMeasurementsPublishNoSeries(t *testing.T) {
	metrics, registry := newTestStorageMetrics(t)

	// Bounded, evaluated, but the rate has never been measured.
	metrics.ObserveResource(withPressure(resourceRow("NEW", storagereport.TierFile), storagereport.PressureNormal))

	labels := map[string]string{"resource": "NEW"}
	_, ok := gaugeValue(t, registry, "semengine_storage_resource_growth_bytes_per_second", labels)
	assert.False(t, ok, "an unmeasured rate is absent, never zero")
	_, ok = gaugeValue(t, registry, "semengine_storage_resource_time_to_threshold_seconds", labels)
	assert.False(t, ok, "no projection without a rate")
	_, ok = gaugeValue(t, registry, "semengine_storage_resource_used_bytes", labels)
	assert.True(t, ok, "what IS measured is still published")
}

// TestStorageMetrics_UnboundedResourceCarriesNoPressureSeries: an unbounded resource whose tier
// offers no ceiling has no pressure state at all, and inventing `normal` for it is a phantom.
func TestStorageMetrics_UnboundedResourceCarriesNoPressureSeries(t *testing.T) {
	metrics, registry := newTestStorageMetrics(t)
	metrics.ObserveResource(unboundedRow("FREE", storagereport.TierFile))

	labels := map[string]string{"resource": "FREE"}
	_, ok := gaugeValue(t, registry, "semengine_storage_resource_pressure", labels)
	assert.False(t, ok, "no band has an input, so there is no state to publish")
	_, ok = gaugeValue(t, registry, "semengine_storage_resource_limit_bytes", labels)
	assert.False(t, ok, "an unbounded resource has no limit to report")
	_, ok = gaugeValue(t, registry, "semengine_storage_resource_headroom_bytes", labels)
	assert.False(t, ok, "and no headroom, because there is nothing to have headroom against")

	// It stays COUNTABLE, which is what keeps it from being the resource nobody can see.
	assert.InDelta(t, 1, requireGauge(t, registry, "semengine_storage_resources",
		map[string]string{"tier": "file", "capacity_state": "unbounded"}), 1e-9)
	assert.InDelta(t, 1, requireGauge(t, registry, "semengine_storage_pressure_resources",
		map[string]string{"tier": "file", "pressure_state": "not-evaluated"}), 1e-9)
}

// TestStorageMetrics_StateTransitionRetractsTheStaleSeries: a resource that stops being bounded
// must stop reporting a limit, not keep the last one forever.
func TestStorageMetrics_StateTransitionRetractsTheStaleSeries(t *testing.T) {
	metrics, registry := newTestStorageMetrics(t)

	metrics.ObserveResource(withPressure(resourceRow("MUTABLE", storagereport.TierFile), storagereport.PressureHigh))
	require.InDelta(t, 1000, requireGauge(t, registry, "semengine_storage_resource_limit_bytes",
		map[string]string{"resource": "MUTABLE"}), 1e-9)

	metrics.ObserveResource(unboundedRow("MUTABLE", storagereport.TierFile))

	labels := map[string]string{"resource": "MUTABLE"}
	_, ok := gaugeValue(t, registry, "semengine_storage_resource_limit_bytes", labels)
	assert.False(t, ok, "the bound is gone; so is the series")
	_, ok = gaugeValue(t, registry, "semengine_storage_resource_pressure", labels)
	assert.False(t, ok)
	assert.InDelta(t, 0, requireGauge(t, registry, "semengine_storage_resources",
		map[string]string{"tier": "file", "capacity_state": "bounded"}), 1e-9)
}

// TestStorageMetrics_OwnerChangeDoesNotStrandTheOldSeries covers a label tuple changing under a
// resource: a bucket added to the descriptor catalog between two collections.
func TestStorageMetrics_OwnerChangeDoesNotStrandTheOldSeries(t *testing.T) {
	metrics, registry := newTestStorageMetrics(t)

	before := withPressure(resourceRow("KV_LATE", storagereport.TierFile), storagereport.PressureNormal)
	before.Resource.Kind = natsclient.ResourceKeyValue
	before.Resource.Attribution = storagereport.AttributionUnattributed
	metrics.ObserveResource(before)

	after := before
	after.Resource.Attribution = storagereport.AttributionAttributed
	after.Resource.Owner = "graph-ingest"
	metrics.ObserveResource(after)

	_, ok := gaugeValue(t, registry, "semengine_storage_resource_used_bytes",
		map[string]string{"resource": "KV_LATE", "owner": "unattributed"})
	assert.False(t, ok, "the resource has one owner at a time; the old label tuple must go")
	_, ok = gaugeValue(t, registry, "semengine_storage_resource_used_bytes",
		map[string]string{"resource": "KV_LATE", "owner": "graph-ingest"})
	assert.True(t, ok)
}

// TestStorageMetrics_ForgetRemovesEveryOneOfItsSeries: a stream that no longer exists must stop
// reporting, or its last pressure reading pages someone forever.
func TestStorageMetrics_ForgetRemovesEveryOneOfItsSeries(t *testing.T) {
	metrics, registry := newTestStorageMetrics(t)

	row := withGrowth(withPressure(resourceRow("GOES", storagereport.TierFile), storagereport.PressureCritical),
		10, time.Hour)
	metrics.ObserveResource(row)
	require.InDelta(t, 3, requireGauge(t, registry, "semengine_storage_resource_pressure",
		map[string]string{"resource": "GOES"}), 1e-9)

	metrics.ForgetResource(row)

	for _, name := range []string{
		"semengine_storage_resource_used_bytes",
		"semengine_storage_resource_limit_bytes",
		"semengine_storage_resource_headroom_bytes",
		"semengine_storage_resource_headroom_ratio",
		"semengine_storage_resource_growth_bytes_per_second",
		"semengine_storage_resource_time_to_threshold_seconds",
		"semengine_storage_resource_pressure",
	} {
		_, ok := gaugeValue(t, registry, name, map[string]string{"resource": "GOES"})
		assert.False(t, ok, "%s must be retracted with the resource", name)
	}
	assert.InDelta(t, 0, requireGauge(t, registry, "semengine_storage_pressure_resources",
		map[string]string{"tier": "file", "pressure_state": "critical"}), 1e-9,
		"a forgotten resource leaves the aggregate count too")
}

// TestStorageMetrics_AggregateCountsAreFullyEmitted keeps the closed enum honest: every
// combination is published on every rebuild, so a state that dropped to zero reports zero rather
// than leaving a stale series behind.
func TestStorageMetrics_AggregateCountsAreFullyEmitted(t *testing.T) {
	metrics, registry := newTestStorageMetrics(t)

	metrics.ObserveResource(withPressure(resourceRow("A", storagereport.TierFile), storagereport.PressureCritical))
	metrics.ObserveResource(withPressure(resourceRow("B", storagereport.TierMemory), storagereport.PressureNormal))

	assert.InDelta(t, 1, requireGauge(t, registry, "semengine_storage_pressure_resources",
		map[string]string{"tier": "file", "pressure_state": "critical"}), 1e-9)
	assert.InDelta(t, 0, requireGauge(t, registry, "semengine_storage_pressure_resources",
		map[string]string{"tier": "file", "pressure_state": "normal"}), 1e-9)
	assert.InDelta(t, 0, requireGauge(t, registry, "semengine_storage_resources",
		map[string]string{"tier": "unknown", "capacity_state": "unknown"}), 1e-9)

	// 3 tiers x (4 pressure states + not-evaluated) = 15; 3 tiers x 3 capacity states = 9. Fixed,
	// and that is the point: the categorical axes never multiply by resource count.
	assert.Equal(t, 15, seriesCount(t, registry, "semengine_storage_pressure_resources"))
	assert.Equal(t, 9, seriesCount(t, registry, "semengine_storage_resources"))
}

// --- account metrics ---------------------------------------------------------

func TestStorageAccountMetrics_OverCommittedTier(t *testing.T) {
	metrics, registry := newTestStorageMetrics(t)

	limit := int64(1 << 30)
	used := int64(1 << 20)
	metrics.ObserveAccount(storagereport.AccountReport{Tiers: []storagereport.TierComparison{{
		Tier: storagereport.TierFile,
		Limit: storagereport.Capacity{
			State: storagereport.CapacityBounded, ConfiguredLimit: &limit, Used: &used,
		},
		DeclaredBytes:    2 << 30,
		BoundedResources: 2,
		State:            storagereport.OvercommitmentOver,
	}}})

	file := map[string]string{"tier": "file"}
	assert.InDelta(t, float64(1<<30), requireGauge(t, registry, "semengine_storage_account_limit_bytes", file), 1e-9)
	assert.InDelta(t, float64(1<<20), requireGauge(t, registry, "semengine_storage_account_used_bytes", file), 1e-9)
	assert.InDelta(t, float64(2<<30), requireGauge(t, registry,
		"semengine_storage_account_declared_bytes", file), 1e-9)
	assert.InDelta(t, 1, requireGauge(t, registry, "semengine_storage_account_overcommitment",
		map[string]string{"tier": "file", "state": "over-committed"}), 1e-9)
	assert.InDelta(t, 0, requireGauge(t, registry, "semengine_storage_account_overcommitment",
		map[string]string{"tier": "file", "state": "within-limit"}), 1e-9)
	assert.InDelta(t, 1, requireGauge(t, registry, "semengine_storage_account_limit_state",
		map[string]string{"tier": "file", "state": "bounded"}), 1e-9)
}

// TestStorageAccountMetrics_UnboundedLimitIsNotApplicableNotSatisfied is the default path: a
// stock server reports no account limit, so this is what most deployments scrape.
func TestStorageAccountMetrics_UnboundedLimitIsNotApplicableNotSatisfied(t *testing.T) {
	metrics, registry := newTestStorageMetrics(t)

	used := int64(4096)
	metrics.ObserveAccount(storagereport.AccountReport{Tiers: []storagereport.TierComparison{{
		Tier:          storagereport.TierFile,
		Limit:         storagereport.Capacity{State: storagereport.CapacityUnbounded, Used: &used},
		DeclaredBytes: 1 << 40,
		State:         storagereport.OvercommitmentNotApplicable,
		Unavailable:   storagereport.OvercommitmentUnavailableUnboundedLimit,
	}}})

	_, ok := gaugeValue(t, registry, "semengine_storage_account_limit_bytes", map[string]string{"tier": "file"})
	assert.False(t, ok, "there is no limit number, so there is no limit series")

	assert.InDelta(t, 1, requireGauge(t, registry, "semengine_storage_account_limit_state",
		map[string]string{"tier": "file", "state": "unbounded"}), 1e-9,
		"unbounded is published EXPLICITLY rather than as an absent series")
	assert.InDelta(t, 1, requireGauge(t, registry, "semengine_storage_account_overcommitment",
		map[string]string{"tier": "file", "state": "not-applicable"}), 1e-9)
	assert.InDelta(t, 0, requireGauge(t, registry, "semengine_storage_account_overcommitment",
		map[string]string{"tier": "file", "state": "within-limit"}), 1e-9,
		"an unbounded limit must never read as a comparison that passed")
	assert.InDelta(t, float64(4096), requireGauge(t, registry,
		"semengine_storage_account_used_bytes", map[string]string{"tier": "file"}), 1e-9,
		"usage is still observable without a bound")
}

func TestStorageAccountMetrics_UnknownLimitIsDistinctFromUnbounded(t *testing.T) {
	metrics, registry := newTestStorageMetrics(t)

	metrics.ObserveAccount(storagereport.AccountReport{
		LimitsUnavailable: "account info unavailable",
		Tiers: []storagereport.TierComparison{{
			Tier:        storagereport.TierFile,
			Limit:       storagereport.Capacity{State: storagereport.CapacityUnknown},
			State:       storagereport.OvercommitmentNotApplicable,
			Unavailable: storagereport.OvercommitmentUnavailableUnknownLimit,
		}},
	})

	assert.InDelta(t, 1, requireGauge(t, registry, "semengine_storage_account_limit_state",
		map[string]string{"tier": "file", "state": "unknown"}), 1e-9)
	assert.InDelta(t, 0, requireGauge(t, registry, "semengine_storage_account_limit_state",
		map[string]string{"tier": "file", "state": "unbounded"}), 1e-9,
		"unreadable is not unlimited")
	_, ok := gaugeValue(t, registry, "semengine_storage_account_used_bytes", map[string]string{"tier": "file"})
	assert.False(t, ok, "an unreadable tier reports no usage either")
}

// TestStorageAccountMetrics_TierThatDisappearsIsRetracted keeps the unknown tier, which exists
// only while something is undescribable, from reporting forever once every resource can be read
// again.
func TestStorageAccountMetrics_TierThatDisappearsIsRetracted(t *testing.T) {
	metrics, registry := newTestStorageMetrics(t)

	unknownLimit := storagereport.Capacity{State: storagereport.CapacityUnknown}
	metrics.ObserveAccount(storagereport.AccountReport{Tiers: []storagereport.TierComparison{
		{Tier: storagereport.TierFile, Limit: unknownLimit, State: storagereport.OvercommitmentNotApplicable},
		{Tier: storagereport.TierUnknown, Limit: unknownLimit, State: storagereport.OvercommitmentNotApplicable},
	}})
	require.InDelta(t, 1, requireGauge(t, registry, "semengine_storage_account_limit_state",
		map[string]string{"tier": "unknown", "state": "unknown"}), 1e-9)

	metrics.ObserveAccount(storagereport.AccountReport{Tiers: []storagereport.TierComparison{
		{Tier: storagereport.TierFile, Limit: unknownLimit, State: storagereport.OvercommitmentNotApplicable},
	}})

	_, ok := gaugeValue(t, registry, "semengine_storage_account_limit_state",
		map[string]string{"tier": "unknown", "state": "unknown"})
	assert.False(t, ok, "a tier that is no longer reported stops reporting")
}

// TestStorageAccountMetrics_ForgetAccountRetractsEveryTierSeries (design D4, O1): when the
// consumer drops the account row it held, every per-tier series published for it goes, the
// freshness gauge keeps the newest collection seen, and a later account row publishes afresh.
func TestStorageAccountMetrics_ForgetAccountRetractsEveryTierSeries(t *testing.T) {
	metrics, registry := newTestStorageMetrics(t)

	collected := time.Now().UTC().Add(-time.Minute)
	report := storagereport.AccountReport{
		CollectedAt: collected,
		Tiers: []storagereport.TierComparison{
			fillingTier(storagereport.TierFile, storagereport.PressureHigh),
			fillingTier(storagereport.TierMemory, storagereport.PressureWarning),
		},
	}
	metrics.ObserveAccount(report)

	perTier := []string{
		"semengine_storage_account_limit_bytes",
		"semengine_storage_account_used_bytes",
		"semengine_storage_account_declared_bytes",
		"semengine_storage_account_growth_bytes_per_second",
		"semengine_storage_account_headroom_bytes",
		"semengine_storage_account_time_to_threshold_seconds",
		"semengine_storage_account_limit_state",
		"semengine_storage_account_overcommitment",
		"semengine_storage_account_pressure_state",
	}
	for _, tier := range []string{"file", "memory"} {
		for _, name := range perTier {
			requireGauge(t, registry, name, map[string]string{"tier": tier})
		}
	}

	metrics.ForgetAccount(report)

	for _, tier := range []string{"file", "memory"} {
		for _, name := range perTier {
			_, ok := gaugeValue(t, registry, name, map[string]string{"tier": tier})
			assert.False(t, ok, "%s{tier=%q} must be retracted with the account row", name, tier)
		}
	}
	assert.InDelta(t, unixSeconds(collected),
		requireGauge(t, registry, "semengine_storage_report_collected_timestamp_seconds", nil), 0.001,
		"a dropped row does not undo the newest collection seen")

	metrics.ObserveAccount(storagereport.AccountReport{
		CollectedAt: collected.Add(time.Second),
		Tiers:       []storagereport.TierComparison{fillingTier(storagereport.TierFile, storagereport.PressureCritical)},
	})
	assert.InDelta(t, 1, requireGauge(t, registry, "semengine_storage_account_pressure_state",
		map[string]string{"tier": "file", "state": "critical"}), 1e-9, "a later account row publishes afresh")
}

// TestStorageMetrics_SecondRegistrationWritesTheCanonicalCollectors (design D4, O2): a second
// surface registered on the same registry must write the collectors the registry serves, never its
// own unregistered candidates, or its series never reach a scrape.
func TestStorageMetrics_SecondRegistrationWritesTheCanonicalCollectors(t *testing.T) {
	registry := metric.NewMetricsRegistry()
	first := newStorageObservabilityMetrics()
	require.NoError(t, first.register(registry))
	second := newStorageObservabilityMetrics()
	require.NoError(t, second.register(registry))

	collected := time.Now().UTC().Add(-30 * time.Second)
	row := withGrowth(withPressure(resourceRow("SECOND", storagereport.TierFile), storagereport.PressureHigh),
		512, time.Hour)
	row.CollectedAt = collected
	second.ObserveResource(row)
	second.ObserveAccount(storagereport.AccountReport{
		Tiers: []storagereport.TierComparison{fillingTier(storagereport.TierFile, storagereport.PressureNormal)},
	})

	for _, name := range []string{
		"semengine_storage_resource_used_bytes",
		"semengine_storage_resource_limit_bytes",
		"semengine_storage_resource_headroom_bytes",
		"semengine_storage_resource_headroom_ratio",
		"semengine_storage_resource_growth_bytes_per_second",
		"semengine_storage_resource_time_to_threshold_seconds",
		"semengine_storage_resource_pressure",
	} {
		requireGauge(t, registry, name, map[string]string{"resource": "SECOND"})
	}
	for _, name := range []string{
		"semengine_storage_account_limit_bytes",
		"semengine_storage_account_used_bytes",
		"semengine_storage_account_declared_bytes",
		"semengine_storage_account_growth_bytes_per_second",
		"semengine_storage_account_headroom_bytes",
		"semengine_storage_account_time_to_threshold_seconds",
		"semengine_storage_account_limit_state",
		"semengine_storage_account_overcommitment",
		"semengine_storage_account_pressure_state",
	} {
		requireGauge(t, registry, name, map[string]string{"tier": "file"})
	}
	assert.InDelta(t, 1, requireGauge(t, registry, "semengine_storage_resources",
		map[string]string{"tier": "file", "capacity_state": "bounded"}), 1e-9)
	assert.InDelta(t, 1, requireGauge(t, registry, "semengine_storage_pressure_resources",
		map[string]string{"tier": "file", "pressure_state": "high"}), 1e-9)
	assert.InDelta(t, unixSeconds(collected),
		requireGauge(t, registry, "semengine_storage_report_collected_timestamp_seconds", nil), 0.001)
}

// TestStorageMetrics_FreshnessGaugeCarriesCollectionTime pins the one series that can report the
// collector STOPPED. Every other series here is stamped with scrape time, so a dead collector's
// numbers keep arriving looking fresh; only a gauge whose VALUE is the collection time turns that
// into `time() - gauge > horizon`.
func TestStorageMetrics_FreshnessGaugeCarriesCollectionTime(t *testing.T) {
	metrics, registry := newTestStorageMetrics(t)

	collected := time.Now().UTC().Add(-90 * time.Second)
	row := resourceRow("FRESH", storagereport.TierFile)
	row.CollectedAt = collected
	metrics.ObserveResource(row)

	got := requireGauge(t, registry, "semengine_storage_report_collected_timestamp_seconds", nil)
	assert.InDelta(t, unixSeconds(collected), got, 0.001,
		"the gauge must carry COLLECTION time, not scrape time")
}

// TestStorageMetrics_FreshnessGaugeNeverMovesBackwards: a watch delivers in no guaranteed order and
// a reconnect can hand back an older row after a newer one; a regressing gauge would manufacture
// staleness that never happened.
func TestStorageMetrics_FreshnessGaugeNeverMovesBackwards(t *testing.T) {
	metrics, registry := newTestStorageMetrics(t)

	newest := time.Now().UTC()
	fresh := resourceRow("A", storagereport.TierFile)
	fresh.CollectedAt = newest
	metrics.ObserveResource(fresh)

	stale := resourceRow("B", storagereport.TierFile)
	stale.CollectedAt = newest.Add(-10 * time.Minute)
	metrics.ObserveResource(stale)

	got := requireGauge(t, registry, "semengine_storage_report_collected_timestamp_seconds", nil)
	assert.InDelta(t, unixSeconds(newest), got, 0.001,
		"a replayed older row must not drag freshness backwards")
}

// TestStorageMetrics_FreshnessGaugeIgnoresAZeroTimestamp keeps an unstamped row from reading as
// 1970, which would render as decades of staleness and fire every horizon at once.
func TestStorageMetrics_FreshnessGaugeIgnoresAZeroTimestamp(t *testing.T) {
	metrics, registry := newTestStorageMetrics(t)

	collected := time.Now().UTC()
	row := resourceRow("A", storagereport.TierFile)
	row.CollectedAt = collected
	metrics.ObserveResource(row)

	unstamped := resourceRow("B", storagereport.TierFile)
	unstamped.CollectedAt = time.Time{}
	metrics.ObserveResource(unstamped)

	got := requireGauge(t, registry, "semengine_storage_report_collected_timestamp_seconds", nil)
	assert.InDelta(t, unixSeconds(collected), got, 0.001)
}
