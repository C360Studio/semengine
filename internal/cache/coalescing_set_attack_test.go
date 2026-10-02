// Attack tests written for SemStreams by its Reviewer Agent; permanent CI fixtures designed to
// break the CoalescingSet implementation. Repaired for SemEngine (design D8, R1a): every test but
// TestAttack_CallbackLatency runs inside a synctest bubble, on a fake clock that advances only when
// every goroutine in the bubble is blocked; t.Parallel cannot be called inside a bubble, so it is
// gone.

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

// TestAttack_RapidAddRemoveSameEntity verifies handling when entity is rapidly added and removed
// before window expires. Tests TOCTOU race condition.
func TestAttack_RapidAddRemoveSameEntity(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		callbackCh := make(chan []string, 10)

		set := NewCoalescingSet(ctx, 100*time.Millisecond, func(keys []string) {
			callbackCh <- keys
		})
		defer set.Shutdown(t.Context())

		// Rapidly add and remove same entity multiple times
		const iterations = 100
		for i := 0; i < iterations; i++ {
			set.Add("entity-volatile")
			set.Remove("entity-volatile")
		}

		// Add a different entity to ensure callback fires
		set.Add("entity-stable")

		// Wait for callback
		select {
		case keys := <-callbackCh:
			// Should NOT contain the volatile entity
			keySet := make(map[string]bool)
			for _, k := range keys {
				keySet[k] = true
			}
			assert.False(t, keySet["entity-volatile"], "volatile entity should not appear (was removed)")
			assert.True(t, keySet["entity-stable"], "stable entity should appear")
		case <-time.After(2 * time.Second):
			t.Fatal("callback did not fire")
		}
	})
}

// TestAttack_HighVolumeBatching verifies system handles 10K entities in single window
func TestAttack_HighVolumeBatching(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		callbackCh := make(chan []string, 1)

		set := NewCoalescingSet(ctx, 200*time.Millisecond, func(keys []string) {
			callbackCh <- keys
		})
		defer set.Shutdown(t.Context())

		// Add 10K unique entities
		const entityCount = 10000
		for i := 0; i < entityCount; i++ {
			set.Add(generateEntityKey(i))
		}

		// Wait for callback
		select {
		case keys := <-callbackCh:
			// Should receive exactly 10K unique keys
			require.Len(t, keys, entityCount, "should receive all 10K entities")

			// Verify uniqueness
			keySet := make(map[string]bool)
			for _, k := range keys {
				require.False(t, keySet[k], "duplicate key detected: %s", k)
				keySet[k] = true
			}
		case <-time.After(5 * time.Second):
			t.Fatal("callback did not fire within timeout")
		}
	})
}

// TestAttack_CallbackLatency verifies long-running callback doesn't block adds
//
// It runs on the real clock, not in a bubble: an Add blocked on the set's mutex is not durably
// blocked, so inside a bubble a regression would stall the fake clock and hang the test instead of
// failing it. The callback is held on a channel the test releases, never on a timer (design D8,
// R1b): the 100 Adds must all return while it is held; the 10 s bound is a failure bound only.
func TestAttack_CallbackLatency(t *testing.T) {
	ctx := context.Background()
	var callCount atomic.Int32
	callbackStarted := make(chan struct{})
	release := make(chan struct{})
	callbackFinished := make(chan struct{})

	set := NewCoalescingSet(ctx, 50*time.Millisecond, func(keys []string) {
		count := callCount.Add(1)
		if count == 1 {
			close(callbackStarted)
			// Expensive processing, held until the test releases it
			<-release
			close(callbackFinished)
		}
	})
	defer set.Shutdown(t.Context())

	// Trigger first callback
	set.Add("entity-1")

	// Wait for callback to start
	<-callbackStarted

	// While callback is running, add more entities
	addsDone := make(chan struct{})
	go func() {
		defer close(addsDone)
		for i := 0; i < 100; i++ {
			set.Add("entity-during-callback")
		}
	}()

	// Adds should not block while the callback runs
	select {
	case <-addsDone:
	case <-time.After(10 * time.Second):
		t.Error("Add() blocked during callback execution")
	}
	assert.Equal(t, 1, set.PendingCount(), "the adds made during the callback are pending")

	// Release the callback and wait for it to finish
	close(release)
	<-callbackFinished
	<-addsDone
}

