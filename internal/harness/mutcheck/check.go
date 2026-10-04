package main

import (
	"context"
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

// sequence makes the check's runs in the order the spec gives, with its early stops.
func sequence(ctx context.Context, runs int, do runFunc) (checkResult, error) {
	return checkResult{}, nil
}

// decide gives the check's verdict and its reason (mutation-check › "Outcome classification").
func decide(c checkResult) (outcome, string) {
	return "", ""
}
