package contract

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// profile renders a Go cover profile with one block per (statements, count) pair for pkg.
func profile(pkg string, blocks ...[2]int) string {
	var b strings.Builder
	b.WriteString("mode: atomic\n")
	for i, blk := range blocks {
		b.WriteString(pkg + "/f.go:" + itoa(i+1) + ".1," + itoa(i+1) + ".9 " + itoa(blk[0]) + " " + itoa(blk[1]) + "\n")
	}
	return b.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	for ; n > 0; n /= 10 {
		d = append([]byte{byte('0' + n%10)}, d...)
	}
	return string(d)
}

const (
	module  = "github.com/c360studio/semengine/"
	harness = module + "internal/harness/"
)

// blocks drops the mode line of a rendered profile, so profiles can be concatenated.
func blocks(p string) string { return strings.TrimPrefix(p, "mode: atomic\n") }

// TestCoverCheckSensitivity: scripts/cover-check.sh (task cover:check) fails when any enforced
// package is below 80% or missing from its profile, and passes at 80%: the three harness packages,
// and message, payloadregistry and natsclient (setup-04a-01-floor task 5.4), which lie outside
// internal/harness. natsclient is measured from the unit and integration profiles merged. Blocks
// repeated across a merged profile count once, covered if any copy was.
func TestCoverCheckSensitivity(t *testing.T) {
	script := filepath.Join(repoRoot(t), "scripts", "cover-check.sh")
	run := func(t *testing.T, unit, integration string) (string, error) {
		t.Helper()
		dir := t.TempDir()
		u, i := filepath.Join(dir, "unit.out"), filepath.Join(dir, "integration.out")
		if err := os.WriteFile(u, []byte(unit), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(i, []byte(integration), 0o644); err != nil {
			t.Fatal(err)
		}
		out, err := exec.Command("bash", script, u, i).CombinedOutput()
		return string(out), err
	}
	eighty := [][2]int{{8, 1}, {2, 0}}
	harnessUnitOK := profile(harness+"lifecycletest", eighty...) + blocks(profile(harness+"probe", eighty...))
	portedUnitOK := blocks(profile(module+"message", eighty...)) + blocks(profile(module+"payloadregistry", eighty...))
	natsclientOK := blocks(profile(module+"natsclient", eighty...))
	unitOK := harnessUnitOK + portedUnitOK + natsclientOK
	fixtureOK := profile(harness+"natsfixture", eighty...)

	if out, err := run(t, unitOK, fixtureOK); err != nil {
		t.Fatalf("80%% on every package refused: %v\n%s", err, out)
	}
	// natsclient's unit and integration runs each cover half of what the floor needs; only their
	// union reaches 80%. Block 1 (4 statements) is covered by the unit run, block 2 (4) by the
	// integration run, block 3 (2) by neither.
	natsclientHalf := func(covered int) string {
		var b strings.Builder
		for i, stmts := range []int{4, 4, 2} {
			count := 0
			if i+1 == covered {
				count = 1
			}
			b.WriteString(module + "natsclient/f.go:" + itoa(i+1) + ".1," + itoa(i+1) + ".9 " + itoa(stmts) + " " + itoa(count) + "\n")
		}
		return b.String()
	}
	t.Run("natsclient merged from both profiles", func(t *testing.T) {
		out, err := run(t, harnessUnitOK+portedUnitOK+natsclientHalf(1), fixtureOK+natsclientHalf(2))
		if err != nil || !strings.Contains(out, "natsclient 80.0%") {
			t.Fatalf("natsclient not measured from the merged profiles: %v\n%s", err, out)
		}
	})
	for _, tc := range []struct {
		name, unit, integration, want string
	}{
		{"natsfixture below", unitOK, profile(harness+"natsfixture", [2]int{7, 1}, [2]int{3, 0}), "natsfixture 70.0%"},
		{"probe below", profile(harness+"lifecycletest", eighty...) + blocks(profile(harness+"probe", [2]int{1, 1}, [2]int{1, 0})) + portedUnitOK + natsclientOK, fixtureOK, "probe 50.0%"},
		{"lifecycletest missing", strings.Replace(unitOK, "lifecycletest", "other", -1), fixtureOK, "lifecycletest: no statements"},
		{"natsfixture only in the unit profile", unitOK + blocks(fixtureOK), profile(harness+"probe", eighty...), "natsfixture: no statements"},
		// Packages outside internal/harness (task 5.4), written before the script read them.
		{"message below", harnessUnitOK + blocks(profile(module+"message", [2]int{7, 1}, [2]int{3, 0})) + blocks(profile(module+"payloadregistry", eighty...)) + natsclientOK, fixtureOK, "message 70.0%"},
		{"payloadregistry missing", harnessUnitOK + blocks(profile(module+"message", eighty...)) + natsclientOK, fixtureOK, "payloadregistry: no statements"},
		{"natsclient below in both profiles", harnessUnitOK + portedUnitOK + blocks(profile(module+"natsclient", [2]int{7, 1}, [2]int{3, 0})), fixtureOK, "natsclient 70.0%"},
		{"natsclient missing from both profiles", harnessUnitOK + portedUnitOK, fixtureOK, "natsclient: no statements"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := run(t, tc.unit, tc.integration)
			if err == nil || !strings.Contains(out, tc.want) {
				t.Fatalf("err=%v, want a failure naming %q\n%s", err, tc.want, out)
			}
		})
	}

	// The same block in two package runs is one block: covered if either run covered it.
	merged := "mode: atomic\n" + harness + "natsfixture/f.go:1.1,1.9 8 0\n" + harness + "natsfixture/f.go:1.1,1.9 8 3\n" +
		harness + "natsfixture/f.go:2.1,2.9 2 0\n"
	if out, err := run(t, unitOK, merged); err != nil || !strings.Contains(out, "natsfixture 80.0%") {
		t.Fatalf("duplicate blocks not merged: %v\n%s", err, out)
	}
}

