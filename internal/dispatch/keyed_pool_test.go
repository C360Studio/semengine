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

// Every test here that waits runs in a synctest bubble: a wait the pool should end but does not
// fails the test as a deadlock, with no wall-clock deadline, and the bubble returning proves no
// pool goroutine is left running.

// shutdown shuts p down under an hour's deadline on the bubble clock and requires nil.
func shutdown(t *testing.T, p *KeyedPool[string]) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()
	require.NoError(t, p.Shutdown(ctx))
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
		Process:    func(context.Context, int, string) {},
	}
}

// TestKeyedPool_ProcessUsesConstructorContext: Process is handed the exact context NewKeyedPool
// was given, and sees it end.
func TestKeyedPool_ProcessUsesConstructorContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		runCtx, cancel := context.WithCancel(context.Background())
		received := make(chan context.Context, 1)
		processDone := make(chan struct{})

		p, err := NewKeyedPool(runCtx, KeyedConfig[string]{
			Lanes:      1,
			QueueDepth: 1,
			Process: func(ctx context.Context, _ int, _ string) {
				received <- ctx
				<-ctx.Done()
				close(processDone)
			},
		})
		require.NoError(t, err)
		require.NoError(t, p.SubmitBlocking(context.Background(), "key", "work"))

		if got := <-received; got != runCtx {
			t.Fatal("Process did not receive the exact pool constructor context")
		}
		cancel()
		<-processDone
		shutdown(t, p)
	})
}

// --- Config validation (task 1.1) ---

func TestKeyedPool_RejectsNilContext(t *testing.T) {
	_, err := NewKeyedPool[string](nil, validKeyedConfig())
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
		{"nil Process", func(c *KeyedConfig[string]) { c.Process = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validKeyedConfig()
			tc.mutate(&cfg)
			_, err := NewKeyedPool(context.Background(), cfg)
			require.ErrorIs(t, err, ErrInvalidConfig)
		})
	}
}

// --- Ordering: same key serial in submit order (task 1.5) ---

func TestKeyedPool_SameKeyOrdered(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var got []string // written by the lane; read only after Shutdown's join
		p, err := NewKeyedPool(context.Background(), KeyedConfig[string]{
			Lanes:      8,
			QueueDepth: 16,
			Process: func(_ context.Context, _ int, s string) {
				got = append(got, s)
			},
		})
		require.NoError(t, err)

		want := []string{"a", "b", "c", "d", "e"}
		for _, s := range want {
			require.NoError(t, p.SubmitBlocking(context.Background(), "one-key", s)) // all → one lane
		}
		shutdown(t, p)
		assert.Equal(t, want, got, "same-key items must process in submit order")
	})
}

// --- Same key always lands on the same lane (task 1.5 / shard-safety) ---

func TestKeyedPool_SameKeySameLane(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		lanes := make(map[int]struct{}) // written by the lanes; read only after Shutdown's join
		var mu sync.Mutex
		p, err := NewKeyedPool(context.Background(), KeyedConfig[string]{
			Lanes:      8,
			QueueDepth: 16,
			Process: func(_ context.Context, lane int, _ string) {
				mu.Lock()
				lanes[lane] = struct{}{}
				mu.Unlock()
			},
		})
		require.NoError(t, err)

		// All 20 items key to one lane with a queue of 16: SubmitBlocking waits for
		// capacity; the test's concern is lane affinity, not queue sizing.
		for i := 0; i < 20; i++ {
			require.NoError(t, p.SubmitBlocking(context.Background(), "sticky", fmt.Sprintf("item-%d", i)))
		}
		shutdown(t, p)
		assert.Len(t, lanes, 1, "all same-key items must run on exactly one lane")
	})
}

// --- Concurrency: distinct keys run in parallel across lanes (task 1.5) ---

