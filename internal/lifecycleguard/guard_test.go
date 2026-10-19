package lifecycleguard

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/c360studio/semengine/internal/harness/lifecycletest"
	"github.com/c360studio/semengine/pkg/errs"
)

var (
	errAcquire = errors.New("test owner: acquire refused")
	errRelease = errors.New("test owner: release refused")
)

// owner is a test owner built only from the guard: Start acquires one worker goroutine under the
// guard and Stop releases it under the guard. What it holds is read from the worker itself, so the
// suite's checks see the release, not the guard's bookkeeping.
type owner struct {
	g Guard
	// failStart makes start fail after the worker is acquired; failRelease makes every release
	// fail while it is set, without releasing.
	failStart   bool
	failRelease bool

	mu       sync.Mutex
	stop     chan struct{}
	done     chan struct{}
	releases int
}

func (o *owner) Start(ctx context.Context) error {
	return o.g.Start(ctx, o.acquire, o.release)
}

func (o *owner) Stop(ctx context.Context) error {
	return o.g.Stop(ctx)
}

func (o *owner) acquire(context.Context) error {
	o.mu.Lock()
	o.stop, o.done = make(chan struct{}), make(chan struct{})
	stop, done := o.stop, o.done
	o.mu.Unlock()
	go func() {
		defer close(done)
		<-stop
	}()
	if o.failStart {
		return errAcquire
	}
	return nil
}

// release ends the worker and waits for it under ctx; it keeps the worker on record when it fails.
func (o *owner) release(ctx context.Context) error {
	o.mu.Lock()
	o.releases++
	if o.failRelease {
		o.mu.Unlock()
		return errRelease
	}
	stop, done := o.stop, o.done
	o.mu.Unlock()
	if stop == nil {
		return nil
	}
	select {
	case <-stop:
	default:
		close(stop)
	}
	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}
	o.mu.Lock()
	o.stop, o.done = nil, nil
	o.mu.Unlock()
	return nil
}

func (o *owner) Observe() lifecycletest.Observation {
	o.mu.Lock()
	defer o.mu.Unlock()
	obs := lifecycletest.Observation{Calls: map[string]int{"release": o.releases}}
	if o.done != nil {
		select {
		case <-o.done:
		default:
			obs.Unresolved = append(obs.Unresolved, "worker")
		}
	}
	return obs
}

// The guard alone gives an owner the portable floor: run with a factory whose Start fails after
// acquiring the worker and whose rollback releases it.
//
// Requirement: lifecycle-suite/Portable floor
func TestGuardOwnerPassesTheLifecycleSuite(t *testing.T) {
	lifecycletest.Run(t,
		func() lifecycletest.Owner { return &owner{} },
		func() lifecycletest.Owner { return &owner{failStart: true} },
		lifecycletest.Promise{})
}

// A failed Start whose rollback also fails reports both errors and keeps the cleanup pending: the
// next Stop retries it, and once that succeeds the owner holds nothing and further Stops are no-ops.
//
// Requirement: lifecycle-suite/Failed start whose own cleanup fails
func TestGuardFailedRollbackLeavesCleanupForStop(t *testing.T) {
	o := &owner{failStart: true, failRelease: true}
	err := o.Start(t.Context())
	if !errors.Is(err, errAcquire) || !errors.Is(err, errRelease) {
		t.Fatalf("Start = %v, want both the start error and the rollback error", err)
	}
	if got := o.Observe().Unresolved; len(got) != 1 {
		t.Fatalf("after the failed rollback the owner holds %v, want the worker kept on record", got)
	}
	if err := o.Start(t.Context()); !errors.Is(err, errs.ErrAlreadyStarted) {
		t.Fatalf("Start with cleanup pending = %v, want ErrAlreadyStarted", err)
	}
	if err := o.Stop(t.Context()); !errors.Is(err, errRelease) {
		t.Fatalf("Stop while release still fails = %v, want the release error", err)
	}
	o.mu.Lock()
	o.failRelease = false
	o.mu.Unlock()
	if err := o.Stop(t.Context()); err != nil {
		t.Fatalf("Stop once release succeeds = %v, want nil", err)
	}
	if got := o.Observe().Unresolved; len(got) != 0 {
		t.Fatalf("after the retried cleanup the owner holds %v", got)
	}
	before := o.Observe()
	if err := o.Stop(t.Context()); err != nil || o.Observe().Calls["release"] != before.Calls["release"] {
		t.Fatalf("repeated Stop = %v, releases %d -> %d; want nil and no call", err, before.Calls["release"], o.Observe().Calls["release"])
	}
}

