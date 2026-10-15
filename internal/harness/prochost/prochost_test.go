package prochost

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/c360studio/semengine/internal/harness/probe"
)

// failureBound bounds waits on events that a correct run delivers at once; it never paces a run.
const failureBound = 10 * time.Second

// The helpers this package's tests start. Each runs only in a child started with its marker.
func TestHelperProcess(t *testing.T) {
	Helper("checkpoints", checkpoints)
	Helper("park", block)
	Helper("parent", func() { helperParent(t) })
	Helper("hello", func() {
		terminated := parkUntilSIGTERM()
		fmt.Println("hello from the helper")
		fmt.Fprintln(os.Stderr, "hello on stderr")
		<-terminated
		fmt.Println(helloTerminated)
	})
}

// helloTerminated is what the hello helper prints once SIGTERM ends its park; only a helper that
// stayed parked until the test signalled it can print it.
const helloTerminated = "hello helper received SIGTERM"

// parkUntilSIGTERM returns a channel that receives on SIGTERM, registered before it returns. A
// helper parks on it, never on a bare select{}: the child runs with no test timeout, so a process
// whose every goroutine blocks is killed by the runtime ("all goroutines are asleep", exit 2).
func parkUntilSIGTERM() <-chan os.Signal {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM)
	return c
}

// block parks the helper until SIGTERM, or until a signal it does not catch ends it.
func block() { <-parkUntilSIGTERM() }

// checkpoints reads one request per line from the FIFO named by HELPER_REQUESTS and, for request
// n, writes checkpoint file n into HELPER_DIR; it blocks reading, so it makes progress only when the
// test asks and the process runs.
func checkpoints() {
	dir := os.Getenv("HELPER_DIR")
	f, err := os.Open(os.Getenv("HELPER_REQUESTS"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		tmp := filepath.Join(dir, ".tmp")
		if err := os.WriteFile(tmp, []byte(sc.Text()), 0o644); err != nil {
			os.Exit(3)
		}
		if err := os.Rename(tmp, filepath.Join(dir, "checkpoint-"+strconv.Itoa(n))); err != nil {
			os.Exit(4)
		}
	}
	block()
}

// session is one checkpoints helper and the write end of its request FIFO.
type session struct {
	p   *Process
	dir string
	req *os.File
}

func startCheckpoints(t *testing.T) *session {
	t.Helper()
	dir := t.TempDir()
	fifo := filepath.Join(dir, "requests")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := Start(t, "checkpoints", "HELPER_DIR="+dir, "HELPER_REQUESTS="+fifo)
	if err != nil {
		t.Fatal(err)
	}
	// Read-write, so the open never blocks on a helper that failed to start; only the helper reads.
	req, err := os.OpenFile(fifo, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = req.Close() })
	return &session{p: p, dir: dir, req: req}
}

func (s *session) request(t *testing.T, n int) {
	t.Helper()
	if _, err := fmt.Fprintf(s.req, "%d\n", n); err != nil {
		t.Fatal(err)
	}
}

func (s *session) count() int {
	m, _ := filepath.Glob(filepath.Join(s.dir, "checkpoint-*"))
	return len(m)
}

func (s *session) awaitCount(t *testing.T, want int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), failureBound)
	defer cancel()
	if _, err := probe.Await(ctx, func(context.Context) (int, error) { return s.count(), nil },
		func(n int) bool { return n >= want }); err != nil {
		t.Fatalf("checkpoint %d never arrived: %v", want, err)
	}
}

// (a) Without its marker the helper is a no-op: Helper returns and runs nothing.
func TestHelperIsANoOpWithoutItsMarker(t *testing.T) {
	for _, marker := range []string{"", "another-helper"} {
		t.Setenv(markerEnv, marker)
		ran := false
		Helper("checkpoints", func() { ran = true })
		if ran {
			t.Fatalf("Helper ran with marker %q", marker)
		}
	}
}