func TestKeyedPool_DifferentKeysConcurrent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const n = 4
		entered := make(chan struct{}, n)
		release := make(chan struct{})
		p, err := NewKeyedPool(context.Background(), KeyedConfig[string]{
			Lanes:      n,
			QueueDepth: 1,
			Process: func(context.Context, int, string) {
				entered <- struct{}{}
				<-release // hold the lane until every item is confirmed in-flight
			},
		})
		require.NoError(t, err)

		for _, k := range keysOnDistinctLanes(p, n) {
			require.NoError(t, p.SubmitBlocking(context.Background(), k, k))
		}
		synctest.Wait() // every lane is in Process or blocked on its queue
		require.Len(t, entered, n, "all items must be in Process at once: lanes run in parallel")
		close(release)
		shutdown(t, p)
	})
}

// --- Backpressure: a full lane blocks SubmitBlocking (task 1.5) ---

func TestKeyedPool_Backpressure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		entered := make(chan struct{})
		block := make(chan struct{})
		var processed []string // written by the lane; read only after Shutdown's join
		p, err := NewKeyedPool(context.Background(), KeyedConfig[string]{
			Lanes:      1,
			QueueDepth: 1,
			Process: func(_ context.Context, _ int, work string) {
				processed = append(processed, work)
				if work == "first" {
					close(entered)
					<-block
				}
			},
		})
		require.NoError(t, err)

		// First item is picked up by the lane goroutine and blocks in Process.
		require.NoError(t, p.SubmitBlocking(context.Background(), "k", "first"))
		<-entered // now the lane is busy and its queue is empty

		// Second fills the depth-1 queue.
		require.NoError(t, p.SubmitBlocking(context.Background(), "k", "second"))

		// A third submit on the full lane waits: once every goroutine in the bubble is blocked,
		// it has still not returned. Ending its context ends the wait with ctx.Err().
		ctx, cancel := context.WithCancel(context.Background())
		returned := make(chan error, 1)
		go func() { returned <- p.SubmitBlocking(ctx, "k", "blocked") }()
		synctest.Wait()
		select {
		case err := <-returned:
			t.Fatalf("SubmitBlocking on a full lane returned %v instead of waiting", err)
		default:
		}
		cancel()
		require.ErrorIs(t, <-returned, context.Canceled)

		close(block)
		shutdown(t, p)
		assert.Equal(t, []string{"first", "second"}, processed, "the refused item never reached Process")
	})
}

// --- Graceful drain: Shutdown finishes buffered work (task 1.5) ---

func TestKeyedPool_ShutdownDrains(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var count atomic.Int64
		p, err := NewKeyedPool(context.Background(), KeyedConfig[string]{
			Lanes:      4,
			QueueDepth: 256,
			Process: func(context.Context, int, string) {
				count.Add(1)
			},
		})
		require.NoError(t, err)

		const total = 200
		for i := 0; i < total; i++ {
			k := fmt.Sprintf("k%d", i)
			require.NoError(t, p.SubmitBlocking(context.Background(), k, k))
		}
		shutdown(t, p)
		assert.Equal(t, int64(total), count.Load(), "Shutdown must drain all buffered work")
	})
}

// --- Shutdown/submit race: an accepted submit is never stranded (Codex P1) ---

func TestKeyedPool_ShutdownSubmitRace(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var processed atomic.Int64
		p, err := NewKeyedPool(context.Background(), KeyedConfig[string]{
			Lanes:      4,
			QueueDepth: 8,
			Process: func(context.Context, int, string) {
				processed.Add(1)
			},
		})
		require.NoError(t, err)

		var acceptedNil atomic.Int64
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
					k := fmt.Sprintf("k%d-%d", id, j)
					if err := p.SubmitBlocking(context.Background(), k, k); err == nil {
						acceptedNil.Add(1)
						startOnce.Do(func() { close(started) })
					}
				}
			}(i)
		}

		<-started // ensure submits are in flight, racing with Shutdown
		shutdown(t, p)
		close(stop)
		wg.Wait()

		// Contract (Codex P1): once Shutdown begins draining, no submit returns
		// success unless that item is guaranteed processed before Shutdown completes.
		// So every nil-returning submit must have been processed — no stranded
		// accepted work.
		assert.Equal(t, acceptedNil.Load(), processed.Load(),
			"every accepted (nil) submit must be processed before Shutdown completes — no stranded work")
	})
}

