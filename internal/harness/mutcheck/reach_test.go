package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// testdata/reach holds one target (plant/target.go), the coverage profile of its unchanged code
// that the reach run would record (profile.txt, from
// `go test -json -count=1 -cpu 1 -race -run '^TestClamp$' -coverpkg=./plant -coverprofile=profile.txt ./plant`
// with go1.26.6), and one line diff per case, taken with
// `git diff --no-index --no-ext-diff --no-textconv --no-color -U0` from the target to a copy with
// the wrong change. TestClamp calls Clamp(3): Clamp's first block (lines 20-25) and its last
// statement (line 32) run; the branch of `if x > Limit` (lines 25-31) and unused (lines 14-17) do
// not. Every wrong change here compiles, and TestClamp passes on each of them.

func reachFile(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "reach", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

type wantRegion struct {
	hunk   hunk // the numbers only; the header is checked to start with "@@ "
	state  reachState
	blocks []string // the deciding blocks as start.col,end.col count
	words  []string // in the region's description or note
}

// TestReachRecordedCases: each scenario of "The changed region and reach", read from a recorded
// line diff against the recorded profile. For a scenario that states a verdict, the check is
// decided as if every baseline and mutant run had passed, so the verdict comes from reach alone.
func TestReachRecordedCases(t *testing.T) {
	target := reachFile(t, "plant/target.go")
	profile := reachFile(t, "profile.txt")
	for _, tc := range []struct {
		name    string
		diff    string
		regions []wantRegion
		want    reachState
		verdict outcome // "" when the scenario states none
		words   []string
	}{
		{"a deletion that was reached", "deletion-reached.diff", []wantRegion{
			{hunk{oldStart: 24, oldCount: 1, newStart: 23}, reached, []string{"20.23,25.15 1"}, []string{"line 24"}},
		}, reached, survivor, nil},
		{"a deletion in a branch that did not run", "deletion-not-reached.diff", []wantRegion{
			{hunk{oldStart: 29, oldCount: 1, newStart: 28}, notReached, []string{"25.15,31.3 0"}, []string{"line 29"}},
		}, notReached, invalid, []string{"did not reach the wrong change"}},
		// Hunk 2 deletes target lines 22-24, which ran. Its mutant-side numbers (+28,0) point into the
		// branch that did not run, and the mutant's lines 22-24 are unused's body, which did not run
		// either: read in the mutant's numbers, the change would not be reached.
		{"regions are read in the target's line numbers", "target-numbers.diff", []wantRegion{
			{hunk{oldStart: 12, newStart: 13, newCount: 7}, notMeasurable, nil, []string{"after target line 12", "outside every function body"}},
			{hunk{oldStart: 22, oldCount: 3, newStart: 28}, reached, []string{"20.23,25.15 1"}, []string{"lines 22-24"}},
		}, reached, "", nil},
		// The neighbouring-lines rule would count line 25, which ran, and say reached.
		{"an insertion in a branch that did not run", "insertion-not-reached.diff", []wantRegion{
			{hunk{oldStart: 25, newStart: 26, newCount: 1}, notReached, []string{"25.15,31.3 0"}, []string{"after target line 25"}},
		}, notReached, invalid, nil},
		// unused's first block begins on its func line, 14, right after the insertion; the
		// insertion is outside every function body, so that block says nothing.
		{"a method added before a function that did not run", "method-before-unrun.diff", []wantRegion{
			{hunk{oldStart: 13, newStart: 14, newCount: 2}, notMeasurable, nil, []string{"after target line 13", "outside every function body"}},
		}, notMeasurable, survivor, []string{"reach could not be measured"}},
		{"two hunks", "two-hunks.diff", []wantRegion{
			{hunk{oldStart: 3, oldCount: 1, newStart: 2}, notMeasurable, nil, []string{"line 3"}},
			{hunk{oldStart: 21, oldCount: 1, newStart: 19}, reached, []string{"20.23,25.15 1"}, []string{"line 21"}},
		}, reached, "", nil},
		{"a change to a declaration", "declaration.diff", []wantRegion{
			{hunk{oldStart: 6, oldCount: 1, newStart: 6, newCount: 1}, notMeasurable, nil, []string{"line 6"}},
		}, notMeasurable, survivor, []string{"reach could not be measured"}},
		// No block contains the position after line 31, the branch's closing brace; the statement
		// that would run right after the inserted line begins on line 32, and it ran.
		{"an insertion before a statement that ran", "insertion-before-next-statement.diff", []wantRegion{
			{hunk{oldStart: 31, newStart: 32, newCount: 1}, reached, []string{"32.2,32.10 1"}, []string{"after target line 31"}},
		}, reached, "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hunks, err := parseHunks(reachFile(t, tc.diff))
			if err != nil {
				t.Fatal(err)
			}
			if len(hunks) != len(tc.regions) {
				t.Fatalf("got %d hunks %+v, want %d", len(hunks), hunks, len(tc.regions))
			}
			state, regions, err := reach(hunks, target, profile, "target.go")
			if err != nil {
				t.Fatal(err)
			}
			if state != tc.want {
				t.Errorf("reach: got %q, want %q", state, tc.want)
			}
			if len(regions) != len(tc.regions) {
				t.Fatalf("got %d regions, want %d", len(regions), len(tc.regions))
			}
			for i, w := range tc.regions {
				got := regions[i]
				h := got.hunk
				if !strings.HasPrefix(h.header, "@@ ") || h.oldStart != w.hunk.oldStart || h.oldCount != w.hunk.oldCount ||
					h.newStart != w.hunk.newStart || h.newCount != w.hunk.newCount {
					t.Errorf("hunk %d: got %+v, want the numbers of %+v", i+1, h, w.hunk)
				}
				if got.state != w.state {
					t.Errorf("region %d (%s): got %q, want %q", i+1, got.region, got.state, w.state)
				}
				var blocks []string
				for _, b := range got.blocks {
					blocks = append(blocks, blockText(b))
				}
				if !slices.Equal(blocks, w.blocks) {
					t.Errorf("region %d: deciding blocks %q, want %q", i+1, blocks, w.blocks)
				}
				for _, word := range w.words {
					if !strings.Contains(got.region+" "+got.note, word) {
						t.Errorf("region %d: %q / %q lacks %q", i+1, got.region, got.note, word)
					}
				}
			}
			if tc.verdict == "" {
				return
			}
			passed := runAs(t, recPass, baselineRun, 1, expectAt("value_test.go:10"))
			c := checkResult{runs: 1, baselines: []runRecord{passed}, mutants: []runRecord{passed}, after: &passed,
				reach: &passed, reachState: state, treeBefore: "x", treeAfter: "x", seed: 1}
			c.mutants[0].kind = mutantRun
			verdict, reason := decide(c)
			if verdict != tc.verdict {
				t.Errorf("verdict: got %s (%s), want %s", verdict, reason, tc.verdict)
			}
			for _, w := range tc.words {
				if !strings.Contains(reason, w) {
					t.Errorf("reason %q lacks %q", reason, w)
				}
			}
		})
	}
}

