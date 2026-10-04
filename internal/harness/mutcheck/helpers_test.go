package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot walks up from the package directory to SemEngine's go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the package")
		}
		dir = parent
	}
}

// testGitEnv keeps the user's and the system's git configuration (signing, hooks, templates)
// out of the planted repositories.
func testGitEnv() []string {
	return append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_AUTHOR_NAME=planted", "GIT_AUTHOR_EMAIL=planted@example.invalid",
		"GIT_COMMITTER_NAME=planted", "GIT_COMMITTER_EMAIL=planted@example.invalid")
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = testGitEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// plantModule writes files into a fresh directory with a copy of scripts/tree-state.sh, makes it
// a git repository with one commit holding everything, and returns its path.
func plantModule(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	script, err := os.ReadFile(filepath.Join(repoRoot(t), "scripts", "tree-state.sh"))
	if err != nil {
		t.Fatal(err)
	}
	writeFiles(t, root, files)
	writeFiles(t, root, map[string]string{"scripts/tree-state.sh": string(script)})
	if err := os.Chmod(filepath.Join(root, "scripts", "tree-state.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	gitIn(t, root, "init", "-q")
	gitIn(t, root, "add", "-A")
	gitIn(t, root, "commit", "-q", "-m", "planted")
	return root
}

// outside writes content to a file in a fresh directory outside every planted module and returns
// its path: where a mutant is kept.
func outside(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// testEnv is the environment a test gives the program: this process's environment without
// GOFLAGS, GOENV, RAPID_SEED and RAPID_NOFAILFILE, with GOENV=off so no Go environment file of
// the host applies, TMPDIR set to a fresh directory, and then the entries given. GORACE sets the
// race detector's wait at exit (one second by default, in every passing run) to zero: the planted
// tests have no exit-time code for it to watch, and the program passes the environment through
// to every run unchanged.
func testEnv(t *testing.T, set ...string) []string {
	t.Helper()
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		switch k {
		case "GOFLAGS", "GOENV", "RAPID_SEED", "RAPID_NOFAILFILE", "TMPDIR":
			continue
		}
		env = append(env, kv)
	}
	env = append(withoutKey(env, "GORACE"), "GOENV=off", "TMPDIR="+t.TempDir(), "GORACE=atexit_sleep_ms=0")
	for _, kv := range set {
		k, _, _ := strings.Cut(kv, "=")
		env = withoutKey(env, k)
		env = append(env, kv)
	}
	return env
}

func withoutKey(env []string, key string) []string {
	var out []string
	for _, kv := range env {
		if !strings.HasPrefix(kv, key+"=") {
			out = append(out, kv)
		}
	}
	return out
}

type result struct {
	code           int
	stdout, stderr string
}

func (r result) String() string {
	return fmt.Sprintf("exit %d\n--- stdout\n%s--- stderr\n%s", r.code, r.stdout, r.stderr)
}

// lastLine is the last line of the program's standard output.
func (r result) lastLine() string {
	lines := strings.Split(strings.TrimRight(r.stdout, "\n"), "\n")
	return lines[len(lines)-1]
}

// runIn runs the program in this process from root, as `task mutate:check` runs it from the
// repository's root.
func runIn(t *testing.T, root string, env []string, args ...string) result {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(t.Context(), root, args, env, &stdout, &stderr)
	return result{code, stdout.String(), stderr.String()}
}

// standIn puts a stand-in go first on PATH for the rest of the test. It appends the arguments of
// every call to the returned log, passes `go env` to the real go, and runs body for every other
// call. The program finds go through PATH, so it runs the stand-in instead of the real go.
func standIn(t *testing.T, body string) string {
	t.Helper()
	realGo, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> '" + log + "'\n" +
		"if [ \"$1\" = env ]; then exec '" + realGo + "' \"$@\"; fi\n" + body
	if err := os.WriteFile(filepath.Join(dir, "go"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

// testCalls lists the go test calls a stand-in's log holds.
func testCalls(t *testing.T, log string) []string {
	t.Helper()
	data, err := os.ReadFile(log)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	var calls []string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "test ") {
			calls = append(calls, line)
		}
	}
	return calls
}

// eventsPath is the absolute path of a recorded stream, for a stand-in to print.
func eventsPath(t *testing.T, name string) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("testdata", "events", name+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// snapshot maps every path under root to a hash of its content, or "dir" for a directory.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		if d.IsDir() {
			files[rel] = "dir"
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		files[rel] = fmt.Sprintf("%x", sha256.Sum256(data))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func requireUnchanged(t *testing.T, before, after map[string]string) {
	t.Helper()
	if maps.Equal(before, after) {
		return
	}
	for k, v := range after {
		if before[k] != v {
			t.Errorf("changed or added: %s", k)
		}
	}
	for k := range before {
		if _, ok := after[k]; !ok {
			t.Errorf("removed: %s", k)
		}
	}
}

// readOnly removes write permission from every directory under root until the test ends.
func readOnly(t *testing.T, root string) {
	t.Helper()
	var dirs []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			dirs = append(dirs, p)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range dirs {
		if err := os.Chmod(d, 0o555); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, d := range dirs {
			_ = os.Chmod(d, 0o755)
		}
	})
}

// section returns the lines of the report from the line that starts with head up to the next
// line that is not indented.
func section(report, head string) []string {
	var lines []string
	in := false
	for _, line := range strings.Split(report, "\n") {
		switch {
		case strings.HasPrefix(line, head):
			in = true
		case in && !strings.HasPrefix(line, " "):
			return lines
		}
		if in {
			lines = append(lines, line)
		}
	}
	return lines
}

// requireLines fails unless, for each fragment, some line of lines contains it.
func requireLines(t *testing.T, what string, lines []string, fragments ...string) {
	t.Helper()
	for _, f := range fragments {
		found := false
		for _, l := range lines {
			if strings.Contains(l, f) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s: no line contains %q in:\n%s", what, f, strings.Join(lines, "\n"))
		}
	}
}
