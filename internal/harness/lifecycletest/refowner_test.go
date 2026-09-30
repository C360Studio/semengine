package lifecycletest

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// failpoint names one lifecycle defect the refowner double can exhibit (design S3).
type failpoint string

const (
	clean                           failpoint = ""
	acceptNilCtx                    failpoint = "acceptNilCtx"
	ignorePreCancelled              failpoint = "ignorePreCancelled"
	startTwiceAllowed               failpoint = "startTwiceAllowed"
	stopBeforeStartPanics           failpoint = "stopBeforeStartPanics"
	stopIgnoresCallerDeadline       failpoint = "stopIgnoresCallerDeadline"
	stopReturnsNilWithWorkerRunning failpoint = "stopReturnsNilWithWorkerRunning"
	secondStopReruns                failpoint = "secondStopReruns"
	restartPromisedButRefused       failpoint = "restartPromisedButRefused"
	abortStopDropsCause             failpoint = "abortStopDropsCause"
)

var errRefownerUsed = errors.New("refowner: already used")

// refowner is a reference owner with one worker goroutine derived from Start authority. Clean, it
// satisfies the floor; each failpoint breaks exactly one rule. Its retained state (workerDone,
// cleanups) is what the checks read through Observe, so a check that passes on the returned error
// alone cannot pass here.
type refowner struct {
	fp          failpoint
	restartable bool
	hang        <-chan struct{} // closed by the test's cleanup; only stopIgnoresCallerDeadline waits on it

	mu             sync.Mutex
	startAttempted bool
	used           bool
	running        bool
	stopped        bool
	cancel         context.CancelFunc
	stopCh         chan struct{}
	stopOnce       *sync.Once
	workerDone     chan struct{}
	cleanups       int
}

