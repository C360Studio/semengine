//go:build integration

// gh#713 regression: a graph-ingest restart against an existing graph must not
// append already-present triples or advance entity revisions when nothing in
// the world changed.
//
// createEntity calls AddToContainers UNCONDITIONALLY (component.go, before the
// KV write), so a re-registration of an already-present ID runs the inference
// again before the atomic Create returns natsclient.ErrKVKeyExists.
// mergeEntityOnLane (the pin's MergeEntity, design D6), by contrast, gates
// hierarchy behind an absence probe. At the pin that inference committed
// container-inverse edges as side effects, which is what re-fired on the request
// lane; since ruling M (#91 comment 6080973822) it writes nothing to a container
// that exists, so the replay must move nothing at all.
//
// A test that only re-submits triples through add_batch never reaches that
// path, so this test replays through CreateEntity — the same conflict path a
// re-registering producer takes.

package graphingest

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/vocabulary"
)

// replayEntityIDs are three entities of one type, the shape gh#713 measured on
// the model registry (containers 6->9, endpoints 4->6 / 3->5 / 2->4).
var replayEntityIDs = []string{
	"c360.platform.robotics.mav1.drone.replay001",
	"c360.platform.robotics.mav1.drone.replay002",
	"c360.platform.robotics.mav1.drone.replay003",
}

// replayContainerIDs are the three containers hierarchy inference materialises
// for that type.
var replayContainerIDs = []string{
	"c360.platform.robotics.mav1.drone.group",
	"c360.platform.robotics.mav1.group.container",
	"c360.platform.robotics.group.container.level",
}

// replayEntity builds the registration payload for one entity. A fresh struct
// per call is required: createEntity APPENDS hierarchy triples onto the
// candidate it is handed, so reusing one would make the second registration a
// different request.
func replayEntity(id string) *graph.EntityState {
	return &graph.EntityState{
		ID:          id,
		MessageType: testEntityType(),
		Triples: withTestMetadata(message.Triple{
			Subject:   id,
			Predicate: "entity.type.class",
			Object:    "robotics.drone",
			Timestamp: time.Now(),
		}),
		UpdatedAt: time.Now(),
	}
}

// entitySnapshot is everything a replay must leave untouched: the KV revision
// and the exact multiset of stored triple identities. The pin also compared
// EntityState.Version; design D15 removes it, and the exact KV revision
// compared below is the fence it stood beside.
type entitySnapshot struct {
	revision uint64
	triples  []string
}

func snapshotStore(t *testing.T, ctx context.Context, comp *Component) map[string]entitySnapshot {
	t.Helper()
	keys, err := comp.entityBucket.Keys(ctx)
	require.NoError(t, err)

	snapshot := make(map[string]entitySnapshot, len(keys))
	for _, key := range keys {
		entry, getErr := comp.entityBucket.Get(ctx, key)
		require.NoError(t, getErr)
		var stored graph.EntityState
		require.NoError(t, graph.UnmarshalEntityState(entry.Value, &stored))

		identities := make([]string, 0, len(stored.Triples))
		for i := range stored.Triples {
			identities = append(identities, message.AppendIdentityKey(stored.Triples[i]))
		}
		sort.Strings(identities)
		snapshot[key] = entitySnapshot{
			revision: entry.Revision,
			triples:  identities,
		}
	}
	return snapshot
}

func countStoredPredicate(t *testing.T, ctx context.Context, comp *Component, id, predicate string) int {
	t.Helper()
	entry, err := comp.entityBucket.Get(ctx, id)
	require.NoError(t, err)
	var stored graph.EntityState
	require.NoError(t, graph.UnmarshalEntityState(entry.Value, &stored))
	count := 0
	for _, triple := range stored.Triples {
		if triple.Predicate == predicate {
			count++
		}
	}
	return count
}

