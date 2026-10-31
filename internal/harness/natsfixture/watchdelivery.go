package natsfixture

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/c360studio/semengine/internal/harness/probe"
)

// The failure these two exist for: at nats.go v1.55.0 a KV watch's Stop only unsubscribes, so a
// delivery already blocked sending into the watch's full updates buffer keeps running after Stop
// until someone reads the buffer (#418), and a watch can be left with a channel that never closes
// (#454). internal/kvwatch.Stop drains for that; a test proving a caller leaves no delivery behind
// first backs the delivery up (AwaitNativeDeliveryBlocked), then checks none is left
// (NativeDeliveryStacks). nats.go offers no way to observe its delivery goroutine, so they read the
// goroutine dump.

// NativeDeliveryStacks returns the stacks of goroutines inside nats.go's KV watch delivery callback,
// or "" when there are none.
func NativeDeliveryStacks() string {
	buf := make([]byte, 1<<20)
	var found []string
	for _, stack := range strings.Split(string(buf[:runtime.Stack(buf, true)]), "\n\n") {
		if strings.Contains(stack, "jetstream.(*kvs).WatchFiltered.func1") {
			found = append(found, stack)
		}
	}
	return strings.Join(found, "\n\n")
}

// AwaitNativeDeliveryBlocked waits, up to bound, for nats.go's watch delivery to be blocked
// sending into its full updates buffer.
func AwaitNativeDeliveryBlocked(ctx context.Context, bound time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, bound)
	defer cancel()
	if _, err := probe.Await(ctx, func(context.Context) (bool, error) {
		return strings.Contains(NativeDeliveryStacks(), "[chan send]"), nil
	}, func(blocked bool) bool { return blocked }); err != nil {
		return fmt.Errorf("the native watch delivery did not back up: %w", err)
	}
	return nil
}

// AwaitNoStacks waits, up to bound, for scan to return "", and returns an error carrying the last
// stacks it saw when it does not. A goroutine that has told its joiner it is done (a deferred
// WaitGroup.Done, a deferred channel close) is still in the dump until it exits, so a single read
// taken just after the join can see one that has already finished (#490). A goroutine that is
// really left behind is still there when the bound ends.
func AwaitNoStacks(ctx context.Context, bound time.Duration, scan func() string) error {
	ctx, cancel := context.WithTimeout(ctx, bound)
	defer cancel()
	if stacks, err := probe.Await(ctx, func(context.Context) (string, error) {
		return scan(), nil
	}, func(stacks string) bool { return stacks == "" }); err != nil {
		return fmt.Errorf("%w\n%s", err, stacks)
	}
	return nil
}
