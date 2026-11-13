package fusionnats

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
)

// Exact-entity tests: Entity reads graph-query's entity verb, whose reply is an
// ExactEntity.

func TestSliceEFusionEntityUsesExactEntityWire(t *testing.T) {
	id := "acme.ops.robotics.gcs.drone.001"
	exact := graph.ExactEntity{Entity: &graph.EntityState{ID: id, Triples: []message.Triple{
		stamped(id, "dc.terms.title", "Scout"),
	}}, KVRevision: 42}
	c := newClient(t, &fakeRequester{resp: mustJSON(t, exact)})

	entity, err := c.Entity(t.Context(), id)
	require.NoError(t, err)
	require.NotNil(t, entity)
	assert.Equal(t, id, entity.ID)
	assert.Equal(t, "Scout", entity.First("dc.terms.title"))
}

// TestSliceEFusionEntityRejectsInvalidExactEvidence: a reply with no entity or a
// zero revision is refused by the client; the wrong entity, and an entity the
// canonical contract refuses, are refused by graph.UnmarshalEntityState with a
// noncanonical_entity_id reset naming the requested ID.
func TestSliceEFusionEntityRejectsInvalidExactEvidence(t *testing.T) {
	id := "acme.ops.robotics.gcs.drone.001"
	t.Run("nil entity", func(t *testing.T) {
		_, err := newClient(t, &fakeRequester{resp: mustJSON(t, graph.ExactEntity{KVRevision: 1})}).Entity(t.Context(), id)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no entity")
	})
	t.Run("zero revision", func(t *testing.T) {
		wire := graph.ExactEntity{Entity: &graph.EntityState{ID: id}}
		_, err := newClient(t, &fakeRequester{resp: mustJSON(t, wire)}).Entity(t.Context(), id)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "zero KV revision")
	})
	for name, wire := range map[string]graph.ExactEntity{
		"requested ID mismatch": {Entity: &graph.EntityState{ID: "acme.ops.robotics.gcs.sensor.002"}, KVRevision: 1},
		"poisoned entity":       {Entity: &graph.EntityState{ID: "bad"}, KVRevision: 1},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := newClient(t, &fakeRequester{resp: mustJSON(t, wire)}).Entity(t.Context(), id)
			require.Error(t, err)
			assert.Nil(t, got)
			var refusal *graph.StateContractError
			require.True(t, errors.As(err, &refusal), "a stored-value refusal, got %v", err)
			assert.Equal(t, graph.GraphStateReasonNoncanonicalEntityID, refusal.Reason)
			assert.Equal(t, id, refusal.EntityID)
		})
	}
}
