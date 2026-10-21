package service

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"testing"

	"pgregory.net/rapid"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/pkg/errs"
)

// Model-based test of the component manager's Start and Stop (design § Generated checks, "Start
// and Stop histories"). A rapid state machine composes up to four components, providers and
// consumers, each scripted to fail its Start or not and to fail its next zero to two Stop calls,
// and draws Start, Stop with a live context, Stop with an ended context, and clearing the armed
// Stop failures. Every action is checked against a test-owned reference model filled from design D2
// and D4, never from the manager's code: which components each Start and Stop calls, in what
// order, what each returns, each component's state and last error, and the manager's health.

// cmPropNames is the pool component names are drawn from, in name order. A component's role is
// drawn apart from its name, so a consumer often sorts before a provider and the launch order
// (providers by name, then consumers by name) differs from plain name order.
var cmPropNames = []string{"comp-a", "comp-b", "comp-c", "comp-d"}

// cmPropStopScript is the queue of failures a component's next Stop calls return, one per call,
// nil once it is empty. The test arms and clears it between actions; the component pops it.
type cmPropStopScript struct {
	mu    sync.Mutex
	queue []error
}

func (s *cmPropStopScript) pop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.queue) == 0 {
		return nil
	}
	err := s.queue[0]
	s.queue = s.queue[1:]
	return err
}

// peek returns the failure the next Stop call will return, nil when none is armed.
func (s *cmPropStopScript) peek() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.queue) == 0 {
		return nil
	}
	return s.queue[0]
}

func (s *cmPropStopScript) clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queue = nil
}

// cmPropModel is the reference model of one composed component: what D2 and D4 say the manager
// has done to it so far.
type cmPropModel struct {
	name       string
	provider   bool
	comp       *lifecycleTestComponent
	script     *cmPropStopScript
	launched   bool // its Start was called
	stoppedNil bool // a Stop of it returned nil, so it is never called again
	state      component.State
	lastErr    error
}

