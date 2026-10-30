package graphview

import (
	"fmt"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

// TestRacingAttachNeverObservesPreAppliedState locks the attach seam (G1):
// once the view has applied revision R for K, any subsequent snapshot holds
// K at >= R — {snapshot + register} is atomic with apply under the
// projection mutex, so an attach racing an apply serializes on one side of
// it and never observes a pre-R value past an applied R.
func TestRacingAttachNeverObservesPreAppliedState(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.endReplay()

		// Scripted: the apply of K@5 is complete before attach.
		h.put("K", "v5", 5)
		snap, sub, err := h.view.SnapshotAndSubscribe(t.Context())
		require.NoError(t, err)
		require.Equal(t, uint64(5), snap.Entries["K"].Revision,
			"snapshot after an applied revision must hold that revision, never pre-R")
		require.Equal(t, "v5", snap.Entries["K"].Value)
		sub.Unsubscribe()

		// Concurrent: attaches race a stream of applies. The invariant is read
		// on the attacher side: the applied revision sampled BEFORE the attach is
		// a floor for the snapshot's K revision (the view had applied at least
		// that much when the snapshot was captured).
		done := make(chan struct{})
		go func() {
			defer close(done)
			for rev := uint64(6); rev <= 105; rev++ {
				h.w.updates <- fakeEntry{key: "K", value: []byte("v"), rev: rev, op: jetstream.KeyValuePut}
			}
		}()
		for range 50 {
			floor, _ := h.view.Applied()
			snap, sub, err := h.view.SnapshotAndSubscribe(t.Context())
			require.NoError(t, err)
			require.GreaterOrEqual(t, snap.Entries["K"].Revision, floor,
				"snapshot K revision fell below the applied watermark read before attach")
			sub.Unsubscribe()
		}
		<-done
	})
}

// TestTickSeamNoStaleDeliveryToLateAttacher is THE tick-seam regression (the
// SemStreams PR #583 shape one seam over, spec "No stale delivery across the
// tick seam"): a coalesced batch holding K@R5 is ticked out, the view then
// applies K@R6, subscriber X attaches with a snapshot at K@R6 — X must NEVER
// receive the R5 value. Under the critical-section fan-out this is
// structural: the batch is enqueued to the subscribers registered at
// detach time inside the same lock hold, so it cannot exist un-enqueued when
// X attaches. The marker write proves the negative deterministically: X's
// FIRST batch drains everything pending for X, so if R5 (or the R6 re-play)
// were buffered for X it would ride in front of the marker.
func TestTickSeamNoStaleDeliveryToLateAttacher(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.endReplay()

		// B attaches before the window so the R5 batch has a real consumer.
		snapB, subB, err := h.view.SnapshotAndSubscribe(t.Context())
		require.NoError(t, err)
		trB := newTracker(t)
		trB.loadSnapshot(snapB)

		// Window 1: K@R5 is detached and enqueued (to B only — X not attached).
		h.put("K", "v5", 5)
		h.tickNow()
		b1 := recvBatch(t, subB)
		trB.apply(b1)
		require.Len(t, b1, 1)
		require.Equal(t, uint64(5), b1[0].Revision)

		// The view applies K@R6, THEN X attaches: snapshot already at R6.
		h.put("K", "v6", 6)
		snapX, subX, err := h.view.SnapshotAndSubscribe(t.Context())
		require.NoError(t, err)
		trX := newTracker(t)
		trX.loadSnapshot(snapX)
		require.Equal(t, uint64(6), snapX.Entries["K"].Revision)

		// Marker write + window 2. For X the pending K@R6 op carries an apply
		// sequence <= X's attach sequence, so it is filtered (already in X's
		// snapshot); the marker is X's ONLY delivery. Any stale K delivery
		// would have to ride this same first batch — its absence is proof.
		h.put("M", "marker", 7)
		h.tickNow()

		bX := recvBatch(t, subX)
		trX.apply(bX)
		require.Len(t, bX, 1, "X's first batch must carry ONLY the marker, got %v", bX)
		require.Equal(t, "M", bX[0].Key)
		require.Equal(t, uint64(7), bX[0].Revision)

		// B (attached pre-R5) receives K@R6 + marker: newer-only, in order.
		bB := recvBatch(t, subB)
		trB.apply(bB)
		require.Len(t, bB, 2)
		require.Equal(t, "K", bB[0].Key)
		require.Equal(t, uint64(6), bB[0].Revision, "B must see R6, never a replay of R5")
		require.Equal(t, "M", bB[1].Key)

		subB.Unsubscribe()
		subX.Unsubscribe()
	})
}

