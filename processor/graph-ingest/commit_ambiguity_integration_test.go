//go:build integration

package graphingest

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/projection"
	"github.com/c360studio/semengine/vocabulary"
)

// foreignWriteBucket is the entity bucket these tests give graph-ingest: a natsfixture.FaultKV
// over the fixture's ENTITY_STATES, which can also make one write of its own to a key just before
// graph-ingest's next update of it. That write moves the key to a new revision, so the bucket
// refuses graph-ingest's update for its revision, as it refuses a writer that lost a race.
type foreignWriteBucket struct {
	*natsfixture.FaultKV

	mu    sync.Mutex
	armed bool
}

// writeAheadOfNextUpdate arms one foreign write: the next Update first stores the key's current
// value again, under a new revision.
func (b *foreignWriteBucket) writeAheadOfNextUpdate() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.armed = true
}

func (b *foreignWriteBucket) Update(ctx context.Context, key string, value []byte, revision uint64) (uint64, error) {
	b.mu.Lock()
	armed := b.armed
	b.armed = false
	b.mu.Unlock()
	if armed {
		current, err := b.Get(ctx, key)
		if err != nil {
			return 0, err
		}
		if _, err := b.Put(ctx, key, current.Value()); err != nil {
			return 0, err
		}
	}
	return b.FaultKV.Update(ctx, key, value, revision)
}

