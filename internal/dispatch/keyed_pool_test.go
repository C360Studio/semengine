package dispatch

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/c360studio/semengine/metric"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stopCtx returns a context with a generous timeout for a graceful Shutdown.
func stopCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// keysOnDistinctLanes returns `count` keys that each hash to a distinct
// lane in p, so a concurrency test can guarantee genuine parallelism
// rather than relying on hash luck. White-box: uses the pool's own
// laneFor so the mapping matches routing exactly.
func keysOnDistinctLanes(p *KeyedPool[string], count int) []string {
	seen := make(map[int]string, count)
	for i := 0; len(seen) < count; i++ {
		k := fmt.Sprintf("key-%d", i)
		lane := p.laneFor(k)
		if _, ok := seen[lane]; !ok {
			seen[lane] = k
		}
	}
	out := make([]string, 0, count)
	for _, k := range seen {
		out = append(out, k)
	}
	return out
}

func validKeyedConfig() KeyedConfig[string] {
	return KeyedConfig[string]{
		Lanes:      2,
		QueueDepth: 4,
		KeyOf:      func(s string) string { return s },
		Process:    func(context.Context, int, string) error { return nil },
	}
}

func TestKeyedPool_ProcessUsesConstructorContext(t *testing.T) {
	type contextKey struct{}
	runCtx, cancel := context.WithCancel(context.WithValue(context.Background(), contextKey{}, "run-value"))
	received := make(chan context.Context, 1)
	processDone := make(chan struct{})

	p, err := NewKeyedPool(runCtx, KeyedConfig[string]{
		Lanes:      1,
		QueueDepth: 1,
		KeyOf:      func(string) string { return "key" },
		Process: func(ctx context.Context, _ int, _ string) error {
			received <- ctx
			<-ctx.Done()
			close(processDone)
			return ctx.Err()
		},
	}, KeyedDeps{})
	require.NoError(t, err)
	require.NoError(t, p.SubmitBlocking(context.Background(), "work"))

	select {
	case got := <-received:
		if got != runCtx {
			t.Fatal("Process did not receive the exact pool constructor context")
		}
		require.Equal(t, "run-value", got.Value(contextKey{}))
	case <-time.After(2 * time.Second):
		t.Fatal("Process did not receive the pool's constructor context")
	}

	cancel()
	select {
	case <-processDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Process did not observe constructor context cancellation")
	}
	require.NoError(t, p.Shutdown(stopCtx(t)))
}

// --- Config validation (task 1.1) ---

func TestKeyedPool_RejectsNilContext(t *testing.T) {
	_, err := NewKeyedPool[string](nil, validKeyedConfig(), KeyedDeps{})
	require.ErrorIs(t, err, ErrInvalidConfig)
}

func TestKeyedPool_RejectsBadConfig(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*KeyedConfig[string])
	}{
		{"zero lanes", func(c *KeyedConfig[string]) { c.Lanes = 0 }},
		{"negative lanes", func(c *KeyedConfig[string]) { c.Lanes = -1 }},
		{"zero queue depth", func(c *KeyedConfig[string]) { c.QueueDepth = 0 }},
		{"nil KeyOf", func(c *KeyedConfig[string]) { c.KeyOf = nil }},
		{"nil Process", func(c *KeyedConfig[string]) { c.Process = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validKeyedConfig()
			tc.mutate(&cfg)
			_, err := NewKeyedPool(context.Background(), cfg, KeyedDeps{})
			require.ErrorIs(t, err, ErrInvalidConfig)
		})
	}
}

// --- Ordering: same key serial in submit order (task 1.5) ---

func TestKeyedPool_SameKeyOrdered(t *testing.T) {
	var mu sync.Mutex
	var got []string
	p, err := NewKeyedPool(context.Background(), KeyedConfig[string]{
		Lanes:      8,
		QueueDepth: 16,
		KeyOf:      func(string) string { return "one-key" }, // all → one lane
		Process: func(_ context.Context, _ int, s string) error {
			mu.Lock()
			got = append(got, s)
			mu.Unlock()
			return nil
		},
	}, KeyedDeps{})
	require.NoError(t, err)

	want := []string{"a", "b", "c", "d", "e"}
	for _, s := range want {
		require.NoError(t, p.SubmitBlocking(context.Background(), s))
	}
	require.NoError(t, p.Shutdown(stopCtx(t)))
	assert.Equal(t, want, got, "same-key items must process in submit order")
}

// --- Same key always lands on the same lane (task 1.5 / shard-safety) ---

