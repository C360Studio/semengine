package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// runKind says which of the check's runs a run is (mutation-check › "The runs").
type runKind int

const (
	baselineRun runKind = iota
	mutantRun
	afterRun
	reachRun
)

// runRecord is one run as the report prints it.
type runRecord struct {
	kind    runKind
	index   int // 1-based within its kind
	args    []string
	goflags string
	wall    time.Duration
	end     processEnd
	obs     observed
	reading reading
}

// checkResult is everything the verdict is decided from.
type checkResult struct {
	runs       int // -runs: the number of baseline runs and of mutant runs asked for
	baselines  []runRecord
	mutants    []runRecord
	after      *runRecord // nil when no after-run was made
	reach      *runRecord // nil when no reach run was made
	reachState reachState // read from the reach run's profile when that run passed
	reachErr   string     // why the reach run's profile could not be read
	treeBefore string
	treeAfter  string
	treeErr    string // why the fingerprint after the last run could not be taken
	seed       uint64
}

// runFunc makes one run of the given kind; its error means the program was interrupted.
type runFunc func(ctx context.Context, kind runKind, index int) (runRecord, error)

// sequence makes the check's runs in the spec's order: the baseline runs, the mutant runs, the
// after-run and, only when the named test passed in every mutant run, the reach run. A baseline
// run that does not pass ends the check; a mutant run ended by the timeout, a signal or the bound
// is the last mutant run.
func sequence(ctx context.Context, runs int, do runFunc) (checkResult, error) {
	c := checkResult{runs: runs}
	for i := 1; i <= runs; i++ {
		r, err := do(ctx, baselineRun, i)
		if err != nil {
			return c, err
		}
		c.baselines = append(c.baselines, r)
		if r.reading.outcome != pass {
			return c, nil
		}
	}
	allPassed := true
	for i := 1; i <= runs; i++ {
		r, err := do(ctx, mutantRun, i)
		if err != nil {
			return c, err
		}
		c.mutants = append(c.mutants, r)
		allPassed = allPassed && r.obs.result == "pass"
		if r.obs.timedOut || r.obs.signal != "" || r.obs.bounded {
			break
		}
	}
	after, err := do(ctx, afterRun, 1)
	if err != nil {
		return c, err
	}
	c.after = &after
	if allPassed {
		r, err := do(ctx, reachRun, 1)
		if err != nil {
			return c, err
		}
		c.reach = &r
	}
	return c, nil
}

// decide gives the check's verdict and its reason (mutation-check › "Outcome classification").
// A tree that changed makes the check inconclusive whatever its runs showed.
func decide(c checkResult) (outcome, string) {
	verdict, reason := decideRuns(c)
	switch {
	case c.treeErr != "":
		return inconclusive, "the tree's fingerprint could not be taken after the last run: " + c.treeErr + also(verdict, reason)
	case c.treeAfter != c.treeBefore:
		return inconclusive, fmt.Sprintf("the tree changed during the check: its fingerprint was %s before the first run and %s after the last", c.treeBefore, c.treeAfter) + also(verdict, reason)
	}
	return verdict, reason
}

func also(verdict outcome, reason string) string {
	if verdict == inconclusive {
		return "; also, " + reason
	}
	return ""
}

func decideRuns(c checkResult) (outcome, string) {
	for _, r := range c.baselines {
		if r.reading.outcome != pass {
			return inconclusive, fmt.Sprintf("baseline run %d of %d did not pass: %s; no mutant run was made", r.index, c.runs, r.reading.reason)
		}
	}
	if len(c.mutants) == 0 {
		return inconclusive, "no mutant run was made"
	}
	for _, r := range c.mutants {
		if r.reading.outcome == inconclusive {
			return inconclusive, fmt.Sprintf("mutant run %d: %s%s", r.index, r.reading.reason, failedAt(r))
		}
	}
	first := c.mutants[0].reading.outcome
	for _, r := range c.mutants[1:] {
		if r.reading.outcome != first {
			var each []string
			for _, m := range c.mutants {
				each = append(each, fmt.Sprintf("run %d %s", m.index, m.reading.outcome))
			}
			return inconclusive, "the mutant runs disagree: " + strings.Join(each, ", ")
		}
	}
	if first == invalid {
		return invalid, "the wrong change does not build"
	}
	if c.after == nil || c.after.reading.outcome != pass {
		why := "it was not made"
		if c.after != nil {
			why = c.after.reading.reason
		}
		return inconclusive, "the after-run on the unchanged code did not pass: " + why
	}
	if first == detection {
		return detection, fmt.Sprintf("each of the %d mutant runs failed the named test with an expected line, and the after-run passed", len(c.mutants))
	}
	switch {
	case c.reach == nil:
		return inconclusive, "no reach run was made"
	case c.reach.reading.outcome != pass:
		return inconclusive, "the reach run on the unchanged code did not pass: " + c.reach.reading.reason
	case c.reachErr != "":
		return inconclusive, "the reach run's coverage profile could not be read: " + c.reachErr
	}
	switch c.reachState {
	case reached:
		return survivor, survivorReason("the reach run shows the test reached the wrong change", c.seed)
	case notMeasurable:
		return survivor, survivorReason("reach could not be measured", c.seed)
	case notReached:
		return invalid, "the test did not reach the wrong change: no executed block of the reach run touches a changed region"
	}
	return inconclusive, fmt.Sprintf("reach was not judged (%q)", c.reachState)
}

func survivorReason(why string, seed uint64) string {
	return fmt.Sprintf("the named test passed in every mutant run and %s; for a test that generates its inputs, the result holds for seed %d", why, seed)
}

// failedAt names the locations a failed run printed, for a reason that would otherwise not say
// where it failed.
func failedAt(r runRecord) string {
	if r.obs.result != "fail" || len(r.obs.locations) == 0 {
		return ""
	}
	return " (it printed " + strings.Join(r.obs.locations, ", ") + ")"
}
