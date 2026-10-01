package natsfixture

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"os"
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
	owner := fmt.Sprintf("host=%s\npid=%d\nstarted=1\nidentity=x\ntoken=%s\ncommand=semengine /x/scripts/test-integration.sh\n",
		host, os.Getpid(), ownerToken)
	if err := os.WriteFile(filepath.Join(lock, "owner"), []byte(owner), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SEMENGINE_DOCKER_ADMISSION_TOKEN", envToken)
	t.Setenv("SEMENGINE_DOCKER_ADMISSION_LOCK_DIR", lock)
	t.Setenv("SEMENGINE_EVIDENCE_DIR", t.TempDir())
	t.Setenv("SEMENGINE_NATS_IMAGE", "nats"+":2.14.7-alpine@sha256:"+strings.Repeat("0", 64))
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

// deadPID returns the pid of a process that has exited and been reaped.
func deadPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	return cmd.Process.Pid
}

// The token must be in a live owner's file: an owner file the runner left behind when it died (a
// SIGKILL skips its EXIT trap) admits nothing, because the lock it records is no longer held.
func TestAdmissionRequiresALiveOwner(t *testing.T) {
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, host, pid, want string
	}{
		{"owner process has exited", host, strconv.Itoa(deadPID(t)), "not live"},
		{"owner on another host", host + "-elsewhere", strconv.Itoa(os.Getpid()), "another host"},
		{"owner pid unreadable", host, "x", "pid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plantLock(t, "tok", "tok")
			owner := fmt.Sprintf("host=%s\npid=%s\nstarted=1\nidentity=x\ntoken=tok\ncommand=semengine /x/scripts/test-integration.sh\n", tc.host, tc.pid)
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
