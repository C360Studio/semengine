//go:build integration

// Integration tests for HierarchyInference integration into graph-ingest component
// Most async watcher tests have been removed - see hierarchy_sync_integration_test.go
// for synchronous hierarchy inference tests.

package graphingest

import (
	"encoding/json"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/natsclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ====================================================================================
// Configuration Tests
// ====================================================================================

func TestComponent_HierarchyInference_ConfigDefaults(t *testing.T) {
	// Verify default config has hierarchy disabled
	config := DefaultConfig()
	assert.False(t, config.EnableHierarchy, "hierarchy should be disabled by default")

	// Verify ApplyDefaults maintains hierarchy disabled
	customConfig := Config{
		Ports: &component.PortConfig{
			Inputs: []component.PortDefinition{
				{Name: "test", Config: component.JetStreamPort{StreamName: "TEST", Subjects: []string{"test.>"}}},
			},
			Outputs: []component.PortDefinition{
				{Name: "test", Config: component.KVWritePort{Bucket: "TEST"}},
			},
		},
	}
	customConfig.ApplyDefaults()
	assert.False(t, customConfig.EnableHierarchy, "ApplyDefaults should keep hierarchy disabled")
}

// ====================================================================================
// Helper Functions
// ====================================================================================

// createTestComponentWithHierarchyConfig creates a test component with specified
// hierarchy setting, on a fixture broker of the test's own with the ENTITY stream
// (design D3).
func createTestComponentWithHierarchyConfig(t *testing.T, enableHierarchy bool) *Component {
	t.Helper()

	return createHierarchyComponentOnClient(t, newFixtureClient(t, entityStream), enableHierarchy)
}

// createHierarchyComponentOnClient builds a graph-ingest component over an
// ALREADY-RUNNING NATS server, so a test can construct a second component over
// the same store — the restart shape gh#713 reports, where a fresh process
// re-registers unchanged entities against a graph that already holds them.
func createHierarchyComponentOnClient(t *testing.T, natsClient *natsclient.Client, enableHierarchy bool) *Component {
	t.Helper()

	config := DefaultConfig()
	config.EnableHierarchy = enableHierarchy

	configJSON, err := json.Marshal(config)
	require.NoError(t, err)

	comp, err := CreateGraphIngest(configJSON, testDependencies(t, natsClient))
	require.NoError(t, err)

	return comp.(*Component)
}