// Tasks 7.1 / 7.3 / 6.3.
func TestComponent_HierarchyReplay_UnchangedEntitiesAdvanceNoRevision(t *testing.T) {
	ctx, cancelOperation := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancelOperation()
	natsClient := newFixtureClient(t, entityStream)

	// ---- First boot: seed the graph. -------------------------------------
	seed := createHierarchyComponentOnClient(t, natsClient, true)
	seedOwner := newGraphIngestTestOwner(seed)
	defer seedOwner.finish(ctx, t)
	require.NoError(t, seed.Initialize())
	require.NoError(t, seed.Start(seedOwner.startContext(ctx)))
	for _, id := range replayEntityIDs {
		require.NoError(t, seed.CreateEntity(ctx, replayEntity(id)))
	}
	require.NoError(t, seedOwner.stop(ctx))

	// ---- Restart: a FRESH component over the SAME store. -----------------
	// Initialize + Start is the production startup path; initStorage re-acquires
	// ENTITY_STATES through the catalog seam exactly as a new process would.
	replay := createHierarchyComponentOnClient(t, natsClient, true)
	replayOwner := newGraphIngestTestOwner(replay)
	defer replayOwner.finish(ctx, t)
	require.NoError(t, replay.Initialize())
	require.NoError(t, replay.Start(replayOwner.startContext(ctx)))

	// Nothing is writing now (the seed component is stopped, the replay
	// component has not been asked to do anything), so this is a quiescent read.
	before := snapshotStore(t, ctx, replay)

	// Seed sanity — WITHOUT these the comparison below could pass vacuously on
	// an empty or degenerate store. These are the counts gh#713's arithmetic
	// depends on: 3 entities + 3 containers. No container holds a `contains`
	// edge (ruling M, #91 comment 6080973822) and no entity a sibling edge
	// (ruling G, #91 comment 6062681355).
	require.Len(t, before, len(replayEntityIDs)+len(replayContainerIDs),
		"seed must have produced 3 entities and 3 containers")
	contains := []string{vocabulary.HierarchyTypeContains, vocabulary.HierarchySystemContains, vocabulary.HierarchyDomainContains}
	for i, predicate := range contains {
		require.Zero(t, countStoredPredicate(t, ctx, replay, replayContainerIDs[i], predicate),
			"%s must hold no %s edge", replayContainerIDs[i], predicate)
	}
	for _, id := range replayEntityIDs {
		require.Zero(t, countStoredPredicate(t, ctx, replay, id, vocabulary.HierarchyTypeSibling),
			"%s must hold no sibling edge", id)
	}

	// ---- Replay the UNCHANGED entities through the 409 path. -------------
	for _, id := range replayEntityIDs {
		err := replay.CreateEntity(ctx, replayEntity(id))
		require.ErrorIs(t, err, natsclient.ErrKVKeyExists,
			"%s already exists, so its re-registration must be a 409 — that is the trigger", id)
	}

	// ---- Nothing may have moved. ----------------------------------------
	after := snapshotStore(t, ctx, replay)

	require.Len(t, after, len(before), "replay must not create keys")
	compared := 0
	for key, want := range before {
		got, ok := after[key]
		require.True(t, ok, "%s disappeared across the replay", key)
		assert.Equal(t, want.revision, got.revision,
			"%s: KV revision advanced on a replay with no source change", key)
		assert.Equal(t, want.triples, got.triples,
			"%s: stored triple cardinality changed across the replay", key)
		compared++
	}
	require.Equal(t, len(replayEntityIDs)+len(replayContainerIDs), compared,
		"every seeded key must have been compared — a skipped comparison proves nothing")

	// Task 6.3: a re-registration re-derives no container edge, so the replay
	// suppresses nothing; the suppressed-duplicates counter, whose one lane is
	// append, holds no series.
	assert.Zero(t, testutil.CollectAndCount(replay.duplicateTriplesSuppressed),
		"the replay must suppress nothing on any lane: it re-derives no container edge")
}
