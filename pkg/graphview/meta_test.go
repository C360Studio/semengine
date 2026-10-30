package graphview

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

// TestDecodeReceivesEntryMeta: the validating decode is handed the entry's
// server write time, so decoded records can embed the KV write time instead
// of guessing from decode-time clocks (the operator-surface timestamp parity
// requirement).
func TestDecodeReceivesEntryMeta(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		created := time.Date(2026, 7, 1, 12, 30, 0, 0, time.UTC)
		var metas []EntryMeta

		w := newFakeWatcher()
		src := &fakeSource{queue: []*fakeWatcher{w}}
		decode := func(_ string, value []byte, meta EntryMeta) (string, bool, error) {
			metas = append(metas, meta)
			return string(value), true, nil
		}
		view, err := New[string](src, decode)
		require.NoError(t, err)
		go func() { _ = view.Run(t.Context()) }()

		w.updates <- fakeEntry{key: "K", value: []byte("v1"), rev: 7, op: jetstream.KeyValuePut, created: created}
		synctest.Wait() // Run has decoded the entry
		require.Len(t, metas, 1)
		require.True(t, metas[0].Created.Equal(created), "decode must receive entry.Created(), got %v", metas[0].Created)
	})
}

// TestDeltaCarriesCreated: every delta op carries the KV entry's Created
// timestamp through the coalesced lane — tombstones have no decoded T, so
// Delta.Created is their only channel for the write time (a delete-marker
// consumer must be able to reproduce entry.Created() exactly, per the old
// per-client watcher wire). LWW coalescing keeps the newest op's Created.
func TestDeltaCarriesCreated(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.endReplay()

		_, sub, err := h.view.SnapshotAndSubscribe(t.Context())
		require.NoError(t, err)

		tPut := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
		tDel := time.Date(2026, 7, 1, 9, 5, 0, 0, time.UTC)
		tSkipPut := time.Date(2026, 7, 1, 9, 10, 0, 0, time.UTC)
		tSkip := time.Date(2026, 7, 1, 9, 15, 0, 0, time.UTC)

		// Upsert carries Created.
		h.send(fakeEntry{key: "K", value: []byte("v1"), rev: 1, op: jetstream.KeyValuePut, created: tPut})
		h.tickNow()
		batch := recvBatch(t, sub)
		require.Len(t, batch, 1)
		require.Equal(t, DeltaUpsert, batch[0].Op)
		require.True(t, batch[0].Created.Equal(tPut), "upsert delta must carry entry.Created()")

		// Delete tombstone carries the delete marker's Created.
		h.send(fakeEntry{key: "K", rev: 2, op: jetstream.KeyValueDelete, created: tDel})
		h.tickNow()
		batch = recvBatch(t, sub)
		require.Len(t, batch, 1)
		require.Equal(t, DeltaDelete, batch[0].Op)
		require.True(t, batch[0].Created.Equal(tDel), "tombstone delta must carry the delete marker's Created()")

		// keep=false on a previously-kept key synthesizes a tombstone carrying
		// the skipping write's Created.
		h.send(fakeEntry{key: "S", value: []byte("v1"), rev: 3, op: jetstream.KeyValuePut, created: tSkipPut})
		h.tickNow()
		batch = recvBatch(t, sub)
		require.Len(t, batch, 1)
		require.Equal(t, DeltaUpsert, batch[0].Op)

		h.send(fakeEntry{key: "S", value: []byte("SKIP"), rev: 4, op: jetstream.KeyValuePut, created: tSkip})
		h.tickNow()
		batch = recvBatch(t, sub)
		require.Len(t, batch, 1)
		require.Equal(t, DeltaDelete, batch[0].Op)
		require.True(t, batch[0].Created.Equal(tSkip), "synthesized tombstone must carry the skipping write's Created()")

		// LWW within one window keeps the newest op's Created alongside its
		// revision (G4: Created never regresses past a newer op).
		tOld := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
		tNew := time.Date(2026, 7, 1, 10, 1, 0, 0, time.UTC)
		h.send(fakeEntry{key: "L", value: []byte("v1"), rev: 5, op: jetstream.KeyValuePut, created: tOld})
		h.send(fakeEntry{key: "L", value: []byte("v2"), rev: 6, op: jetstream.KeyValuePut, created: tNew})
		h.tickNow()
		batch = recvBatch(t, sub)
		require.Len(t, batch, 1)
		require.Equal(t, uint64(6), batch[0].Revision)
		require.True(t, batch[0].Created.Equal(tNew), "coalescing must keep the newest op's Created")

		sub.Unsubscribe()
	})
}
