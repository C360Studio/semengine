package retry_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/c360studio/semengine/pkg/retry"
)

func TestReviewerRetryNilContextRefusedBeforeOperation(t *testing.T) {
	calls := 0
	defer func() {
		if p := recover(); p != nil {
			t.Errorf("nil context panicked after %d callbacks: %v", calls, p)
		}
	}()
	err := retry.Do(nil, retry.DefaultConfig(), func() error { calls++; return errors.New("retry") })
	if err == nil || calls != 0 {
		t.Errorf("nil context: err=%v calls=%d", err, calls)
	}
}

func TestReviewerRetryJitterTinyDelayDoesNotPanic(t *testing.T) {
	defer func() {
		if p := recover(); p != nil {
			t.Errorf("accepted positive jitter delay panicked: %v", p)
		}
	}()
	err := retry.Do(t.Context(), retry.Config{MaxAttempts: 2, InitialDelay: time.Nanosecond, MaxDelay: time.Second, Multiplier: 2, AddJitter: true}, func() error { return errors.New("retry") })
	if err == nil {
		t.Error("failed attempts returned nil")
	}
}

func TestReviewerRetryEndedContextDoesNotRunOperation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	calls := 0
	err := retry.Do(ctx, retry.DefaultConfig(), func() error { calls++; return nil })
	if !errors.Is(err, context.Canceled) || calls != 0 {
		t.Errorf("ended context: err=%v calls=%d", err, calls)
	}
}
