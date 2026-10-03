package lifecycletest

import (
	"context"
	"errors"
	"reflect"
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

// A1: retained state is a compile-time requirement of every owner the checks accept. An optional
// interface discovered at run time is the silent-skip shape: an owner without it would compile and
// fail every completion check on a technicality, or worse, be skipped.
func TestOwnerRequiresObserve(t *testing.T) {
	owner := reflect.TypeOf((*Owner)(nil)).Elem()
	if _, ok := owner.MethodByName("Observe"); !ok {
		t.Fatal("lifecycletest.Owner does not require Observe; an unobservable owner compiles")
	}
}

func TestIsNil(t *testing.T) {
	var typed *refowner
	if !isNil(nil) || !isNil(typed) || isNil(&observedScripted{}) {
		t.Fatal("isNil misclassifies an owner")
	}
}

// panicky panics where the checks expect a refusal or a failure: Stop(nil), and any Start once
// started is set.
type panicky struct {
	observedScripted
	started bool
}

func (p *panicky) Start(ctx context.Context) error {
	if ctx == nil {
		return errors.New("nil")
	}
	if p.started {
		panic("second Start")
	}
	p.started = true
	return nil
}

func (p *panicky) Stop(ctx context.Context) error {
	if ctx == nil {
		panic("nil Stop context")
	}
	return nil
}

// No check counts a panic as a refusal (lifecycle-suite › "Start panics on a nil context"), at any
// of the sites that expect one: Start(nil) is the refowner matrix's nilStartPanics row; these are
// Stop(nil), the second Start without a restart promise, and the must-fail owner's Start.
func TestPanicIsNeverARefusal(t *testing.T) {
	for name, check := range map[string]func(context.Context, Owner) error{
		"Stop(nil)": CheckNilContextsRefused,
		"second Start": func(ctx context.Context, o Owner) error {
			return CheckSecondStartRefusedOrRestartCycle(ctx, o, Promise{})
		},
		// A must-fail owner whose Start panics has not failed its Start: it crashed.
		"failed Start": func(ctx context.Context, o Owner) error {
			o.(*panicky).started = true
			return CheckFailedStartHoldsNothing(ctx, o)
		},
	} {
		err := check(t.Context(), &panicky{})
		var pe *panicError
		if !errors.As(err, &pe) || !strings.Contains(err.Error(), "not a refusal") {
			t.Errorf("%s: check = %v, want a panicError", name, err)
		}
	}
}