func TestKeyedPool_SameKeySameLane(t *testing.T) {
	var mu sync.Mutex
	lanes := make(map[int]struct{})
	p, err := NewKeyedPool(context.Background(), KeyedConfig[string]{
		Lanes:      8,
		QueueDepth: 16,
		KeyOf:      func(string) string { return "sticky" },
		Process: func(_ context.Context, lane int, _ string) error {
			mu.Lock()
			lanes[lane] = struct{}{}
			mu.Unlock()
			return nil
		},
	}, KeyedDeps{})
	require.NoError(t, err)

	// All 20 items key to one lane with a queue of 16: SubmitBlocking waits for
	// capacity; the test's concern is lane affinity, not queue sizing.
	for i := 0; i < 20; i++ {
		require.NoError(t, p.SubmitBlocking(context.Background(), fmt.Sprintf("item-%d", i)))
	}
	require.NoError(t, p.Shutdown(stopCtx(t)))
	assert.Len(t, lanes, 1, "all same-key items must run on exactly one lane")
}

// --- Concurrency: distinct keys run in parallel across lanes (task 1.5) ---

func TestKeyedPool_DifferentKeysConcurrent(t *testing.T) {
	const n = 4
	entered := make(chan struct{}, n)
	release := make(chan struct{})
	p, err := NewKeyedPool(context.Background(), KeyedConfig[string]{
		Lanes:      n,
		QueueDepth: 1,
		KeyOf:      func(s string) string { return s },
		Process: func(_ context.Context, _ int, _ string) error {
			entered <- struct{}{}
			<-release // hold the lane until every item is confirmed in-flight
			return nil
		},
	}, KeyedDeps{})
	require.NoError(t, err)

	for _, k := range keysOnDistinctLanes(p, n) {
		require.NoError(t, p.SubmitBlocking(context.Background(), k))
	}
	// All n must be executing simultaneously (each entered before any released).
	for i := 0; i < n; i++ {
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			t.Fatalf("only %d of %d items ran concurrently — lanes not parallel", i, n)
		}
	}
	close(release)
	require.NoError(t, p.Shutdown(stopCtx(t)))
}

// --- Backpressure: a full lane blocks SubmitBlocking (task 1.5) ---

func TestKeyedPool_Backpressure(t *testing.T) {
	entered := make(chan struct{}, 1)
	block := make(chan struct{})
	p, err := NewKeyedPool(context.Background(), KeyedConfig[string]{
		Lanes:      1,
		QueueDepth: 1,
		KeyOf:      func(string) string { return "k" },
		Process: func(_ context.Context, _ int, _ string) error {
			select {
			case entered <- struct{}{}:
			default:
			}
			<-block
			return nil
		},
	}, KeyedDeps{})
	require.NoError(t, err)

	// First item is picked up by the lane goroutine and blocks in Process.
	require.NoError(t, p.SubmitBlocking(context.Background(), "first"))
	<-entered // now the lane is busy and its queue is empty

	// Second fills the depth-1 queue.
	require.NoError(t, p.SubmitBlocking(context.Background(), "second"))

	// SubmitBlocking on the full lane blocks until its ctx expires.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, p.SubmitBlocking(ctx, "blocked"), context.DeadlineExceeded)

	close(block)
	require.NoError(t, p.Shutdown(stopCtx(t)))
}

// --- Graceful drain: Shutdown finishes buffered work (task 1.5) ---

func TestKeyedPool_ShutdownDrains(t *testing.T) {
	var count int64
	p, err := NewKeyedPool(context.Background(), KeyedConfig[string]{
		Lanes:      4,
		QueueDepth: 256,
		KeyOf:      func(s string) string { return s },
		Process: func(_ context.Context, _ int, _ string) error {
			atomic.AddInt64(&count, 1)
			return nil
		},
	}, KeyedDeps{})
	require.NoError(t, err)

	const total = 200
	for i := 0; i < total; i++ {
		require.NoError(t, p.SubmitBlocking(context.Background(), fmt.Sprintf("k%d", i)))
	}
	require.NoError(t, p.Shutdown(stopCtx(t)))
	assert.Equal(t, int64(total), atomic.LoadInt64(&count), "Shutdown must drain all buffered work")
}

// --- Shutdown/submit race: an accepted submit is never stranded (Codex P1) ---