// TestIntegration_TypedClientPreservesCommitAmbiguity is task 4.1 (#20; design D8, row "#20 (Q7)";
// projection-mutation, "Commit ambiguity is preserved"). graph-ingest's query and mutation handlers
// are served on a fixture broker over the fixture's ENTITY_STATES, wrapped in a FaultKV, and the
// typed projection.MutationClient calls them over the wire. An update the bucket applied while
// graph-ingest saw a timeout reaches the caller as commit-unknown; once the fault is cleared, a
// revision conflict the bucket refused and an entity not found reach it as not committed.
//
// The handlers are subscribed after the bucket is set, as TestIntegration_PrefixQuery_
// IndivisibleEntityTooLarge does, so the subscription goroutines start after the write to
// entityBucket: a Start would subscribe them over its own bucket first.
func TestIntegration_TypedClientPreservesCommitAmbiguity(t *testing.T) {
	ctx, cancelOperation := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancelOperation()
	t.Cleanup(vocabulary.SnapshotRegistry())
	vocabulary.Register("test.identity.name")
	vocabulary.Register("test.state.value")

	client := newFixtureClient(t)
	comp := createTestComponent(t, withAuthority("c360", "test"))
	comp.natsClient = client
	entityStates, err := kvcatalog.EnsureCatalogBucket(ctx, client, graph.BucketEntityStates)
	require.NoError(t, err)
	bucket := &foreignWriteBucket{FaultKV: natsfixture.NewFaultKV(entityStates)}
	comp.entityBucket = client.NewKVStore(bucket)
	require.NoError(t, comp.setupQueryHandlers(ctx))
	require.NoError(t, comp.setupMutationHandlers(ctx))
	require.NoError(t, client.GetConnection().Flush())

	contract := projection.Contract{
		Name: "commit-ambiguity", MessageType: testMutationType, EntityPattern: "*.*.mutation.system.widget.*",
		BirthPredicates: []string{"test.identity.name"},
		Groups: []projection.PredicateGroup{
			{Name: "state", Mode: projection.ModeReconcile, Predicates: []string{"test.state.value"}},
		},
	}
	mutations, err := projection.NewMutationClient(projection.MutationClientConfig{
		NATS: client, Contracts: []projection.Contract{contract}, Timeout: wireRequestTimeout,
	})
	require.NoError(t, err)

	const id = "c360.test.mutation.system.widget.ambiguous"
	created, err := mutations.Create(ctx, projection.CreateMutation{
		Contract: contract.Name, Entity: &graph.EntityState{ID: id},
		Triples: []message.Triple{
			{Subject: id, Predicate: "test.identity.name", Object: "widget"},
			{Subject: id, Predicate: "test.state.value", Object: "created"},
		},
		Metadata: projection.MutationMetadata{RequestID: "create-ambiguous", Source: fixtureSource, Timestamp: fixtureTime},
	})
	require.NoError(t, err)
	require.Equal(t, projection.CommitVerified, created.Commit)

	reconcile := func(object string, at time.Time) (projection.MutationReceipt, error) {
		return mutations.Reconcile(ctx, projection.ReconcileMutation{
			Contract: contract.Name, Group: "state", EntityID: id,
			Desired:  []message.Triple{{Subject: id, Predicate: "test.state.value", Object: object}},
			Metadata: projection.MutationMetadata{Source: fixtureSource, Timestamp: at},
		})
	}

	// Backend write timeout: the update reaches the bucket, which applies it, and graph-ingest
	// sees nats.ErrTimeout in place of the bucket's reply.
	bucket.FailAfter(natsfixture.KVUpdate, nats.ErrTimeout)
	receipt, err := reconcile("unconfirmed", fixtureTime.Add(time.Minute))
	require.Equal(t, projection.CommitUnknown, receipt.Commit, "receipt of an update the server did not confirm")
	var mutationErr *projection.MutationError
	require.ErrorAs(t, err, &mutationErr)
	assert.Equal(t, projection.MutationCommitUnknown, mutationErr.Kind)
	assert.Equal(t, projection.CommitUnknown, mutationErr.Commit)
	assert.Equal(t, errs.ErrorTransient, mutationErr.Class)
	assert.Equal(t, graph.ErrorCodeInternal, mutationErr.Code)
	var classified *errs.ClassifiedError
	require.ErrorAs(t, err, &classified, "the reply's classification stays on the error")
	assert.Equal(t, errs.ErrorTransient, classified.Class)
	assert.Equal(t, graph.ErrorCodeInternal, classified.Code)
	assert.Equal(t, 1, bucket.Calls()[natsfixture.KVUpdate], "the update reached the real bucket once")
	applied, err := entityStates.Get(ctx, id)
	require.NoError(t, err)
	assert.Greater(t, applied.Revision(), created.KVRevision, "the fault let the write stand")
	assert.Equal(t, "unconfirmed", stateValue(t, applied.Value()))

	bucket.FailAfter(natsfixture.KVUpdate, nil)

	// Revision conflict: the bucket refuses the update, so nothing of this request is stored.
	bucket.writeAheadOfNextUpdate()
	receipt, err = reconcile("conflicted", fixtureTime.Add(2*time.Minute))
	assert.Equal(t, projection.CommitNotCommitted, receipt.Commit)
	require.ErrorAs(t, err, &mutationErr)
	assert.Equal(t, projection.MutationRevisionConflict, mutationErr.Kind)
	assert.Equal(t, projection.CommitNotCommitted, mutationErr.Commit)
	assert.Equal(t, errs.ErrorInvalid, mutationErr.Class)
	assert.Equal(t, graph.ErrorCodeRevisionMismatch, mutationErr.Code)
	refused, err := entityStates.Get(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, applied.Revision()+1, refused.Revision(), "only the foreign write moved the revision")
	assert.Equal(t, "unconfirmed", stateValue(t, refused.Value()))

	// Entity not found: graph-ingest refuses the delete before any write.
	receipt, err = mutations.Delete(ctx, projection.DeleteMutation{
		EntityID: "c360.test.mutation.system.widget.absent", ExpectedRevision: 1,
		Metadata: projection.MutationMetadata{RequestID: "delete-absent"},
	})
	assert.Equal(t, projection.CommitNotCommitted, receipt.Commit)
	require.ErrorAs(t, err, &mutationErr)
	assert.Equal(t, projection.MutationNotFound, mutationErr.Kind)
	assert.Equal(t, projection.CommitNotCommitted, mutationErr.Commit)
	assert.Equal(t, graph.ErrorCodeEntityNotFound, mutationErr.Code)
}

// stateValue is the object of the one test.state.value statement in a stored entity.
func stateValue(t *testing.T, stored []byte) any {
	t.Helper()
	entity := decodeEntity(t, stored)
	var objects []any
	for _, triple := range entity.Triples {
		if triple.Predicate == "test.state.value" {
			objects = append(objects, triple.Object)
		}
	}
	require.Len(t, objects, 1, "test.state.value statements")
	return objects[0]
}
