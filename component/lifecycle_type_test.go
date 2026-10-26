package component_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/internal/harness/lifecycletest"
)

var (
	errStart   = errors.New("test start refused")
	errCleanup = errors.New("test cleanup refused")
)

// readBound is how long, on the real clock, every read in one test may take before the test
// reports that a read waited. A synctest bubble does not count a goroutine blocked on a mutex as
// durably blocked, so a read waiting on a lock held across start or cleanup would hang the binary
// until its timeout rather than deadlock the bubble (measured, notes/2.1.md). A read that does not
// wait returns in microseconds; ten seconds is slack for a loaded runner at one CPU under -race.
const readBound = 10 * time.Second

// realDeadline returns the Done channel of a context that ends readBound from now on the real
// clock. It must be called outside the bubble: a context's Done channel is made on first use, and
// one made inside a bubble cannot be closed by the real-clock timer outside it.
func realDeadline(t *testing.T) <-chan struct{} {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), readBound)
	t.Cleanup(cancel)
	return ctx.Done()
}

// held is a start or cleanup function the test holds open: it closes entered when it begins and
// returns err once the test lets it go. It is called at most once.
type held struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	err     error
}

// newHeld returns a held function and lets it go in t's cleanup, so a failed assertion never
// leaves a bubble goroutine waiting on release (which the bubble would report as a deadlock).
func newHeld(t *testing.T, err error) *held {
	h := &held{entered: make(chan struct{}), release: make(chan struct{}), err: err}
	t.Cleanup(h.let)
	return h
}

func (h *held) fn(context.Context) error {
	close(h.entered)
	<-h.release
	return h.err
}

func (h *held) let() { h.once.Do(func() { close(h.release) }) }

// goCall runs a lifecycle call that the holds keep from returning, and delivers its result. In
// t's cleanup it lets the holds go and waits for the call to return, so the terminal Stop never
// runs beside it.
func goCall(t *testing.T, call func() error, holds ...*held) <-chan error {
	result := make(chan error, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		result <- call()
	}()
	t.Cleanup(func() {
		for _, h := range holds {
			h.let()
		}
		<-finished
	})
	return result
}

// stopAtEnd stops l in t's cleanup under a fresh finite bound. Register it before any held
// function: cleanups run last-first, so the held functions are let go before this Stop waits.
func stopAtEnd(t *testing.T, l interface{ Stop(context.Context) error }) {
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := l.Stop(ctx); err != nil {
			t.Errorf("terminal Stop: %v", err)
		}
	})
}

func nop(context.Context) error { return nil }

func failStart(context.Context) error { return errStart }

func initialize(t *testing.T, l *component.Lifecycle) {
	t.Helper()
	if err := l.Initialize(); err != nil {
		t.Fatalf("Initialize = %v, want nil", err)
	}
}

// read takes both reads on a goroutine and returns them. A read still waiting at the real-clock
// deadline (see readBound) ends the test: the next call on the shell (a count, another read)
// would wait on the same lock, and t's cleanups let the held functions go, which frees it.
func read(t *testing.T, l *component.Lifecycle, deadline <-chan struct{}, stuck ...string) (component.HealthStatus, component.FlowMetrics) {
	t.Helper()
	type reads struct {
		health component.HealthStatus
		flow   component.FlowMetrics
	}
	done := make(chan reads, 1)
	go func() { done <- reads{l.Health(stuck...), l.DataFlow()} }()
	select {
	case r := <-done:
		return r.health, r.flow
	case <-deadline:
		t.Fatalf("Health and DataFlow had not returned after %s of real time: a read waited", readBound)
		return component.HealthStatus{}, component.FlowMetrics{}
	}
}

// wantHealth is the design's D4 table for one Health read.
func wantHealth(now time.Time, running bool, uptime time.Duration, errorCount int, stuck string) component.HealthStatus {
	h := component.HealthStatus{LastCheck: now, ErrorCount: errorCount, Status: "stopped"}
	switch {
	case !running:
	case stuck != "":
		h.Status, h.LastError, h.Uptime = "degraded", stuck, uptime
	default:
		h.Healthy, h.Status, h.Uptime = true, "running", uptime
	}
	return h
}

// sameHealth and sameFlow compare every field, the times with Equal.
func sameHealth(got, want component.HealthStatus) bool {
	if !got.LastCheck.Equal(want.LastCheck) {
		return false
	}
	got.LastCheck = want.LastCheck
	return got == want
}

func sameFlow(got, want component.FlowMetrics) bool {
	if !got.LastActivity.Equal(want.LastActivity) {
		return false
	}
	got.LastActivity = want.LastActivity
	return got == want
}

