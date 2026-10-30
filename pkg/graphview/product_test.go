package graphview_test

// Product-shaped tests of the kept symbols no repository code reads yet (design D3): each
// calls the package the way a product built on SemEngine would, through its exported surface
// only. Their contract lines are ADR-081 Decision 3 (Subscribe), Decision 6 (List,
// KeyedEntry) and ADR-085 Decision 5 (Applied, Snapshot.Revision, Snapshot.AppliedAt); the
// sentinels ErrNotReady, ErrViewStopped, ErrKeyNotFound and ErrAlreadyStarted are matched by
// name, as a product would.

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/pkg/graphview"
)

const tick = 100 * time.Millisecond

// write is one KV entry a product's bucket would deliver.
type write struct {
	key     string
	value   string
	rev     uint64
	created time.Time
}

func (w write) Bucket() string                  { return "DEVICES" }
func (w write) Key() string                     { return w.key }
func (w write) Value() []byte                   { return []byte(w.value) }
func (w write) Revision() uint64                { return w.rev }
func (w write) Created() time.Time              { return w.created }
func (w write) Delta() uint64                   { return 0 }
func (w write) Operation() jetstream.KeyValueOp { return jetstream.KeyValuePut }

// bucket stands in for a jetstream.KeyValue: one watch whose update channel the test feeds,
// closed when the watch's context ends, as nats.go's is.
type bucket struct{ updates chan jetstream.KeyValueEntry }

func (b *bucket) WatchAll(ctx context.Context, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	context.AfterFunc(ctx, func() { close(b.updates) })
	return b, nil
}

func (b *bucket) Updates() <-chan jetstream.KeyValueEntry { return b.updates }
func (b *bucket) Stop() error                             { return nil }

// deliver hands one write to the view and waits until the view has applied it.
func (b *bucket) deliver(w jetstream.KeyValueEntry) {
	b.updates <- w
	synctest.Wait() // Run has applied the write and blocked again
}

// decodeName keeps every value except "SKIP", as a product's validating decode would keep its
// own records and leave the rest out.
func decodeName(_ string, value []byte, _ graphview.EntryMeta) (string, bool, error) {
	if string(value) == "SKIP" {
		return "", false, nil
	}
	return string(value), true, nil
}

// runView starts a view over a fresh bucket on its own goroutine. stop ends Run's context and
// returns Run's error.
func runView(t *testing.T) (view *graphview.View[string], b *bucket, stop func() error) {
	t.Helper()
	b = &bucket{updates: make(chan jetstream.KeyValueEntry)}
	view, err := graphview.New[string](b, decodeName, graphview.WithTickInterval(tick))
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- view.Run(ctx) }()
	synctest.Wait() // Run has opened the watch and waits for its first write
	return view, b, func() error { cancel(); return <-done }
}

// nextBatch lets one tick pass and returns the batch the subscription then holds.
func nextBatch(t *testing.T, sub *graphview.Subscription[string]) []graphview.Delta[string] {
	t.Helper()
	<-time.After(tick)
	synctest.Wait() // the view has fanned the window out and the delivery goroutine has sent it
	select {
	case batch, ok := <-sub.Deltas():
		require.True(t, ok, "Deltas closed: %v", sub.Err())
		return batch
	default:
		t.Fatal("no batch after a tick")
		return nil
	}
}

// TestProductSubscribeTriggersOnChange: a trigger-shaped consumer attaches with Subscribe
// (ADR-081 D3) once the view has caught up, and receives the changes after its attach, not
// the ones before it. Before the replay ends Subscribe returns ErrNotReady; a second Run
// returns ErrAlreadyStarted; after the stop the subscription closes with ErrViewStopped and
// Subscribe returns it.
func TestProductSubscribeTriggersOnChange(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		view, b, stop := runView(t)
		_, err := view.Subscribe(t.Context())
		require.ErrorIs(t, err, graphview.ErrNotReady)
		require.ErrorIs(t, view.Run(t.Context()), graphview.ErrAlreadyStarted)

		b.deliver(nil) // end of the replay
		require.NoError(t, view.WaitCaughtUp(t.Context()))
		b.deliver(write{key: "device.1", value: "before", rev: 1})
		trigger, err := view.Subscribe(t.Context())
		require.NoError(t, err)
		b.deliver(write{key: "device.2", value: "after", rev: 2})

		batch := nextBatch(t, trigger)
		require.Len(t, batch, 1, "only the change after the attach: %v", batch)
		require.Equal(t, "device.2", batch[0].Key)
		require.Equal(t, graphview.DeltaUpsert, batch[0].Op)

		require.ErrorIs(t, stop(), context.Canceled)
		_, open := <-trigger.Deltas()
		require.False(t, open)
		require.ErrorIs(t, trigger.Err(), graphview.ErrViewStopped)
		_, err = view.Subscribe(t.Context())
		require.ErrorIs(t, err, graphview.ErrViewStopped)
	})
}

// TestProductListPagesByKey: List (ADR-081 D6) returns the keys under a prefix sorted by key,
// the whole match set sorted before the limit, as KeyedEntry values with their revisions; Get
// of an absent key returns ErrKeyNotFound.
func TestProductListPagesByKey(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		view, b, stop := runView(t)
		// Written in an order that is neither sorted nor reversed.
		keys := []string{"dev.g", "dev.c", "dev.j", "dev.a", "dev.e", "dev.h", "dev.b", "dev.i", "dev.d", "dev.f"}
		for i, k := range keys {
			b.deliver(write{key: k, value: "v" + k, rev: uint64(i + 1)})
		}
		b.deliver(write{key: "other.x", value: "x", rev: 11})
		b.deliver(nil)

		page, err := view.List("dev.", 3)
		require.NoError(t, err)
		require.Equal(t, []graphview.KeyedEntry[string]{
			{Key: "dev.a", Value: "vdev.a", Revision: 4},
			{Key: "dev.b", Value: "vdev.b", Revision: 7},
			{Key: "dev.c", Value: "vdev.c", Revision: 2},
		}, page)
		all, err := view.List("", 0)
		require.NoError(t, err)
		require.Len(t, all, 11)

		_, _, err = view.Get("dev.z")
		require.ErrorIs(t, err, graphview.ErrKeyNotFound)
		require.ErrorIs(t, stop(), context.Canceled)
	})
}

// TestProductCurrencyReport: Applied (ADR-085 D5) reports the newest applied revision and its
// server write time, zero before anything is applied; a write the decode leaves out still
// advances it; a snapshot carries the same pair in Revision and AppliedAt.
func TestProductCurrencyReport(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		view, b, stop := runView(t)
		rev, at := view.Applied()
		require.Zero(t, rev)
		require.True(t, at.IsZero(), "no write applied, so no currency to report")

		written := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
		b.deliver(write{key: "device.1", value: "on", rev: 1, created: written})
		b.deliver(write{key: "device.2", value: "SKIP", rev: 2, created: written.Add(time.Second)})
		b.deliver(nil)

		rev, at = view.Applied()
		require.Equal(t, uint64(2), rev)
		require.Equal(t, written.Add(time.Second), at)

		snap, sub, err := view.SnapshotAndSubscribe(t.Context())
		require.NoError(t, err)
		require.Equal(t, rev, snap.Revision)
		require.Equal(t, at, snap.AppliedAt)
		sub.Unsubscribe()
		require.ErrorIs(t, stop(), context.Canceled)
	})
}
