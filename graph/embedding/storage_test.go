package embedding

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

// savePending writes a pending record through the one pending write the package
// carries (the pin's tests used the dropped SavePending as setup).
func savePending(t *testing.T, s *Storage, entityID, contentHash string, sourceRevision uint64) {
	t.Helper()
	saved, err := s.SavePendingGuarded(t.Context(), &Record{
		EntityID:       entityID,
		ContentHash:    contentHash,
		SourceText:     "text",
		SourceRevision: sourceRevision,
	})
	if err != nil {
		t.Fatalf("SavePendingGuarded: %v", err)
	}
	if !saved {
		t.Fatalf("SavePendingGuarded(%s, rev %d) did not save", entityID, sourceRevision)
	}
}

// TestSavePendingGuarded_Decisions pins the guarded pending write at the storage
// boundary. A generated record at a same-or-newer source revision is never
// downgraded to pending; everything else writes (absent: create; pending or failed:
// overwrite; generated at an OLDER revision: overwrite, because a newer source state
// is being queued).
func TestSavePendingGuarded_Decisions(t *testing.T) {
	t.Parallel()
	const entityID = "acme.ops.robotics.gcs.drone.guard1"

	type seed func(t *testing.T, s *Storage)
	cases := []struct {
		name       string
		seed       seed
		queueRev   uint64
		wantSaved  bool
		wantStatus Status
		wantRev    uint64
	}{
		{
			name:       "absent key creates pending",
			seed:       func(*testing.T, *Storage) {},
			queueRev:   5,
			wantSaved:  true,
			wantStatus: StatusPending,
			wantRev:    5,
		},
		{
			name: "pending record overwritten",
			seed: func(t *testing.T, s *Storage) {
				savePending(t, s, entityID, "", 4)
			},
			queueRev:   5,
			wantSaved:  true,
			wantStatus: StatusPending,
			wantRev:    5,
		},
		{
			name: "newer pending record overwritten by an older one",
			seed: func(t *testing.T, s *Storage) {
				savePending(t, s, entityID, "", 6)
			},
			queueRev:   5,
			wantSaved:  true,
			wantStatus: StatusPending,
			wantRev:    5,
		},
		{
			name: "failed record overwritten (re-queue recovers)",
			seed: func(t *testing.T, s *Storage) {
				savePending(t, s, entityID, "", 4)
				if err := s.SaveFailed(t.Context(), entityID, "boom", failReasonContentError, 4); err != nil {
					t.Fatal(err)
				}
			},
			queueRev:   5,
			wantSaved:  true,
			wantStatus: StatusPending,
			wantRev:    5,
		},
		{
			name: "generated at OLDER revision overwritten (newer source state)",
			seed: func(t *testing.T, s *Storage) {
				savePending(t, s, entityID, "", 4)
				if err := s.SaveGenerated(t.Context(), entityID, []float32{1}, "bm25-384", 384, "h", 4); err != nil {
					t.Fatal(err)
				}
			},
			queueRev:   5,
			wantSaved:  true,
			wantStatus: StatusPending,
			wantRev:    5,
		},
		{
			name: "generated at SAME revision skipped (restart re-delivery)",
			seed: func(t *testing.T, s *Storage) {
				savePending(t, s, entityID, "", 5)
				if err := s.SaveGenerated(t.Context(), entityID, []float32{1}, "bm25-384", 384, "h", 5); err != nil {
					t.Fatal(err)
				}
			},
			queueRev:   5,
			wantSaved:  false,
			wantStatus: StatusGenerated,
			wantRev:    5,
		},
		{
			name: "generated at NEWER revision skipped (stale repair re-drive)",
			seed: func(t *testing.T, s *Storage) {
				savePending(t, s, entityID, "", 6)
				if err := s.SaveGenerated(t.Context(), entityID, []float32{1}, "bm25-384", 384, "h", 6); err != nil {
					t.Fatal(err)
				}
			},
			queueRev:   5,
			wantSaved:  false,
			wantStatus: StatusGenerated,
			wantRev:    6,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			index := newMemKV()
			s := newTestStorage(t, index, newMemKV())
			tc.seed(t, s)
			before := index.revision(entityID)

			saved, err := s.SavePendingGuarded(t.Context(), &Record{
				EntityID:       entityID,
				SourceText:     "fresh text",
				SourceRevision: tc.queueRev,
			})
			if err != nil {
				t.Fatalf("SavePendingGuarded: %v", err)
			}
			if saved != tc.wantSaved {
				t.Fatalf("saved = %v, want %v", saved, tc.wantSaved)
			}
			if !saved && index.revision(entityID) != before {
				t.Fatalf("a refused pending write moved the key from revision %d to %d", before, index.revision(entityID))
			}

			rec, err := s.GetEmbedding(t.Context(), entityID)
			if err != nil {
				t.Fatal(err)
			}
			if rec == nil {
				t.Fatal("record absent after guarded save decision")
			}
			if rec.Status != tc.wantStatus {
				t.Fatalf("Status = %q, want %q", rec.Status, tc.wantStatus)
			}
			if rec.SourceRevision != tc.wantRev {
				t.Fatalf("SourceRevision = %d, want %d", rec.SourceRevision, tc.wantRev)
			}
			if tc.wantStatus == StatusGenerated && len(rec.Vector) == 0 {
				t.Fatal("the skip must leave the stored vector untouched")
			}
		})
	}
}

