//go:build integration

package graphembedding

import (
	"context"
	"encoding/json"
	"maps"
	"testing"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/embedding"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/graph/readiness"
	"github.com/c360studio/semengine/natsclient"
)

// The failures below are made on the test's NATS server the way an operator's stream edit would,
// with no hook in the component (#186):
//
//   - ENTITY_STATES reads fail once its stream stops allowing direct gets: the component's
//     handle was bound with direct gets on, so nats.go keeps sending them and gets no responder.
//   - EMBEDDING_INDEX writes and deletes fail once its stream's largest message is one byte: a
//     record and a delete marker (its headers) are both larger. Reads still answer.
//
// The tests show each trigger works: they observe the failure it causes before they rely on it.

// editStream changes the stream behind bucket.
func editStream(t *testing.T, client *natsclient.Client, bucket string, edit func(*jetstream.StreamConfig)) {
	t.Helper()
	js, err := client.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	stream, err := js.Stream(t.Context(), "KV_"+bucket)
	if err != nil {
		t.Fatalf("stream of %s: %v", bucket, err)
	}
	config := stream.CachedInfo().Config
	edit(&config)
	if _, err := js.UpdateStream(t.Context(), config); err != nil {
		t.Fatalf("update the stream of %s: %v", bucket, err)
	}
}

func directGets(on bool) func(*jetstream.StreamConfig) {
	return func(c *jetstream.StreamConfig) { c.AllowDirect = on }
}

func writesFail(fail bool) func(*jetstream.StreamConfig) {
	return func(c *jetstream.StreamConfig) {
		c.MaxMsgSize = -1
		if fail {
			c.MaxMsgSize = 1
		}
	}
}

// lastRevision is the revision of bucket's latest write, a delete included.
func lastRevision(t *testing.T, client *natsclient.Client, bucket string) uint64 {
	t.Helper()
	js, err := client.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	stream, err := js.Stream(t.Context(), "KV_"+bucket)
	if err != nil {
		t.Fatalf("stream of %s: %v", bucket, err)
	}
	info, err := stream.Info(t.Context())
	if err != nil {
		t.Fatalf("stream info of %s: %v", bucket, err)
	}
	return info.State.LastSeq
}

// assertFailed checks the failed map holds exactly want now.
func (h *harness) assertFailed(t *testing.T, what string, want map[string]uint64) {
	t.Helper()
	count, reasons := h.c.failed.counts()
	var total uint64
	for _, n := range want {
		total += n
	}
	if uint64(count) != total || !maps.Equal(reasons, want) {
		t.Fatalf("%s: failed %d %v; want %d %v", what, count, reasons, total, want)
	}
}

// publishedStatus reads graph-embedding's GRAPH_STATUS entry as a reader of the key does: its
// raw bytes and the envelope they decode to.
func publishedStatus(ctx context.Context, client *natsclient.Client) (status, error) {
	reader, err := kvcatalog.OpenCatalogReader(ctx, client, graph.BucketGraphStatus)
	if err != nil {
		return status{}, err
	}
	entry, err := reader.Get(ctx, readiness.KeyGraphEmbedding)
	if err != nil {
		return status{}, err
	}
	s := status{raw: map[string]json.RawMessage{}}
	if err := json.Unmarshal(entry.Value(), &s.envelope); err != nil {
		return status{}, err
	}
	return s, json.Unmarshal(entry.Value(), &s.raw)
}

type status struct {
	envelope graph.IndexStatusResponse
	raw      map[string]json.RawMessage
}