// --- A submit after Shutdown is rejected ---

func TestKeyedPool_SubmitAfterShutdown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p, err := NewKeyedPool(context.Background(), validKeyedConfig())
		require.NoError(t, err)
		shutdown(t, p)
		require.ErrorIs(t, p.SubmitBlocking(context.Background(), "x", "x"), ErrStopped)
	})
}

// --- Panic recovery: lane survives, disposition invoked (task 1.5, H2) ---

func TestKeyedPool_PanicRecovery(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Written by the one lane; read only after Shutdown's join.
		var disposed []string
		var recoveredVal any
		var processedAfter int

		reg := metric.NewMetricsRegistry()
		p, err := NewKeyedPool(context.Background(), KeyedConfig[string]{
			Lanes:      1,
			QueueDepth: 16,
			Name:       "panic_pool",
			Process: func(_ context.Context, _ int, s string) {
				if s == "boom" {
					panic("kaboom")
				}
				processedAfter++
			},
			OnPanic: func(s string, r any) {
				disposed = append(disposed, s)
				recoveredVal = r
			},
			MetricsRegistry: reg,
		})
		require.NoError(t, err)

		for _, s := range []string{"boom", "after1", "after2"} {
			require.NoError(t, p.SubmitBlocking(context.Background(), "same", s)) // one lane, ordered
		}
		shutdown(t, p)

		assert.Equal(t, []string{"boom"}, disposed, "OnPanic must fire for the panicking item")
		assert.Equal(t, "kaboom", recoveredVal, "recovered value passed to OnPanic")
		assert.Equal(t, 2, processedAfter, "lane must survive the panic and process later same-lane items")
		assert.Equal(t, float64(3), counterValue(t, reg, "dispatch_completed_total", "panic_pool"),
			"panicked item still counts as completed")
	})
}

// --- Metrics move (task 2.2) ---

func TestKeyedPool_MetricsMove(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reg := metric.NewMetricsRegistry()
		p, err := NewKeyedPool(context.Background(), KeyedConfig[string]{
			Lanes:           4,
			QueueDepth:      64,
			Name:            "test_pool",
			Process:         func(context.Context, int, string) {},
			MetricsRegistry: reg,
		})
		require.NoError(t, err)

		const total = 50
		for i := 0; i < total; i++ {
			k := fmt.Sprintf("k%d", i)
			require.NoError(t, p.SubmitBlocking(context.Background(), k, k))
		}
		shutdown(t, p)

		// Confirm the Prometheus objects actually moved.
		assert.Equal(t, float64(total), counterValue(t, reg, "dispatch_submitted_total", "test_pool"))
		assert.Equal(t, float64(total), counterValue(t, reg, "dispatch_completed_total", "test_pool"),
			"every submitted item completes")
		assert.Equal(t, float64(0), gaugeValue(t, reg, "dispatch_inflight", "test_pool"),
			"inflight returns to zero after drain")
		assert.Equal(t, uint64(total), histogramCount(t, reg, "dispatch_queue_wait_seconds", "test_pool"))
		assert.Equal(t, uint64(total), histogramCount(t, reg, "dispatch_processing_duration_seconds", "test_pool"))
	})
}

// TestKeyedPoolWritesTheRegisteredCollector: two pools with one name on one registry share each
// collector (design D4). The second pool writes the collector metric.RegisterOrGet returned, not
// its own candidate, so the registry gathers both pools' work.
func TestKeyedPoolWritesTheRegisteredCollector(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reg := metric.NewMetricsRegistry()
		runPool := func(items int) {
			p, err := NewKeyedPool(context.Background(), KeyedConfig[string]{
				Lanes: 2, QueueDepth: 8, Name: "shared",
				Process:         func(context.Context, int, string) {},
				MetricsRegistry: reg,
			})
			require.NoError(t, err)
			for i := 0; i < items; i++ {
				k := fmt.Sprintf("k%d", i)
				require.NoError(t, p.SubmitBlocking(context.Background(), k, k))
			}
			shutdown(t, p)
		}
		runPool(2)
		runPool(3)

		assert.Equal(t, float64(5), counterValue(t, reg, "dispatch_submitted_total", "shared"))
		assert.Equal(t, float64(5), counterValue(t, reg, "dispatch_completed_total", "shared"))
		assert.Equal(t, uint64(5), histogramCount(t, reg, "dispatch_queue_wait_seconds", "shared"))
		assert.Equal(t, uint64(5), histogramCount(t, reg, "dispatch_processing_duration_seconds", "shared"))
	})
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
			Process:         func(context.Context, int, string) {},
			MetricsRegistry: reg,
		})
		require.Error(t, err)
		assert.Nil(t, p)
	})
}

