package graphview

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"
)

// TestBootstrapGatesAttach (G5): attach and point reads before the initial
// replay completes fail with the typed not-ready error — a partial bootstrap
// snapshot is never served unmarked. Readiness is caught-up, not started.
func TestBootstrapGatesAttach(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)

		// Mid-replay: entries applied, no end-of-replay marker yet.
		h.put("k1", "a", 1)

		_, _, err := h.view.SnapshotAndSubscribe(t.Context())
		require.ErrorIs(t, err, ErrNotReady, "attach mid-bootstrap must be gated")
		_, err = h.view.Subscribe(t.Context())
		require.ErrorIs(t, err, ErrNotReady, "delta-only attach is gated the same way")
		_, _, err = h.view.Get("k1")
		require.ErrorIs(t, err, ErrNotReady, "point reads honor the readiness gate")
		_, err = h.view.List("", 0)
		require.ErrorIs(t, err, ErrNotReady)

		// WaitCaughtUp observes the gate (bounded by the caller's context).
		cancelled, cancel := context.WithCancel(t.Context())
		cancel()
		err = h.view.WaitCaughtUp(cancelled)
		require.ErrorIs(t, err, context.Canceled)

		// The watermark advances during replay even though the view is gated.
		rev, _ := h.view.Applied()
		require.Equal(t, uint64(1), rev)

		// End of replay: the gate lifts and the snapshot is complete.
		h.endReplay()
		snap, sub, err := h.view.SnapshotAndSubscribe(t.Context())
		require.NoError(t, err)
		require.Len(t, snap.Entries, 1)
		require.Equal(t, uint64(1), snap.Entries["k1"].Revision)
		sub.Unsubscribe()
	})
}

// TestWatcherLossFailsClosed (G5): when the watch's update channel closes
// while Run's context is live, every attached subscriber receives the explicit
// staleness signal (terminal close with Err() = wrapped ErrWatcherLost), point
// reads refuse to serve the frozen projection as live, new attaches observe
// not-ready semantics that also identify the loss, OnWatcherLost fires once,
// and Run returns the loss.
func TestWatcherLossFailsClosed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.put("k1", "a", 1)
		h.endReplay()

		_, subX, err := h.view.SnapshotAndSubscribe(t.Context())
		require.NoError(t, err)
		subY, err := h.view.Subscribe(t.Context())
		require.NoError(t, err)

		// Watcher loss: the updates channel closes while Run's context is live.
		h.w.lose()
		runErr := h.runResult()
		require.ErrorIs(t, runErr, ErrWatcherLost)
		require.NotErrorIs(t, runErr, context.Canceled)
		lost := h.lostCalls()
		require.Len(t, lost, 1, "OnWatcherLost fires once per loss")
		require.Error(t, lost[0])

		// Every subscriber observes the explicit terminal staleness signal.
		recvClosed(t, subX)
		require.ErrorIs(t, subX.Err(), ErrWatcherLost)
		recvClosed(t, subY)
		require.ErrorIs(t, subY.Err(), ErrWatcherLost)

		// The frozen projection is not served as live.
		_, _, err = h.view.Get("k1")
		require.ErrorIs(t, err, ErrWatcherLost)
		require.ErrorIs(t, err, ErrNotReady, "fail-closed is a not-ready state")

		// New attaches observe not-ready semantics carrying the loss.
		_, _, err = h.view.SnapshotAndSubscribe(t.Context())
		require.ErrorIs(t, err, ErrNotReady)
		require.ErrorIs(t, err, ErrWatcherLost)

		// WaitCaughtUp reports the loss instead of blocking forever.
		err = h.view.WaitCaughtUp(t.Context())
		require.ErrorIs(t, err, ErrWatcherLost)
	})
}
