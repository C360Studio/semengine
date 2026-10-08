//go:build integration

package graphingest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_QueryHandlers tests query handlers with real NATS JetStream.
// Only the production-wire (NATS-suffixed) handlers are exercised here;
// the former msg-style handlers (handleQueryEntity, handleQueryBatch) were
// deleted as part of gh#164 part 1 dead-code cleanup.
func TestIntegration_QueryHandlers(t *testing.T) {
	ctx, cancelOperation := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancelOperation()

	natsClient := newFixtureClient(t, entityStream)

	// Create component
	config := DefaultConfig()
	deps := component.Dependencies{
		NATSClient:      natsClient,
		PayloadRegistry: newTestPayloadRegistry(t),
		Platform:        component.PlatformMeta{Org: testDeploymentOrg, Platform: testDeploymentPlatform},
	}

	configJSON, err := json.Marshal(config)
	require.NoError(t, err)

	comp, err := CreateGraphIngest(configJSON, deps)
	require.NoError(t, err)

	component := comp.(*Component)
	owner := newGraphIngestTestOwner(component)
	defer owner.finish(ctx, t)
	require.NoError(t, component.Initialize())
	require.NoError(t, component.Start(owner.startContext(ctx)))

	// The pin slept 100ms here "for component to be ready". Start returns only once
	// the buckets are provisioned and the query handlers are subscribed, so nothing
	// was being waited for.

	// Create test entities
	entities := []*graph.EntityState{
		{
			ID:          "c360.platform.robotics.mav1.drone.001",
			MessageType: testEntityType(),
			Triples: withTestMetadata(
				message.Triple{
					Subject:   "c360.platform.robotics.mav1.drone.001",
					Predicate: "robotics.status.armed",
					Object:    true,
					Timestamp: time.Now(),
				},
			),
			UpdatedAt: time.Now(),
		},
		{
			ID:          "c360.platform.robotics.mav1.drone.002",
			MessageType: testEntityType(),
			Triples: withTestMetadata(
				message.Triple{
					Subject:   "c360.platform.robotics.mav1.drone.002",
					Predicate: "robotics.battery.level",
					Object:    85.5,
					Timestamp: time.Now(),
				},
			),
			UpdatedAt: time.Now(),
		},
	}

	// Store entities
	for _, entity := range entities {
		require.NoError(t, component.CreateEntity(ctx, entity))
	}

	t.Run("batch query with real NATS", func(t *testing.T) {
		// Use the component's built-in batch query handler (registered during Start)
		batchSubject := "graph.ingest.query.batch"

		// Send batch query request
		request := map[string][]string{
			"ids": {
				"c360.platform.robotics.mav1.drone.001",
				"c360.platform.robotics.mav1.drone.002",
			},
		}
		requestJSON, err := json.Marshal(request)
		require.NoError(t, err)

		// The batch handler is classified (natsclient's RPC error contract), so it is
		// called with RequestClassified; the pin used Request, which decodes an error
		// reply's body as if it were a success.
		responseData, err := natsClient.RequestClassified(ctx, batchSubject, requestJSON, wireRequestTimeout)
		require.NoError(t, err)

		// Verify response - batch query returns {"entities": [...]} format
		var response struct {
			Entities []graph.EntityState `json:"entities"`
		}
		err = json.Unmarshal(responseData, &response)
		require.NoError(t, err)

		assert.Equal(t, 2, len(response.Entities))
	})
}
