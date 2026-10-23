package service

import (
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/natsclient/storagereport"
)

// The storage observability service's shared test helpers (design D7, U-a). Every service is
// built the one production way, NewStorageObservabilityService, so a test sees only what the
// constructor composes. The row builders are carried from the pin
// (storage_observability_test.go:24-73, storage_observability_tier_metrics_test.go:21-59,
// storage_observability_http_test.go:87-97) with the report types now in storagereport and the
// pin's unexported constructors spelled as the values they returned (design D7, "Unexported
// names").

// newTestStorageObservability builds a storage observability service through
// NewStorageObservabilityService from config, a JSON StorageObservabilityConfig, over client and
// logger, with a fresh metrics registry, which it returns so a test can read the service's series.
func newTestStorageObservability(
	t *testing.T, client *natsclient.Client, config string, logger *slog.Logger,
) (*StorageObservabilityService, *metric.MetricsRegistry) {
	t.Helper()
	registry := metric.NewMetricsRegistry()
	built, err := NewStorageObservabilityService(json.RawMessage(config), &Dependencies{
		NATSClient:      client,
		MetricsRegistry: registry,
		Logger:          logger,
	})
	require.NoError(t, err)
	s, ok := built.(*StorageObservabilityService)
	require.True(t, ok, "NewStorageObservabilityService returned %T, want *StorageObservabilityService", built)
	return s, registry
}

// resourceRow is a bounded ordinary stream at 10% of its byte limit with no growth measured yet.
func resourceRow(name string, tier storagereport.StorageTier) storagereport.ResourceReport {
	return storagereport.ResourceReport{
		Resource: storagereport.StorageResource{
			Name:        name,
			Kind:        natsclient.ResourceOrdinaryStream,
			Attribution: storagereport.AttributionNotApplicable,
			Tier:        tier,
			Bytes:       storagereport.NewCapacity(1000, 100, true),
			Messages:    storagereport.NewCapacity(0, 1, true),
		},
		CollectedAt: time.Now().UTC(),
		ProducedBy:  "unit-test",
		Growth: storagereport.Growth{
			State:       storagereport.GrowthUnknown,
			Unavailable: storagereport.GrowthUnavailableNoPriorObservation,
		},
	}
}

// withPressure gives row an evaluated pressure state raised by its headroom.
func withPressure(row storagereport.ResourceReport, state storagereport.PressureState) storagereport.ResourceReport {
	headroomBytes := int64(900)
	headroomFraction := 0.9
	row.Projection = storagereport.Projection{
		HeadroomBytes:    &headroomBytes,
		HeadroomFraction: &headroomFraction,
	}
	row.Pressure = storagereport.Pressure{
		Evaluated:    true,
		State:        state,
		RaisedBy:     storagereport.PressureInputHeadroom,
		FromHeadroom: state,
	}
	return row
}

// withGrowth gives row a measured growth rate and a time to its threshold.
func withGrowth(
	row storagereport.ResourceReport, bytesPerSecond float64, timeToThreshold time.Duration,
) storagereport.ResourceReport {
	rate := bytesPerSecond
	remaining := timeToThreshold
	row.Growth = storagereport.Growth{State: storagereport.GrowthKnown, BytesPerSecond: &rate}
	row.Projection.TimeToThreshold = &remaining
	return row
}

// unboundedRow is a resource that declares no byte bound, so neither its projection nor its
// pressure can be evaluated.
func unboundedRow(name string, tier storagereport.StorageTier) storagereport.ResourceReport {
	row := resourceRow(name, tier)
	row.Resource.Bytes = storagereport.NewCapacity(0, 4096, true)
	row.Projection = storagereport.Projection{
		HeadroomUnavailable:        storagereport.ProjectionUnavailableUnbounded,
		TimeToThresholdUnavailable: storagereport.ProjectionUnavailableUnbounded,
	}
	row.Pressure = storagereport.Pressure{Unavailable: storagereport.PressureUnavailableUnbounded}
	return row
}

// unknownRow is a resource whose capacity could not be read, in an unknown tier.
func unknownRow(name string) storagereport.ResourceReport {
	row := resourceRow(name, storagereport.TierUnknown)
	row.Resource.Bytes = storagereport.Capacity{State: storagereport.CapacityUnknown}
	row.Resource.Messages = storagereport.Capacity{State: storagereport.CapacityUnknown}
	row.Projection = storagereport.Projection{
		HeadroomUnavailable:        storagereport.ProjectionUnavailableUnknownCapacity,
		TimeToThresholdUnavailable: storagereport.ProjectionUnavailableUnknownCapacity,
	}
	row.Pressure = storagereport.Pressure{Unavailable: storagereport.PressureUnavailableUnknownCapacity}
	return row
}

// fillingTier is a bounded tier with a measured rate and a projection.
func fillingTier(tier storagereport.StorageTier, state storagereport.PressureState) storagereport.TierComparison {
	rate := 4096.0
	headroom := int64(1 << 20)
	remaining := 90 * time.Minute
	return storagereport.TierComparison{
		Tier:          tier,
		Limit:         storagereport.NewCapacity(1<<30, 1<<20, true),
		DeclaredBytes: 1 << 24,
		State:         storagereport.OvercommitmentWithin,
		Growth: storagereport.Growth{
			State:          storagereport.GrowthKnown,
			BytesPerSecond: &rate,
			ObservedOver:   30 * time.Minute,
		},
		Projection: storagereport.Projection{
			HeadroomBytes:   &headroom,
			TimeToThreshold: &remaining,
		},
		Pressure: storagereport.Pressure{
			Evaluated:        true,
			State:            state,
			RaisedBy:         storagereport.PressureInputTimeToThreshold,
			EvaluatedAgainst: storagereport.PressureBasisAccountTier,
		},
	}
}

// inheritedPressureRow is a resource with no bound of its own whose pressure came from its tier:
// the published shape of an archival stream.
func inheritedPressureRow(
	name string, tier storagereport.StorageTier, state storagereport.PressureState,
) storagereport.ResourceReport {
	row := unboundedRow(name, tier)
	row.Pressure = storagereport.Pressure{
		Evaluated:        true,
		State:            state,
		RaisedBy:         storagereport.PressureInputTimeToThreshold,
		EvaluatedAgainst: storagereport.PressureBasisAccountTier,
	}
	return row
}
