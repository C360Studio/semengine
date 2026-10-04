package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// heldPipe is a FIFO a stand-in's helper writes "started <pid> <parent pid>" to and then holds
// open. The test holds a write end of its own until the helper has written, so a read waits for
// the helper instead of seeing end of file first. A FIFO reads to end of file only once every
// process holding its write end has exited, so end of file after the program has returned proves
// that the helper and the stand-in that started it are gone (as TestFetchLeavesNoProcess in
// internal/harness/pindiff does).
type heldPipe struct {
	path        string
	reader      *os.File
	ownWriter   *os.File
	pid, parent int
}

func newHeldPipe(t *testing.T) *heldPipe {
	t.Helper()
	p := &heldPipe{path: filepath.Join(t.TempDir(), "helper.fifo")}
	if err := syscall.Mkfifo(p.path, 0o600); err != nil {
		t.Fatal(err)
	}
	var err error
	if p.reader, err = os.OpenFile(p.path, os.O_RDONLY|syscall.O_NONBLOCK, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.reader.Close() })
	if p.ownWriter, err = os.OpenFile(p.path, os.O_WRONLY, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.ownWriter.Close() })
	if err := syscall.SetNonblock(int(p.reader.Fd()), false); err != nil {
		t.Fatal(err)
	}
	return p
}

// holdingStandIn is the body of a stand-in go whose go test calls open the pipe, start a helper
// that writes its pid and its parent's to it, and then wait for the helper, which never ends.
func (p *heldPipe) holdingStandIn(t *testing.T) string {
	t.Helper()
	helper := filepath.Join(t.TempDir(), "helper.sh")
	if err := os.WriteFile(helper, []byte("printf 'started %s %s\\n' $$ $PPID >&3\nexec tail -f /dev/null\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return "exec 3>'" + p.path + "'\nsh '" + helper + "' &\nwait\n"
}

// started waits for the helper's line, or for done to report that the program ended first.
func (p *heldPipe) started(t *testing.T, done <-chan result) {
	t.Helper()
	read := make(chan error, 1)
	go func() {
		_, err := fmt.Fscanf(p.reader, "started %d %d\n", &p.pid, &p.parent)
		read <- err
	}()
	select {
	case err := <-read:
		if err != nil {
			t.Fatalf("reading the stand-in helper's line: %v", err)
		}
	case got := <-done:
		_ = p.ownWriter.Close()
		t.Fatalf("the program ended before the stand-in's helper ran:\n%s", got)
	}
	_ = p.ownWriter.Close()
}

// ended waits for the program to end after it should have stopped its run. The bound only stops
// a failing test from hanging: a program that does not stop the run fails here, and the helper and
// the stand-in it held are killed so the program can return.
func (p *heldPipe) ended(t *testing.T, done <-chan result) result {
	t.Helper()
	select {
	case got := <-done:
		return got
	case <-time.After(30 * time.Second):
		_ = syscall.Kill(p.pid, syscall.SIGKILL)
		if p.parent > 1 {
			_ = syscall.Kill(p.parent, syscall.SIGKILL)
		}
		t.Fatal("the program had not ended 30s after it should have stopped the run")
		return result{}
	}
}

// requireGone fails unless the pipe reads to end of file: no process the stand-in started, and
// not the stand-in itself, still holds it. Nothing on the passing path waits on a clock; the bound
// only stops a failing test from hanging, and the pids let it clean up.
func (p *heldPipe) requireGone(t *testing.T) {
	t.Helper()
	ended := make(chan error, 1)
	go func() {
		_, err := io.ReadAll(p.reader)
		ended <- err
	}()
	select {
	case err := <-ended:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(30 * time.Second):
		_ = syscall.Kill(p.pid, syscall.SIGKILL)
		if p.parent > 1 {
			_ = syscall.Kill(p.parent, syscall.SIGKILL)
		}
		t.Fatal("a process the run started still holds the helper's pipe after the program returned")
	}
}

// TestRunPastItsBoundIsStopped (mutation-check › "The runs", "A run that does not end"): with
// -timeout 200ms, a go test that has not ended by 400ms is stopped with every process it started,
// the check is inconclusive and names the bound, and no further run is made.
func TestRunPastItsBoundIsStopped(t *testing.T) {
	pipe := newHeldPipe(t)
	log := standIn(t, pipe.holdingStandIn(t))
	root := plantModule(t, standInModule)
	env := testEnv(t, "GOFLAGS=")
	done := make(chan result, 1)
	go func() {
		done <- runIn(t, root, env, "-pkg", "./p", "-test", "TestValue", "-file", "p/p.go",
			"-mutant", outside(t, "p.go", valueMutant), "-expect", "value_test.go:10", "-timeout", "200ms")
	}()
	pipe.started(t, done)
	got := pipe.ended(t, done)
	if got.code == 0 || !strings.HasPrefix(got.lastLine(), "verdict: inconclusive") {
		t.Errorf("got exit %d, last line %q; want a non-zero exit and an inconclusive verdict\n%s", got.code, got.lastLine(), got)
	}
	for _, w := range []string{"baseline run 1", "400ms", "stopped"} {
		if !strings.Contains(got.lastLine(), w) {
			t.Errorf("the verdict does not say %q: %q", w, got.lastLine())
		}
	}
	if calls := testCalls(t, log); len(calls) != 1 {
		t.Errorf("got %d go test calls, want 1: %q", len(calls), calls)
	}
	pipe.requireGone(t)
}

// TestTreeChangedDuringCheck (mutation-check › "The program writes nothing inside the repository",
// "The tree changes during the check"): the named test writes a file into its package directory
// that git does not ignore. The mutant runs detect the wrong change, and the check is still
// inconclusive, saying the tree changed.
func TestTreeChangedDuringCheck(t *testing.T) {
	body := "printf 'written\\n' > \"$PWD/p/written.txt\"\n" +
		"case \" $GOFLAGS \" in *\" -overlay=\"*) cat '" + eventsPath(t, "fail-expected") + "'; exit 1 ;; esac\n" +
		"cat '" + eventsPath(t, "pass") + "'\n"
	standIn(t, body)
	root := plantModule(t, standInModule)
	got := runIn(t, root, testEnv(t, "GOFLAGS="), "-pkg", "./p", "-test", "TestValue", "-file", "p/p.go",
		"-mutant", outside(t, "p.go", valueMutant), "-expect", "value_test.go:10", "-runs", "1")
	if got.code == 0 || !strings.HasPrefix(got.lastLine(), "verdict: inconclusive") ||
		!strings.Contains(got.lastLine(), "the tree changed during the check") {
		t.Fatalf("got exit %d, last line %q; want inconclusive because the tree changed\n%s", got.code, got.lastLine(), got)
	}
	requireLines(t, "report", strings.Split(got.stdout, "\n"), "run mutant 1: detection")
}
