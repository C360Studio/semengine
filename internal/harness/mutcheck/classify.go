package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
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

// matches reports whether one output line of the named test is an expected line: it begins, after
// its indentation, with an expected location followed by a colon, or contains an expected text.
func (e expectation) matches(line string) bool {
	trimmed := strings.TrimLeft(line, " \t")
	for _, loc := range e.locations {
		if strings.HasPrefix(trimmed, loc+":") {
			return true
		}
	}
	for _, text := range e.texts {
		if strings.Contains(line, text) {
			return true
		}
	}
	return false
}

// event is the part of a go test -json event the reading uses.
type event struct {
	Action      string
	Test        string
	Output      string
	FailedBuild string
}

const (
	timeoutPanic = "panic: test timed out after"
	raceReport   = "race detected during execution of test"
)

var (
	// locationLine is a line as testing prints it for t.Error, t.Fatal and t.Log: file.go:N: text.
	// A stack frame starts with a path or a tab and is not one.
	locationLine = regexp.MustCompile(`^([^\s:/]+\.go:[0-9]+):(\s|$)`)
	// signalLine is how go test reports a test process ended by a signal (R08: "signal: killed").
	signalLine = regexp.MustCompile(`^signal: (.+)$`)
)

// observe reads one run's go test -json stream for the named test and the lines it prints. A
// line of the named test is an output line whose test is the named test or one of its subtests.
func observe(stream []byte, end processEnd, test string, exp expectation) observed {
	o := observed{facts: facts{signal: end.signal, bounded: end.bounded, exitZero: end.code == 0}}
	ours := func(name string) bool { return name == test || strings.HasPrefix(name, test+"/") }
	scanner := bufio.NewScanner(bytes.NewReader(stream))
	scanner.Buffer(make([]byte, 0, 64*1024), 64*1024*1024)
	for scanner.Scan() {
		var e event
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			// A line go test did not encode is output of no test.
			e = event{Action: "output", Output: scanner.Text()}
		}
		switch e.Action {
		case "build-fail":
			o.buildFailed = true
		case "build-output":
			o.buildLines = append(o.buildLines, strings.TrimRight(e.Output, "\n"))
		case "run":
			o.selected = o.selected || e.Test == test
		case "pass", "fail", "skip":
			if e.Test == test {
				o.result = e.Action
			}
			o.buildFailed = o.buildFailed || (e.Test == "" && e.FailedBuild != "")
		case "output":
			o.readLine(strings.TrimRight(e.Output, "\n"), e.Test, ours(e.Test), exp)
		}
	}
	return o
}

func (o *observed) readLine(line, test string, ours bool, exp expectation) {
	trimmed := strings.TrimLeft(line, " \t")
	if strings.HasPrefix(trimmed, "panic: ") || strings.HasPrefix(trimmed, "fatal error: ") {
		o.notes = append(o.notes, trimmed)
	}
	o.timedOut = o.timedOut || strings.HasPrefix(trimmed, timeoutPanic)
	if m := signalLine.FindStringSubmatch(trimmed); m != nil && o.signal == "" {
		o.signal = m[1]
	}
	if test == "" && (strings.Contains(line, "[build failed]") || strings.Contains(line, "[setup failed]")) {
		o.buildFailed = true
	}
	if !ours {
		return
	}
	if m := locationLine.FindStringSubmatch(trimmed); m != nil && !slices.Contains(o.locations, m[1]) {
		o.locations = append(o.locations, m[1])
	}
	o.race = o.race || strings.Contains(line, raceReport)
	if exp.matches(line) {
		o.expectedHit = true
		o.expectedLines = append(o.expectedLines, line)
	}
}

// read gives one run's reading by the rules of design D5. A run of the unchanged code (baseline,
// after or reach) reads pass or inconclusive; a mutant run can also read detection or invalid.
// A run ended by the timeout, a signal or the bound is inconclusive whatever it printed first.
func read(f facts, mutant bool, bound time.Duration) reading {
	switch {
	case f.bounded:
		return reading{inconclusive, fmt.Sprintf("the run had not ended by the bound of %s, twice its timeout, and was stopped with every process it started", bound)}
	case f.buildFailed && mutant:
		return reading{invalid, "the wrong change does not build"}
	case f.buildFailed:
		return reading{inconclusive, "the unchanged code does not build"}
	case !f.selected:
		return reading{inconclusive, "the name selected no test; a test in a file built only with the integration tag is not run by this command"}
	case f.timedOut:
		return reading{inconclusive, "go test's whole-run timeout ended the run"}
	case f.signal != "":
		return reading{inconclusive, "the test process was ended by a signal: " + f.signal}
	case f.result == "pass" && f.exitZero:
		return reading{pass, "the named test passed and go test exited zero"}
	case f.result == "pass":
		return reading{inconclusive, "the named test passed but go test exited non-zero"}
	case f.result == "fail" && mutant && f.expectedHit:
		return reading{detection, "the named test failed with an expected line"}
	case f.result == "fail" && mutant && f.race:
		return reading{inconclusive, "the named test failed with a race report and no expected line"}
	case f.result == "fail" && mutant:
		return reading{inconclusive, "the named test failed with no expected line"}
	case f.result == "fail":
		return reading{inconclusive, "the named test failed on the unchanged code"}
	case f.result == "skip":
		return reading{inconclusive, "the named test was skipped"}
	default:
		return reading{inconclusive, "the named test reported no result"}
	}
}