func (o *refowner) Start(ctx context.Context) error {
	if ctx == nil {
		if o.fp != acceptNilCtx {
			return errors.New("refowner: nil Start context")
		}
		ctx = context.Background() // the defect under test: a nil context silently becomes a root
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.startAttempted = true
	if err := ctx.Err(); err != nil && o.fp != ignorePreCancelled {
		return err
	}
	if o.running {
		if o.fp == startTwiceAllowed {
			return nil
		}
		return errRefownerUsed
	}
	if o.used && !o.restartable && o.fp != startTwiceAllowed {
		return errRefownerUsed
	}
	runCtx, cancel := context.WithCancel(ctx)
	o.used, o.running, o.stopped = true, true, false
	o.cancel, o.stopCh, o.stopOnce, o.workerDone = cancel, make(chan struct{}), &sync.Once{}, make(chan struct{})
	stopCh, done := o.stopCh, o.workerDone
	go func() {
		defer close(done)
		select {
		case <-runCtx.Done():
		case <-stopCh:
		}
	}()
	return nil
}

func (o *refowner) Stop(ctx context.Context) error {
	if ctx == nil && o.fp != acceptNilCtx {
		return errors.New("refowner: nil Stop context")
	}
	o.mu.Lock()
	if !o.running {
		defer o.mu.Unlock()
		if !o.startAttempted && o.fp == stopBeforeStartPanics {
			var workerDone chan struct{}
			close(workerDone) // the defect under test: Stop assumes Start built its state
		}
		if o.stopped && o.fp == secondStopReruns {
			o.cleanups++
		}
		return nil
	}
	if ctx != nil && ctx.Err() != nil {
		switch o.fp {
		case stopIgnoresCallerDeadline:
			o.mu.Unlock()
			<-o.hang
			return nil
		case abortStopDropsCause:
			o.finishLocked()
			o.mu.Unlock()
			return nil
		}
		o.mu.Unlock()
		return ctx.Err() // the bound is not a join: keep the worker for a later Stop
	}
	if o.fp == stopReturnsNilWithWorkerRunning {
		o.running, o.stopped = false, true
		o.mu.Unlock()
		return nil
	}
	o.stopOnce.Do(func() { close(o.stopCh) })
	done := o.workerDone
	o.mu.Unlock()
	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.finishLocked()
	return nil
}

func (o *refowner) finishLocked() {
	o.stopOnce.Do(func() { close(o.stopCh) })
	o.cancel()
	o.running, o.stopped = false, true
	o.cleanups++
}

func (o *refowner) Observe() Observation {
	o.mu.Lock()
	defer o.mu.Unlock()
	obs := Observation{Calls: map[string]int{"cleanup": o.cleanups}}
	if o.workerDone != nil {
		select {
		case <-o.workerDone:
		default:
			obs.Unresolved = append(obs.Unresolved, "worker")
		}
	}
	return obs
}

type check struct {
	name string
	run  func(context.Context, Owner, Promise) error
}

var checks = []check{
	{"NilContextsRefused", func(ctx context.Context, o Owner, _ Promise) error { return CheckNilContextsRefused(ctx, o) }},
	{"PreCancelledStartRefused", func(ctx context.Context, o Owner, _ Promise) error { return CheckPreCancelledStartRefused(ctx, o) }},
	{"StopBeforeStartSafe", func(ctx context.Context, o Owner, _ Promise) error { return CheckStopBeforeStartSafe(ctx, o) }},
	{"ControlledStopUnderLiveStartAuthority", func(ctx context.Context, o Owner, _ Promise) error {
		return CheckControlledStopUnderLiveStartAuthority(ctx, o)
	}},
	{"AbortStopPreservesCause", func(ctx context.Context, o Owner, _ Promise) error { return CheckAbortStopPreservesCause(ctx, o) }},
	{"RepeatedStopIsNoOp", func(ctx context.Context, o Owner, _ Promise) error { return CheckRepeatedStopIsNoOp(ctx, o) }},
	{"SecondStartRefusedOrRestartCycle", CheckSecondStartRefusedOrRestartCycle},
}

// newRefowner builds a double whose hang channel is released when the test ends, so a failpoint
// that never returns leaks nothing past its test.
func newRefowner(t *testing.T, fp failpoint, restartable bool) *refowner {
	hang := make(chan struct{})
	t.Cleanup(func() { close(hang) })
	return &refowner{fp: fp, restartable: restartable, hang: hang}
}

// finalize stops a double the check may have left running, as Run does after each check, and
// requires the worker gone. A failpoint double may refuse or panic in that Stop too; what matters
// here is only that nothing it started outlives the test.
func finalize(t *testing.T, o *refowner) {
	t.Helper()
	_ = finish(t.Context(), o)
	if obs := o.Observe(); len(obs.Unresolved) > 0 && o.fp != stopReturnsNilWithWorkerRunning {
		t.Errorf("finalize refowner (%s): still holds %v", o.fp, obs.Unresolved)
	}
	if o.fp == stopReturnsNilWithWorkerRunning {
		o.mu.Lock()
		if o.cancel != nil {
			o.cancel() // this double never stops its own worker; end it through Start authority
		}
		o.mu.Unlock()
	}
}

// TestChecksPassAgainstCleanDouble: every check returns nil for a clean owner, with and without a
// restart promise.
func TestChecksPassAgainstCleanDouble(t *testing.T) {
	for _, promise := range []Promise{{Restart: false}, {Restart: true}} {
		for _, c := range checks {
			o := newRefowner(t, clean, promise.Restart)
			if err := c.run(t.Context(), o, promise); err != nil {
				t.Errorf("%s (restart=%t) on the clean double: %v", c.name, promise.Restart, err)
			}
			finalize(t, o)
		}
	}
}

// TestEachFailpointTripsExactlyItsCheck is the sensitivity matrix (lifecycle-suite › "Suite detects
// each violation"): with one failpoint enabled, the mapped check returns an error and every other
// check returns nil.
func TestEachFailpointTripsExactlyItsCheck(t *testing.T) {
	for _, tc := range []struct {
		fp      failpoint
		want    string
		promise Promise
	}{
		{acceptNilCtx, "NilContextsRefused", Promise{}},
		{ignorePreCancelled, "PreCancelledStartRefused", Promise{}},
		{stopBeforeStartPanics, "StopBeforeStartSafe", Promise{}},
		{stopReturnsNilWithWorkerRunning, "ControlledStopUnderLiveStartAuthority", Promise{}},
		{stopIgnoresCallerDeadline, "AbortStopPreservesCause", Promise{}},
		{abortStopDropsCause, "AbortStopPreservesCause", Promise{}},
		{secondStopReruns, "RepeatedStopIsNoOp", Promise{}},
		{startTwiceAllowed, "SecondStartRefusedOrRestartCycle", Promise{}},
		{restartPromisedButRefused, "SecondStartRefusedOrRestartCycle", Promise{Restart: true}},
	} {
		t.Run(string(tc.fp), func(t *testing.T) {
			for _, c := range checks {
				o := newRefowner(t, tc.fp, false)
				err := c.run(t.Context(), o, tc.promise)
				switch {
				case c.name == tc.want && err == nil:
					t.Errorf("%s did not detect %s", c.name, tc.fp)
				case c.name == tc.want:
					t.Logf("%s detected %s: %v", c.name, tc.fp, err)
				case err != nil:
					t.Errorf("%s tripped on %s, which is not its failpoint: %v", c.name, tc.fp, err)
				}
				finalize(t, o)
			}
		})
	}
}

// TestRunOverCleanDouble drives Run itself, as an adopter would.
func TestRunOverCleanDouble(t *testing.T) {
	Run(t, func() Owner { return newRefowner(t, clean, false) }, Promise{})
	Run(t, func() Owner { return newRefowner(t, clean, true) }, Promise{Restart: true})
}
