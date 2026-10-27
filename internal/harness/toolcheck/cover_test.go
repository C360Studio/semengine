package toolcheck

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

// d11Targets are the coverage targets setup-04a-02 design D11 adds (task 5.2d), and those later
// changes add (simplify-04a-13 design D2: natsclient/storagereport; setup-04a-03-boot-config-manager
// design D5: config; setup-04a-03-boot-service-base design D7: service; port-composition: composition),
// each with the profile scripts/cover-check.sh measures it from.
var d11Targets = []struct{ dir, kind string }{
	{"processor/graph-ingest", "merged"},
	{"graph", "merged"},
	{"graph/kvcatalog", "merged"},
	{"graph/readiness", "unit"},
	{"graph/inference", "unit"},
	{"pkg/projection", "unit"},
	{"internal/graphmutation", "unit"},
	{"component", "merged"},
	{"storage/storeregistry", "unit"},
	{"pkg/lifecycle", "merged"},
	{"natsclient/storagereport", "merged"},
	{"config", "merged"},
	{"service", "merged"},
	{"composition", "unit"},
}

// TestCoverCheckSensitivity: scripts/cover-check.sh (task cover:check) fails when any enforced
// package is below 80% or missing from its profile, and passes at 80%: the three harness packages;
// message, payloadregistry and natsclient (setup-04a-01-floor task 5.4), which lie outside
// internal/harness; and the d11Targets list. natsclient and its merged targets are measured
// from the unit and integration profiles merged, its unit targets from the unit profile alone.
// Blocks repeated across a merged profile count once, covered if any copy was.
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
	// d11Unit renders D11's targets into a unit profile at 80%, but for skip (left out) and
	// below (at 70%).
	d11Unit := func(skip, below string) string {
		var b strings.Builder
		for _, tg := range d11Targets {
			switch tg.dir {
			case skip:
			case below:
				b.WriteString(blocks(profile(module+tg.dir, [2]int{7, 1}, [2]int{3, 0})))
			default:
				b.WriteString(blocks(profile(module+tg.dir, eighty...)))
			}
		}
		return b.String()
	}
	d11OK := d11Unit("", "")
	unitOK := harnessUnitOK + portedUnitOK + natsclientOK + d11OK
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
		out, err := run(t, harnessUnitOK+portedUnitOK+natsclientHalf(1)+d11OK, fixtureOK+natsclientHalf(2))
		if err != nil || !strings.Contains(out, "natsclient 80.0%") {
			t.Fatalf("natsclient not measured from the merged profiles: %v\n%s", err, out)
		}
	})
	for _, tc := range []struct {
		name, unit, integration, want string
	}{
		{"natsfixture below", unitOK, profile(harness+"natsfixture", [2]int{7, 1}, [2]int{3, 0}), "natsfixture 70.0%"},
		{"probe below", profile(harness+"lifecycletest", eighty...) + blocks(profile(harness+"probe", [2]int{1, 1}, [2]int{1, 0})) + portedUnitOK + natsclientOK + d11OK, fixtureOK, "probe 50.0%"},
		{"lifecycletest missing", strings.Replace(unitOK, "lifecycletest", "other", -1), fixtureOK, "lifecycletest: no statements"},
		{"natsfixture only in the unit profile", unitOK + blocks(fixtureOK), profile(harness+"probe", eighty...), "natsfixture: no statements"},
		// Packages outside internal/harness (task 5.4), written before the script read them.
		{"message below", harnessUnitOK + blocks(profile(module+"message", [2]int{7, 1}, [2]int{3, 0})) + blocks(profile(module+"payloadregistry", eighty...)) + natsclientOK + d11OK, fixtureOK, "message 70.0%"},
		{"payloadregistry missing", harnessUnitOK + blocks(profile(module+"message", eighty...)) + natsclientOK + d11OK, fixtureOK, "payloadregistry: no statements"},
		{"natsclient below in both profiles", harnessUnitOK + portedUnitOK + blocks(profile(module+"natsclient", [2]int{7, 1}, [2]int{3, 0})) + d11OK, fixtureOK, "natsclient 70.0%"},
		{"natsclient missing from both profiles", harnessUnitOK + portedUnitOK + d11OK, fixtureOK, "natsclient: no statements"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := run(t, tc.unit, tc.integration)
			if err == nil || !strings.Contains(out, tc.want) {
				t.Fatalf("err=%v, want a failure naming %q\n%s", err, tc.want, out)
			}
		})
	}
	// Design D11's targets (task 5.2d), written before the script read them: each fails below the
	// floor and when absent from both profiles, and is read from the profile its kind names: a merged
	// target found only in the integration profile is measured, a unit target there is not.
	others := harnessUnitOK + portedUnitOK + natsclientOK
	for _, tg := range d11Targets {
		t.Run(tg.dir+" below", func(t *testing.T) {
			out, err := run(t, others+d11Unit("", tg.dir), fixtureOK)
			if want := "cover: " + tg.dir + " 70.0%"; err == nil || !strings.Contains(out, want) {
				t.Fatalf("err=%v, want a failure naming %q\n%s", err, want, out)
			}
		})
		t.Run(tg.dir+" missing from both profiles", func(t *testing.T) {
			out, err := run(t, others+d11Unit(tg.dir, ""), fixtureOK)
			if want := "cover: " + tg.dir + ": no statements"; err == nil || !strings.Contains(out, want) {
				t.Fatalf("err=%v, want a failure naming %q\n%s", err, want, out)
			}
		})
		t.Run(tg.dir+" only in the integration profile", func(t *testing.T) {
			out, err := run(t, others+d11Unit(tg.dir, ""), fixtureOK+blocks(profile(module+tg.dir, eighty...)))
			switch tg.kind {
			case "merged":
				if want := "cover: " + tg.dir + " 80.0%"; err != nil || !strings.Contains(out, want) {
					t.Fatalf("merged target not measured from the integration profile: err=%v, want %q\n%s", err, want, out)
				}
			default:
				if want := "cover: " + tg.dir + ": no statements"; err == nil || !strings.Contains(out, want) {
					t.Fatalf("unit target measured from the integration profile: err=%v, want %q\n%s", err, want, out)
				}
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

// TestCoverCheckRefusesAnUnmeasuredTree (integration-test-runner › "A failed run's profile is
// refused"): with no arguments, scripts/cover-check.sh reads coverage/integration.coverprofile only
// when coverage/integration.tree, which task test:integration writes after a passing go test, holds
// the fingerprint of the tree as it is now. A copy of the script runs in a throwaway root, with a
// fake go that passes and a fake scripts/tree-state.sh that prints a fixed fingerprint.
func TestCoverCheckRefusesAnUnmeasuredTree(t *testing.T) {
	const refusal = "no passing integration run measured this tree"
	run := func(t *testing.T, fingerprint *string) (string, error) {
		t.Helper()
		root := copyScript(t, "cover-check.sh")
		if err := os.WriteFile(filepath.Join(root, "scripts", "tree-state.sh"), []byte("#!/bin/sh\necho 0123456789ab\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "coverage"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "coverage", "integration.coverprofile"), []byte("mode: atomic\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if fingerprint != nil {
			if err := os.WriteFile(filepath.Join(root, "coverage", "integration.tree"), []byte(*fingerprint+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		cmd := exec.Command("bash", filepath.Join(root, "scripts", "cover-check.sh"))
		cmd.Dir = root
		cmd.Env = fakeBin(t, map[string]string{"go": "#!/bin/sh\nexit 0\n"})
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	other, same := "ba9876543210", "0123456789ab"
	for _, tc := range []struct {
		name        string
		fingerprint *string
	}{
		{"no fingerprint: the last run failed or never ran", nil},
		{"another tree's fingerprint", &other},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := run(t, tc.fingerprint)
			if err == nil || !strings.Contains(out, refusal) {
				t.Fatalf("err=%v, want a refusal saying %q\n%s", err, refusal, out)
			}
		})
	}
	// The refusal is not unconditional: this tree's fingerprint gets past it. Coverage itself then
	// fails, since the fake go wrote no unit profile; only the refusal matters here.
	t.Run("this tree's fingerprint", func(t *testing.T) {
		if out, _ := run(t, &same); strings.Contains(out, refusal) {
			t.Fatalf("refused a profile measured on this tree:\n%s", out)
		}
	})
}

// TestCoverCheckUnitRunCoversUnitAndMergedTargets: in its no-argument mode scripts/cover-check.sh
// writes the unit profile with one go test run, over every target measured from the unit profile
// alone or merged (natsclient, and the d11Targets list), and not over an integration-only
// target (natsfixture), whose owner tests need Docker. A fake go records its arguments and fails,
// which ends the script there.
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
	want := []string{"./internal/harness/lifecycletest/", "./internal/harness/probe/", "./message/", "./payloadregistry/", "./natsclient/"}
	for _, tg := range d11Targets {
		want = append(want, "./"+tg.dir+"/")
	}
	for _, pkg := range want {
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
