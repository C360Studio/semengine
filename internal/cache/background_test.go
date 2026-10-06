package cache

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Background-work shapes (design D7; the background-work capability). Each test that starts work
// runs inside synctest.Test: the bubble returns only once every goroutine started in it has
// exited, so a test that returns proves nothing was left running. The work is started with a
// context only the failure path cancels, never t.Context(): the bubble cancels t.Context() before
// it waits for its goroutines, which would end a goroutine the stop under test failed to end.

// requireJoined calls stop and requires that it returns nil within an hour of the bubble's clock
// with the work's goroutine gone (exited is closed by that goroutine as it returns). A goroutine
// left on its ticker would otherwise keep the bubble's clock running and hang the test, so on
// failure the work's context is cancelled (release) and the test fails instead.
func requireJoined(t *testing.T, stop func() error, exited <-chan struct{}, release context.CancelFunc) {
	t.Helper()
	type result struct {
		err    error
		joined bool
	}
	returned := make(chan result, 1)
	go func() {
		err := stop()
		// Read the moment stop returns, before anything else in this goroutine can yield.
		select {
		case <-exited:
			returned <- result{err, true}
		default:
			returned <- result{err, false}
		}
	}()
	select {
	case r := <-returned:
		if !r.joined {
			release()
			t.Fatal("stop returned while the background goroutine was still running")
		}
		require.NoError(t, r.err)
	case <-time.After(time.Hour):
		release()
		<-returned
		t.Fatal("stop did not return within an hour of the bubble's clock")
	}
}

// CoalescingSet takes shape 3: its goroutine runs the caller's callback, so stopping it waits on
// work the set does not control, and Shutdown(ctx) bounds that wait by the caller's context.

func TestCoalescingSetShutdownLeavesNothingRunning(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, release := context.WithCancel(context.Background())
		delivered := make(chan []string, 1)
		set := newTestCoalescingSet(ctx, t, 50*time.Millisecond, func(keys []string) {
			delivered <- keys
		})
		set.Add("entity-1")
		assert.Equal(t, []string{"entity-1"}, <-delivered)

		requireJoined(t, func() error {
			stopCtx, cancel := context.WithTimeout(context.Background(), time.Hour)
			defer cancel()
			return set.Shutdown(stopCtx)
		}, set.done, release)
	})
}

// TestCoalescingSetNilContextRefusedAtTheCall: NewCoalescingSet returns an error, so a nil context
// is refused with one, as NewTTL refuses it (design D7: an error where the entry returns one), and
// no goroutine starts.
func TestCoalescingSetNilContextRefusedAtTheCall(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var fired bool
		var nilCtx context.Context // the nil context is the input under test
		set, err := NewCoalescingSet(nilCtx, time.Millisecond, func([]string) { fired = true })
		assert.True(t, errs.IsInvalid(err), "a nil context is an invalid argument: %v", err)
		assert.Nil(t, set)
		// Had a goroutine started, it would still be running here and the bubble would not return;
		// the clock is moved past several windows to give it every chance to run.
		<-time.After(10 * time.Millisecond)
		synctest.Wait()
		assert.False(t, fired, "no callback ran")
	})
}

func TestCoalescingSetShutdownUnderABlockedCallback(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := make(chan struct{})
		release := make(chan struct{})
		returned := make(chan struct{})
		set := newTestCoalescingSet(context.Background(), t, 50*time.Millisecond, func([]string) {
			close(started)
			<-release
			close(returned)
		})
		set.Add("entity-1")
		<-started

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		err := set.Shutdown(ctx)
		require.ErrorIs(t, err, context.DeadlineExceeded, "Shutdown returns ctx.Err() while the callback blocks")
		select {
		case <-returned:
			t.Fatal("the callback returned before it was released")
		default:
		}

		close(release)
		<-returned
		require.NoError(t, set.Shutdown(context.Background()), "a later Shutdown returns nil")
	})
}

func TestCoalescingSetShutdownRefusesNilContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		set := newTestCoalescingSet(context.Background(), t, time.Hour, func([]string) {})
		var nilCtx context.Context // the nil context is the input under test
		require.Error(t, set.Shutdown(nilCtx))
		require.NoError(t, set.Shutdown(context.Background()))
	})
}

// The TTL and hybrid caches take shape 2: their cleanup goroutine waits only on its own ticker and
// shutdown channel, so Close cancels and joins with no fixed wait.

func TestTTLCacheCloseLeavesNothingRunning(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, release := context.WithCancel(context.Background())
		c, err := NewTTL[string](ctx, 100*time.Millisecond, 50*time.Millisecond)
		require.NoError(t, err)
		_, err = c.Set("k", "v")
		require.NoError(t, err)
		// Past the TTL and a cleanup tick: the cleanup goroutine has removed the entry.
		<-time.After(200 * time.Millisecond)
		synctest.Wait()
		assert.Zero(t, c.Size())

		requireJoined(t, c.Close, c.(*ttlCache[string]).done, release)
	})
}

