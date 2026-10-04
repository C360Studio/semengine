package main

import (
	"time"
)

// outcome is the reading of one run, or the verdict of a whole check. pass is a reading only: a
// run of the unchanged code that passed, or a mutant run in which the named test passed.
type outcome string

const (
	pass         outcome = "pass"
	detection    outcome = "detection"
	survivor     outcome = "survivor"
	invalid      outcome = "invalid"
	inconclusive outcome = "inconclusive"
)

// reading is one run's outcome and why.
type reading struct {
	outcome outcome
	reason  string
}

// expectation is the intended assertion as the implementer named it: locations as Go prints them
// at the start of an output line (file.go:N), and fixed texts an output line contains.
type expectation struct {
	locations []string
	texts     []string
}

// facts are the per-run inputs a reading is a function of (design D5, D11 item 1).
type facts struct {
	buildFailed bool   // the build or the package's setup failed
	selected    bool   // a run event for the named test
	timedOut    bool   // go test's whole-run timeout panic
	signal      string // the signal that ended the run's process; "" when none did
	bounded     bool   // stopped by the program at twice the timeout
	result      string // the named test's pass, fail or skip event; "" when it reported none
	exitZero    bool   // go test exited zero
	expectedHit bool   // an expected line among the named test's output
	race        bool   // a race report among the named test's output
}

// observed is one run's facts with the lines the report prints.
type observed struct {
	facts
	expectedLines []string // every expected line, in full
	locations     []string // each file.go:N that starts an output line of the named test, once
	notes         []string // panics and fatal errors, wherever they were printed
	buildLines    []string // the build's output
}

// processEnd is how the go command of a run ended.
type processEnd struct {
	code    int    // exit status; -1 when it did not exit by itself
	signal  string // the signal that ended the go command itself; "" when none did
	bounded bool   // the program stopped it at the bound
}

// observe reads one run's go test -json stream for the named test and the lines it prints.
func observe(stream []byte, end processEnd, test string, exp expectation) observed {
	return observed{}
}

// read gives one run's reading by the rules of design D5. A run of the unchanged code (baseline,
// after or reach) reads pass or inconclusive; a mutant run can also read detection or invalid.
func read(f facts, mutant bool, bound time.Duration) reading {
	return reading{}
}
