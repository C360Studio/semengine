package graphview

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

// TestRunLeavesNothingBehind (spec "Started, used and stopped"): a snapshot
// subscription and a delta-only subscription receive a change, Run's context
// ends, Run returns context.Canceled with both closed, and the bubble ends with
// no goroutine left. The subscriptions' contexts never end, so only Run's stop
// can end their delivery goroutines; synctest.Test fails with a deadlock when a
// goroutine is still blocked once the test function has returned.
func TestRunLeavesNothingBehind(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.endReplay()

		never := context.Background() // no end, so a leaked delivery goroutine stays blocked
		_, snapSub, err := h.view.SnapshotAndSubscribe(never)
		require.NoError(t, err)
		deltaSub, err := h.view.Subscribe(never)
		require.NoError(t, err)

		h.put("k", "v", 1)
		h.tickNow()
		for _, sub := range []*Subscription[string]{snapSub, deltaSub} {
			batch := recvBatch(t, sub)
			require.Len(t, batch, 1)
			require.Equal(t, "v", batch[0].Value)
		}

		require.ErrorIs(t, h.stop(), context.Canceled)
		for _, sub := range []*Subscription[string]{snapSub, deltaSub} {
			require.Empty(t, recvClosed(t, sub))
			require.ErrorIs(t, sub.Err(), ErrViewStopped)
		}
		require.Equal(t, int32(1), h.w.stops.Load(), "the watch is stopped once")
	})
}

// TestRunContextEndIsAStop (spec "Context end and channel close both ready"):
// an OnApply hook holds Run while the test ends Run's context, which closes the
// watch's update channel; when the hook returns, Run's next read finds both the
// context's end and the closed channel ready, and picks either. On 200 fresh
// views each must report a stop: OnWatcherLost never fires, a subscription with
// a live context closes with ErrViewStopped, and Run returns context.Canceled.
// A close counted as a loss whatever the context (the pin's shape, view.go:200-202)
// fails here with near certainty: each view picks the closed channel half the time.
func TestRunContextEndIsAStop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		for i := range 200 {
			w := newFakeWatcher()
			held, release := make(chan struct{}), make(chan struct{})
			var lost atomic.Int32
			view, err := New[string](&fakeSource{queue: []*fakeWatcher{w}}, decodeTest, WithHooks(Hooks{
				OnApply: func(key string, _ uint64) {
					if key == "hold" {
						close(held)
						<-release
					}
				},
				OnWatcherLost: func(error) { lost.Add(1) },
			}))
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(t.Context())
			done := make(chan error, 1)
			go func() { done <- view.Run(ctx) }()
			w.updates <- nil
			require.NoError(t, view.WaitCaughtUp(t.Context()))
			sub, err := view.Subscribe(t.Context())
			require.NoError(t, err)

			w.updates <- fakeEntry{key: "hold", value: []byte("v"), rev: 1, op: jetstream.KeyValuePut}
			<-held
			cancel()
			synctest.Wait() // the watch context's AfterFunc has closed the update channel; Run is in the hook
			select {
			case _, ok := <-w.updates:
				require.False(t, ok, "view %d: the update channel must be closed, not carrying an entry", i)
			default:
				t.Fatalf("view %d: the update channel is still open while Run is held", i)
			}
			close(release)

			runErr := <-done // orders every hook call and terminate Run made before the reads below
			require.ErrorIs(t, runErr, context.Canceled, "view %d", i)
			require.NotErrorIs(t, runErr, ErrWatcherLost, "view %d", i)
			require.Zero(t, lost.Load(), "view %d: OnWatcherLost fired on a stop", i)
			require.ErrorIs(t, sub.Err(), ErrViewStopped, "view %d", i)
			recvClosed(t, sub)
		}
	})
}

// errOpen is the WatchAll failure refusingSource returns.
var errOpen = errors.New("test: watch refused")

// refusingSource fails every WatchAll with errOpen.
type refusingSource struct{ opened int }

func (s *refusingSource) WatchAll(context.Context, ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	s.opened++
	return nil, errOpen
}

