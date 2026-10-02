// Written for SemStreams by its Tester Agent (CoalescingSet: windowed batch deduplication for
// async coordination). Repaired for SemEngine (design D8, R1a): every test runs inside a synctest
// bubble, where the set's ticker uses a fake clock that advances only when every goroutine in the
// bubble is blocked. A test moves the clock with <-time.After and settles the bubble with
// synctest.Wait before it asserts; t.Parallel cannot be called inside a bubble, so it is gone.

package cache

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCoalescingSet_AddCollectsKeys verifies that adding keys accumulates them in the pending set.
func TestCoalescingSet_AddCollectsKeys(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		var callbackFired atomic.Bool
		set := NewCoalescingSet(ctx, 100*time.Millisecond, func(keys []string) {
			callbackFired.Store(true)
		})
		defer set.Close()

		// Add multiple keys
		set.Add("entity-1")
		set.Add("entity-2")
		set.Add("entity-3")

		// Verify keys are collected (callback hasn't fired yet)
		require.False(t, callbackFired.Load(), "callback should not fire before window expires")
	})
}

// TestCoalescingSet_DeduplicatesKeys verifies that the same key added multiple times appears once in batch.
func TestCoalescingSet_DeduplicatesKeys(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		callbackCh := make(chan []string, 1)
		set := NewCoalescingSet(ctx, 50*time.Millisecond, func(keys []string) {
			callbackCh <- keys
		})
		defer set.Close()

		// Add the same key multiple times
		set.Add("entity-1")
		set.Add("entity-1")
		set.Add("entity-1")
		set.Add("entity-2")
		set.Add("entity-2")

		// Wait for callback
		select {
		case keys := <-callbackCh:
			require.Len(t, keys, 2, "should deduplicate to 2 unique keys")
			keySet := make(map[string]bool)
			for _, k := range keys {
				keySet[k] = true
			}
			assert.True(t, keySet["entity-1"], "entity-1 should be present")
			assert.True(t, keySet["entity-2"], "entity-2 should be present")
		case <-time.After(1 * time.Second):
			t.Fatal("callback did not fire within timeout")
		}
	})
}

// TestCoalescingSet_CallbackFiresAfterWindow verifies callback fires with collected keys after window.
func TestCoalescingSet_CallbackFiresAfterWindow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		callbackCh := make(chan []string, 1)
		windowDuration := 50 * time.Millisecond

		set := NewCoalescingSet(ctx, windowDuration, func(keys []string) {
			callbackCh <- keys
		})
		defer set.Close()

		// Add keys
		set.Add("entity-1")
		set.Add("entity-2")

		// Callback should not fire immediately: 10 ms into the bubble's clock, with every goroutine
		// settled, nothing has been delivered (design D8, the coalescing_set_test.go:92 row).
		<-time.After(10 * time.Millisecond)
		synctest.Wait()
		select {
		case <-callbackCh:
			t.Fatal("callback fired too early")
		default:
		}

		// Wait for window to expire
		select {
		case keys := <-callbackCh:
			require.NotEmpty(t, keys, "callback should receive keys")
			keySet := make(map[string]bool)
			for _, k := range keys {
				keySet[k] = true
			}
			assert.True(t, keySet["entity-1"])
			assert.True(t, keySet["entity-2"])
		case <-time.After(2 * windowDuration):
			t.Fatal("callback did not fire within expected window")
		}
	})
}

