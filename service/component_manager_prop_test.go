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
// examples. What makes each assertion run: each branch has a named witness history that reaches
// it, run through the same assertions before the generated run and failing if it misses its
// branch (#500). The generated run then explores further: the first Start's outcome is drawn and
// the composition built to reach it, and a quarter of the histories begin with a Stop before any
// Start. How often random histories reach each branch is only logged, never asserted, so a run
// with few checks (-rapid.checks) cannot fail on what it happened to draw. The run's seed and
// completed checks are recorded in the notes when the test first runs (task 2.2).
//
// spec: service-lifecycle / The component manager's Start bounds Start
// spec: service-lifecycle / The component manager's health reads every component
func TestPropComponentManagerStopHistory(t *testing.T) {
	// One witness per branch: a history known to reach it, run through the same assertions before
	// the generated run and failing if it misses its branch, so every assertion runs on every run
	// whatever rapid draws (#500). Components are listed in name order.
	witnesses := []struct {
		branch  string
		comps   []cmPropComponent
		actions []string
	}{
		{"clean start", []cmPropComponent{{name: "comp-a"}}, []string{"start"}},
		{"provider phase failed with a consumer unlaunched",
			[]cmPropComponent{{name: "comp-a", provider: true, startFails: true}, {name: "comp-b"}}, []string{"start"}},
		{"consumer phase failed",
			[]cmPropComponent{{name: "comp-a", provider: true}, {name: "comp-b", startFails: true}}, []string{"start"}},
		{"rollback stop failed",
			[]cmPropComponent{{name: "comp-a", provider: true, startFails: true, stopFailures: 1}, {name: "comp-b"}}, []string{"start"}},
		{"stop retried a strict subset",
			[]cmPropComponent{{name: "comp-a", provider: true, stopFailures: 1}, {name: "comp-b"}}, []string{"start", "stopLive", "stopLive"}},
		{"stop before start", []cmPropComponent{{name: "comp-a"}}, []string{"stopLive"}},
		{"start refused after stop", []cmPropComponent{{name: "comp-a"}}, []string{"stopLive", "start"}},
		{"stop with an ended context", []cmPropComponent{{name: "comp-a"}}, []string{"start", "stopEnded"}},
		{"health named two failed components",
			[]cmPropComponent{{name: "comp-a", provider: true, startFails: true, stopFailures: 1}, {name: "comp-b", provider: true, startFails: true, stopFailures: 1}},
			[]string{"start"}},
	}
	for _, w := range witnesses {
		t.Run(w.branch, func(t *testing.T) {
			reached := map[string]int{}
			actions, done := newCMPropHistory(t, t, w.comps, reached)
			defer done()
			for _, action := range w.actions {
				actions[action]()
				actions[""]()
			}
			if reached[w.branch] == 0 {
				t.Fatalf("witness %+v then %v did not reach %q, so its assertions never ran", w.comps, w.actions, w.branch)
			}
		})
	}

	explored := map[string]int{}
	rapid.Check(t, func(rt *rapid.T) {
		actions, done := newCMPropHistory(t, rt, drawCMPropComponents(rt), explored)
		defer done()

		// A history is an optional Stop before any Start, the first Start, then any sequence of
		// the four actions. Nothing else can come first: a Stop with an ended context is not
		// drawn there (cmPropHistory.stopEnded), and clearing before Start is the same as
		// drawing no Stop failure.
		if rapid.IntRange(0, 3).Draw(rt, "stop before start") == 0 {
			actions["stopLive"]()
			actions[""]()
		}
		actions["start"]()
		actions[""]()
		repeat := map[string]func(*rapid.T){}
		for name, action := range actions {
			repeat[name] = func(*rapid.T) { action() }
		}
		rt.Repeat(repeat)
	})
	for _, w := range witnesses {
		t.Logf("branch %q reached %d times by random histories", w.branch, explored[w.branch])
	}
}

// cmPropComponent is one composed component as drawn or written for a witness: its role, whether
// its Start fails, and how many of its next Stop calls fail.
type cmPropComponent struct {
	name         string
	provider     bool
	startFails   bool
	stopFailures int
}