// TestRunOpenFailure (spec "The watch cannot open"): a WatchAll that fails while
// the context is live fails the view: Run returns that error, WaitCaughtUp,
// reads and attaches return an error matching ErrWatcherLost and ErrNotReady,
// and OnWatcherLost does not fire, because the watch was never held. When the
// context has already ended the failure is a stop. Run runs on the test's
// goroutine, so its return means nothing of Run's is running, and the bubble
// checks no other goroutine was left.
func TestRunOpenFailure(t *testing.T) {
	t.Run("live context", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			var lost int
			view, err := New[string](&refusingSource{}, decodeTest, WithHooks(Hooks{
				OnWatcherLost: func(error) { lost++ },
			}))
			require.NoError(t, err)
			require.ErrorIs(t, view.Run(t.Context()), errOpen)

			err = view.WaitCaughtUp(t.Context())
			require.ErrorIs(t, err, ErrWatcherLost)
			require.ErrorIs(t, err, errOpen)
			_, _, err = view.Get("k")
			require.ErrorIs(t, err, ErrNotReady)
			require.ErrorIs(t, err, ErrWatcherLost)
			_, err = view.Subscribe(t.Context())
			require.ErrorIs(t, err, ErrWatcherLost)
			require.Zero(t, lost, "OnWatcherLost must not fire when the watch never opened")
		})
	})
	t.Run("ended context", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			view, err := New[string](&refusingSource{}, decodeTest)
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			runErr := view.Run(ctx)
			require.ErrorIs(t, runErr, context.Canceled)
			require.NotErrorIs(t, runErr, errOpen)
			require.ErrorIs(t, view.WaitCaughtUp(t.Context()), ErrViewStopped)
		})
	})
}

// TestDecodeOncePerWrite (spec "Many subscribers"): with five subscriptions
// attached, one write is decoded once and reaches all five, over the view's one
// watch.
func TestDecodeOncePerWrite(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var decodes atomic.Int32
		h := newHarnessDecoding(t, func(key string, value []byte, meta EntryMeta) (string, bool, error) {
			decodes.Add(1)
			return decodeTest(key, value, meta)
		})
		h.endReplay()

		var subs []*Subscription[string]
		for range 3 {
			_, sub, err := h.view.SnapshotAndSubscribe(t.Context())
			require.NoError(t, err)
			subs = append(subs, sub)
		}
		for range 2 {
			sub, err := h.view.Subscribe(t.Context())
			require.NoError(t, err)
			subs = append(subs, sub)
		}

		h.put("k", "v", 1) // returns once Run has applied the write, so decode has run
		require.Equal(t, int32(1), decodes.Load(), "one write is decoded once, whatever the subscriber count")
		h.tickNow()
		for i, sub := range subs {
			batch := recvBatch(t, sub)
			require.Len(t, batch, 1, "subscription %d", i)
			require.Equal(t, "v", batch[0].Value, "subscription %d", i)
		}
		require.Equal(t, int32(1), decodes.Load(), "fan-out decodes nothing")
		require.Equal(t, 1, h.src.opened, "one watch serves every subscription")
	})
}

// TestDetachedReasonSurvivesStop (spec "A detached subscription outlives the
// stop and the loss"): a subscription whose context ended keeps
// context.Canceled and one that unsubscribed keeps nil, through a later stop
// or lost watch.
func TestDetachedReasonSurvivesStop(t *testing.T) {
	for _, end := range []string{"stop", "loss"} {
		t.Run(end, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := newHarness(t)
				h.endReplay()

				ctx, cancel := context.WithCancel(t.Context())
				cancelled, err := h.view.Subscribe(ctx)
				require.NoError(t, err)
				unsubscribed, err := h.view.Subscribe(t.Context())
				require.NoError(t, err)
				cancel()
				unsubscribed.Unsubscribe()
				recvClosed(t, cancelled)
				recvClosed(t, unsubscribed)

				if end == "stop" {
					require.ErrorIs(t, h.stop(), context.Canceled)
				} else {
					h.w.lose()
					require.ErrorIs(t, h.runResult(), ErrWatcherLost)
				}
				require.ErrorIs(t, cancelled.Err(), context.Canceled)
				require.NotErrorIs(t, cancelled.Err(), ErrViewStopped)
				require.NotErrorIs(t, cancelled.Err(), ErrWatcherLost)
				require.NoError(t, unsubscribed.Err())
			})
		})
	}
}

// TestDetachReasonIsTheFirst: a subscription keeps the reason it was first
// detached with. A context end followed by Unsubscribe (a deferred Unsubscribe
// after the context ended) leaves context.Canceled; an Unsubscribe followed by
// the context's end leaves nil.
func TestDetachReasonIsTheFirst(t *testing.T) {
	t.Run("context end, then Unsubscribe", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := newHarness(t)
			h.endReplay()
			ctx, cancel := context.WithCancel(t.Context())
			sub, err := h.view.Subscribe(ctx)
			require.NoError(t, err)
			cancel()
			recvClosed(t, sub)
			sub.Unsubscribe()
			require.ErrorIs(t, sub.Err(), context.Canceled)
			require.ErrorIs(t, h.stop(), context.Canceled)
		})
	})
	t.Run("Unsubscribe, then context end", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := newHarness(t)
			h.endReplay()
			ctx, cancel := context.WithCancel(t.Context())
			sub, err := h.view.Subscribe(ctx)
			require.NoError(t, err)
			sub.Unsubscribe()
			recvClosed(t, sub)
			cancel()
			synctest.Wait() // anything the context's end would run in the bubble has run
			require.NoError(t, sub.Err())
			require.ErrorIs(t, h.stop(), context.Canceled)
		})
	})
}
