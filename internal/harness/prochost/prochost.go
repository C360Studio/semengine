// Package prochost hosts a helper process for process-kill proofs (openspec change
// setup-04a-01-floor, capability process-host). A helper is a function in the test binary: the
// test binary re-executes itself with an environment marker that selects it, so no runtime binary
// is built. The child runs in its own process group with stdout and stderr in files, and the test
// can signal, pause, resume, kill and wait for it. No helper outlives its test: Start registers a
// cleanup that kills the group and joins the child. The start, group and signal paths are
// generalised from internal/harness/runner/runner_test.go. Test-only: contract test T-B1 refuses
// any production import.
//
// A helper that must stay up parks on a signal (signal.Notify for SIGTERM, then receive), never on
// a bare select{}. The child runs with only -test.run, so no test-timeout timer keeps the runtime
// waiting: once every goroutine blocks, Go kills it with "all goroutines are asleep" and exit 2.
package prochost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const (
	// markerEnv names the helper a re-executed test binary runs.
	markerEnv = "SEMENGINE_HELPER"
	// evidenceEnv is the integration lane's evidence directory.
	evidenceEnv = "SEMENGINE_EVIDENCE_DIR"
	// cleanupBudget is the fresh finite authority the test cleanup gives its wait for a killed
	// group. A SIGKILLed child is reaped at once; the budget only decides when the cleanup reports
	// a child that is not, and the cleanup still joins it afterwards.
	cleanupBudget = 30 * time.Second
)

// Helper runs fn and exits the process when the environment marker names this helper, and
// otherwise returns at once. Call it from the test binary's TestHelperProcess, once per helper;
// in a normal test run every call is a no-op.
func Helper(name string, fn func()) {
	if name == "" || os.Getenv(markerEnv) != name {
		return
	}
	fn()
	os.Exit(0)
}

// ExitStatus is how a helper ended: Code is its exit code, or -1 when a signal ended it, and
// Signal is that signal, zero otherwise.
type ExitStatus struct {
	Code   int
	Signal syscall.Signal
}

// Process is one running helper. Signals go to its whole process group.
type Process struct {
	name           string
	pid            int
	stdout, stderr string // output files

	done   chan struct{} // closed once the child has been reaped
	status ExitStatus    // valid once done is closed
	err    error         // a wait error other than the exit status; valid once done is closed
}

// Start re-executes the test binary to run the helper name, with env added to this process's
// environment, in a new process group. Output goes to files under SEMENGINE_EVIDENCE_DIR when it is
// set and under t.TempDir() otherwise. t's cleanup kills the group and waits for the child.
func Start(t testing.TB, name string, env ...string) (*Process, error) {
	t.Helper()
	if name == "" {
		return nil, errors.New("prochost: Start with an empty helper name")
	}
	dir := os.Getenv(evidenceEnv)
	if dir == "" {
		dir = t.TempDir()
	}
	dir = filepath.Join(dir, "prochost")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("prochost: output directory: %w", err)
	}
	base := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()) + "-" + name + "-"
	stdout, err := os.CreateTemp(dir, base+"*.stdout")
	if err != nil {
		return nil, fmt.Errorf("prochost: stdout file: %w", err)
	}
	defer func() { _ = stdout.Close() }()
	stderr, err := os.Create(strings.TrimSuffix(stdout.Name(), ".stdout") + ".stderr")
	if err != nil {
		return nil, fmt.Errorf("prochost: stderr file: %w", err)
	}
	defer func() { _ = stderr.Close() }()

	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperProcess$")
	cmd.Env = append(append(os.Environ(), env...), markerEnv+"="+name)
	cmd.Stdout, cmd.Stderr = stdout, stderr // files, so Wait copies nothing
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("prochost: start helper %s: %w", name, err)
	}
	p := &Process{name: name, pid: cmd.Process.Pid, stdout: stdout.Name(), stderr: stderr.Name(), done: make(chan struct{})}
	// The reaper is joined by the cleanup below, which kills the group first.
	go p.reap(cmd)
	t.Cleanup(func() {
		if err := p.Kill(); err != nil {
			t.Errorf("prochost: cleanup kill of helper %s (pid %d): %v", name, p.pid, err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), cleanupBudget)
		defer cancel()
		if _, err := p.Wait(ctx); err != nil {
			t.Errorf("prochost: helper %s not reaped after SIGKILL: %v", name, err)
		}
		<-p.done
	})
	return p, nil
}

