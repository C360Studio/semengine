package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"time"
)

// withEnv returns environ with each entry of set in place of the entries with its key.
func withEnv(environ []string, set ...string) []string {
	var env []string
	for _, kv := range environ {
		k, _, _ := strings.Cut(kv, "=")
		if !slices.ContainsFunc(set, func(s string) bool { return strings.HasPrefix(s, k+"=") }) {
			env = append(env, kv)
		}
	}
	return append(env, set...)
}

// command is a child process in a process group of its own. When ctx is done while it runs (the
// bound of a run, or SIGINT or SIGTERM to the program), the whole group is killed, so a process it
// started in that group dies with it; one it started in a group of its own does not. A group is
// not signalled after its leader exited by itself: its number may then belong to a process this
// program did not start. PWD is the directory the child runs in: Go takes its working directory
// from PWD when PWD names that directory, and looks overlay paths up under it without resolving
// symbolic links, so a caller's PWD through a link would hide the overlay (design D15, P26).
func command(ctx context.Context, dir string, env []string, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir, cmd.Env = dir, withEnv(env, "PWD="+dir)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	return cmd
}

// gitEnv is the environment of the program's git commands: reading the tree's state does not
// refresh git's index (design D1).
func gitEnv(environ []string) []string { return withEnv(environ, "GIT_OPTIONAL_LOCKS=0") }

// runGit runs git in root. A diff that found differences exits 1, which is not an error here.
func runGit(ctx context.Context, root string, environ []string, args ...string) ([]byte, error) {
	cmd := command(ctx, root, gitEnv(environ), "git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 && args[0] == "diff" {
		err = nil
	}
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// fingerprint is what scripts/tree-state.sh prints for the tree it lives in.
func fingerprint(ctx context.Context, root string, environ []string) (string, error) {
	cmd := command(ctx, root, gitEnv(environ), filepath.Join(root, "scripts", "tree-state.sh"))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("scripts/tree-state.sh: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}

// runPattern anchors each part of -test, so it selects that test, or that subtest, exactly.
func runPattern(test string) string {
	parts := strings.Split(test, "/")
	for i, p := range parts {
		parts[i] = "^" + regexp.QuoteMeta(p) + "$"
	}
	return strings.Join(parts, "/")
}

// testArgs is the command line every run of one check shares (mutation-check › "The runs").
func testArgs(in inputs) []string {
	return []string{"test", "-json", "-count=1", "-cpu", "1", "-race", "-timeout", in.timeout.String(), "-run", runPattern(in.test)}
}

// runSpec is one go test to make.
type runSpec struct {
	label string // names its log files
	args  []string
	env   []string
	dir   string
	logs  string
	bound time.Duration
}

// goTest runs one go test, its output going to files in the log directory. A run that has not
// ended by the bound, and every run when ctx is cancelled (SIGINT or SIGTERM to the program), is
// stopped by killing its whole process group. Its error means the run could not be made, or the
// program was interrupted (then it is ctx's error).
func goTest(ctx context.Context, s runSpec) ([]byte, processEnd, time.Duration, error) {
	streamPath := filepath.Join(s.logs, s.label+".jsonl")
	stream, err := os.Create(streamPath)
	if err != nil {
		return nil, processEnd{}, 0, err
	}
	defer stream.Close()
	stderr, err := os.Create(filepath.Join(s.logs, s.label+".stderr"))
	if err != nil {
		return nil, processEnd{}, 0, err
	}
	defer stderr.Close()

	runCtx, cancel := context.WithTimeout(ctx, s.bound)
	defer cancel()
	cmd := command(runCtx, s.dir, s.env, "go", s.args...)
	cmd.Stdout, cmd.Stderr = stream, stderr
	start := time.Now()
	runErr := cmd.Run()
	wall := time.Since(start)
	if ctx.Err() != nil {
		return nil, processEnd{}, wall, ctx.Err()
	}
	if cmd.ProcessState == nil {
		return nil, processEnd{}, wall, fmt.Errorf("go test could not be started: %w", runErr)
	}
	end := processEnd{code: cmd.ProcessState.ExitCode(), bounded: runCtx.Err() != nil}
	if ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		end.signal = ws.Signal().String()
	}
	out, err := os.ReadFile(streamPath)
	return out, end, wall, err
}