// TestSaveGenerated_NoIndexRecord_DoesNotPanicOrResurrect: a save whose index key has
// disappeared (the hop-1 tombstone deleted it while the embedder ran) is dropped with
// ErrRecordGone, and the key is not re-created.
func TestSaveGenerated_NoIndexRecord_DoesNotPanicOrResurrect(t *testing.T) {
	t.Parallel()
	index := newMemKV()
	s := newTestStorage(t, index, newMemKV())
	const entityID = "acme.ops.robotics.gcs.drone.001"

	err := s.SaveGenerated(t.Context(), entityID, []float32{1, 2, 3}, "all-MiniLM-L6-v2", 3, "content-hash", 1)
	if !errors.Is(err, ErrRecordGone) {
		t.Fatalf("SaveGenerated on a missing key = %v, want ErrRecordGone", err)
	}
	if index.has(entityID) {
		t.Fatal("SaveGenerated resurrected a deleted embedding record")
	}
}

// TestSaveFailed_NoIndexRecord_DoesNotPanicOrResurrect is the same for the failure
// lane: a record already gone has nothing to mark.
func TestSaveFailed_NoIndexRecord_DoesNotPanicOrResurrect(t *testing.T) {
	t.Parallel()
	index := newMemKV()
	s := newTestStorage(t, index, newMemKV())
	const entityID = "acme.ops.robotics.gcs.drone.002"

	err := s.SaveFailed(t.Context(), entityID, "generation failed: connection refused", failReasonConnectionRefused, 1)
	if !errors.Is(err, ErrRecordGone) {
		t.Fatalf("SaveFailed on a missing key = %v, want ErrRecordGone", err)
	}
	if index.has(entityID) {
		t.Fatal("SaveFailed resurrected a deleted embedding record")
	}
}

