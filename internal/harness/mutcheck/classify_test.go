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
// ./exit, whose TestMain exits 3 after its test passes; killed-before-run ran ./early, whose
// TestMain kills its own process before any test starts. A stream holds no exit status, so each
// case below states the status go test exited with, as it was recorded.
//
// hang, hang-pass and logged-timeout were recorded the same way with go1.26.10, from
// basic/hang_test.go: hang ran TestHang with the mutant overlay and -timeout 1s (Value returns 2,
// so the test blocks with no assertion); hang-pass ran TestHang on the unchanged code with
// -timeout 2m; logged-timeout ran TestLoggedTimeout on the unchanged code with -timeout 2m (the
// test prints go test's timeout panic text and passes).

const testBound = 4 * time.Minute

// raceText is the line Go's race detector prints in a test that raced.
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
	recKilledEarly  = recordedRun{"killed-before-run", "TestEarly", processEnd{code: 1}}
	recHang         = recordedRun{"hang", "TestHang", processEnd{code: 1}}
	recLoggedTO     = recordedRun{"logged-timeout", "TestLoggedTimeout", processEnd{code: 0}}
	// recTimeoutBeforeRun is the timeout recording read for a test that never ran in it: go
	// test's timeout ended the run before the named test started.
	recTimeoutBeforeRun = recordedRun{"timeout", "TestOther", processEnd{code: 1}}
	// recBoundedTimeout is the timeout recording as a run the program stopped at the bound.
	recBoundedTimeout = recordedRun{"timeout", "TestTimeout", processEnd{code: -1, signal: "killed", bounded: true}}
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

// TestObserveRecordedRuns: each recorded per-run case is read into the facts and lines written
// here from the recording itself, and given the reading the spec's rules give it.
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
		// An expected line before the panic does not change the reading.
		{"a hang after an expected line", recTimeout, true, expectAt("value_test.go:53"), want{
			facts:   facts{selected: true, timedOut: true, expectedHit: true},
			outcome: hang, words: []string{"timeout"},
			expected: []string{"    value_test.go:53: Value() = 2, want 1"}, locations: []string{"value_test.go:53"},
			note: "panic: test timed out after 1s"}},
		{"a hang with no expected line", recHang, true, expectAt("value_test.go:99"), want{
			facts:   facts{selected: true, timedOut: true},
			outcome: hang, words: []string{"timeout"},
			note: "panic: test timed out after 1s"}},
		{"a timeout on the unchanged code", recHang, false, expectAt("value_test.go:99"), want{
			facts:   facts{selected: true, timedOut: true},
			outcome: inconclusive, words: []string{"timeout"}}},
		{"a timeout before the named test ran", recTimeoutBeforeRun, true, expectAt("value_test.go:53"), want{
			facts:   facts{timedOut: true},
			outcome: inconclusive, words: []string{"timeout"}}},
		{"a test that passed after printing the timeout text", recLoggedTO, true, expectAt("value_test.go:99"), want{
			facts:   facts{selected: true, timedOut: true, result: "pass", exitZero: true},
			outcome: inconclusive, words: []string{"timeout"}}},
		{"a timeout panic stopped at the bound", recBoundedTimeout, true, expectAt("value_test.go:53"), want{
			facts:   facts{selected: true, timedOut: true, expectedHit: true, signal: "killed", bounded: true},
			outcome: inconclusive, words: []string{"bound"},
			expected: []string{"    value_test.go:53: Value() = 2, want 1"}, locations: []string{"value_test.go:53"}}},
		{"killed by a signal", recKilled, true, expectAt("value_test.go:60"), want{
			facts:   facts{selected: true, signal: "killed"},
			outcome: inconclusive, words: []string{"signal", "killed"}}},
		{"killed before the named test started", recKilledEarly, true, expectAt("early_test.go:15"), want{
			facts:   facts{signal: "killed"},
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
		if r.outcome != inconclusive || !strings.Contains(r.reason, testBound.String()) || !strings.Contains(r.reason, "process group") {
			t.Errorf("mutant=%v: got %s (%s), want inconclusive naming %s and the process group", mutant, r.outcome, r.reason, testBound)
		}
	}
}