// TestCoalescingSet_BatchCleared verifies that after callback, set is empty for next window.
func TestCoalescingSet_BatchCleared(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		var callCount atomic.Int32
		var firstBatch, secondBatch []string
		var mu sync.Mutex

		set := NewCoalescingSet(ctx, 50*time.Millisecond, func(keys []string) {
			count := callCount.Add(1)
			mu.Lock()
			defer mu.Unlock()
			if count == 1 {
				firstBatch = keys
			} else if count == 2 {
				secondBatch = keys
			}
		})
		defer set.Close()

		// First window - add keys
		set.Add("entity-1")
		set.Add("entity-2")

		// Wait for first callback
		<-time.After(100 * time.Millisecond)
		synctest.Wait()

		// Verify first batch received. The batch is read under the lock and asserted after it is
		// released: at the pin a failed require here held the lock, the next callback blocked on
		// it, and the deferred Close waited forever.
		mu.Lock()
		first := firstBatch
		mu.Unlock()
		require.Len(t, first, 2, "first batch should have 2 keys")

		// Second window - add different key
		set.Add("entity-3")

		// Wait for second callback
		<-time.After(100 * time.Millisecond)
		synctest.Wait()

		// Verify second batch has only the new key (batch was cleared)
		mu.Lock()
		second := secondBatch
		mu.Unlock()
		require.Len(t, second, 1, "second batch should have only 1 key (batch was cleared)")
		assert.Equal(t, "entity-3", second[0])
	})
}

// TestCoalescingSet_RemoveExcludesFromBatch verifies removed keys don't appear in callback.
func TestCoalescingSet_RemoveExcludesFromBatch(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		callbackCh := make(chan []string, 1)
		set := NewCoalescingSet(ctx, 50*time.Millisecond, func(keys []string) {
			callbackCh <- keys
		})
		defer set.Close()

		// Add keys
		set.Add("entity-1")
		set.Add("entity-2")
		set.Add("entity-3")

		// Remove one key before window expires
		set.Remove("entity-2")

		// Wait for callback
		select {
		case keys := <-callbackCh:
			require.Len(t, keys, 2, "should have 2 keys after removal")
			keySet := make(map[string]bool)
			for _, k := range keys {
				keySet[k] = true
			}
			assert.True(t, keySet["entity-1"])
			assert.False(t, keySet["entity-2"], "removed key should not be present")
			assert.True(t, keySet["entity-3"])
		case <-time.After(1 * time.Second):
			t.Fatal("callback did not fire")
		}
	})
}

func TestCoalescingSet_RemovePrefixExcludesMatchingKeys(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		received := make(chan []string, 1)
		set := NewCoalescingSet(ctx, 50*time.Millisecond, func(keys []string) {
			received <- keys
		})
		defer func() { require.NoError(t, set.Close()) }()

		set.Add("entity-1\x00watch-a\x001")
		set.Add("entity-1\x00watch-b\x002")
		set.Add("entity-10\x00watch-a\x001")
		set.RemovePrefix("entity-1\x00")

		select {
		case keys := <-received:
			require.ElementsMatch(t, []string{"entity-10\x00watch-a\x001"}, keys)
		case <-time.After(500 * time.Millisecond):
			t.Fatal("timed out waiting for coalesced batch")
		}
	})
}

func TestCoalescingSet_DrainReturnsAndClearsPendingKeys(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		set := NewCoalescingSet(context.Background(), time.Hour, nil)
		set.Add("entity-1")
		set.Add("entity-2")
		require.ElementsMatch(t, []string{"entity-1", "entity-2"}, set.Drain())
		require.Zero(t, set.PendingCount())
		require.Empty(t, set.Drain())
		require.NoError(t, set.Close())
	})
}

func TestCoalescingSet_MutationResultsTrackPendingOwnership(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		set := NewCoalescingSet(context.Background(), time.Hour, nil)
		require.True(t, set.Add("entity-1"))
		require.False(t, set.Add("entity-1"))
		require.False(t, set.Remove("missing"))
		require.True(t, set.Remove("entity-1"))
		require.True(t, set.Add("entity-1\x00watch-a"))
		require.True(t, set.Add("entity-1\x00watch-b"))
		require.Equal(t, 2, set.RemovePrefix("entity-1\x00"))
		require.Zero(t, set.PendingCount())
		require.NoError(t, set.Close())
	})
}