// TestSaveGenerated_ConcurrentDeleteIsRaceFree interleaves the tombstone delete and the
// generated write across goroutines under -race: neither may panic, and the save
// returns nil or ErrRecordGone.
func TestSaveGenerated_ConcurrentDeleteIsRaceFree(t *testing.T) {
	t.Parallel()
	s := newTestStorage(t, newMemKV(), newMemKV())
	const iterations = 200

	for i := range iterations {
		entityID := "acme.ops.robotics.gcs.drone.00" + string(rune('a'+i%26))
		savePending(t, s, entityID, "hash", uint64(i+1))

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			if err := s.DeleteEmbedding(t.Context(), entityID); err != nil {
				t.Errorf("DeleteEmbedding: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			err := s.SaveGenerated(t.Context(), entityID, []float32{1, 2, 3}, "m", 3, "hash", uint64(i+1))
			if err != nil && !errors.Is(err, ErrRecordGone) {
				t.Errorf("SaveGenerated: unexpected error %v", err)
			}
		}()
		wg.Wait()
	}
}

// TestSaveGenerated_NewerRevisionWinsOutOfOrderCompletion: revision 6 completes before
// revision 5; the index ends holding 6's vector and 5's late write is dropped with
// ErrSupersededRevision.
func TestSaveGenerated_NewerRevisionWinsOutOfOrderCompletion(t *testing.T) {
	t.Parallel()
	s := newTestStorage(t, newMemKV(), newMemKV())
	const entityID = "acme.ops.robotics.gcs.drone.001"
	savePending(t, s, entityID, "", 1)

	if err := s.SaveGenerated(t.Context(), entityID, []float32{6, 6, 6}, "m", 3, "hash-6", 6); err != nil {
		t.Fatalf("SaveGenerated(rev 6): %v", err)
	}
	if err := s.SaveGenerated(t.Context(), entityID, []float32{5, 5, 5}, "m", 3, "hash-5", 5); !errors.Is(err, ErrSupersededRevision) {
		t.Fatalf("SaveGenerated(rev 5) = %v, want ErrSupersededRevision", err)
	}

	rec, err := s.GetEmbedding(t.Context(), entityID)
	if err != nil {
		t.Fatalf("GetEmbedding: %v", err)
	}
	if rec == nil || len(rec.Vector) != 3 || rec.Vector[0] != 6 {
		t.Fatalf("index holds vector %v, want the newer revision's [6 6 6]", vectorOf(rec))
	}
	if rec.SourceRevision != 6 {
		t.Fatalf("stored SourceRevision = %d, want 6", rec.SourceRevision)
	}
}

// racingKV injects exactly one revision conflict on the first Update, running a hook
// first so a test can land a NEWER record between this writer's read and its Update.
type racingKV struct {
	*memKV
	onFirstUpdate func()
	fired         bool
}

func (k *racingKV) Update(ctx context.Context, key string, value []byte, rev uint64) (uint64, error) {
	if !k.fired {
		k.fired = true
		if k.onFirstUpdate != nil {
			k.onFirstUpdate()
		}
		return 0, jetstream.ErrKeyExists // wrong last sequence: a concurrent writer moved the key
	}
	return k.memKV.Update(ctx, key, value, rev)
}

// TestSaveGenerated_ConcurrentCommitDoesNotClobberViaCAS: the first Update conflicts
// while a newer revision (10) commits. The save re-reads, sees the newer revision and
// drops itself, leaving 10's vector.
func TestSaveGenerated_ConcurrentCommitDoesNotClobberViaCAS(t *testing.T) {
	t.Parallel()
	base := newMemKV()
	index := &racingKV{memKV: base}
	s := newTestStorage(t, index, newMemKV())
	const entityID = "acme.ops.robotics.gcs.drone.002"
	savePending(t, s, entityID, "", 1)

	index.onFirstUpdate = func() {
		newer := &Record{
			EntityID: entityID, Vector: []float32{10, 10, 10}, ContentHash: "hash-10",
			Model: "m", Dimensions: 3, Status: StatusGenerated, SourceRevision: 10,
		}
		data, _ := json.Marshal(newer)
		if _, err := base.Put(t.Context(), entityID, data); err != nil {
			t.Errorf("inject newer commit: %v", err)
		}
	}

	if err := s.SaveGenerated(t.Context(), entityID, []float32{5, 5, 5}, "m", 3, "hash-5", 5); !errors.Is(err, ErrSupersededRevision) {
		t.Fatalf("SaveGenerated(rev 5) = %v, want ErrSupersededRevision", err)
	}
	rec, err := s.GetEmbedding(t.Context(), entityID)
	if err != nil {
		t.Fatalf("GetEmbedding: %v", err)
	}
	if rec == nil || rec.SourceRevision != 10 || len(rec.Vector) != 3 || rec.Vector[0] != 10 {
		t.Fatalf("index holds %v (rev %d), want the committed rev-10 vector", vectorOf(rec), sourceRevOf(rec))
	}
}

// TestSaveGenerated_ContentHashAndVectorFromSameGeneration: the stored content hash
// and vector both come from the call, never the pending record's hash.
func TestSaveGenerated_ContentHashAndVectorFromSameGeneration(t *testing.T) {
	t.Parallel()
	s := newTestStorage(t, newMemKV(), newMemKV())
	const entityID = "acme.ops.robotics.gcs.drone.003"
	savePending(t, s, entityID, "stale-hop1-hash", 3)

	if err := s.SaveGenerated(t.Context(), entityID, []float32{7, 8, 9}, "m", 3, "fresh-hop2-key", 3); err != nil {
		t.Fatalf("SaveGenerated: %v", err)
	}
	rec, err := s.GetEmbedding(t.Context(), entityID)
	if err != nil {
		t.Fatalf("GetEmbedding: %v", err)
	}
	if rec == nil {
		t.Fatal("no record after SaveGenerated")
	}
	if rec.ContentHash != "fresh-hop2-key" {
		t.Fatalf("ContentHash = %q, want %q", rec.ContentHash, "fresh-hop2-key")
	}
	if len(rec.Vector) != 3 || rec.Vector[0] != 7 {
		t.Fatalf("Vector = %v, want [7 8 9]", rec.Vector)
	}
}

// TestSaveFailed_DoesNotClobberNewerSuccess: an older revision's late failure does not
// overwrite a newer success.
func TestSaveFailed_DoesNotClobberNewerSuccess(t *testing.T) {
	t.Parallel()
	s := newTestStorage(t, newMemKV(), newMemKV())
	const entityID = "acme.ops.robotics.gcs.drone.004"
	savePending(t, s, entityID, "", 1)

	if err := s.SaveGenerated(t.Context(), entityID, []float32{9, 9, 9}, "m", 3, "hash-6", 6); err != nil {
		t.Fatalf("SaveGenerated(rev 6): %v", err)
	}
	if err := s.SaveFailed(t.Context(), entityID, "old revision failed", failReasonEmbedderError, 5); !errors.Is(err, ErrSupersededRevision) {
		t.Fatalf("SaveFailed(rev 5) = %v, want ErrSupersededRevision", err)
	}
	rec, err := s.GetEmbedding(t.Context(), entityID)
	if err != nil {
		t.Fatalf("GetEmbedding: %v", err)
	}
	if rec == nil || rec.Status != StatusGenerated || len(rec.Vector) != 3 || rec.Vector[0] != 9 {
		t.Fatalf("record = %+v, want the newer generated vector intact", rec)
	}
}

// TestSaveGenerated_VanishedRecordStillErrRecordGone: neither save re-creates a
// vanished record.
func TestSaveGenerated_VanishedRecordStillErrRecordGone(t *testing.T) {
	t.Parallel()
	index := newMemKV()
	s := newTestStorage(t, index, newMemKV())
	const entityID = "acme.ops.robotics.gcs.drone.005"

	if err := s.SaveGenerated(t.Context(), entityID, []float32{1, 2, 3}, "m", 3, "hash", 1); !errors.Is(err, ErrRecordGone) {
		t.Fatalf("SaveGenerated on a vanished record = %v, want ErrRecordGone", err)
	}
	if index.has(entityID) {
		t.Fatal("SaveGenerated resurrected a vanished record")
	}
	if err := s.SaveFailed(t.Context(), entityID, "boom", failReasonInternal, 1); !errors.Is(err, ErrRecordGone) {
		t.Fatalf("SaveFailed on a vanished record = %v, want ErrRecordGone", err)
	}
	if index.has(entityID) {
		t.Fatal("SaveFailed resurrected a vanished record")
	}
}

// TestSaveFailed_EqualRevisionDoesNotDowngradeSuccess: a failure at revision R after a
// success at R is refused with ErrSupersededRevision; generated wins at equal revision.
func TestSaveFailed_EqualRevisionDoesNotDowngradeSuccess(t *testing.T) {
	t.Parallel()
	s := newTestStorage(t, newMemKV(), newMemKV())
	const entityID = "acme.ops.robotics.gcs.drone.eqrev"
	const rev = uint64(9)
	savePending(t, s, entityID, "", rev)

	if err := s.SaveGenerated(t.Context(), entityID, []float32{4, 5, 6}, "m", 3, "hash-R", rev); err != nil {
		t.Fatalf("SaveGenerated(R): %v", err)
	}
	if err := s.SaveFailed(t.Context(), entityID, "late duplicate failure", failReasonInternal, rev); !errors.Is(err, ErrSupersededRevision) {
		t.Fatalf("SaveFailed(R) after SaveGenerated(R) = %v, want ErrSupersededRevision", err)
	}
	rec, err := s.GetEmbedding(t.Context(), entityID)
	if err != nil {
		t.Fatalf("GetEmbedding: %v", err)
	}
	if rec == nil || rec.Status != StatusGenerated || len(rec.Vector) != 3 || rec.Vector[0] != 4 {
		t.Fatalf("record = %+v, want the generated vector [4 5 6] intact", rec)
	}
}

// TestSaveGenerated_EqualRevisionWinsOverPriorFailure: a success at revision R after a
// failure at R lands.
func TestSaveGenerated_EqualRevisionWinsOverPriorFailure(t *testing.T) {
	t.Parallel()
	s := newTestStorage(t, newMemKV(), newMemKV())
	const entityID = "acme.ops.robotics.gcs.drone.eqrev2"
	const rev = uint64(9)
	savePending(t, s, entityID, "", rev)

	if err := s.SaveFailed(t.Context(), entityID, "boom", failReasonInternal, rev); err != nil {
		t.Fatalf("SaveFailed(R): %v", err)
	}
	if err := s.SaveGenerated(t.Context(), entityID, []float32{7, 8, 9}, "m", 3, "hash-R", rev); err != nil {
		t.Fatalf("SaveGenerated(R) after SaveFailed(R): %v", err)
	}
	rec, err := s.GetEmbedding(t.Context(), entityID)
	if err != nil {
		t.Fatalf("GetEmbedding: %v", err)
	}
	if rec == nil || rec.Status != StatusGenerated || len(rec.Vector) != 3 || rec.Vector[0] != 7 {
		t.Fatalf("record = %+v, want the generated vector [7 8 9]", rec)
	}
}

// conflictKV refuses every Update as a revision conflict and counts the attempts.
type conflictKV struct {
	*memKV
	mu      sync.Mutex
	updates int
}

func (k *conflictKV) Update(context.Context, string, []byte, uint64) (uint64, error) {
	k.mu.Lock()
	k.updates++
	k.mu.Unlock()
	return 0, jetstream.ErrKeyExists
}

// TestSaveExhaustionMatchesMaxRetriesExceeded (design D3, changed): when every
// compare-and-set conflicts, each read-modify-write gives up after the KVStore's own
// budget (MaxRetries+1 attempts) with an error matching
// natsclient.ErrKVMaxRetriesExceeded, and the stored record is unchanged. At the pin
// the budget was the package's own 3 and the error ErrCASExhausted.
func TestSaveExhaustionMatchesMaxRetriesExceeded(t *testing.T) {
	t.Parallel()
	const (
		entityID   = "acme.ops.robotics.gcs.drone.cas"
		dedupKey   = "dedup-key"
		maxRetries = 4
	)
	budget := func(o *natsclient.KVOptions) {
		o.MaxRetries = maxRetries
		o.RetryDelay = time.Millisecond
		o.MaxRetryDelay = time.Millisecond
	}
	cases := []struct {
		name string
		call func(ctx context.Context, s *Storage) error
	}{
		{"SavePendingGuarded", func(ctx context.Context, s *Storage) error {
			_, err := s.SavePendingGuarded(ctx, &Record{EntityID: entityID, SourceText: "x", SourceRevision: 2})
			return err
		}},
		{"SaveGenerated", func(ctx context.Context, s *Storage) error {
			return s.SaveGenerated(ctx, entityID, []float32{1}, "m", 1, "h", 2)
		}},
		{"SaveFailed", func(ctx context.Context, s *Storage) error {
			return s.SaveFailed(ctx, entityID, "boom", failReasonInternal, 2)
		}},
		{"SaveDedup", func(ctx context.Context, s *Storage) error {
			return s.SaveDedup(ctx, dedupKey, []float32{1}, "other-model", 1)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			index := &conflictKV{memKV: newMemKV()}
			dedup := &conflictKV{memKV: newMemKV()}
			pending, _ := json.Marshal(&Record{EntityID: entityID, Status: StatusPending, SourceRevision: 1})
			if _, err := index.Put(t.Context(), entityID, pending); err != nil {
				t.Fatal(err)
			}
			old, _ := json.Marshal(&DedupRecord{Vector: []float32{9}, Model: "m", Dimensions: 1})
			if _, err := dedup.Put(t.Context(), dedupKey, old); err != nil {
				t.Fatal(err)
			}
			s := NewStorage(newTestKVStore(t, index, budget), newTestKVStore(t, dedup, budget))

			err := tc.call(t.Context(), s)
			if !errors.Is(err, natsclient.ErrKVMaxRetriesExceeded) {
				t.Fatalf("%s under constant conflict = %v, want natsclient.ErrKVMaxRetriesExceeded", tc.name, err)
			}
			if got := index.updates + dedup.updates; got != maxRetries+1 {
				t.Fatalf("%s made %d compare-and-set attempts, want the KVStore's budget %d", tc.name, got, maxRetries+1)
			}
			rec, err := s.GetEmbedding(t.Context(), entityID)
			if err != nil {
				t.Fatal(err)
			}
			if rec == nil || rec.Status != StatusPending || rec.SourceRevision != 1 {
				t.Fatalf("record after exhaustion = %+v, want the pending record at revision 1", rec)
			}
		})
	}
}

// deleteKV answers every Delete with a fixed error.
type deleteKV struct {
	*memKV
	err error
}

func (k *deleteKV) Delete(context.Context, string, ...jetstream.KVDeleteOpt) error { return k.err }

// TestDeleteAtRevisionOutcomes (design D3, the no-text removal): the fenced removal
// reports removed when the delete succeeds, nothing removed and no error when the key
// moved or is gone, and the error otherwise. The real server's fence is driven by
// TestRevisionGuardHistoryMatchesModel; this pins the mapping of each answer.
func TestDeleteAtRevisionOutcomes(t *testing.T) {
	t.Parallel()
	transport := errors.New("transport failed")
	cases := []struct {
		name        string
		deleteErr   error
		wantRemoved bool
		wantErr     error
	}{
		{"removed", nil, true, nil},
		{"key moved", jetstream.ErrKeyExists, false, nil},
		{"key gone", jetstream.ErrKeyNotFound, false, nil},
		{"other error", transport, false, transport},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := newTestStorage(t, &deleteKV{memKV: newMemKV(), err: tc.deleteErr}, newMemKV())
			removed, err := s.deleteAtRevision(t.Context(), "acme.ops.robotics.gcs.drone.fence", 7)
			if removed != tc.wantRemoved {
				t.Fatalf("removed = %v, want %v", removed, tc.wantRemoved)
			}
			if tc.wantErr == nil && err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want one matching %v", err, tc.wantErr)
			}
		})
	}
}