// TestPropComponentManagerStopHistory checks every history against the model (design § Generated
// checks; D2; D4, #360):
//
//   - The first Start starts every provider, then, only if no provider failed, every consumer. It
//     returns nil exactly when no Start failed; otherwise its error carries each Start failure,
//     naming the component, and the rollback stops every launched component, the failed ones
//     included, in exact reverse launch order (consumers by name descending, then providers by
//     name descending), its Stop failures joined into Start's error.
//   - A Start after any Start or after a Stop with a live context is refused with
//     errs.ErrAlreadyStarted and starts and stops nothing.
//   - A Stop with a live context before any Start stops nothing and returns nil. Otherwise it stops
//     every launched component whose Stop has not yet returned nil, in exact reverse launch order,
//     and returns nil exactly when none of those Stops failed, carrying each failure otherwise.
//   - A Stop with an ended context stops nothing and returns the context's error.
//   - A component is initialized until launched, started after a Start that succeeded, stopped
//     after a Stop that returned nil, and failed, with that error as its last error, after a
//     failed Start or Stop.
//   - The health check is unhealthy exactly when a component is failed, naming each failed
//     component in name order, each carrying its last error; healthy otherwise.
//
// The model stays sequential (design § Generated checks); the concurrent reads are proven by
// examples. What makes each assertion run: the first Start's outcome is drawn and the composition
// built to reach it, a quarter of the histories begin with a Stop before any Start, and every
// branch above is counted when a history reaches it; the test fails at the end unless each count
// is above zero, so a generator that stops reaching one fails instead of passing quietly. At
// rapid's default 100 checks the generator alone, measured outside the repository, reached every
// branch in each of 20,000 runs, the rarest at least 6 times a run; a run told to make far fewer
// checks (-rapid.checks) can fail that count, and says which branch it missed. The run's seed and
// completed checks are recorded in the notes when the test first runs (task 2.2).
//
// spec: service-lifecycle / The component manager's Start bounds Start
// spec: service-lifecycle / The component manager's health reads every component
func TestPropComponentManagerStopHistory(t *testing.T) {
	reached := map[string]int{}
	branches := []string{
		"clean start", "provider phase failed with a consumer unlaunched", "consumer phase failed",
		"rollback stop failed", "stop retried a strict subset", "stop before start",
		"start refused after stop", "stop with an ended context", "health named two failed components",
	}

	rapid.Check(t, func(rt *rapid.T) {
		// The first Start's outcome is drawn first and the composition built to reach it, so every
		// outcome is reached by construction in about a third of the histories: a measured run of
		// the generator alone found independent draws left one outcome unreached in 1% of runs.
		outcome := rapid.SampledFrom([]string{"clean", "provider phase fails", "consumer phase fails"}).
			Draw(rt, "start outcome")
		minLen := 1
		if outcome != "clean" {
			minLen = 2 // a failing component and one of the other role
		}
		names := rapid.SliceOfNDistinct(rapid.SampledFrom(cmPropNames), minLen, len(cmPropNames), rapid.ID[string]).
			Draw(rt, "names")
		slices.Sort(names)
		var forcedProvider, forcedConsumer string
		if outcome != "clean" {
			forcedProvider = rapid.SampledFrom(names).Draw(rt, "a provider")
			others := slices.DeleteFunc(slices.Clone(names), func(name string) bool { return name == forcedProvider })
			forcedConsumer = rapid.SampledFrom(others).Draw(rt, "a consumer")
		}

		log := &lifecycleCallLog{}
		models := map[string]*cmPropModel{}
		var providers, consumers []string
		var composed []testComponent
		for _, name := range names {
			m := &cmPropModel{
				name:   name,
				comp:   newLifecycleTestComponent(name, log),
				script: &cmPropStopScript{},
				state:  component.StateInitialized,
			}
			switch name {
			case forcedProvider:
				m.provider = true
			case forcedConsumer:
				m.provider = false
			default:
				m.provider = rapid.Bool().Draw(rt, name+" provider")
			}
			var startFails bool
			switch outcome {
			case "provider phase fails":
				// The forced provider fails; any other component may too.
				startFails = name == forcedProvider || rapid.IntRange(0, 2).Draw(rt, name+" start fails") == 0
			case "consumer phase fails":
				// No provider fails; the forced consumer does, and any other consumer may.
				startFails = name == forcedConsumer ||
					(!m.provider && rapid.IntRange(0, 2).Draw(rt, name+" start fails") == 0)
			}
			if startFails {
				m.comp.startErr = fmt.Errorf("start failure in %s", name)
			}
			for i := range rapid.IntRange(0, 2).Draw(rt, name+" stop failures") {
				m.script.queue = append(m.script.queue, fmt.Errorf("stop failure %d in %s", i+1, name))
			}
			script := m.script
			m.comp.onStop = func(context.Context, int64) error { return script.pop() }
			models[name] = m
			if m.provider {
				providers = append(providers, name)
				composed = append(composed,
					testComponent{name: name, instance: &lifecycleStoreProvider{lifecycleTestComponent: m.comp}})
			} else {
				consumers = append(consumers, name)
				composed = append(composed, m.comp.testComponent())
			}
		}
		// The launch order the design fixes: providers by name, then consumers by name.
		launchOrder := slices.Concat(providers, consumers)
		reverseLaunch := slices.Clone(launchOrder)
		slices.Reverse(reverseLaunch)

		cm := newTestComponentManager(t, composed...)
		startCalled, stopAdmitted := false, false

		defer func() {
			// Release whatever the history left running: with every failure cleared, a Stop
			// returns nil whatever came before.
			for _, m := range models {
				m.script.clear()
			}
			if err := cm.Stop(stopContext(t)); err != nil {
				rt.Errorf("final Stop with no armed failure: %v", err)
			}
		}()

		// stopsSince returns the components whose Stop was called since the log held mark entries.
		stopsSince := func(mark int) []string { return log.stopOrder()[mark:] }

		// nextStopFailures reads, from the scripts, the failure each component's next Stop returns.
		// It is read before the action: a failed Start's rollback runs inside Start (design D2), so
		// by the time Start returns the rollback has already popped the queues.
		nextStopFailures := func() map[string]error {
			next := map[string]error{}
			for name, m := range models {
				next[name] = m.script.peek()
			}
			return next
		}

		// expectStops predicts which of the launched components not yet stopped a stop sequence
		// calls, in reverse launch order, and which failure each returns, from next as read before
		// the action.
		expectStops := func(next map[string]error) (visits []string, fired map[string]error) {
			fired = map[string]error{}
			for _, name := range reverseLaunch {
				m := models[name]
				if !m.launched || m.stoppedNil {
					continue
				}
				visits = append(visits, name)
				if err := next[name]; err != nil {
					fired[name] = err
				}
			}
			return visits, fired
		}

		// applyStops records in the model what a stop sequence that called visits did.
		applyStops := func(visits []string, fired map[string]error) {
			for _, name := range visits {
				m := models[name]
				if err := fired[name]; err != nil {
					m.state, m.lastErr = component.StateFailed, err
					continue
				}
				m.stoppedNil, m.state, m.lastErr = true, component.StateStopped, nil
			}
		}

		requireStops := func(rt *rapid.T, action string, mark int, want []string) {
			if got := stopsSince(mark); !slices.Equal(got, want) {
				rt.Fatalf("%s stopped %v, want exact reverse launch order %v", action, got, want)
			}
		}

		requireCarries := func(rt *rapid.T, action string, err error, fired map[string]error) {
			for name, want := range fired {
				if !errors.Is(err, want) {
					rt.Fatalf("%s returned %v, which does not carry %s's failure %q", action, err, name, want)
				}
			}
		}

		start := func(rt *rapid.T) {
			mark := len(log.stopOrder())
			startsBefore := map[string]int64{}
			for name, m := range models {
				startsBefore[name] = m.comp.starts.Load()
			}
			next := nextStopFailures()
			err := cm.Start(t.Context())

			if startCalled || stopAdmitted {
				if !errors.Is(err, errs.ErrAlreadyStarted) {
					rt.Fatalf("Start after a Start or a Stop returned %v, want errs.ErrAlreadyStarted", err)
				}
				if stopAdmitted && !startCalled {
					reached["start refused after stop"]++
				}
				for name, m := range models {
					if got := m.comp.starts.Load(); got != startsBefore[name] {
						rt.Fatalf("a refused Start started %s", name)
					}
				}
				requireStops(rt, "a refused Start", mark, nil)
				return
			}
			startCalled = true

			// Phase one starts every provider; phase two runs only if none failed.
			startFailed := map[string]error{}
			launch := func(phase []string) {
				for _, name := range phase {
					m := models[name]
					m.launched = true
					if m.comp.startErr != nil {
						startFailed[name] = m.comp.startErr
						m.state, m.lastErr = component.StateFailed, m.comp.startErr
						continue
					}
					m.state, m.lastErr = component.StateStarted, nil
				}
			}
			launch(providers)
			providerFailed := len(startFailed) > 0
			if !providerFailed {
				launch(consumers)
			}

			if len(startFailed) == 0 {
				if err != nil {
					rt.Fatalf("Start with no armed Start failure returned %v", err)
				}
				reached["clean start"]++
				requireStops(rt, "a Start that succeeded", mark, nil)
				return
			}
			if providerFailed && len(consumers) > 0 {
				reached["provider phase failed with a consumer unlaunched"]++
			} else if !providerFailed {
				reached["consumer phase failed"]++
			}
			if err == nil {
				rt.Fatalf("Start returned nil with Start failures armed in %v", slices.Sorted(maps.Keys(startFailed)))
			}
			requireCarries(rt, "Start", err, startFailed)
			for name := range startFailed {
				if want := fmt.Sprintf("component %q", name); !strings.Contains(err.Error(), want) {
					rt.Fatalf("Start's error %q does not name %s", err.Error(), want)
				}
			}
			// The rollback stops every launched component, the failed ones included.
			visits, fired := expectStops(next)
			requireStops(rt, "the failed Start's rollback", mark, visits)
			requireCarries(rt, "Start", err, fired)
			if len(fired) > 0 {
				reached["rollback stop failed"]++
			}
			applyStops(visits, fired)
		}

		stopLive := func(rt *rapid.T) {
			mark := len(log.stopOrder())
			visits, fired := expectStops(nextStopFailures())
			err := cm.Stop(stopContext(t))
			first := !startCalled && !stopAdmitted
			stopAdmitted = true

			if first {
				reached["stop before start"]++
				if err != nil {
					rt.Fatalf("Stop before any Start returned %v, want nil", err)
				}
				requireStops(rt, "a Stop before any Start", mark, nil)
				return
			}
			requireStops(rt, "Stop", mark, visits)
			launched := 0
			for _, m := range models {
				if m.launched {
					launched++
				}
			}
			if len(visits) > 0 && len(visits) < launched {
				reached["stop retried a strict subset"]++
			}
			if len(fired) == 0 {
				if err != nil {
					rt.Fatalf("Stop with no armed Stop failure in %v returned %v", visits, err)
				}
			} else {
				if err == nil {
					rt.Fatalf("Stop returned nil with Stop failures armed in %v", slices.Sorted(maps.Keys(fired)))
				}
				requireCarries(rt, "Stop", err, fired)
			}
			applyStops(visits, fired)
		}

		// The history never holds a Stop with an ended context before the first Start: whether it
		// uses the manager up is not stated (design D2), and the history starts before Repeat.
		stopEnded := func(rt *rapid.T) {
			mark := len(log.stopOrder())
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			err := cm.Stop(ctx)
			reached["stop with an ended context"]++
			if !errors.Is(err, context.Canceled) {
				rt.Fatalf("Stop with an ended context returned %v, want context.Canceled", err)
			}
			requireStops(rt, "a Stop with an ended context", mark, nil)
		}

		// Clearing disarms every Stop failure still queued. A Start failure is not cleared: the
		// first Start runs before Repeat, and a later Start is refused, so it would change nothing.
		clearFailures := func(*rapid.T) {
			for _, m := range models {
				m.script.clear()
			}
		}

		check := func(rt *rapid.T) {
			statuses := cm.GetComponentStatus()
			if len(statuses) != len(models) {
				rt.Fatalf("GetComponentStatus reports %d components, want %d", len(statuses), len(models))
			}
			var failed []string
			for _, name := range names {
				m := models[name]
				status, found := statuses[name]
				if !found {
					rt.Fatalf("GetComponentStatus has no %s", name)
				}
				if status.State != m.state {
					rt.Fatalf("component %s is %s, want %s", name, status.State, m.state)
				}
				if m.lastErr == nil && status.LastError != nil {
					rt.Fatalf("component %s (%s) has last error %v, want none", name, status.State, status.LastError)
				}
				if m.lastErr != nil && !errors.Is(status.LastError, m.lastErr) {
					rt.Fatalf("component %s's last error is %v, want %q", name, status.LastError, m.lastErr)
				}
				want := int64(0)
				if m.launched {
					want = 1
				}
				if got := m.comp.starts.Load(); got != want {
					rt.Fatalf("component %s was started %d times, want %d", name, got, want)
				}
				if m.state == component.StateFailed {
					failed = append(failed, name)
				}
			}

			err := cm.healthCheck()
			if len(failed) == 0 {
				if err != nil {
					rt.Fatalf("health check with no failed component returned %v", err)
				}
				return
			}
			if err == nil {
				rt.Fatalf("health check returned healthy with %v failed", failed)
			}
			if len(failed) >= 2 {
				reached["health named two failed components"]++
			}
			message, last := err.Error(), -1
			for _, name := range names {
				m := models[name]
				at := strings.Index(message, fmt.Sprintf("component %q failed", name))
				if m.state != component.StateFailed {
					if at >= 0 {
						rt.Fatalf("health check %q names %s, which is %s", message, name, m.state)
					}
					continue
				}
				if at < 0 {
					rt.Fatalf("health check %q does not name failed component %s", message, name)
				}
				if at < last {
					rt.Fatalf("health check %q does not name the failed components %v in name order", message, failed)
				}
				last = at
				if !errors.Is(err, m.lastErr) {
					rt.Fatalf("health check %q does not carry %s's last error %q", message, name, m.lastErr)
				}
			}
		}

		// A history is an optional Stop before any Start, the first Start, then any sequence of
		// the four actions. Nothing else can come first: a Stop with an ended context is not
		// drawn there (above), and clearing before Start is the same as drawing no Stop failure.
		if rapid.IntRange(0, 3).Draw(rt, "stop before start") == 0 {
			stopLive(rt)
			check(rt)
		}
		start(rt)
		check(rt)
		rt.Repeat(map[string]func(*rapid.T){
			"start":         start,
			"stopLive":      stopLive,
			"stopEnded":     stopEnded,
			"clearFailures": clearFailures,
			"":              check,
		})
	})

	if t.Failed() {
		return
	}
	for _, branch := range branches {
		t.Logf("branch %q reached %d times", branch, reached[branch])
	}
	for _, branch := range branches {
		if reached[branch] == 0 {
			t.Errorf("no history reached %q, so its assertions never ran", branch)
		}
	}
}