// TestAttack_ContextCancellationDuringCallback verifies context cancellation during callback
func TestAttack_ContextCancellationDuringCallback(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		callbackStarted := make(chan struct{})
		callbackPanicked := make(chan interface{}, 1)

		set := NewCoalescingSet(ctx, 50*time.Millisecond, func(keys []string) {
			// Signal callback started
			select {
			case callbackStarted <- struct{}{}:
			default:
			}

			// Check if we can access keys without panic
			defer func() {
				if r := recover(); r != nil {
					callbackPanicked <- r
				}
			}()

			// Simulate some work
			_ = len(keys)
		})
		defer set.Shutdown(t.Context())

		// Add entity to trigger callback
		set.Add("entity-1")

		// Wait for first callback
		<-callbackStarted

		// Cancel context DURING callback
		cancel()

		// Wait to see if callback panics
		select {
		case r := <-callbackPanicked:
			t.Fatalf("callback panicked after context cancellation: %v", r)
		case <-time.After(200 * time.Millisecond):
			// Good - no panic
		}

		// Verify set stopped cleanly
		<-time.After(100 * time.Millisecond)
		synctest.Wait()
	})
}

// TestAttack_ConcurrentAddRemove verifies concurrent add/remove operations don't cause races
func TestAttack_ConcurrentAddRemove(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		var callbackCount atomic.Int32

		set := NewCoalescingSet(ctx, 100*time.Millisecond, func(keys []string) {
			callbackCount.Add(1)
		})
		defer set.Shutdown(t.Context())

		// Run concurrent add/remove operations
		const numGoroutines = 50
		const opsPerGoroutine = 100

		var wg sync.WaitGroup
		wg.Add(numGoroutines * 2)

		// Adders
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < opsPerGoroutine; j++ {
					set.Add(generateEntityKey(id*opsPerGoroutine + j))
				}
			}(i)
		}

		// Removers (removing what adders add)
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < opsPerGoroutine; j++ {
					set.Remove(generateEntityKey(id*opsPerGoroutine + j))
				}
			}(i)
		}

		wg.Wait()

		// System should not crash or deadlock. Whether any generated key is still pending depends
		// on how the adders and removers interleaved, so the pin's "at least one callback" after a
		// fixed 300 ms failed whenever the removers had emptied the set (design P19). A sentinel
		// makes the next window's batch certain: exactly one callback, carrying the sentinel, and
		// the set is empty after it.
		require.True(t, set.Add("sentinel"))
		<-time.After(100 * time.Millisecond)
		synctest.Wait()

		count := callbackCount.Load()
		assert.Equal(t, int32(1), count, "one window, one callback")
		assert.Zero(t, set.PendingCount(), "the batch took every pending key")
	})
}

// TestAttack_CloseWhileCallbackRunning verifies Shutdown() waits for callback to complete
func TestAttack_CloseWhileCallbackRunning(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		callbackRunning := make(chan struct{})
		callbackFinished := make(chan struct{})

		set := NewCoalescingSet(ctx, 50*time.Millisecond, func(keys []string) {
			close(callbackRunning)
			// Simulate long-running callback
			<-time.After(150 * time.Millisecond)
			close(callbackFinished)
		})

		// Add entity to trigger callback
		set.Add("entity-1")

		// Wait for callback to start
		<-callbackRunning

		// Close while callback is running
		closeDone := make(chan struct{})
		go func() {
			require.NoError(t, set.Shutdown(t.Context()))
			close(closeDone)
		}()

		// Close should wait for callback to finish
		select {
		case <-closeDone:
			// Verify callback actually finished
			select {
			case <-callbackFinished:
				// Good - callback completed before Close returned
			default:
				t.Fatal("Shutdown() returned before callback finished")
			}
		case <-time.After(1 * time.Second):
			t.Fatal("Shutdown() hung waiting for callback")
		}
	})
}

