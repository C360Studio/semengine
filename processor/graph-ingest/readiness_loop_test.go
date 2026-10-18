package graphingest

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/c360studio/semengine/graph/readiness"
	"github.com/c360studio/semengine/natsclient"
)

// recordingStatusWriter is a readiness.StatusWriter that records the instant of each Put.
// Inside a synctest bubble that instant is the bubble's clock, so the test reads the
// loop's cadence exactly.
type recordingStatusWriter struct {
	mu   sync.Mutex
	puts []time.Time
}

func (w *recordingStatusWriter) Put(_ context.Context, _ string, _ []byte) (uint64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.puts = append(w.puts, time.Now())
	return uint64(len(w.puts)), nil
}

func (w *recordingStatusWriter) times() []time.Time {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]time.Time(nil), w.puts...)
}

// TestStatusMetricsLoopPublishesOnTheHeartbeat is the status loop's cadence (design D5),
// proved without NATS in a synctest bubble: one publish when the loop starts, one per
// readiness.DefaultHeartbeat after it, and none once its context ends. The component is
// built by the factory and binds no consumer, so a tick reads no backlog; its publisher
// writes to a recording fake.
func TestStatusMetricsLoopPublishesOnTheHeartbeat(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		natsClient, err := natsclient.NewClient("")
		if err != nil {
			t.Fatal(err)
		}
		configJSON, err := json.Marshal(DefaultConfig())
		if err != nil {
			t.Fatal(err)
		}
		comp, err := CreateGraphIngest(configJSON, testDependencies(t, natsClient))
		if err != nil {
			t.Fatal(err)
		}
		c := comp.(*Component)
		writer := &recordingStatusWriter{}
		c.statusPublisher, err = readiness.NewPublisher(writer, readiness.KeyGraphIngest)
		if err != nil {
			t.Fatal(err)
		}

		start := time.Now()
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() {
			defer close(done)
			c.statusMetricsLoop(ctx)
		}()

		const heartbeats = 3
		want := []time.Time{start}
		synctest.Wait()
		assertPutTimes(t, "at start", writer.times(), want)
		for i := 1; i <= heartbeats; i++ {
			<-time.After(readiness.DefaultHeartbeat)
			synctest.Wait()
			want = append(want, start.Add(time.Duration(i)*readiness.DefaultHeartbeat))
			assertPutTimes(t, "after a heartbeat", writer.times(), want)
		}

		cancel()
		<-done
		<-time.After(2 * readiness.DefaultHeartbeat)
		synctest.Wait()
		assertPutTimes(t, "after the context ended", writer.times(), want)
	})
}

func assertPutTimes(t *testing.T, when string, got, want []time.Time) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d publishes at %v, want %d at %v", when, len(got), got, len(want), want)
	}
	for i := range want {
		if !got[i].Equal(want[i]) {
			t.Fatalf("%s: publish %d at %v, want %v", when, i, got[i], want[i])
		}
	}
}