// TestSnapshotDeltaSeamScripted drives the G1 no-gap/no-dup contract with a
// scripted interleaving: keys changed at seq <= S are ONLY in the snapshot,
// keys changed after S are ONLY deltas, and a key spanning the seam (k2
// updated after S) arrives as the newer delta without resurrecting the
// snapshot revision.
func TestSnapshotDeltaSeamScripted(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.put("k1", "a", 1)
		h.put("k2", "b", 2)
		h.endReplay()
		h.put("k3", "c", 3)

		snap, sub, err := h.view.SnapshotAndSubscribe(t.Context())
		require.NoError(t, err)
		tr := newTracker(t)
		tr.loadSnapshot(snap)
		require.Len(t, snap.Entries, 3, "snapshot must hold every key applied at <= S")
		require.Equal(t, uint64(3), snap.Revision)

		// Changes after S: a new key and an update to a snapshotted key.
		h.put("k4", "d", 4)
		h.put("k2", "b2", 5)
		h.tickNow()

		batch := recvBatch(t, sub)
		tr.apply(batch)
		require.Len(t, batch, 2, "exactly the two post-S changes — no duplicate of k1/k3, no gap")
		require.Equal(t, "k4", batch[0].Key)
		require.Equal(t, uint64(4), batch[0].Revision)
		require.Equal(t, "k2", batch[1].Key)
		require.Equal(t, uint64(5), batch[1].Revision)
		require.Equal(t, map[string]string{"k1": "a", "k2": "b2", "k3": "c", "k4": "d"}, tr.values)
		sub.Unsubscribe()
	})
}

// TestSnapshotDeltaSeamUnderConcurrentWrites drives the attach seam under
// -race: writes stream while subscribers attach and Run's ticker fires.
// Every subscriber materializes snapshot+deltas through the tracker (per-key
// high-water regression = failure) and must converge to the state the writes
// define — no gap, no dup, no inversion, regardless of interleaving. The
// expected state is computed from the writes, never read back from the view.
func TestSnapshotDeltaSeamUnderConcurrentWrites(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.endReplay()

		const (
			writes  = 400
			keys    = 13
			readers = 8
			marker  = "ZZ-marker"
		)
		keyOf := func(i int) string { return string(rune('a' + i%keys)) }
		valueOf := func(i int) string { return string([]byte{byte('A' + i%26)}) }
		want := map[string]string{marker: "done"}
		for i := range writes {
			want[keyOf(i)] = valueOf(i)
		}

		// Writer: raw sends, each returning once Run has received the entry.
		writerDone := make(chan struct{})
		go func() {
			defer close(writerDone)
			for i := range writes {
				h.w.updates <- fakeEntry{key: keyOf(i), value: []byte(valueOf(i)), rev: uint64(i + 1), op: jetstream.KeyValuePut}
			}
		}()
		// Readers: attach mid-stream, materialize, hold until the marker. Each
		// reads with a blocking receive; Run's ticker delivers as the bubble's
		// clock advances. A reader records its failure in its own slot and the
		// test goroutine asserts on it after wg.Wait (FailNow is valid only on
		// the test goroutine).
		var wg sync.WaitGroup
		results := make([]*tracker, readers)
		failures := make([]error, readers)
		for r := range readers {
			wg.Go(func() {
				snap, sub, err := h.view.SnapshotAndSubscribe(t.Context())
				if err != nil {
					failures[r] = fmt.Errorf("attach: %w", err)
					return
				}
				defer sub.Unsubscribe()
				tr := newTracker(t)
				tr.loadSnapshot(snap)
				for tr.revs[marker] == 0 {
					batch, ok := <-sub.Deltas()
					if !ok {
						failures[r] = fmt.Errorf("deltas closed before the marker (Err: %v)", sub.Err())
						return
					}
					if err := tr.applyBatch(batch); err != nil {
						failures[r] = err
						return
					}
				}
				results[r] = tr
			})
		}

		<-writerDone
		h.w.updates <- fakeEntry{key: marker, value: []byte("done"), rev: writes + 1, op: jetstream.KeyValuePut}
		wg.Wait() // every reader has seen the marker, or recorded its failure
		for r := range readers {
			require.NoError(t, failures[r], "reader %d", r)
			require.Equal(t, want, results[r].values, "reader %d did not converge to the written state", r)
		}
	})
}
