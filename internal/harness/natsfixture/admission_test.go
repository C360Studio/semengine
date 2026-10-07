package natsfixture

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"syscall"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// admissionEnv lists every variable the runner exports to the fixture. Tests here set them
// explicitly with t.Setenv, so they cannot run in parallel and do not depend on how they were run.
var admissionEnv = []string{
	"SEMENGINE_DOCKER_ADMISSION_TOKEN", "SEMENGINE_DOCKER_ADMISSION_LOCK_DIR", "SEMENGINE_EVIDENCE_DIR", "SEMENGINE_NATS_IMAGE",
}

// plantLock writes a lock owner record holding token and points the fixture's environment at it.
func plantLock(t *testing.T, ownerToken, envToken string) {
	t.Helper()
	lock := filepath.Join(t.TempDir(), "lock")
	if err := os.Mkdir(lock, 0o755); err != nil {
		t.Fatal(err)
	}
	// The owner is this test process on this host: live, as the runner is while its tests run.
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	owner := fmt.Sprintf("host=%s\npid=%d\nstarted=1\nidentity=%s\ntoken=%s\ncommand=semengine /x/scripts/test-integration.sh\n",
		host, os.Getpid(), runnerIdentity(t, os.Getpid()), ownerToken)
	if err := os.WriteFile(filepath.Join(lock, "owner"), []byte(owner), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SEMENGINE_DOCKER_ADMISSION_TOKEN", envToken)
	t.Setenv("SEMENGINE_DOCKER_ADMISSION_LOCK_DIR", lock)
	t.Setenv("SEMENGINE_EVIDENCE_DIR", t.TempDir())
	t.Setenv("SEMENGINE_NATS_IMAGE", "nats"+":2.14.7-alpine@sha256:"+strings.Repeat("0", 64))
}

// runnerIdentity reads pid's start time as the runner records its own: scripts/test-integration.sh's
// command text, run by bash, whose command substitution removes the trailing newline and keeps any
// trailing blanks. It shares no code with admission's read, which must equal it.
func runnerIdentity(t *testing.T, pid int) string {
	t.Helper()
	out, err := exec.CommandContext(t.Context(), "bash", "-c",
		`printf %s "$(ps -o lstart= -p "$1" 2>/dev/null | sed "s/^[[:space:]]*//")"`, "_", strconv.Itoa(pid)).Output()
	if err != nil {
		t.Fatalf("read the start time of pid %d: %v", pid, err)
	}
	if len(out) == 0 {
		t.Fatalf("ps printed no start time for pid %d: these tests need one for their own process", pid)
	}
	return string(out)
}

// S1-9: Start refuses without admission and makes no Docker or NATS call. The environment
// variable alone is never trusted: its token must be in the live owner file.
func TestS1_9AdmissionRefusal(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T)
		want  string
	}{
		{"no runner environment", func(t *testing.T) {
			for _, v := range admissionEnv {
				t.Setenv(v, "")
			}
		}, "SEMENGINE_DOCKER_ADMISSION_TOKEN"},
		{"token absent from the owner file", func(t *testing.T) { plantLock(t, "runner-token", "forged-token") }, "not in"},
		{"token is a prefix of the owner's", func(t *testing.T) { plantLock(t, "runner-token-2", "runner-token") }, "not in"},
		{"no owner file", func(t *testing.T) {
			plantLock(t, "tok", "tok")
			if err := os.Remove(filepath.Join(os.Getenv("SEMENGINE_DOCKER_ADMISSION_LOCK_DIR"), "owner")); err != nil {
				t.Fatal(err)
			}
		}, "owner"},
		{"no evidence directory", func(t *testing.T) {
			plantLock(t, "tok", "tok")
			t.Setenv("SEMENGINE_EVIDENCE_DIR", "")
		}, "SEMENGINE_EVIDENCE_DIR"},
		{"no image", func(t *testing.T) {
			plantLock(t, "tok", "tok")
			t.Setenv("SEMENGINE_NATS_IMAGE", "")
		}, "SEMENGINE_NATS_IMAGE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.setup(t)
			f := New(t)
			err := f.Start(t.Context())
			if !errors.Is(err, ErrNotAdmitted) {
				t.Fatalf("Start = %v, want ErrNotAdmitted", err)
			}
			for _, want := range []string{tc.want, "task test:integration -- "} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not mention %q", err, want)
				}
			}
			if calls := f.totalCalls(); calls != 0 {
				t.Errorf("refused Start made %d dependency call(s): %v", calls, f.callCounts())
			}
			if rem := f.remaining(); len(rem) != 0 {
				t.Errorf("refused Start retained %v", rem)
			}
			// A refusal before acting consumes nothing: Stop is a no-op, and the fixture can
			// still be started once admitted.
			if err := f.Stop(t.Context()); err != nil {
				t.Errorf("Stop after refusal = %v", err)
			}
		})
	}
}