// drawCMPropComponents draws a composition, in name order.
func drawCMPropComponents(rt *rapid.T) []cmPropComponent {
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
	comps := make([]cmPropComponent, 0, len(names))
	for _, name := range names {
		c := cmPropComponent{name: name}
		switch name {
		case forcedProvider:
			c.provider = true
		case forcedConsumer:
			c.provider = false
		default:
			c.provider = rapid.Bool().Draw(rt, name+" provider")
		}
		switch outcome {
		case "provider phase fails":
			// The forced provider fails; any other component may too.
			c.startFails = name == forcedProvider || rapid.IntRange(0, 2).Draw(rt, name+" start fails") == 0
		case "consumer phase fails":
			// No provider fails; the forced consumer does, and any other consumer may.
			c.startFails = name == forcedConsumer ||
				(!c.provider && rapid.IntRange(0, 2).Draw(rt, name+" start fails") == 0)
		}
		c.stopFailures = rapid.IntRange(0, 2).Draw(rt, name+" stop failures")
		comps = append(comps, c)
	}
	return comps
}

// newCMPropHistory composes comps (in name order) on a new manager and returns the actions a
// history is made of, each checked against the model and failing tb, keyed as rapid's Repeat
// takes them ("" is the check after every action), and done, which releases what the history
// left running. reached counts each branch a history reaches.
func newCMPropHistory(t *testing.T, tb rapid.TB, comps []cmPropComponent, reached map[string]int) (map[string]func(), func()) {
	log := &lifecycleCallLog{}
	models := map[string]*cmPropModel{}
	var names, providers, consumers []string
	var composed []testComponent
	for _, c := range comps {
		m := &cmPropModel{
			name:     c.name,
			provider: c.provider,
			comp:     newLifecycleTestComponent(c.name, log),
			script:   &cmPropStopScript{},
			state:    component.StateInitialized,
		}
		if c.startFails {
			m.comp.startErr = fmt.Errorf("start failure in %s", c.name)
		}
		for i := range c.stopFailures {
			m.script.queue = append(m.script.queue, fmt.Errorf("stop failure %d in %s", i+1, c.name))
		}
		script := m.script
		m.comp.onStop = func(context.Context, int64) error { return script.pop() }
		models[c.name] = m
		names = append(names, c.name)
		if m.provider {
			providers = append(providers, c.name)
			composed = append(composed,
				testComponent{name: c.name, instance: &lifecycleStoreProvider{lifecycleTestComponent: m.comp}})
		} else {
			consumers = append(consumers, c.name)
			composed = append(composed, m.comp.testComponent())
		}
	}
	// The launch order the design fixes: providers by name, then consumers by name.
	launchOrder := slices.Concat(providers, consumers)
	reverseLaunch := slices.Clone(launchOrder)
	slices.Reverse(reverseLaunch)

	cm := newTestComponentManager(t, composed...)
	h := &cmPropHistory{
		t: t, tb: tb, reached: reached, log: log, models: models, names: names,
		providers: providers, consumers: consumers, reverseLaunch: reverseLaunch, cm: cm,
	}
	return map[string]func(){
		"start":         h.start,
		"stopLive":      h.stopLive,
		"stopEnded":     h.stopEnded,
		"clearFailures": h.clearFailures,
		"":              h.check,
	}, h.done
}

// cmPropHistory is one composition on a new manager and the model of what D2 and D4 say the
// manager has done to it so far. Each method is one action of a history or a helper of one,
// failing tb when the manager departs from the model.
type cmPropHistory struct {
	t             *testing.T
	tb            rapid.TB
	reached       map[string]int
	log           *lifecycleCallLog
	models        map[string]*cmPropModel
	names         []string
	providers     []string
	consumers     []string
	reverseLaunch []string // launch order reversed: the order every stop sequence follows
	cm            *ComponentManager
	startCalled   bool
	stopAdmitted  bool
}

func (h *cmPropHistory) done() {
	// Release whatever the history left running: with every failure cleared, a Stop
	// returns nil whatever came before.
	for _, m := range h.models {
		m.script.clear()
	}
	if err := h.cm.Stop(stopContext(h.t)); err != nil {
		h.tb.Errorf("final Stop with no armed failure: %v", err)
	}
}

// stopsSince returns the components whose Stop was called since the log held mark entries.
func (h *cmPropHistory) stopsSince(mark int) []string { return h.log.stopOrder()[mark:] }