// TestSaveDedup_RecordsVectorSpace: a dedup record carries the model and dimensions of
// its vector, and a second save from the same vector space leaves the stored record as
// it was, writing nothing (design D3, changed: the pin rewrote it to append an entity
// ID to the dropped EntityIDs field).
func TestSaveDedup_RecordsVectorSpace(t *testing.T) {
	t.Parallel()
	dedup := newMemKV()
	s := newTestStorage(t, newMemKV(), dedup)

	ident := EmbedderIdentity{Type: "http", Model: "all-MiniLM-L6-v2", Dimensions: 384, MaxTextLen: 8000}
	key := DedupKey(ident, "hello world")

	if err := s.SaveDedup(t.Context(), key, []float32{1, 2, 3}, ident.Model, ident.Dimensions); err != nil {
		t.Fatalf("SaveDedup: %v", err)
	}
	got, err := s.GetByContentHash(t.Context(), key)
	if err != nil {
		t.Fatalf("GetByContentHash: %v", err)
	}
	if got == nil {
		t.Fatal("dedup record not found at its own key")
	}
	if got.Model != ident.Model || got.Dimensions != ident.Dimensions {
		t.Fatalf("identity = (%q, %d), want (%q, %d)", got.Model, got.Dimensions, ident.Model, ident.Dimensions)
	}
	revision := dedup.revision(key)
	entry, err := dedup.Get(t.Context(), key)
	if err != nil {
		t.Fatal(err)
	}
	stored := string(entry.Value())

	// Same content, same vector space, a different vector: the stored record stands.
	if err := s.SaveDedup(t.Context(), key, []float32{4, 5, 6}, ident.Model, ident.Dimensions); err != nil {
		t.Fatalf("SaveDedup (same space): %v", err)
	}
	if dedup.revision(key) != revision {
		t.Fatalf("a same-space SaveDedup moved the key from revision %d to %d; it must write nothing", revision, dedup.revision(key))
	}
	entry, err = dedup.Get(t.Context(), key)
	if err != nil {
		t.Fatal(err)
	}
	if string(entry.Value()) != stored {
		t.Fatalf("stored record changed to %s, want %s", entry.Value(), stored)
	}
}