// TestAttack_ZeroWindowRaceCondition verifies zero window doesn't cause TOCTOU with callback firing twice
// This was the original bug in Debouncer that CoalescingSet should NOT have.
func TestAttack_ZeroWindowRaceCondition(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		var callbackCount atomic.Int32
		var receivedBatches [][]string
		var mu sync.Mutex

		delivered := make(chan struct{}, 1)
		set := NewCoalescingSet(ctx, 0, func(keys []string) {
			callbackCount.Add(1)
			mu.Lock()
			receivedBatches = append(receivedBatches, keys)
			mu.Unlock()
			select {
			case delivered <- struct{}{}:
			default:
			}
		})
		defer set.Shutdown(t.Context())

		// Add same entity multiple times rapidly with zero window
		const iterations = 100
		for i := 0; i < iterations; i++ {
			set.Add("entity-1")
		}

		// Allow callbacks to fire. A zero window ticks every nanosecond of the bubble's clock, so
		// the test waits for the first delivery rather than a fixed span, which would be 10^8
		// ticks; then it lets a further microsecond (a thousand ticks) pass.
		<-delivered
		<-time.After(time.Microsecond)
		synctest.Wait()

		// Verify entity-1 appears in at most ONE batch per window
		mu.Lock()
		defer mu.Unlock()

		entity1Count := 0
		for _, batch := range receivedBatches {
			for _, key := range batch {
				if key == "entity-1" {
					entity1Count++
				}
			}
		}

		// Despite 100 additions, entity-1 should appear limited number of times
		// (once per tick, not 100 times). Use generous threshold for slow CI.
		assert.LessOrEqual(t, entity1Count, 50,
			"entity should not appear 100 times (deduplication failed)")
	})
}

// TestAttack_ConcurrentClose verifies multiple goroutines calling Shutdown() simultaneously
func TestAttack_ConcurrentClose(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		set := NewCoalescingSet(ctx, 50*time.Millisecond, func(keys []string) {})

		// Add some keys
		set.Add("entity-1")

		// Multiple goroutines call Shutdown() simultaneously
		const numClosers = 20
		var wg sync.WaitGroup
		wg.Add(numClosers)

		errors := make(chan error, numClosers)

		for i := 0; i < numClosers; i++ {
			go func() {
				defer wg.Done()
				err := set.Shutdown(t.Context())
				errors <- err
			}()
		}

		wg.Wait()
		close(errors)

		// All Shutdown() calls should succeed (idempotent)
		for err := range errors {
			require.NoError(t, err, "Shutdown() should be idempotent")
		}
	})
}

// TestAttack_AddAfterClose verifies Add() after Shutdown() doesn't panic or hang
func TestAttack_AddAfterClose(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		set := NewCoalescingSet(ctx, 50*time.Millisecond, func(keys []string) {})

		// Close immediately
		require.NoError(t, set.Shutdown(t.Context()))

		// Try to add after close - should not panic
		require.NotPanics(t, func() {
			set.Add("entity-after-close")
		})

		// Add should not hang
		done := make(chan struct{})
		go func() {
			set.Add("entity-after-close-2")
			close(done)
		}()

		select {
		case <-done:
			// Good - Add returned
		case <-time.After(1 * time.Second):
			t.Fatal("Add() hung after Shutdown()")
		}
	})
}

// TestAttack_LargeKeyNames verifies handling of very large key strings
func TestAttack_LargeKeyNames(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		callbackCh := make(chan []string, 1)

		set := NewCoalescingSet(ctx, 50*time.Millisecond, func(keys []string) {
			callbackCh <- keys
		})
		defer set.Shutdown(t.Context())

		// Create very large key (1MB)
		largeKey := generateLargeString(1024 * 1024)
		set.Add(largeKey)
		set.Add("normal-key")

		// Wait for callback
		select {
		case keys := <-callbackCh:
			require.Len(t, keys, 2, "should receive both keys")
			// Verify large key is preserved
			keySet := make(map[string]bool)
			for _, k := range keys {
				keySet[k] = true
			}
			assert.True(t, keySet[largeKey], "large key should be present")
			assert.True(t, keySet["normal-key"], "normal key should be present")
		case <-time.After(2 * time.Second):
			t.Fatal("callback did not fire")
		}
	})
}

// Helper function to generate unique entity keys
func generateEntityKey(id int) string {
	// Ensure uniqueness by including the full ID
	return "entity-" + string(rune('A'+id%26)) + "-" + string(rune('0'+id%10)) + "-" + string(rune('0'+(id/10)%10)) + "-" + string(rune('0'+(id/100)%10)) + "-" + string(rune('0'+(id/1000)%10))
}

// Helper function to generate large strings
func generateLargeString(size int) string {
	b := make([]byte, size)
	for i := range b {
		b[i] = byte('A' + i%26)
	}
	return string(b)
}
