package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// standInModule is the planted module of the tests that run the program against a stand-in go.
// The stand-in never builds it; it is there for the program's checks of the target, the mutant,
// the package and the tree.
var standInModule = map[string]string{
	"go.mod":                 "module planted.invalid/m\n\ngo 1.26\n",
	"p/p.go":                 "package p\n\nimport \"strconv\"\n\nvar label string\n\n// Value returns one.\nfunc Value() int {\n\tlabel = strconv.Itoa(1)\n\treturn 1\n}\n",
	"p/p_test.go":            "package p\n\nimport \"testing\"\n\nfunc TestValue(t *testing.T) {\n\tif Value() != 1 {\n\t\tt.Fatal(Value())\n\t}\n}\n",
	"scripts/merge-check.sh": "#!/usr/bin/env bash\nexit 0\n",
}

// valueMutant is p/p.go with Value returning 2.
var valueMutant = strings.Replace(standInModule["p/p.go"], "return 1", "return 2", 1)

// passingStandIn prints the recorded passing stream for every go test call.
func passingStandIn(t *testing.T) string {
	return "cat '" + eventsPath(t, "pass") + "'\n"
}

// TestRefusals (mutation-check › "The command and its inputs"): one case for each scenario, and
// for -seed 0, a GOFLAGS that sets -overlay, a mutant inside the module and a mutant identical to
// its target, a target that does not exist and a target outside the module. Each refusal exits non-zero, names the input and the reason, prints no verdict, and
// starts no run: the stand-in go's log holds no go test call.
func TestRefusals(t *testing.T) {
	mutant := func(t *testing.T) string { return outside(t, "p.go", valueMutant) }
	base := func(t *testing.T, _ string) []string {
		return []string{"-pkg", "./p", "-test", "TestValue", "-file", "p/p.go", "-mutant", mutant(t), "-expect", "p_test.go:7"}
	}
	shared := plantModule(t, standInModule)
	replace := func(args []string, flag, value string) []string {
		out := append([]string(nil), args...)
		for i := range out {
			if out[i] == flag {
				out[i+1] = value
			}
		}
		return out
	}
	for _, tc := range []struct {
		name  string
		args  func(t *testing.T, root string) []string
		env   func(t *testing.T, root string) []string // extra entries for the program's environment
		setup func(t *testing.T, root string)
		own   bool // the case writes into the module, so it gets one of its own
		words []string
	}{
		{name: "a script as the target",
			args: func(t *testing.T, root string) []string {
				return replace(base(t, root), "-file", "scripts/merge-check.sh")
			},
			words: []string{"-file", "only a Go source file that is not a test file can be the target", "manual procedure", "docs/testing.md"}},
		{name: "a test file as the target",
			args:  func(t *testing.T, root string) []string { return replace(base(t, root), "-file", "p/p_test.go") },
			words: []string{"-file", "the test, its inputs and its expectations stay unchanged", "manual procedure", "docs/testing.md"}},
		{name: "no expected assertion",
			args: func(t *testing.T, _ string) []string {
				return []string{"-pkg", "./p", "-test", "TestValue", "-file", "p/p.go", "-mutant", mutant(t)}
			},
			words: []string{"-expect", "-expect-text"}},
		{name: "a package pattern",
			args:  func(t *testing.T, root string) []string { return replace(base(t, root), "-pkg", "./...") },
			words: []string{"-pkg", "one package"}},
		{name: "coverage in GOFLAGS set with go env -w",
			args: base,
			env: func(t *testing.T, _ string) []string {
				file := filepath.Join(t.TempDir(), "go.env")
				goEnvWrite(t, file, "GOFLAGS=-cover")
				return []string{"GOENV=" + file}
			},
			words: []string{"GOFLAGS", "-cover", "under coverage Go would build the target from the file on disk"}},
		{name: "a failure file left by an earlier run",
			args: base,
			setup: func(t *testing.T, root string) {
				writeFiles(t, root, map[string]string{"p/testdata/rapid/TestValue/TestValue-20261004-1.fail": "# left behind\n"})
			},
			own:   true,
			words: []string{"p/testdata/rapid/TestValue/TestValue-20261004-1.fail", "does not track"}},
		{name: "-seed 0",
			args:  func(t *testing.T, root string) []string { return append(base(t, root), "-seed", "0") },
			words: []string{"-seed", "random"}},
		{name: "GOFLAGS that sets -overlay",
			args: base,
			env: func(t *testing.T, _ string) []string {
				return []string{"GOFLAGS=-overlay=" + filepath.Join(t.TempDir(), "o.json")}
			},
			words: []string{"GOFLAGS", "-overlay"}},
		{name: "a mutant inside the module",
			args: func(t *testing.T, root string) []string {
				writeFiles(t, root, map[string]string{"p/copy.go.txt": valueMutant})
				return replace(base(t, root), "-mutant", filepath.Join(root, "p", "copy.go.txt"))
			},
			own:   true,
			words: []string{"-mutant", "inside the module"}},
		{name: "a mutant identical to its target",
			args: func(t *testing.T, root string) []string {
				return replace(base(t, root), "-mutant", outside(t, "p.go", standInModule["p/p.go"]))
			},
			words: []string{"-mutant", "same content as the target"}},
		{name: "a target that does not exist",
			args:  func(t *testing.T, root string) []string { return replace(base(t, root), "-file", "p/missing.go") },
			words: []string{"-file", "p/missing.go", "does not exist"}},
		{name: "a malformed expected location",
			args:  func(t *testing.T, root string) []string { return replace(base(t, root), "-expect", "probe_test.go") },
			words: []string{"-expect", `"probe_test.go"`, "file.go:N"}},
		{name: "an empty expected text",
			args: func(t *testing.T, _ string) []string {
				return []string{"-pkg", "./p", "-test", "TestValue", "-file", "p/p.go", "-mutant", mutant(t), "-expect-text", ""}
			},
			words: []string{"-expect-text"}},
		{name: "no go.mod at the root",
			args: base,
			setup: func(t *testing.T, root string) {
				if err := os.Remove(filepath.Join(root, "go.mod")); err != nil {
					t.Fatal(err)
				}
			},
			own:   true,
			words: []string{"go.mod", "repository's root"}},
		{name: "temporary files inside the module",
			args: base,
			env: func(t *testing.T, root string) []string {
				dir := filepath.Join(root, "tmp")
				if err := os.Mkdir(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				return []string{"TMPDIR=" + dir}
			},
			own:   true,
			words: []string{"TMPDIR", "would be written into the repository"}},
		{name: "a target outside the module",
			args: func(t *testing.T, root string) []string {
				return replace(base(t, root), "-file", outside(t, "q.go", "package q\n"))
			},
			words: []string{"-file", "outside the module"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log := standIn(t, passingStandIn(t))
			root := shared
			if tc.own {
				root = plantModule(t, standInModule)
			}
			if tc.setup != nil {
				tc.setup(t, root)
			}
			var extra []string
			if tc.env != nil {
				extra = tc.env(t, root)
			}
			got := runIn(t, root, testEnv(t, extra...), tc.args(t, root)...)
			if got.code == 0 {
				t.Fatalf("got exit 0, want a refusal\n%s", got)
			}
			for _, w := range tc.words {
				if !strings.Contains(got.stderr, w) {
					t.Errorf("the refusal does not say %q\n%s", w, got)
				}
			}
			if strings.Contains(got.stdout, "verdict:") {
				t.Errorf("a refusal printed a verdict\n%s", got)
			}
			if calls := testCalls(t, log); len(calls) > 0 {
				t.Errorf("a refusal started %d run(s): %q", len(calls), calls)
			}
		})
	}
}

// TestCommittedFailureFileIsNotRefused (mutation-check, "A failure file that is committed"): a
// failure file git tracks is a fixed input, and the check runs.
func TestCommittedFailureFileIsNotRefused(t *testing.T) {
	log := standIn(t, passingStandIn(t))
	files := map[string]string{"p/testdata/rapid/TestValue/TestValue-20261004-1.fail": "# curated\n"}
	for k, v := range standInModule {
		files[k] = v
	}
	root := plantModule(t, files)
	got := runIn(t, root, testEnv(t, "GOFLAGS="), "-pkg", "./p", "-test", "TestValue", "-file", "p/p.go",
		"-mutant", outside(t, "p.go", valueMutant), "-expect", "p_test.go:7", "-runs", "1")
	if strings.Contains(got.stderr, "testdata/rapid") {
		t.Errorf("refused on a committed failure file\n%s", got)
	}
	if calls := testCalls(t, log); len(calls) == 0 {
		t.Errorf("no run started\n%s", got)
	}
}

// goEnvWrite sets one value in a Go environment file with the real go env -w, as a caller would.
func goEnvWrite(t *testing.T, file, kv string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "go", "env", "-w", kv)
	cmd.Env = append(withoutKey(withoutKey(os.Environ(), "GOFLAGS"), "GOENV"), "GOENV="+file)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go env -w %s: %v\n%s", kv, err, out)
	}
}
