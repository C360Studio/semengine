package natsclient

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// A caller that cancels while a request is in flight gets context.Canceled, and that cancellation
// never counts toward the shared circuit breaker, on every request method that records failures
// (early check E1). The responder blocks until the test lets it go, and the test cancels only once
// the responder has been entered, so the cancellation always lands while the attempt waits for its
// reply.
//
// Every path reports rather than hangs (Codex F42): each subtest has its own subject and
// responder, so no invocation outlives its subtest's channels; the responder's release is a close,
// safe after the responder has exited or if it never ran; a test-owned bound limits both the wait
// for entry and the wait for the call to return after the cancellation (early check G3); and the
// cancelling goroutine is joined on every outcome.
func TestRequestCancelledInFlightRecordsNoFailure(t *testing.T) {
	c, err := NewClient(embeddedServerURL(t), WithHealthInterval(0))
	require.NoError(t, err)
	require.NoError(t, c.Connect(t.Context()))
	closeInCleanup(t, c)

	retry := RetryConfig{MaxRetries: 2, InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond, BackoffMultiplier: 1}
	for _, tc := range []struct {
		name string
		call func(ctx context.Context, subject string) error
	}{
		{"Request", func(ctx context.Context, subject string) error {
			_, err := c.Request(ctx, subject, nil, time.Minute)
			return err
		}},
		{"RequestWithHeaders", func(ctx context.Context, subject string) error {
			_, err := c.RequestWithHeaders(ctx, subject, nil, nil, time.Minute)
			return err
		}},
		{"RequestClassified", func(ctx context.Context, subject string) error {
			_, err := c.RequestClassified(ctx, subject, nil, time.Minute)
			return err
		}},
		{"RequestWithRetry", func(ctx context.Context, subject string) error {
			_, err := c.RequestWithRetry(ctx, subject, nil, time.Minute, retry)
			return err
		}},
		{"RequestWithRetryClassified", func(ctx context.Context, subject string) error {
			_, err := c.RequestWithRetryClassified(ctx, subject, nil, time.Minute, retry)
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			subject := "cancel.inflight." + tc.name
			// entered holds one signal: the first invocation records its entry, a later one (a
			// retry the code under test should not make) never blocks on it.
			entered := make(chan struct{}, 1)
			release := make(chan struct{})
			sub, err := c.SubscribeForRequests(t.Context(), subject, func(ctx context.Context, _ []byte) ([]byte, error) {
				select {
				case entered <- struct{}{}:
				default:
				}
				select {
				case <-release:
				case <-ctx.Done():
				}
				return []byte("late"), nil
			})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, sub.Unsubscribe()) })
			require.NoError(t, c.GetConnection().FlushTimeout(10*time.Second))

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			// bound is the test's own limit on the whole call, entry and return, far beyond any
			// healthy dispatch and well under the per-attempt time.Minute: a call that ignores the
			// cancellation fails a named assertion here instead of reaching the suite timeout.
			const limit = 10 * time.Second
			bound, stopBound := context.WithTimeout(t.Context(), limit)
			defer stopBound()
			returned := make(chan struct{})
			waiterDone := make(chan struct{})
			var cancelledAfterEntry, entryTimedOut bool
			go func() {
				defer close(waiterDone)
				select {
				case <-entered: // the request reached the responder and is waiting for its reply
					cancelledAfterEntry = true
					cancel()
				case <-bound.Done():
					entryTimedOut = true
					cancel() // so a call that honours cancellation returns and the test reports
				case <-returned: // the call ended before the responder was entered
				}
			}()

			before := c.GetStatus().FailureCount
			result := make(chan error, 1) // buffered: the call never blocks on a test that has stopped waiting
			go func() { result <- tc.call(ctx, subject) }()
			callReturned := false
			select {
			case err = <-result:
				callReturned = true
				close(returned)
			case <-bound.Done():
			}
			close(release) // also lets a call that ignored the cancellation finish with the late reply
			<-waiterDone

			require.False(t, entryTimedOut, "the responder was not entered within %v; call returned %t, err %v", limit, callReturned, err)
			require.True(t, callReturned, "the call did not return within %v although it was cancelled after the responder was entered", limit)
			require.True(t, cancelledAfterEntry, "the call returned before the responder was entered: %v", err)
			require.ErrorIs(t, err, context.Canceled)
			require.Equal(t, before, c.GetStatus().FailureCount, "a caller's cancellation counted as a transport failure")
		})
	}
}