// TestCoalescingSet_EmptyBatchNoCallback verifies no callback (or empty slice) when no keys collected.
func TestCoalescingSet_EmptyBatchNoCallback(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		var callbackFired atomic.Bool
		callbackCh := make(chan []string, 10)

		set := NewCoalescingSet(ctx, 30*time.Millisecond, func(keys []string) {
			callbackFired.Store(true)
			callbackCh <- keys
		})
		defer set.Close()

		// Don't add any keys, just wait for multiple windows
		<-time.After(150 * time.Millisecond)
		synctest.Wait()

		// Implementation may choose to not call callback OR call with empty slice
		// Both are acceptable behaviors - verify consistency
		if callbackFired.Load() {
			// If callback was called, it should have empty slices
			close(callbackCh)
			for keys := range callbackCh {
				assert.Empty(t, keys, "callback should receive empty slice when no keys")
			}
		}
		// If callback was never called, that's also acceptable
	})
}

// TestCoalescingSet_ZeroWindow verifies immediate firing with zero window duration.
func TestCoalescingSet_ZeroWindow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		callbackCh := make(chan []string, 1)

		set := NewCoalescingSet(ctx, 0, func(keys []string) {
			callbackCh <- keys
		})
		defer set.Close()

		set.Add("entity-1")

		// With zero window, callback should fire very quickly
		select {
		case keys := <-callbackCh:
			require.NotEmpty(t, keys, "should receive keys")
			assert.Contains(t, keys, "entity-1")
		case <-time.After(100 * time.Millisecond):
			t.Fatal("callback did not fire quickly with zero window")
		}
	})
}

// TestCoalescingSet_CloseStopsCallback verifies after Close(), no more callbacks fire.
func TestCoalescingSet_CloseStopsCallback(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		var callCount atomic.Int32

		set := NewCoalescingSet(ctx, 30*time.Millisecond, func(keys []string) {
			callCount.Add(1)
		})

		// Add keys and wait for first callback
		set.Add("entity-1")
		<-time.After(80 * time.Millisecond)
		synctest.Wait()

		initialCount := callCount.Load()
		require.Greater(t, initialCount, int32(0), "should have at least one callback before close")

		// Close the set
		err := set.Close()
		require.NoError(t, err, "Close should not error")

		// Wait and verify no more callbacks
		<-time.After(150 * time.Millisecond)
		synctest.Wait()
		finalCount := callCount.Load()

		// Count should not increase after Close
		assert.Equal(t, initialCount, finalCount, "no new callbacks should fire after Close()")
	})
}

// TestCoalescingSet_ContextCancellation verifies callback stops when context cancelled.
func TestCoalescingSet_ContextCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		var callCount atomic.Int32

		set := NewCoalescingSet(ctx, 30*time.Millisecond, func(keys []string) {
			callCount.Add(1)
		})
		defer set.Close()

		// Add keys and wait for first callback
		set.Add("entity-1")
		<-time.After(80 * time.Millisecond)
		synctest.Wait()

		initialCount := callCount.Load()
		require.Greater(t, initialCount, int32(0), "should have at least one callback before cancellation")

		// Cancel context
		cancel()

		// Wait and verify no more callbacks
		<-time.After(150 * time.Millisecond)
		synctest.Wait()
		finalCount := callCount.Load()

		// Count should not increase significantly after cancellation
		// (allow for one in-flight callback)
		assert.LessOrEqual(t, finalCount, initialCount+1, "callbacks should stop after context cancellation")
	})
}

// TestCoalescingSet_ConcurrentAdds verifies many goroutines adding keys safely.
func TestCoalescingSet_ConcurrentAdds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		var receivedKeys atomic.Value // Will store []string
		doneCh := make(chan struct{})

		set := NewCoalescingSet(ctx, 100*time.Millisecond, func(keys []string) {
			receivedKeys.Store(keys)
			close(doneCh)
		})
		defer set.Close()

		// Many goroutines adding keys concurrently
		const numGoroutines = 50
		const keysPerGoroutine = 20

		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < keysPerGoroutine; j++ {
					// Each goroutine adds some unique and some duplicate keys
					if j%2 == 0 {
						set.Add("shared-key")
					} else {
						set.Add("unique-key")
					}
				}
			}(i)
		}

		wg.Wait()

		// Wait for callback
		select {
		case <-doneCh:
			keys := receivedKeys.Load().([]string)
			require.NotEmpty(t, keys, "should receive keys")
			// Should have deduplicated the shared keys
			keySet := make(map[string]bool)
			for _, k := range keys {
				keySet[k] = true
			}
			// At minimum we should have the shared-key
			assert.True(t, keySet["shared-key"], "should have shared-key")
		case <-time.After(2 * time.Second):
			t.Fatal("callback did not fire")
		}
	})
}

