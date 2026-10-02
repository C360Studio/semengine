// Package lifecycletest is the portable owner-lifecycle floor (openspec change
// setup-02-isolated-harness, capability lifecycle-suite), adapted from SemStreams
// component/lifecycle_test_suite.go at 5457b345 (ledger row L4) without testify, without aggregate
// goroutine or memory counts (they cannot prove an owner's join), and as error-returning checks so
// each one's sensitivity is testable against a failpoint double.
//
// The floor is not proof of an owner's drain and join protocol; each owner still needs focused
// tests for its blocked callbacks, partial startup, and finalisation. Test-only: contract test T-B1
// refuses any production import.
package lifecycletest

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"
)

// Bounds for the checks' own calls. stopBound is the fresh finite authority a controlled Stop gets;
// startBound caps a Start that never returns; grace is how long past a call's own deadline the check
// waits before reporting that the owner ignored it. They bound failure, not the healthy path.
const (
	startBound = 2 * time.Minute
	stopBound  = 30 * time.Second
	grace      = 2 * time.Second
)

// Owner is anything with a Start/Stop lifecycle whose retained state the checks can read. Observe
// is required at compile time: the checks that judge completion (a nil Stop released everything; a
// repeated Stop changed nothing; a refused call acquired nothing) need it, because a returned error
// alone is not evidence of a join, and an optional interface found missing at run time is the
// silent-skip shape the floor exists to prevent.
type Owner interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Observer
}

// Observer reports an owner's retained state.
type Observer interface {
	Observe() Observation
}

// Observation is an owner's retained state at one instant: the resources it still holds, and a
// count per operation it has performed (cleanup runs, Docker calls), which a no-op must not change.
type Observation struct {
	Unresolved []string
	Calls      map[string]int
}

// Promise states what an owner promises beyond the floor.
type Promise struct {
	// Restart promises that a stopped owner accepts a second full Start/Stop cycle. Without it, a
	// second Start must be refused.
	Restart bool
}

// Factory returns a fresh, unstarted owner. Run calls it once per check.
type Factory func() Owner

var errAbortCause = errors.New("lifecycletest: abort")

// Run drives every check as a subtest against a fresh owner, then stops that owner under a fresh
// bound derived from the test's context so a check that left it running leaks nothing.
func Run(t *testing.T, factory Factory, promise Promise) {
	t.Helper()
	for _, c := range []struct {
		name  string
		check func(context.Context, Owner) error
	}{
		{"NilContextsRefused", CheckNilContextsRefused},
		{"PreCancelledStartRefused", CheckPreCancelledStartRefused},
		{"StopBeforeStartSafe", CheckStopBeforeStartSafe},
		{"ControlledStopUnderLiveStartAuthority", CheckControlledStopUnderLiveStartAuthority},
		{"AbortStopPreservesCause", CheckAbortStopPreservesCause},
		{"RepeatedStopIsNoOp", CheckRepeatedStopIsNoOp},
		{"SecondStartRefusedOrRestartCycle", func(ctx context.Context, o Owner) error {
			return CheckSecondStartRefusedOrRestartCycle(ctx, o, promise)
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			o := factory()
			if isNil(o) {
				t.Fatal("factory returned a nil owner")
			}
			if err := c.check(t.Context(), o); err != nil {
				t.Error(err)
			}
			if err := finish(t.Context(), o); err != nil {
				t.Errorf("terminal Stop after %s: %v", c.name, err)
			}
		})
	}
}

// finish is Run's terminal Stop: fresh finite authority derived from the still-live test context.
func finish(ctx context.Context, o Owner) error {
	stopCtx, cancel := context.WithTimeout(ctx, stopBound)
	defer cancel()
	return stop(ctx, stopCtx, o)
}

// CheckNilContextsRefused checks that Start(nil) and Stop(nil) return errors and acquire nothing. A nil
// context silently promoted to a root is how an owner loses its caller's cancellation.
func CheckNilContextsRefused(ctx context.Context, o Owner) error {
	var nilCtx context.Context
	if err := call(ctx, o, "Start(nil)", startBound, func() error { return o.Start(nilCtx) }); err == nil {
		return errors.New("Start(nil) returned nil; want an error")
	} else if notARefusal(err) {
		return err
	}
	if err := call(ctx, o, "Stop(nil)", stopBound, func() error { return o.Stop(nilCtx) }); err == nil {
		return errors.New("Stop(nil) returned nil; want an error")
	} else if notARefusal(err) {
		return err
	}
	return requireNothingRetained(o, "after refusing nil contexts")
}

