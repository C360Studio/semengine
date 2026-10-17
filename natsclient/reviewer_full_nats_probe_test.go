package natsclient

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

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
