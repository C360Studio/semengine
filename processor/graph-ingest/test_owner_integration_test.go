//go:build integration

package graphingest

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestGraphIngestTestOwnerWaitsForObservedClosure: the test owner's stop returns only
// after the component's Stop has joined its input, and keeps the Start authority live
// until then. The input is a real consumer whose callback the test holds
// (bindHeldInput, lifecycle_owner_test.go).
func TestGraphIngestTestOwnerWaitsForObservedClosure(t *testing.T) {
	setupCtx, cancelSetup := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancelSetup()
	component, pause := newFixtureInputComponent(t)
	bindHeldInput(setupCtx, setupCtx, t, component, pause)
	startGuardAcquiringNothing(t, component) // the pin set lifecycleUsed: true (design D22)

	operationCtx, cancelOperation := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancelOperation()
	owner := newGraphIngestTestOwner(component)
	startCtx := owner.startContext(operationCtx)
	stopDone := make(chan struct{})
	var stopErr error
	go func() {
		stopErr = owner.stop(operationCtx)
		close(stopDone)
	}()
	t.Cleanup(func() {
		pause.Release()
		joinCtx, cancelJoin := context.WithTimeout(context.WithoutCancel(t.Context()), 5*time.Second)
		defer cancelJoin()
		select {
		case <-stopDone:
		case <-joinCtx.Done():
			t.Errorf("incomplete cleanup: concrete Stop did not join after the callback was released: %v", joinCtx.Err())
		}
	})
	awaitReleaseBegun(operationCtx, t, component)
	select {
	case <-stopDone:
		t.Fatalf("Stop returned while a consume callback is held: %v", stopErr)
	default:
	}
	if err := startCtx.Err(); err != nil {
		t.Fatalf("Start authority ended while the input's callback was held: %v", err)
	}
	pause.Release()
	select {
	case <-stopDone:
		if stopErr != nil {
			t.Fatalf("Stop after the callback returned: %v", stopErr)
		}
	case <-operationCtx.Done():
		t.Fatalf("Stop did not return after the callback returned: %v", operationCtx.Err())
	}
	if !errors.Is(startCtx.Err(), context.Canceled) {
		t.Fatalf("Start authority after joined Stop = %v, want canceled", startCtx.Err())
	}
}
