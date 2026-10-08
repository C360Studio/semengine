package graphingest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/vocabulary"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ====================================================================================
// Query Handler: handleQueryPrefixNATS Tests
// ====================================================================================

func TestComponent_HandleQueryPrefix_Success(t *testing.T) {
	tests := []struct {
		name              string
		storedIDs         []string
		prefix            string
		limit             int
		expectedCount     int
		expectedContained []string
	}{
		{
			name: "match by org prefix",
			storedIDs: []string{
				"acme.ops.logistics.warehouse.robot.robot-001",
				"acme.ops.logistics.warehouse.robot.robot-002",
				"c360.platform.robotics.mav1.drone.001",
			},
			prefix:        "acme",
			limit:         10,
			expectedCount: 2,
			expectedContained: []string{
				"acme.ops.logistics.warehouse.robot.robot-001",
				"acme.ops.logistics.warehouse.robot.robot-002",
			},
		},
		{
			name: "match by full prefix path",
			storedIDs: []string{
				"c360.platform.robotics.mav1.drone.001",
				"c360.platform.robotics.mav1.drone.002",
				"c360.platform.robotics.mav2.drone.001",
			},
			prefix:        "c360.platform.robotics.mav1",
			limit:         10,
			expectedCount: 2,
			expectedContained: []string{
				"c360.platform.robotics.mav1.drone.001",
				"c360.platform.robotics.mav1.drone.002",
			},
		},
		{
			name: "empty prefix returns all",
			storedIDs: []string{
				"acme.ops.logistics.warehouse.robot.robot-001",
				"c360.platform.robotics.mav1.drone.001",
			},
			prefix:        "",
			limit:         10,
			expectedCount: 2,
			expectedContained: []string{
				"acme.ops.logistics.warehouse.robot.robot-001",
				"c360.platform.robotics.mav1.drone.001",
			},
		},
		{
			name: "limit restricts count",
			storedIDs: []string{
				"c360.platform.robotics.mav1.drone.001",
				"c360.platform.robotics.mav1.drone.002",
				"c360.platform.robotics.mav1.drone.003",
			},
			prefix:        "c360",
			limit:         2,
			expectedCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comp := createTestComponentWithMockKV(t, authorityOfFixture(t, tt.storedIDs[0]))
			ctx := context.Background()

			// These tables deliberately mix deployments — "match by org prefix"
			// has no meaning otherwise — so an ID under this component's own
			// authority is created through the production path and one under a
			// peer's is seeded as the mirror an import lane would have left.
			for _, id := range tt.storedIDs {
				seedOwnedOrMirrored(t, comp, &graph.EntityState{
					ID:          id,
					MessageType: testEntityType(),
					Triples: []message.Triple{
						{
							Subject:   id,
							Predicate: "test.entity.predicate",
							Object:    "test-value",
							Timestamp: time.Now(),
						},
					},
					UpdatedAt: time.Now(),
				})
			}

			// Create request
			request := map[string]any{
				"prefix": tt.prefix,
				"limit":  tt.limit,
			}
			requestJSON, err := json.Marshal(request)
			require.NoError(t, err)

			// Call handler
			responseJSON, err := comp.handleQueryPrefixWithMaxPayload(ctx, requestJSON, 1<<20)
			require.NoError(t, err)

			// Parse response - should be entities envelope
			var resp struct {
				Entities []graph.EntityState `json:"entities"`
			}
			err = json.Unmarshal(responseJSON, &resp)
			require.NoError(t, err, "response should be valid entities envelope JSON")

			assert.Equal(t, tt.expectedCount, len(resp.Entities), "should return expected count")

			// Verify expected entities are contained
			entityIDs := make(map[string]bool)
			for _, e := range resp.Entities {
				entityIDs[e.ID] = true
				// Verify entity has triples (full entity, not just ID)
				assert.NotEmpty(t, e.Triples, "entity %s should have triples", e.ID)
			}

			for _, expectedID := range tt.expectedContained {
				assert.True(t, entityIDs[expectedID], "should contain entity %s", expectedID)
			}
		})
	}
}

