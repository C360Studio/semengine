package main

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// absSource is the planted target: Abs and nothing else.
const absSource = "package prop\n\n// Abs returns the absolute value of x.\nfunc Abs(x int) int {\n\tif x < 0 {\n\t\treturn -x\n\t}\n\treturn x\n}\n"

// absWrong is Abs wrong only at x = -7. Measured with go1.26.6 and Rapid v1.3.0, TestAbs finds it
// for seeds 1, 2, 4, 7 and 9 and misses it for seeds 3, 5, 6, 8 and 10 (calibrated over seeds 1
// to 10 on this module, with RAPID_NOFAILFILE=true, recorded on PR #82 with task 2.10).
var absWrong = strings.Replace(absSource, "func Abs(x int) int {\n", "func Abs(x int) int {\n\tif x == -7 {\n\t\treturn x\n\t}\n", 1)

// rapidModule is a planted module whose property test TestAbs draws x from -1000 to 1000 with Rapid
// and fails on line 13 when Abs(x) is not |x|. It requires Rapid at the version this repository's
// go.mod requires, with that version's go.sum lines, so its runs read Rapid from the module cache
// with GOPROXY=off.
func rapidModule(t *testing.T) map[string]string {
	t.Helper()
	root := repoRoot(t)
	mod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^\s*pgregory\.net/rapid (v\S+)`).FindSubmatch(mod)
	if m == nil {
		t.Fatal("this repository's go.mod does not require pgregory.net/rapid")
	}
	version := string(m[1])
	sums, err := os.ReadFile(filepath.Join(root, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	var sum []string
	for _, line := range strings.Split(string(sums), "\n") {
		if strings.HasPrefix(line, "pgregory.net/rapid "+version+" ") || strings.HasPrefix(line, "pgregory.net/rapid "+version+"/go.mod ") {
			sum = append(sum, line)
		}
	}
	if len(sum) != 2 {
		t.Fatalf("go.sum has %d lines for pgregory.net/rapid %s, want 2", len(sum), version)
	}
	return map[string]string{
		"go.mod":      "module planted.invalid/m\n\ngo 1.26\n\nrequire pgregory.net/rapid " + version + "\n",
		"go.sum":      strings.Join(sum, "\n") + "\n",
		"prop/abs.go": absSource,
		"prop/abs_test.go": "package prop\n\nimport (\n\t\"testing\"\n\n\t\"pgregory.net/rapid\"\n)\n\n" +
			"func TestAbs(t *testing.T) {\n" +
			"\trapid.Check(t, func(t *rapid.T) {\n" +
			"\t\tx := rapid.IntRange(-1000, 1000).Draw(t, \"x\")\n" +
			"\t\tif got := Abs(x); got < 0 || (got != x && got != -x) {\n" +
			"\t\t\tt.Fatalf(\"Abs(%d) = %d\", x, got)\n" + // line 13
			"\t\t}\n\t})\n}\n",
	}
}

// rapidEnv is what the program's environment adds for a planted Rapid module: no proxy, so Rapid
// comes from the module cache, and that cache named explicitly, because testEnv turns the host's
// Go environment file off. Rapid is downloaded into the cache first if it is not there, as any
// go test of this repository would.
func rapidEnv(t *testing.T) []string {
	t.Helper()
	download := exec.CommandContext(t.Context(), "go", "mod", "download", "pgregory.net/rapid")
	download.Dir = repoRoot(t)
	if out, err := download.CombinedOutput(); err != nil {
		t.Fatalf("go mod download pgregory.net/rapid: %v\n%s", err, out)
	}
	out, err := exec.CommandContext(t.Context(), "go", "env", "GOMODCACHE").Output()
	if err != nil {
		t.Fatal(err)
	}
	return []string{"GOFLAGS=", "GOPROXY=off", "GOMODCACHE=" + strings.TrimSpace(string(out))}
}

// requireNoFailureFiles fails if anything exists under the package's testdata/rapid directory.
func requireNoFailureFiles(t *testing.T, root string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, "prop", "testdata", "rapid")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("prop/testdata/rapid exists after the check (%v)", err)
	}
}

// TestRapidPropertyDetected (mutation-check › "A property test"): with seed 1 the property finds
// the wrong change. The verdict is detection, and no file is written under prop/testdata/rapid.
func TestRapidPropertyDetected(t *testing.T) {
	root := plantModule(t, rapidModule(t))
	got := runIn(t, root, testEnv(t, rapidEnv(t)...), "-pkg", "./prop", "-test", "TestAbs", "-file", "prop/abs.go",
		"-mutant", outside(t, "abs.go", absWrong), "-expect", "abs_test.go:13", "-seed", "1", "-runs", "1")
	if got.code != 0 || !strings.HasPrefix(got.lastLine(), "verdict: detection") {
		t.Fatalf("got exit %d, last line %q; want exit 0 and a detection\n%s", got.code, got.lastLine(), got)
	}
	requireNoFailureFiles(t, root)
}

// TestRapidPropertySeedMisses (mutation-check › "A property test whose seed misses the wrong
// change"): with seed 3 the property passes on the wrong change. The verdict is survivor, and its
// reason names seed 3 and says the result holds for it. Its reach run is the package's one real
// reach run: the profile go test writes, and the block it gives for the insertion, are read end to
// end.
func TestRapidPropertySeedMisses(t *testing.T) {
	root := plantModule(t, rapidModule(t))
	got := runIn(t, root, testEnv(t, rapidEnv(t)...), "-pkg", "./prop", "-test", "TestAbs", "-file", "prop/abs.go",
		"-mutant", outside(t, "abs.go", absWrong), "-expect", "abs_test.go:13", "-seed", "3", "-runs", "1")
	last := got.lastLine()
	if got.code == 0 || !strings.HasPrefix(last, "verdict: survivor") || !strings.Contains(last, "holds for seed 3") {
		t.Fatalf("got exit %d, last line %q; want a non-zero exit and a survivor that holds for seed 3\n%s", got.code, last, got)
	}
	requireLines(t, "reach run", section(got.stdout, "run reach 1:"), "-coverpkg=./prop", "reach: reached",
		"an insertion after target line 4: reached", "blocks 4.21,5.11")
	requireNoFailureFiles(t, root)
}
