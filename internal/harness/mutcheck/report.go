package main

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

// report is everything the report block prints (mutation-check › "Report and exit status").
type report struct {
	args        []string
	commit      string
	dirty       bool
	in          inputs
	p           prepared
	targetAfter string // the target's SHA-256 after the last run
	mutantCopy  string
	diff        []byte
	hunks       []hunk
	bound       time.Duration
	c           checkResult
	regions     []regionResult // read from the reach run's profile
	logs        string
	verdict     outcome
	reason      string
}

var shellSafe = regexp.MustCompile(`^[A-Za-z0-9_./=:,+@%-]+$`)

// shellQuote quotes an argument that a shell would otherwise split or expand.
func shellQuote(args []string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = a
		if !shellSafe.MatchString(a) {
			quoted[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		}
	}
	return strings.Join(quoted, " ")
}

func orNone(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, ", ")
}

func writeReport(w io.Writer, r report) {
	pf := func(format string, a ...any) { fmt.Fprintf(w, format+"\n", a...) }
	dirty := "no"
	if r.dirty {
		dirty = "yes"
	}
	pf("mutation check report")
	pf("command: mutcheck %s", shellQuote(r.args))
	pf("commit: %s", r.commit)
	pf("tree: %s before the first run, %s after the last; uncommitted changes: %s", r.c.treeBefore, treeAfter(r.c), dirty)
	pf("go: %s", r.p.goversion)
	pf("package: %s", r.in.pkg)
	pf("test: %s (-run %s)", r.in.test, shellQuote([]string{runPattern(r.in.test)}))
	pf("target: %s", r.p.target)
	pf("target sha256: %s before, %s after", sha256Hex(r.p.targetSrc), r.targetAfter)
	pf("wrong change, from the target to its copy at %s:", r.mutantCopy)
	for _, line := range strings.Split(strings.TrimRight(string(r.diff), "\n"), "\n") {
		pf("  %s", line)
	}
	if len(r.hunks) > 1 {
		pf("hunks: %d (more than one hunk)", len(r.hunks))
	} else {
		pf("hunks: %d", len(r.hunks))
	}
	for _, h := range r.hunks {
		pf("  %s: %s", h.header, regionText(h))
	}
	pf("expected locations: %s", orNone(r.in.expect.locations))
	pf("expected texts: %s", orNone(r.in.expect.texts))
	pf("seed: %d (RAPID_SEED=%d and RAPID_NOFAILFILE=true in every run)", r.in.seed, r.in.seed)
	pf("bound: %s, twice the timeout of %s: a run not ended by then is stopped with its process group", r.bound, r.in.timeout)
	for _, run := range allRuns(r.c) {
		writeRun(pf, run)
		if run.kind == reachRun && run.reading.outcome == pass {
			writeReach(pf, r)
		}
	}
	pf("logs: %s (on this machine only)", r.logs)
	pf("verdict: %s: %s", r.verdict, r.reason)
}

func treeAfter(c checkResult) string {
	if c.treeErr != "" {
		return "(not taken: " + c.treeErr + ")"
	}
	return c.treeAfter
}

func allRuns(c checkResult) []runRecord {
	runs := append(append([]runRecord{}, c.baselines...), c.mutants...)
	for _, r := range []*runRecord{c.after, c.reach} {
		if r != nil {
			runs = append(runs, *r)
		}
	}
	return runs
}

func writeRun(pf func(string, ...any), r runRecord) {
	pf("run %s %d: %s: %s", kindName(r.kind), r.index, r.reading.outcome, r.reading.reason)
	pf("  command: go %s", shellQuote(r.args))
	goflags := r.goflags
	if goflags == "" {
		goflags = "(empty)"
	}
	pf("  GOFLAGS: %s", goflags)
	if r.end.signal != "" {
		pf("  exit status: ended by signal %s", r.end.signal)
	} else {
		pf("  exit status: %d", r.end.code)
	}
	pf("  wall time: %s", r.wall.Round(time.Millisecond))
	if len(r.obs.expectedLines) == 0 {
		pf("  expected lines: none")
	}
	for _, line := range r.obs.expectedLines {
		pf("  expected line: %s", line)
	}
	pf("  failure locations: %s", orNone(r.obs.locations))
	for _, note := range r.obs.notes {
		pf("  note: %s", note)
	}
	if r.obs.buildFailed {
		for _, line := range r.obs.buildLines {
			pf("  build: %s", line)
		}
	}
}

func writeReach(pf func(string, ...any), r report) {
	if r.c.reachErr != "" {
		pf("  reach: not judged: %s", r.c.reachErr)
		return
	}
	pf("  reach: %s", r.c.reachState)
	for _, region := range r.regions {
		line := fmt.Sprintf("    %s: %s", region.region, region.state)
		if region.note != "" {
			line += ": " + region.note
		}
		var blocks []string
		for _, b := range region.blocks {
			blocks = append(blocks, fmt.Sprintf("%d.%d,%d.%d count %d", b.startLine, b.startCol, b.endLine, b.endCol, b.count))
		}
		if len(blocks) > 0 {
			line += "; blocks " + strings.Join(blocks, ", ")
		}
		pf("%s", line)
	}
}

func kindName(k runKind) string {
	return [...]string{baselineRun: "baseline", mutantRun: "mutant", afterRun: "after", reachRun: "reach"}[k]
}
