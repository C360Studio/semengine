package graphclustering

import (
	"context"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

// TestDetectionIntervalIsRead: the ticker Start runs publishes every detection_interval, 30 s by
// default, and not before; it stops when its context ends (graph-clustering spec, "Kept keys are
// read"). The component is built through CreateGraphClustering, so the interval is the one the
// configuration resolved. Inside a synctest bubble a wait on time.After advances the fake clock
// exactly, and synctest.Wait returns once the ticker goroutine is blocked, so each count is read
// after every tick due by then has run (docs/testing.md).
func TestDetectionIntervalIsRead(t *testing.T) {
	for raw, every := range map[string]time.Duration{`{}`: 30 * time.Second, `{"detection_interval": "5m"}`: 5 * time.Minute} {
		t.Run(raw, func(t *testing.T) {
			c := mustCreate(t, raw)
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				var ticks atomic.Int32
				done := make(chan struct{})
				go func() {
					defer close(done)
					c.runTicks(ctx, func(context.Context) { ticks.Add(1) })
				}()

				<-time.After(every - time.Nanosecond)
				synctest.Wait()
				if ticks.Load() != 0 {
					t.Fatalf("%d ticks before %v; want none", ticks.Load(), every)
				}
				<-time.After(time.Nanosecond)
				synctest.Wait()
				if ticks.Load() != 1 {
					t.Fatalf("%d ticks at %v; want 1", ticks.Load(), every)
				}
				<-time.After(2 * every)
				synctest.Wait()
				if ticks.Load() != 3 {
					t.Fatalf("%d ticks at %v; want 3", ticks.Load(), 3*every)
				}

				cancel()
				<-done // the ticker returned once its context ended
				<-time.After(every)
				if ticks.Load() != 3 {
					t.Fatalf("%d ticks after the context ended; want still 3", ticks.Load())
				}
			})
		})
	}
}
