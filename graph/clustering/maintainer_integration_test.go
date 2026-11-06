//go:build integration

package clustering

import (
	"testing"

	"github.com/c360studio/semengine/natsclient"
)

// TestUnchangedRecordAddsNoRevision: on a real COMMUNITY_INDEX, a tick with nothing
// applied since the last adds no revision to the bucket; a same-content upsert raises its
// groups' entity_revision, so the next tick writes each of the three records once and the
// tick after it writes nothing.
func TestUnchangedRecordAddsNoRevision(t *testing.T) {
	id := "acme.dep1.src.docs.note.n1"
	// setup applies id at revision 1 on a fresh bucket and publishes its three groups; the
	// returned lastSeq reads the bucket's last sequence.
	setup := func(t *testing.T) (*Maintainer, *NATSCommunityStorage, func() uint64) {
		t.Helper()
		kv, storage := communityBucket(t)
		m, err := NewMaintainer(storage)
		if err != nil {
			t.Fatal(err)
		}
		lastSeq := func() uint64 {
			t.Helper()
			seq, err := natsclient.BucketLastSeq(t.Context(), kv)
			if err != nil {
				t.Fatal(err)
			}
			return seq
		}
		mustUpsert(t, m, id, 1, "alpha beta")
		m.BootstrapDone()
		mustTick(t, m)
		if seq := lastSeq(); seq != 3 {
			t.Fatalf("bucket last sequence %d after the first tick, want 3 (one per group)", seq)
		}
		return m, storage, lastSeq
	}

	t.Run("nothing applied", func(t *testing.T) {
		m, _, lastSeq := setup(t)
		mustTick(t, m)
		mustUpsert(t, m, id, 1, "gamma") // obsolete: at the revision already applied
		mustTick(t, m)
		if seq := lastSeq(); seq != 3 {
			t.Fatalf("ticks with nothing applied moved the bucket from 3 to %d", seq)
		}
	})

	t.Run("same-content upsert", func(t *testing.T) {
		m, storage, lastSeq := setup(t)
		mustUpsert(t, m, id, 2, "beta alpha")
		mustTick(t, m)
		if seq := lastSeq(); seq != 6 {
			t.Fatalf("bucket last sequence %d after a same-content upsert, want 6 (each group once more)", seq)
		}
		mustTick(t, m)
		if seq := lastSeq(); seq != 6 {
			t.Fatalf("a second tick moved the bucket from 6 to %d", seq)
		}
		records, err := storage.GetAllCommunities(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if len(records) != 3 {
			t.Fatalf("GetAllCommunities = %d records, want 3", len(records))
		}
		for _, r := range records {
			if r.EntityRevision != 2 {
				t.Fatalf("record %d.%s entity_revision = %d, want 2", r.Level, r.ID, r.EntityRevision)
			}
		}
	})
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