// noDocker replaces every dependency with one that fails the test: these tests prove what the
// fixture does before, or instead of, touching Docker.
func noDocker(t *testing.T, f *Fixture) {
	t.Helper()
	f.deps.start = func(context.Context, testcontainers.GenericContainerRequest) (testcontainers.Container, error) {
		t.Error("unexpected Docker start")
		return nil, errors.New("unexpected Docker start")
	}
}

// absentPID returns a pid that no process has and none can be given: the largest pid_t, far above
// the highest pid either kernel the tests run on hands out (Linux caps pid_max at 2^22, macOS at
// 99999). ps prints no start time for it, which is how ownerLive sees an owner that has exited and
// been reaped. An exited child's pid would do only until the kernel gave it to a new process: that
// process would be refused too, but for its start time, which is another case below.
func absentPID(t *testing.T) int {
	t.Helper()
	pid := math.MaxInt32
	// ESRCH is "no such process"; any other answer means the premise above does not hold here.
	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("kill(%d, 0) = %v, want ESRCH: no process may hold this pid", pid, err)
	}
	return pid
}

// The token must be in a live owner's file: an owner file the runner left behind when it died (a
// SIGKILL skips its EXIT trap) admits nothing, because the lock it records is no longer held. Nor
// does one whose pid another process now has: that process's start time is not the record's.
func TestAdmissionRequiresALiveOwner(t *testing.T) {
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	self := func(*testing.T) string { return strconv.Itoa(os.Getpid()) }
	// Each case below differs from a live owner (this host, this process, its start time) in one field.
	live := runnerIdentity(t, os.Getpid())
	for _, tc := range []struct {
		name, host string
		pid        func(t *testing.T) string // called inside the case, so a failed premise fails only that case
		identity   string
		want       string
	}{
		{"owner pid is not a running process", host, func(t *testing.T) string { return strconv.Itoa(absentPID(t)) }, live, "not live"},
		{"owner on another host", host + "-elsewhere", self, live, "another host"},
		{"owner pid unreadable", host, func(*testing.T) string { return "x" }, live, "pid"},
		{"owner pid names a process with another start time", host, self, "Mon Jan  1 00:00:00 1990", "Mon Jan  1 00:00:00 1990"},
		{"owner identity unknown", host, self, "unknown", "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plantLock(t, "tok", "tok")
			owner := fmt.Sprintf("host=%s\npid=%s\nstarted=1\nidentity=%s\ntoken=tok\ncommand=semengine /x/scripts/test-integration.sh\n",
				tc.host, tc.pid(t), tc.identity)
			if err := os.WriteFile(filepath.Join(os.Getenv("SEMENGINE_DOCKER_ADMISSION_LOCK_DIR"), "owner"), []byte(owner), 0o644); err != nil {
				t.Fatal(err)
			}
			f := New(t)
			noDocker(t, f)
			err := f.Start(t.Context())
			if !errors.Is(err, ErrNotAdmitted) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Start = %v, want ErrNotAdmitted naming %q", err, tc.want)
			}
		})
	}
}

