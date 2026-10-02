package resource

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

// Every test whose outcome depends on the Watcher's timers runs inside a synctest bubble, where
// those timers use a fake clock: time advances only when every goroutine in the bubble is blocked,
// so no test paces itself on the real clock (design D8, R1a).

func TestWatcher_WaitForStartup_ImmediateSuccess(t *testing.T) {
	// Resource available immediately
	checkFn := func(_ context.Context) error {
		return nil
	}

	w := NewWatcher("test-resource", checkFn, DefaultConfig())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if !w.WaitForStartup(ctx) {
		t.Error("expected WaitForStartup to return true when resource is available")
	}

	if !w.IsAvailable() {
		t.Error("expected IsAvailable to return true after successful startup")
	}
}

func TestWatcher_WaitForStartup_EventualSuccess(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Resource becomes available after 3 attempts
		var attempts atomic.Int32
		checkFn := func(_ context.Context) error {
			if attempts.Add(1) < 3 {
				return errors.New("not ready")
			}
			return nil
		}

		cfg := DefaultConfig()
		cfg.StartupInterval = 10 * time.Millisecond // Fast for testing
		w := NewWatcher("test-resource", checkFn, cfg)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if !w.WaitForStartup(ctx) {
			t.Error("expected WaitForStartup to return true after eventual availability")
		}

		if attempts.Load() != 3 {
			t.Errorf("expected 3 attempts, got %d", attempts.Load())
		}
	})
}

func TestWatcher_WaitForStartup_ExhaustsAttempts(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Resource never becomes available
		checkFn := func(_ context.Context) error {
			return errors.New("never ready")
		}

		cfg := DefaultConfig()
		cfg.StartupAttempts = 3
		cfg.StartupInterval = 10 * time.Millisecond
		w := NewWatcher("test-resource", checkFn, cfg)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if w.WaitForStartup(ctx) {
			t.Error("expected WaitForStartup to return false when attempts exhausted")
		}

		if w.IsAvailable() {
			t.Error("expected IsAvailable to return false after failed startup")
		}
	})
}

func TestWatcher_WaitForStartup_ContextCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Context cancelled during startup
		checkFn := func(_ context.Context) error {
			return errors.New("not ready")
		}

		cfg := DefaultConfig()
		cfg.StartupAttempts = 100 // Many attempts
		cfg.StartupInterval = 50 * time.Millisecond
		w := NewWatcher("test-resource", checkFn, cfg)

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		start := time.Now()
		result := w.WaitForStartup(ctx)
		elapsed := time.Since(start)

		if result {
			t.Error("expected WaitForStartup to return false when context cancelled")
		}

		// Should have returned at the context's deadline, not waited for all attempts
		// (100 × 50ms). On the bubble's clock the deadline is exact.
		if elapsed != 100*time.Millisecond {
			t.Errorf("expected return at the 100ms deadline, took %v", elapsed)
		}
	})
}

// runWatcher runs w.Run on a goroutine the test owns and returns a stop function that cancels
// it and waits for Run to return, failing the test unless Run reports the cancellation.
func runWatcher(t *testing.T, w *Watcher) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	return func() {
		t.Helper()
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Errorf("Run after cancel = %v, want context.Canceled", err)
		}
	}
}

func TestWatcher_BackgroundCheck_ResourceBecomesAvailable(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Resource unavailable at first, then becomes available in background
		var ready atomic.Bool
		var availableCalled atomic.Bool

		checkFn := func(_ context.Context) error {
			if ready.Load() {
				return nil
			}
			return errors.New("not ready")
		}

		cfg := DefaultConfig()
		cfg.StartupAttempts = 1
		cfg.StartupInterval = 10 * time.Millisecond
		cfg.RecheckInterval = 50 * time.Millisecond
		cfg.OnAvailable = func() {
			availableCalled.Store(true)
		}

		w := NewWatcher("test-resource", checkFn, cfg)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Start up - resource not available
		if w.WaitForStartup(ctx) {
			t.Error("expected startup to fail initially")
		}

		// Start background checking
		stop := runWatcher(t, w)

		// Make resource available
		ready.Store(true)

		// One recheck interval later the background check has run; Wait lets it finish.
		<-time.After(cfg.RecheckInterval)
		synctest.Wait()

		if !w.IsAvailable() {
			t.Error("expected resource to become available via background check")
		}

		if !availableCalled.Load() {
			t.Error("expected OnAvailable callback to be called")
		}

		stop()
	})
}

