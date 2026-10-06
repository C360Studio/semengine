// Package resource provides utilities for monitoring resource availability.
// Use Watcher for components that depend on resources (KV buckets, streams)
// created by other components, enabling graceful startup and recovery.
package resource

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// Watcher monitors availability of a resource and provides callbacks
// when availability changes. It handles:
//   - Startup waiting with bounded retries
//   - Background re-checking if resource unavailable at startup
//   - Detection if resource disappears after becoming available
type Watcher struct {
	name    string
	checkFn func(ctx context.Context) error
	logger  *slog.Logger

	// Callbacks
	onAvailable func()
	onLost      func()

	// Config
	startupAttempts int
	startupInterval time.Duration
	recheckInterval time.Duration
	healthInterval  time.Duration // How often to verify still available

	// State
	available atomic.Bool
	mu        sync.Mutex
	running   bool // a Run is in progress; guarded by mu
}

// Config holds configuration for a Watcher.
type Config struct {
	// StartupAttempts is the number of retries during WaitForStartup (default: 10)
	StartupAttempts int

	// StartupInterval is the delay between startup retries (default: 500ms)
	StartupInterval time.Duration

	// RecheckInterval is how often to re-check when in fallback mode (default: 60s)
	RecheckInterval time.Duration

	// HealthInterval is how often to verify resource still available (default: 30s)
	// Set to 0 to disable health checking
	HealthInterval time.Duration

	// OnAvailable is called when resource becomes available (optional)
	OnAvailable func()

	// OnLost is called when resource becomes unavailable after being available (optional)
	OnLost func()

	// Logger for watcher events (optional, uses slog.Default if nil)
	Logger *slog.Logger
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		StartupAttempts: 10,
		StartupInterval: 500 * time.Millisecond,
		RecheckInterval: 60 * time.Second,
		HealthInterval:  30 * time.Second,
	}
}

// NewWatcher creates a new resource watcher.
// The checkFn should return nil if the resource is available, error otherwise.
func NewWatcher(name string, checkFn func(ctx context.Context) error, cfg Config) *Watcher {
	// Apply defaults
	if cfg.StartupAttempts <= 0 {
		cfg.StartupAttempts = 10
	}
	if cfg.StartupInterval <= 0 {
		cfg.StartupInterval = 500 * time.Millisecond
	}
	if cfg.RecheckInterval <= 0 {
		cfg.RecheckInterval = 60 * time.Second
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	return &Watcher{
		name:            name,
		checkFn:         checkFn,
		logger:          cfg.Logger,
		onAvailable:     cfg.OnAvailable,
		onLost:          cfg.OnLost,
		startupAttempts: cfg.StartupAttempts,
		startupInterval: cfg.StartupInterval,
		recheckInterval: cfg.RecheckInterval,
		healthInterval:  cfg.HealthInterval,
	}
}

// WaitForStartup blocks until the resource is available or startup attempts exhausted.
// Returns true if resource is available, false if entering fallback mode.
// If false is returned, call Run on a goroutine you own to enable periodic re-checking.
func (w *Watcher) WaitForStartup(ctx context.Context) bool {
	for attempt := 1; attempt <= w.startupAttempts; attempt++ {
		if err := w.checkFn(ctx); err == nil {
			w.available.Store(true)
			w.logger.Info("resource available",
				"resource", w.name,
				"attempt", attempt)
			return true
		}

		if attempt < w.startupAttempts {
			select {
			case <-ctx.Done():
				w.logger.Debug("startup wait cancelled",
					"resource", w.name,
					"attempt", attempt)
				return false
			case <-time.After(w.startupInterval):
				// Continue to next attempt
			}
		}
	}

	w.logger.Info("resource not available after startup attempts, entering fallback mode",
		"resource", w.name,
		"attempts", w.startupAttempts)
	return false
}

// Run performs the periodic availability check until ctx ends, then returns ctx.Err().
// Call it on a goroutine you own, typically after WaitForStartup returns false; Run returning
// is the join, so cancelling ctx and waiting for Run to return leaves nothing running. The
// OnAvailable and OnLost callbacks run on the caller's goroutine, inside Run.
//
// Run refuses a nil context, and refuses to start while another Run of the same Watcher is in
// progress; both return an error without calling the check.
func (w *Watcher) Run(ctx context.Context) error {
	if ctx == nil {
		return errors.New("resource: Run called with a nil context")
	}

	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return errors.New("resource: watcher " + w.name + " is already running")
	}
	w.running = true
	w.mu.Unlock()
	defer func() {
		w.mu.Lock()
		w.running = false
		w.mu.Unlock()
	}()

	// Determine which interval to use based on current state
	getInterval := func() time.Duration {
		if w.available.Load() {
			if w.healthInterval > 0 {
				return w.healthInterval
			}
			return w.recheckInterval // Fall back to recheck interval if health disabled
		}
		return w.recheckInterval
	}

	ticker := time.NewTicker(getInterval())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			w.checkAndUpdate(ctx)
			// Adjust ticker interval based on current state
			ticker.Reset(getInterval())
		}
	}
}

// checkAndUpdate checks resource availability and updates state.
func (w *Watcher) checkAndUpdate(ctx context.Context) {
	wasAvailable := w.available.Load()
	err := w.checkFn(ctx)
	isAvailable := err == nil

	if isAvailable && !wasAvailable {
		// Resource became available
		w.available.Store(true)
		w.logger.Info("resource became available",
			"resource", w.name)
		if w.onAvailable != nil {
			w.onAvailable()
		}
	} else if !isAvailable && wasAvailable {
		// Resource was lost
		w.available.Store(false)
		w.logger.Warn("resource lost",
			"resource", w.name,
			"error", err)
		if w.onLost != nil {
			w.onLost()
		}
	}
}

// IsAvailable returns whether the resource is currently available.
func (w *Watcher) IsAvailable() bool {
	return w.available.Load()
}

// Name returns the resource name.
func (w *Watcher) Name() string {
	return w.name
}