func TestHybridCacheCloseLeavesNothingRunning(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, release := context.WithCancel(context.Background())
		c, err := NewFromConfig[string](ctx, Config{
			Enabled: true, Strategy: StrategyHybrid, MaxSize: 10,
			TTL: 100 * time.Millisecond, CleanupInterval: 50 * time.Millisecond,
		})
		require.NoError(t, err)
		_, err = c.Set("k", "v")
		require.NoError(t, err)
		<-time.After(200 * time.Millisecond)
		synctest.Wait()
		assert.Zero(t, c.Size())

		requireJoined(t, c.Close, c.(*hybridCache[string]).done, release)
	})
}

func TestCacheConstructorsRefuseNilContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var nilCtx context.Context // the nil context is the input under test
		ttl, err := NewTTL[string](nilCtx, time.Minute, time.Second)
		require.Error(t, err, "NewTTL")
		assert.True(t, ttl == nil, "NewTTL returns a nil Cache, not a nil *ttlCache inside one")

		for _, cfg := range []Config{
			{Enabled: true, Strategy: StrategyTTL, TTL: time.Minute, CleanupInterval: time.Second},
			{Enabled: true, Strategy: StrategyHybrid, MaxSize: 10, TTL: time.Minute, CleanupInterval: time.Second},
			{Enabled: true, Strategy: StrategySimple},
			{Enabled: false},
		} {
			c, err := NewFromConfig[string](nilCtx, cfg)
			require.Error(t, err, "NewFromConfig %+v", cfg)
			assert.True(t, c == nil, "NewFromConfig %+v returns a nil Cache", cfg)
		}
		// Had a cleanup goroutine started, the bubble would not return.
	})
}

// TestCacheCloseAfterContextEndAndAgain: the cleanup goroutine also ends with its context; a Close
// after that, and a second Close, return nil at once.
func TestCacheCloseAfterContextEndAndAgain(t *testing.T) {
	for _, strategy := range []Strategy{StrategyTTL, StrategyHybrid} {
		t.Run(string(strategy), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				c, err := NewFromConfig[string](ctx, Config{
					Enabled: true, Strategy: strategy, MaxSize: 10,
					TTL: time.Minute, CleanupInterval: time.Second,
				})
				require.NoError(t, err)
				cancel()
				synctest.Wait()
				require.NoError(t, c.Close(), "Close after the context ended")
				require.NoError(t, c.Close(), "a second Close")
			})
		})
	}
}

// TestCacheConstructorsReturnNilCacheOnError: every constructor that returns Cache[V] and an error
// returns a nil Cache with the error, never a nil pointer inside a non-nil interface, which a
// caller checking c != nil would take for a cache. The error is a metrics registration refusal:
// the registry already holds a collector named semengine_cache_hits_total with another help.
func TestCacheConstructorsReturnNilCacheOnError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		registry := metric.NewMetricsRegistry()
		require.NoError(t, registry.PrometheusRegistry().Register(prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "semengine", Subsystem: "cache", Name: "hits_total",
			ConstLabels: prometheus.Labels{"component": "taken"}, Help: "another help",
		})))
		opt := WithMetrics[string](registry, "taken")
		ctx := context.Background()

		constructors := map[string]func() (Cache[string], error){
			"NewSimple": func() (Cache[string], error) { return NewSimple[string](opt) },
			"NewLRU":    func() (Cache[string], error) { return NewLRU[string](10, opt) },
			"NewTTL":    func() (Cache[string], error) { return NewTTL[string](ctx, time.Minute, time.Second, opt) },
			"newHybrid": func() (Cache[string], error) {
				return newHybrid[string](ctx, 10, time.Minute, time.Second, opt)
			},
		}
		for _, cfg := range []Config{
			{Enabled: true, Strategy: StrategySimple},
			{Enabled: true, Strategy: StrategyLRU, MaxSize: 10},
			{Enabled: true, Strategy: StrategyTTL, TTL: time.Minute, CleanupInterval: time.Second},
			{Enabled: true, Strategy: StrategyHybrid, MaxSize: 10, TTL: time.Minute, CleanupInterval: time.Second},
		} {
			constructors["NewFromConfig/"+string(cfg.Strategy)] = func() (Cache[string], error) {
				return NewFromConfig[string](ctx, cfg, opt)
			}
		}
		for name, construct := range constructors {
			c, err := construct()
			assert.Error(t, err, name)
			assert.True(t, c == nil, "%s returns a nil Cache with its error, got %#v", name, c)
		}
	})
}

