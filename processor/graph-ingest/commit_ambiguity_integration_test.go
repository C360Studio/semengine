//go:build integration

package graphingest

import (
	"context"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
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
	// The test stands in for a Start that completed, so the handlers serve (checkpoint 3 B2).
	comp.startCompleted.Store(true)
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

// TestIntegration_OversizedResponseAfterCommitIsCommitUnknown is task 4.16, the defect #173
// describes (Codex interim review R1, PR #93 comment 6112098987; projection-mutation, "Commit
// ambiguity is preserved"). A reconcile commits, and its success response, which echoes the
// request's RequestID beside the stored entity, is larger than the broker's payload limit, so
// natsclient answers with its class-invalid response_too_large reply in its place. The typed client
// reads that reply as commit-unknown: the write stands.
func TestIntegration_OversizedResponseAfterCommitIsCommitUnknown(t *testing.T) {
	// The bound covers the fixture's setup as well as the requests: in Codex's probe a 30-second
	// bound ran out before the reconcile was sent.
	ctx, cancelOperation := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancelOperation()
	t.Cleanup(vocabulary.SnapshotRegistry())
	vocabulary.Register("test.identity.name")
	vocabulary.Register("test.state.value")

	client := newFixtureClient(t)
	comp := createTestComponent(t, withAuthority("c360", "test"))
	comp.natsClient = client
	entityStates, err := kvcatalog.EnsureCatalogBucket(ctx, client, graph.BucketEntityStates)
	require.NoError(t, err)
	comp.entityBucket = client.NewKVStore(entityStates)
	// The test stands in for a Start that completed, so the handlers serve (checkpoint 3 B2).
	comp.startCompleted.Store(true)
	require.NoError(t, comp.setupQueryHandlers(ctx))
	require.NoError(t, comp.setupMutationHandlers(ctx))
	require.NoError(t, client.GetConnection().Flush())

	contract := projection.Contract{
		Name: "oversized-response", MessageType: testMutationType, EntityPattern: "*.*.mutation.system.widget.*",
		BirthPredicates: []string{"test.identity.name"},
		Groups: []projection.PredicateGroup{
			{Name: "state", Mode: projection.ModeReconcile, Predicates: []string{"test.state.value"}},
		},
	}
	mutations, err := projection.NewMutationClient(projection.MutationClientConfig{
		NATS: client, Contracts: []projection.Contract{contract}, Timeout: wireRequestTimeout,
	})
	require.NoError(t, err)

	const id = "c360.test.mutation.system.widget.oversized"
	created, err := mutations.Create(ctx, projection.CreateMutation{
		Contract: contract.Name, Entity: &graph.EntityState{ID: id},
		Triples: []message.Triple{
			{Subject: id, Predicate: "test.identity.name", Object: strings.Repeat("identity", 1024)},
			{Subject: id, Predicate: "test.state.value", Object: "created"},
		},
		Metadata: projection.MutationMetadata{RequestID: "create-oversized", Source: fixtureSource, Timestamp: fixtureTime},
	})
	require.NoError(t, err)
	require.Equal(t, projection.CommitVerified, created.Commit)

	// The request fits under the payload limit; the response adds the stored 8 KiB identity
	// statement to the echoed RequestID and crosses it. Desired is empty, so the reconcile clears
	// the state statement and stores nothing of the RequestID.
	maxPayload := int(client.GetConnection().MaxPayload())
	receipt, err := mutations.Reconcile(ctx, projection.ReconcileMutation{
		Contract: contract.Name, Group: "state", EntityID: id, ExpectedRevision: created.KVRevision,
		Metadata: projection.MutationMetadata{
			Source: fixtureSource, Timestamp: fixtureTime.Add(time.Minute), RequestID: strings.Repeat("r", maxPayload-2048),
		},
	})

	// The reconcile committed: a later revision, without the state statement, and the stored value
	// alone is far below the limit.
	stored, getErr := entityStates.Get(ctx, id)
	require.NoError(t, getErr)
	require.Greater(t, stored.Revision(), created.KVRevision, "the reconcile's write stands")
	for _, triple := range decodeEntity(t, stored.Value()).Triples {
		require.NotEqual(t, "test.state.value", triple.Predicate, "the reconcile cleared the state statement")
	}
	require.Less(t, len(stored.Value()), maxPayload/2, "the echoed RequestID, not the stored value, crosses the limit")

	var mutationErr *projection.MutationError
	require.ErrorAs(t, err, &mutationErr)
	assert.Equal(t, errs.ErrorInvalid, mutationErr.Class, "natsclient's reply in place of the oversized response")
	// natsclient's own code (errorCodeResponseTooLarge), spelled as the wire carries it.
	assert.Equal(t, "response_too_large", mutationErr.Code)
	assert.Equal(t, projection.CommitUnknown, receipt.Commit, "receipt of a reconcile that committed")
	assert.Equal(t, projection.CommitUnknown, mutationErr.Commit)
	assert.Equal(t, projection.MutationCommitUnknown, mutationErr.Kind)
}

// lostReplyBucket is the entity bucket of TestIntegration_AppendAfterUnconfirmedWriteIsCommitUnknown,
// adapted from Codex's probe (PR #93 comment 6112098987, R2). Once armed, its next Update is stored
// by the real bucket, the stored bytes are kept, another writer then changes the key (interfere),
// and the caller sees nats.ErrTimeout: a write that stood and whose reply was lost.
type lostReplyBucket struct {
	jetstream.KeyValue
	interfere func(ctx context.Context, real jetstream.KeyValue, key string) error

	armed   atomic.Bool
	mu      sync.Mutex
	written []byte
}

func (b *lostReplyBucket) Update(ctx context.Context, key string, value []byte, revision uint64) (uint64, error) {
	if !b.armed.CompareAndSwap(true, false) {
		return b.KeyValue.Update(ctx, key, value, revision)
	}
	if _, err := b.KeyValue.Update(ctx, key, value, revision); err != nil {
		return 0, err
	}
	if err := b.interfere(ctx, b.KeyValue, key); err != nil {
		return 0, err
	}
	b.mu.Lock()
	b.written = slices.Clone(value)
	b.mu.Unlock()
	return 0, nats.ErrTimeout
}

// stored is the value the armed Update stored, or nil if it did not store one and interfere.
func (b *lostReplyBucket) stored() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.written
}