// CheckPreCancelledStartRefused checks that a Start whose context is already cancelled, then one already past
// its deadline, is refused with the matching context error before acting. A refusal before acting
// consumes nothing: the second Start is judged on its own context, and Stop afterwards is safe.
func CheckPreCancelledStartRefused(ctx context.Context, o Owner) error {
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := call(ctx, o, "Start(cancelled)", startBound, func() error { return o.Start(cancelled) }); !errors.Is(err, context.Canceled) {
		return fmt.Errorf("Start with a cancelled context returned %v; want context.Canceled", err)
	}
	expired, cancelExpired := context.WithDeadline(ctx, time.Now().Add(-time.Second))
	defer cancelExpired()
	if err := call(ctx, o, "Start(expired)", startBound, func() error { return o.Start(expired) }); !errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("Start with an expired context returned %v; want context.DeadlineExceeded", err)
	}
	if err := requireNothingRetained(o, "after refused Starts"); err != nil {
		return err
	}
	if err := finish(ctx, o); err != nil {
		return fmt.Errorf("Stop after refused Starts: %w", err)
	}
	return nil
}

// CheckStopBeforeStartSafe checks that Stop on an owner never started returns nil, without panicking, and
// leaves nothing retained.
func CheckStopBeforeStartSafe(ctx context.Context, o Owner) error {
	if err := finish(ctx, o); err != nil {
		return fmt.Errorf("Stop before Start: %w", err)
	}
	return requireNothingRetained(o, "after Stop before Start")
}

// CheckControlledStopUnderLiveStartAuthority checks that with Start's context still live, Stop under fresh
// finite authority returns nil within its bound and has released everything. The check never
// cancels Start authority first; an owner that needs its caller to cancel before it can stop is
// not controlled.
func CheckControlledStopUnderLiveStartAuthority(ctx context.Context, o Owner) error {
	startCtx, cancelStart := context.WithCancel(ctx)
	defer cancelStart()
	if err := call(ctx, o, "Start", startBound, func() error { return o.Start(startCtx) }); err != nil {
		return fmt.Errorf("Start: %w", err)
	}
	if err := finish(ctx, o); err != nil {
		return fmt.Errorf("controlled Stop: %w", err)
	}
	if startCtx.Err() != nil {
		return fmt.Errorf("Start authority ended during controlled Stop: %w", startCtx.Err())
	}
	return requireNothingRetained(o, "after a controlled Stop returned nil")
}

// CheckAbortStopPreservesCause checks that after Start authority is cancelled (abort), a Stop handed an
// already-ended context returns promptly with an error carrying that context's error or cause. A
// nil return would claim a completed join the owner had no authority to wait for; not returning at
// all is reported with the owner's retained state instead of hanging the test.
func CheckAbortStopPreservesCause(ctx context.Context, o Owner) error {
	startCtx, cancelStart := context.WithCancel(ctx)
	if err := call(ctx, o, "Start", startBound, func() error { return o.Start(startCtx) }); err != nil {
		cancelStart()
		return fmt.Errorf("Start: %w", err)
	}
	cancelStart()
	stopCtx, cancelStop := context.WithCancelCause(ctx)
	cancelStop(errAbortCause)
	err := stop(ctx, stopCtx, o)
	var bound *boundError
	switch {
	case errors.As(err, &bound):
		return err
	case !errors.Is(err, stopCtx.Err()) && !errors.Is(err, context.Cause(stopCtx)):
		// Includes a nil Stop, which stop already reports as a claimed join past the bound.
		return fmt.Errorf("abort Stop did not preserve its context's %v: %w", stopCtx.Err(), err)
	}
	return nil
}

// CheckRepeatedStopIsNoOp checks that after a Stop that returned nil, a second Stop returns nil within its
// bound and changes nothing the owner reports.
func CheckRepeatedStopIsNoOp(ctx context.Context, o Owner) error {
	startCtx, cancelStart := context.WithCancel(ctx)
	defer cancelStart()
	if err := call(ctx, o, "Start", startBound, func() error { return o.Start(startCtx) }); err != nil {
		return fmt.Errorf("Start: %w", err)
	}
	if err := finish(ctx, o); err != nil {
		return fmt.Errorf("first Stop: %w", err)
	}
	before := o.Observe()
	if err := finish(ctx, o); err != nil {
		return fmt.Errorf("repeated Stop: %w", err)
	}
	after := o.Observe()
	if !reflect.DeepEqual(before, after) {
		return fmt.Errorf("repeated Stop was not a no-op: state before %+v, after %+v", before, after)
	}
	return nil
}

