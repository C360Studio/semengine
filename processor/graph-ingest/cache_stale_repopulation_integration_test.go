//go:build integration

package graphingest

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/internal/cache"
	"github.com/c360studio/semengine/internal/harness/semantictest"
	"github.com/c360studio/semengine/message"
)

// pausingGetBucket is ENTITY_STATES with one pause: the first Get of key returns its entry
// only after the test closes release, and closes paused once the entry has been read. A
// cache-miss reader captures the key's invalidation generation before its Get and
// repopulates the cache after it, so the pause holds the reader in that window.
type pausingGetBucket struct {
	jetstream.KeyValue
	key     string
	paused  chan struct{}
	release chan struct{}
	// claimed is set by the one Get that pauses; every other Get, the concurrent
	// write's included, passes straight through.
	claimed atomic.Bool
}

func (b *pausingGetBucket) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	entry, err := b.KeyValue.Get(ctx, key)
	if key == b.key && b.claimed.CompareAndSwap(false, true) {
		close(b.paused)
		<-b.release
	}
	return entry, err
}

// TestIntegration_CacheStaleRepopulationRace is the deterministic regression
// lock for the read-through-cache stale-repopulation race in the entity query
// cache (30s TTL).
//
// The bug: the cache-miss read path did KV Get -> unmarshal -> Set with no guard
// against a concurrent invalidate landing between the Get and the Set. A reader
// that loaded rev1 from KV, then was pre-empted while a concurrent write
// committed rev2 and invalidated the (already-absent) cache entry, would Set rev1
// back into the cache AFTER the invalidate — resurrecting pre-write state for the
// full TTL and violating read-after-write coherence (the gated-DAG dispatch stall
// was one victim).
//
// The fix establishes a per-key invalidation-generation guard: the repopulating
// Set is dropped if the key was invalidated during the read window. This test
// drives that window deterministically with explicit channels (no sleeps): the
// bucket under ENTITY_STATES pauses the reader's KV Get once it has read rev1,
// before the reader's Set; the test commits rev2 (marker) while it is paused,
// then releases the reader. Without the guard
// the reader's stale Set wins and the final read observes rev1 (no marker);
// with the guard the Set is skipped and the final read observes rev2.
func TestIntegration_CacheStaleRepopulationRace(t *testing.T) {
	ctx, cancelOperation := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancelOperation()
	const id = "stalecache.ops.dom.sys.type.entity1"

	// The component is not started: the test gives it the ENTITY_STATES store and the
	// entity query cache Start would build, so no running goroutine reads the store
	// it is given.
	client := newFixtureClient(t)
	c := createTestComponent(t, withAuthority("stalecache", "ops"))
	c.natsClient = client
	entityStates, err := kvcatalog.EnsureCatalogBucket(ctx, client, graph.BucketEntityStates)
	require.NoError(t, err)
	// The first Get of id is the reader's below: the seed creates without reading it.
	bucket := &pausingGetBucket{
		KeyValue: entityStates, key: id, paused: make(chan struct{}), release: make(chan struct{}),
	}
	c.entityBucket = client.NewKVStore(bucket)
	entityCache, err := cache.NewSimple[graph.EntityState]()
	require.NoError(t, err)
	c.entityCache = entityCache
	t.Cleanup(func() { _ = entityCache.Close() })

	// rev1: create the entity WITHOUT the marker. CreateEntity invalidates the
	// cache, so the first read below is a guaranteed cache miss.
	seedPrefixEntity(t, ctx, c, id)

	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(bucket.release) }) }

	// Reader: misses the cache, reads rev1 from KV, then is paused in the Get
	// before the repopulating Set.
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		_, _, _ = c.fetchEntitiesConcurrent(ctx, []string{id}, 1)
	}()
	// This runs even if an assertion exits early. Release the paused Get exactly
	// once, then give the owned reader a finite join budget.
	defer func() {
		release()
		// The pin's 5s join budgets are 10s here: docs/testing.md asks a failure bound
		// in a test against a broker for the test's context or at least ten seconds.
		joinCtx, cancelJoin := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancelJoin()
		select {
		case <-readerDone:
		case <-joinCtx.Done():
			t.Errorf("cache reader did not join: %v", joinCtx.Err())
		}
	}()

	// Wait until the reader has read rev1 and is paused before its Set.
	select {
	case <-bucket.paused:
	case <-readerDone:
		t.Fatal("cache reader returned before its KV Get paused; race window was not exercised")
	case <-ctx.Done():
		t.Fatalf("cache reader did not reach its KV Get: %v", ctx.Err())
	}

	// rev2: commit the marker while the reader is paused. The append commits a new
	// revision AND invalidates the cache (bumps the coherence generation) — this
	// is the concurrent write W2 the paused reader's rev1 read did not see.
	appended, err := c.addTriplesLane(ctx, []message.Triple{{
		Subject:    id,
		Predicate:  semantictest.Predicate(t, "test", "stale", "marker"),
		Object:     "committed",
		Source:     fixtureSource,
		Timestamp:  time.Now(),
		Confidence: 1.0,
	}}, dedupLaneAddBatch)
	require.NoError(t, err)
	require.Contains(t, appended.CommittedRevisions, id, "the marker append must commit")

	// Release the paused reader so it attempts its (now stale) repopulating Set.
	release()
	joinCtx, cancelJoin := context.WithTimeout(ctx, 10*time.Second)
	defer cancelJoin()
	select {
	case <-readerDone:
	case <-joinCtx.Done():
		t.Fatalf("cache reader did not finish after release: %v", joinCtx.Err())
	}

	// A fresh read MUST observe rev2 (the marker), never a resurrected rev1.
	// Without the guard the paused reader's Set writes rev1 back into the cache
	// after the append's invalidate, so this read hits stale state and fails.
	got, _, err := c.fetchEntitiesConcurrent(ctx, []string{id}, 1)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.NotNil(t, got[0].GetTriple(semantictest.Predicate(t, "test", "stale", "marker")),
		"fresh read must observe the rev2 marker — a slow reader must not resurrect "+
			"pre-write rev1 into the cache past a concurrent invalidate")
}