// shellOwner is a lifecycletest owner built only from a Lifecycle and one worker goroutine. Its
// start runs the worker under a run context of its own, which its cleanup ends and joins; what it
// holds is read from the worker itself, so the suite sees the release, not the shell's state.
type shellOwner struct {
	lifecycle component.Lifecycle
	// failStart makes start fail after the worker began; underStartCtx runs the worker under the
	// Start context instead of its own run context, the shape ADR-113 forbids.
	failStart     bool
	underStartCtx bool

	mu       sync.Mutex
	cancel   context.CancelFunc
	done     chan struct{}
	cleanups int
}

func newShellOwner(t *testing.T, failStart, underStartCtx bool) *shellOwner {
	o := &shellOwner{failStart: failStart, underStartCtx: underStartCtx}
	if err := o.lifecycle.Initialize(); err != nil {
		t.Errorf("Initialize = %v, want nil", err)
	}
	return o
}

func (o *shellOwner) Start(ctx context.Context) error {
	return o.lifecycle.Start(ctx, o.start, o.cleanup)
}

func (o *shellOwner) Stop(ctx context.Context) error { return o.lifecycle.Stop(ctx) }

func (o *shellOwner) start(ctx context.Context) error {
	var runCtx context.Context
	var cancel context.CancelFunc
	if o.underStartCtx {
		runCtx, cancel = context.WithCancel(ctx)
	} else {
		runCtx, cancel = context.WithCancel(context.WithoutCancel(ctx))
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		<-runCtx.Done()
	}()
	o.mu.Lock()
	o.cancel, o.done = cancel, done
	o.mu.Unlock()
	if o.failStart {
		return errStart
	}
	return nil
}

func (o *shellOwner) cleanup(ctx context.Context) error {
	o.mu.Lock()
	o.cleanups++
	cancel, done := o.cancel, o.done
	o.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}
	o.mu.Lock()
	o.cancel, o.done = nil, nil
	o.mu.Unlock()
	return nil
}

func (o *shellOwner) Observe() lifecycletest.Observation {
	o.mu.Lock()
	defer o.mu.Unlock()
	obs := lifecycletest.Observation{Calls: map[string]int{"cleanup": o.cleanups}}
	if o.done != nil {
		select {
		case <-o.done:
		default:
			obs.Unresolved = append(obs.Unresolved, "worker")
		}
	}
	return obs
}

// A shell-only owner passes the portable floor, the failed-start check on an owner whose start
// fails after its worker began. Real clock: lifecycletest.Run calls t.Run, which a bubble refuses.
//
// spec: component-lifecycle / The shell admits one Start after Initialize
func TestLifecycleOwnerPassesTheLifecycleSuite(t *testing.T) {
	lifecycletest.Run(t,
		func() lifecycletest.Owner { return newShellOwner(t, false, false) },
		func() lifecycletest.Owner { return newShellOwner(t, true, false) },
		lifecycletest.Promise{})
}

