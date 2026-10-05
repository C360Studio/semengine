package natsclient

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// A caller that cancels while a request is in flight gets context.Canceled, and that cancellation
// never counts toward the shared circuit breaker, on every request method that records failures
// (early check E1). The responder blocks until the test lets it go, so the cancellation always
// lands while the attempt waits for its reply.
func TestRequestCancelledInFlightRecordsNoFailure(t *testing.T) {
	c, err := NewClient(embeddedServerURL(t), WithHealthInterval(0))
	require.NoError(t, err)
	require.NoError(t, c.Connect(t.Context()))
	closeInCleanup(t, c)

	const subject = "cancel.inflight"
	entered := make(chan struct{})
	// Each subtest releases its own invocation once its call has returned: request callbacks run
	// one at a time, so a held handler would hold the next subtest's request behind it.
	release := make(chan struct{})
	_, err = c.SubscribeForRequests(t.Context(), subject, func(ctx context.Context, _ []byte) ([]byte, error) {
		entered <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
		}
		return []byte("late"), nil
	})
	require.NoError(t, err)
	require.NoError(t, c.GetConnection().FlushTimeout(10*time.Second))

	retry := RetryConfig{MaxRetries: 2, InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond, BackoffMultiplier: 1}
	for _, tc := range []struct {
		name string
		call func(context.Context) error
	}{
		{"Request", func(ctx context.Context) error { _, err := c.Request(ctx, subject, nil, time.Minute); return err }},
		{"RequestWithHeaders", func(ctx context.Context) error {
			_, err := c.RequestWithHeaders(ctx, subject, nil, nil, time.Minute)
			return err
		}},
		{"RequestClassified", func(ctx context.Context) error {
			_, err := c.RequestClassified(ctx, subject, nil, time.Minute)
			return err
		}},
		{"RequestWithRetry", func(ctx context.Context) error {
			_, err := c.RequestWithRetry(ctx, subject, nil, time.Minute, retry)
			return err
		}},
		{"RequestWithRetryClassified", func(ctx context.Context) error {
			_, err := c.RequestWithRetryClassified(ctx, subject, nil, time.Minute, retry)
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			go func() {
				<-entered // the request reached the responder and is waiting for its reply
				cancel()
			}()
			before := c.GetStatus().FailureCount
			err := tc.call(ctx)
			release <- struct{}{} // the handler entered (the cancel waited for it), so it is waiting here
			require.ErrorIs(t, err, context.Canceled)
			require.Equal(t, before, c.GetStatus().FailureCount, "a caller's cancellation counted as a transport failure")
		})
	}
}