// TestCoalescingSet_AddDuringCallback verifies keys added during callback go to next batch.
func TestCoalescingSet_AddDuringCallback(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		var callCount atomic.Int32
		var firstBatch, secondBatch []string
		var mu sync.Mutex
		callbackStarted := make(chan struct{})

		set := NewCoalescingSet(ctx, 50*time.Millisecond, func(keys []string) {
			count := callCount.Add(1)

			mu.Lock()
			if count == 1 {
				firstBatch = keys
				close(callbackStarted)
				// Simulate some processing time
				<-time.After(20 * time.Millisecond)
			} else if count == 2 {
				secondBatch = keys
			}
			mu.Unlock()
		})
		defer set.Close()

		// First window
		set.Add("entity-1")

		// Wait for callback to start
		<-callbackStarted

		// Add key during callback execution
		set.Add("entity-2")

		// Wait for second callback
		<-time.After(150 * time.Millisecond)
		synctest.Wait()

		// Verify batches, read under the lock and asserted after it is released
		mu.Lock()
		first, second := firstBatch, secondBatch
		mu.Unlock()
		require.NotEmpty(t, first, "first batch should have keys")
		require.NotEmpty(t, second, "second batch should have the key added during first callback")

		// First batch should only have entity-1
		assert.Len(t, first, 1)
		assert.Equal(t, "entity-1", first[0])

		// Second batch should only have entity-2
		assert.Len(t, second, 1)
		assert.Equal(t, "entity-2", second[0])
	})
}

// TestCoalescingSet_RapidUpdates verifies flood of updates coalesces properly.
func TestCoalescingSet_RapidUpdates(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		callbackCh := make(chan []string, 1)

		set := NewCoalescingSet(ctx, 100*time.Millisecond, func(keys []string) {
			callbackCh <- keys
		})
		defer set.Close()

		// Rapidly add the same keys many times
		const iterations = 1000
		for i := 0; i < iterations; i++ {
			set.Add("entity-1")
			set.Add("entity-2")
			set.Add("entity-3")
		}

		// Wait for callback
		select {
		case keys := <-callbackCh:
			// Should deduplicate to exactly 3 keys despite 3000 additions
			require.Len(t, keys, 3, "should deduplicate rapid updates")
			keySet := make(map[string]bool)
			for _, k := range keys {
				keySet[k] = true
			}
			assert.True(t, keySet["entity-1"])
			assert.True(t, keySet["entity-2"])
			assert.True(t, keySet["entity-3"])
		case <-time.After(2 * time.Second):
			t.Fatal("callback did not fire")
		}
	})
}

// TestCoalescingSet_EntityUpdateScenario simulates entity watcher pattern.
func TestCoalescingSet_EntityUpdateScenario(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		callbackCh := make(chan []string, 1)

		set := NewCoalescingSet(ctx, 100*time.Millisecond, func(keys []string) {
			callbackCh <- keys
		})
		defer set.Close()

		// Simulate real-world entity update patterns
		// Entity A updated 5 times rapidly
		for i := 0; i < 5; i++ {
			set.Add("entity-A")
			<-time.After(5 * time.Millisecond)
		}

		// Entity B updated 3 times
		for i := 0; i < 3; i++ {
			set.Add("entity-B")
			<-time.After(5 * time.Millisecond)
		}

		// Entity C updated then deleted before window expires
		set.Add("entity-C")
		<-time.After(10 * time.Millisecond)
		set.Remove("entity-C") // Entity deleted

		// Wait for callback
		select {
		case keys := <-callbackCh:
			// Should receive only A and B (C was removed)
			require.Len(t, keys, 2, "should have A and B, not C (was removed)")
			keySet := make(map[string]bool)
			for _, k := range keys {
				keySet[k] = true
			}
			assert.True(t, keySet["entity-A"], "entity-A should be present")
			assert.True(t, keySet["entity-B"], "entity-B should be present")
			assert.False(t, keySet["entity-C"], "entity-C should not be present (was removed)")
		case <-time.After(2 * time.Second):
			t.Fatal("callback did not fire")
		}
	})
}

