package lifecycletest

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// scripted is an owner whose Start and Stop return fixed errors and which may or may not expose
// its state; it exercises how the checks report owners that fail rather than misbehave.
type scripted struct {
	startErr, stopErr error
}

func (s *scripted) Start(ctx context.Context) error {
	if ctx == nil {
		return errors.New("nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.startErr
}

func (s *scripted) Stop(ctx context.Context) error {
	if ctx == nil {
		return errors.New("nil")
	}
	return s.stopErr
}

type observedScripted struct {
	scripted
	obs Observation
}

func (o *observedScripted) Observe() Observation { return o.obs }

// TestChecksFailClosedWithoutObserver: an owner that cannot report its retained state does not
// pass a completion check on its return values alone.
func TestChecksFailClosedWithoutObserver(t *testing.T) {
	for name, check := range map[string]func(context.Context, Owner) error{
		"NilContextsRefused":                    CheckNilContextsRefused,
		"PreCancelledStartRefused":              CheckPreCancelledStartRefused,
		"StopBeforeStartSafe":                   CheckStopBeforeStartSafe,
		"ControlledStopUnderLiveStartAuthority": CheckControlledStopUnderLiveStartAuthority,
		"RepeatedStopIsNoOp":                    CheckRepeatedStopIsNoOp,
	} {
		err := check(t.Context(), &scripted{})
		if err == nil || !strings.Contains(err.Error(), "does not implement lifecycletest.Observer") {
			t.Errorf("%s over an unobservable owner = %v, want a fail-closed error", name, err)
		}
	}
	err := CheckSecondStartRefusedOrRestartCycle(t.Context(), &scripted{}, Promise{})
	if err == nil || !strings.Contains(err.Error(), "does not implement lifecycletest.Observer") {
		t.Errorf("SecondStart over an unobservable owner = %v, want a fail-closed error", err)
	}
}

// TestChecksPropagateOwnerFailures: a Start or Stop error is reported with the step that failed.
func TestChecksPropagateOwnerFailures(t *testing.T) {
	startFails := errors.New("start: broker unreachable")
	stopFails := errors.New("stop: drain refused")
	for _, tc := range []struct {
		name  string
		owner *observedScripted
		check func(context.Context, Owner) error
		want  error
	}{
		{"controlled start", &observedScripted{scripted: scripted{startErr: startFails}}, CheckControlledStopUnderLiveStartAuthority, startFails},
		{"controlled stop", &observedScripted{scripted: scripted{stopErr: stopFails}}, CheckControlledStopUnderLiveStartAuthority, stopFails},
		{"abort start", &observedScripted{scripted: scripted{startErr: startFails}}, CheckAbortStopPreservesCause, startFails},
		{"repeated start", &observedScripted{scripted: scripted{startErr: startFails}}, CheckRepeatedStopIsNoOp, startFails},
		{"repeated first stop", &observedScripted{scripted: scripted{stopErr: stopFails}}, CheckRepeatedStopIsNoOp, stopFails},
		{"stop before start", &observedScripted{scripted: scripted{stopErr: stopFails}}, CheckStopBeforeStartSafe, stopFails},
		{"refused starts then stop", &observedScripted{scripted: scripted{stopErr: stopFails}}, CheckPreCancelledStartRefused, stopFails},
		{"second start: first start", &observedScripted{scripted: scripted{startErr: startFails}},
			func(ctx context.Context, o Owner) error {
				return CheckSecondStartRefusedOrRestartCycle(ctx, o, Promise{})
			}, startFails},
		{"restart: first stop", &observedScripted{scripted: scripted{stopErr: stopFails}},
			func(ctx context.Context, o Owner) error {
				return CheckSecondStartRefusedOrRestartCycle(ctx, o, Promise{Restart: true})
			}, stopFails},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.check(t.Context(), tc.owner); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want it to wrap %v", err, tc.want)
			}
		})
	}
}

// TestRetainedStateIsReported: a nil Stop that leaves a resource named by Observe fails, and the
// message names the resource.
func TestRetainedStateIsReported(t *testing.T) {
	o := &observedScripted{obs: Observation{Unresolved: []string{"consumer orders"}}}
	err := CheckStopBeforeStartSafe(t.Context(), o)
	if err == nil || !strings.Contains(err.Error(), "consumer orders") {
		t.Fatalf("got %v, want the retained consumer named", err)
	}
}

func TestIsNil(t *testing.T) {
	var typed *refowner
	if !isNil(nil) || !isNil(typed) || isNil(&scripted{}) {
		t.Fatal("isNil misclassifies an owner")
	}
}
