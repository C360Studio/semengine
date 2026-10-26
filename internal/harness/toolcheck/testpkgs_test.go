package toolcheck

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// plant writes files under root, creating their directories.
func plant(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// gitIn runs git in dir with a fixed identity and no signing, failing the test on error.
func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.invalid",
		"-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// runIn runs a script of the planted tree root with env and returns its combined output and
// exit status.
func runIn(t *testing.T, root string, env []string, script string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command("bash", append([]string{filepath.Join(root, "scripts", script)}, args...)...)
	cmd.Dir = root
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		return string(out), exit.ExitCode()
	case err != nil:
		t.Fatalf("run %s: %v", script, err)
	}
	return string(out), 0
}

const integrationLine = "//go:build integration\n\n"

// TestTestPkgsLists (merge-gate › "Tool tests and repository guards are separate packages";
// integration-test-runner › "Only packages with integration tests"): scripts/test-pkgs.sh prints
// each list on one line in a planted git module. The expected lists are written out by hand.
func TestTestPkgsLists(t *testing.T) {
	root := copyScript(t, "test-pkgs.sh")
	plant(t, root, map[string]string{
		"go.mod":     "module example.com/m\n\ngo 1.22\n",
		".gitignore": "ignored/\n",
		// Only untagged tests: a unit package, not an integration one.
		"a/a.go":      "package a\n",
		"a/a_test.go": "package a\n",
		// The only Go file is an integration-tagged test, so go list ./... does not name it.
		"b/b_test.go": integrationLine + "package b\n",
		// testdata at the top level and nested.
		"testdata/t_test.go":   integrationLine + "package t\n",
		"d/d.go":               "package d\n",
		"d/testdata/d_test.go": integrationLine + "package d\n",
		// An ignored tagged file.
		"ignored/i_test.go": integrationLine + "package i\n",
		// The repository guards stay unit; the three tool packages do not.
		"internal/harness/contract/c_test.go": "package contract\n",
		"internal/harness/toolcheck/doc.go":   "package toolcheck\n",
		"internal/harness/mutcheck/m.go":      "package mutcheck\n",
		"internal/harness/pindiff/p.go":       "package pindiff\n",
	})
	gitIn(t, root, "init", "-q")
	gitIn(t, root, "add", ".")
	gitIn(t, root, "commit", "-q", "-m", "init")
	// Added after the commit, so git does not track it.
	plant(t, root, map[string]string{
		"e/e.go":      "package e\n",
		"e/e_test.go": "  //go:build integration  \n\npackage e\n",
	})
	env := os.Environ()

	for _, tc := range []struct{ list, want string }{
		{"integration", "./b ./e\n"},
		{"unit", "./a ./d ./e ./internal/harness/contract\n"},
		{"tools", "./internal/harness/mutcheck ./internal/harness/pindiff ./internal/harness/toolcheck\n"},
	} {
		out, status := runIn(t, root, env, "test-pkgs.sh", tc.list)
		if status != 0 || out != tc.want {
			t.Errorf("test-pkgs.sh %s: exit %d, output %q; want exit 0, output %q", tc.list, status, out, tc.want)
		}
	}

	empty := copyScript(t, "test-pkgs.sh")
	plant(t, empty, map[string]string{
		"go.mod":      "module example.com/m\n\ngo 1.22\n",
		"a/a_test.go": "package a\n",
	})
	gitIn(t, empty, "init", "-q")
	gitIn(t, empty, "add", ".")
	gitIn(t, empty, "commit", "-q", "-m", "init")
	if out, status := runIn(t, empty, env, "test-pkgs.sh", "integration"); status == 0 {
		t.Errorf("test-pkgs.sh integration with no tagged file: exit 0, want non-zero\n%s", out)
	}
	// A module whose only package is a tool package has an empty unit list.
	toolsOnly := copyScript(t, "test-pkgs.sh")
	plant(t, toolsOnly, map[string]string{
		"go.mod":                            "module example.com/m\n\ngo 1.22\n",
		"internal/harness/toolcheck/doc.go": "package toolcheck\n",
	})
	if out, status := runIn(t, toolsOnly, env, "test-pkgs.sh", "unit"); status == 0 {
		t.Errorf("test-pkgs.sh unit with only a tool package: exit 0, want non-zero\n%s", out)
	}
	if out, status := runIn(t, empty, env, "test-pkgs.sh", "nonsense"); status == 0 {
		t.Errorf("test-pkgs.sh nonsense: exit 0, want non-zero\n%s", out)
	}
}
