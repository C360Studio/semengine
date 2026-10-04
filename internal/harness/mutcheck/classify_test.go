package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// The streams under testdata/events were recorded with go1.26.6 from the module in
// testdata/record, with the program's own command line:
//
//	GOFLAGS=<overlay or empty> go test -json -count=1 -cpu 1 -race -timeout <2m, 1s for timeout> -run '^<test>$' ./basic
//
// The mutant runs mapped basic/value.go to mutant/value.go.txt (Value returns 2) with
// GOFLAGS=-overlay=<file>; build-failed mapped it to mutant/broken.go.txt; pass-exit-nonzero ran
// ./exit, whose TestMain exits 3 after its test passes. A stream holds no exit status, so each
// case below states the status go test exited with, as it was recorded.

const testBound = 4 * time.Minute

// raceText is the line Go's race detector prints in a test that raced (design P18).
const raceText = "race detected during execution of test"

type recordedRun struct {
	file string // under testdata/events, without .jsonl
	test string
	end  processEnd
}

var (
	recPass         = recordedRun{"pass", "TestValue", processEnd{code: 0}}
	recFailExpected = recordedRun{"fail-expected", "TestValue", processEnd{code: 1}}
	recLogThenFail  = recordedRun{"log-then-fail", "TestLogThenFail", processEnd{code: 1}}
	recPanicAfter   = recordedRun{"panic-after", "TestPanicAfter", processEnd{code: 1}}
	recSubtest      = recordedRun{"subtest", "TestOuter/inner", processEnd{code: 1}}
	recRace         = recordedRun{"race", "TestRace", processEnd{code: 1}}
	recTimeout      = recordedRun{"timeout", "TestTimeout", processEnd{code: 1}}
	recKilled       = recordedRun{"killed", "TestKilled", processEnd{code: 1}}
	recBuildFailed  = recordedRun{"build-failed", "TestValue", processEnd{code: 1}}
	recNoTest       = recordedRun{"no-test", "TestNope", processEnd{code: 0}}
	recPassNonzero  = recordedRun{"pass-exit-nonzero", "TestPasses", processEnd{code: 1}}
)