// TestStrandedWithoutRevisionClearsOnlyOnRepair: a failed entity read has no source revision, so
// the entity is stranded: its revision drains, it is counted failed, and no terminal outcome from
// the Worker clears it, at a revision above every delivered one or at the highest revision (the
// Worker's drain of an unreadable record). The repair lane's convergence clears it once the read
// answers (design D3; pin TestObsoleteTerminal_CannotClearStranding_PendingWriteSite and
// _TombstoneSite).
func TestStrandedWithoutRevisionClearsOnlyOnRepair(t *testing.T) {
	client := fixtureClient(t)
	h := startEmbedding(t, client, `{}`, nil)
	a := entityID(1)
	h.awaitGenerated(t, a, h.put(t, a, entityValue(t, a, nil, "doc.meta.title", "first")))

	editStream(t, client, graph.BucketEntityStates, directGets(false))
	rev := h.put(t, a, entityValue(t, a, nil, "doc.meta.title", "second"))
	// Hop 1 marks the read failure before it drains the revision, so the floor reaching rev
	// orders the read of the map after the mark.
	h.awaitIndexed(t, "the unread revision drained", rev)
	stranded := map[string]uint64{embedding.ReasonEntityReadFailed: 1}
	h.assertFailed(t, "after the failed read", stranded)

	// The Worker's outcomes, through its production callback: none clears the stranded entity.
	for _, outcome := range []embedding.TerminalOutcome{embedding.OutcomeGenerated, embedding.OutcomeSkipped,
		embedding.OutcomeDeleted} {
		h.c.onTerminal(t.Context(), a, rev+100, outcome, "")
		h.c.onTerminal(t.Context(), a, ^uint64(0), outcome, "")
	}
	h.c.onTerminal(t.Context(), a, ^uint64(0), embedding.OutcomeFailed, "timeout")
	h.assertFailed(t, "after the Worker's outcomes", stranded)
	// A repair pass while reads still fail marks the entity again.
	h.c.repair(t.Context())
	h.assertFailed(t, "after a repair pass that cannot read", stranded)
	if s := await(t, "the stranded entity published",
		func(ctx context.Context) (status, error) { return publishedStatus(ctx, client) },
		func(s status) bool { return s.envelope.FailedCount == 1 }); s.envelope.Ready ||
		s.envelope.State != graph.IndexStateDegraded ||
		!maps.Equal(s.envelope.FailedReasons, stranded) {
		t.Fatalf("published envelope %+v; want degraded with reasons %v", s.envelope, stranded)
	}

	editStream(t, client, graph.BucketEntityStates, directGets(true))
	h.c.repair(t.Context()) // the read answers: the pending record is written at rev
	h.assertFailed(t, "after the repair pass", nil)
	h.awaitGenerated(t, a, rev)
	s := await(t, "the repaired entity published",
		func(ctx context.Context) (status, error) { return publishedStatus(ctx, client) },
		func(s status) bool { return s.envelope.Ready })
	for _, key := range []string{"failed_count", "failed_reasons", "first_failure_at"} {
		if _, ok := s.raw[key]; ok {
			t.Errorf("published envelope after the repair carries %s: %v", key, s.raw)
		}
	}
}