// (b) A started helper runs in its own process group, and its output goes under the test's
// temporary directory.
func TestStartedHelperHasItsOwnGroupAndCapturedOutput(t *testing.T) {
	want := filepath.Dir(t.TempDir()) // t.TempDir's siblings share this parent
	p, err := Start(t, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if !p.Alive() {
		stderr, _ := os.ReadFile(p.stderr)
		t.Fatalf("helper %d exited before its group was read; stderr:\n%s", p.pid, stderr)
	}
	out, err := exec.Command("ps", "-o", "pgid=", "-p", strconv.Itoa(p.pid)).Output()
	if err != nil {
		t.Fatalf("ps pgid: %v", err)
	}
	if pgid, _ := strconv.Atoi(strings.TrimSpace(string(out))); pgid != p.pid || pgid == syscall.Getpgrp() {
		t.Fatalf("helper pgid %d, pid %d, test's group %d: want its own group", pgid, p.pid, syscall.Getpgrp())
	}
	for stream, file := range map[string]string{"hello from the helper": p.stdout, "hello on stderr": p.stderr} {
		if !strings.HasPrefix(file, want+string(filepath.Separator)) {
			t.Fatalf("output file %s is not under %s", file, want)
		}
		ctx, cancel := context.WithTimeout(t.Context(), failureBound)
		_, err := probe.Await(ctx, func(context.Context) (string, error) {
			data, err := os.ReadFile(file)
			return string(data), err
		}, func(s string) bool { return strings.Contains(s, stream) })
		cancel()
		if err != nil {
			t.Fatalf("%q never reached %s: %v", stream, file, err)
		}
	}
	// The helper stayed parked until now: it ends on SIGTERM, cleanly, and says so.
	if err := p.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("SIGTERM: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), failureBound)
	defer cancel()
	status, err := p.Wait(ctx)
	if err != nil {
		t.Fatalf("wait after SIGTERM: %v", err)
	}
	data, _ := os.ReadFile(p.stdout)
	if status != (ExitStatus{}) || !strings.Contains(string(data), helloTerminated) {
		stderr, _ := os.ReadFile(p.stderr)
		t.Fatalf("helper status %+v, stdout %q, stderr %q: want exit 0 after printing %q",
			status, data, stderr, helloTerminated)
	}

}

// failedTB is a test that reports itself failed when failed is set, and records what is logged to
// it. Its cleanups run on the embedded test.
type failedTB struct {
	testing.TB
	failed bool
	logs   []string
}

func (f *failedTB) Failed() bool { return f.failed || f.TB.Failed() }

func (f *failedTB) Logf(format string, args ...any) {
	f.logs = append(f.logs, fmt.Sprintf(format, args...))
}

// process-host › "Helper is started": when its test has failed by the time the cleanup reaps the
// child, the child's stdout and stderr are written to the test log; a passing test's log does not
// carry them. The subtest's cleanup has run when t.Run returns.
func TestFailedTestLogsTheHelperOutput(t *testing.T) {
	for _, failed := range []bool{true, false} {
		tb := &failedTB{failed: failed}
		t.Run(fmt.Sprintf("failed=%t", failed), func(t *testing.T) {
			tb.TB = t
			p, err := Start(tb, "hello")
			if err != nil {
				t.Fatal(err)
			}
			type seen struct {
				out   string
				alive bool
			}
			for stream, file := range map[string]string{"hello from the helper": p.stdout, "hello on stderr": p.stderr} {
				ctx, cancel := context.WithTimeout(t.Context(), failureBound)
				got, err := probe.Await(ctx, func(context.Context) (seen, error) {
					alive := p.Alive() // read before the file: output written before an exit is still seen
					data, err := os.ReadFile(file)
					return seen{string(data), alive}, err
				}, func(s seen) bool { return strings.Contains(s.out, stream) || !s.alive })
				cancel()
				if err == nil && !strings.Contains(got.out, stream) {
					stderr, _ := os.ReadFile(p.stderr)
					t.Fatalf("helper %d exited before %q reached %s; stderr:\n%s", p.pid, stream, file, stderr)
				}
				if err != nil {
					t.Fatalf("%q never reached %s: %v", stream, file, err)
				}
			}
		})
		logged := strings.Join(tb.logs, "\n")
		for _, want := range []string{"hello from the helper", "hello on stderr"} {
			if strings.Contains(logged, want) != failed {
				t.Errorf("failed=%t: test log %q, want %q in it only when the test failed", failed, logged, want)
			}
		}
	}
}

// (c) A kill between two checkpoints leaves the first checkpoint's file and not the second's, and
// Wait returns the killed status within its bound. The second request is already queued when the
// kill lands: the helper is paused, so only the kill decides that checkpoint 2 never happens.
func TestKillBetweenCheckpoints(t *testing.T) {
	s := startCheckpoints(t)
	s.request(t, 1)
	s.awaitCount(t, 1)
	if err := s.p.Pause(); err != nil {
		t.Fatal(err)
	}
	awaitState(t, s.p, func(st string) bool { return strings.HasPrefix(st, "T") })
	s.request(t, 2)
	if err := s.p.Kill(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), failureBound)
	defer cancel()
	st, err := s.p.Wait(ctx)
	if err != nil {
		t.Fatalf("Wait after Kill: %v", err)
	}
	if st.Signal != syscall.SIGKILL || st.Code != -1 {
		t.Fatalf("exit status %+v, want killed by SIGKILL", st)
	}
	if s.p.Alive() {
		t.Fatal("Alive after Wait returned its status")
	}
	_, err1 := os.Stat(filepath.Join(s.dir, "checkpoint-1"))
	_, err2 := os.Stat(filepath.Join(s.dir, "checkpoint-2"))
	if err1 != nil || !errors.Is(err2, os.ErrNotExist) {
		t.Fatalf("checkpoint 1: %v; checkpoint 2: %v; want the first only", err1, err2)
	}
}

// Once the child is reaped its group id may belong to another process, so no signal is sent:
// Kill is a no-op and Signal, Pause and Resume refuse.
func TestNoSignalAfterReap(t *testing.T) {
	s := startCheckpoints(t)
	if err := s.p.Kill(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), failureBound)
	defer cancel()
	if _, err := s.p.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	orig := signalGroup
	calls := 0
	signalGroup = func(pgid int, sig syscall.Signal) error { calls++; return orig(pgid, sig) }
	t.Cleanup(func() { signalGroup = orig })
	if err := s.p.Kill(); err != nil {
		t.Fatalf("Kill after reap = %v, want nil", err)
	}
	for name, err := range map[string]error{"Signal": s.p.Signal(syscall.SIGTERM), "Pause": s.p.Pause(), "Resume": s.p.Resume()} {
		if err == nil {
			t.Errorf("%s after reap returned nil", name)
		}
	}
	if calls != 0 {
		t.Fatalf("%d signal(s) sent to a reaped helper's group", calls)
	}
}

