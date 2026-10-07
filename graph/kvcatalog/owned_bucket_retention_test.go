package kvcatalog

import (
	"testing"

	"github.com/c360studio/semengine/graph"
)

// TestIsFrameworkOwnedBucket_IncludesOperationalBuckets pins the F2/F3
// write-ownership fixes: GRAPH_INGEST_APPLIED_SEQ (redelivery-guard stamps, #715)
// and GRAPH_STATUS (readiness envelopes) are members of the owned set, so a
// generic rule update_kv cannot forge a sequence stamp or readiness at either
// guard site. Both are correctness-critical no-eviction state; the retention
// backstop ranges the catalog's no-lifecycle rows, so membership alone puts
// them under it.
func TestIsFrameworkOwnedBucket_IncludesOperationalBuckets(t *testing.T) {
	t.Parallel()
	for _, bucket := range []string{graph.BucketGraphIngestAppliedSeq, graph.BucketGraphStatus} {
		if !IsFrameworkOwnedBucket(bucket) {
			t.Errorf("%s must be reported as framework-owned", bucket)
		}
	}
}

// TestIsFrameworkOwnedBucket_NoEmbeddingsCache pins the EMBEDDINGS_CACHE
// deletion (reopen-framework-owned-bucket-guards): the dead
// created-but-never-read-or-written surface is gone from the owned set, with
// no bucket existing solely to carry a guard exemption. One owned bucket sits
// outside the retention backstop by design: a strict-retention row (the
// semengine_config_* family, ADR-104) is verified at every acquisition instead
// of reconciled by this backstop.
func TestIsFrameworkOwnedBucket_NoEmbeddingsCache(t *testing.T) {
	t.Parallel()
	if IsFrameworkOwnedBucket("EMBEDDINGS_CACHE") {
		t.Error("EMBEDDINGS_CACHE must not be reported as framework-owned (surface deleted)")
	}
}