// A running owner whose Stop fails keeps its cleanup pending: each later Stop runs cleanup again
// and returns its result, and Stop returns nil and calls nothing only after a cleanup succeeded.
//
// Requirement: lifecycle-suite/Portable floor
func TestGuardFailedStopLeavesCleanupForNextStop(t *testing.T) {
	o := &owner{}
	if err := o.Start(t.Context()); err != nil {
		t.Fatalf("Start = %v", err)
	}
	o.mu.Lock()
	o.failRelease = true
	o.mu.Unlock()
	for want := 1; want <= 2; want++ {
		if err := o.Stop(t.Context()); !errors.Is(err, errRelease) {
			t.Fatalf("Stop %d while release fails = %v, want the release error", want, err)
		}
		if obs := o.Observe(); len(obs.Unresolved) != 1 || obs.Calls["release"] != want {
			t.Fatalf("after failed Stop %d the owner holds %v with %d release call(s); want the worker and %d",
				want, obs.Unresolved, obs.Calls["release"], want)
		}
	}
	o.mu.Lock()
	o.failRelease = false
	o.mu.Unlock()
	if err := o.Stop(t.Context()); err != nil {
		t.Fatalf("Stop once release succeeds = %v, want nil", err)
	}
	if obs := o.Observe(); len(obs.Unresolved) != 0 || obs.Calls["release"] != 3 {
		t.Fatalf("after the successful Stop the owner holds %v with %d release call(s); want nothing and 3",
			obs.Unresolved, obs.Calls["release"])
	}
	if err := o.Stop(t.Context()); err != nil || o.Observe().Calls["release"] != 3 {
		t.Fatalf("Stop after the successful one = %v with %d release call(s); want nil and still 3",
			err, o.Observe().Calls["release"])
	}
}

// A Stop that arrives while Start is still acquiring waits for that Start to finish and then
// runs the cleanup that Start was given; a Stop whose own context ends first returns that
// context's error.
//
// Requirement: lifecycle-suite/Portable floor
func TestGuardStopWaitsForAnInFlightStart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var g Guard
		entered, proceed := make(chan struct{}), make(chan struct{})
		released := 0
		started := make(chan error, 1)
		go func() {
			started <- g.Start(t.Context(), func(context.Context) error {
				close(entered)
				<-proceed
				return nil
			}, func(context.Context) error { released++; return nil })
		}()
		<-entered

		short, cancel := context.WithCancel(t.Context())
		stopped := make(chan error, 1)
		go func() { stopped <- g.Stop(short) }()
		synctest.Wait()
		select {
		case err := <-stopped:
			t.Fatalf("Stop returned %v while Start was still in flight", err)
		default:
		}
		cancel()
		if err := <-stopped; !errors.Is(err, context.Canceled) {
			t.Fatalf("Stop whose context ended first = %v, want context.Canceled", err)
		}

		go func() { stopped <- g.Stop(t.Context()) }()
		synctest.Wait()
		close(proceed)
		if err := <-started; err != nil {
			t.Fatalf("Start = %v", err)
		}
		if err := <-stopped; err != nil || released != 1 {
			t.Fatalf("Stop after the in-flight Start = %v with %d release(s); want nil and one", err, released)
		}
	})
}

