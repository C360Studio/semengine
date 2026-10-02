package probe

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

// closed reports whether ch is closed without blocking; a nil channel is never closed.
func closed(ch <-chan struct{}) bool {
	if ch == nil {
		return false
	}
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// waitClosed fails the test if ch does not close before the test's own context ends; the bound
// is the test binary's -timeout, never a sleep.
func waitClosed(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	if ch == nil {
		t.Fatalf("%s: nil channel", what)
	}
	select {
	case <-ch:
	case <-t.Context().Done():
		t.Fatalf("%s: not closed before the test ended", what)
	}
}

func TestCallbackEnterReleaseJoin(t *testing.T) {
	cb := NewCallback()
	if closed(cb.Entered()) || closed(cb.Joined()) {
		t.Fatal("fresh callback already entered or joined")
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	returned := make(chan struct{})
	go func() {
		cb.Block(ctx)
		close(returned)
	}()
	waitClosed(t, cb.Entered(), "Entered")
	if closed(cb.Joined()) || closed(returned) {
		t.Fatal("Block returned before Release")
	}
	cb.Release()
	cb.Release() // idempotent: a deferred release after an explicit one must not panic
	waitClosed(t, cb.Joined(), "Joined")
	waitClosed(t, returned, "Block returned")
	if err := cb.ContextErrAtRelease(); err != nil {
		t.Fatalf("ContextErrAtRelease = %v, want nil: the context was live throughout the block", err)
	}
}

// TestCallbackRecordsCancelledContext proves ContextErrAtRelease is an observation, not a
// constant: a context cancelled while the callback is blocked is reported.
func TestCallbackRecordsCancelledContext(t *testing.T) {
	cb := NewCallback()
	ctx, cancel := context.WithCancel(t.Context())
	go cb.Block(ctx)
	waitClosed(t, cb.Entered(), "Entered")
	cancel()
	cb.Release()
	waitClosed(t, cb.Joined(), "Joined")
	if err := cb.ContextErrAtRelease(); !errors.Is(err, context.Canceled) {
		t.Fatalf("ContextErrAtRelease = %v, want context.Canceled", err)
	}
}

func TestCallbackReleaseBeforeEntry(t *testing.T) {
	cb := NewCallback()
	cb.Release()
	cb.Block(t.Context())
	if !closed(cb.Entered()) || !closed(cb.Joined()) {
		t.Fatal("a released callback must still record entry and join")
	}
}

func TestObservedContextSignalsFirstDone(t *testing.T) {
	parent, cancel := context.WithCancel(t.Context())
	defer cancel()
	ctx := Observe(parent)
	if closed(ctx.Observed()) {
		t.Fatal("observed before any Done call")
	}
	if ctx.Err() != nil {
		t.Fatal("Err on a live parent")
	}
	if closed(ctx.Observed()) {
		t.Fatal("Err is not a Done observation")
	}
	done := ctx.Done()
	if !closed(ctx.Observed()) {
		t.Fatal("Done did not signal the observation")
	}
	cancel()
	waitClosed(t, done, "Done after parent cancel")
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("Err = %v, want the parent's", ctx.Err())
	}
}

func TestObservedContextHoldsUntilProceed(t *testing.T) {
	proceed := make(chan struct{})
	ctx := ObserveAndHold(t.Context(), proceed)
	gotDone := make(chan struct{})
	go func() {
		_ = ctx.Done()
		close(gotDone)
	}()
	waitClosed(t, ctx.Observed(), "Observed")
	if closed(gotDone) {
		t.Fatal("Done returned while held")
	}
	close(proceed)
	waitClosed(t, gotDone, "Done after proceed")
	_ = ctx.Done() // later calls do not hold again
}

func TestAwaitReturnsWhenConditionHolds(t *testing.T) {
	n := 0
	got, err := Await(t.Context(), func(context.Context) (int, error) {
		n++
		return n, nil
	}, func(v int) bool { return v >= 3 })
	if err != nil || got != 3 {
		t.Fatalf("Await = %d, %v; want 3, nil", got, err)
	}
}

// TestAwaitReportsLastObservation is the spec scenario: the condition never holds, and the
// failure carries the last value and the last error rather than only "timed out". Every observation
// returns its own value and its own error, and the expectation is whatever the observer returned
// last, so the result does not depend on how many observations fit before the deadline (flake #49:
// the earlier version alternated errors and passed only when that count was even). The bubble's
// clock makes the wait instant.
func TestAwaitReportsLastObservation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
		defer cancel()
		var (
			calls             int
			firstErr, lastErr error
			lastValue         string
		)
		got, err := Await(ctx, func(context.Context) (string, error) {
			calls++
			lastValue = fmt.Sprintf("state=restarting#%d", calls)
			lastErr = fmt.Errorf("inspect: container restarting (call %d)", calls)
			if calls == 1 {
				firstErr = lastErr
			}
			return lastValue, lastErr
		}, func(string) bool { return false })
		if err == nil {
			t.Fatal("Await returned nil for a condition that never held")
		}
		t.Logf("Await failure message after %d call(s): %v", calls, err)
		if calls < 2 {
			t.Fatalf("observer called %d time(s), want at least 2 so that last differs from first", calls)
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("err = %v, want it to wrap context.DeadlineExceeded", err)
		}
		if !errors.Is(err, lastErr) {
			t.Errorf("err = %v, want it to wrap the last observation error %q", err, lastErr)
		}
		if errors.Is(err, firstErr) {
			t.Errorf("err = %v, wraps the first observation error %q, want only the last", err, firstErr)
		}
		for _, want := range []string{lastValue, lastErr.Error(), "observation"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("err = %q, want it to mention %q", err, want)
			}
		}
		if got != lastValue {
			t.Errorf("returned value = %q, want the last observation %q", got, lastValue)
		}
	})
}