// Every history the shell can be in, read with and without stuck states (an empty one among
// them), with and without counted errors, and before and after counted inputs; every field of
// both reads is compared with the design's D4 table. Examples suffice: the reads depend only on
// which of these nine histories the shell is in and on the inputs, and the table lists all nine.
//
// spec: component-lifecycle / The shell is running from start's return until cleanup begins
// spec: component-lifecycle / Health reads running and the stuck states it is given
// spec: component-lifecycle / DataFlow reports counted inputs and their rates
func TestLifecycleReadsInEveryPhase(t *testing.T) {
	for _, row := range []struct {
		name    string
		running bool
		// setup puts the shell in its history at the bubble's start time.
		setup func(t *testing.T, l *component.Lifecycle)
	}{
		{"new", false, func(*testing.T, *component.Lifecycle) {}},
		{"start held open", false, func(t *testing.T, l *component.Lifecycle) {
			initialize(t, l)
			start := newHeld(t, nil)
			goCall(t, func() error { return l.Start(t.Context(), start.fn, nop) }, start)
			<-start.entered
		}},
		{"running", true, func(t *testing.T, l *component.Lifecycle) {
			initialize(t, l)
			if err := l.Start(t.Context(), nop, nop); err != nil {
				t.Fatalf("Start = %v", err)
			}
		}},
		{"start failed and rolled back", false, func(t *testing.T, l *component.Lifecycle) {
			initialize(t, l)
			if err := l.Start(t.Context(), failStart, nop); !errors.Is(err, errStart) {
				t.Fatalf("Start = %v, want the start error", err)
			}
		}},
		{"rollback cleanup held open", false, func(t *testing.T, l *component.Lifecycle) {
			initialize(t, l)
			cleanup := newHeld(t, nil)
			goCall(t, func() error { return l.Start(t.Context(), failStart, cleanup.fn) }, cleanup)
			<-cleanup.entered
		}},
		{"stop cleanup held open", false, func(t *testing.T, l *component.Lifecycle) {
			initialize(t, l)
			cleanup := newHeld(t, nil)
			if err := l.Start(t.Context(), nop, cleanup.fn); err != nil {
				t.Fatalf("Start = %v", err)
			}
			goCall(t, func() error { return l.Stop(t.Context()) }, cleanup)
			<-cleanup.entered
		}},
		{"after a stop whose cleanup failed", false, func(t *testing.T, l *component.Lifecycle) {
			initialize(t, l)
			failed := false
			// The first cleanup fails; the terminal Stop's retry succeeds.
			cleanup := func(context.Context) error {
				if failed {
					return nil
				}
				failed = true
				return errCleanup
			}
			if err := l.Start(t.Context(), nop, cleanup); err != nil {
				t.Fatalf("Start = %v", err)
			}
			if err := l.Stop(t.Context()); !errors.Is(err, errCleanup) {
				t.Fatalf("Stop = %v, want the cleanup error", err)
			}
		}},
		{"after a stop that succeeded", false, func(t *testing.T, l *component.Lifecycle) {
			initialize(t, l)
			if err := l.Start(t.Context(), nop, nop); err != nil {
				t.Fatalf("Start = %v", err)
			}
			if err := l.Stop(t.Context()); err != nil {
				t.Fatalf("Stop = %v", err)
			}
		}},
		{"stop before start", false, func(t *testing.T, l *component.Lifecycle) {
			initialize(t, l)
			if err := l.Stop(t.Context()); err != nil {
				t.Fatalf("Stop = %v", err)
			}
		}},
	} {
		t.Run(row.name, func(t *testing.T) {
			deadline := realDeadline(t)
			synctest.Test(t, func(t *testing.T) {
				var l component.Lifecycle
				stopAtEnd(t, &l)
				t0 := time.Now()
				row.setup(t, &l)

				check := func(stage string, uptime time.Duration, errorCount int, wantFlow component.FlowMetrics) {
					t.Helper()
					for _, s := range []struct {
						states []string
						joined string
					}{
						{nil, ""},
						{[]string{""}, ""},
						{[]string{"poison", "", "backlog fault"}, "poison; backlog fault"},
					} {
						health, flow := read(t, &l, deadline, s.states...)
						want := wantHealth(time.Now(), row.running, uptime, errorCount, s.joined)
						if !sameHealth(health, want) {
							t.Errorf("%s, Health(%q) = %+v, want %+v", stage, s.states, health, want)
						}
						if !sameFlow(flow, wantFlow) {
							t.Errorf("%s, DataFlow() = %+v, want %+v", stage, flow, wantFlow)
						}
					}
				}
				// rates is wantFlow with the rates set only when the row is running.
				rates := func(f component.FlowMetrics, mps, bps, errorRate float64) component.FlowMetrics {
					if row.running {
						f.MessagesPerSecond, f.BytesPerSecond, f.ErrorRate = mps, bps, errorRate
					}
					return f
				}

				<-time.After(2 * time.Second)
				synctest.Wait()
				check("nothing counted", 2*time.Second, 0, component.FlowMetrics{})

				l.CountError()
				l.CountError()
				// Errors with no input: ErrorRate stays zero and Healthy stays as running says.
				check("two errors, no input", 2*time.Second, 2, component.FlowMetrics{})

				for range 4 {
					l.CountInput(100)
				}
				lastInput := t0.Add(2 * time.Second)
				<-time.After(2 * time.Second)
				synctest.Wait()
				// Four inputs of 100 bytes over a 4 s uptime, two errors per four inputs.
				check("four inputs and two errors", 4*time.Second, 2,
					rates(component.FlowMetrics{Messages: 4, Bytes: 400, LastActivity: lastInput}, 1, 100, 0.5))
			})
		})
	}
}

// Both reads return while start is held open and again while a Stop's cleanup is held open.
//
// spec: component-lifecycle / The shell's reads never wait
func TestLifecycleReadsDoNotWaitForStartOrStop(t *testing.T) {
	deadline := realDeadline(t)
	synctest.Test(t, func(t *testing.T) {
		var l component.Lifecycle
		stopAtEnd(t, &l)
		initialize(t, &l)
		start, cleanup := newHeld(t, nil), newHeld(t, nil)

		started := goCall(t, func() error { return l.Start(t.Context(), start.fn, cleanup.fn) }, start)
		<-start.entered
		read(t, &l, deadline)
		start.let()
		if err := <-started; err != nil {
			t.Fatalf("Start = %v", err)
		}

		stopped := goCall(t, func() error { return l.Stop(t.Context()) }, cleanup)
		<-cleanup.entered
		read(t, &l, deadline)
		cleanup.let()
		if err := <-stopped; err != nil {
			t.Fatalf("Stop = %v", err)
		}
	})
}

