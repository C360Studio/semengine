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

// TestKeyedPoolRunContextCancelThenShutdownLeavesNothingRunning: cancelling the context
// NewKeyedPool was given stops nothing; the Shutdown that follows returns nil, and the bubble
// returning proves no goroutine of the pool, lane or gauge updater, is left running.
func TestKeyedPoolRunContextCancelThenShutdownLeavesNothingRunning(t *testing.T) {
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

		ctx, cancelStop := context.WithTimeout(context.Background(), time.Hour)
		defer cancelStop()
		require.NoError(t, p.Shutdown(ctx))
	})
}

// processCall is one call of Process as a test saw it: the item, and the error of the context
// Process was given, read at the call.
type processCall struct {
	work   string
	ctxErr error
}

// TestSubmitAfterRunCancelIsProcessed (#131's reproduction): once the context NewKeyedPool was
// given has ended, an item SubmitBlocking accepts still reaches Process, with that ended
// context, and Shutdown returns nil.
func TestSubmitAfterRunCancelIsProcessed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		run, cancel := context.WithCancel(context.Background())
		var calls []processCall // written by the lane; read only after Shutdown's join
		p, err := NewKeyedPool(run, KeyedConfig[string]{
			Lanes: 1, QueueDepth: 1,
			KeyOf: func(string) string { return "key" },
			Process: func(ctx context.Context, _ int, work string) error {
				calls = append(calls, processCall{work: work, ctxErr: ctx.Err()})
				return nil
			},
		}, KeyedDeps{})
		require.NoError(t, err)

		cancel()
		synctest.Wait() // every pool goroutine has seen the cancel and is blocked or gone

		require.NoError(t, p.SubmitBlocking(context.Background(), "late"), "the submit is accepted")
		ctx, cancelStop := context.WithTimeout(context.Background(), time.Hour)
		defer cancelStop()
		require.NoError(t, p.Shutdown(ctx))
		require.Equal(t, []processCall{{work: "late", ctxErr: context.Canceled}}, calls,
			"the accepted item reached Process with the ended run context")
	})
}

// TestRunCancelProcessesQueuedItems: items waiting in a lane's queue when the context
// NewKeyedPool was given ends all reach Process, in submit order, each with the ended context,
// before Shutdown returns nil. A lane that left its queue on the cancel would still process all
// 64 only by winning a random select once per item, with odds of 2^-64.
func TestRunCancelProcessesQueuedItems(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const queued = 64
		run, cancel := context.WithCancel(context.Background())
		entered := make(chan struct{})
		release := make(chan struct{})
		var calls []processCall // written by the lane; read only after Shutdown's join
		p, err := NewKeyedPool(run, KeyedConfig[string]{
			Lanes: 1, QueueDepth: queued,
			KeyOf: func(string) string { return "key" },
			Process: func(ctx context.Context, _ int, work string) error {
				calls = append(calls, processCall{work: work, ctxErr: ctx.Err()})
				if work == "first" {
					close(entered)
					<-release
				}
				return nil
			},
		}, KeyedDeps{})
		require.NoError(t, err)

		want := []processCall{{work: "first", ctxErr: nil}}
		require.NoError(t, p.SubmitBlocking(context.Background(), "first"))
		<-entered
		for i := 0; i < queued; i++ { // these wait in the lane's queue
			work := fmt.Sprintf("queued-%d", i)
			want = append(want, processCall{work: work, ctxErr: context.Canceled})
			require.NoError(t, p.SubmitBlocking(context.Background(), work))
		}

		cancel()
		close(release)
		ctx, cancelStop := context.WithTimeout(context.Background(), time.Hour)
		defer cancelStop()
		require.NoError(t, p.Shutdown(ctx))
		require.Equal(t, want, calls, "every queued item reached Process, in order, with the ended run context")
	})
}

// TestShutdownAfterJoinReturnsNilUnderEndedContext: once the lanes have exited, Shutdown
// returns nil even when its own context has already ended. A Shutdown that chose at random
// between the join and its context would return nil on all 64 calls with odds of 2^-64.
func TestShutdownAfterJoinReturnsNilUnderEndedContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p, err := NewKeyedPool(context.Background(), validKeyedConfig(), KeyedDeps{})
		require.NoError(t, err)
		require.NoError(t, p.SubmitBlocking(context.Background(), "work"))
		ctx, cancelStop := context.WithTimeout(context.Background(), time.Hour)
		defer cancelStop()
		require.NoError(t, p.Shutdown(ctx), "the first Shutdown joins the lanes")

		ended, cancelEnded := context.WithCancel(context.Background())
		cancelEnded() // the ended context is the input under test
		for i := 0; i < 64; i++ {
			require.NoError(t, p.Shutdown(ended), "call %d after the join", i)
		}
	})
}