// TestDedupIdentityChange_NeverServesStaleVector: write under one identity, read under
// another, and the stale vector is unreachable.
func TestDedupIdentityChange_NeverServesStaleVector(t *testing.T) {
	t.Parallel()
	s := newTestStorage(t, newMemKV(), newMemKV())

	const text = "an entity description that is identical under both configs"
	bm25 := EmbedderIdentity{Type: "bm25", Model: "bm25-384", Dimensions: 384, MaxTextLen: 4000}
	neural := EmbedderIdentity{Type: "http", Model: "all-MiniLM-L6-v2", Dimensions: 384, MaxTextLen: 8000}

	if err := s.SaveDedup(t.Context(), DedupKey(bm25, text), []float32{9, 9, 9}, bm25.Model, bm25.Dimensions); err != nil {
		t.Fatalf("SaveDedup under bm25: %v", err)
	}
	got, err := s.GetByContentHash(t.Context(), DedupKey(neural, text))
	if err != nil {
		t.Fatalf("GetByContentHash under neural identity: %v", err)
	}
	if got != nil {
		t.Fatalf("stale bm25 vector %v served to the neural embedder", got.Vector)
	}
}

// TestSaveDedup_ReplacesRecordFromAnotherVectorSpace: a record with no identity, or
// another model's, is replaced by the freshly generated vector, never annotated.
func TestSaveDedup_ReplacesRecordFromAnotherVectorSpace(t *testing.T) {
	t.Parallel()
	s := newTestStorage(t, newMemKV(), newMemKV())
	const key = "legacy-content-hash"
	freshVector := []float32{1, 2, 3}

	if err := s.SaveDedup(t.Context(), key, []float32{9, 9, 9}, "", 0); err != nil {
		t.Fatalf("SaveDedup (identityless): %v", err)
	}
	if err := s.SaveDedup(t.Context(), key, freshVector, "all-MiniLM-L6-v2", 384); err != nil {
		t.Fatalf("SaveDedup (regenerated): %v", err)
	}
	got, err := s.GetByContentHash(t.Context(), key)
	if err != nil {
		t.Fatalf("GetByContentHash: %v", err)
	}
	if got == nil {
		t.Fatal("dedup record missing")
	}
	if len(got.Vector) != len(freshVector) || got.Vector[0] != freshVector[0] {
		t.Fatalf("Vector = %v, want the regenerated %v", got.Vector, freshVector)
	}
	if got.Model != "all-MiniLM-L6-v2" || got.Dimensions != 384 {
		t.Fatalf("identity = (%q, %d), want (all-MiniLM-L6-v2, 384)", got.Model, got.Dimensions)
	}
}

