package retry

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRetry_Success(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		cfg := Config{
			MaxAttempts:  3,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
			Multiplier:   2.0,
			AddJitter:    false, // Disable for predictable tests
		}

		attempts := 0
		err := Do(ctx, cfg, func() error {
			attempts++
			if attempts < 3 {
				return errors.New("transient error")
			}
			return nil // Success on third attempt
		})

		assert.NoError(t, err)
		assert.Equal(t, 3, attempts)
	})
}

func TestRetry_AllAttemptsFail(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		cfg := Config{
			MaxAttempts:  3,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
			Multiplier:   2.0,
			AddJitter:    false,
		}

		attempts := 0
		err := Do(ctx, cfg, func() error {
			attempts++
			return errors.New("persistent error")
		})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed after 3 attempts")
		assert.Equal(t, 3, attempts)
	})
}

func TestRetry_ContextCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cfg := Config{
			MaxAttempts:  5,
			InitialDelay: 100 * time.Millisecond,
			MaxDelay:     1 * time.Second,
			Multiplier:   2.0,
			AddJitter:    false,
		}

		attempts := 0
		go func() {
			<-time.After(50 * time.Millisecond) // the bubble's clock: no real-clock pacing
			cancel()                            // Cancel during retry
		}()

		err := Do(ctx, cfg, func() error {
			attempts++
			return errors.New("error")
		})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "retry cancelled")
		assert.Less(t, attempts, 5) // Should not complete all attempts
	})
}

func TestRetry_BackoffTiming(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		cfg := Config{
			MaxAttempts:  4,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
			Multiplier:   2.0,
			AddJitter:    false,
		}

		start := time.Now()
		attempts := 0

		_ = Do(ctx, cfg, func() error {
			attempts++
			return errors.New("error")
		})

		elapsed := time.Since(start)

		// Delays: 10ms + 20ms + 40ms; on the bubble's clock there is no overhead
		assert.Equal(t, 70*time.Millisecond, elapsed)
		assert.Equal(t, 4, attempts)
	})
}

func TestRetry_MaxDelay(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		cfg := Config{
			MaxAttempts:  4,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     25 * time.Millisecond, // Low max delay
			Multiplier:   10.0,                  // High multiplier
			AddJitter:    false,
		}

		start := time.Now()

		_ = Do(ctx, cfg, func() error {
			return errors.New("error")
		})

		elapsed := time.Since(start)

		// Delays: 10ms + 25ms (capped) + 25ms (capped); on the bubble's clock there is no overhead
		assert.Equal(t, 60*time.Millisecond, elapsed)
	})
}

func TestRetry_WithResult(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		cfg := Config{
			MaxAttempts:  3,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
			Multiplier:   2.0,
			AddJitter:    false,
		}

		attempts := 0
		result, err := DoWithResult(ctx, cfg, func() (string, error) {
			attempts++
			if attempts < 3 {
				return "", errors.New("not ready")
			}
			return "success", nil
		})

		assert.NoError(t, err)
		assert.Equal(t, "success", result)
		assert.Equal(t, 3, attempts)
	})
}

func TestRetry_DefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, 3, cfg.MaxAttempts)
	assert.Equal(t, 100*time.Millisecond, cfg.InitialDelay)
	assert.Equal(t, 5*time.Second, cfg.MaxDelay)
	assert.Equal(t, 2.0, cfg.Multiplier)
	assert.True(t, cfg.AddJitter)
}

func TestRetry_QuickConfig(t *testing.T) {
	cfg := Quick()
	assert.Equal(t, 10, cfg.MaxAttempts)
	assert.Equal(t, 50*time.Millisecond, cfg.InitialDelay)
	assert.Equal(t, 1*time.Second, cfg.MaxDelay)
}

func TestRetry_PersistentConfig(t *testing.T) {
	cfg := Persistent()
	assert.Equal(t, 30, cfg.MaxAttempts)
	assert.Equal(t, 200*time.Millisecond, cfg.InitialDelay)
	assert.Equal(t, 10*time.Second, cfg.MaxDelay)
}