func TestWatcher_BackgroundCheck_ResourceLost(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Resource available initially, then lost
		var ready atomic.Bool
		ready.Store(true)

		var lostCalled atomic.Bool

		checkFn := func(_ context.Context) error {
			if ready.Load() {
				return nil
			}
			return errors.New("resource lost")
		}

		cfg := DefaultConfig()
		cfg.StartupAttempts = 1
		cfg.HealthInterval = 50 * time.Millisecond
		cfg.OnLost = func() {
			lostCalled.Store(true)
		}

		w := NewWatcher("test-resource", checkFn, cfg)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Start up - resource available
		if !w.WaitForStartup(ctx) {
			t.Error("expected startup to succeed")
		}

		// Start background health checking
		stop := runWatcher(t, w)

		// Simulate resource loss
		ready.Store(false)

		// One health interval later the health check has run; Wait lets it finish.
		<-time.After(cfg.HealthInterval)
		synctest.Wait()

		if w.IsAvailable() {
			t.Error("expected resource to be marked unavailable after loss")
		}

		if !lostCalled.Load() {
			t.Error("expected OnLost callback to be called")
		}

		stop()
	})
}

// TestWatcher_Run_SecondRunRefused replaces the pin's TestWatcher_BackgroundCheck_Idempotent:
// a second Run while one is in progress returns an error without starting a second loop.
func TestWatcher_Run_SecondRunRefused(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var checks atomic.Int32
		cfg := DefaultConfig()
		cfg.RecheckInterval = time.Minute
		w := NewWatcher("test-resource", func(_ context.Context) error {
			checks.Add(1)
			return errors.New("not ready")
		}, cfg)

		stop := runWatcher(t, w)
		synctest.Wait() // the first Run is parked on its ticker

		// The second Run gets its own context so that, were it to start a loop, the test can
		// still end it; a refusal returns at once.
		second, cancelSecond := context.WithCancel(t.Context())
		secondDone := make(chan error, 1)
		go func() { secondDone <- w.Run(second) }()
		synctest.Wait()
		select {
		case err := <-secondDone:
			if err == nil {
				t.Fatal("second concurrent Run = nil, want an error")
			}
		default:
			cancelSecond()
			<-secondDone
			t.Fatal("second concurrent Run did not return at once; it started a second loop")
		}
		cancelSecond()

		<-time.After(cfg.RecheckInterval + cfg.RecheckInterval/2)
		synctest.Wait()
		if got := checks.Load(); got != 1 {
			t.Fatalf("checks after one tick = %d, want 1 (one loop)", got)
		}
		stop()

		// Once the first Run has returned, the Watcher can run again.
		stop = runWatcher(t, w)
		stop()
	})
}

func TestWatcher_Name(t *testing.T) {
	checkFn := func(_ context.Context) error {
		return nil
	}

	w := NewWatcher("my-resource", checkFn, DefaultConfig())

	if w.Name() != "my-resource" {
		t.Errorf("expected name 'my-resource', got '%s'", w.Name())
	}
}