// TestCacheConcurrentCloseJoinsEveryCaller: Close is safe from many goroutines at once (cache.go
// promises thread safety), and every call returns only once the cleanup goroutine has exited.
// The oracle is the goroutine's own done channel, read the moment each Close returns; a close of
// an already closed channel panics and fails the run. Rounds repeat because the race between the
// callers' check and close is narrow (Codex's probe: 73 TTL and 85 hybrid panics in 2,000 rounds
// of 32 callers at a7dbf9d).
func TestCacheConcurrentCloseJoinsEveryCaller(t *testing.T) {
	const rounds, callers = 2000, 32
	for _, strategy := range []Strategy{StrategyTTL, StrategyHybrid} {
		t.Run(string(strategy), func(t *testing.T) {
			for range rounds {
				c, err := NewFromConfig[int](context.Background(), Config{
					Enabled: true, Strategy: strategy, MaxSize: 10,
					TTL: time.Minute, CleanupInterval: time.Second,
				})
				require.NoError(t, err)
				var done <-chan struct{}
				switch cc := c.(type) {
				case *ttlCache[int]:
					done = cc.done
				case *hybridCache[int]:
					done = cc.done
				}
				start := make(chan struct{})
				results := make(chan string, callers)
				for range callers {
					go func() {
						<-start
						err := c.Close()
						select {
						case <-done:
							results <- fmt.Sprint(err)
						default:
							results <- "returned before the cleanup goroutine exited"
						}
					}()
				}
				close(start)
				for range callers {
					if r := <-results; r != "<nil>" {
						t.Fatalf("Close: %s", r)
					}
				}
			}
		})
	}
}

// bubbleGoroutinesRunning counts goroutines in a synctest bubble whose stack holds any of
// frames, read from the runtime's own stack dump.
func bubbleGoroutinesRunning(frames ...string) int {
	buf := make([]byte, 1<<20)
	buf = buf[:runtime.Stack(buf, true)]
	n := 0
	for _, g := range strings.Split(string(buf), "\n\n") {
		if !strings.Contains(g, "synctest bubble") {
			continue
		}
		for _, f := range frames {
			if strings.Contains(g, f) {
				n++
				break
			}
		}
	}
	return n
}

// bubbleCleanupGoroutines counts goroutines in a synctest bubble running a TTL or hybrid
// cleanup loop.
func bubbleCleanupGoroutines() int {
	return bubbleGoroutinesRunning("(*ttlCache[...]).cleanup", "(*hybridCache[...]).cleanup")
}

// TestCacheConstructorsRefuseInvalidDimensions: NewLRU, NewTTL and the hybrid constructor apply
// the dimension checks Config.Validate makes for NewFromConfig, before constructing anything: a
// zero or negative size, TTL or cleanup interval returns a classified invalid error, a nil Cache,
// and starts no cleanup goroutine (read from the runtime's stack dump inside the bubble).
func TestCacheConstructorsRefuseInvalidDimensions(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		cases := map[string]func() (Cache[int], error){}
		for _, bad := range []int{0, -1} {
			d := time.Duration(bad) * time.Second
			cases[fmt.Sprintf("NewLRU size %d", bad)] = func() (Cache[int], error) { return NewLRU[int](bad) }
			cases[fmt.Sprintf("NewTTL ttl %v", d)] = func() (Cache[int], error) { return NewTTL[int](ctx, d, time.Second) }
			cases[fmt.Sprintf("NewTTL cleanup %v", d)] = func() (Cache[int], error) { return NewTTL[int](ctx, time.Minute, d) }
			cases[fmt.Sprintf("hybrid size %d", bad)] = func() (Cache[int], error) {
				return newHybrid[int](ctx, bad, time.Minute, time.Second)
			}
			cases[fmt.Sprintf("hybrid ttl %v", d)] = func() (Cache[int], error) {
				return newHybrid[int](ctx, 10, d, time.Second)
			}
			cases[fmt.Sprintf("hybrid cleanup %v", d)] = func() (Cache[int], error) {
				return newHybrid[int](ctx, 10, time.Minute, d)
			}
		}
		var accepted []Cache[int]
		for name, construct := range cases {
			c, err := construct()
			assert.True(t, errs.IsInvalid(err), "%s: want a classified invalid error, got %v", name, err)
			assert.True(t, c == nil, "%s: want a nil Cache, got %T", name, c)
			if c != nil {
				accepted = append(accepted, c)
			}
		}
		synctest.Wait()
		assert.Zero(t, bubbleCleanupGoroutines(), "a refused constructor started a cleanup goroutine")
		for _, c := range accepted {
			_ = c.Close()
		}
	})
}
