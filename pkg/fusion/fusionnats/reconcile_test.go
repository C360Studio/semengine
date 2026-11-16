package fusionnats

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/pkg/fusion"
)

func ent(id string) graph.EntityState { return graph.EntityState{ID: id} }

// TestReconcileHydration covers the ADR-084 set logic: what a handler might
// send that a client trusting the reply would mishandle. These are the named
// witnesses of TestReconcileTotal.
func TestReconcileHydration(t *testing.T) {
	a, b, c := "acme.ops.d.s.t.a", "acme.ops.d.s.t.b", "acme.ops.d.s.t.c"

	t.Run("restores request order", func(t *testing.T) {
		// The handler answers cache hits first; the engine ranks by position.
		got := reconcileHydration([]string{a, b, c}, graph.EntityBatchResponse{
			Entities: []graph.EntityState{ent(c), ent(a), ent(b)},
		})
		require.Len(t, got.Entities, 3)
		assert.Equal(t, []string{a, b, c},
			[]string{got.Entities[0].ID, got.Entities[1].ID, got.Entities[2].ID})
		assert.Empty(t, got.Unhydrated)
	})

	t.Run("carries the handler's reported reason", func(t *testing.T) {
		got := reconcileHydration([]string{a, b}, graph.EntityBatchResponse{
			Entities: []graph.EntityState{ent(a)},
			Missing:  []graph.MissingEntity{{ID: b, Reason: graph.MissingNotFound}},
		})
		require.Len(t, got.Entities, 1)
		require.Len(t, got.Unhydrated, 1)
		assert.Equal(t, fusion.Unhydrated{Handle: b, Reason: fusion.UnhydratedNotFound}, got.Unhydrated[0])
	})

	t.Run("an out-of-set reason is unknown", func(t *testing.T) {
		got := reconcileHydration([]string{a, b}, graph.EntityBatchResponse{
			Missing: []graph.MissingEntity{{ID: a, Reason: graph.MissingError}, {ID: b, Reason: "later"}},
		})
		assert.Equal(t, []fusion.Unhydrated{
			{Handle: a, Reason: fusion.UnhydratedError},
			{Handle: b, Reason: fusion.UnhydratedUnknown},
		}, got.Unhydrated)
	})

	t.Run("an ID in neither list is unknown, not not_found", func(t *testing.T) {
		got := reconcileHydration([]string{a, b}, graph.EntityBatchResponse{
			Entities: []graph.EntityState{ent(a)},
		})
		require.Len(t, got.Unhydrated, 1)
		assert.Equal(t, fusion.Unhydrated{Handle: b, Reason: fusion.UnhydratedUnknown}, got.Unhydrated[0])
	})

	t.Run("every requested ID is accounted for exactly once", func(t *testing.T) {
		got := reconcileHydration([]string{a, b, c}, graph.EntityBatchResponse{
			Entities: []graph.EntityState{ent(b)},
			Missing:  []graph.MissingEntity{{ID: a, Reason: graph.MissingNotFound}},
		})
		seen := map[string]int{}
		for _, e := range got.Entities {
			seen[e.ID]++
		}
		for _, u := range got.Unhydrated {
			seen[u.Handle]++
		}
		assert.Equal(t, map[string]int{a: 1, b: 1, c: 1}, seen)
	})

	t.Run("a duplicate requested ID is reported once", func(t *testing.T) {
		got := reconcileHydration([]string{a, a}, graph.EntityBatchResponse{})
		assert.Len(t, got.Unhydrated, 1, "one entity, one report, even if asked for twice")
		got = reconcileHydration([]string{a, a}, graph.EntityBatchResponse{
			Entities: []graph.EntityState{ent(a)},
		})
		assert.Len(t, got.Entities, 1)
	})

	t.Run("an ID in both lists counts as hydrated", func(t *testing.T) {
		got := reconcileHydration([]string{a}, graph.EntityBatchResponse{
			Entities: []graph.EntityState{ent(a)},
			Missing:  []graph.MissingEntity{{ID: a, Reason: graph.MissingNotFound}},
		})
		assert.Len(t, got.Entities, 1)
		assert.Empty(t, got.Unhydrated)
	})

	t.Run("an entity nobody asked for is dropped", func(t *testing.T) {
		got := reconcileHydration([]string{a}, graph.EntityBatchResponse{
			Entities: []graph.EntityState{ent(a), ent(b)},
		})
		require.Len(t, got.Entities, 1)
		assert.Equal(t, a, got.Entities[0].ID)
	})

	t.Run("nothing hydrated reports every ID", func(t *testing.T) {
		got := reconcileHydration([]string{a, b}, graph.EntityBatchResponse{
			Missing: []graph.MissingEntity{
				{ID: a, Reason: graph.MissingNotFound},
				{ID: b, Reason: graph.MissingNotFound},
			},
		})
		assert.Empty(t, got.Entities)
		assert.Len(t, got.Unhydrated, 2, "an all-missing batch must not look like an empty graph")
	})
}
