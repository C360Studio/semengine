// Package kvwatch stops a NATS KV watch and waits for its delivery to close, so a caller that
// returns after stopping its watch leaves no nats.go delivery running behind it. Each caller wraps
// Stop's error with the bucket it watches.
package kvwatch

import (
	"context"
	"errors"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// DrainBudget bounds Stop's wait for the watch's delivery to close after the caller's context has ended.
const DrainBudget = 5 * time.Second

// ErrDeliveryOpen says the watch's update channel was still open when DrainBudget ran out.
var ErrDeliveryOpen = errors.New("kvwatch: the watch's delivery did not close after Stop")

// Stop stops the watch and drains its updates until nats.go closes the channel, for at most
// DrainBudget after ctx, the caller's, has ended. Entries read here are dropped. At nats.go v1.54.0
// Stop only unsubscribes: a delivery already blocked on the full updates buffer ends only when the
// buffer is read, and the channel closes after it, so on that path returning is a join
// (graph/follower's TestFollowerRunReturnsAndLeavesNothing drives it with a backlogged watch). The
// bound is for the one path with nothing to join: WatchAll subscribes before it installs the
// handler that closes the channel (jetstream/kv.go:1356-1368), and nats.go reads that handler
// once, when its delivery goroutine exits (nats.go:3830-3834). A watch whose context ends, or whose
// connection closes, in between leaves the channel open for good with no delivery goroutine
// running (#454). TestStopGivesUpOnADeliveryThatNeverCloses drives the bound on a scripted watcher;
// the window itself has not been reproduced against nats.go.
//
// Stop returns nil, or the watch's Stop error when it says more than that the watch was already
// gone. When the budget runs out it returns ErrDeliveryOpen, joined with the watch's Stop error if
// there was one. A nil context is refused before the watcher is touched.
func Stop(ctx context.Context, watcher jetstream.KeyWatcher) error {
	if ctx == nil {
		return errors.New("kvwatch: Stop needs a non-nil context")
	}
	stopErr := watcher.Stop()
	// Terminal cleanup after the caller's context has ended, so it runs detached and bounded.
	drainCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), DrainBudget)
	defer cancel()
	updates := watcher.Updates()
	for open := true; open; {
		select {
		case _, open = <-updates:
		case <-drainCtx.Done():
			// The delivery never closed, so a Stop error cannot be read as "already gone".
			return errors.Join(ErrDeliveryOpen, stopErr)
		}
	}
	// The delivery has closed. Stop's error then says only that the watch was already gone: nats.go
	// also unsubscribes when the watch's context ends (js.go, subscribe), and can delete the
	// consumer before Stop does (seen as ErrConsumerNotFound in graph/follower's
	// TestFollowerRunReturnsAndLeavesNothing).
	if stopErr != nil && !errors.Is(stopErr, nats.ErrBadSubscription) &&
		!errors.Is(stopErr, nats.ErrConnectionClosed) && !errors.Is(stopErr, nats.ErrConsumerNotFound) {
		return stopErr
	}
	return nil
}