// TestRepairConvergesStranded: the repair lane retries each of hop 1's three failures and only
// those (design D3; pin TestFailedDelete_MarksStrandedAndRepairs,
// TestRepairScope_EmbedderSideReasonNotReDriven). A read that succeeds while the pending write
// after it fails marks the entity again, now for the write. A tombstone whose record delete fails
// is marked at its revision; when the repair pass finds the entity absent and the delete
// succeeds, the entity converges. The Worker's failure of an entity is not retried.
func TestRepairConvergesStranded(t *testing.T) {
	client := fixtureClient(t)
	h := startEmbedding(t, client, `{}`, nil)
	a, b, gone := entityID(1), entityID(2), entityID(3)
	h.awaitGenerated(t, a, h.put(t, a, entityValue(t, a, nil, "doc.meta.title", "a first")))
	h.awaitGenerated(t, b, h.put(t, b, entityValue(t, b, nil, "doc.meta.title", "b first")))
	// The Worker's failure, through its production callback, of an entity ENTITY_STATES does not
	// hold: a repair pass that retried it would find it absent and clear it.
	h.c.onTerminal(t.Context(), gone, 5, embedding.OutcomeFailed, "timeout")
	h.assertFailed(t, "after the Worker's failure", map[string]uint64{"timeout": 1})

	editStream(t, client, graph.BucketEntityStates, directGets(false))
	editStream(t, client, graph.BucketEmbeddingIndex, writesFail(true))
	revA := h.put(t, a, entityValue(t, a, nil, "doc.meta.title", "a second"))
	h.awaitIndexed(t, "A's unread revision drained", revA)
	editStream(t, client, graph.BucketEntityStates, directGets(true))
	if err := h.states.Delete(t.Context(), b); err != nil {
		t.Fatalf("delete %s: %v", b, err)
	}
	// Hop 1 marks the failed delete before it completes the tombstone's revision.
	h.awaitIndexed(t, "B's tombstone indexed", lastRevision(t, client, graph.BucketEntityStates))
	h.assertFailed(t, "after A's read and B's delete failed", map[string]uint64{
		embedding.ReasonEntityReadFailed: 1, embedding.ReasonDeleteFailed: 1, "timeout": 1})

	// The read answers now and the write after it fails: A is marked for the write.
	h.c.repair(t.Context())
	h.assertFailed(t, "after a pass with writes failing", map[string]uint64{
		embedding.ReasonPendingWriteFailed: 1, embedding.ReasonDeleteFailed: 1, "timeout": 1})
	if record, err := h.record(t.Context(), b); err != nil || record == nil {
		t.Fatalf("B's record while deletes fail = %+v, %v; want still there", record, err)
	}

	editStream(t, client, graph.BucketEmbeddingIndex, writesFail(false))
	h.c.repair(t.Context())
	h.assertFailed(t, "after a pass with writes answering", map[string]uint64{"timeout": 1})
	if record, err := h.record(t.Context(), b); err != nil || record != nil {
		t.Errorf("B's record after the repair = %+v, %v; want none", record, err)
	}
	h.awaitGenerated(t, a, revA)
	h.awaitIndexed(t, "A's revision indexed after the repair", revA)
	s := await(t, "the Worker's failure published alone",
		func(ctx context.Context) (status, error) { return publishedStatus(ctx, client) },
		func(s status) bool { return s.envelope.FailedCount == 1 })
	if !maps.Equal(s.envelope.FailedReasons, map[string]uint64{"timeout": 1}) || s.envelope.Ready {
		t.Errorf("published envelope %+v; want not ready with reasons {timeout: 1}", s.envelope)
	}
}

// TestStartSeedsFailedMap: the failed records EMBEDDING_INDEX holds at Start are counted once
// Start returns, with their reasons, and the first envelope carries them (design D3, "Seeded from
// Storage.ScanFailed"); with none, nothing is counted. The seeded entity is not in ENTITY_STATES,
// so nothing delivered changes its record.
func TestStartSeedsFailedMap(t *testing.T) {
	t.Run("failed records", func(t *testing.T) {
		client := fixtureClient(t)
		index, err := kvcatalog.EnsureCatalogBucket(t.Context(), client, graph.BucketEmbeddingIndex)
		if err != nil {
			t.Fatal(err)
		}
		dedup, err := kvcatalog.EnsureCatalogBucket(t.Context(), client, graph.BucketEmbeddingDedup)
		if err != nil {
			t.Fatal(err)
		}
		earlier := embedding.NewStorage(client.NewKVStore(index), client.NewKVStore(dedup))
		a := entityID(1)
		if _, err := earlier.SavePendingGuarded(t.Context(), &embedding.Record{
			EntityID: a, SourceText: "failed before Start", SourceRevision: 4,
		}); err != nil {
			t.Fatal(err)
		}
		if err := earlier.SaveFailed(t.Context(), a, "embedder timed out", "timeout", 4); err != nil {
			t.Fatal(err)
		}

		h := startEmbedding(t, client, `{}`, nil)
		h.assertFailed(t, "once Start returned", map[string]uint64{"timeout": 1})
		s := await(t, "the first envelope",
			func(ctx context.Context) (status, error) { return publishedStatus(ctx, client) },
			func(status) bool { return true })
		if s.envelope.Ready || s.envelope.FailedCount != 1 ||
			!maps.Equal(s.envelope.FailedReasons, map[string]uint64{"timeout": 1}) {
			t.Errorf("first envelope %+v; want not ready, one failure, reasons {timeout: 1}", s.envelope)
		}
	})
	t.Run("none", func(t *testing.T) {
		h := startEmbedding(t, fixtureClient(t), `{}`, nil)
		if count, reasons := h.c.failed.counts(); count != 0 || reasons != nil {
			t.Errorf("failed once Start returned = %d %v; want 0 and nil", count, reasons)
		}
	})
}