func putRecord(t *testing.T, kv *replayKV, rec *Record) {
	t.Helper()
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := kv.Put(t.Context(), rec.EntityID, data); err != nil {
		t.Fatalf("put: %v", err)
	}
}

// TestScanFailed_SeedsFromSnapshot: the bootstrap scan collects exactly the failed
// records from the watch's snapshot, with their reasons, and ignores the others.
func TestScanFailed_SeedsFromSnapshot(t *testing.T) {
	t.Parallel()
	kv := &replayKV{memKV: newMemKV()}
	s := newTestStorage(t, kv, newMemKV())

	putRecord(t, kv, &Record{EntityID: "acme.ops.src.embed.record.fail-1", Status: StatusFailed, Reason: "connection_refused", ErrorMsg: "dial: refused"})
	putRecord(t, kv, &Record{EntityID: "acme.ops.src.embed.record.fail-2", Status: StatusFailed, Reason: "content_error"})
	putRecord(t, kv, &Record{EntityID: "acme.ops.src.embed.record.ok", Status: StatusGenerated, Vector: []float32{1, 2, 3}})
	putRecord(t, kv, &Record{EntityID: "acme.ops.src.embed.record.pending", Status: StatusPending, SourceText: "x"})

	got, err := s.ScanFailed(t.Context())
	if err != nil {
		t.Fatalf("ScanFailed: %v", err)
	}
	byID := map[string]string{}
	for _, e := range got {
		byID[e.EntityID] = e.Reason
	}
	if len(byID) != 2 {
		t.Fatalf("ScanFailed returned %d entries (%v), want exactly the 2 failed records", len(byID), byID)
	}
	if byID["acme.ops.src.embed.record.fail-1"] != "connection_refused" || byID["acme.ops.src.embed.record.fail-2"] != "content_error" {
		t.Fatalf("failed reasons not seeded correctly: %v", byID)
	}
}

