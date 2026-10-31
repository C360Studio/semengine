package natsfixture

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestAwaitNoStacksPassesOnceTheGoroutineIsGone: a goroutine still in the dump on the first
// reads, gone on a later one, is not reported as left behind (#490).
func TestAwaitNoStacksPassesOnceTheGoroutineIsGone(t *testing.T) {
	reads := 0
	scan := func() string {
		reads++
		if reads < 3 {
			return "goroutine 7 [runnable]:"
		}
		return ""
	}
	if err := AwaitNoStacks(t.Context(), time.Minute, scan); err != nil {
		t.Fatalf("AwaitNoStacks = %v; want nil once the scan is empty", err)
	}
	if reads != 3 {
		t.Fatalf("scan read %d times; want 3", reads)
	}
}

// TestAwaitNoStacksReportsAGoroutineThatStays: a goroutine still there when the wait ends is
// reported, with its stack, so the wait cannot hide a real leftover.
func TestAwaitNoStacksReportsAGoroutineThatStays(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	reads := 0
	err := AwaitNoStacks(ctx, time.Minute, func() string {
		if reads++; reads == 2 {
			cancel() // ends the wait on the event, not on the bound
		}
		return "goroutine 9 [chan send]:"
	})
	if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "goroutine 9 [chan send]:") {
		t.Fatalf("AwaitNoStacks = %v; want the context's error and the last stacks", err)
	}
}
