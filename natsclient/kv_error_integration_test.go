//go:build integration

package natsclient

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/c360studio/semengine/pkg/retry"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestKVStore_ErrorBoundaries tests edge cases and error conditions
func TestKVStore_ErrorBoundaries(t *testing.T) {
	// Use real NATS via natsfixture
	testClient := newFixtureClient(t)
	client := testClient.Client

	ctx := context.Background()
	bucket, err := client.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
		Bucket:      "test-error-boundaries",
		Description: "Test error boundaries",
	})
	require.NoError(t, err)

	t.Run("value_size_limits", func(t *testing.T) {
		kv := client.NewKVStore(bucket, func(opts *KVOptions) {
			opts.MaxRetries = 3
			opts.RetryDelay = 10 * time.Millisecond
			opts.Timeout = time.Second
			opts.MaxValueSize = 100 // Small limit for testing
		})

		// Try to write value that exceeds limit
		largeValue := make([]byte, 200) // Exceeds MaxValueSize
		for i := range largeValue {
			largeValue[i] = 'x'
		}

		err := kv.UpdateWithRetry(ctx, "large-key", func(_ []byte) ([]byte, error) {
			return largeValue, nil
		})

		// Should fail with validation error
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "value size validation failed")
		assert.Contains(t, err.Error(), "exceeds maximum")

		// Value at the limit should work
		limitValue := make([]byte, 100)
		err = kv.UpdateWithRetry(ctx, "limit-key", func(_ []byte) ([]byte, error) {
			return limitValue, nil
		})
		assert.NoError(t, err)
	})

	t.Run("update_function_errors", func(t *testing.T) {
		kv := client.NewKVStore(bucket)

		// Update function that always fails
		expectedErr := errors.New("custom update error")
		err := kv.UpdateWithRetry(ctx, "error-key", func(_ []byte) ([]byte, error) {
			return nil, expectedErr
		})

		// Should propagate the error with context
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "update function error")
		assert.Contains(t, err.Error(), "custom update error")
	})

	t.Run("concurrent_updates_stress", func(t *testing.T) {
		kv := client.NewKVStore(bucket, func(opts *KVOptions) {
			opts.MaxRetries = 20 // High retry count for stress test
			opts.RetryDelay = 5 * time.Millisecond
			opts.Timeout = 5 * time.Second
			opts.UseExponentialBackoff = true
			opts.MaxRetryDelay = 100 * time.Millisecond
		})

		// Initialize counter
		err := kv.UpdateWithRetry(ctx, "counter", func(_ []byte) ([]byte, error) {
			return []byte("0"), nil
		})
		require.NoError(t, err)

		// Launch multiple goroutines to increment counter
		const numGoroutines = 10
		const incrementsPerGoroutine = 5
		var wg sync.WaitGroup

		successCount := int32(0)
		failCount := int32(0)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for j := 0; j < incrementsPerGoroutine; j++ {
					err := kv.UpdateWithRetry(ctx, "counter", func(current []byte) ([]byte, error) {
						// Parse current value
						var val int
						if len(current) > 0 {
							fmt.Sscanf(string(current), "%d", &val)
						}
						// Increment
						val++
						return []byte(fmt.Sprintf("%d", val)), nil
					})
					if err == nil {
						atomic.AddInt32(&successCount, 1)
					} else {
						atomic.AddInt32(&failCount, 1)
						t.Logf("Goroutine %d increment %d failed: %v", id, j, err)
					}
				}
			}(i)
		}

		wg.Wait()

		// Verify final count
		entry, err := kv.Get(ctx, "counter")
		require.NoError(t, err)

		var finalCount int
		fmt.Sscanf(string(entry.Value), "%d", &finalCount)

		// All increments should succeed despite high concurrency
		expectedCount := numGoroutines * incrementsPerGoroutine
		assert.Equal(t, expectedCount, finalCount, "Final counter value mismatch")
		assert.Equal(t, int32(expectedCount), successCount, "Not all updates succeeded")
		assert.Equal(t, int32(0), failCount, "Some updates failed")
	})

	t.Run("timeout_behavior", func(t *testing.T) {
		// Create a KV store with very short timeout
		kv := client.NewKVStore(bucket, func(opts *KVOptions) {
			opts.MaxRetries = 1
			opts.RetryDelay = time.Millisecond
			opts.Timeout = 1 * time.Nanosecond // Extremely short timeout
		})

		// This should timeout
		err := kv.UpdateWithRetry(ctx, "timeout-key", func(_ []byte) ([]byte, error) {
			return []byte("value"), nil
		})

		// Should get context deadline exceeded
		assert.Error(t, err)
		assert.True(t,
			errors.Is(err, context.DeadlineExceeded) ||
				strings.Contains(err.Error(), "deadline exceeded"),
			"Expected deadline exceeded error, got: %v", err)
	})

	t.Run("nil_and_empty_values", func(t *testing.T) {
		kv := client.NewKVStore(bucket)

		// Test nil value
		err := kv.UpdateWithRetry(ctx, "nil-key", func(_ []byte) ([]byte, error) {
			return nil, nil
		})
		assert.NoError(t, err)

		entry, err := kv.Get(ctx, "nil-key")
		require.NoError(t, err)
		assert.Equal(t, 0, len(entry.Value))

		// Test empty slice
		err = kv.UpdateWithRetry(ctx, "empty-key", func(_ []byte) ([]byte, error) {
			return []byte{}, nil
		})
		assert.NoError(t, err)

		entry, err = kv.Get(ctx, "empty-key")
		require.NoError(t, err)
		assert.Equal(t, 0, len(entry.Value))

		// Test transition from value to nil
		err = kv.UpdateWithRetry(ctx, "transition-key", func(_ []byte) ([]byte, error) {
			return []byte("initial"), nil
		})
		require.NoError(t, err)

		err = kv.UpdateWithRetry(ctx, "transition-key", func(current []byte) ([]byte, error) {
			assert.Equal(t, "initial", string(current))
			return nil, nil
		})
		assert.NoError(t, err)
	})

	t.Run("max_retries_exhaustion", func(t *testing.T) {
		kv := client.NewKVStore(bucket, func(opts *KVOptions) {
			opts.MaxRetries = 2 // Low retry count
			opts.RetryDelay = 5 * time.Millisecond
			opts.Timeout = time.Second
		})

		// Create initial value
		_, err := bucket.Create(ctx, "exhaustion-key", []byte("v1"))
		require.NoError(t, err)

		// Every attempt conflicts: the update function writes the key itself after the attempt
		// read it, so the attempt's revision is stale when it writes. The pin raced a 2 ms
		// background writer against a 10 ms sleep in the update function instead.
		counter := 2
		err = kv.UpdateWithRetry(ctx, "exhaustion-key", func(_ []byte) ([]byte, error) {
			_, putErr := bucket.Put(ctx, "exhaustion-key", []byte(fmt.Sprintf("v%d", counter)))
			counter++
			if putErr != nil {
				return nil, putErr
			}
			return []byte("my-update"), nil
		})

		// Should get max retries exceeded
		assert.Error(t, err)
		assert.True(t,
			errors.Is(err, ErrKVMaxRetriesExceeded) ||
				strings.Contains(err.Error(), "max retries exceeded"),
			"Expected max retries error, got: %v", err)
	})

	t.Run("invalid_json_handling", func(t *testing.T) {
		kv := client.NewKVStore(bucket)

		// Put invalid JSON
		_, err := bucket.Put(ctx, "bad-json", []byte("{invalid json}"))
		require.NoError(t, err)

		// Try to update as JSON
		err = kv.UpdateJSON(ctx, "bad-json", func(_ map[string]any) error {
			// Should never reach here
			t.Fatal("UpdateJSON should fail on invalid JSON")
			return nil
		})

		// Should get JSON parse error
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unmarshal")
	})

	t.Run("update_deleted_key", func(t *testing.T) {
		kv := client.NewKVStore(bucket)

		// Create and delete a key
		_, err := bucket.Create(ctx, "deleted-key", []byte("value"))
		require.NoError(t, err)

		err = bucket.Delete(ctx, "deleted-key")
		require.NoError(t, err)

		// Try to update deleted key - should treat as new
		err = kv.UpdateWithRetry(ctx, "deleted-key", func(current []byte) ([]byte, error) {
			assert.Nil(t, current, "Deleted key should be treated as nil")
			return []byte("new-value"), nil
		})
		assert.NoError(t, err)

		// Verify it was created
		entry, err := kv.Get(ctx, "deleted-key")
		require.NoError(t, err)
		assert.Equal(t, "new-value", string(entry.Value))
	})

	t.Run("update_deleted_key_must_exist_caller_surfaces_clean_not_found", func(t *testing.T) {
		// Companion to "update_deleted_key": that test validated the create-new
		// semantics (caller's updateFn happily produces a value when current is nil).
		// This test validates the must-exist semantics: a caller that returns
		// retry.NonRetryable(ErrKVKeyNotFound) when current is nil must see
		// that sentinel terminate the retry loop cleanly, NOT burn the full
		// retry budget into ErrKVMaxRetriesExceeded.
		//
		// This is the scenario filed in issue #122. The fix in IsKVNotFoundError
		// recognizing jetstream.ErrKeyDeleted is the defensive guard; this test
		// validates the end-to-end behavior the must-exist callers (e.g.
		// graph-ingest's handleEntityUpdateWithTriples) depend on.
		kv := client.NewKVStore(bucket, func(opts *KVOptions) {
			opts.MaxRetries = 3
			opts.RetryDelay = 10 * time.Millisecond
			opts.Timeout = time.Second
		})

		_, err := bucket.Create(ctx, "must-exist-tombstone-key", []byte("initial"))
		require.NoError(t, err)
		err = bucket.Delete(ctx, "must-exist-tombstone-key")
		require.NoError(t, err)

		var nilCurrentObserved int
		err = kv.UpdateWithRetry(ctx, "must-exist-tombstone-key", func(current []byte) ([]byte, error) {
			if len(current) == 0 {
				nilCurrentObserved++
				return nil, retry.NonRetryable(ErrKVKeyNotFound)
			}
			return []byte("should-not-reach"), nil
		})

		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrKVKeyNotFound),
			"must-exist caller on tombstoned key must surface ErrKVKeyNotFound via errors.Is; got: %v", err)
		assert.False(t, errors.Is(err, ErrKVMaxRetriesExceeded),
			"NonRetryable must short-circuit before retry exhaustion; got: %v", err)
		assert.Equal(t, 1, nilCurrentObserved,
			"updateFn must be invoked exactly once — NonRetryable stops the loop on first attempt")
	})

	t.Run("panic_recovery", func(t *testing.T) {
		kv := client.NewKVStore(bucket)

		// Update function that panics
		err := func() (err error) {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("panic recovered: %v", r)
				}
			}()

			return kv.UpdateWithRetry(ctx, "panic-key", func(_ []byte) ([]byte, error) {
				panic("test panic")
			})
		}()

		// The panic should be caught at a higher level
		// This tests that our code doesn't suppress panics
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "panic")
	})
}
