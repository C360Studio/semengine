//go:build integration

package graphingest

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/internal/cache"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/vocabulary"
)

// newUnconfirmedWriteComponent builds graph-ingest through its factory over the fixture's
// ENTITY_STATES wrapped in a natsfixture.FaultKV, with an entity query cache, and returns the
// component, the wrapper and the real bucket. It is not started: the tests call the write and
// query methods directly, so no goroutine reads the bucket the test sets.
func newUnconfirmedWriteComponent(ctx context.Context, t *testing.T) (*Component, *natsfixture.FaultKV, jetstream.KeyValue) {
	t.Helper()
	client := newFixtureClient(t)
	comp := createTestComponent(t, withAuthority("c360", "test"))
	comp.natsClient = client
	entityStates, err := kvcatalog.EnsureCatalogBucket(ctx, client, graph.BucketEntityStates)
	require.NoError(t, err)
	bucket := natsfixture.NewFaultKV(entityStates)
	comp.entityBucket = client.NewKVStore(bucket)
	entityCache, err := cache.NewSimple[graph.EntityState]()
	require.NoError(t, err)
	comp.entityCache = entityCache
	t.Cleanup(func() { _ = entityCache.Close() })
	return comp, bucket, entityStates
}

// batchQuery reads ids through the batch query handler, which serves from the entity cache.
func batchQuery(ctx context.Context, t *testing.T, c *Component, ids ...string) graph.EntityBatchResponse {
	t.Helper()
	body, err := json.Marshal(graph.EntityBatchRequest{IDs: ids})
	require.NoError(t, err)
	raw, err := c.handleQueryBatchNATS(ctx, body)
	require.NoError(t, err)
	var resp graph.EntityBatchResponse
	require.NoError(t, json.Unmarshal(raw, &resp))
	return resp
}

// TestIntegration_UnconfirmedAppendInvalidatesCache is task 2.1 (#174; design D1). A batch query
// caches entity E; an append whose update the bucket stores is answered with nats.ErrTimeout, so
// graph-ingest sees an error for a write that stood. The next batch query must return the
// appended statement, not the entry cached before the write.
func TestIntegration_UnconfirmedAppendInvalidatesCache(t *testing.T) {
	ctx, cancelOperation := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancelOperation()
	t.Cleanup(vocabulary.SnapshotRegistry())
	vocabulary.Register("test.identity.name")
	vocabulary.Register("test.event.seen")

	c, bucket, entityStates := newUnconfirmedWriteComponent(ctx, t)
	const id = "c360.test.unconfirmed.system.widget.append"
	require.NoError(t, c.CreateEntity(ctx, &graph.EntityState{
		ID: id, MessageType: testEntityType(),
		Triples: withTestMetadata(message.Triple{Subject: id, Predicate: "test.identity.name", Object: "widget"}),
	}))

	seen := func(entity graph.EntityState) bool {
		return slices.ContainsFunc(entity.Triples, func(triple message.Triple) bool {
			return triple.Predicate == "test.event.seen" && triple.Object == "unconfirmed-event"
		})
	}
	before := batchQuery(ctx, t, c, id)
	require.Len(t, before.Entities, 1)
	require.False(t, seen(before.Entities[0]), "the entity is cached before the append")

	// Backend write timeout: the update reaches the bucket, which stores it, and graph-ingest
	// sees nats.ErrTimeout in place of the bucket's reply.
	bucket.FailAfter(natsfixture.KVUpdate, nats.ErrTimeout)
	result, err := c.addTriplesLane(ctx, []message.Triple{{
		Subject: id, Predicate: "test.event.seen", Object: "unconfirmed-event",
		Source: fixtureSource, Timestamp: fixtureTime.Add(time.Minute), Confidence: 1.0,
	}}, dedupLaneAddBatch)
	require.Error(t, err, "the append's reply was lost")
	assert.NotContains(t, result.CommittedRevisions, id)
	assert.Equal(t, 1, bucket.Calls()[natsfixture.KVUpdate], "the update reached the real bucket once")
	bucket.FailAfter(natsfixture.KVUpdate, nil)
	stored, err := entityStates.Get(ctx, id)
	require.NoError(t, err)
	require.True(t, seen(decodeEntity(t, id, stored.Value())), "the fault let the append stand")

	after := batchQuery(ctx, t, c, id)
	require.Len(t, after.Entities, 1)
	assert.True(t, seen(after.Entities[0]),
		"a batch query after an append that stood must not be served the entry cached before it")
}

// TestIntegration_UnconfirmedDeleteClearsPoison is task 2.1 (#174; design D1). Entity E is
// stored as a value graph-ingest refuses and recorded in the poison inventory; the operator's
// repair, a delete at its revision, is stored by the bucket and answered with nats.ErrTimeout.
// The inventory must no longer hold E, read through DebugStatus, and Health must be healthy:
// the failed delete adds to the error count, which Health reports and does not read (design
// D3, task 3.1).
func TestIntegration_UnconfirmedDeleteClearsPoison(t *testing.T) {
	ctx, cancelOperation := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancelOperation()

	c, bucket, entityStates := newUnconfirmedWriteComponent(ctx, t)
	// Health reads running; the component is not started (the tests call the write methods
	// directly), so the test marks it running as poisonScopingTestComponent does.
	c.running = true
	const id = "c360.test.unconfirmed.system.widget.poisoned"
	revision, err := entityStates.Put(ctx, id,
		[]byte(`{"id":"`+id+`","triples":[{"subject":"`+id+`","predicate":"legacy.predicate","object":1}]}`)) // predicate-audit:invalid {"kind":"stored-predicate","value":"legacy.predicate","reason":"arity"}
	require.NoError(t, err)

	_, err = c.handleQueryEntityNATS(ctx, []byte(`{"id":"`+id+`"}`))
	assertIngestResetRequired(t, err)
	poisoned := func() bool {
		status, ok := c.DebugStatus().(entityPoisonDebugStatus)
		require.True(t, ok)
		return slices.ContainsFunc(status.PoisonedEntities, func(entry entityPoisonDebugEntry) bool {
			return entry.EntityID == id
		})
	}
	require.True(t, poisoned(), "the read records the refused value in the poison inventory")

	bucket.FailAfter(natsfixture.KVDelete, nats.ErrTimeout)
	err = c.deleteEntityAtRevision(ctx, id, revision)
	require.ErrorIs(t, err, nats.ErrTimeout, "the delete's reply was lost")
	assert.Equal(t, 1, bucket.Calls()[natsfixture.KVDelete], "the delete reached the real bucket once")
	bucket.FailAfter(natsfixture.KVDelete, nil)
	_, err = entityStates.Get(ctx, id)
	require.True(t, natsclient.IsKVNotFoundError(err), "the fault let the delete stand: %v", err)

	assert.False(t, poisoned(), "a delete that stood must clear the poison record of the key it removed")
	health := c.Health()
	assert.True(t, health.Healthy, "the repaired component must read healthy: %+v", health)
}