// CheckSecondStartRefusedOrRestartCycle checks that without a restart promise, a second Start is refused and
// leaves the running owner's state untouched; with one, a full second Start/Stop cycle succeeds.
func CheckSecondStartRefusedOrRestartCycle(ctx context.Context, o Owner, promise Promise) error {
	startCtx, cancelStart := context.WithCancel(ctx)
	defer cancelStart()
	if err := call(ctx, o, "Start", startBound, func() error { return o.Start(startCtx) }); err != nil {
		return fmt.Errorf("first Start: %w", err)
	}
	if promise.Restart {
		if err := finish(ctx, o); err != nil {
			return fmt.Errorf("first Stop: %w", err)
		}
		if err := call(ctx, o, "Start", startBound, func() error { return o.Start(startCtx) }); err != nil {
			return fmt.Errorf("restart was promised but the second Start returned %w", err)
		}
		if err := finish(ctx, o); err != nil {
			return fmt.Errorf("second Stop: %w", err)
		}
		return nil
	}
	before := o.Observe()
	if err := call(ctx, o, "Start", startBound, func() error { return o.Start(startCtx) }); err == nil {
		return errors.New("second Start returned nil without a restart promise; want a refusal")
	} else if notARefusal(err) {
		return err
	}
	after := o.Observe()
	if !reflect.DeepEqual(before, after) {
		return fmt.Errorf("refused second Start changed state: before %+v, after %+v", before, after)
	}
	return nil
}

// stop calls Stop under stopCtx and holds it to that context: a nil return after stopCtx ended is
// not a completed join. The wait itself runs under the check's parent, which stays live even when
// stopCtx is handed over already ended.
func stop(parent, stopCtx context.Context, o Owner) error {
	bound := stopBound
	if deadline, ok := stopCtx.Deadline(); ok {
		bound = time.Until(deadline)
	}
	if stopCtx.Err() != nil {
		bound = 0
	}
	err := call(parent, o, "Stop", bound, func() error { return o.Stop(stopCtx) })
	if err == nil && stopCtx.Err() != nil {
		return fmt.Errorf("Stop returned nil after its context ended (%v); a bound is not a join (%s)", stopCtx.Err(), describe(o))
	}
	return err
}

// boundError reports an owner call that did not return within its bound plus grace. The call's
// goroutine is abandoned, not joined: the owner broke its contract, and the report names what it
// still holds instead of hanging the test.
type boundError struct {
	op    string
	bound time.Duration
	state string
}

func (e *boundError) Error() string {
	return fmt.Sprintf("%s did not return within its bound %s plus %s grace; retained: %s", e.op, e.bound, grace, e.state)
}

func isBoundErr(err error) bool {
	var b *boundError
	return errors.As(err, &b)
}

// panicError reports an owner call that panicked. A panic is never a refusal: an owner that
// panics on a nil context refuses nothing, it crashes its caller.
type panicError struct {
	op    string
	value any
}

func (e *panicError) Error() string {
	return fmt.Sprintf("%s panicked: %v; a panic is not a refusal", e.op, e.value)
}

// notARefusal reports whether err, returned where the check expects a refusal, is instead a call
// that hung or panicked.
func notARefusal(err error) bool {
	var p *panicError
	return isBoundErr(err) || errors.As(err, &p)
}

// call runs one owner operation, converting a panic into a panicError and a hang into a boundError.
func call(ctx context.Context, o Owner, op string, bound time.Duration, fn func() error) error {
	result := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				result <- &panicError{op: op, value: r}
			}
		}()
		result <- fn()
	}()
	timer := time.NewTimer(max(bound, 0) + grace)
	defer timer.Stop()
	select {
	case err := <-result:
		return err
	case <-timer.C:
		return &boundError{op: op, bound: max(bound, 0), state: describe(o)}
	case <-ctx.Done():
		return fmt.Errorf("%s: test context ended while waiting: %w", op, ctx.Err())
	}
}

func requireNothingRetained(o Owner, when string) error {
	obs := o.Observe()
	if len(obs.Unresolved) > 0 {
		return fmt.Errorf("%s the owner still holds %s", when, slices.Clone(obs.Unresolved))
	}
	return nil
}

// describe renders retained state for a failure message.
func describe(o Owner) string {
	obs := o.Observe()
	return fmt.Sprintf("unresolved %v, calls %v", obs.Unresolved, obs.Calls)
}

func isNil(o Owner) bool {
	if o == nil {
		return true
	}
	v := reflect.ValueOf(o)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	}
	return false
}