// TestScanFailed_NormalizesReasonAndSeedsRevision: the scan collapses every stored
// reason outside the bounded enum to reasonUnknown and carries each record's source
// revision.
func TestScanFailed_NormalizesReasonAndSeedsRevision(t *testing.T) {
	t.Parallel()
	kv := &replayKV{memKV: newMemKV()}
	s := newTestStorage(t, kv, newMemKV())

	putRecord(t, kv, &Record{EntityID: "acme.ops.src.embed.record.valid", Status: StatusFailed, Reason: failReasonConnectionRefused, SourceRevision: 7})
	putRecord(t, kv, &Record{EntityID: "acme.ops.src.embed.record.empty", Status: StatusFailed, Reason: "", SourceRevision: 8})
	putRecord(t, kv, &Record{EntityID: "acme.ops.src.embed.record.arbitrary", Status: StatusFailed, Reason: "semembed-503-upstream-x9f2a", SourceRevision: 9})
	putRecord(t, kv, &Record{EntityID: "acme.ops.src.embed.record.future", Status: StatusFailed, Reason: "quota_exceeded", SourceRevision: 10})

	got, err := s.ScanFailed(t.Context())
	if err != nil {
		t.Fatalf("ScanFailed: %v", err)
	}
	byID := map[string]FailedEntry{}
	for _, e := range got {
		byID[e.EntityID] = e
	}
	want := map[string]FailedEntry{
		"acme.ops.src.embed.record.valid":     {EntityID: "acme.ops.src.embed.record.valid", Reason: failReasonConnectionRefused, SourceRevision: 7},
		"acme.ops.src.embed.record.empty":     {EntityID: "acme.ops.src.embed.record.empty", Reason: reasonUnknown, SourceRevision: 8},
		"acme.ops.src.embed.record.arbitrary": {EntityID: "acme.ops.src.embed.record.arbitrary", Reason: reasonUnknown, SourceRevision: 9},
		"acme.ops.src.embed.record.future":    {EntityID: "acme.ops.src.embed.record.future", Reason: reasonUnknown, SourceRevision: 10},
	}
	if len(byID) != len(want) {
		t.Fatalf("ScanFailed returned %v, want %v", byID, want)
	}
	for id, w := range want {
		if byID[id] != w {
			t.Errorf("entry %s = %+v, want %+v", id, byID[id], w)
		}
	}
}

// TestDerivedReasons_NotInPersistedFailureEnum: the derived-write reasons are in-memory
// only, so the persisted-failure enum collapses them to reasonUnknown (#625 at the pin).
func TestDerivedReasons_NotInPersistedFailureEnum(t *testing.T) {
	t.Parallel()
	for _, reason := range []string{ReasonDeleteFailed, ReasonPendingWriteFailed, ReasonEntityReadFailed} {
		if got := normalizeFailureReason(reason); got != reasonUnknown {
			t.Fatalf("normalizeFailureReason(%q) = %q, want %q", reason, got, reasonUnknown)
		}
	}
}

// TestScoredEntityLess_DeterministicTopKUnderPermutation: when more than k candidates
// share a similarity, the top k after sorting by ScoredEntityLess are the
// lexicographically smallest IDs, whatever the input order.
func TestScoredEntityLess_DeterministicTopKUnderPermutation(t *testing.T) {
	t.Parallel()
	const k = 8
	ids := []string{
		"o.p.s.d.t.e01", "o.p.s.d.t.e02", "o.p.s.d.t.e03", "o.p.s.d.t.e04",
		"o.p.s.d.t.e05", "o.p.s.d.t.e06", "o.p.s.d.t.e07", "o.p.s.d.t.e08",
		"o.p.s.d.t.e09", "o.p.s.d.t.e10", "o.p.s.d.t.e11", "o.p.s.d.t.e12",
		"o.p.s.d.t.e13", "o.p.s.d.t.e14", "o.p.s.d.t.e15", "o.p.s.d.t.e16",
		"o.p.s.d.t.e17", "o.p.s.d.t.e18", "o.p.s.d.t.e19", "o.p.s.d.t.e20",
	}
	base := make([]ScoredEntity, 0, len(ids))
	for _, id := range ids {
		base = append(base, ScoredEntity{EntityID: id, Similarity: 0.87})
	}
	wantIDs := append([]string(nil), ids...)
	sort.Strings(wantIDs)
	wantIDs = wantIDs[:k]

	rng := rand.New(rand.NewSource(1))
	for run := range 200 {
		perm := append([]ScoredEntity(nil), base...)
		rng.Shuffle(len(perm), func(i, j int) { perm[i], perm[j] = perm[j], perm[i] })
		sort.Slice(perm, func(i, j int) bool { return ScoredEntityLess(perm[i], perm[j]) })
		for i := range wantIDs {
			if perm[i].EntityID != wantIDs[i] {
				t.Fatalf("run %d: top-k index %d = %q, want %q", run, i, perm[i].EntityID, wantIDs[i])
			}
		}
	}
}

func vectorOf(r *Record) []float32 {
	if r == nil {
		return nil
	}
	return r.Vector
}

func sourceRevOf(r *Record) uint64 {
	if r == nil {
		return 0
	}
	return r.SourceRevision
}

// countingKV is a memKV that counts every bucket call Storage can make.
type countingKV struct {
	*memKV
	mu    sync.Mutex
	count int
}

func (k *countingKV) called() {
	k.mu.Lock()
	k.count++
	k.mu.Unlock()
}

func (k *countingKV) calls() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.count
}

func (k *countingKV) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	k.called()
	return k.memKV.Get(ctx, key)
}

