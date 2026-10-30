package graphview

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"
)

// TestNewValidatesArguments: the view is explicitly constructed; nil
// injections are refused up front, not discovered at Run.
func TestNewValidatesArguments(t *testing.T) {
	_, err := New[string](nil, decodeTest)
	require.Error(t, err)
	_, err = New[string](&fakeSource{}, nil)
	require.Error(t, err)
}

// TestRunRejectsNilContext: Run is an exported error-returning boundary, so a
// nil context is reported instead of panicking in context.WithCancel.
func TestRunRejectsNilContext(t *testing.T) {
	view, err := New[string](&fakeSource{}, decodeTest)
	require.NoError(t, err)
	require.Error(t, view.Run(nil))
}

// TestContextBoundariesRejectNil: subscription and wait contexts are
// call-scoped lifecycle controls, so nil is rejected at the exported
// error-returning boundary rather than reaching a delivery goroutine.
func TestContextBoundariesRejectNil(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.endReplay()

		require.Error(t, h.view.WaitCaughtUp(nil))
		_, _, err := h.view.SnapshotAndSubscribe(nil)
		require.Error(t, err)
		_, err = h.view.Subscribe(nil)
		require.Error(t, err)

		h.view.mu.Lock()
		require.Empty(t, h.view.subs, "invalid contexts must not attach subscribers")
		h.view.mu.Unlock()
	})
}

// TestRunLifecycleGuards: a second Run on a running view is refused with
// ErrAlreadyStarted and leaves the first running; once Run has returned, Run,
// reads and WaitCaughtUp all return ErrViewStopped. A replacement is a new view.
func TestRunLifecycleGuards(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.endReplay()
		require.ErrorIs(t, h.view.Run(t.Context()), ErrAlreadyStarted)
		h.put("k", "v", 1)
		val, _, err := h.view.Get("k")
		require.NoError(t, err, "the first Run keeps running after the refused second")
		require.Equal(t, "v", val)

		require.ErrorIs(t, h.stop(), context.Canceled)
		require.ErrorIs(t, h.view.Run(t.Context()), ErrViewStopped)
		_, _, err = h.view.Get("k")
		require.ErrorIs(t, err, ErrViewStopped)
		require.ErrorIs(t, h.view.WaitCaughtUp(t.Context()), ErrViewStopped)
		require.Equal(t, 1, h.src.opened, "neither refused Run opened a watch")
	})
}

// TestUnsubscribeReleasesState: detach removes the subscriber from the
// fan-out set and releases its buffered deltas; the channel closes cleanly
// (Err() == nil) and subsequent ticks retain nothing for it.
func TestUnsubscribeReleasesState(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.endReplay()

		_, sub, err := h.view.SnapshotAndSubscribe(t.Context())
		require.NoError(t, err)

		// Leave an undrained pending delta behind, then detach.
		h.put("K", "v1", 1)
		h.tickNow()
		require.Len(t, h.takeFanOuts(), 1, "the window reached the subscriber")
		sub.Unsubscribe()
		recvClosed(t, sub)
		require.NoError(t, sub.Err(), "clean detach reports no terminal error")

		sub.mu.Lock()
		require.Nil(t, sub.pending, "detach must release the subscriber's buffered state")
		sub.mu.Unlock()

		h.view.mu.Lock()
		require.Empty(t, h.view.subs, "detached subscriber must leave the fan-out set")
		h.view.mu.Unlock()

		// Later windows fan out to zero subscribers without retaining state.
		h.put("K", "v2", 2)
		h.tickNow()
		require.Empty(t, h.takeFanOuts(), "a window with no subscriber fans out to no one")
		h.view.mu.Lock()
		require.Empty(t, h.view.pending, "detached window is dropped, not retained")
		h.view.mu.Unlock()

		// Unsubscribe is idempotent.
		sub.Unsubscribe()
	})
}

// TestSubscriberContextCancelDetaches: cancelling the subscription context
// detaches exactly like Unsubscribe, with the context error as the reason.
func TestSubscriberContextCancelDetaches(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.endReplay()

		ctx, cancel := context.WithCancel(t.Context())
		sub, err := h.view.Subscribe(ctx)
		require.NoError(t, err)
		cancel()
		recvClosed(t, sub)
		require.ErrorIs(t, sub.Err(), context.Canceled)

		h.view.mu.Lock()
		require.Empty(t, h.view.subs)
		h.view.mu.Unlock()
	})
}

