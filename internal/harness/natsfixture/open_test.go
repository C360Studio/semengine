package natsfixture

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// recordingTB stands in for a test so the helper's failure paths can be observed without failing
// the real test: it records Fatalf and Errorf and collects cleanups for the test to run.
type recordingTB struct {
	testing.TB
	ctx      context.Context
	fatals   []string
	errors   []string
	cleanups []func()
}

func (r *recordingTB) Helper()                  {}
func (r *recordingTB) Context() context.Context { return r.ctx }
func (r *recordingTB) Cleanup(f func())         { r.cleanups = append(r.cleanups, f) }
func (r *recordingTB) Fatalf(format string, args ...any) {
	r.fatals = append(r.fatals, fmt.Sprintf(format, args...))
}
func (r *recordingTB) Errorf(format string, args ...any) {
	r.errors = append(r.errors, fmt.Sprintf(format, args...))
}

// runCleanups runs the recorded cleanups last-registered first, as testing does.
func (r *recordingTB) runCleanups() {
	for i := len(r.cleanups) - 1; i >= 0; i-- {
		r.cleanups[i]()
	}
}

// Requirement: nats-fixture/Connected value for a package's tests; Scenario: Opening fails
func TestOpenFailureNamesURLAndRegistersNoClose(t *testing.T) {
	const url = "fixture-url-a"
	refused := errors.New("connection refused")
	rec := &recordingTB{TB: t, ctx: t.Context()}
	var openedWith string
	closed := false
	got := Open(rec, url, func(ctx context.Context, u string) (int, func(context.Context) error, error) {
		openedWith = u
		if _, ok := ctx.Deadline(); !ok {
			t.Error("open called without a deadline")
		}
		return 7, func(context.Context) error { closed = true; return nil }, refused
	})
	if openedWith != url {
		t.Fatalf("open called with %q, want %q", openedWith, url)
	}
	if got != 0 {
		t.Errorf("Open returned %d on failure, want the zero value", got)
	}
	if len(rec.fatals) != 1 || !strings.Contains(rec.fatals[0], url) || !strings.Contains(rec.fatals[0], refused.Error()) {
		t.Fatalf("fatals = %q, want one naming %q and %q", rec.fatals, url, refused)
	}
	if len(rec.cleanups) != 0 {
		t.Fatalf("%d cleanup(s) registered after a failed open, want none", len(rec.cleanups))
	}
	rec.runCleanups()
	if closed {
		t.Fatal("close ran after a failed open")
	}
}

// Requirement: nats-fixture/Connected value for a package's tests; Scenario: Close runs first, bounded
func TestOpenCloseErrorIsATestError(t *testing.T) {
	rec := &recordingTB{TB: t, ctx: t.Context()}
	failed := errors.New("drain failed")
	var hadDeadline bool
	got := Open(rec, "fixture-url-b", func(context.Context, string) (string, func(context.Context) error, error) {
		return "value", func(ctx context.Context) error {
			_, hadDeadline = ctx.Deadline()
			return failed
		}, nil
	})
	if got != "value" || len(rec.fatals) != 0 {
		t.Fatalf("Open = %q, fatals %q; want the opened value and no failure", got, rec.fatals)
	}
	if len(rec.cleanups) != 1 {
		t.Fatalf("%d cleanup(s) registered, want one close", len(rec.cleanups))
	}
	rec.runCleanups()
	if !hadDeadline {
		t.Error("close ran without a deadline")
	}
	if len(rec.errors) != 1 || !strings.Contains(rec.errors[0], failed.Error()) {
		t.Fatalf("errors = %q, want one naming %q", rec.errors, failed)
	}
}

// An empty URL (such as an unstarted fixture's URL()) fails the test rather than calling open with
// an empty address.
func TestOpenRefusesAnEmptyURL(t *testing.T) {
	rec := &recordingTB{TB: t, ctx: t.Context()}
	called := false
	Open(rec, "", func(context.Context, string) (int, func(context.Context) error, error) {
		called = true
		return 0, nil, nil
	})
	if called || len(rec.fatals) != 1 || !strings.Contains(rec.fatals[0], "empty URL") {
		t.Fatalf("open called = %v, fatals %q; want no call and one failure naming the empty URL", called, rec.fatals)
	}
}