// TestCoalescingSet_MultipleCloseCalls verifies Close() is idempotent.
func TestCoalescingSet_MultipleCloseCalls(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		set := NewCoalescingSet(ctx, 50*time.Millisecond, func(keys []string) {})

		// First close
		err := set.Close()
		require.NoError(t, err)

		// Second close should not panic or error
		err = set.Close()
		require.NoError(t, err)

		// Third close
		err = set.Close()
		require.NoError(t, err)
	})
}

// TestCoalescingSet_CallbackPanic verifies system handles panics in callback gracefully.
func TestCoalescingSet_CallbackPanic(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		var callCount atomic.Int32

		set := NewCoalescingSet(ctx, 50*time.Millisecond, func(keys []string) {
			count := callCount.Add(1)
			if count == 1 {
				panic("intentional panic in callback")
			}
		})
		defer set.Close()

		// Add keys to trigger first callback (which panics)
		set.Add("entity-1")

		// Wait for first window
		<-time.After(100 * time.Millisecond)
		synctest.Wait()

		// Add keys for second window
		set.Add("entity-2")

		// Wait for second window
		<-time.After(100 * time.Millisecond)
		synctest.Wait()

		// System should recover and continue processing: the second window's batch reached the
		// callback, so the goroutine survived the first callback's panic. (At the pin this test
		// asserted nothing; the count is the observation that the goroutine is still running.)
		assert.Equal(t, int32(2), callCount.Load(), "the batch after the panic is delivered")
	})
}

// TestCoalescingSet_NilCallback verifies behavior with nil callback.
func TestCoalescingSet_NilCallback(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()

		// Should not panic with nil callback
		require.NotPanics(t, func() {
			set := NewCoalescingSet(ctx, 50*time.Millisecond, nil)
			defer set.Close()

			set.Add("entity-1")
			<-time.After(100 * time.Millisecond)
		})
	})
}

// TestCoalescingSet_RemoveNonexistentKey verifies removing non-existent key is safe.
func TestCoalescingSet_RemoveNonexistentKey(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		callbackCh := make(chan []string, 1)

		set := NewCoalescingSet(ctx, 50*time.Millisecond, func(keys []string) {
			callbackCh <- keys
		})
		defer set.Close()

		// Add keys
		set.Add("entity-1")
		set.Add("entity-2")

		// Remove non-existent key (should be no-op)
		set.Remove("entity-999")

		// Wait for callback
		select {
		case keys := <-callbackCh:
			require.Len(t, keys, 2, "removing non-existent key should not affect batch")
			keySet := make(map[string]bool)
			for _, k := range keys {
				keySet[k] = true
			}
			assert.True(t, keySet["entity-1"])
			assert.True(t, keySet["entity-2"])
		case <-time.After(1 * time.Second):
			t.Fatal("callback did not fire")
		}
	})
}

// TestCoalescingSet_EmptyKeyString verifies handling of empty string keys.
func TestCoalescingSet_EmptyKeyString(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		callbackCh := make(chan []string, 1)

		set := NewCoalescingSet(ctx, 50*time.Millisecond, func(keys []string) {
			callbackCh <- keys
		})
		defer set.Close()

		// Add empty string key (implementation-defined behavior)
		set.Add("")
		set.Add("entity-1")

		// Wait for callback
		select {
		case keys := <-callbackCh:
			// Implementation may choose to accept or reject empty keys
			// Verify it handles consistently without panic
			require.NotEmpty(t, keys, "should receive some keys")
		case <-time.After(1 * time.Second):
			t.Fatal("callback did not fire")
		}
	})
}
