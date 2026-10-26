package component

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/c360studio/semengine/internal/lifecycleguard"
	"github.com/c360studio/semengine/pkg/errs"
)

// Lifecycle is the processor shell of ADR-108: the part of a component that is the same for every
// component, which is starting once, stopping, rolling back a failed start, and answering Health
// and DataFlow. A LifecycleComponent holds a Lifecycle as an unexported field and answers the
// lifecycle half of that interface with one-line calls to it.
//
// The component supplies what only it knows: a start function that acquires its resources and
// creates the run context for its continuing work, a cleanup function that releases them in its
// own order and ends that run context (ADR-113), and its current stuck states at each Health
// call. The shell holds no context and no cancel and creates neither, decides no release order,
// reports no readiness and registers no metrics: its counters are plain integers.
//
// The zero value is a shell that has not been initialized. A Lifecycle must not be copied after
// first use.
type Lifecycle struct {
	guard lifecycleguard.Guard

	// mu guards the fields below. It is never held while start or cleanup runs, so Health and
	// DataFlow never wait for a Start or Stop in progress.
	mu          sync.Mutex
	initialized bool
	// running is set when start returns nil and cleared when cleanup begins, for a Stop or for
	// a failed start's rollback; startedAt is when it was last set.
	running      bool
	startedAt    time.Time
	messages     int64
	bytes        int64
	errorCount   int
	lastActivity time.Time
}

// Initialize marks the shell initialized and returns nil, every time. It does no I/O; it returns
// an error only so a component's Initialize can be the one line that calls it.
func (l *Lifecycle) Initialize() error {
	l.mu.Lock()
	l.initialized = true
	l.mu.Unlock()
	return nil
}

// Start refuses a shell that was never initialized, running nothing, so a Start after Initialize
// is still admitted. Otherwise it is lifecycleguard's Start: a nil or ended context and a second
// Start are refused; start runs under ctx; a start that fails or panics is rolled back through
// cleanup, and a rollback that fails stays pending for the next Stop. The shell is running from
// the moment start returns nil until cleanup begins.
//
// cleanup runs after a partial start as well as a complete one, so it must tolerate handles that
// were never acquired. It must not panic: a cleanup that panics leaves the guard's state behind
// it, so a later Stop cannot try again. The shell inherits that limitation from
// internal/lifecycleguard (#309).
func (l *Lifecycle) Start(ctx context.Context, start, cleanup func(context.Context) error) error {
	l.mu.Lock()
	initialized := l.initialized
	l.mu.Unlock()
	if !initialized {
		return errs.WrapFatal(errors.New("component not initialized"), "Lifecycle", "Start", "initialization check")
	}
	return l.guard.Start(ctx, func(ctx context.Context) error {
		if err := start(ctx); err != nil {
			return err
		}
		l.mu.Lock()
		l.running, l.startedAt = true, time.Now()
		l.mu.Unlock()
		return nil
	}, func(ctx context.Context) error {
		l.mu.Lock()
		l.running = false
		l.mu.Unlock()
		return cleanup(ctx)
	})
}

// Stop is lifecycleguard's Stop: it runs the cleanup Start was given and returns its result. Stop
// before Start returns nil and uses up the shell, a Stop after a successful cleanup is a no-op,
// and a cleanup that fails stays pending for the next Stop.
func (l *Lifecycle) Stop(ctx context.Context) error {
	return l.guard.Stop(ctx)
}

// Health reports whether the shell is running and the stuck states the component passes in. Not
// running: Healthy false, Status "stopped", no LastError and zero Uptime, whatever stuck holds.
// Running with no stuck state: Healthy true and Status "running". Running with stuck states:
// Healthy false, Status "degraded" and LastError the states joined by "; ". An empty string is
// not a stuck state and is ignored. ErrorCount is the number of CountError calls and never lowers
// Healthy (ADR-109, Decision 2). Health may be called at any time and never waits for Start or
// Stop.
func (l *Lifecycle) Health(stuck ...string) HealthStatus {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	h := HealthStatus{LastCheck: now, ErrorCount: l.errorCount, Status: "stopped"}
	if !l.running {
		return h
	}
	h.Uptime = now.Sub(l.startedAt)
	var states []string
	for _, s := range stuck {
		if s != "" {
			states = append(states, s)
		}
	}
	if len(states) > 0 {
		h.Status, h.LastError = "degraded", strings.Join(states, "; ")
		return h
	}
	h.Healthy, h.Status = true, "running"
	return h
}

// DataFlow reports the CountInput totals since construction, running or not, and LastActivity,
// the time of the last CountInput or the zero time before the first. While running it also
// reports the totals per second of uptime and, once an input was counted, the counted errors per
// input; otherwise those rates are zero. DataFlow may be called at any time and never waits for
// Start or Stop.
func (l *Lifecycle) DataFlow() FlowMetrics {
	l.mu.Lock()
	defer l.mu.Unlock()
	f := FlowMetrics{Messages: l.messages, Bytes: l.bytes, LastActivity: l.lastActivity}
	if !l.running {
		return f
	}
	if secs := time.Since(l.startedAt).Seconds(); secs > 0 {
		f.MessagesPerSecond = float64(l.messages) / secs
		f.BytesPerSecond = float64(l.bytes) / secs
	}
	if l.messages > 0 {
		f.ErrorRate = float64(l.errorCount) / float64(l.messages)
	}
	return f
}

// CountInput counts one input message of the given payload bytes and records its time as the
// last activity.
func (l *Lifecycle) CountInput(bytes int) {
	l.mu.Lock()
	l.messages++
	l.bytes += int64(bytes)
	l.lastActivity = time.Now()
	l.mu.Unlock()
}

// CountError counts one processing error, reported as Health's ErrorCount and in DataFlow's
// ErrorRate.
func (l *Lifecycle) CountError() {
	l.mu.Lock()
	l.errorCount++
	l.mu.Unlock()
}