func stream(t *testing.T, file string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "events", file+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func expectAt(locations ...string) expectation { return expectation{locations: locations} }

// TestObserveRecordedRuns (design D5, D11 item 1): each recorded per-run case is read into the
// facts and lines written here from the recording itself, and given the reading the spec's rules
// give it.
func TestObserveRecordedRuns(t *testing.T) {
	type want struct {
		facts     facts
		outcome   outcome
		words     []string // each appears in the reading's reason
		expected  []string // the expected lines, in full
		locations []string
		note      string // appears among the notes
	}
	for _, tc := range []struct {
		name   string
		rec    recordedRun
		mutant bool
		exp    expectation
		want   want
	}{
		{"pass, baseline", recPass, false, expectAt("value_test.go:10"), want{
			facts: facts{selected: true, result: "pass", exitZero: true}, outcome: pass}},
		{"pass, mutant", recPass, true, expectAt("value_test.go:10"), want{
			facts: facts{selected: true, result: "pass", exitZero: true}, outcome: pass}},
		{"detected at the expected location", recFailExpected, true, expectAt("value_test.go:10"), want{
			facts:    facts{selected: true, result: "fail", expectedHit: true},
			outcome:  detection,
			expected: []string{"    value_test.go:10: Value() = 2, want 1"}, locations: []string{"value_test.go:10"}}},
		{"detected by an expected text", recFailExpected, true, expectation{texts: []string{"want 1"}}, want{
			facts:    facts{selected: true, result: "fail", expectedHit: true},
			outcome:  detection,
			expected: []string{"    value_test.go:10: Value() = 2, want 1"}, locations: []string{"value_test.go:10"}}},
		{"a different assertion fails", recFailExpected, true, expectAt("value_test.go:11"), want{
			facts:   facts{selected: true, result: "fail"},
			outcome: inconclusive, words: []string{"no expected line"}, locations: []string{"value_test.go:10"}}},
		{"a location is matched whole, not as a prefix", recFailExpected, true, expectAt("value_test.go:1"), want{
			facts:   facts{selected: true, result: "fail"},
			outcome: inconclusive, locations: []string{"value_test.go:10"}}},
		{"the unchanged code fails", recFailExpected, false, expectAt("value_test.go:10"), want{
			facts:    facts{selected: true, result: "fail", expectedHit: true},
			outcome:  inconclusive,
			expected: []string{"    value_test.go:10: Value() = 2, want 1"}, locations: []string{"value_test.go:10"}}},
		{"a log line at the expected location", recLogThenFail, true, expectAt("value_test.go:15"), want{
			facts:     facts{selected: true, result: "fail", expectedHit: true},
			outcome:   detection,
			expected:  []string{"    value_test.go:15: checking Value"},
			locations: []string{"value_test.go:15", "value_test.go:17"}}},
		{"a panic after the expected failure", recPanicAfter, true, expectAt("value_test.go:23"), want{
			facts:    facts{selected: true, result: "fail", expectedHit: true},
			outcome:  detection,
			expected: []string{"    value_test.go:23: Value() = 2, want 1"}, locations: []string{"value_test.go:23"},
			note: "panic: assignment to entry in nil map"}},
		{"a subtest named Name/Sub", recSubtest, true, expectAt("value_test.go:34"), want{
			facts:    facts{selected: true, result: "fail", expectedHit: true},
			outcome:  detection,
			expected: []string{"    value_test.go:34: Value() = 2, want 1"}, locations: []string{"value_test.go:34"}}},
		{"a subtest's line counts for the named test", recordedRun{"subtest", "TestOuter", processEnd{code: 1}}, true,
			expectAt("value_test.go:34"), want{
				facts:    facts{selected: true, result: "fail", expectedHit: true},
				outcome:  detection,
				expected: []string{"    value_test.go:34: Value() = 2, want 1"}, locations: []string{"value_test.go:34"}}},
		{"a race report that is not expected", recRace, true, expectAt("value_test.go:43"), want{
			facts:   facts{selected: true, result: "fail", race: true},
			outcome: inconclusive, words: []string{"race report"}, locations: []string{"testing.go:1712"}}},
		{"a race report that is expected", recRace, true, expectation{texts: []string{raceText}}, want{
			facts:     facts{selected: true, result: "fail", race: true, expectedHit: true},
			outcome:   detection,
			expected:  []string{"    testing.go:1712: race detected during execution of test"},
			locations: []string{"testing.go:1712"}}},
		{"a whole-run timeout after an expected line", recTimeout, true, expectAt("value_test.go:53"), want{
			facts:   facts{selected: true, timedOut: true, expectedHit: true},
			outcome: inconclusive, words: []string{"timeout"},
			expected: []string{"    value_test.go:53: Value() = 2, want 1"}, locations: []string{"value_test.go:53"},
			note: "panic: test timed out after 1s"}},
		{"killed by a signal", recKilled, true, expectAt("value_test.go:60"), want{
			facts:   facts{selected: true, signal: "killed"},
			outcome: inconclusive, words: []string{"signal", "killed"}}},
		{"the wrong change does not build", recBuildFailed, true, expectAt("value_test.go:10"), want{
			facts:   facts{buildFailed: true},
			outcome: invalid, words: []string{"does not build"}}},
		{"the unchanged code does not build", recBuildFailed, false, expectAt("value_test.go:10"), want{
			facts:   facts{buildFailed: true},
			outcome: inconclusive, words: []string{"does not build"}}},
		{"nothing selected", recNoTest, false, expectAt("value_test.go:10"), want{
			facts:   facts{exitZero: true},
			outcome: inconclusive, words: []string{"selected no test", "integration"}}},
		{"passed, but go test exited non-zero", recPassNonzero, true, expectAt("exit_test.go:13"), want{
			facts:   facts{selected: true, result: "pass"},
			outcome: inconclusive, words: []string{"non-zero"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := observe(stream(t, tc.rec.file), tc.rec.end, tc.rec.test, tc.exp)
			if got.facts != tc.want.facts {
				t.Errorf("facts:\n got  %+v\n want %+v", got.facts, tc.want.facts)
			}
			if !slices.Equal(got.expectedLines, tc.want.expected) {
				t.Errorf("expected lines: got %q, want %q", got.expectedLines, tc.want.expected)
			}
			if !slices.Equal(got.locations, tc.want.locations) {
				t.Errorf("locations: got %q, want %q", got.locations, tc.want.locations)
			}
			if tc.want.note != "" && !slices.ContainsFunc(got.notes, func(n string) bool { return strings.Contains(n, tc.want.note) }) {
				t.Errorf("notes %q lack %q", got.notes, tc.want.note)
			}
			r := read(got.facts, tc.mutant, testBound)
			if r.outcome != tc.want.outcome {
				t.Errorf("reading: got %s (%s), want %s", r.outcome, r.reason, tc.want.outcome)
			}
			for _, w := range tc.want.words {
				if !strings.Contains(r.reason, w) {
					t.Errorf("reason %q lacks %q", r.reason, w)
				}
			}
		})
	}
}

// TestObserveBoundedRun: a run the program stopped at the bound reads inconclusive and names the
// bound, whatever its stream showed before it was stopped.
func TestObserveBoundedRun(t *testing.T) {
	head := strings.Join(strings.SplitAfter(string(stream(t, "pass")), "\n")[:3], "")
	end := processEnd{code: -1, signal: "killed", bounded: true}
	got := observe([]byte(head), end, "TestValue", expectAt("value_test.go:10"))
	want := facts{selected: true, signal: "killed", bounded: true}
	if got.facts != want {
		t.Fatalf("facts:\n got  %+v\n want %+v", got.facts, want)
	}
	for _, mutant := range []bool{false, true} {
		r := read(got.facts, mutant, testBound)
		if r.outcome != inconclusive || !strings.Contains(r.reason, testBound.String()) {
			t.Errorf("mutant=%v: got %s (%s), want inconclusive naming %s", mutant, r.outcome, r.reason, testBound)
		}
	}
}

// wantReading is the reading the spec gives, written from "Outcome classification" and design D5
// as the conditions for each outcome, not as the program's ordered cases. Detection needs a
// failed named test with an expected line; a run ended by the timeout, a signal or the bound is
// inconclusive whatever it printed; a mutant that does not build is invalid; an unchanged-code
// run passes only when the named test passed and go test exited zero.
func wantReading(f facts, mutant bool) outcome {
	endedEarly := f.timedOut || f.signal != "" || f.bounded
	ran := !f.buildFailed && f.selected && !endedEarly
	switch {
	case mutant && f.buildFailed && !f.bounded:
		return invalid
	case ran && f.result == "pass" && f.exitZero:
		return pass
	case mutant && ran && f.result == "fail" && f.expectedHit:
		return detection
	default:
		return inconclusive
	}
}

// TestReadEveryCombination enumerates every combination of the per-run inputs, for a mutant run
// and a run of the unchanged code, and compares the program's reading with wantReading.
func TestReadEveryCombination(t *testing.T) {
	results := []string{"", "pass", "fail", "skip"}
	checked := 0
	for bits := 0; bits < 1<<8; bits++ {
		for _, result := range results {
			bit := func(i int) bool { return bits&(1<<i) != 0 }
			f := facts{
				buildFailed: bit(0), selected: bit(1), timedOut: bit(2), bounded: bit(4),
				result: result, exitZero: bit(5), expectedHit: bit(6), race: bit(7),
			}
			if bit(3) {
				f.signal = "killed"
			}
			for _, mutant := range []bool{false, true} {
				checked++
				got := read(f, mutant, testBound)
				if want := wantReading(f, mutant); got.outcome != want {
					t.Errorf("%+v mutant=%v: got %s (%s), want %s", f, mutant, got.outcome, got.reason, want)
					continue
				}
				if got.reason == "" {
					t.Errorf("%+v mutant=%v: %s with no reason", f, mutant, got.outcome)
				}
				// The words the spec puts in particular reasons.
				switch {
				case f.bounded && !strings.Contains(got.reason, testBound.String()):
					t.Errorf("%+v: reason %q does not name the bound", f, got.reason)
				case !f.bounded && !f.buildFailed && !f.selected && !strings.Contains(got.reason, "integration"):
					t.Errorf("%+v: reason %q does not mention integration tests", f, got.reason)
				case got.outcome == inconclusive && !f.bounded && !f.buildFailed && f.selected && !f.timedOut &&
					f.signal != "" && !strings.Contains(got.reason, f.signal):
					t.Errorf("%+v: reason %q does not name the signal", f, got.reason)
				case mutant && got.outcome == inconclusive && f.selected && !f.buildFailed && !f.timedOut &&
					f.signal == "" && !f.bounded && f.result == "fail" && f.race && !f.expectedHit &&
					!strings.Contains(got.reason, "race"):
					t.Errorf("%+v: reason %q does not mention the race report", f, got.reason)
				}
			}
		}
	}
	if want := 2 * 256 * len(results); checked != want {
		t.Fatalf("checked %d combinations, want %d", checked, want)
	}
}

// runAs reads a recorded run as a run of the given kind.
func runAs(t *testing.T, rec recordedRun, kind runKind, index int, exp expectation) runRecord {
	t.Helper()
	obs := observe(stream(t, rec.file), rec.end, rec.test, exp)
	return runRecord{kind: kind, index: index, end: rec.end, obs: obs, reading: read(obs.facts, kind == mutantRun, testBound)}
}

var kindNames = map[runKind]string{baselineRun: "baseline", mutantRun: "mutant", afterRun: "after", reachRun: "reach"}

// planned answers each run the sequence asks for from a plan of recorded runs, and lists the runs
// asked for, so a test sees which runs an early stop left out. A run missing from the plan is the
// passing recording.
type planned struct {
	t        *testing.T
	exp      expectation
	plan     map[string]recordedRun // keyed "baseline 2", "mutant 1", "after 1", "reach 1"
	asked    []string
	cancel   context.CancelFunc // when set, called once the run named by cancelAt is asked for
	cancelAt string
}

func (p *planned) run(ctx context.Context, kind runKind, index int) (runRecord, error) {
	name := fmt.Sprintf("%s %d", kindNames[kind], index)
	p.asked = append(p.asked, name)
	if p.cancel != nil && name == p.cancelAt {
		p.cancel()
		return runRecord{}, ctx.Err()
	}
	rec, ok := p.plan[name]
	if !ok {
		rec = recPass
	}
	exp := p.exp
	if kind != mutantRun {
		exp = expectAt("value_test.go:10")
	}
	return runAs(p.t, rec, kind, index, exp), nil
}

// TestCheckVerdicts: one named example for each rule of "The runs" and "Outcome classification"
// that spans runs: agreement, early stops, the after-run, the fingerprint and reach. The baseline,
// after and reach runs are the passing recording unless the plan says otherwise.
func TestCheckVerdicts(t *testing.T) {
	all := func(rec recordedRun, kinds ...string) map[string]recordedRun {
		m := map[string]recordedRun{}
		for _, k := range kinds {
			m[k] = rec
		}
		return m
	}
	mutants3 := []string{"mutant 1", "mutant 2", "mutant 3"}
	full := []string{"baseline 1", "baseline 2", "baseline 3", "mutant 1", "mutant 2", "mutant 3", "after 1"}
	withReach := append(slices.Clone(full), "reach 1")
	for _, tc := range []struct {
		name       string
		exp        expectation
		plan       map[string]recordedRun
		reachState reachState
		treeAfter  string // the fingerprint after the last run; "before" when unchanged
		wantRuns   []string
		want       outcome
		words      []string
	}{
		{name: "detected", exp: expectAt("value_test.go:10"), plan: all(recFailExpected, mutants3...),
			wantRuns: full, want: detection, words: []string{"expected line"}},
		{name: "a baseline run fails", exp: expectAt("value_test.go:10"),
			plan:     map[string]recordedRun{"baseline 2": recFailExpected},
			wantRuns: []string{"baseline 1", "baseline 2"}, want: inconclusive, words: []string{"baseline run 2"}},
		{name: "a flaky baseline", exp: expectAt("value_test.go:10"),
			plan:     map[string]recordedRun{"baseline 3": recFailExpected},
			wantRuns: []string{"baseline 1", "baseline 2", "baseline 3"}, want: inconclusive, words: []string{"baseline run 3"}},
		{name: "a mutant run times out", exp: expectAt("value_test.go:53"),
			plan:     map[string]recordedRun{"mutant 1": recTimeout},
			wantRuns: []string{"baseline 1", "baseline 2", "baseline 3", "mutant 1", "after 1"},
			want:     inconclusive, words: []string{"timeout"}},
		{name: "killed by a signal", exp: expectAt("value_test.go:60"),
			plan:     map[string]recordedRun{"mutant 1": recKilled},
			wantRuns: []string{"baseline 1", "baseline 2", "baseline 3", "mutant 1", "after 1"},
			want:     inconclusive, words: []string{"killed"}},
		{name: "mutant runs disagree", exp: expectAt("value_test.go:10"),
			plan:     map[string]recordedRun{"mutant 1": recFailExpected, "mutant 3": recFailExpected},
			wantRuns: full, want: inconclusive, words: []string{"disagree"}},
		{name: "a different assertion fails", exp: expectAt("value_test.go:11"), plan: all(recFailExpected, mutants3...),
			wantRuns: full, want: inconclusive, words: []string{"value_test.go:10"}},
		{name: "a failure that names no expected location", exp: expectAt("value_test.go:99"),
			plan: all(recPassNonzero, mutants3...), wantRuns: withReach, want: inconclusive},
		{name: "a race report that is not expected", exp: expectAt("value_test.go:43"), plan: all(recRace, mutants3...),
			wantRuns: full, want: inconclusive, words: []string{"race"}},
		{name: "a race report that is expected", exp: expectation{texts: []string{raceText}}, plan: all(recRace, mutants3...),
			wantRuns: full, want: detection},
		{name: "a panic after the expected failure", exp: expectAt("value_test.go:23"), plan: all(recPanicAfter, mutants3...),
			wantRuns: full, want: detection},
		{name: "the wrong change does not build", exp: expectAt("value_test.go:10"), plan: all(recBuildFailed, mutants3...),
			wantRuns: full, want: invalid, words: []string{"does not build"}},
		{name: "the after-run fails", exp: expectAt("value_test.go:10"),
			plan:     map[string]recordedRun{"mutant 1": recFailExpected, "mutant 2": recFailExpected, "mutant 3": recFailExpected, "after 1": recFailExpected},
			wantRuns: full, want: inconclusive, words: []string{"after-run"}},
		{name: "the tree changes during the check", exp: expectAt("value_test.go:10"), plan: all(recFailExpected, mutants3...),
			treeAfter: "changed", wantRuns: full, want: inconclusive, words: []string{"tree changed during the check"}},
		{name: "a survivor that was reached", exp: expectAt("value_test.go:10"), reachState: reached,
			wantRuns: withReach, want: survivor, words: []string{"seed 7", "holds"}},
		{name: "a survivor whose reach could not be measured", exp: expectAt("value_test.go:10"), reachState: notMeasurable,
			wantRuns: withReach, want: survivor, words: []string{"reach could not be measured", "seed 7"}},
		{name: "not reached", exp: expectAt("value_test.go:10"), reachState: notReached,
			wantRuns: withReach, want: invalid, words: []string{"did not reach the wrong change"}},
		{name: "the reach run fails", exp: expectAt("value_test.go:10"), reachState: reached,
			plan:     map[string]recordedRun{"reach 1": recFailExpected},
			wantRuns: withReach, want: inconclusive, words: []string{"reach run"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &planned{t: t, exp: tc.exp, plan: tc.plan}
			c, err := sequence(t.Context(), 3, p.run)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(p.asked, tc.wantRuns) {
				t.Errorf("runs made: got %q, want %q", p.asked, tc.wantRuns)
			}
			c.seed, c.treeBefore, c.treeAfter = 7, "before", "before"
			if tc.treeAfter != "" {
				c.treeAfter = tc.treeAfter
			}
			if c.reach != nil {
				c.reachState = tc.reachState
			}
			got, reason := decide(c)
			if got != tc.want {
				t.Errorf("verdict: got %s (%s), want %s", got, reason, tc.want)
			}
			for _, w := range tc.words {
				if !strings.Contains(reason, w) {
					t.Errorf("reason %q lacks %q", reason, w)
				}
			}
		})
	}
}

// TestSequenceInterrupted: a run that reports the program was interrupted ends the sequence with
// that error, and no later run is made.
func TestSequenceInterrupted(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	p := &planned{t: t, exp: expectAt("value_test.go:10"), cancel: cancel, cancelAt: "mutant 2"}
	_, err := sequence(ctx, 3, p.run)
	if err == nil {
		t.Fatal("got no error from an interrupted sequence")
	}
	if want := []string{"baseline 1", "baseline 2", "baseline 3", "mutant 1", "mutant 2"}; !slices.Equal(p.asked, want) {
		t.Fatalf("runs made: got %q, want %q", p.asked, want)
	}
}