// A Start before Initialize is refused and runs nothing; after Initialize (nil every time) a
// Start is admitted, so the refused one did not use the shell up.
//
// spec: component-lifecycle / The shell admits one Start after Initialize
func TestLifecycleStartBeforeInitializeIsRefusedAndLeavesItUnused(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var l component.Lifecycle
		stopAtEnd(t, &l)
		starts, cleanups := 0, 0
		start := func(context.Context) error { starts++; return nil }
		cleanup := func(context.Context) error { cleanups++; return nil }

		if err := l.Start(t.Context(), start, cleanup); err == nil {
			t.Fatal("Start before Initialize = nil, want a refusal")
		}
		if starts != 0 || cleanups != 0 {
			t.Fatalf("refused Start ran start %d and cleanup %d time(s), want neither", starts, cleanups)
		}
		if h := l.Health(); h.Healthy || h.Status != "stopped" {
			t.Fatalf("after the refused Start, Health = %+v, want stopped", h)
		}

		for range 2 {
			initialize(t, &l)
		}
		if err := l.Start(t.Context(), start, cleanup); err != nil {
			t.Fatalf("Start after Initialize = %v, want nil", err)
		}
		if h := l.Health(); starts != 1 || !h.Healthy || h.Status != "running" {
			t.Fatalf("after the admitted Start, start ran %d time(s) and Health = %+v; want once and running", starts, h)
		}
		if err := l.Stop(t.Context()); err != nil || cleanups != 1 {
			t.Fatalf("Stop = %v with %d cleanup(s), want nil and one", err, cleanups)
		}
	})
}

// No read taken while a failing start runs, while its rollback runs or after Start returned
// reports running, and the same holds for a start that panics. One input is counted before Start
// and the clock moved before each read, so a shell wrongly running would also report an uptime and
// rates.
//
// spec: component-lifecycle / The shell is running from start's return until cleanup begins
func TestLifecycleFailedStartNeverRuns(t *testing.T) {
	deadline := realDeadline(t)
	synctest.Test(t, func(t *testing.T) {
		requireStopped := func(l *component.Lifecycle, when string) {
			t.Helper()
			<-time.After(time.Second)
			synctest.Wait()
			health, flow := read(t, l, deadline)
			if health.Healthy || health.Status != "stopped" || health.Uptime != 0 {
				t.Errorf("%s, Health = %+v, want stopped with no uptime", when, health)
			}
			if flow.MessagesPerSecond != 0 || flow.BytesPerSecond != 0 || flow.ErrorRate != 0 {
				t.Errorf("%s, DataFlow = %+v, want zero rates", when, flow)
			}
		}

		var l component.Lifecycle
		stopAtEnd(t, &l)
		initialize(t, &l)
		l.CountInput(10)
		start, cleanup := newHeld(t, errStart), newHeld(t, nil)
		result := goCall(t, func() error { return l.Start(t.Context(), start.fn, cleanup.fn) }, start, cleanup)
		<-start.entered
		requireStopped(&l, "while the failing start runs")
		start.let()
		<-cleanup.entered
		requireStopped(&l, "while its rollback runs")
		cleanup.let()
		if err := <-result; !errors.Is(err, errStart) {
			t.Fatalf("Start = %v, want the start error", err)
		}
		requireStopped(&l, "after the failed Start returned")

		var p component.Lifecycle
		stopAtEnd(t, &p)
		initialize(t, &p)
		p.CountInput(10)
		func() {
			defer func() { _ = recover() }() // a panic out of Start is not under test; what it leaves is
			if err := p.Start(t.Context(), func(context.Context) error { panic("start panicked") }, nop); err == nil {
				t.Error("Start with a panicking start = nil, want an error or the panic")
			}
		}()
		requireStopped(&p, "after a start that panicked")
	})
}

// The shell cannot stop a start from running its worker under the Start context, but the suite
// catches it on the component's own run: CheckStartContextEndStopsNothing, called directly inside
// one bubble, where its settle window is exact, fails such an owner and passes the owner whose
// worker runs under its own run context.
//
// spec: component-lifecycle / The shell holds no context
func TestLifecycleSuiteCatchesWorkUnderTheStartContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		good := newShellOwner(t, false, false)
		stopAtEnd(t, good)
		if err := lifecycletest.CheckStartContextEndStopsNothing(t.Context(), good); err != nil {
			t.Errorf("owner with its own run context: check = %v, want nil", err)
		}

		bad := newShellOwner(t, false, true)
		stopAtEnd(t, bad)
		err := lifecycletest.CheckStartContextEndStopsNothing(t.Context(), bad)
		if err == nil || !strings.Contains(err.Error(), "stopped holding [worker]") {
			t.Errorf("owner whose worker runs under the Start context: check = %v, want it to report the worker gone", err)
		}
	})
}
