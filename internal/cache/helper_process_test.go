package cache

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"github.com/c360studio/semengine/internal/harness/prochost"
)

const (
	callbackPanicHelper = "coalescing-callback-panic"
	callbackPanicValue  = "coalescing callback panic sentinel"
)

// TestHelperProcess holds the helpers this package's tests start in a child process; each runs
// only in a child started with its marker (internal/harness/prochost).
func TestHelperProcess(_ *testing.T) {
	prochost.Helper(callbackPanicHelper, func() {
		// Parks on SIGTERM, never on a bare select{}: a fully blocked child is killed by the
		// runtime, which would also exit 2.
		terminated := make(chan os.Signal, 1)
		signal.Notify(terminated, syscall.SIGTERM)
		set := NewCoalescingSet(context.Background(), time.Millisecond, func([]string) {
			panic(callbackPanicValue)
		})
		set.Add("work")
		<-terminated
	})
}