func TestRetry_ZeroAttempts(t *testing.T) {
	ctx := context.Background()
	cfg := Config{
		MaxAttempts: 0, // Should still run once
	}

	attempts := 0
	err := Do(ctx, cfg, func() error {
		attempts++
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 1, attempts)
}

func TestRetry_JitterBounds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		const initialDelay = 100 * time.Millisecond
		const maxJitter = initialDelay / 4
		cfg := Config{
			MaxAttempts:  2,
			InitialDelay: initialDelay,
			MaxDelay:     time.Second,
			Multiplier:   2.0,
			AddJitter:    true,
		}

		sawPositiveJitter := false
		distinctElapsed := make(map[time.Duration]struct{})

		for i := range 100 {
			start := time.Now()
			attempts := 0
			err := Do(ctx, cfg, func() error {
				attempts++
				if attempts == 1 {
					return errors.New("transient error")
				}
				return nil
			})
			elapsed := time.Since(start)
			if err != nil || attempts != 2 {
				t.Fatalf("iteration %d: err=%v attempts=%d, want nil and 2", i, err, attempts)
			}
			if elapsed < initialDelay || elapsed >= initialDelay+maxJitter {
				t.Fatalf("iteration %d: elapsed=%v out of bounds [%v, %v)", i, elapsed, initialDelay, initialDelay+maxJitter)
			}
			if elapsed > initialDelay {
				sawPositiveJitter = true
			}
			distinctElapsed[elapsed] = struct{}{}
		}

		if !sawPositiveJitter || len(distinctElapsed) < 2 {
			t.Fatalf("expected non-zero varying jitter across 100 runs; sawPositiveJitter=%v distinct=%d",
				sawPositiveJitter, len(distinctElapsed))
		}
	})
}

func TestRetry_ConcurrentWithJitter(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		cfg := Config{
			MaxAttempts:  3,
			InitialDelay: 20 * time.Millisecond,
			MaxDelay:     200 * time.Millisecond,
			Multiplier:   2.0,
			AddJitter:    true,
		}
		// Base delays: 20ms + 40ms = 60ms; max jitter: <5ms + <10ms = <15ms -> [60ms, 75ms).
		const minElapsed = 60 * time.Millisecond
		const maxElapsed = 75 * time.Millisecond

		const workers = 16
		var wg sync.WaitGroup
		wg.Add(workers)
		for i := range workers {
			go func(worker int) {
				defer wg.Done()
				start := time.Now()
				attempts := 0
				err := Do(ctx, cfg, func() error {
					attempts++
					if attempts < 3 {
						return errors.New("transient error")
					}
					return nil
				})
				elapsed := time.Since(start)
				if err != nil || attempts != 3 {
					t.Errorf("worker %d: err=%v attempts=%d, want nil and 3", worker, err, attempts)
				}
				if elapsed < minElapsed || elapsed >= maxElapsed {
					t.Errorf("worker %d: elapsed=%v out of bounds [%v, %v)", worker, elapsed, minElapsed, maxElapsed)
				}
			}(i)
		}
		wg.Wait()
	})
}

// Benchmark to ensure performance
func BenchmarkRetry_Success(b *testing.B) {
	ctx := context.Background()
	cfg := Config{
		MaxAttempts:  1,
		InitialDelay: 1 * time.Millisecond,
		AddJitter:    false,
	}

	for i := 0; i < b.N; i++ {
		_ = Do(ctx, cfg, func() error {
			return nil
		})
	}
}

// Example for documentation
func ExampleDo() {
	ctx := context.Background()
	cfg := DefaultConfig()

	err := Do(ctx, cfg, func() error {
		// Your operation that might fail
		return connectToService()
	})

	_ = err // Handle error after all retries exhausted
}

// Stub function for example
func connectToService() error {
	return nil
}
