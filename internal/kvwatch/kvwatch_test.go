package kvwatch

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// openWatcher is a jetstream.KeyWatcher whose update channel never closes: the state nats.go
// v1.54.0 leaves a watch in when its context ends between Subscribe and the install of the
// handler that closes the channel (#454).
type openWatcher struct{ updates chan jetstream.KeyValueEntry }

func (w openWatcher) Updates() <-chan jetstream.KeyValueEntry { return w.updates }
func (w openWatcher) Stop() error                             { return nil }

// TestStopGivesUpOnADeliveryThatNeverCloses: after the caller's context ends, Stop waits for the
// watch's delivery to close for at most 5 s (the entity-state-follower spec, "The watch's delivery
// never closes"), then returns ErrDeliveryOpen. The bubble's clock advances only when every
// goroutine in it is blocked, so time.Since reads exactly how long Stop waited; with no bound the
// bubble deadlocks and the test fails.
func TestStopGivesUpOnADeliveryThatNeverCloses(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		start := time.Now()
		err := Stop(ctx, openWatcher{updates: make(chan jetstream.KeyValueEntry)})
		if !errors.Is(err, ErrDeliveryOpen) {
			t.Fatalf("Stop = %v; want ErrDeliveryOpen", err)
		}
		if elapsed := time.Since(start); elapsed != 5*time.Second {
			t.Errorf("Stop returned after %v; want 5s", elapsed)
		}
	})
}
