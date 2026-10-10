package lifecyclecleanup

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

type contextKey string

func TestFailedStartRollbackTimeoutIsFrameworkOwned(t *testing.T) {
	if failedStartRollbackTimeout != 5*time.Second {
		t.Fatalf("failedStartRollbackTimeout = %v, want 5s", failedStartRollbackTimeout)
	}
}

// Requirement: lifecycle-suite/Failed-start rollback helper; Scenario: Nil parent
func TestRollbackFailedStartRejectsNilParentBeforeCallback(t *testing.T) {
	called := false
	err := RollbackFailedStart(nil, func(context.Context) error {
		called = true
		return nil
	})
	if err == nil {
		t.Fatal("RollbackFailedStart(nil, callback) returned nil")
	}
	if called {
		t.Fatal("rollback callback ran with a nil parent")
	}
}

// A nil rollback is refused: returning nil would tell the caller its rollback succeeded when none
// ran, and it would then clear its cleanup-pending record (design D7.2 of setup-04a-02).
//
// Requirement: lifecycle-suite/Failed-start rollback helper; Scenario: Nil rollback
func TestRollbackFailedStartNilRollback(t *testing.T) {
	err := RollbackFailedStart(t.Context(), nil)
	if err == nil || !strings.Contains(err.Error(), "nil rollback") {
		t.Fatalf("RollbackFailedStart(ctx, nil) = %v, want an error naming the nil rollback", err)
	}
}

// Requirement: lifecycle-suite/Failed-start rollback helper; Scenario: Cancelled parent
// Requirement: lifecycle-suite/Failed-start rollback helper; Scenario: Expired parent
func TestRollbackFailedStartDetachesCancellationPreservesValuesAndBoundsWork(t *testing.T) {
	const key contextKey = "owner"
	tests := []struct {
		name   string
		parent func() context.Context
	}{
		{name: "canceled", parent: func() context.Context {
			parent, cancel := context.WithCancel(context.WithValue(t.Context(), key, "research"))
			cancel()
			return parent
		}},
		{name: "expired deadline", parent: func() context.Context {
			parent, cancel := context.WithDeadline(context.WithValue(t.Context(), key, "research"), time.Unix(1, 0))
			t.Cleanup(cancel)
			return parent
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			called := false
			err := RollbackFailedStart(test.parent(), func(ctx context.Context) error {
				called = true
				if got := ctx.Value(key); got != "research" {
					t.Fatalf("rollback context value = %v, want research", got)
				}
				if err := ctx.Err(); err != nil {
					t.Fatalf("rollback context inherited parent completion: %v", err)
				}
				if _, ok := ctx.Deadline(); !ok {
					t.Fatal("rollback context has no deadline")
				}
				return nil
			})
			if err != nil {
				t.Fatalf("RollbackFailedStart() = %v", err)
			}
			if !called {
				t.Fatal("rollback callback did not run synchronously")
			}
		})
	}
}

// The rollback's deadline is five seconds after the call, whatever the parent's deadline was. The
// bubble's clock does not move while the rollback runs, so the deadline is exact.
//
// Requirement: lifecycle-suite/Failed-start rollback helper
func TestRollbackFailedStartDeadlineIsFiveSecondsAfterTheCall(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		parent, cancel := context.WithDeadline(t.Context(), time.Now().Add(time.Hour))
		defer cancel()
		called := time.Now()
		var deadline time.Time
		err := RollbackFailedStart(parent, func(ctx context.Context) error {
			deadline, _ = ctx.Deadline()
			return nil
		})
		if err != nil {
			t.Fatalf("RollbackFailedStart() = %v", err)
		}
		if want := called.Add(5 * time.Second); !deadline.Equal(want) {
			t.Fatalf("rollback deadline = %v, want %v (five seconds after the call)", deadline, want)
		}
	})
}

// Requirement: lifecycle-suite/Failed-start rollback helper; Scenario: Rollback outlives the budget
func TestRollbackFailedStartJoinsCallbackAndExpiryErrors(t *testing.T) {
	callbackErr := errors.New("rollback failed")
	err := rollbackFailedStart(t.Context(), time.Nanosecond, func(ctx context.Context) error {
		<-ctx.Done()
		return callbackErr
	})
	if !errors.Is(err, callbackErr) {
		t.Fatalf("rollback error = %v, want callback error", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("rollback error = %v, want deadline exceeded", err)
	}
}