// TestStopDeliversTerminalClose: ending Run's context is a stop, not a loss,
// even though it also closes the watch's update channel. Every subscription
// gets an explicit terminal close with ErrViewStopped, including one that is
// not draining; OnWatcherLost does not fire; the watch is stopped; and Run
// returns the context's error.
func TestStopDeliversTerminalClose(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.endReplay()

		_, subA, err := h.view.SnapshotAndSubscribe(t.Context())
		require.NoError(t, err)
		subB, err := h.view.Subscribe(t.Context())
		require.NoError(t, err)

		// subA has an undrained window in flight when the stop lands.
		h.put("K", "v1", 1)
		h.tickNow()

		runErr := h.stop()
		require.ErrorIs(t, runErr, context.Canceled)
		require.NotErrorIs(t, runErr, ErrWatcherLost, "a context end is a stop, not a lost watch")
		recvClosed(t, subA)
		require.ErrorIs(t, subA.Err(), ErrViewStopped)
		require.NotErrorIs(t, subA.Err(), ErrWatcherLost)
		recvClosed(t, subB)
		require.ErrorIs(t, subB.Err(), ErrViewStopped)
		require.Empty(t, h.lostCalls(), "OnWatcherLost must not fire on a stop")
		require.Equal(t, int32(1), h.w.stops.Load(), "the watch is stopped once")
	})
}

// TestGetAndListSurface: coherent point reads and deterministic bounded
// listing over the same projection the delta stream serves.
func TestGetAndListSurface(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.put("a.1", "v1", 1)
		h.put("a.2", "v2", 2)
		h.put("b.1", "v3", 3)
		h.endReplay()

		val, rev, err := h.view.Get("a.2")
		require.NoError(t, err)
		require.Equal(t, "v2", val)
		require.Equal(t, uint64(2), rev)

		_, _, err = h.view.Get("missing")
		require.ErrorIs(t, err, ErrKeyNotFound)

		all, err := h.view.List("", 0)
		require.NoError(t, err)
		require.Len(t, all, 3)
		require.Equal(t, []string{"a.1", "a.2", "b.1"}, []string{all[0].Key, all[1].Key, all[2].Key},
			"complete result set is sorted deterministically")

		prefixed, err := h.view.List("a.", 0)
		require.NoError(t, err)
		require.Len(t, prefixed, 2)

		limited, err := h.view.List("", 2)
		require.NoError(t, err)
		require.Len(t, limited, 2)
		require.Equal(t, "a.1", limited[0].Key, "sort happens before the limit")
	})
}

// TestSkippedKeyMapsToAbsence: keep=false decodes are not part of the view —
// a skipped write on a previously-kept key converges subscribers to absence
// via a tombstone at that revision; a skipped write on an unknown key only
// advances the watermark.
func TestSkippedKeyMapsToAbsence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.put("K", "v1", 1)
		h.endReplay()

		snap, sub, err := h.view.SnapshotAndSubscribe(t.Context())
		require.NoError(t, err)
		tr := newTracker(t)
		tr.loadSnapshot(snap)

		// Unknown key skipped: watermark advances, nothing delivered.
		h.put("meta", "SKIP", 2)
		rev, _ := h.view.Applied()
		require.Equal(t, uint64(2), rev)
		h.tickNow()
		requireNoBatch(t, sub)

		// Previously-kept key turns not-ready: subscribers converge to absence.
		h.put("K", "SKIP", 3)
		h.tickNow()
		batch := recvBatch(t, sub)
		tr.apply(batch)
		require.Len(t, batch, 1)
		require.Equal(t, DeltaDelete, batch[0].Op)
		require.Equal(t, uint64(3), batch[0].Revision)
		_, _, err = h.view.Get("K")
		require.ErrorIs(t, err, ErrKeyNotFound)
		sub.Unsubscribe()
	})
}

// TestDeltaOnlySubscribeIsTriggerShaped: a delta-only attach carries no
// snapshot and receives exactly the changes after its attach sequence.
func TestDeltaOnlySubscribeIsTriggerShaped(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.put("pre", "v", 1)
		h.endReplay()

		sub, err := h.view.Subscribe(t.Context())
		require.NoError(t, err)

		// A pre-attach change is not replayed to a delta-only subscriber.
		h.put("post", "v", 2)
		h.tickNow()
		batch := recvBatch(t, sub)
		require.Len(t, batch, 1)
		require.Equal(t, "post", batch[0].Key)
		sub.Unsubscribe()
	})
}
