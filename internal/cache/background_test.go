package cache

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

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
		set := NewCoalescingSet(ctx, 50*time.Millisecond, func(keys []string) {
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

func TestCoalescingSetNilContextPanicsAtTheCall(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var fired bool
		var nilCtx context.Context // the nil context is the input under test
		assert.Panics(t, func() { NewCoalescingSet(nilCtx, time.Millisecond, func([]string) { fired = true }) })
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
		set := NewCoalescingSet(context.Background(), 50*time.Millisecond, func([]string) {
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
		set := NewCoalescingSet(context.Background(), time.Hour, nil)
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
