package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The end-to-end cases run the real toolchain on planted modules, as pindiff's
// TestCommandExitStatus does, with -runs 1. Planted sources split the literals the repository's
// text guards match ("//go:" + "build").

// childBuildModule's test builds the command tool with go build and runs it. Its test file is
// built only with the planted tag, so a run whose GOFLAGS lacks -tags=planted selects no test; it
// fails on line 15 unless RAPID_SEED is 7 and RAPID_NOFAILFILE is true, and on line 26, the
// expected line, when the tool prints anything but 42.
var childBuildModule = map[string]string{
	"go.mod":         "module planted.invalid/m\n\ngo 1.26\n",
	"tool/main.go":   "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(answer()) }\n",
	"tool/answer.go": "package main\n\nfunc answer() int { return 42 }\n",
	"check/check_test.go": "//go:" + "build planted\n\npackage check\n\n" +
		"import (\n\t\"os\"\n\t\"os/exec\"\n\t\"path/filepath\"\n\t\"strings\"\n\t\"testing\"\n)\n\n" +
		"func TestTool(t *testing.T) {\n" +
		"\tif os.Getenv(\"RAPID_SEED\") != \"7\" || os.Getenv(\"RAPID_NOFAILFILE\") != \"true\" {\n" +
		"\t\tt.Fatalf(\"RAPID_SEED=%q RAPID_NOFAILFILE=%q\", os.Getenv(\"RAPID_SEED\"), os.Getenv(\"RAPID_NOFAILFILE\"))\n" + // line 15
		"\t}\n" +
		"\tbin := filepath.Join(t.TempDir(), \"tool\")\n" +
		"\tif out, err := exec.Command(\"go\", \"build\", \"-o\", bin, \"planted.invalid/m/tool\").CombinedOutput(); err != nil {\n" +
		"\t\tt.Fatalf(\"go build: %v\\n%s\", err, out)\n" +
		"\t}\n" +
		"\tout, err := exec.Command(bin).Output()\n" +
		"\tif err != nil {\n" +
		"\t\tt.Fatal(err)\n" +
		"\t}\n" +
		"\tif got := strings.TrimSpace(string(out)); got != \"42\" {\n" +
		"\t\tt.Fatalf(\"the tool printed %q, want 42\", got)\n" + // line 26
		"\t}\n}\n",
}

// TestDetectionThroughAChildBuild (mutation-check › "A read-only tree", "A build the test starts",
// "Flags set with go env -w", "The environment is the same in every run"): the wrong change is in
// a command the test builds with go build. The module's every directory is read-only; the caller's
// -tags=planted is set with go env -w in a Go environment file and absent from the process
// environment; the seed is 7. The check is a detection, so every run selected the test (the tag),
// passed its environment check (the seed) and, in the mutant run, the child build saw the wrong
// change (the overlay, through GOFLAGS). Every file's content hash is the same afterwards.
func TestDetectionThroughAChildBuild(t *testing.T) {
	root := plantModule(t, childBuildModule)
	goenv := filepath.Join(t.TempDir(), "go.env")
	goEnvWrite(t, goenv, "GOFLAGS=-tags=planted")
	mutant := outside(t, "answer.go", strings.Replace(childBuildModule["tool/answer.go"], "42", "41", 1))
	before := snapshot(t, root)
	readOnly(t, root)

	got := runIn(t, root, testEnv(t, "GOENV="+goenv), "-pkg", "./check", "-test", "TestTool",
		"-file", "tool/answer.go", "-mutant", mutant, "-expect", "check_test.go:26", "-seed", "7", "-runs", "1")
	if got.code != 0 || !strings.HasPrefix(got.lastLine(), "verdict: detection") {
		t.Fatalf("got exit %d, last line %q; want exit 0 and a detection\n%s", got.code, got.lastLine(), got)
	}
	requireLines(t, "mutant run", section(got.stdout, "run mutant 1:"),
		`the tool printed "41", want 42`, "GOFLAGS: -tags=planted -overlay=")
	for _, head := range []string{"run baseline 1:", "run after 1:"} {
		lines := section(got.stdout, head)
		requireLines(t, head, lines, "GOFLAGS: -tags=planted")
		requireLines(t, head, []string{strings.Join(lines, "\n")}, "pass")
		if strings.Contains(strings.Join(lines, "\n"), "-overlay") {
			t.Errorf("%s carries the overlay:\n%s", head, strings.Join(lines, "\n"))
		}
	}
	requireLines(t, "report", strings.Split(got.stdout, "\n"), "seed: 7")
	requireUnchanged(t, before, snapshot(t, root))
}

// survivorModule's Total counts its calls on line 7; its test checks only the sum.
var survivorModule = map[string]string{
	"go.mod":              "module planted.invalid/m\n\ngo 1.26\n",
	"plant/total.go":      "package plant\n\nvar calls int\n\n// Total returns the sum of xs.\nfunc Total(xs []int) int {\n\tcalls++\n\tsum := 0\n\tfor _, x := range xs {\n\t\tsum += x\n\t}\n\treturn sum\n}\n",
	"plant/total_test.go": "package plant\n\nimport \"testing\"\n\nfunc TestTotal(t *testing.T) {\n\tif got := Total([]int{1, 2}); got != 3 {\n\t\tt.Fatalf(\"Total = %d, want 3\", got)\n\t}\n}\n",
}

// TestDeletionSurvivorEndToEnd (mutation-check › "A deletion that was reached"): the wrong change
// deletes line 7, which runs and which the test does not observe. Every mutant run passes, the
// real reach run's profile shows the line executed, and the verdict is survivor with its seed.
func TestDeletionSurvivorEndToEnd(t *testing.T) {
	root := plantModule(t, survivorModule)
	mutant := outside(t, "total.go", strings.Replace(survivorModule["plant/total.go"], "\tcalls++\n", "", 1))
	got := runIn(t, root, testEnv(t, "GOFLAGS="), "-pkg", "./plant", "-test", "TestTotal",
		"-file", "plant/total.go", "-mutant", mutant, "-expect", "total_test.go:7", "-seed", "3", "-runs", "1")
	if got.code == 0 || !strings.HasPrefix(got.lastLine(), "verdict: survivor") || !strings.Contains(got.lastLine(), "seed 3") {
		t.Fatalf("got exit %d, last line %q; want a non-zero exit and a survivor naming seed 3\n%s", got.code, got.lastLine(), got)
	}
	requireLines(t, "reach run", section(got.stdout, "run reach 1:"), "reach: reached", "target line 7", "-coverpkg")
}

// TestDetectionThroughASymlink (mutation-check › "A repository reached through a symbolic link"):
// the program runs in a symbolic link to the planted module, with PWD naming the link, as a shell
// that changed into the link leaves it. The wrong change (line 10 subtracts) fails TestTotal on
// line 7 when the module is reached directly, so the verdict is detection.
func TestDetectionThroughASymlink(t *testing.T) {
	root := plantModule(t, survivorModule)
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	mutant := outside(t, "total.go", strings.Replace(survivorModule["plant/total.go"], "sum += x", "sum -= x", 1))
	got := runIn(t, link, testEnv(t, "GOFLAGS=", "PWD="+link), "-pkg", "./plant", "-test", "TestTotal",
		"-file", "plant/total.go", "-mutant", mutant, "-expect", "total_test.go:7", "-runs", "1")
	if got.code != 0 || !strings.HasPrefix(got.lastLine(), "verdict: detection") {
		t.Fatalf("got exit %d, last line %q; want exit 0 and a detection\n%s", got.code, got.lastLine(), got)
	}
}
