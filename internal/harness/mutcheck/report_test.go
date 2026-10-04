package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// detectingStandIn prints the recorded failure for a mutant run (its GOFLAGS carries the overlay)
// and the recorded pass otherwise.
func detectingStandIn(t *testing.T) string {
	return "case \" $GOFLAGS \" in *\" -overlay=\"*) cat '" + eventsPath(t, "fail-expected") + "'; exit 1 ;; esac\n" +
		"cat '" + eventsPath(t, "pass") + "'\n"
}

func fileSHA(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func treeState(t *testing.T, root string) string {
	t.Helper()
	out, err := exec.CommandContext(t.Context(), filepath.Join(root, "scripts", "tree-state.sh")).Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

// TestReportDetection (mutation-check › "Report and exit status", "A detection passes" and "Logs
// are named as local"): a detection exits zero, its last line names the verdict, and the report
// holds every field the requirement lists.
func TestReportDetection(t *testing.T) {
	standIn(t, detectingStandIn(t))
	root := plantModule(t, standInModule)
	mutant := outside(t, "p.go", valueMutant)
	targetSHA := fileSHA(t, filepath.Join(root, "p", "p.go"))
	args := []string{"-pkg", "./p", "-test", "TestValue", "-file", "p/p.go", "-mutant", mutant,
		"-expect", "value_test.go:10", "-expect-text", "want 1", "-runs", "1"}
	got := runIn(t, root, testEnv(t, "GOFLAGS=-tags=planted"), args...)
	if got.code != 0 || !strings.HasPrefix(got.lastLine(), "verdict: detection") {
		t.Fatalf("got exit %d, last line %q; want exit 0 and a last line naming detection\n%s", got.code, got.lastLine(), got)
	}
	report := strings.Split(got.stdout, "\n")
	requireLines(t, "report", report,
		"command: mutcheck -pkg ./p -test TestValue -file p/p.go -mutant "+mutant+" -expect value_test.go:10 -expect-text 'want 1' -runs 1",
		"commit: "+gitIn(t, root, "rev-parse", "HEAD"),
		"tree: "+treeState(t, root)+" before the first run",
		"uncommitted changes: no",
		"go: go1.",
		"package: ./p",
		"test: TestValue",
		"target: p/p.go",
		"target sha256: "+targetSHA+" before, "+targetSHA+" after",
		"@@ -10 +10 @@", "-\treturn 1", "+\treturn 2",
		"hunks: 1",
		"target line 10",
		"expected locations: value_test.go:10",
		"expected texts: want 1",
		"seed: 1",
		"bound: 4m0s, twice the timeout of 2m0s: a run not ended by then is stopped with its process group",
	)
	baseline := section(got.stdout, "run baseline 1:")
	requireLines(t, "baseline run", baseline,
		"run baseline 1: pass",
		"command: go test -json -count=1 -cpu 1 -race -timeout 2m0s -run '^TestValue$' ./p",
		"GOFLAGS: -tags=planted",
		"exit status: 0",
		"wall time: ",
		"expected lines: none",
		"failure locations: none")
	requireLines(t, "mutant run", section(got.stdout, "run mutant 1:"),
		"run mutant 1: detection",
		"command: go test -json -count=1 -cpu 1 -race -timeout 2m0s -run '^TestValue$' ./p",
		"GOFLAGS: -tags=planted -overlay=",
		"exit status: 1",
		"wall time: ",
		"expected line:     value_test.go:10: Value() = 2, want 1",
		"failure locations: value_test.go:10")
	requireLines(t, "after-run", section(got.stdout, "run after 1:"), "run after 1: pass", "GOFLAGS: -tags=planted")
	requireLogs(t, got.stdout, root)
}

// TestReportSurvivor (mutation-check › "A survivor fails", "Two hunks" in the report): a survivor
// exits non-zero, its last line names the verdict, its reason and the seed, and the report lists
// both hunks of the wrong change, says there is more than one, gives the reach run's blocks, and
// prints each run's GOFLAGS, the overlay in the mutant run's only.
func TestReportSurvivor(t *testing.T) {
	profile := outside(t, "cover.out", "mode: atomic\nplanted.invalid/m/p/p.go:8.18,11.2 2 1\n")
	standIn(t, "for a in \"$@\"; do case \"$a\" in -coverprofile=*) cp '"+profile+"' \"${a#-coverprofile=}\" ;; esac; done\n"+
		passingStandIn(t))
	root := plantModule(t, standInModule)
	twoHunks := strings.Replace(strings.Replace(standInModule["p/p.go"], "import \"strconv\"\n", "", 1), "\tlabel = strconv.Itoa(1)\n", "", 1)
	got := runIn(t, root, testEnv(t, "GOFLAGS=-tags=planted"), "-pkg", "./p", "-test", "TestValue", "-file", "p/p.go",
		"-mutant", outside(t, "p.go", twoHunks), "-expect", "value_test.go:10", "-seed", "9", "-runs", "1")
	last := got.lastLine()
	if got.code == 0 || !strings.HasPrefix(last, "verdict: survivor: ") || !strings.Contains(last, "seed 9") {
		t.Fatalf("got exit %d, last line %q; want a non-zero exit and a last line naming survivor, its reason and seed 9\n%s", got.code, last, got)
	}
	report := strings.Split(got.stdout, "\n")
	requireLines(t, "report", report, "hunks: 2 (more than one hunk)", "@@ -3 +2,0 @@", "@@ -9 +7,0 @@", "seed: 9")
	requireLines(t, "mutant run", section(got.stdout, "run mutant 1:"), "GOFLAGS: -tags=planted -overlay=")
	reachRun := section(got.stdout, "run reach 1:")
	requireLines(t, "reach run", reachRun, "run reach 1: pass", "-coverpkg=./p", "GOFLAGS: -tags=planted",
		"reach: reached", "target line 9: reached", "8.18,11.2", "target line 3: not measurable")
	for _, head := range []string{"run baseline 1:", "run after 1:", "run reach 1:"} {
		if s := strings.Join(section(got.stdout, head), "\n"); strings.Contains(s, "-overlay") {
			t.Errorf("%s carries the overlay:\n%s", head, s)
		}
	}
	requireLogs(t, got.stdout, root)
}

// requireLogs fails unless the report names an existing log directory outside the module and
// says it is on this machine only.
func requireLogs(t *testing.T, report, root string) {
	t.Helper()
	logs := section(report, "logs: ")
	if len(logs) == 0 || !strings.HasSuffix(logs[0], " (on this machine only)") {
		t.Fatalf("no logs line saying it is on this machine only:\n%s", report)
	}
	dir := strings.TrimSuffix(strings.TrimPrefix(logs[0], "logs: "), " (on this machine only)")
	if _, err := os.Stat(filepath.Join(dir, "baseline-1.jsonl")); err != nil {
		t.Errorf("the log directory %s lacks the first run's log: %v", dir, err)
	}
	if rel, err := filepath.Rel(root, dir); err == nil && !strings.HasPrefix(rel, "..") {
		t.Errorf("the log directory %s is inside the module %s", dir, root)
	}
}

// TestReportRecordsAPanic (mutation-check › "A panic after the expected failure"): each mutant run
// prints an expected line and then panics; the verdict is detection, and the report records the
// panic.
func TestReportRecordsAPanic(t *testing.T) {
	standIn(t, "case \" $GOFLAGS \" in *\" -overlay=\"*) cat '"+eventsPath(t, "panic-after")+"'; exit 1 ;; esac\n"+
		"cat '"+eventsPath(t, "panic-after-pass")+"'\n")
	root := plantModule(t, standInModule)
	got := runIn(t, root, testEnv(t, "GOFLAGS="), "-pkg", "./p", "-test", "TestPanicAfter", "-file", "p/p.go",
		"-mutant", outside(t, "p.go", valueMutant), "-expect", "value_test.go:23", "-runs", "1")
	if got.code != 0 || !strings.HasPrefix(got.lastLine(), "verdict: detection") {
		t.Fatalf("got exit %d, last line %q; want exit 0 and a detection\n%s", got.code, got.lastLine(), got)
	}
	requireLines(t, "mutant run", section(got.stdout, "run mutant 1:"),
		"expected line:     value_test.go:23: Value() = 2, want 1",
		"note: panic: assignment to entry in nil map")
}

// TestHunksAreNotMerged (mutation-check › "Hunks are not merged"): the caller's git configuration
// sets diff.interHunkContext to 5, and the wrong change replaces lines 7 and 10 of the target,
// three lines apart. The report lists two hunks, and lines 8 and 9, which are unchanged, are in
// neither region.
func TestHunksAreNotMerged(t *testing.T) {
	standIn(t, detectingStandIn(t))
	root := plantModule(t, standInModule)
	config := outside(t, "gitconfig", "[diff]\n\tinterHunkContext = 5\n")
	src := strings.Replace(standInModule["p/p.go"], "// Value returns one.", "// Value returns two.", 1)
	mutant := outside(t, "p.go", strings.Replace(src, "return 1", "return 2", 1))
	got := runIn(t, root, testEnv(t, "GOFLAGS=", "GIT_CONFIG_GLOBAL="+config), "-pkg", "./p", "-test", "TestValue",
		"-file", "p/p.go", "-mutant", mutant, "-expect", "value_test.go:10", "-runs", "1")
	if got.code != 0 || !strings.HasPrefix(got.lastLine(), "verdict: detection") {
		t.Fatalf("got exit %d, last line %q; want exit 0 and a detection\n%s", got.code, got.lastLine(), got)
	}
	report := strings.Split(got.stdout, "\n")
	requireLines(t, "report", report, "hunks: 2 (more than one hunk)", "@@ -7 +7 @@: target line 7", "@@ -10 +10 @@: target line 10")
	for _, line := range report {
		if strings.Contains(line, "target lines 7-") || strings.Contains(line, "@@ -7,") {
			t.Errorf("a region holds the unchanged lines 8 and 9: %q", line)
		}
	}
}
