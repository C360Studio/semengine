package embedding

import (
	"log/slog"
	"testing"
)

// TestDedupRecordUsable_RejectsIdentitylessLegacyRecord: a dedup record with no
// Model is never usable.
//
// The Worker stamps Model and Dimensions on every dedup record it writes, but
// EMBEDDING_DEDUP is durable, has no TTL and is never cleared, so a record read
// from it may lack them. Such a record does not say which vector space its vector
// belongs to: served under an http embedder, a BM25 vector would be stored under
// the neural model's name. Both spaces are 384-dim by default, so cosine returns a
// plausible score and no health signal fires. An identityless record is
// regenerated instead; the cost is one re-embed per such record, once.
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
