package embedding

import (
	"log/slog"
	"testing"
)

// TestDedupRecordUsable_RejectsIdentitylessLegacyRecord: a dedup record with no
// Model is never usable.
//
// Such a record can still be reached. EMBEDDING_INDEX pending records are durable KV
// with no TTL, and Worker.Run watches the bucket, whose snapshot re-delivers every current
// value on restart. handleKVEntry skips only NON-pending records, so a stale pending
// record whose ContentHash was derived from the text alone is replayed as pending, and
// hop 2 uses that ContentHash verbatim:
//
//  1. An entity is queued under embedder_type bm25 with a text-only key. Its pending
//     record holds ContentHash(text); EMBEDDING_DEDUP[ContentHash(text)] holds a BM25
//     vector with no Model.
//  2. The operator switches to http against the same NATS state.
//  3. Hop 2 replays the stale pending record and looks up that text-only key.
//  4. Were a record with no Model usable, the BM25 record would be served.
//  5. saveAndNotify would stamp that BM25 vector with the neural model's name.
//
// Both spaces are 384-dim by default, so cosine returns a plausible score and no
// health signal fires. An identityless record must be regenerated; the cost is one
// re-embed per such record, once.
func TestDedupRecordUsable_RejectsIdentitylessLegacyRecord(t *testing.T) {
	t.Parallel()

	w := NewWorker(nil, &stubEmbedder{model: "all-MiniLM-L6-v2", dimensions: 384}, slog.Default())

	legacy := &DedupRecord{Vector: []float32{9, 9, 9}} // no Model or Dimensions: identityless
	if w.dedupRecordUsable("acme.ops.robotics.gcs.drone.001", "old-layout-key", legacy) {
		t.Fatal("an identityless dedup record was treated as usable; " +
			"a replayed pre-fix pending record can serve its bm25 vector stamped with the neural model's name (gh#612)")
	}

	matching := &DedupRecord{Vector: []float32{1, 1, 1}, Model: "all-MiniLM-L6-v2", Dimensions: 384}
	if !w.dedupRecordUsable("acme.ops.robotics.gcs.drone.001", "current-key", matching) {
		t.Fatal("a record from this embedder's own vector space must stay usable")
	}

	mismatched := &DedupRecord{Vector: []float32{1, 1, 1}, Model: "bm25-384", Dimensions: 384}
	if w.dedupRecordUsable("acme.ops.robotics.gcs.drone.001", "current-key", mismatched) {
		t.Fatal("a record from a different vector space must not be usable")
	}
}