func blockText(b block) string {
	return fmt.Sprintf("%d.%d,%d.%d %d", b.startLine, b.startCol, b.endLine, b.endCol, b.count)
}

// TestReachRegionsAreNotMeasurableWithoutAProfileBlock: a region no block of the profile overlaps
// or touches is not measurable, and so is a wrong change with no region at all; neither reads as
// not reached.
func TestReachRegionsAreNotMeasurableWithoutAProfileBlock(t *testing.T) {
	target := reachFile(t, "plant/target.go")
	empty := []byte("mode: atomic\n")
	hunks, err := parseHunks(reachFile(t, "deletion-not-reached.diff"))
	if err != nil {
		t.Fatal(err)
	}
	if state, _, err := reach(hunks, target, empty, "target.go"); err != nil || state != notMeasurable {
		t.Errorf("profile with no block: got %q, %v; want %q", state, err, notMeasurable)
	}
	if state, _, err := reach(nil, target, reachFile(t, "profile.txt"), "target.go"); err != nil || state != notMeasurable {
		t.Errorf("no hunk: got %q, %v; want %q", state, err, notMeasurable)
	}
}

// TestParseHunksRefusesWhatItCannotRead: a hunk header it cannot read is an error, not a hunk.
func TestParseHunksRefusesWhatItCannotRead(t *testing.T) {
	for _, diff := range []string{"@@ -x +1 @@\n", "@@ -1 @@\n", "@@ -1,2 +3,x @@\n"} {
		if hunks, err := parseHunks([]byte(diff)); err == nil {
			t.Errorf("%q: got hunks %+v and no error", diff, hunks)
		}
	}
}
