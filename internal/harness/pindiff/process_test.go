package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// harness-boundaries › "Carried entries match the pin": when the program returns, no process it
// started for a fetch is still running, though the fetch was cut off while waiting. git runs a
// transport helper for a remote named hold://; the stand-in here opens a FIFO the test reads,
// writes its pid and its parent's, and then waits forever. The fetch is cut off by cancelling the
// run's context once the helper has written, which is the path the fetch bound takes too
// (exec's Cancel; TestCheckPinUnreadable covers the bound itself). A FIFO reads to end of file
// only once every process holding its write end has exited, so end of file after the program
// returns is the proof that the helper is gone. Nothing on the passing path waits on a clock: the
// 30-second bound only stops a failing run from hanging, and the pids let it clean up.
func TestFetchLeavesNoProcess(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "helper.fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	// The read end opens without waiting for a writer; the test then holds a write end of its own,
	// so a read waits for the helper instead of seeing end of file before the helper has started.
	reader, err := os.OpenFile(fifo, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	ownWriter, err := os.OpenFile(fifo, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer ownWriter.Close()
	if err := syscall.SetNonblock(int(reader.Fd()), false); err != nil {
		t.Fatal(err)
	}

	bin := filepath.Join(dir, "bin")
	helper := "#!/bin/sh\nexec 3>'" + fifo + "'\nprintf 'started %s %s\\n' $$ $PPID >&3\nexec tail -f /dev/null\n"
	writeFiles(t, bin, map[string]string{"git-remote-hold": helper})
	if err := os.Chmod(filepath.Join(bin, "git-remote-hold"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	sha := strings.Repeat("1", 40)
	root := tree(t, ledgerOf(row{"pkg/a", sha, "internal/a", "carry"}), checkTree)
	env := map[string]string{"SEMENGINE_PIN_REMOTE": "hold://semstreams", "SEMENGINE_PIN_FETCH_BOUND": "30s"}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	finished := make(chan outcome, 1)
	go func() {
		var stdout, stderr bytes.Buffer
		code := run(ctx, root, []string{"check"}, func(k string) string { return env[k] }, &stdout, &stderr)
		finished <- outcome{code, stdout.String(), stderr.String()}
	}()

	// The helper's first line proves the fetch really started it.
	var pid, parent int
	started := make(chan error, 1)
	go func() {
		_, err := fmt.Fscanf(reader, "started %d %d\n", &pid, &parent)
		started <- err
	}()
	select {
	case err := <-started:
		if err != nil {
			t.Fatalf("reading the stand-in helper's first line: %v", err)
		}
	case got := <-finished:
		_ = ownWriter.Close()
		t.Fatalf("the program returned before the stand-in helper ran:\n%s", got)
	}
	_ = ownWriter.Close()
	cancel()
	got := <-finished
	requirePinUnreadable(t, got, sha)

	ended := make(chan error, 1)
	go func() {
		_, err := io.ReadAll(reader)
		ended <- err
	}()
	select {
	case err := <-ended:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(30 * time.Second):
		// Clean up what the failing run left behind: the helper and the git process that ran it,
		// never this test's own process group.
		_ = syscall.Kill(pid, syscall.SIGKILL)
		if parent > 1 {
			_ = syscall.Kill(parent, syscall.SIGKILL)
		}
		t.Fatal("a process the fetch started still holds the helper's pipe after the program returned")
	}
}