// TestIntegration_AppendAfterUnconfirmedWriteIsCommitUnknown is task 4.18 (Codex interim review R2,
// PR #93 comment 6112098987; owner ruling, #91 comment 6112779037; projection-mutation, "Commit
// ambiguity is preserved"). An append's update is stored and its reply is lost, and another writer
// then deletes the entity or stores a value graph-ingest refuses (an entity under another ID). The
// receipt reports commit-unknown: the append's write stood, and a refusal of the entity as it is
// now says nothing about it. The handlers are subscribed after the bucket is set (notes/4.1.md,
// choice 5).
func TestIntegration_AppendAfterUnconfirmedWriteIsCommitUnknown(t *testing.T) {
	const (
		id    = "c360.test.mutation.system.widget.unconfirmed"
		other = "c360.test.mutation.system.widget.other"
	)
	foreign, err := graph.MarshalEntityState(&graph.EntityState{ID: other, Triples: []message.Triple{
		{Subject: other, Predicate: "test.identity.name", Object: "other", Source: fixtureSource, Timestamp: fixtureTime},
	}})
	require.NoError(t, err)

	for _, tc := range []struct {
		name      string
		interfere func(ctx context.Context, real jetstream.KeyValue, key string) error
	}{
		{"deleted", func(ctx context.Context, real jetstream.KeyValue, key string) error {
			return real.Delete(ctx, key)
		}},
		{"refused value", func(ctx context.Context, real jetstream.KeyValue, key string) error {
			_, err := real.Put(ctx, key, foreign)
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancelOperation := context.WithTimeout(t.Context(), 120*time.Second)
			defer cancelOperation()
			t.Cleanup(vocabulary.SnapshotRegistry())
			vocabulary.Register("test.identity.name")
			vocabulary.Register("test.event.seen")

			client := newFixtureClient(t)
			comp := createTestComponent(t, withAuthority("c360", "test"))
			comp.natsClient = client
			entityStates, err := kvcatalog.EnsureCatalogBucket(ctx, client, graph.BucketEntityStates)
			require.NoError(t, err)
			bucket := &lostReplyBucket{KeyValue: entityStates, interfere: tc.interfere}
			comp.entityBucket = client.NewKVStore(bucket)
			// The test stands in for a Start that completed, so the handlers serve (checkpoint 3 B2).
			comp.startCompleted.Store(true)
			require.NoError(t, comp.setupQueryHandlers(ctx))
			require.NoError(t, comp.setupMutationHandlers(ctx))
			require.NoError(t, client.GetConnection().Flush())

			contract := projection.Contract{
				Name: "unconfirmed-append", MessageType: testMutationType, EntityPattern: "*.*.mutation.system.widget.*",
				BirthPredicates: []string{"test.identity.name"},
				Groups: []projection.PredicateGroup{
					{Name: "events", Mode: projection.ModeAppend, Predicates: []string{"test.event.seen"}},
				},
			}
			mutations, err := projection.NewMutationClient(projection.MutationClientConfig{
				NATS: client, Contracts: []projection.Contract{contract}, Timeout: wireRequestTimeout,
			})
			require.NoError(t, err)
			created, err := mutations.Create(ctx, projection.CreateMutation{
				Contract: contract.Name, Entity: &graph.EntityState{ID: id},
				Triples:  []message.Triple{{Subject: id, Predicate: "test.identity.name", Object: "widget"}},
				Metadata: projection.MutationMetadata{RequestID: "create-unconfirmed", Source: fixtureSource, Timestamp: fixtureTime},
			})
			require.NoError(t, err)
			require.Equal(t, projection.CommitVerified, created.Commit)

			bucket.armed.Store(true)
			receipt, err := mutations.Append(ctx, projection.AppendMutation{
				Contract: contract.Name, Group: "events", EntityID: id,
				Triples:  []message.Triple{{Subject: id, Predicate: "test.event.seen", Object: "unconfirmed-event"}},
				Metadata: projection.MutationMetadata{RequestID: "append-unconfirmed", Source: fixtureSource, Timestamp: fixtureTime.Add(time.Minute)},
			})

			written := bucket.stored()
			require.NotNil(t, written, "the append's update must be stored, and the other writer's change made, before the reply is lost")
			assert.True(t, slices.ContainsFunc(decodeEntity(t, written).Triples, func(triple message.Triple) bool {
				return triple.Predicate == "test.event.seen" && triple.Object == "unconfirmed-event"
			}), "the stored update holds the appended statement")

			assert.Equal(t, projection.CommitUnknown, receipt.Commit, "receipt of an append whose write stood")
			var mutationErr *projection.MutationError
			require.ErrorAs(t, err, &mutationErr)
			t.Logf("receipt %s; kind %s, class %s, code %q: %v",
				receipt.Commit, mutationErr.Kind, mutationErr.Class, mutationErr.Code, err)
			assert.Equal(t, projection.MutationCommitUnknown, mutationErr.Kind)
			assert.Equal(t, projection.CommitUnknown, mutationErr.Commit)
			var classified *errs.ClassifiedError
			require.ErrorAs(t, err, &classified, "the reply's classification stays on the error")
			assert.Equal(t, errs.ErrorTransient, classified.Class)
			assert.Equal(t, graph.ErrorCodeInternal, classified.Code)
		})
	}
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
