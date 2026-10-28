package graphingest

import (
	"context"
	"errors"
	"testing"

	"github.com/c360studio/semengine/pkg/errs"
)

// TestLifecycleOwnerStopBeforeStartIsTerminal reads, at the pin, lifecycleUsed after the
// refused Start and after the refused Stop(nil). The guard keeps its state to itself
// (design D22), so each read is a probe, assertGuardUnused, which uses the guard up: each
// probe runs on a fresh component that repeats the steps before it. The components passed
// Initialize, so Start reaches the guard (design D13).
func TestLifecycleOwnerStopBeforeStartIsTerminal(t *testing.T) {
	newOwner := func() *Component {
		owner := createTestComponent(t)
		if err := owner.Initialize(); err != nil {
			t.Fatalf("Initialize: %v", err)
		}
		return owner
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	startCanceled := func(owner *Component) {
		t.Helper()
		if err := owner.Start(canceled); err == nil {
			t.Fatal("pre-canceled Start succeeded")
		}
	}
	stopNil := func(owner *Component) {
		t.Helper()
		if err := owner.Stop(nil); err == nil {
			t.Fatal("Stop(nil) succeeded")
		}
	}

	owner := newOwner()
	startCanceled(owner)
	assertGuardUnused(t, owner) // the pre-canceled Start consumed no lifecycle authority

	owner = newOwner()
	startCanceled(owner)
	stopNil(owner)
	assertGuardUnused(t, owner) // Stop(nil) consumed no lifecycle authority

	owner = newOwner()
	startCanceled(owner)
	stopNil(owner)
	if err := owner.Stop(t.Context()); err != nil {
		t.Fatalf("Stop before Start: %v", err)
	}
	if err := owner.Start(t.Context()); !errors.Is(err, errs.ErrAlreadyStarted) {
		t.Fatalf("Start after terminal Stop error = %v, want ErrAlreadyStarted", err)
	}
	if err := owner.Stop(t.Context()); err != nil {
		t.Fatalf("repeated completed Stop: %v", err)
	}
}
