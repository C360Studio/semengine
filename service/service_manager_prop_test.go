package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/c360studio/semengine/pkg/errs"
)

// Model-based test of Manager.StopAll (design § Generated checks, "StopAll passes"). A rapid
// state machine composes register (before StartAll only), StartAll, arm a stop failure, and
// StopAll, and checks each action against a test-owned reference model of which services a pass
// must visit, filled from design D2, never from the manager's code.

// propStopCtxKey tags the per-pass token the test threads through StopAll, so the spy can show
// which context reached it. It marks an observation in a test; no production state travels in it.
type propStopCtxKey struct{}

// propStopService implements the idempotent-Stop contract: teardown happens once, and a Stop after
// it returns the drawn repeat style, nil or errs.ErrAlreadyStopped, both counted as stopped.
type propStopService struct {
	MockService
	visits      *[]string // the visit log shared by every service of one history
	teardowns   int
	stoppedFlag bool
	failNext    error // an armed failure, consumed by the next visit
	repeatStyle error // nil or errs.ErrAlreadyStopped

	// lastStopCtx records the context this spy was handed: a test spy recording an argument. The
	// rule against retaining a context binds production structs.
	lastStopCtx context.Context
}

func (s *propStopService) Stop(ctx context.Context) error {
	s.lastStopCtx = ctx
	*s.visits = append(*s.visits, s.name)
	if s.stoppedFlag {
		return s.repeatStyle
	}
	if s.failNext != nil {
		err := s.failNext
		s.failNext = nil
		return err
	}
	s.teardowns++
	s.stoppedFlag = true
	return nil
}

// TestStopAllRetryAfterFailedPassTreatsAlreadyStoppedAsClean is the pin's promoted rapid
// counterexample (pin service_manager_prop_test.go:82), with StartAll first: a failed pass is
// retried whole in reverse registration order, and errs.ErrAlreadyStopped on the retry is clean.
//
// spec: service-lifecycle / StopAll counts a service already stopped as stopped
func TestStopAllRetryAfterFailedPassTreatsAlreadyStoppedAsClean(t *testing.T) {
	visits := []string{}
	svcA := &propStopService{
		MockService: MockService{name: "svcA", status: StatusRunning, healthy: true},
		visits:      &visits,
		failNext:    fmt.Errorf("genuine stop failure in svcA"),
	}
	svcB := &propStopService{
		MockService: MockService{name: "svcB", status: StatusRunning, healthy: true},
		visits:      &visits,
		repeatStyle: errs.ErrAlreadyStopped,
	}
	// Registration order is load-bearing: StopAll visits in reverse, svcB then svcA.
	manager := startedManagerWith(t, svcA, svcB)

	err := manager.StopAll(stopContext(t))
	if err == nil || !strings.Contains(err.Error(), "svcA") {
		t.Fatalf("first pass returned %v, want svcA's failure named", err)
	}
	for _, name := range []string{"svcA", "svcB"} {
		if _, found := manager.GetService(name); !found {
			t.Fatalf("GetService(%q) found nothing after a failed pass; the retry needs it", name)
		}
	}

	visits = visits[:0]
	if err := manager.StopAll(stopContext(t)); err != nil {
		t.Fatalf("retry returned %v, want nil: errs.ErrAlreadyStopped counts as stopped", err)
	}
	if want := []string{"svcB", "svcA"}; !slices.Equal(visits, want) {
		t.Fatalf("retry visited %v, want exact reverse registration order %v", visits, want)
	}
	if svcA.teardowns != 1 || svcB.teardowns != 1 {
		t.Fatalf("teardowns svcA=%d svcB=%d, want one each", svcA.teardowns, svcB.teardowns)
	}
	if _, found := manager.GetService("svcA"); found {
		t.Fatal("GetService found svcA after a nil StopAll")
	}
}

