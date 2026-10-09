//go:build integration

// Integration tests for HierarchyInference integration into graph-ingest component
// Most async watcher tests have been removed - see hierarchy_sync_integration_test.go
// for synchronous hierarchy inference tests.

package graphingest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/vocabulary"
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

// TestComponent_HierarchyContainerDeletedIsBornAgain holds #130 and design D21
// through the real bucket: the inference keeps no record of which containers
// exist, so after a type's container is deleted through graph-ingest's own
// delete, the next birth of that type creates the container again and is born
// with its container edge. At the pin a cache answered for the deleted
// container, the inverse edge was refused for an absent subject, and the entity
// was born without hierarchy.
func TestComponent_HierarchyContainerDeletedIsBornAgain(t *testing.T) {
	ctx, cancelOperation := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancelOperation()
	comp := createTestComponentWithHierarchyConfig(t, true)
	owner := newGraphIngestTestOwner(comp)
	defer owner.finish(ctx, t)
	require.NoError(t, comp.Initialize())
	require.NoError(t, comp.Start(owner.startContext(ctx)))

	const first = "c360.platform.robotics.mav1.drone.first"
	const second = "c360.platform.robotics.mav1.drone.second"
	const containerID = "c360.platform.robotics.mav1.drone.group"

	birth := func(entityID string) {
		t.Helper()
		now := time.Now()
		require.NoError(t, comp.CreateEntity(ctx, &graph.EntityState{
			ID:          entityID,
			MessageType: testEntityType(),
			Triples: withTestMetadata(message.Triple{
				Subject: entityID, Predicate: "entity.type.class", Object: "robotics.drone", Timestamp: now,
			}),
			UpdatedAt: now,
		}))
	}
	stored := func(entityID string) graph.EntityState {
		t.Helper()
		entry, err := comp.entityBucket.Get(ctx, entityID)
		require.NoError(t, err, "%s is stored", entityID)
		var entity graph.EntityState
		require.NoError(t, json.Unmarshal(entry.Value, &entity))
		return entity
	}

	birth(first)
	entry, err := comp.entityBucket.Get(ctx, containerID)
	require.NoError(t, err, "the first birth creates the type container")
	deleteRequest, err := json.Marshal(graph.DeleteEntityRequest{EntityID: containerID, ExpectedRevision: entry.Revision})
	require.NoError(t, err)
	_, err = comp.handleCanonicalDelete(ctx, deleteRequest)
	require.NoError(t, err)
	_, err = comp.entityBucket.Get(ctx, containerID)
	require.True(t, natsclient.IsKVNotFoundError(err), "the container is deleted: %v", err)

	birth(second)

	assert.Equal(t, 1, statementCount(stored(second).Triples, vocabulary.HierarchyTypeMember, containerID),
		"the second entity is born with its container edge")
	container := stored(containerID)
	assert.Equal(t, 1, statementCount(container.Triples, "entity.type.class", "hierarchy.container"),
		"the container exists again")
	assert.Equal(t, 1, statementCount(container.Triples, vocabulary.HierarchyTypeContains, second))
	assert.Zero(t, statementCount(container.Triples, vocabulary.HierarchyTypeContains, first),
		"the deleted container's statements went with it")
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