// blockedPool starts a one-lane pool named name on reg whose Process holds the first item until
// release is closed, submits first and then waiting more items, and returns once those wait in
// the lane's queue. Call inside a synctest bubble.
func blockedPool(t *testing.T, reg *metric.MetricsRegistry, name string, waiting int, release <-chan struct{}) *KeyedPool[string] {
	t.Helper()
	entered := make(chan struct{})
	p, err := NewKeyedPool(context.Background(), KeyedConfig[string]{
		Lanes: 1, QueueDepth: waiting + 1, Name: name,
		Process: func(_ context.Context, _ int, work string) {
			if work == "first" {
				close(entered)
				<-release
			}
		},
		MetricsRegistry: reg,
	})
	require.NoError(t, err)
	require.NoError(t, p.SubmitBlocking(context.Background(), "key", "first"))
	<-entered
	for i := 0; i < waiting; i++ {
		require.NoError(t, p.SubmitBlocking(context.Background(), "key", fmt.Sprintf("waiting-%d", i)))
	}
	return p
}

// shutdownPool releases a blockedPool and shuts it down under a context with an hour's deadline.
func shutdownPool(t *testing.T, p *KeyedPool[string], release chan struct{}) {
	t.Helper()
	close(release)
	shutdown(t, p)
}

// TestKeyedPoolQueueDepthCountsWaitingItems: the queue-depth gauge counts the items accepted and
// not yet passed to Process, as soon as they are accepted, with no clock time passing; the item
// in Process is not counted. Once Shutdown has drained the pool, the gauge reads 0.
func TestKeyedPoolQueueDepthCountsWaitingItems(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reg := metric.NewMetricsRegistry()
		release := make(chan struct{})
		p := blockedPool(t, reg, "depth", 3, release)

		assert.Equal(t, float64(3), gaugeValue(t, reg, "dispatch_queue_depth", "depth"),
			"three items wait behind the one in Process")
		shutdownPool(t, p, release)
		assert.Equal(t, float64(0), gaugeValue(t, reg, "dispatch_queue_depth", "depth"),
			"nothing waits once Shutdown has drained the pool")
	})
}

// TestKeyedPoolQueueDepthSumsPoolsOfOneName: two pools of one name on one registry share the
// queue-depth gauge, which reads their combined waiting items; when one is drained the other's
// count stays. A replacement pool of that name, made after both, counts only its own items: a
// gauge that read the first pool's queues would read 0 here.
func TestKeyedPoolQueueDepthSumsPoolsOfOneName(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reg := metric.NewMetricsRegistry()
		depth := func() float64 { return gaugeValue(t, reg, "dispatch_queue_depth", "shared") }

		releaseA, releaseB := make(chan struct{}), make(chan struct{})
		a := blockedPool(t, reg, "shared", 2, releaseA)
		b := blockedPool(t, reg, "shared", 3, releaseB)
		assert.Equal(t, float64(5), depth(), "two waiting in one pool and three in the other")

		shutdownPool(t, a, releaseA)
		assert.Equal(t, float64(3), depth(), "the remaining pool's three still wait")
		shutdownPool(t, b, releaseB)
		assert.Equal(t, float64(0), depth(), "both pools drained")

		releaseC := make(chan struct{})
		c := blockedPool(t, reg, "shared", 4, releaseC)
		assert.Equal(t, float64(4), depth(), "the replacement pool counts its own waiting items")
		shutdownPool(t, c, releaseC)
		assert.Equal(t, float64(0), depth(), "all pools drained")
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
