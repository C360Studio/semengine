//go:build integration

package clustering

import (
	"testing"

	"github.com/c360studio/semengine/natsclient"
)

// TestUnchangedRecordAddsNoRevision: on a real COMMUNITY_INDEX, a tick with no change, and
// a tick after a change that leaves every record equal, add no revision to the bucket.
func TestUnchangedRecordAddsNoRevision(t *testing.T) {
	kv, storage := communityBucket(t)
	m, err := NewMaintainer(storage)
	if err != nil {
		t.Fatal(err)
	}
	id := "acme.dep1.src.docs.note.n1"
	mustUpsert(t, m, id, 1, "alpha beta")
	m.BootstrapDone()
	mustTick(t, m)

	lastSeq := func() uint64 {
		t.Helper()
		seq, err := natsclient.BucketLastSeq(t.Context(), kv)
		if err != nil {
			t.Fatal(err)
		}
		return seq
	}
	published := lastSeq()
	if published != 3 {
		t.Fatalf("bucket last sequence %d after the first tick, want 3 (one per group)", published)
	}

	mustTick(t, m)
	if seq := lastSeq(); seq != published {
		t.Fatalf("an unchanged tick moved the bucket from %d to %d", published, seq)
	}
	mustUpsert(t, m, id, 2, "beta alpha")
	mustTick(t, m)
	if seq := lastSeq(); seq != published {
		t.Fatalf("a tick with equal records moved the bucket from %d to %d", published, seq)
	}
}

// TestKVRevisionStartsAtOne is the probe behind refusing revision 0 (Maintainer.Upsert):
// on the pinned nats.go and nats-server, the first entry of a new bucket has revision 1,
// a stream sequence, so 0 is no entry's revision. That it is never 0 in general is not
// promised by nats.go's documentation.
func TestKVRevisionStartsAtOne(t *testing.T) {
	kv, _ := communityBucket(t)
	rev, err := kv.Put(t.Context(), "0.acme.dep1.src.docs", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if rev != 1 {
		t.Fatalf("first revision = %d, want 1", rev)
	}
}