func (p *Process) reap(cmd *exec.Cmd) {
	defer close(p.done)
	err := cmd.Wait()
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		p.err = err
	}
	if cmd.ProcessState == nil {
		p.status = ExitStatus{Code: -1}
		return
	}
	ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
	switch {
	case ok && ws.Signaled():
		p.status = ExitStatus{Code: -1, Signal: ws.Signal()}
	default:
		p.status = ExitStatus{Code: cmd.ProcessState.ExitCode()}
	}
}

// signalGroup signals the process group pgid. It is a variable so a test can count the calls.
var signalGroup = func(pgid int, sig syscall.Signal) error { return syscall.Kill(-pgid, sig) }

// Signal sends sig, which must be a syscall.Signal, to the helper's process group. Once the
// helper has been reaped it sends nothing and returns an error, as Pause and Resume do.
//
// A reap that lands between the Alive check and the signal is not prevented: the kernel frees
// the pid inside the reaper's blocking wait, so no lock can sit between them, and closing the
// window needs a per-platform waitid(WNOWAIT) reaper. Reuse inside that window needs the pid
// space to wrap within microseconds; for a test harness that residual is accepted.
func (p *Process) Signal(sig os.Signal) error {
	if !p.Alive() {
		// Reaped: the group id may now name another process's group.
		return fmt.Errorf("prochost: helper %s (pid %d) has exited; no signal sent", p.name, p.pid)
	}
	s, ok := sig.(syscall.Signal)
	if !ok {
		return fmt.Errorf("prochost: signal %v is not a syscall.Signal", sig)
	}
	if err := signalGroup(p.pid, s); err != nil {
		return fmt.Errorf("prochost: signal %v to helper %s (group %d): %w", sig, p.name, p.pid, err)
	}
	return nil
}

// Pause stops the helper's process group (SIGSTOP).
func (p *Process) Pause() error { return p.Signal(syscall.SIGSTOP) }

// Resume continues the helper's process group (SIGCONT).
func (p *Process) Resume() error { return p.Signal(syscall.SIGCONT) }

// Kill kills the helper's process group (SIGKILL). Once the helper has been reaped it sends
// nothing and returns nil; a group already gone is not an error.
func (p *Process) Kill() error {
	if !p.Alive() {
		return nil // reaped: the group id may now name another process's group
	}
	if err := signalGroup(p.pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("prochost: kill helper %s (group %d): %w", p.name, p.pid, err)
	}
	return nil
}

// StderrPath is the file the helper's stderr goes to. A test reads it once Wait has returned, for
// what the child printed as it ended, such as a runtime panic.
func (p *Process) StderrPath() string { return p.stderr }

// Alive reports whether the helper has not yet exited. It reads the host's own record of the
// child, not the pid, so a reused pid cannot make it true.
func (p *Process) Alive() bool {
	select {
	case <-p.done:
		return false
	default:
		return true
	}
}

// Wait returns the helper's exit status once it has exited, or, when ctx ends first, ctx's error
// together with the process state last observed.
func (p *Process) Wait(ctx context.Context) (ExitStatus, error) {
	if ctx == nil {
		return ExitStatus{}, errors.New("prochost: Wait with a nil context")
	}
	select {
	case <-p.done:
		return p.status, p.err
	default:
	}
	select {
	case <-p.done:
		return p.status, p.err
	case <-ctx.Done():
		state, err := psState(p.pid)
		if err != nil {
			state = "unknown (" + err.Error() + ")"
		}
		return ExitStatus{}, fmt.Errorf("prochost: helper %s (pid %d) still running, state %s: %w", p.name, p.pid, state, ctx.Err())
	}
}

// psState is the process state ps reports for pid, such as S, R or T (stopped).
func psState(pid int) (string, error) {
	out, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return "", fmt.Errorf("ps stat %d: %w", pid, err)
	}
	return strings.TrimSpace(string(out)), nil
}
