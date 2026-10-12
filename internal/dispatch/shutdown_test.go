package dispatch

import (
	"context"
	"fmt"
	"testing"
	"testing/synctest"
	"time"

	"github.com/c360studio/semengine/metric"
	"github.com/stretchr/testify/require"
)

// KeyedPool takes the Shutdown(ctx) shape (background-work spec; design D5): its lanes run the
// caller's Process, so stopping waits on work the pool does not control, and the wait is bounded
// by the caller's context alone. Each test runs in a synctest bubble, so the bubble returning
// proves no lane, coordinator or metrics goroutine is left running.

// TestKeyedPoolShutdownWithoutDeadlineWaitsForJoin: given a context with no deadline, Shutdown
// returns only after the lanes have drained and exited, however long Process takes. It has no
// wait of its own: an hour on the bubble's clock passes while it still waits.
func TestKeyedPoolShutdownWithoutDeadlineWaitsForJoin(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// A lane that left its queue on Shutdown would still process all of these only by
		// winning a random select once per item, with odds of 2^-queued.
		const queued = 16
		entered := make(chan string, 1+queued)
		release := make(chan struct{})
		var processed []string // written by the lane; read only after Shutdown's join
		p, err := NewKeyedPool(context.Background(), KeyedConfig[string]{
			Lanes: 1, QueueDepth: queued,
			KeyOf: func(string) string { return "key" },
			Process: func(_ context.Context, _ int, work string) error {
				entered <- work
				<-release
				processed = append(processed, work)
				return nil
			},
		}, KeyedDeps{})
		require.NoError(t, err)

		want := []string{"first"}
		require.NoError(t, p.SubmitBlocking(context.Background(), "first"))
		require.Equal(t, "first", <-entered)
		for i := 0; i < queued; i++ { // these wait in the lane's queue
			work := fmt.Sprintf("queued-%d", i)
			want = append(want, work)
			require.NoError(t, p.SubmitBlocking(context.Background(), work))
		}

		returned := make(chan error, 1)
		go func() {
			noDeadline := context.Background() // the context without a deadline is the input under test
			returned <- p.Shutdown(noDeadline)
		}()
		<-time.After(time.Hour)
		select {
		case err := <-returned:
			t.Fatalf("Shutdown returned %v while Process was still running", err)
		default:
		}

		close(release)
		require.NoError(t, <-returned)
		require.Equal(t, want, processed, "Shutdown drained the queued items before it returned")
	})
}

// TestKeyedPoolShutdownUnderABlockedProcess: while Process blocks, Shutdown returns ctx.Err()
// when its context ends; Shutdown has begun, so a later SubmitBlocking is refused with
// ErrStopped; once Process returns, the lane drains and exits and a later Shutdown returns nil.
func TestKeyedPoolShutdownUnderABlockedProcess(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		entered := make(chan struct{})
		release := make(chan struct{})
		p, err := NewKeyedPool(context.Background(), KeyedConfig[string]{
			Lanes: 1, QueueDepth: 1,
			KeyOf: func(string) string { return "key" },
			Process: func(context.Context, int, string) error {
				close(entered)
				<-release
				return nil
			},
		}, KeyedDeps{})
		require.NoError(t, err)
		require.NoError(t, p.SubmitBlocking(context.Background(), "work"))
		<-entered

		first, cancelFirst := context.WithTimeout(context.Background(), time.Second)
		defer cancelFirst()
		require.ErrorIs(t, p.Shutdown(first), context.DeadlineExceeded, "Shutdown returns ctx.Err() while Process blocks")
		require.ErrorIs(t, p.SubmitBlocking(context.Background(), "late"), ErrStopped,
			"a submit after Shutdown began is refused")

		close(release)
		later, cancelLater := context.WithTimeout(context.Background(), time.Hour)
		defer cancelLater()
		require.NoError(t, p.Shutdown(later), "a later Shutdown returns nil once the lane has exited")
	})
}

// TestKeyedPoolShutdownRefusesNilContext: a nil context is refused at the call, and the refusal
// begins no shutdown: the pool still accepts and runs work.
func TestKeyedPoolShutdownRefusesNilContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		processed := make(chan string, 1)
		p, err := NewKeyedPool(context.Background(), KeyedConfig[string]{
			Lanes: 1, QueueDepth: 1,
			KeyOf: func(string) string { return "key" },
			Process: func(_ context.Context, _ int, work string) error {
				processed <- work
				return nil
			},
		}, KeyedDeps{})
		require.NoError(t, err)

		var nilCtx context.Context // the nil context is the input under test
		require.Error(t, p.Shutdown(nilCtx))
		require.NoError(t, p.SubmitBlocking(context.Background(), "after"))
		require.Equal(t, "after", <-processed)

		ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
		defer cancel()
		require.NoError(t, p.Shutdown(ctx))
	})
}

// TestKeyedPoolRunContextCancelLeavesNothingRunning: cancelling the context NewKeyedPool was
// given aborts the pool, and with no Shutdown called no goroutine of the pool is left running.
func TestKeyedPoolRunContextCancelLeavesNothingRunning(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		run, cancel := context.WithCancel(context.Background())
		processed := make(chan string, 1)
		p, err := NewKeyedPool(run, KeyedConfig[string]{
			Lanes: 2, QueueDepth: 1, Name: "aborted",
			KeyOf: func(s string) string { return s },
			Process: func(_ context.Context, _ int, work string) error {
				processed <- work
				return nil
			},
		}, KeyedDeps{MetricsRegistry: metric.NewMetricsRegistry()}) // with metrics, so the gauge updater runs too
		require.NoError(t, err)
		require.NoError(t, p.SubmitBlocking(run, "work"))
		require.Equal(t, "work", <-processed)
		cancel()
	})
}