// A Start whose acquisition panics still rolls back what it acquired before the panic continues,
// and once that rollback has succeeded a Stop does not run the cleanup again: it runs once in all.
func TestGuardRollsBackAPanickingStart(t *testing.T) {
	var g Guard
	cleanups := 0
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("the start function's panic was swallowed")
			}
		}()
		_ = g.Start(t.Context(), func(context.Context) error { panic("acquire") },
			func(context.Context) error { cleanups++; return nil })
	}()
	if cleanups != 1 {
		t.Fatalf("a panicking Start ran its cleanup %d time(s), want one rollback", cleanups)
	}
	if err := g.Stop(t.Context()); err != nil {
		t.Fatalf("Stop after the rolled-back Start = %v", err)
	}
	if cleanups != 1 {
		t.Fatalf("Start's rollback and a later Stop ran the cleanup %d time(s) in all, want 1", cleanups)
	}
}

// Stop before Start uses up the guard: a later Start is refused before acquiring anything.
func TestGuardStopBeforeStartRefusesLaterStart(t *testing.T) {
	var g Guard
	if err := g.Stop(t.Context()); err != nil {
		t.Fatalf("Stop before Start = %v", err)
	}
	err := g.Start(t.Context(), func(context.Context) error {
		t.Fatal("Start after Stop acquired")
		return nil
	}, func(context.Context) error { return nil })
	if !errors.Is(err, errs.ErrAlreadyStarted) {
		t.Fatalf("Start after Stop = %v, want ErrAlreadyStarted", err)
	}
}

// Start and Stop refuse a nil context alike, as invalid data, before running anything. They answer
// an ended context differently on purpose: Start refuses it as invalid, since a Start given an ended
// context cannot begin, and Stop returns the context's own error, as the tree's other stop methods
// do. A refusal leaves the guard as it was: after a refused Start a live Start is admitted, and
// after a refused Stop a live Stop runs the cleanup once.
func TestGuardRefusesNilAndEndedContexts(t *testing.T) {
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	expired, cancelExpired := context.WithDeadline(t.Context(), time.Unix(0, 0))
	defer cancelExpired()

	invalidMatching := func(target error) func(error) bool {
		return func(err error) bool { return errs.IsInvalid(err) && errors.Is(err, target) }
	}
	contextsOwn := func(target error) func(error) bool {
		// The context's own error, unwrapped, as Stop's doc promises: identity, not errors.Is.
		return func(err error) bool { return err == target }
	}
	rows := []struct {
		method   string // "Start" on a fresh guard, or "Stop" on a started one
		ctxName  string
		ctx      context.Context
		requires func(error) bool
		want     string // what requires checks, as the failure line words it
	}{
		{"Start", "nil", nil, invalidMatching(errs.ErrInvalidData),
			"want an invalid refusal matching errs.ErrInvalidData"},
		{"Start", "cancelled", cancelled, invalidMatching(context.Canceled),
			fmt.Sprintf("want an invalid refusal matching %v", context.Canceled)},
		{"Start", "expired", expired, invalidMatching(context.DeadlineExceeded),
			fmt.Sprintf("want an invalid refusal matching %v", context.DeadlineExceeded)},
		{"Stop", "nil", nil, invalidMatching(errs.ErrInvalidData),
			"want an invalid refusal matching errs.ErrInvalidData"},
		{"Stop", "cancelled", cancelled, contextsOwn(context.Canceled),
			fmt.Sprintf("want the context's own error %v", context.Canceled)},
		{"Stop", "expired", expired, contextsOwn(context.DeadlineExceeded),
			fmt.Sprintf("want the context's own error %v", context.DeadlineExceeded)},
	}
	nilErrs := map[string]error{}
	for _, r := range rows {
		var g Guard
		call := r.method + "(" + r.ctxName + ")"
		runs := 0 // runs of start in a Start row, of the cleanup in a Stop row
		counted := func(context.Context) error { runs++; return nil }
		none := func(context.Context) error { return nil }

		var err error
		if r.method == "Start" {
			err = g.Start(r.ctx, counted, none)
		} else {
			if err := g.Start(t.Context(), none, counted); err != nil {
				t.Fatalf("Start before %s = %v", call, err)
			}
			err = g.Stop(r.ctx)
		}
		if r.ctx == nil {
			nilErrs[r.method] = err
		}
		if !r.requires(err) {
			t.Errorf("%s = %v, %s", call, err, r.want)
		}
		if runs != 0 {
			t.Errorf("refused %s ran its function %d time(s), want none", call, runs)
		}

		if r.method == "Start" {
			err = g.Start(t.Context(), counted, none)
		} else {
			err = g.Stop(t.Context())
		}
		if err != nil || runs != 1 {
			t.Errorf("live %s after %s = %v with %d run(s), want nil and one", r.method, call, err, runs)
		}
	}

	startMsg, stopMsg := fmt.Sprint(nilErrs["Start"]), fmt.Sprint(nilErrs["Stop"])
	if strings.ReplaceAll(startMsg, "Start", "Stop") != stopMsg {
		t.Errorf("Start(nil) and Stop(nil) messages %q and %q differ beyond the method name", startMsg, stopMsg)
	}
}