// TestObserveElapsed: the package's elapsed time is read from the recorded package pass or fail
// event, and only from it: a test's own Elapsed is not the package's, and a stream cut before the
// package's result reports none, which differs from zero.
func TestObserveElapsed(t *testing.T) {
	head := []byte(strings.Join(strings.SplitAfter(string(stream(t, "pass")), "\n")[:5], ""))
	for _, tc := range []struct {
		name     string
		data     []byte
		want     time.Duration
		reported bool
	}{
		{"a package that passed", stream(t, "pass"), 1247 * time.Millisecond, true},
		{"a package that failed by the timeout", stream(t, "timeout"), 1168 * time.Millisecond, true},
		{"a hang", stream(t, "hang"), 1358 * time.Millisecond, true},
		{"only the test's own elapsed time", head, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := observe(tc.data, processEnd{}, "TestValue", expectAt("value_test.go:10"))
			if got.elapsed != tc.want || got.elapsedReported != tc.reported {
				t.Errorf("elapsed: got %s (reported %v), want %s (reported %v)", got.elapsed, got.elapsedReported, tc.want, tc.reported)
			}
		})
	}
}

// wantReading is the reading the spec gives, written from "Outcome classification" as the
// conditions for each outcome, not as the program's ordered cases. Detection needs a failed named
// test with an expected line; a mutant run that built, ran the named test and was ended by go
// test's timeout with no result, no signal and before the bound is a hang; any other run ended by
// the timeout, a signal or the bound is inconclusive whatever it printed; a mutant that does not
// build is invalid; an unchanged-code run passes only when the named test passed and go test
// exited zero.
func wantReading(f facts, mutant bool) outcome {
	endedEarly := f.timedOut || f.signal != "" || f.bounded
	ran := !f.buildFailed && f.selected && !endedEarly
	switch {
	case mutant && f.buildFailed && !f.bounded:
		return invalid
	case mutant && !f.buildFailed && f.selected && f.timedOut && f.result == "" && f.signal == "" && !f.bounded:
		return hang
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
				case !f.bounded && !f.buildFailed && f.timedOut && !strings.Contains(got.reason, "timeout"):
					t.Errorf("%+v: reason %q does not name the timeout", f, got.reason)
				case !f.bounded && !f.buildFailed && !f.timedOut && f.signal != "" && !strings.Contains(got.reason, f.signal):
					t.Errorf("%+v: reason %q does not name the signal", f, got.reason)
				case !f.bounded && !f.buildFailed && !f.timedOut && f.signal == "" && !f.selected &&
					!strings.Contains(got.reason, "integration"):
					t.Errorf("%+v: reason %q does not mention integration tests", f, got.reason)
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
	plan     map[string]recordedRun   // keyed "baseline 2", "mutant 1", "after 1", "reach 1"
	elapsed  map[string]time.Duration // the package's elapsed time a run reports, in place of its recording's; negative: none reported
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
	r := runAs(p.t, rec, kind, index, exp)
	if d, ok := p.elapsed[name]; ok {
		r.obs.elapsed, r.obs.elapsedReported = d, d >= 0
	}
	return r, nil
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
	// hung is the runs a check makes when mutant run 1 ends early: no mutant run 2, no reach run.
	hung := []string{"baseline 1", "baseline 2", "baseline 3", "mutant 1", "after 1"}
	controls3s := map[string]time.Duration{"baseline 1": 3 * time.Second, "baseline 2": 3 * time.Second, "baseline 3": 3 * time.Second, "after 1": 3 * time.Second}
	for _, tc := range []struct {
		name       string
		exp        expectation
		plan       map[string]recordedRun
		reachState reachState
		treeAfter  string                   // the fingerprint after the last run; "before" when unchanged
		timeout    time.Duration            // -timeout; 2m, the program's default, when zero
		elapsed    map[string]time.Duration // see planned.elapsed
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
		// mutation-check › "Detection by hang" and "What a detection by hang reports".
		{name: "detected by hang", exp: expectAt("value_test.go:99"), timeout: 45 * time.Second, elapsed: controls3s,
			plan:     map[string]recordedRun{"mutant 1": recHang},
			wantRuns: hung, want: detection,
			words: []string{"by hang, not by an assertion", "timeout of 45s", "margin of 15s", "slowest baseline run took 3s",
				"after-run 3s", "would read the same"}},
		{name: "a hang after an expected line", exp: expectAt("value_test.go:53"),
			plan:     map[string]recordedRun{"mutant 1": recTimeout},
			wantRuns: hung, want: detection, words: []string{"by hang", "1.247s"}},
		{name: "a slow baseline", exp: expectAt("value_test.go:99"), timeout: 45 * time.Second,
			elapsed:  map[string]time.Duration{"baseline 1": 3 * time.Second, "baseline 2": 20 * time.Second, "baseline 3": 3 * time.Second, "after 1": 3 * time.Second},
			plan:     map[string]recordedRun{"mutant 1": recHang},
			wantRuns: hung, want: inconclusive, words: []string{"baseline run 2", "20s", "margin of 15s"}},
		{name: "a slow after-run", exp: expectAt("value_test.go:99"), timeout: 45 * time.Second,
			elapsed:  map[string]time.Duration{"baseline 1": 3 * time.Second, "baseline 2": 3 * time.Second, "baseline 3": 3 * time.Second, "after 1": 20 * time.Second},
			plan:     map[string]recordedRun{"mutant 1": recHang},
			wantRuns: hung, want: inconclusive, words: []string{"after-run", "20s", "margin of 15s"}},
		// "At most a third": a control run at exactly the margin is within it.
		{name: "a baseline at exactly the margin", exp: expectAt("value_test.go:99"), timeout: 45 * time.Second,
			elapsed:  map[string]time.Duration{"baseline 1": 3 * time.Second, "baseline 2": 15 * time.Second, "baseline 3": 3 * time.Second, "after 1": 3 * time.Second},
			plan:     map[string]recordedRun{"mutant 1": recHang},
			wantRuns: hung, want: detection, words: []string{"by hang", "slowest baseline run took 15s", "margin of 15s"}},
		// The recorded runs' 1.247s is over a third of 3s: the -race build's exit pause alone puts a
		// -timeout under about 3.75s out of reach of a detection by hang.
		{name: "a timeout too short for the margin", exp: expectAt("value_test.go:99"), timeout: 3 * time.Second,
			plan:     map[string]recordedRun{"mutant 1": recHang},
			wantRuns: hung, want: inconclusive, words: []string{"baseline run 1", "1.247s", "margin of 1s"}},
		{name: "an after-run that reported no elapsed time", exp: expectAt("value_test.go:99"),
			elapsed:  map[string]time.Duration{"after 1": -1},
			plan:     map[string]recordedRun{"mutant 1": recHang},
			wantRuns: hung, want: inconclusive, words: []string{"after-run", "no elapsed time"}},
		{name: "stopped at the bound", exp: expectAt("value_test.go:53"), timeout: 45 * time.Second, elapsed: controls3s,
			plan:     map[string]recordedRun{"mutant 1": recBoundedTimeout},
			wantRuns: hung, want: inconclusive, words: []string{"bound"}},
		{name: "a timeout before the named test runs", exp: expectAt("value_test.go:53"),
			plan:     map[string]recordedRun{"mutant 1": recTimeoutBeforeRun},
			wantRuns: hung, want: inconclusive, words: []string{"mutant run 1", "timeout"}},
		// The named test passed in its one mutant run, so the reach run is made as before.
		{name: "a test that reported a result", exp: expectAt("value_test.go:99"),
			plan:     map[string]recordedRun{"mutant 1": recLoggedTO},
			wantRuns: append(slices.Clone(hung), "reach 1"), want: inconclusive, words: []string{"mutant run 1", "timeout"}},
		{name: "a passing mutant run before the hang", exp: expectAt("value_test.go:99"),
			plan:     map[string]recordedRun{"mutant 2": recHang},
			wantRuns: []string{"baseline 1", "baseline 2", "baseline 3", "mutant 1", "mutant 2", "after 1"},
			want:     inconclusive, words: []string{"disagree", "run 1 pass", "run 2 hang", "2 of the 3 mutant runs"}},
		{name: "a detected mutant run before the hang", exp: expectAt("value_test.go:10"),
			plan:     map[string]recordedRun{"mutant 1": recFailExpected, "mutant 2": recHang},
			wantRuns: []string{"baseline 1", "baseline 2", "baseline 3", "mutant 1", "mutant 2", "after 1"},
			want:     inconclusive, words: []string{"disagree", "run 1 detection", "run 2 hang", "2 of the 3 mutant runs"}},
		// mutation-check › "Inconclusive", "Whole-run timeout": the after-run ends by the timeout.
		{name: "whole-run timeout", exp: expectAt("value_test.go:10"),
			plan:     map[string]recordedRun{"mutant 1": recFailExpected, "mutant 2": recFailExpected, "mutant 3": recFailExpected, "after 1": recTimeout},
			wantRuns: full, want: inconclusive, words: []string{"after-run", "timeout"}},
		{name: "killed by a signal", exp: expectAt("value_test.go:60"),
			plan:     map[string]recordedRun{"mutant 1": recKilled},
			wantRuns: []string{"baseline 1", "baseline 2", "baseline 3", "mutant 1", "after 1"},
			want:     inconclusive, words: []string{"killed"}},
		{name: "mutant runs disagree", exp: expectAt("value_test.go:10"),
			plan:     map[string]recordedRun{"mutant 1": recFailExpected, "mutant 3": recFailExpected},
			wantRuns: full, want: inconclusive, words: []string{"disagree"}},
		{name: "a different assertion fails", exp: expectAt("value_test.go:11"), plan: all(recFailExpected, mutants3...),
			wantRuns: full, want: inconclusive, words: []string{"value_test.go:10"}},
		// mutation-check › "Detection", "A failure that names no expected location": non-zero exits
		// with no expected line and no timeout panic.
		{name: "a failure that names no expected location", exp: expectAt("value_test.go:99"),
			plan: all(recPassNonzero, mutants3...), wantRuns: withReach, want: inconclusive},
		{name: "a failure at no expected location", exp: expectAt("value_test.go:99"),
			plan: all(recFailExpected, mutants3...), wantRuns: full, want: inconclusive, words: []string{"no expected line"}},
		{name: "a race report that is not expected", exp: expectAt("value_test.go:43"), plan: all(recRace, mutants3...),
			wantRuns: full, want: inconclusive, words: []string{"race"}},
		{name: "a race report that is expected", exp: expectation{texts: []string{raceText}}, plan: all(recRace, mutants3...),
			wantRuns: full, want: detection},
		{name: "a panic after the expected failure", exp: expectAt("value_test.go:23"), plan: all(recPanicAfter, mutants3...),
			wantRuns: full, want: detection},
		{name: "the wrong change does not build", exp: expectAt("value_test.go:10"), plan: all(recBuildFailed, mutants3...),
			wantRuns: full, want: invalid, words: []string{"does not build"}},
		// Invalid needs every baseline run and the after-run to pass: a build that breaks for every
		// run after the baselines says nothing about the wrong change.
		{name: "the build breaks after the baselines", exp: expectAt("value_test.go:10"),
			plan:     all(recBuildFailed, append(slices.Clone(mutants3), "after 1")...),
			wantRuns: full, want: inconclusive, words: []string{"after-run", "does not build"}},
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
			p := &planned{t: t, exp: tc.exp, plan: tc.plan, elapsed: tc.elapsed}
			c, err := sequence(t.Context(), 3, p.run)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(p.asked, tc.wantRuns) {
				t.Errorf("runs made: got %q, want %q", p.asked, tc.wantRuns)
			}
			c.seed, c.treeBefore, c.treeAfter = 7, "before", "before"
			c.timeout = 2 * time.Minute
			if tc.timeout != 0 {
				c.timeout = tc.timeout
			}
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
