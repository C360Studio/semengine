package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/natsclient/storagereport"
)

// The tier ceiling's own series (design D4, U-c), carried from the pin
// (storage_observability_tier_metrics_test.go:61-149 and :224-244) with the series named
// semengine_storage_*. They exist because the tier ceiling is the ONLY ceiling an unbounded
// resource has: such a resource inherits the tier's pressure state, so without them an operator
// could see that an archive is critical and have no way to see the number that made it critical,
// or how long they have.

func TestAccountTierMetrics_PublishTheCeilingsOwnCapacity(t *testing.T) {
	metrics, registry := newTestStorageMetrics(t)

	metrics.ObserveAccount(storagereport.AccountReport{
		Tiers: []storagereport.TierComparison{fillingTier(storagereport.TierFile, storagereport.PressureHigh)},
	})

	file := map[string]string{"tier": "file"}
	assert.Equal(t, 4096.0, requireGauge(t, registry, "semengine_storage_account_growth_bytes_per_second", file))
	assert.Equal(t, float64(1<<20), requireGauge(t, registry, "semengine_storage_account_headroom_bytes", file))
	assert.Equal(t, (90 * time.Minute).Seconds(),
		requireGauge(t, registry, "semengine_storage_account_time_to_threshold_seconds", file))

	// The state enum is emitted in FULL, so "high" is a value an operator alerts on rather than
	// the absence of the other series.
	assert.Equal(t, 1.0, requireGauge(t, registry, "semengine_storage_account_pressure_state",
		map[string]string{"tier": "file", "state": "high"}))
	for _, quiet := range []string{"normal", "warning", "critical", "not-evaluated"} {
		assert.Equal(t, 0.0, requireGauge(t, registry, "semengine_storage_account_pressure_state",
			map[string]string{"tier": "file", "state": quiet}),
			"state %q must report zero rather than leaving its last count standing", quiet)
	}
}

// TestAccountTierMetrics_AnUnevaluableCeilingIsExplicit keeps the absent-versus-zero rule at the
// tier level. A stock server bounds neither tier, and a zero headroom there would read as an
// exhausted account.
func TestAccountTierMetrics_AnUnevaluableCeilingIsExplicit(t *testing.T) {
	metrics, registry := newTestStorageMetrics(t)

	metrics.ObserveAccount(storagereport.AccountReport{
		Tiers: []storagereport.TierComparison{{
			Tier:  storagereport.TierFile,
			Limit: storagereport.Capacity{State: storagereport.CapacityUnbounded},
			State: storagereport.OvercommitmentNotApplicable,
			Growth: storagereport.Growth{
				State:       storagereport.GrowthUnknown,
				Unavailable: storagereport.GrowthUnavailableUnknownUsage,
			},
			Pressure: storagereport.Pressure{Unavailable: storagereport.PressureUnavailableUnbounded},
		}},
	})

	file := map[string]string{"tier": "file"}
	_, ok := gaugeValue(t, registry, "semengine_storage_account_headroom_bytes", file)
	assert.False(t, ok, "an unbounded tier has no headroom; a zero would read as exhausted")
	_, ok = gaugeValue(t, registry, "semengine_storage_account_time_to_threshold_seconds", file)
	assert.False(t, ok, "no ceiling means nothing to project toward")
	_, ok = gaugeValue(t, registry, "semengine_storage_account_growth_bytes_per_second", file)
	assert.False(t, ok, "an unmeasured rate is absent, never zero")

	assert.Equal(t, 1.0, requireGauge(t, registry, "semengine_storage_account_pressure_state",
		map[string]string{"tier": "file", "state": "not-evaluated"}))
}

// TestAccountTierMetrics_ATierThatLeavesTheReportStopsReporting extends the retraction rule to the
// tier ceiling's series. The unknown tier disappears once every resource is describable again,
// and a frozen last reading would page someone forever.
func TestAccountTierMetrics_ATierThatLeavesTheReportStopsReporting(t *testing.T) {
	metrics, registry := newTestStorageMetrics(t)

	metrics.ObserveAccount(storagereport.AccountReport{
		Tiers: []storagereport.TierComparison{
			fillingTier(storagereport.TierFile, storagereport.PressureHigh),
			fillingTier(storagereport.TierUnknown, storagereport.PressureCritical),
		},
	})
	unknown := map[string]string{"tier": "unknown"}
	require.NotZero(t, requireGauge(t, registry, "semengine_storage_account_headroom_bytes", unknown))

	metrics.ObserveAccount(storagereport.AccountReport{
		Tiers: []storagereport.TierComparison{fillingTier(storagereport.TierFile, storagereport.PressureHigh)},
	})

	for _, name := range []string{
		"semengine_storage_account_headroom_bytes",
		"semengine_storage_account_time_to_threshold_seconds",
		"semengine_storage_account_growth_bytes_per_second",
	} {
		_, ok := gaugeValue(t, registry, name, unknown)
		assert.False(t, ok, "%s must be retracted for a tier the report no longer carries", name)
	}
	_, ok := gaugeValue(t, registry, "semengine_storage_account_pressure_state",
		map[string]string{"tier": "unknown", "state": "critical"})
	assert.False(t, ok, "the retracted tier's pressure enum must go with it")
}

// TestInheritedPressure_ReachesThePressureGaugeWithoutFabricatingHeadroom: a resource with no bound
// of its own publishes the pressure it inherited from its tier, but none of the tier's numbers as
// its own.
func TestInheritedPressure_ReachesThePressureGaugeWithoutFabricatingHeadroom(t *testing.T) {
	metrics, registry := newTestStorageMetrics(t)

	metrics.ObserveResource(inheritedPressureRow("CAMPAIGN_LEDGER", storagereport.TierFile,
		storagereport.PressureCritical))

	labels := map[string]string{"resource": "CAMPAIGN_LEDGER", "tier": "file"}
	assert.Equal(t, 3.0, requireGauge(t, registry, "semengine_storage_resource_pressure", labels),
		"an archival stream must reach the gauge every pressure alert reads")

	_, ok := gaugeValue(t, registry, "semengine_storage_resource_headroom_bytes", labels)
	assert.False(t, ok, "the tier's headroom is shared and is not this resource's; "+
		"publishing it here would be a fabricated per-resource number")
	_, ok = gaugeValue(t, registry, "semengine_storage_resource_limit_bytes", labels)
	assert.False(t, ok, "an unbounded resource publishes no limit")

	// And it counts under its tier's band rather than under not-evaluated, which is what keeps a
	// dashboard from showing archives as uncovered.
	assert.Equal(t, 1.0, requireGauge(t, registry, "semengine_storage_pressure_resources",
		map[string]string{"tier": "file", "pressure_state": "critical"}))
	assert.Equal(t, 0.0, requireGauge(t, registry, "semengine_storage_pressure_resources",
		map[string]string{"tier": "file", "pressure_state": "not-evaluated"}))
}
