package main

import (
	"errors"
	"os/exec"
	"path/filepath"
	"testing"
)

// The program run as a separate process from the tree's root, as `task ledger:check` runs it
// (`go run ./internal/harness/pindiff check`): a planted carry violation exits non-zero and names
// the violation; the same tree with no carry entry exits 0. The binary is built here and run from
// a temporary tree, because go run needs the module as its working directory and the program
// reads the ledger from its own.
func TestCommandExitStatus(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "pindiff")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	remote, shas := pinRepo(t, map[string]string{"pkg/a/a.go": "package a\n"})
	files := map[string]string{"internal/a/a.go": "package a // edited\n"}

	runBin := func(root string) (string, error) {
		cmd := exec.CommandContext(t.Context(), bin, "check")
		cmd.Dir = root
		cmd.Env = append(testGitEnv(), "SEMENGINE_PIN_REMOTE="+remote)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	out, err := runBin(tree(t, ledgerOf(row{"pkg/a", shas[0], "internal/a", "carry"}), files))
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() == 0 {
		t.Fatalf("planted carry violation: got err %v, want a non-zero exit\n%s", err, out)
	}
	requireLine(t, out, "carry entry pkg/a:", "internal/a/a.go", "differs from the pin")

	out, err = runBin(tree(t, ledgerOf(row{"pkg/a", shas[0], "internal/a", "adapt"}), files))
	if err != nil {
		t.Fatalf("no carry entry: got err %v, want exit 0\n%s", err, out)
	}
	requireLine(t, out, "no carry entry")
}