func (k *countingKV) Put(ctx context.Context, key string, value []byte) (uint64, error) {
	k.called()
	return k.memKV.Put(ctx, key, value)
}

func (k *countingKV) Create(ctx context.Context, key string, value []byte, opts ...jetstream.KVCreateOpt) (uint64, error) {
	k.called()
	return k.memKV.Create(ctx, key, value, opts...)
}

func (k *countingKV) Update(ctx context.Context, key string, value []byte, revision uint64) (uint64, error) {
	k.called()
	return k.memKV.Update(ctx, key, value, revision)
}

func (k *countingKV) Delete(ctx context.Context, key string, opts ...jetstream.KVDeleteOpt) error {
	k.called()
	return k.memKV.Delete(ctx, key, opts...)
}

// TestStorageRefusesInvalidIdentityBeforeAnyBucketCall (#467): every entity-ID entry point
// refuses an ID that is not a literal six-part entity ID, and every dedup-key entry point
// a key that is not a literal KV key, with an invalid-input error and no bucket call. A
// valid input reaches the bucket, so the count can tell the two apart.
func TestStorageRefusesInvalidIdentityBeforeAnyBucketCall(t *testing.T) {
	t.Parallel()
	const valid = "acme.ops.robotics.gcs.drone.001"
	// Six well-formed tokens, 257 bytes in all: one past types.MaxEntityIDBytes.
	oversized := "acme.ops.robotics.gcs.drone." + strings.Repeat("a", 257-len("acme.ops.robotics.gcs.drone."))
	badIDs := []string{"", "not-an-entity", "acme.ops.robotics.gcs.drone", "acme.ops.robotics.gcs.drone.001.x",
		"acme.ops..gcs.drone.001", "acme.ops.robotics.gcs.drone.0*1", "acme.ops.robotics.gcs.drone._01", oversized}
	badKeys := []string{"", "a.*", "a.>", "a..b", "a b", "." + "x", strings.Repeat("k", natsclient.MaxKVLiteralKeyBytes+1)}

	entityCalls := map[string]func(ctx context.Context, s *Storage, id string) error{
		"SavePendingGuarded": func(ctx context.Context, s *Storage, id string) error {
			_, err := s.SavePendingGuarded(ctx, &Record{EntityID: id, SourceText: "body", SourceRevision: 1})
			return err
		},
		"SaveGenerated": func(ctx context.Context, s *Storage, id string) error {
			return s.SaveGenerated(ctx, id, []float32{1}, "m", 1, "h", 1)
		},
		"SaveFailed": func(ctx context.Context, s *Storage, id string) error {
			return s.SaveFailed(ctx, id, "boom", failReasonInternal, 1)
		},
		"GetEmbedding": func(ctx context.Context, s *Storage, id string) error {
			_, err := s.GetEmbedding(ctx, id)
			return err
		},
		"DeleteEmbedding": func(ctx context.Context, s *Storage, id string) error {
			return s.DeleteEmbedding(ctx, id)
		},
		"deleteAtRevision": func(ctx context.Context, s *Storage, id string) error {
			_, err := s.deleteAtRevision(ctx, id, 1)
			return err
		},
	}
	keyCalls := map[string]func(ctx context.Context, s *Storage, key string) error{
		"GetByContentHash": func(ctx context.Context, s *Storage, key string) error {
			_, err := s.GetByContentHash(ctx, key)
			return err
		},
		"SaveDedup": func(ctx context.Context, s *Storage, key string) error {
			return s.SaveDedup(ctx, key, []float32{1}, "m", 1)
		},
	}

	check := func(t *testing.T, name, input string, wantRefused bool, call func(ctx context.Context, s *Storage) error) {
		t.Helper()
		index, dedup := &countingKV{memKV: newMemKV()}, &countingKV{memKV: newMemKV()}
		err := call(t.Context(), newTestStorage(t, index, dedup))
		calls := index.calls() + dedup.calls()
		if wantRefused && (!errs.IsInvalid(err) || calls != 0) {
			t.Fatalf("%s(%q) = %v after %d bucket calls; want an invalid-input error before any bucket call", name, input, err, calls)
		}
		if !wantRefused && calls == 0 {
			t.Fatalf("%s(%q) made no bucket call; a valid input must reach the bucket (err %v)", name, input, err)
		}
	}
	for name, call := range entityCalls {
		for _, id := range badIDs {
			check(t, name, id, true, func(ctx context.Context, s *Storage) error { return call(ctx, s, id) })
		}
		check(t, name, valid, false, func(ctx context.Context, s *Storage) error { return call(ctx, s, valid) })
	}
	for name, call := range keyCalls {
		for _, key := range badKeys {
			check(t, name, key, true, func(ctx context.Context, s *Storage) error { return call(ctx, s, key) })
		}
		check(t, name, "dedup-key", false, func(ctx context.Context, s *Storage) error { return call(ctx, s, "dedup-key") })
	}
	if _, err := newTestStorage(t, newMemKV(), newMemKV()).SavePendingGuarded(t.Context(), nil); !errs.IsInvalid(err) {
		t.Fatalf("SavePendingGuarded(nil) = %v, want an invalid-input error", err)
	}
}