// Wait whose context ends first returns the context's error and says what it last observed.
func TestWaitBoundedByItsContext(t *testing.T) {
	s := startCheckpoints(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.p.Wait(ctx); !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "running") {
		t.Fatalf("Wait = %v, want context.Canceled naming the observed state", err)
	}
	var nilCtx context.Context
	if _, err := s.p.Wait(nilCtx); err == nil {
		t.Fatal("Wait accepted a nil context")
	}
}

// (d) After Pause the process is observed stopped; a request written while it is stopped adds no
// checkpoint; after Resume the checkpoint arrives.
func TestPauseStopsProgress(t *testing.T) {
	s := startCheckpoints(t)
	s.request(t, 1)
	s.awaitCount(t, 1)
	if err := s.p.Pause(); err != nil {
		t.Fatal(err)
	}
	awaitState(t, s.p, func(st string) bool { return strings.HasPrefix(st, "T") })
	s.request(t, 2)
	n := s.count()
	if st, err := psState(s.p.pid); err != nil || !strings.HasPrefix(st, "T") {
		t.Fatalf("state after the request: %q %v; the count %d was not read while stopped", st, err, n)
	}
	if n != 1 {
		t.Fatalf("checkpoints while stopped: %d, want 1", n)
	}
	if err := s.p.Resume(); err != nil {
		t.Fatal(err)
	}
	s.awaitCount(t, 2)
}

