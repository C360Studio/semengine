package toolcheck

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// merge-gate › "The merge check is ateam review gate at the pin", scenario "Wrapper passes the exit
// status through": scripts/merge-check.sh runs `ateam review gate` with its arguments and exits with
// the program's status. The gate's own rules are tested by the pinned release (ateam
// internal/review gate_test.go); here a fake program records its arguments and exits as told.

const fakeAteam = `#!/bin/sh
printf '%s\n' "$*" > "$FAKE_ATEAM_ARGS"
exit "$FAKE_ATEAM_EXIT"
`

// decoyAteam stands first on PATH when the fake is reached through ATEAM, so a script that ignores
// ATEAM never runs the real ateam (which would read GitHub) and is caught by its arguments.
const decoyAteam = `#!/bin/sh
printf 'ateam on PATH: %s\n' "$*" > "$FAKE_ATEAM_ARGS"
exit 97
`

// runWrapper runs script (the text of merge-check.sh) from a throwaway root with args, the fake
// program reached through ATEAM (with a decoy ateam first on PATH) when viaEnv is set, else as
// ateam first on PATH. It returns the
// exit status and the arguments the fake received.
func runWrapper(t *testing.T, script string, viaEnv bool, exit int, args ...string) (int, string) {
	t.Helper()
	root, _ := writeTree(t, map[string]string{"scripts/merge-check.sh": script})
	argsFile := filepath.Join(t.TempDir(), "args")
	extra := []string{"FAKE_ATEAM_ARGS=" + argsFile, fmt.Sprintf("FAKE_ATEAM_EXIT=%d", exit)}
	onPath := fakeAteam
	if viaEnv {
		prog := filepath.Join(t.TempDir(), "fake-ateam")
		if err := os.WriteFile(prog, []byte(fakeAteam), 0o755); err != nil {
			t.Fatal(err)
		}
		onPath = decoyAteam
		extra = append(extra, "ATEAM="+prog)
	}
	// An ATEAM the test process inherited is dropped; only the case's own is set.
	var env []string
	for _, kv := range fakeBin(t, map[string]string{"ateam": onPath}) {
		if !strings.HasPrefix(kv, "ATEAM=") {
			env = append(env, kv)
		}
	}
	env = append(env, extra...)
	cmd := exec.Command("bash", append([]string{filepath.Join(root, "scripts", "merge-check.sh")}, args...)...)
	cmd.Dir = root
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	status := 0
	var ee *exec.ExitError
	switch {
	case errors.As(err, &ee):
		status = ee.ExitCode()
	case err != nil:
		t.Fatalf("run merge-check.sh: %v\n%s", err, out)
	}
	got, _ := os.ReadFile(argsFile)
	return status, strings.TrimSpace(string(got))
}

// wrapperViolations runs the script once per exit status the gate has (0 pass, 1 fail, 2
// unavailable), through ATEAM and through PATH, and names every run whose status or arguments
// differ from what the fake was told.
func wrapperViolations(t *testing.T, script string) []string {
	t.Helper()
	var out []string
	for _, viaEnv := range []bool{true, false} {
		how := "ateam on PATH"
		if viaEnv {
			how = "ATEAM"
		}
		for _, exit := range []int{0, 1, 2} {
			status, args := runWrapper(t, script, viaEnv, exit, "452")
			if status != exit {
				out = append(out, fmt.Sprintf("merge-check.sh via %s: the program exited %d, the script %d", how, exit, status))
			}
			if args != "review gate 452" {
				out = append(out, fmt.Sprintf("merge-check.sh via %s: the program got arguments %q, want %q", how, args, "review gate 452"))
			}
		}
	}
	return out
}

func TestMergeCheckWrapper(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "scripts", "merge-check.sh"))
	if err != nil {
		t.Fatal(err)
	}
	requireNoViolations(t, "merge-check.sh", wrapperViolations(t, string(data)))
}

func TestMergeCheckWrapperSensitivity(t *testing.T) {
	const good = "#!/usr/bin/env bash\nset -euo pipefail\nexec \"${ATEAM:-ateam}\" review gate \"$@\"\n"
	requireNoViolations(t, "clean fixture", wrapperViolations(t, good))
	for _, tc := range []struct {
		name, old, repl string
		wants           []string
	}{
		{"arguments dropped", ` "$@"`, "", []string{"got arguments", "review gate 452"}},
		{"status swallowed", `exec "${ATEAM:-ateam}" review gate "$@"`, `"${ATEAM:-ateam}" review gate "$@" || true`,
			[]string{"exited 1, the script 0"}},
		{"ATEAM ignored", `"${ATEAM:-ateam}"`, "ateam", []string{"via ATEAM", "got arguments"}},
		{"another subcommand", "review gate", "review check", []string{"review gate 452"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(good, tc.old) {
				t.Fatalf("fixture lacks %q", tc.old)
			}
			requireViolation(t, wrapperViolations(t, strings.Replace(good, tc.old, tc.repl, 1)), tc.wants...)
		})
	}
}
