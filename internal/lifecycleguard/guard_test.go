package lifecycleguard

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"

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
	return o.g.Stop(ctx, o.release)
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

// A Stop that arrives while Start is still acquiring waits for that Start to finish and then
// releases what it acquired; a Stop whose own context ends first returns that context's error.
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
			}, func(context.Context) error { return nil })
		}()
		<-entered

		short, cancel := context.WithCancel(t.Context())
		stopped := make(chan error, 1)
		go func() { stopped <- g.Stop(short, func(context.Context) error { released++; return nil }) }()
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

		go func() { stopped <- g.Stop(t.Context(), func(context.Context) error { released++; return nil }) }()
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

// A Start whose acquisition panics still rolls back what it acquired before the panic continues.
func TestGuardRollsBackAPanickingStart(t *testing.T) {
	var g Guard
	rolledBack := false
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("the start function's panic was swallowed")
			}
		}()
		_ = g.Start(t.Context(), func(context.Context) error { panic("acquire") },
			func(context.Context) error { rolledBack = true; return nil })
	}()
	if !rolledBack {
		t.Fatal("a panicking Start did not roll back")
	}
	if err := g.Stop(t.Context(), func(context.Context) error {
		t.Fatal("Stop ran cleanup after a rollback that succeeded")
		return nil
	}); err != nil {
		t.Fatalf("Stop after the rolled-back Start = %v", err)
	}
}

// Stop before Start uses up the guard: a later Start is refused before acquiring anything.
func TestGuardStopBeforeStartRefusesLaterStart(t *testing.T) {
	var g Guard
	if err := g.Stop(t.Context(), func(context.Context) error {
		t.Fatal("Stop before Start ran cleanup")
		return nil
	}); err != nil {
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
