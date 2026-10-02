package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// testGitEnv is the environment of every git call a test makes: the user's and the system's git
// configuration are ignored, and commits get a fixed author and date.
func testGitEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			env = append(env, kv)
		}
	}
	return append(env,
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_TERMINAL_PROMPT=0",
		"GIT_AUTHOR_NAME=pin", "GIT_AUTHOR_EMAIL=pin@example.invalid", "GIT_AUTHOR_DATE=2026-01-01T00:00:00Z",
		"GIT_COMMITTER_NAME=pin", "GIT_COMMITTER_EMAIL=pin@example.invalid", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z",
	)
}

func testGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = testGitEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// pinRepo makes a local repository that stands in for SemStreams, one commit per snapshot (each
// snapshot is the whole tree at that commit), and returns its path and the commits' SHAs in order.
func pinRepo(t *testing.T, snapshots ...map[string]string) (string, []string) {
	t.Helper()
	dir := t.TempDir()
	testGit(t, dir, "init", "-q", "-b", "main")
	var shas []string
	for _, files := range snapshots {
		testGit(t, dir, "rm", "-rq", "--ignore-unmatch", ".")
		writeFiles(t, dir, files)
		testGit(t, dir, "add", "-A")
		testGit(t, dir, "commit", "-q", "--allow-empty", "-m", "snapshot")
		shas = append(shas, testGit(t, dir, "rev-parse", "HEAD"))
	}
	return dir, shas
}

func writeFiles(t *testing.T, root string, files map[string]string) {
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

// tree writes a SemEngine working tree: the files given and docs/admission-ledger.yaml.
func tree(t *testing.T, ledger string, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	writeFiles(t, root, files)
	writeFiles(t, root, map[string]string{"docs/admission-ledger.yaml": ledger})
	return root
}

// row is the part of a ledger entry the program reads; the other six fields are the schema
// test's business.
type row struct{ path, sha, dest, disp string }

func ledgerOf(rows ...row) string {
	var b strings.Builder
	b.WriteString("# test ledger\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "- source_path: %s\n  source_sha: %s\n  destination: %s\n  disposition: %s\n", r.path, r.sha, r.dest, r.disp)
	}
	return b.String()
}

type outcome struct {
	code           int
	stdout, stderr string
}

// runIn runs the program in-process with root as its working directory and env as its
// environment.
func runIn(t *testing.T, root string, env map[string]string, args ...string) outcome {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(t.Context(), root, args, func(k string) string { return env[k] }, &stdout, &stderr)
	return outcome{code, stdout.String(), stderr.String()}
}

func remoteEnv(remote string) map[string]string {
	return map[string]string{"SEMENGINE_PIN_REMOTE": remote}
}

func (o outcome) String() string {
	return fmt.Sprintf("exit %d\n--- stdout\n%s--- stderr\n%s", o.code, o.stdout, o.stderr)
}

// requireLine fails unless one line of text contains every fragment.
func requireLine(t *testing.T, text string, fragments ...string) {
	t.Helper()
	for _, line := range strings.Split(text, "\n") {
		all := true
		for _, f := range fragments {
			all = all && strings.Contains(line, f)
		}
		if all {
			return
		}
	}
	t.Fatalf("no line holds all of %q in:\n%s", fragments, text)
}