func TestKeyedPool_ShutdownSubmitRace(t *testing.T) {
	var processed int64
	p, err := NewKeyedPool(context.Background(), KeyedConfig[string]{
		Lanes:      4,
		QueueDepth: 8,
		KeyOf:      func(s string) string { return s },
		Process: func(_ context.Context, _ int, _ string) error {
			atomic.AddInt64(&processed, 1)
			return nil
		},
	}, KeyedDeps{})
	require.NoError(t, err)

	var acceptedNil int64
	var wg sync.WaitGroup
	stop := make(chan struct{})
	started := make(chan struct{})
	var startOnce sync.Once
	const submitters = 8
	for i := 0; i < submitters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; ; j++ {
				select {
				case <-stop:
					return
				default:
				}
				// SubmitBlocking returns nil (accepted) or ErrStopped. Only a
				// nil return asserts the item WILL be processed.
				if err := p.SubmitBlocking(context.Background(), fmt.Sprintf("k%d-%d", id, j)); err == nil {
					atomic.AddInt64(&acceptedNil, 1)
					startOnce.Do(func() { close(started) })
				}
			}
		}(i)
	}

	<-started // ensure submits are in flight, racing with Shutdown
	require.NoError(t, p.Shutdown(stopCtx(t)))
	close(stop)
	wg.Wait()

	// Contract (Codex P1): once Shutdown begins draining, no submit returns
	// success unless that item is guaranteed processed before Shutdown completes.
	// So every nil-returning submit must have been processed — no stranded
	// accepted work.
	assert.Equal(t, atomic.LoadInt64(&acceptedNil), atomic.LoadInt64(&processed),
		"every accepted (nil) submit must be processed before Shutdown completes — no stranded work")
}

// --- A submit after Shutdown is rejected ---

func TestKeyedPool_SubmitAfterShutdown(t *testing.T) {
	p, err := NewKeyedPool(context.Background(), validKeyedConfig(), KeyedDeps{})
	require.NoError(t, err)
	require.NoError(t, p.Shutdown(stopCtx(t)))
	require.ErrorIs(t, p.SubmitBlocking(context.Background(), "x"), ErrStopped)
}

// --- Panic recovery: lane survives, disposition invoked (task 1.5, H2) ---

func TestKeyedPool_PanicRecovery(t *testing.T) {
	var mu sync.Mutex
	var disposed []string
	var recoveredVal any
	var processedAfter int64

	reg := metric.NewMetricsRegistry()
	p, err := NewKeyedPool(context.Background(), KeyedConfig[string]{
		Lanes:      1,
		QueueDepth: 16,
		Name:       "panic_pool",
		KeyOf:      func(string) string { return "same" }, // one lane, ordered
		Process: func(_ context.Context, _ int, s string) error {
			if s == "boom" {
				panic("kaboom")
			}
			atomic.AddInt64(&processedAfter, 1)
			return nil
		},
		OnPanic: func(s string, r any) {
			mu.Lock()
			disposed = append(disposed, s)
			recoveredVal = r
			mu.Unlock()
		},
	}, KeyedDeps{MetricsRegistry: reg})
	require.NoError(t, err)

	require.NoError(t, p.SubmitBlocking(context.Background(), "boom"))
	require.NoError(t, p.SubmitBlocking(context.Background(), "after1"))
	require.NoError(t, p.SubmitBlocking(context.Background(), "after2"))
	require.NoError(t, p.Shutdown(stopCtx(t)))

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"boom"}, disposed, "OnPanic must fire for the panicking item")
	assert.Equal(t, "kaboom", recoveredVal, "recovered value passed to OnPanic")
	assert.Equal(t, int64(2), atomic.LoadInt64(&processedAfter),
		"lane must survive the panic and process later same-lane items")
	assert.Equal(t, float64(3), counterValue(t, reg, "dispatch_completed_total", "panic_pool"),
		"panicked item still counts as completed")
}

// --- Metrics move (task 2.2) ---