// TestAwaitClearsEarlierObservationError pins the ruled mixed history: a clean final
// observation clears the earlier error even though the condition never holds.
func TestAwaitClearsEarlierObservationError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		earlierErr := errors.New("first observation failed")
		const finalValue = "state=ready"
		calls := 0
		got, err := Await(ctx, func(context.Context) (string, error) {
			calls++
			switch calls {
			case 1:
				return "state=failed", earlierErr
			case 2:
				cancel()
				return finalValue, nil
			default:
				return "unexpected extra observation", nil
			}
		}, func(string) bool { return false })
		if calls != 2 {
			t.Errorf("observer called %d times, want exactly 2", calls)
		}
		if got != finalValue {
			t.Errorf("returned value = %q, want final observation %q", got, finalValue)
		}
		if err == nil {
			t.Fatal("Await returned nil for a condition that never held")
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want it to wrap context.Canceled", err)
		}
		for _, want := range []string{finalValue, "(no error)"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("err = %q, want it to mention %q", err, want)
			}
		}
		if strings.Contains(err.Error(), earlierErr.Error()) {
			t.Errorf("err = %q, want no earlier observation error text", err)
		}
		if errors.Is(err, earlierErr) {
			t.Errorf("err = %v, want no earlier observation error wrapping", err)
		}
	})
}

// TestAwaitReportsLastObservationWithoutError covers the other branch of the failure message: no
// observation ever failed, so the message says so and wraps only the context's error.
func TestAwaitReportsLastObservationWithoutError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
		defer cancel()
		calls := 0
		var lastValue string
		got, err := Await(ctx, func(context.Context) (string, error) {
			calls++
			lastValue = fmt.Sprintf("state=starting#%d", calls)
			return lastValue, nil
		}, func(string) bool { return false })
		if err == nil {
			t.Fatal("Await returned nil for a condition that never held")
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("err = %v, want it to wrap context.DeadlineExceeded", err)
		}
		for _, want := range []string{lastValue, "(no error)"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("err = %q, want it to mention %q", err, want)
			}
		}
		if got != lastValue {
			t.Errorf("returned value = %q, want the last observation %q", got, lastValue)
		}
	})
}

func TestAwaitRefusesNilContext(t *testing.T) {
	var nilCtx context.Context // the case under test: callers never pass nil, and Await says so
	_, err := Await(nilCtx, func(context.Context) (int, error) { return 0, nil }, func(int) bool { return true })
	if err == nil {
		t.Fatal("Await(nil, ...) = nil error")
	}
}

func TestAwaitObservesUnderCallerContext(t *testing.T) {
	type key struct{}
	ctx := context.WithValue(t.Context(), key{}, "caller")
	_, err := Await(ctx, func(obsCtx context.Context) (bool, error) {
		return obsCtx.Value(key{}) == "caller", nil
	}, func(ok bool) bool { return ok })
	if err != nil {
		t.Fatalf("observation did not receive the caller's context: %v", err)
	}
}