// TestPropStopAllShutdownContract checks every history against the model (design § Generated
// checks): a StopAll after StartAll, or after a failed pass, visits every service once in exact
// reverse registration order and hands each the pass's context; it returns nil exactly when no
// armed failure fired, a nil or errs.ErrAlreadyStopped repeat counting as clean; after a nil pass a
// later StopAll visits nothing and returns nil. A StopAll before StartAll visits nothing and returns
// nil, and a StartAll after any StopAll, or a second StartAll, is refused with
// errs.ErrAlreadyStarted. The arm action is a no-op when no service is left to fail (the pin's
// t.Skip at :168). What makes each assertion run: "stopAll" reaches a visiting pass whenever
// "startAll" precedes it, and a failing pass whenever "armStopFailure" drew a service first; both
// orders are within rapid's default action count. Seed and completed checks are recorded in the
// notes when the test first runs (task 2.2).
//
// spec: service-lifecycle / StopAll counts a service already stopped as stopped
func TestPropStopAllShutdownContract(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		manager := createTestServiceManager(ManagerConfig{}, createTestServiceDependencies())
		visits := []string{}
		services := map[string]*propStopService{}
		var order []string
		started, stopCalled, cleanPass := false, false, false
		passes := 0

		add := func(name string, repeat error) {
			svc := &propStopService{
				MockService: MockService{name: name, status: StatusRunning, healthy: true},
				visits:      &visits,
				repeatStyle: repeat,
			}
			if err := manager.RegisterInstance(name, svc); err != nil {
				rt.Fatalf("RegisterInstance(%q): %v", name, err)
			}
			order = append(order, name)
			services[name] = svc
		}
		// Every composition holds a component-manager (design D3); here it is one of the spies.
		add("component-manager", nil)

		defer func() {
			if !started || cleanPass {
				return
			}
			for _, svc := range services {
				svc.failNext = nil
			}
			ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), stopBound)
			defer cancel()
			if err := manager.StopAll(ctx); err != nil {
				rt.Errorf("final StopAll with no armed failure: %v", err)
			}
		}()

		rt.Repeat(map[string]func(*rapid.T){
			"register": func(rt *rapid.T) {
				if started || stopCalled {
					return // registration is drawn before StartAll only
				}
				repeat := rapid.SampledFrom([]error{nil, errs.ErrAlreadyStopped}).Draw(rt, "repeatStyle")
				add(fmt.Sprintf("svc%02d", len(order)), repeat)
			},
			"startAll": func(rt *rapid.T) {
				before := slices.Clone(visits)
				err := manager.StartAll(t.Context())
				if started || stopCalled {
					if !errors.Is(err, errs.ErrAlreadyStarted) {
						rt.Fatalf("StartAll after StartAll or StopAll returned %v, want errs.ErrAlreadyStarted", err)
					}
					if !slices.Equal(visits, before) {
						rt.Fatalf("a refused StartAll stopped %v", visits[len(before):])
					}
					return
				}
				if err != nil {
					rt.Fatalf("first StartAll: %v", err)
				}
				started = true
			},
			"armStopFailure": func(rt *rapid.T) {
				var candidates []string
				for _, name := range order {
					if !services[name].stoppedFlag {
						candidates = append(candidates, name)
					}
				}
				if len(candidates) == 0 {
					return // no service left to fail
				}
				name := rapid.SampledFrom(candidates).Draw(rt, "failing")
				services[name].failNext = fmt.Errorf("genuine stop failure in %s", name)
			},
			"stopAll": func(rt *rapid.T) {
				visits = visits[:0]
				visiting := started && !cleanPass
				var expectFailing []string
				if visiting {
					for _, name := range order {
						if services[name].failNext != nil {
							expectFailing = append(expectFailing, name)
						}
					}
				}
				passes++
				token := fmt.Sprintf("pass%02d", passes)
				parent := context.WithValue(context.WithoutCancel(t.Context()), propStopCtxKey{}, token)
				ctx, cancel := context.WithTimeout(parent, stopBound)
				defer cancel()

				err := manager.StopAll(ctx)
				stopCalled = true

				if !visiting {
					if len(visits) != 0 || err != nil {
						rt.Fatalf("StopAll before StartAll or after a nil pass visited %v and returned %v, want nothing and nil", visits, err)
					}
					return
				}
				want := slices.Clone(order)
				slices.Reverse(want)
				if !slices.Equal(visits, want) {
					rt.Fatalf("visited %v, want exact reverse registration order %v", visits, want)
				}
				for _, name := range order {
					if got := services[name].lastStopCtx.Value(propStopCtxKey{}); got != token {
						rt.Fatalf("service %s got a context carrying %v, want the pass's %q", name, got, token)
					}
				}
				if len(expectFailing) == 0 {
					if err != nil {
						rt.Fatalf("a pass with no armed failure returned %v", err)
					}
					cleanPass = true
					return
				}
				if err == nil {
					rt.Fatalf("armed failures in %v were not returned", expectFailing)
				}
				for _, name := range expectFailing {
					if !strings.Contains(err.Error(), name) {
						rt.Fatalf("StopAll's error %q does not name %s", err.Error(), name)
					}
				}
			},
			"": func(rt *rapid.T) {
				// Checked after every action: a started manager keeps every service until a nil
				// pass, and holds none after it (design D2, pin :928-931). Before StartAll the
				// design states nothing about GetService after StopAll, so nothing is checked.
				if !started {
					return
				}
				for _, name := range order {
					if _, found := manager.GetService(name); found == cleanPass {
						rt.Fatalf("GetService(%q) found=%v after clean pass=%v", name, found, cleanPass)
					}
				}
			},
		})
	})
}