// A Stop that arrives while another Stop runs the cleanup is refused as transient at once, without
// running the cleanup; the first Stop then finishes, and a later Stop returns nil and runs nothing.
// The test learns that the first Stop is inside the cleanup from the cleanup's own signal, not from
// the guard's state.
func TestGuardRefusesAConcurrentStop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var g Guard
		var runs atomic.Int32
		entered, release := make(chan struct{}), make(chan struct{})
		cleanup := func(context.Context) error {
			if runs.Add(1) == 1 {
				close(entered)
				<-release
			}
			return nil
		}
		if err := g.Start(t.Context(), func(context.Context) error { return nil }, cleanup); err != nil {
			t.Fatalf("Start = %v", err)
		}

		stopA := make(chan error, 1)
		go func() { stopA <- g.Stop(t.Context()) }()
		<-entered

		stopB := make(chan error, 1)
		go func() { stopB <- g.Stop(t.Context()) }()
		synctest.Wait()
		select {
		case err := <-stopB:
			if !errs.IsTransient(err) {
				t.Errorf("Stop while another Stop runs cleanup = %v, want a transient refusal", err)
			}
		default:
			t.Errorf("second Stop did not return while the first ran cleanup")
		}
		if n := runs.Load(); n != 1 {
			t.Errorf("cleanup ran %d time(s), want 1", n)
		}

		close(release)
		if err := <-stopA; err != nil {
			t.Errorf("first Stop = %v, want nil", err)
		}
		if err := g.Stop(t.Context()); err != nil {
			t.Errorf("third Stop = %v, want nil", err)
		}
		if n := runs.Load(); n != 1 {
			t.Errorf("cleanup ran %d time(s), want 1", n)
		}
	})
}

// Stop runs the cleanup the admitted Start was given. A second Start is refused and records
// nothing, so its cleanup never runs: Stop runs the first Start's cleanup once, and a repeated
// Stop runs nothing.
func TestGuardStopRunsTheCleanupStartWasGiven(t *testing.T) {
	var g Guard
	callsA, callsB := 0, 0
	acquire := func(context.Context) error { return nil }
	if err := g.Start(t.Context(), acquire, func(context.Context) error { callsA++; return nil }); err != nil {
		t.Fatalf("first Start = %v, want nil", err)
	}
	err := g.Start(t.Context(), acquire, func(context.Context) error { callsB++; return nil })
	if !errors.Is(err, errs.ErrAlreadyStarted) {
		t.Errorf("second Start = %v, want ErrAlreadyStarted", err)
	}

	if err := g.Stop(t.Context()); err != nil {
		t.Errorf("Stop = %v, want nil", err)
	}
	// B's count first, and t.Errorf for both: the refused Start's line prints whatever else fails.
	if callsB != 0 {
		t.Errorf("Stop ran the refused Start's cleanup %d time(s), want 0", callsB)
	}
	if callsA != 1 {
		t.Errorf("Stop ran the admitted Start's cleanup %d time(s), want 1", callsA)
	}

	if err := g.Stop(t.Context()); err != nil || callsA != 1 || callsB != 0 {
		t.Errorf("repeated Stop = %v with calls A %d, B %d, want nil, 1, 0", err, callsA, callsB)
	}
}