// TestTreeStateSeesUntrackedContent: cover:check refuses a profile measured on another tree by
// comparing scripts/tree-state.sh fingerprints, so editing a new, untracked test file must change
// the fingerprint, not only adding or removing it.
func TestTreeStateSeesUntrackedContent(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.invalid"}, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(filepath.Join(repoRoot(t), "scripts", "tree-state.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scripts", "tree-state.sh"), src, 0o755); err != nil {
		t.Fatal(err)
	}
	git("init", "-q")
	git("add", ".")
	git("commit", "-q", "-m", "init")
	state := func() string {
		t.Helper()
		out, err := exec.Command("bash", filepath.Join(dir, "scripts", "tree-state.sh")).Output()
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(out))
	}
	untracked := filepath.Join(dir, "new_test.go")
	if err := os.WriteFile(untracked, []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := state()
	if err := os.WriteFile(untracked, []byte("package x\n\nfunc TestMore() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if after := state(); after == before {
		t.Fatalf("editing an untracked file left the fingerprint at %s", before)
	}
}

// TestCoverCheckUnitRunCoversUnitAndMergedTargets: in its no-argument mode scripts/cover-check.sh
// writes the unit profile with one go test run, over every target measured from the unit profile
// alone or merged (natsclient), and not over an integration-only target (natsfixture), whose owner
// tests need Docker. A fake go records its arguments and fails, which ends the script there.
func TestCoverCheckUnitRunCoversUnitAndMergedTargets(t *testing.T) {
	root := copyScript(t, "cover-check.sh")
	argsFile := filepath.Join(t.TempDir(), "args")
	env := fakeBin(t, map[string]string{"go": "#!/bin/sh\necho \"$@\" > \"$ARGS_FILE\"\nexit 1\n"}, "ARGS_FILE="+argsFile)
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "cover-check.sh"))
	cmd.Dir = root
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("cover-check.sh exited 0 after its go test failed:\n%s", out)
	}
	raw, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("the fake go was not run: %v", err)
	}
	args := strings.Fields(string(raw))
	has := func(pkg string) bool {
		for _, a := range args {
			if a == pkg {
				return true
			}
		}
		return false
	}
	for _, pkg := range []string{"./internal/harness/lifecycletest/", "./internal/harness/probe/", "./message/", "./payloadregistry/", "./natsclient/"} {
		if !has(pkg) {
			t.Errorf("the unit run does not cover %s: go %s", pkg, raw)
		}
	}
	if has("./internal/harness/natsfixture/") {
		t.Errorf("the unit run covers the integration-only natsfixture: go %s", raw)
	}
}

// TestCoverCheckPrintsFailingTest (merge-gate › "Coverage gate names a failing test"): in its
// no-argument mode scripts/cover-check.sh runs go test itself, and a test that fails there must
// be named in its output. A copy of the script runs in a throwaway root against a fake go that
// prints a --- FAIL line and exits 1.
func TestCoverCheckPrintsFailingTest(t *testing.T) {
	root := copyScript(t, "cover-check.sh")
	env := fakeBin(t, map[string]string{"go": "#!/bin/sh\necho '--- FAIL: TestPlanted (0.00s)'\necho FAIL\nexit 1\n"})
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "cover-check.sh"))
	cmd.Dir = root
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("cover-check.sh exited 0 after its go test failed:\n%s", out)
	}
	if !strings.Contains(string(out), "--- FAIL: TestPlanted") {
		t.Fatalf("cover-check.sh output does not name the failing test (err=%v):\n%s", err, out)
	}
}
