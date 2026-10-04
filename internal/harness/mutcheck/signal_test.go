package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// TestSIGTERMStopsEveryProcess (mutation-check › "Interrupted"): SIGTERM reaches the built
// program while the go test of its first run waits, having started a helper. When the program
// exits, neither the stand-in go nor its helper is running, the exit status is non-zero, no
// verdict was printed, and the planted module is unchanged. The program is built with the
// inherited environment, so an outer check's overlay reaches it, and run with -overlay removed
// from GOFLAGS, so that it does not refuse (design D11, "The self-check").
func TestSIGTERMStopsEveryProcess(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "mutcheck")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	pipe := newHeldPipe(t)
	standIn(t, pipe.holdingStandIn(t))
	root := plantModule(t, standInModule)
	before := snapshot(t, root)

	cmd := exec.CommandContext(t.Context(), bin, "-pkg", "./p", "-test", "TestValue", "-file", "p/p.go",
		"-mutant", outside(t, "p.go", valueMutant), "-expect", "value_test.go:10")
	cmd.Dir = root
	cmd.Env = testEnv(t, "GOFLAGS="+withoutOverlay(os.Getenv("GOFLAGS")))
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan result, 1)
	go func() {
		err := cmd.Wait()
		code := 0
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			code = exit.ExitCode()
		} else if err != nil {
			code = -1
		}
		done <- result{code, stdout.String(), stderr.String()}
	}()
	pipe.started(t, done)
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	got := <-done
	if got.code == 0 {
		t.Errorf("got exit 0 after SIGTERM, want non-zero\n%s", got)
	}
	if strings.Contains(got.stdout+got.stderr, "verdict:") {
		t.Errorf("printed a verdict after SIGTERM\n%s", got)
	}
	pipe.requireGone(t)
	requireUnchanged(t, before, snapshot(t, root))
}

// withoutOverlay removes every -overlay setting from a GOFLAGS value.
func withoutOverlay(goflags string) string {
	var kept []string
	for _, f := range strings.Fields(goflags) {
		if !strings.HasPrefix(strings.TrimLeft(f, "-"), "overlay") {
			kept = append(kept, f)
		}
	}
	return strings.Join(kept, " ")
}
