package graphingest

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type graphIngestTestDrain struct {
	called atomic.Int32
	drain  func(context.Context) error
}

func (d *graphIngestTestDrain) Drain(ctx context.Context) error {
	d.called.Add(1)
	return d.drain(ctx)
}

func TestGraphIngestTestOwnerControlledStopPreservesAuthority(t *testing.T) {
	opCtx, cancelOperation := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancelOperation()
	var startCtx context.Context
	drain := &graphIngestTestDrain{drain: func(stopCtx context.Context) error {
		if _, finite := stopCtx.Deadline(); !finite {
			return errors.New("Stop received no finite deadline")
		}
		if err := startCtx.Err(); err != nil {
			return errors.New("accepted Start authority ended before Stop")
		}
		return nil
	}}
	component := createTestComponent(t)
	component.subscriptions = []graphIngestCoreSubscription{drain}
	startGuardAcquiringNothing(t, component) // the pin set lifecycleUsed: true (design D22)
	owner := newGraphIngestTestOwner(component)
	startCtx = owner.startContext(opCtx)
	if err := owner.stop(opCtx); err != nil {
		t.Fatalf("controlled Stop: %v", err)
	}
	if err := startCtx.Err(); !errors.Is(err, context.Canceled) {
		t.Fatalf("Start authority after Stop = %v, want canceled", err)
	}
	if err := opCtx.Err(); err != nil {
		t.Fatalf("operation authority ended before assertions: %v", err)
	}
	owner.finish(opCtx, t)
	if got := drain.called.Load(); got != 1 {
		t.Fatalf("concrete Drain attempts = %d, want 1", got)
	}
}

func TestGraphIngestTestOwnerFailedAttemptIsNotRetried(t *testing.T) {
	opCtx, cancelOperation := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancelOperation()
	want := errors.New("subscription drain failed")
	drain := &graphIngestTestDrain{drain: func(context.Context) error { return want }}
	component := createTestComponent(t)
	component.subscriptions = []graphIngestCoreSubscription{drain}
	leaveGuardCleanupPending(t, component) // the pin set lifecycleUsed and cleanupPending (design D22)
	owner := newGraphIngestTestOwner(component)
	if err := owner.stop(opCtx); !errors.Is(err, want) {
		t.Fatalf("concrete Stop error = %v, want %v", err, want)
	}
	owner.finish(opCtx, t)
	if got := drain.called.Load(); got != 1 {
		t.Fatalf("failed-start Drain attempts = %d, want 1", got)
	}
}

func TestGraphIngestTestOwnerCanceledOperationKeepsFreshTerminalAuthority(t *testing.T) {
	operationCtx, cancelOperation := context.WithCancel(t.Context())
	want := errors.New("core subscription cleanup failed")
	drain := &graphIngestTestDrain{drain: func(stopCtx context.Context) error {
		if stopCtx.Err() != nil {
			return errors.New("fresh Stop context was canceled with operation authority")
		}
		if _, finite := stopCtx.Deadline(); !finite {
			return errors.New("fresh Stop context has no deadline")
		}
		return want
	}}
	component := createTestComponent(t)
	component.subscriptions = []graphIngestCoreSubscription{drain}
	startGuardAcquiringNothing(t, component) // the pin set lifecycleUsed: true (design D22)
	owner := newGraphIngestTestOwner(component)
	owner.startContext(operationCtx)
	cancelOperation()
	err := owner.stop(operationCtx)
	if !errors.Is(err, want) || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled-operation Stop error = %v, want concrete and operation causes", err)
	}
	if got := drain.called.Load(); got != 1 {
		t.Fatalf("concrete Stop attempts = %d, want 1", got)
	}
}
