package natsclient

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestReviewerNegativeRequestRetryRefuses(t *testing.T) {
	c, err := NewClient(embeddedServerURL(t), WithHealthInterval(0))
	require.NoError(t, err)
	require.NoError(t, c.Connect(t.Context()))
	closeInCleanup(t, c)
	var calls atomic.Int64
	_, err = c.SubscribeForRequests(t.Context(), "review.retry", func(context.Context, []byte) ([]byte, error) {
		calls.Add(1)
		return []byte("handled"), nil
	})
	require.NoError(t, err)
	require.NoError(t, c.GetConnection().Flush())
	// Positive control proves a zero retry count still performs one request.
	data, err := c.RequestWithRetryClassified(t.Context(), "review.retry", nil, 10*time.Second, RetryConfig{MaxRetries: 0})
	require.NoError(t, err)
	require.Equal(t, "handled", string(data))
	require.Equal(t, int64(1), calls.Load())
	t.Run("classified", func(t *testing.T) {
		data, err := c.RequestWithRetryClassified(t.Context(), "review.retry", nil, 10*time.Second, RetryConfig{MaxRetries: -1})
		t.Logf("negative retry: response=%q err=%v handler calls=%d", data, err, calls.Load())
		if err == nil {
			t.Error("invalid negative retry budget returned success without attempting request")
		}
	})
	t.Run("raw", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("invalid negative retry budget panicked: %v", r)
			}
		}()
		_, err := c.RequestWithRetry(t.Context(), "review.retry", nil, 10*time.Second, RetryConfig{MaxRetries: -1})
		if err == nil {
			t.Error("invalid negative retry budget returned success")
		}
	})
}

// Run with -race. Public status polling should be safe during public Close.
func TestReviewerGetStatusConcurrentClose(t *testing.T) {
	c, err := NewClient(embeddedServerURL(t), WithHealthInterval(0))
	require.NoError(t, err)
	require.NoError(t, c.Connect(t.Context()))
	closeInCleanup(t, c)
	started, done := make(chan struct{}), make(chan struct{})
	var stop atomic.Bool
	go func() {
		defer close(done)
		close(started)
		for !stop.Load() {
			_ = c.GetStatus()
		}
	}()
	<-started
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	err = c.Close(ctx)
	stop.Store(true)
	<-done
	require.NoError(t, err)
}