// A ps that exits 0 and prints nothing shows no start time, so it proves nothing live, even against
// a record with no identity, whose missing value would otherwise equal the empty read.
func TestAdmissionRefusesAnEmptyStartTime(t *testing.T) {
	plantLock(t, "tok", "tok")
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	owner := fmt.Sprintf("host=%s\npid=%d\nstarted=1\ntoken=tok\ncommand=semengine /x/scripts/test-integration.sh\n", host, os.Getpid())
	if err := os.WriteFile(filepath.Join(os.Getenv("SEMENGINE_DOCKER_ADMISSION_LOCK_DIR"), "owner"), []byte(owner), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "ps"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	f := New(t)
	noDocker(t, f)
	if err := f.Start(t.Context()); !errors.Is(err, ErrNotAdmitted) || !strings.Contains(err.Error(), "no start time") {
		t.Fatalf("Start = %v, want ErrNotAdmitted naming no start time", err)
	}
}

// A live owner admits: plantLock's record names this process with the start time the runner's own
// command reads for it. Admission reads under its caller's context, so a caller already cancelled
// is told so rather than refused.
func TestAdmissionAdmitsALiveOwner(t *testing.T) {
	plantLock(t, "tok", "tok")
	adm, err := admit(t.Context())
	if err != nil {
		t.Fatalf("admit = %v, want the live owner admitted", err)
	}
	if want := os.Getenv("SEMENGINE_EVIDENCE_DIR"); adm.evidenceDir != want {
		t.Errorf("evidence directory = %q, want %q", adm.evidenceDir, want)
	}
	if want := os.Getenv("SEMENGINE_NATS_IMAGE"); adm.image != want {
		t.Errorf("image = %q, want %q", adm.image, want)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := admit(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("admit with a cancelled context = %v, want context.Canceled", err)
	}
}

// M2: Stop waits for the fixture's operation slot under its own context. A Start parked in a
// Docker call does not make a bounded Stop unbounded.
func TestStopHonoursItsContextWhileStartHoldsTheFixture(t *testing.T) {
	plantLock(t, "tok", "tok")
	f := New(t)
	entered, release := make(chan struct{}), make(chan struct{})
	f.deps.start = func(context.Context, testcontainers.GenericContainerRequest) (testcontainers.Container, error) {
		close(entered)
		<-release
		return nil, errors.New("start released by the test")
	}
	started := make(chan error, 1)
	go func() { started <- f.Start(t.Context()) }()
	<-entered
	defer func() {
		close(release)
		<-started
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	stopped := make(chan error, 1)
	go func() { stopped <- f.Stop(ctx) }()
	// The bound below is a watchdog for the failure, not a synchronisation: a correct Stop returns
	// as soon as its 100ms context ends.
	watchdog := time.NewTimer(10 * time.Second)
	defer watchdog.Stop()
	select {
	case err := <-stopped:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Stop = %v, want context.DeadlineExceeded", err)
		}
	case <-watchdog.C:
		t.Fatal("Stop ignored its context while Start held the fixture")
	}
}

// M5: a Start whose context is cancelled while a dependency fails for its own reason still
// reports the cancellation to errors.Is: a caller asking "was I cancelled?" gets the truth.
func TestCancelledStartIsRecognisable(t *testing.T) {
	plantLock(t, "tok", "tok")
	f := New(t)
	ctx, cancel := context.WithCancel(t.Context())
	f.deps.start = func(context.Context, testcontainers.GenericContainerRequest) (testcontainers.Container, error) {
		cancel()
		return nil, errors.New("daemon: request aborted")
	}
	err := f.Start(ctx)
	var fe *Error
	if !errors.As(err, &fe) || fe.Phase != PhaseStart {
		t.Fatalf("Start = %v, want a start-phase *Error", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("errors.Is(%v, context.Canceled) = false", err)
	}
}