func awaitState(t *testing.T, p *Process, done func(string) bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), failureBound)
	defer cancel()
	if _, err := probe.Await(ctx, func(context.Context) (string, error) { return psState(p.pid) }, done); err != nil {
		t.Fatalf("process state never matched: %v", err)
	}
}

// startIdentity is ps's start time for pid: a pid reused by a later process has another. lstart
// has one-second resolution, so a pid reused within the same second reads as the same identity;
// that makes the check fail loudly, never pass wrongly.
func startIdentity(pid int) (string, error) {
	out, err := exec.Command("ps", "-o", "lstart=", "-p", strconv.Itoa(pid)).Output()
	return strings.TrimLeft(strings.TrimSuffix(string(out), "\n"), " \t"), err
}

// (e) A test that returns with its helper running leaves no process behind: the pid either names
// no process or a process with another start identity, so a reused pid cannot pass for the helper.
func TestNoHelperOutlivesItsTest(t *testing.T) {
	var pid int
	var identity string
	var p *Process
	t.Run("leaves its helper running", func(t *testing.T) {
		s := startCheckpoints(t)
		s.request(t, 1)
		s.awaitCount(t, 1)
		if err := s.p.Pause(); err != nil { // a stopped helper must be reaped too
			t.Fatal(err)
		}
		p, pid = s.p, s.p.pid
		var err error
		if identity, err = startIdentity(pid); err != nil || identity == "" {
			t.Fatalf("start identity of %d: %q %v", pid, identity, err)
		}
	})
	if p.Alive() {
		t.Fatal("helper alive after its test's cleanup")
	}
	if now, err := startIdentity(pid); err == nil && now == identity {
		t.Fatalf("pid %d still runs the helper started %s", pid, identity)
	}
	if err := syscall.Kill(-pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("process group %d still exists: %v", pid, err)
	}
}

// helperExitReason is the text a helper writes to its stderr when it exits because its test process
// has ended.
const helperExitReason = "test process ended"

// helperParent is a test process inside a helper: it starts a park helper (the grandchild), writes
// the grandchild's pid, start identity and stderr file to HELPER_DIR/grandchild, and parks until
// SIGTERM. A signal it does not catch ends it there; on SIGTERM it ends as HELPER_END says. Neither
// way runs its cleanups, so nothing it did ends the grandchild.
func helperParent(t *testing.T) {
	terminated := parkUntilSIGTERM()
	g, err := Start(t, "park")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	identity, err := startIdentity(g.pid)
	if err != nil || identity == "" {
		fmt.Fprintf(os.Stderr, "start identity of %d: %q %v\n", g.pid, identity, err)
		os.Exit(2)
	}
	record := filepath.Join(os.Getenv("HELPER_DIR"), "grandchild")
	if err := os.WriteFile(record+".tmp", fmt.Appendf(nil, "%d\n%s\n%s\n", g.pid, identity, g.stderr), 0o644); err != nil {
		os.Exit(3)
	}
	if err := os.Rename(record+".tmp", record); err != nil {
		os.Exit(4)
	}
	<-terminated
	switch os.Getenv("HELPER_END") {
	case "returned":
		return // Helper then exits through os.Exit(0)
	case "panicked":
		// Off the test goroutine, as testing's -timeout alarm panics: no cleanup runs.
		go func() { panic("helper-parent panics on a goroutine of its own") }()
		block()
	}
}

// grandchild is what helperParent recorded about the helper it started.
type grandchild struct {
	pid      int
	identity string
	stderr   string
}

