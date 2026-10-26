package graphingest

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/types"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ====================================================================================
// Metrics Registry Tests
// ====================================================================================

// The pin's TestGetEntitiesUpdatedMetric_NilRegistry and _ValidRegistry tested the
// package-level getter getEntitiesUpdatedMetric and its sync.Once global. Design D4
// removes both: each component builds its collectors and registers them on its own
// registry. A nil registry is TestComponent_MetricsRegistry_CreateComponent below and
// TestGraphIngestNilRegistryRegistersNothing; a valid one is
// TestGraphIngestMetricsRegisterOnItsRegistry.

func TestComponent_MetricsUpdate_EntityOperations(t *testing.T) {
	comp := createTestComponentWithMockKV(t)
	ctx := newTestContext(t)
	before := testutil.ToFloat64(comp.entitiesUpdated)

	entity := newTestEntity("c360.platform.test.sys.type.001")
	require.NoError(t, comp.CreateEntity(ctx, entity))
	assert.Equal(t, before+1, testutil.ToFloat64(comp.entitiesUpdated), "a created entity counts one update")

	// A second create of the same key is refused and updates nothing.
	require.Error(t, comp.CreateEntity(ctx, newTestEntity(entity.ID)))
	assert.Equal(t, before+1, testutil.ToFloat64(comp.entitiesUpdated), "a refused create counts no update")
}

func TestComponent_MetricsAtomic_MessageProcessing(t *testing.T) {
	comp, bucket := createTestComponentWithMockKVBucket(t)
	ctx := newTestContext(t)
	messages := comp.DataFlow().Messages
	bytes := comp.DataFlow().Bytes
	errorCount := comp.Health().ErrorCount

	// An in-process create is not an input message (#302; design D2): DataFlow counts
	// stream deliveries and mutation requests only.
	require.NoError(t, comp.CreateEntity(ctx, newTestEntity("c360.platform.test.sys.type.001")))
	assert.Equal(t, messages, comp.DataFlow().Messages, "an in-process create counts no message")
	assert.Equal(t, bytes, comp.DataFlow().Bytes, "an in-process create counts no bytes")
	assert.Equal(t, errorCount, comp.Health().ErrorCount, "a successful write counts no error")

	bucket.createFunc = func(context.Context, string, []byte, ...jetstream.KVCreateOpt) (uint64, error) {
		return 0, errors.New("injected KV failure")
	}
	require.Error(t, comp.CreateEntity(ctx, newTestEntity("c360.platform.test.sys.type.002")))
	assert.Equal(t, errorCount+1, comp.Health().ErrorCount, "a failed write counts one error")
	assert.Equal(t, messages, comp.DataFlow().Messages, "a failed in-process write counts no message")
}

func TestComponent_MetricsRegistry_CreateComponent(t *testing.T) {
	tests := []struct {
		name           string
		provideMetrics bool
	}{
		{
			name:           "with metrics registry",
			provideMetrics: true,
		},
		{
			name:           "without metrics registry (nil)",
			provideMetrics: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newTestDependencies(t)

			if tt.provideMetrics {
				deps.MetricsRegistry = metric.NewMetricsRegistry()
			} else {
				deps.MetricsRegistry = nil
			}

			comp, err := createComponentWithDeps(t, DefaultConfig(), deps)
			require.NoError(t, err, "component creation should succeed with or without a registry")

			// The counter counts whether or not a registry exports it.
			comp.entityBucket = deps.NATSClient.NewKVStore(newMockKVBucket())
			before := testutil.ToFloat64(comp.entitiesUpdated)
			require.NoError(t, comp.CreateEntity(newTestContext(t), newTestEntity("c360.platform.test.sys.type.001")))
			assert.Equal(t, before+1, testutil.ToFloat64(comp.entitiesUpdated))
		})
	}
}

// ====================================================================================
// Helper Functions
// ====================================================================================

func newTestDependencies(t *testing.T) component.Dependencies {
	t.Helper()

	natsClient := newTestNATSClient(t)

	return component.Dependencies{
		NATSClient:      natsClient,
		PayloadRegistry: newTestPayloadRegistry(t),
		Platform:        types.PlatformMeta{Org: testDeploymentOrg, Platform: testDeploymentPlatform},
		// MetricsRegistry intentionally not set - tests will set as needed
	}
}

// newTestNATSClient creates a NATS client for testing (unconnected)
func newTestNATSClient(t *testing.T) *natsclient.Client {
	t.Helper()

	natsClient, err := natsclient.NewClient("")
	require.NoError(t, err)

	return natsClient
}

// createComponentWithDeps creates a component with specified dependencies
func createComponentWithDeps(t *testing.T, config Config, deps component.Dependencies) (*Component, error) {
	t.Helper()

	configJSON, err := json.Marshal(config)
	require.NoError(t, err)

	comp, err := CreateGraphIngest(configJSON, deps)
	if err != nil {
		return nil, err
	}

	return comp.(*Component), nil
}