func TestWatcher_ConfigDefaults(t *testing.T) {
	checkFn := func(_ context.Context) error {
		return nil
	}

	// Empty config should get defaults
	w := NewWatcher("test", checkFn, Config{})

	if w.startupAttempts != 10 {
		t.Errorf("expected startupAttempts=10, got %d", w.startupAttempts)
	}
	if w.startupInterval != 500*time.Millisecond {
		t.Errorf("expected startupInterval=500ms, got %v", w.startupInterval)
	}
	if w.recheckInterval != 60*time.Second {
		t.Errorf("expected recheckInterval=60s, got %v", w.recheckInterval)
	}
}

func TestWatcher_ConcurrentAccess(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Test that IsAvailable is safe for concurrent access
		var ready atomic.Bool
		ready.Store(true)

		checkFn := func(_ context.Context) error {
			if ready.Load() {
				return nil
			}
			return errors.New("not ready")
		}

		cfg := DefaultConfig()
		cfg.HealthInterval = 10 * time.Millisecond
		w := NewWatcher("test-resource", checkFn, cfg)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		w.WaitForStartup(ctx)
		stop := runWatcher(t, w)

		// Multiple goroutines reading availability
		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 100; j++ {
					_ = w.IsAvailable()
					<-time.After(time.Millisecond)
				}
			}()
		}

		// Toggle availability during concurrent reads
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				ready.Store(false)
				<-time.After(10 * time.Millisecond)
				ready.Store(true)
				<-time.After(10 * time.Millisecond)
			}
		}()

		wg.Wait()
		stop()
	})
}

func TestWatcher_HealthCheckDisabled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// When HealthInterval is 0, use recheck interval for available resources
		var checkCount atomic.Int32
		checkFn := func(_ context.Context) error {
			checkCount.Add(1)
			return nil
		}

		cfg := DefaultConfig()
		cfg.HealthInterval = 0 // Disabled
		cfg.RecheckInterval = 50 * time.Millisecond
		w := NewWatcher("test-resource", checkFn, cfg)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		w.WaitForStartup(ctx)
		stop := runWatcher(t, w)

		// Wait for a few recheck intervals
		<-time.After(200 * time.Millisecond)
		synctest.Wait()
		stop()

		// Should have done startup check plus a few background checks
		if checkCount.Load() < 2 {
			t.Errorf("expected at least 2 checks, got %d", checkCount.Load())
		}
	})
}

// TestWatcher_Run_ReturnsOnCancelAndLeavesNothing: Run checks on every tick and returns ctx.Err()
// once its context is cancelled. The synctest bubble returning proves no goroutine Run started is
// still running (background-work › "Run returns when its context ends").
func TestWatcher_Run_ReturnsOnCancelAndLeavesNothing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var checks atomic.Int32
		cfg := DefaultConfig()
		cfg.RecheckInterval = time.Minute
		w := NewWatcher("run", func(context.Context) error {
			checks.Add(1)
			return errors.New("not ready")
		}, cfg)

		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() { done <- w.Run(ctx) }()

		// Half an interval past the third tick: exactly three checks, one per tick.
		<-time.After(3*cfg.RecheckInterval + cfg.RecheckInterval/2)
		synctest.Wait()
		if got := checks.Load(); got != 3 {
			t.Fatalf("checks after three ticks = %d, want 3", got)
		}

		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("Run after cancel = %v, want context.Canceled", err)
		}
		if got := checks.Load(); got != 3 {
			t.Fatalf("checks after Run returned = %d, want 3", got)
		}
	})
}

// TestWatcher_Run_NilContextRefused: Run(nil) returns an error at the call and calls no check
// (background-work › "Nil context at the entry").
func TestWatcher_Run_NilContextRefused(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var checks atomic.Int32
		w := NewWatcher("nil", func(context.Context) error {
			checks.Add(1)
			return nil
		}, DefaultConfig())

		var nilCtx context.Context // the nil context is the input under test
		if err := w.Run(nilCtx); err == nil {
			t.Fatal("Run(nil) = nil, want an error")
		}
		if got := checks.Load(); got != 0 {
			t.Fatalf("checks after Run(nil) = %d, want 0", got)
		}
	})
}