func TestKeyedPool_MetricsMove(t *testing.T) {
	reg := metric.NewMetricsRegistry()
	p, err := NewKeyedPool(context.Background(), KeyedConfig[string]{
		Lanes:      4,
		QueueDepth: 64,
		Name:       "test_pool",
		KeyOf:      func(s string) string { return s },
		Process:    func(context.Context, int, string) error { return nil },
	}, KeyedDeps{MetricsRegistry: reg})
	require.NoError(t, err)

	const total = 50
	for i := 0; i < total; i++ {
		require.NoError(t, p.SubmitBlocking(context.Background(), fmt.Sprintf("k%d", i)))
	}
	require.NoError(t, p.Shutdown(stopCtx(t)))

	// Confirm the Prometheus objects actually moved.
	assert.Equal(t, float64(total), counterValue(t, reg, "dispatch_submitted_total", "test_pool"))
	assert.Equal(t, float64(total), counterValue(t, reg, "dispatch_completed_total", "test_pool"),
		"every submitted item completes")
	assert.Equal(t, float64(0), gaugeValue(t, reg, "dispatch_inflight", "test_pool"),
		"inflight returns to zero after drain")
	assert.Equal(t, uint64(total), histogramCount(t, reg, "dispatch_queue_wait_seconds", "test_pool"))
	assert.Equal(t, uint64(total), histogramCount(t, reg, "dispatch_processing_duration_seconds", "test_pool"))
}

// TestKeyedPoolWritesTheRegisteredCollector: two pools with one name on one registry share each
// collector (design D4). The second pool writes the collector metric.RegisterOrGet returned, not
// its own candidate, so the registry gathers both pools' work.
func TestKeyedPoolWritesTheRegisteredCollector(t *testing.T) {
	reg := metric.NewMetricsRegistry()
	runPool := func(items int) {
		p, err := NewKeyedPool(context.Background(), KeyedConfig[string]{
			Lanes: 2, QueueDepth: 8, Name: "shared",
			KeyOf:   func(s string) string { return s },
			Process: func(context.Context, int, string) error { return nil },
		}, KeyedDeps{MetricsRegistry: reg})
		require.NoError(t, err)
		for i := 0; i < items; i++ {
			require.NoError(t, p.SubmitBlocking(context.Background(), fmt.Sprintf("k%d", i)))
		}
		require.NoError(t, p.Shutdown(stopCtx(t)))
	}
	runPool(2)
	runPool(3)

	assert.Equal(t, float64(5), counterValue(t, reg, "dispatch_submitted_total", "shared"))
	assert.Equal(t, float64(5), counterValue(t, reg, "dispatch_completed_total", "shared"))
	assert.Equal(t, uint64(5), histogramCount(t, reg, "dispatch_queue_wait_seconds", "shared"))
	assert.Equal(t, uint64(5), histogramCount(t, reg, "dispatch_processing_duration_seconds", "shared"))
}

// TestKeyedPoolReturnsARegistrationRefusal: when the registry refuses one of the pool's
// collectors, NewKeyedPool returns the refusal and no pool; at the pin the refusal was dropped.
// The bubble returning shows no lane was started.
func TestKeyedPoolReturnsARegistrationRefusal(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reg := metric.NewMetricsRegistry()
		// A counter already holds the key of the pool's queue-depth gauge.
		_, err := metric.RegisterOrGet(reg, "keyed_dispatch", "dispatch_queue_depth/refused",
			prometheus.NewCounter(prometheus.CounterOpts{Name: "unrelated_total", Help: "holds the key"}))
		require.NoError(t, err)

		p, err := NewKeyedPool(context.Background(), KeyedConfig[string]{
			Lanes: 1, QueueDepth: 1, Name: "refused",
			KeyOf:   func(s string) string { return s },
			Process: func(context.Context, int, string) error { return nil },
		}, KeyedDeps{MetricsRegistry: reg})
		require.Error(t, err)
		assert.Nil(t, p)
	})
}

// findMetric gathers a single metric by name + pool label from the
// registry's Prometheus output.
func findMetric(t *testing.T, reg *metric.MetricsRegistry, name, pool string) *dto.Metric {
	t.Helper()
	families, err := reg.PrometheusRegistry().Gather()
	require.NoError(t, err)
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		for _, m := range f.Metric {
			for _, l := range m.Label {
				if l.GetName() == "pool" && l.GetValue() == pool {
					return m
				}
			}
		}
	}
	t.Fatalf("metric %q{pool=%q} not found", name, pool)
	return nil
}

func counterValue(t *testing.T, reg *metric.MetricsRegistry, name, pool string) float64 {
	t.Helper()
	return findMetric(t, reg, name, pool).GetCounter().GetValue()
}

func gaugeValue(t *testing.T, reg *metric.MetricsRegistry, name, pool string) float64 {
	t.Helper()
	return findMetric(t, reg, name, pool).GetGauge().GetValue()
}

func histogramCount(t *testing.T, reg *metric.MetricsRegistry, name, pool string) uint64 {
	t.Helper()
	return findMetric(t, reg, name, pool).GetHistogram().GetSampleCount()
}