func readGrandchild(t *testing.T, dir string) grandchild {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), failureBound)
	defer cancel()
	lines, err := probe.Await(ctx, func(context.Context) ([]string, error) {
		data, err := os.ReadFile(filepath.Join(dir, "grandchild"))
		return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n"), err
	}, func([]string) bool { return true })
	if err != nil || len(lines) != 3 {
		t.Fatalf("grandchild record never arrived: %q %v", lines, err)
	}
	pid, err := strconv.Atoi(lines[0])
	if err != nil {
		t.Fatalf("grandchild pid %q: %v", lines[0], err)
	}
	return grandchild{pid: pid, identity: lines[1], stderr: lines[2]}
}

// running reports whether g's pid still names the process helperParent started.
func (g grandchild) running() bool {
	now, err := startIdentity(g.pid)
	return err == nil && now == g.identity
}

// process-host › "A helper ends when its test process ends": a helper started by a test process that
// ends without running its cleanups exits on its own and writes why to its stderr. The test process
// is itself a helper (the helper-parent), so the outer test can end it in each such way; its
// TMPDIR is the outer test's directory, so what it leaves on disk goes with the outer cleanup.
func TestHelperEndsWhenItsTestProcessEnds(t *testing.T) {
	for _, row := range []struct {
		name string
		end  func(*Process) error
		want ExitStatus // how the helper-parent ended
	}{
		{"killed", (*Process).Kill, ExitStatus{Code: -1, Signal: syscall.SIGKILL}},
		{"interrupted", func(p *Process) error { return p.Signal(syscall.SIGINT) }, ExitStatus{Code: -1, Signal: syscall.SIGINT}},
		{"returned", func(p *Process) error { return p.Signal(syscall.SIGTERM) }, ExitStatus{Code: 0}},
		{"panicked", func(p *Process) error { return p.Signal(syscall.SIGTERM) }, ExitStatus{Code: 2}},
	} {
		t.Run(row.name, func(t *testing.T) {
			dir := t.TempDir()
			parent, err := Start(t, "parent", "TMPDIR="+dir, "HELPER_DIR="+dir, "HELPER_END="+row.name)
			if err != nil {
				t.Fatal(err)
			}
			g := readGrandchild(t, dir)
			t.Cleanup(func() {
				if !g.running() {
					return
				}
				// The grandchild leads a process group of its own.
				if err := syscall.Kill(-g.pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
					t.Errorf("cleanup kill of grandchild %d: %v", g.pid, err)
				}
				ctx, cancel := context.WithTimeout(context.Background(), failureBound)
				defer cancel()
				if _, err := probe.Await(ctx, func(context.Context) (bool, error) { return g.running(), nil },
					func(r bool) bool { return !r }); err != nil {
					t.Errorf("grandchild %d still running after the cleanup's SIGKILL: %v", g.pid, err)
				}
			})
			if !g.running() {
				t.Fatalf("grandchild %d (started %s) not running before the helper-parent ended", g.pid, g.identity)
			}

			if err := row.end(parent); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), failureBound)
			defer cancel()
			if status, err := parent.Wait(ctx); err != nil || status != row.want {
				t.Fatalf("helper-parent ended %+v %v, want %+v", status, err, row.want)
			}

			ctx, cancel = context.WithTimeout(t.Context(), failureBound)
			defer cancel()
			if _, err := probe.Await(ctx, func(context.Context) (bool, error) { return g.running(), nil },
				func(r bool) bool { return !r }); err != nil {
				t.Errorf("grandchild %d (started %s) still running after its test process ended: %v", g.pid, g.identity, err)
			}
			if data, err := os.ReadFile(g.stderr); err != nil || !strings.Contains(string(data), helperExitReason) {
				t.Errorf("grandchild stderr %q %v: want it to name the reason %q", data, err, helperExitReason)
			}
		})
	}
}

func TestStartRefusesAnEmptyName(t *testing.T) {
	if _, err := Start(t, ""); err == nil {
		t.Fatal("Start accepted an empty helper name")
	}
}