// nextStopFailures reads, from the scripts, the failure each component's next Stop returns.
// It is read before the action: a failed Start's rollback runs inside Start (design D2), so
// by the time Start returns the rollback has already popped the queues.
func (h *cmPropHistory) nextStopFailures() map[string]error {
	next := map[string]error{}
	for name, m := range h.models {
		next[name] = m.script.peek()
	}
	return next
}

// expectStops predicts which of the launched components not yet stopped a stop sequence
// calls, in reverse launch order, and which failure each returns, from next as read before
// the action.
func (h *cmPropHistory) expectStops(next map[string]error) (visits []string, fired map[string]error) {
	fired = map[string]error{}
	for _, name := range h.reverseLaunch {
		m := h.models[name]
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
func (h *cmPropHistory) applyStops(visits []string, fired map[string]error) {
	for _, name := range visits {
		m := h.models[name]
		if err := fired[name]; err != nil {
			m.state, m.lastErr = component.StateFailed, err
			continue
		}
		m.stoppedNil, m.state, m.lastErr = true, component.StateStopped, nil
	}
}

func (h *cmPropHistory) requireStops(action string, mark int, want []string) {
	if got := h.stopsSince(mark); !slices.Equal(got, want) {
		h.tb.Fatalf("%s stopped %v, want exact reverse launch order %v", action, got, want)
	}
}

func (h *cmPropHistory) requireCarries(action string, err error, fired map[string]error) {
	for name, want := range fired {
		if !errors.Is(err, want) {
			h.tb.Fatalf("%s returned %v, which does not carry %s's failure %q", action, err, name, want)
		}
	}
}

func (h *cmPropHistory) start() {
	mark := len(h.log.stopOrder())
	startsBefore := map[string]int64{}
	for name, m := range h.models {
		startsBefore[name] = m.comp.starts.Load()
	}
	next := h.nextStopFailures()
	err := h.cm.Start(h.t.Context())

	if h.startCalled || h.stopAdmitted {
		if !errors.Is(err, errs.ErrAlreadyStarted) {
			h.tb.Fatalf("Start after a Start or a Stop returned %v, want errs.ErrAlreadyStarted", err)
		}
		if h.stopAdmitted && !h.startCalled {
			h.reached["start refused after stop"]++
		}
		for name, m := range h.models {
			if got := m.comp.starts.Load(); got != startsBefore[name] {
				h.tb.Fatalf("a refused Start started %s", name)
			}
		}
		h.requireStops("a refused Start", mark, nil)
		return
	}
	h.startCalled = true

	// Phase one starts every provider; phase two runs only if none failed.
	startFailed := map[string]error{}
	launch := func(phase []string) {
		for _, name := range phase {
			m := h.models[name]
			m.launched = true
			if m.comp.startErr != nil {
				startFailed[name] = m.comp.startErr
				m.state, m.lastErr = component.StateFailed, m.comp.startErr
				continue
			}
			m.state, m.lastErr = component.StateStarted, nil
		}
	}
	launch(h.providers)
	providerFailed := len(startFailed) > 0
	if !providerFailed {
		launch(h.consumers)
	}

	if len(startFailed) == 0 {
		if err != nil {
			h.tb.Fatalf("Start with no armed Start failure returned %v", err)
		}
		h.reached["clean start"]++
		h.requireStops("a Start that succeeded", mark, nil)
		return
	}
	if providerFailed && len(h.consumers) > 0 {
		h.reached["provider phase failed with a consumer unlaunched"]++
	} else if !providerFailed {
		h.reached["consumer phase failed"]++
	}
	if err == nil {
		h.tb.Fatalf("Start returned nil with Start failures armed in %v", slices.Sorted(maps.Keys(startFailed)))
	}
	h.requireCarries("Start", err, startFailed)
	for name := range startFailed {
		if want := fmt.Sprintf("component %q", name); !strings.Contains(err.Error(), want) {
			h.tb.Fatalf("Start's error %q does not name %s", err.Error(), want)
		}
	}
	// The rollback stops every launched component, the failed ones included.
	visits, fired := h.expectStops(next)
	h.requireStops("the failed Start's rollback", mark, visits)
	h.requireCarries("Start", err, fired)
	if len(fired) > 0 {
		h.reached["rollback stop failed"]++
	}
	h.applyStops(visits, fired)
}

func (h *cmPropHistory) stopLive() {
	mark := len(h.log.stopOrder())
	visits, fired := h.expectStops(h.nextStopFailures())
	err := h.cm.Stop(stopContext(h.t))
	first := !h.startCalled && !h.stopAdmitted
	h.stopAdmitted = true

	if first {
		h.reached["stop before start"]++
		if err != nil {
			h.tb.Fatalf("Stop before any Start returned %v, want nil", err)
		}
		h.requireStops("a Stop before any Start", mark, nil)
		return
	}
	h.requireStops("Stop", mark, visits)
	launched := 0
	for _, m := range h.models {
		if m.launched {
			launched++
		}
	}
	if len(visits) > 0 && len(visits) < launched {
		h.reached["stop retried a strict subset"]++
	}
	if len(fired) == 0 {
		if err != nil {
			h.tb.Fatalf("Stop with no armed Stop failure in %v returned %v", visits, err)
		}
	} else {
		if err == nil {
			h.tb.Fatalf("Stop returned nil with Stop failures armed in %v", slices.Sorted(maps.Keys(fired)))
		}
		h.requireCarries("Stop", err, fired)
	}
	h.applyStops(visits, fired)
}

// The history never holds a Stop with an ended context before the first Start: whether it
// uses the manager up is not stated (design D2), and the history starts before Repeat.
func (h *cmPropHistory) stopEnded() {
	mark := len(h.log.stopOrder())
	ctx, cancel := context.WithCancel(h.t.Context())
	cancel()
	err := h.cm.Stop(ctx)
	h.reached["stop with an ended context"]++
	if !errors.Is(err, context.Canceled) {
		h.tb.Fatalf("Stop with an ended context returned %v, want context.Canceled", err)
	}
	h.requireStops("a Stop with an ended context", mark, nil)
}

// Clearing disarms every Stop failure still queued. A Start failure is not cleared: the
// first Start runs before Repeat, and a later Start is refused, so it would change nothing.
func (h *cmPropHistory) clearFailures() {
	for _, m := range h.models {
		m.script.clear()
	}
}

func (h *cmPropHistory) check() {
	statuses := h.cm.GetComponentStatus()
	if len(statuses) != len(h.models) {
		h.tb.Fatalf("GetComponentStatus reports %d components, want %d", len(statuses), len(h.models))
	}
	var failed []string
	for _, name := range h.names {
		m := h.models[name]
		status, found := statuses[name]
		if !found {
			h.tb.Fatalf("GetComponentStatus has no %s", name)
		}
		if status.State != m.state {
			h.tb.Fatalf("component %s is %s, want %s", name, status.State, m.state)
		}
		if m.lastErr == nil && status.LastError != nil {
			h.tb.Fatalf("component %s (%s) has last error %v, want none", name, status.State, status.LastError)
		}
		if m.lastErr != nil && !errors.Is(status.LastError, m.lastErr) {
			h.tb.Fatalf("component %s's last error is %v, want %q", name, status.LastError, m.lastErr)
		}
		want := int64(0)
		if m.launched {
			want = 1
		}
		if got := m.comp.starts.Load(); got != want {
			h.tb.Fatalf("component %s was started %d times, want %d", name, got, want)
		}
		if m.state == component.StateFailed {
			failed = append(failed, name)
		}
	}

	err := h.cm.healthCheck()
	if len(failed) == 0 {
		if err != nil {
			h.tb.Fatalf("health check with no failed component returned %v", err)
		}
		return
	}
	if err == nil {
		h.tb.Fatalf("health check returned healthy with %v failed", failed)
	}
	if len(failed) >= 2 {
		h.reached["health named two failed components"]++
	}
	message, last := err.Error(), -1
	for _, name := range h.names {
		m := h.models[name]
		at := strings.Index(message, fmt.Sprintf("component %q failed", name))
		if m.state != component.StateFailed {
			if at >= 0 {
				h.tb.Fatalf("health check %q names %s, which is %s", message, name, m.state)
			}
			continue
		}
		if at < 0 {
			h.tb.Fatalf("health check %q does not name failed component %s", message, name)
		}
		if at < last {
			h.tb.Fatalf("health check %q does not name the failed components %v in name order", message, failed)
		}
		last = at
		if !errors.Is(err, m.lastErr) {
			h.tb.Fatalf("health check %q does not carry %s's last error %q", message, name, m.lastErr)
		}
	}
}
