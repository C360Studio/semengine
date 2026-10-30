package graphview

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"
)

// TestOnSubscribersFiresOnAttachAndDetach: the subscriber-count hook fires
// with the new count on every attach (both modes) and every detach path —
// Unsubscribe, subscription-context cancel — so a gauge wired to it tracks
// the live fan-out set.
func TestOnSubscribersFiresOnAttachAndDetach(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.endReplay()

		_, subA, err := h.view.SnapshotAndSubscribe(t.Context())
		require.NoError(t, err)
		require.Equal(t, []int{1}, h.takeSubscribers())

		subB, err := h.view.Subscribe(t.Context())
		require.NoError(t, err)
		require.Equal(t, []int{2}, h.takeSubscribers())

		ctxC, cancelC := context.WithCancel(t.Context())
		subC, err := h.view.Subscribe(ctxC)
		require.NoError(t, err)
		require.Equal(t, []int{3}, h.takeSubscribers())

		subA.Unsubscribe()
		require.Equal(t, []int{2}, h.takeSubscribers())

		// Context cancellation detaches like Unsubscribe and reports the count.
		cancelC()
		recvClosed(t, subC)
		require.Equal(t, []int{1}, h.takeSubscribers())

		// Idempotent detach does not re-fire.
		subA.Unsubscribe()
		require.Empty(t, h.takeSubscribers(), "idempotent Unsubscribe must not re-fire OnSubscribers")

		subB.Unsubscribe()
		require.Equal(t, []int{0}, h.takeSubscribers())
	})
}

// TestOnSubscribersReportsZeroOnStop: ending Run's context clears the fan-out
// set and reports zero — the terminal gauge value matches the terminal state.
func TestOnSubscribersReportsZeroOnStop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.endReplay()

		sub, err := h.view.Subscribe(t.Context())
		require.NoError(t, err)
		require.Equal(t, []int{1}, h.takeSubscribers())

		require.ErrorIs(t, h.stop(), context.Canceled)
		recvClosed(t, sub)
		require.ErrorIs(t, sub.Err(), ErrViewStopped)
		require.Equal(t, []int{0}, h.takeSubscribers())
	})
}

// TestOnSubscribersReportsZeroOnWatcherLoss: fail-closed terminates every
// subscription; the count hook reports zero alongside OnWatcherLost so the
// staleness gauge and the subscriber gauge cannot disagree.
func TestOnSubscribersReportsZeroOnWatcherLoss(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.endReplay()

		sub, err := h.view.Subscribe(t.Context())
		require.NoError(t, err)
		require.Equal(t, []int{1}, h.takeSubscribers())

		h.w.lose()
		require.ErrorIs(t, h.runResult(), ErrWatcherLost)
		require.Len(t, h.lostCalls(), 1)
		recvClosed(t, sub)
		require.ErrorIs(t, sub.Err(), ErrWatcherLost)
		require.Equal(t, []int{0}, h.takeSubscribers())
	})
}

// TestOnFanOutReportsPendingSize: a fresh subscriber's first window enqueues
// into an empty buffer — maxPending is exactly the changed-key cardinality
// and nothing was overwritten. Empty windows and zero-subscriber windows
// fire no OnFanOut.
func TestOnFanOutReportsPendingSize(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.endReplay()

		// Zero-subscriber window with changes: no OnFanOut.
		h.put("k0", "v", 1)
		h.tickNow()
		require.Empty(t, h.takeFanOuts(), "zero-subscriber window must not report fan-out stats")

		_, sub, err := h.view.SnapshotAndSubscribe(t.Context())
		require.NoError(t, err)
		require.Equal(t, []int{1}, h.takeSubscribers())

		// Empty window: no OnFanOut.
		h.tickNow()
		require.Empty(t, h.takeFanOuts(), "empty window must not report fan-out stats")

		// Three keys in one window into an empty (never-notified) buffer:
		// deterministic maxPending == 3, overwritten == 0.
		h.put("k1", "a", 2)
		h.put("k2", "b", 3)
		h.put("k3", "c", 4)
		h.tickNow()
		require.Equal(t, [][2]int{{0, 3}}, h.takeFanOuts(),
			"no undrained delta existed to overwrite; maxPending is the buffer size after enqueue")
		sub.Unsubscribe()
	})
}

// TestOnFanOutCountsOverwrites: an undrained subscriber's pending delta being
// replaced by a newer one for the same key is the at-most-once drop — the
// slow-client staleness signal. With four single-key windows and a consumer
// that never reads, at least one overwrite MUST be reported (buffer + in-flight
// capacity is three), and after draining the subscriber converges to the
// newest revision.
func TestOnFanOutCountsOverwrites(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.endReplay()

		_, sub, err := h.view.SnapshotAndSubscribe(t.Context())
		require.NoError(t, err)
		require.Equal(t, []int{1}, h.takeSubscribers())

		overwritten := 0
		for rev := uint64(1); rev <= 4; rev++ {
			h.put("K", "v"+string(rune('0'+rev)), rev)
			h.tickNow()
			// Every window here has a changed key and a subscriber, so OnFanOut
			// fires exactly once per window.
			f := h.takeFanOuts()
			require.Len(t, f, 1)
			overwritten += f[0][0]
			require.LessOrEqual(t, f[0][1], 1, "single-key windows can never buffer more than one pending delta")
		}
		require.GreaterOrEqual(t, overwritten, 1,
			"four undrained single-key windows must overwrite at least once (capacity is three)")

		// Drain: the subscriber converges to the newest revision, never regresses.
		tr := newTracker(t)
		for tr.revs["K"] < 4 {
			tr.apply(recvBatch(t, sub))
		}
		require.Equal(t, "v4", tr.values["K"])
		sub.Unsubscribe()
	})
}