func TestComponent_HandleQueryPrefix_ReturnsFullEntities(t *testing.T) {
	// This test specifically verifies that the response contains full entity data
	// not just entity IDs (the bug that was fixed)
	comp := createTestComponentWithMockKV(t, withAuthority("c360", "platform"))
	ctx := context.Background()

	// Create entity with triples
	entity := &graph.EntityState{
		ID:          "c360.platform.robotics.mav1.drone.001",
		MessageType: testEntityType(),
		Triples: []message.Triple{
			{
				Subject:   "c360.platform.robotics.mav1.drone.001",
				Predicate: "robotics.status.armed",
				Object:    true,
				Timestamp: time.Now(),
			},
			{
				Subject:   "c360.platform.robotics.mav1.drone.001",
				Predicate: "robotics.battery.level",
				Object:    85.5,
				Timestamp: time.Now(),
			},
		},
		UpdatedAt: time.Now(),
	}
	require.NoError(t, comp.CreateEntity(ctx, entity))

	// Query by prefix
	request := map[string]any{
		"prefix": "c360",
		"limit":  10,
	}
	requestJSON, err := json.Marshal(request)
	require.NoError(t, err)

	responseJSON, err := comp.handleQueryPrefixWithMaxPayload(ctx, requestJSON, 1<<20)
	require.NoError(t, err)

	// Parse response as entities envelope
	var resp struct {
		Entities []graph.EntityState `json:"entities"`
	}
	err = json.Unmarshal(responseJSON, &resp)
	require.NoError(t, err, "response should be valid entities envelope")

	require.Len(t, resp.Entities, 1, "should return 1 entity")

	// Verify it's a full entity with all data
	assert.Equal(t, entity.ID, resp.Entities[0].ID)
	// 2 user triples + the ADR-054 entity.indexing.profile stamp (appended at
	// creation). No producer declared a profile here, so it defaults to the
	// control floor; the user triples keep their original leading order.
	assert.Len(t, resp.Entities[0].Triples, 3, "should have 2 user triples + the indexing-profile stamp")
	assert.Equal(t, "robotics.status.armed", resp.Entities[0].Triples[0].Predicate)
	profile, ok := resp.Entities[0].GetPropertyValue(vocabulary.EntityIndexingProfile)
	assert.True(t, ok, "created entity should carry entity.indexing.profile")
	assert.Equal(t, vocabulary.IndexingProfileControl, profile)
}

func TestComponent_HandleQueryPrefix_InvalidRequest(t *testing.T) {
	comp := createTestComponentWithMockKV(t, withAuthority("c360", "logistics"))
	ctx := context.Background()

	// Malformed JSON
	_, err := comp.handleQueryPrefixWithMaxPayload(ctx, []byte(`{invalid json}`), 1<<20)
	assert.Error(t, err, "should return error for invalid JSON")
}

func TestComponent_HandleQueryPrefix_NoMatches(t *testing.T) {
	comp := createTestComponentWithMockKV(t, withAuthority("c360", "platform"))
	ctx := context.Background()

	// Store some entities
	entity := &graph.EntityState{
		ID:          "c360.platform.robotics.mav1.drone.001",
		MessageType: testEntityType(),
		Triples:     []message.Triple{},
		UpdatedAt:   time.Now(),
	}
	require.NoError(t, comp.CreateEntity(ctx, entity))

	// Query with non-matching prefix
	request := map[string]any{
		"prefix": "nonexistent",
		"limit":  10,
	}
	requestJSON, err := json.Marshal(request)
	require.NoError(t, err)

	responseJSON, err := comp.handleQueryPrefixWithMaxPayload(ctx, requestJSON, 1<<20)
	require.NoError(t, err)

	var resp struct {
		Entities []graph.EntityState `json:"entities"`
	}
	err = json.Unmarshal(responseJSON, &resp)
	require.NoError(t, err)

	assert.Empty(t, resp.Entities, "should return empty array when no matches")
}
