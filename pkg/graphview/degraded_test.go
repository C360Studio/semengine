package graphview

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/nats-io/nats.go/jetstream"

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

// TestRunLostWatchFailsClosed (spec "The update channel closes"): when the
// update channel closes while Run's context is live, both subscriptions have
// closed their Deltas with an error matching ErrWatcherLost by the time
// OnWatcherLost fires, it fires once, Get reports the loss, and Run returns it.
// Neither subscriber reads before the loss, and one has a delivery blocked on
// its full channel, so a consumer that never reads cannot hold the report.
func TestRunLostWatchFailsClosed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := newFakeWatcher()
		var subs []*Subscription[string]
		type observation struct {
			cause  error
			open   []bool
			errors []error
		}
		var seen []observation
		view, err := New[string](&fakeSource{queue: []*fakeWatcher{w}}, decodeTest, WithHooks(Hooks{
			OnWatcherLost: func(cause error) {
				o := observation{cause: cause}
				for _, s := range subs {
					// Drain anything buffered; the channel must then be closed.
					open := true
				drain:
					for {
						select {
						case _, ok := <-s.Deltas():
							if !ok {
								open = false
								break drain
							}
						default:
							break drain
						}
					}
					o.open = append(o.open, open)
					o.errors = append(o.errors, s.Err())
				}
				seen = append(seen, o)
			},
		}), WithTickInterval(testTick))
		require.NoError(t, err)
		done := make(chan error, 1)
		go func() { done <- view.Run(t.Context()) }()
		w.updates <- nil
		require.NoError(t, view.WaitCaughtUp(t.Context()))

		_, subA, err := view.SnapshotAndSubscribe(t.Context())
		require.NoError(t, err)
		subB, err := view.Subscribe(t.Context())
		require.NoError(t, err)
		subs = []*Subscription[string]{subA, subB}
		// Two windows nobody reads: each delivery goroutine fills its channel
		// with the first and blocks sending the second.
		for rev := uint64(1); rev <= 2; rev++ {
			w.updates <- fakeEntry{key: "k", value: []byte("v"), rev: rev, op: jetstream.KeyValuePut}
			<-time.After(testTick)
		}
		synctest.Wait() // both delivery goroutines are blocked; subs is read only by the hook after this

		w.lose()
		runErr := <-done
		require.ErrorIs(t, runErr, ErrWatcherLost)
		require.Len(t, seen, 1, "OnWatcherLost fires once")
		require.Error(t, seen[0].cause)
		for i := range subs {
			require.False(t, seen[0].open[i], "subscription %d's Deltas still open when OnWatcherLost fired", i)
			require.ErrorIs(t, seen[0].errors[i], ErrWatcherLost, "subscription %d", i)
		}
		_, _, err = view.Get("k")
		require.ErrorIs(t, err, ErrWatcherLost)
	})
}
