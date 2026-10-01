package lifecycletest

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/c360studio/semengine/internal/harness/probe"
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
	// workerExit, when set, holds the worker after it has been signalled and before it closes
	// workerDone, so a test can force the interleaving where a Stop returns ahead of the worker's exit.
	workerExit *probe.Callback

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
	// restartPromisedButRefused is a restartable owner whose second Start refuses anyway.
	if o.used && (!o.restartable || o.fp == restartPromisedButRefused) && o.fp != startTwiceAllowed {
		return errRefownerUsed
	}
	runCtx, cancel := context.WithCancel(ctx)
	o.used, o.running, o.stopped = true, true, false
	o.cancel, o.stopCh, o.stopOnce, o.workerDone = cancel, make(chan struct{}), &sync.Once{}, make(chan struct{})
	stopCh, done, exit := o.stopCh, o.workerDone, o.workerExit
	go func() {
		defer close(done)
		select {
		case <-runCtx.Done():
		case <-stopCh:
		}
		if exit != nil {
			exit.Block(runCtx)
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
			// The defect under test is only the dropped cause. Keep the worker for a later Stop, as
			// the clean path does: marking the owner stopped here without joining would also let the
			// next Stop return nil ahead of the worker's exit (issue #40).
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

// finalize stops a double the check may have left running, as Run does after each check, requires
// that the double then holds nothing, and joins every worker the double started before it returns.
//
// A Stop that signalled its worker and returned nil claims a join: the requirement is checked
// before finalize waits, so a Stop that returned ahead of its worker's exit (issue #40) is reported,
// not rescued. A double whose Stop never signalled its worker (stopReturnsNilWithWorkerRunning's
// declared defect) has that worker ended through Start authority and joined first; the assertion
// then holds for it too, with no exemption.
func finalize(t *testing.T, o *refowner) {
	t.Helper()
	_ = finish(t.Context(), o)
	o.mu.Lock()
	cancel, stopCh, done := o.cancel, o.stopCh, o.workerDone
	o.mu.Unlock()
	if done == nil {
		return // never started: nothing to join
	}
	select {
	case <-stopCh:
	default: // Stop never signalled the worker: end it through Start authority and join it
		cancel()
		<-done
	}
	if obs := o.Observe(); len(obs.Unresolved) > 0 {
		t.Errorf("finalize refowner (%s): still holds %v", o.fp, obs.Unresolved)
	}
	cancel()
	<-done // the worker selects on its Start context, so cancel ends it
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

// failpointCase declares one failpoint with the check that must detect it and the promise the
// double is built under. failpointTable is the one list every sensitivity test reads.
type failpointCase struct {
	fp      failpoint
	want    string // the name of the entry in checks that must return an error
	promise Promise
}

var failpointTable = []failpointCase{
	{acceptNilCtx, "NilContextsRefused", Promise{}},
	{ignorePreCancelled, "PreCancelledStartRefused", Promise{}},
	{stopBeforeStartPanics, "StopBeforeStartSafe", Promise{}},
	{stopReturnsNilWithWorkerRunning, "ControlledStopUnderLiveStartAuthority", Promise{}},
	{stopIgnoresCallerDeadline, "AbortStopPreservesCause", Promise{}},
	{abortStopDropsCause, "AbortStopPreservesCause", Promise{}},
	{secondStopReruns, "RepeatedStopIsNoOp", Promise{}},
	{startTwiceAllowed, "SecondStartRefusedOrRestartCycle", Promise{}},
	{restartPromisedButRefused, "SecondStartRefusedOrRestartCycle", Promise{Restart: true}},
}

// failpointGaps reports every declared failpoint that the table does not map to exactly one check
// named in cs.
func failpointGaps(declared []failpoint, table []failpointCase, cs []check) []string {
	rows := map[failpoint]int{}
	var gaps []string
	for _, tc := range table {
		rows[tc.fp]++
		switch {
		case !slices.Contains(declared, tc.fp):
			gaps = append(gaps, fmt.Sprintf("%s: in the table but not declared", tc.fp))
		case tc.want == "":
			gaps = append(gaps, fmt.Sprintf("%s: no expected check", tc.fp))
		case !slices.ContainsFunc(cs, func(c check) bool { return c.name == tc.want }):
			gaps = append(gaps, fmt.Sprintf("%s: expected check %q is not in checks", tc.fp, tc.want))
		}
	}
	for _, fp := range declared {
		switch rows[fp] {
		case 0:
			gaps = append(gaps, fmt.Sprintf("%s: declared with no table row", fp))
		case 1:
		default:
			gaps = append(gaps, fmt.Sprintf("%s: %d table rows", fp, rows[fp]))
		}
	}
	return gaps
}

// declaredFailpoints reads the failpoint constants from this file's source, so a constant added
// without a table row is seen. The clean value ("") is not a failpoint.
func declaredFailpoints(t *testing.T) []failpoint {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "refowner_test.go", nil, 0)
	if err != nil {
		t.Fatalf("parse refowner_test.go: %v", err)
	}
	var out []failpoint
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs := spec.(*ast.ValueSpec)
			if id, ok := vs.Type.(*ast.Ident); !ok || id.Name != "failpoint" {
				continue
			}
			for i, name := range vs.Names {
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok {
					t.Fatalf("failpoint %s: value is not a string literal", name.Name)
				}
				v, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatalf("failpoint %s: %v", name.Name, err)
				}
				if v != "" {
					out = append(out, failpoint(v))
				}
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("refowner_test.go declares no failpoint constant")
	}
	return out
}

// TestFailpointTableComplete requires every declared failpoint to map to one check, and shows the
// requirement can fail: each planted table has one hole and the hole's failpoint must be named.
func TestFailpointTableComplete(t *testing.T) {
	declared := declaredFailpoints(t)
	if gaps := failpointGaps(declared, failpointTable, checks); len(gaps) > 0 {
		t.Errorf("failpoint table has gaps: %v", gaps)
	}

	without := func(fp failpoint) []failpointCase {
		var out []failpointCase
		for _, tc := range failpointTable {
			if tc.fp != fp {
				out = append(out, tc)
			}
		}
		return out
	}
	with := func(row failpointCase) []failpointCase {
		return append(without(row.fp), row)
	}
	for name, planted := range map[string]struct {
		table []failpointCase
		hole  failpoint
	}{
		"row missing":        {without(secondStopReruns), secondStopReruns},
		"no expected check":  {with(failpointCase{fp: abortStopDropsCause}), abortStopDropsCause},
		"unknown check":      {with(failpointCase{fp: acceptNilCtx, want: "NoSuchCheck"}), acceptNilCtx},
		"declared twice":     {append(slices.Clone(failpointTable), failpointCase{fp: ignorePreCancelled, want: "NilContextsRefused"}), ignorePreCancelled},
		"row never declared": {append(slices.Clone(failpointTable), failpointCase{fp: "undeclared", want: "NilContextsRefused"}), "undeclared"},
	} {
		gaps := failpointGaps(declared, planted.table, checks)
		if !slices.ContainsFunc(gaps, func(g string) bool { return strings.Contains(g, string(planted.hole)) }) {
			t.Errorf("planted table (%s): gaps %v do not name %s", name, gaps, planted.hole)
		}
	}
}

// TestEachFailpointTripsExactlyItsCheck is the sensitivity matrix (lifecycle-suite › "Suite detects
// each violation"): with one failpoint enabled, the mapped check returns an error and every other
// check returns nil. Each subtest runs in a synctest bubble, so the checks' bound-plus-grace timers
// fire on the bubble's fake clock once every goroutine is blocked, instead of costing real seconds.
func TestEachFailpointTripsExactlyItsCheck(t *testing.T) {
	for _, tc := range failpointTable {
		t.Run(string(tc.fp), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				for _, c := range checks {
					// The double is as restartable as the promise says; only a failpoint may break it.
					o := newRefowner(t, tc.fp, tc.promise.Restart)
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
		})
	}
}

// TestAbortStopThenFinishJoinsWorker forces issue #40's interleaving instead of waiting for the
// scheduler to produce it: after CheckAbortStopPreservesCause, the worker has been signalled but is
// held before it closes workerDone, and finalize's Stop runs. In a synctest bubble, Wait returns only
// once every goroutine is durably blocked, so whether that Stop has returned is decided by the
// double's code, not by timing: a Stop that returned while the worker is still held claimed a join
// it never made. The cases are the clean double and every row of failpointTable except those whose
// expected check is ControlledStopUnderLiveStartAuthority: a Stop that returns nil ahead of its join
// is that failpoint's declared defect.
func TestAbortStopThenFinishJoinsWorker(t *testing.T) {
	fps := []failpoint{clean}
	for _, tc := range failpointTable {
		if tc.want != "ControlledStopUnderLiveStartAuthority" {
			fps = append(fps, tc.fp)
		}
	}
	for _, fp := range fps {
		t.Run("fp="+string(fp), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				o := newRefowner(t, fp, false)
				o.workerExit = probe.NewCallback()
				defer o.workerExit.Release()
				_ = CheckAbortStopPreservesCause(t.Context(), o) // its verdict is the matrix's concern
				<-o.workerExit.Entered()

				finished := make(chan error, 1)
				go func() { finished <- finish(t.Context(), o) }()
				synctest.Wait()
				select {
				case <-finished:
					if obs := o.Observe(); len(obs.Unresolved) > 0 {
						t.Fatalf("finalize's Stop returned while the owner still holds %v", obs.Unresolved)
					}
				default: // blocked on the worker's join, as it must be
				}
				o.workerExit.Release()
				<-finished
				if obs := o.Observe(); len(obs.Unresolved) > 0 {
					t.Errorf("after the worker exited, the owner still holds %v", obs.Unresolved)
				}
			})
		})
	}
}

// TestRunOverCleanDouble drives Run itself, as an adopter would.
func TestRunOverCleanDouble(t *testing.T) {
	Run(t, func() Owner { return newRefowner(t, clean, false) }, Promise{})
	Run(t, func() Owner { return newRefowner(t, clean, true) }, Promise{Restart: true})
}
