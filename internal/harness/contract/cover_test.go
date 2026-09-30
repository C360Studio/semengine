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

const harness = "github.com/c360studio/semengine/internal/harness/"

// TestCoverCheckSensitivity: scripts/cover-check.sh (task cover:check) fails when any of the
// three enforced packages is below 80% or missing from its profile, and passes at 80%. Blocks
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
	unitOK := profile(harness+"lifecycletest", eighty...) + strings.TrimPrefix(profile(harness+"probe", eighty...), "mode: atomic\n")
	fixtureOK := profile(harness+"natsfixture", eighty...)

	if out, err := run(t, unitOK, fixtureOK); err != nil {
		t.Fatalf("80%% on every package refused: %v\n%s", err, out)
	}
	for _, tc := range []struct {
		name, unit, integration, want string
	}{
		{"natsfixture below", unitOK, profile(harness+"natsfixture", [2]int{7, 1}, [2]int{3, 0}), "natsfixture 70.0%"},
		{"probe below", profile(harness+"lifecycletest", eighty...) + strings.TrimPrefix(profile(harness+"probe", [2]int{1, 1}, [2]int{1, 0}), "mode: atomic\n"), fixtureOK, "probe 50.0%"},
		{"lifecycletest missing", strings.Replace(unitOK, "lifecycletest", "other", -1), fixtureOK, "lifecycletest: no statements"},
		{"natsfixture only in the unit profile", unitOK + strings.TrimPrefix(fixtureOK, "mode: atomic\n"), profile(harness+"probe", eighty...), "natsfixture: no statements"},
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
