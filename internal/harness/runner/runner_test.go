package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/c360studio/semengine/internal/harness/probe"
)

// semstreamsOwnerKeys is the owner-record format of SemStreams scripts/run-integration-tests.sh
// at 5457b3458936f668b71d2fea061f67f8d7d01e67, lines 213-220, recorded here as the independent
// oracle: this runner writes these keys, in this order, followed by identity_utc, a key
// SemStreams' runner ignores, so each runner can judge the other's lock.
var semstreamsOwnerKeys = []string{"host", "pid", "started", "identity", "token", "command"}

// fakeDocker records every invocation and answers the handful of subcommands the runner uses.
// FAKE_IMAGE_CACHED=0 makes the image absent, FAKE_PULL_FAILS=1 fails the pull, and
// FAKE_PS_SURVIVORS=N makes `docker ps` report one container for the first N calls.
// FAKE_PULL_MODE=hang makes the pull record its pid and block, as a slow registry does; pull.pid
// appears only once it holds the pid.
const fakeDocker = `#!/usr/bin/env bash
echo "$*" >> "$FAKE_DIR/docker.log"
case "$1 ${2:-}" in
  "info "*) echo "fake info" ;;
  "version "*) echo "fake version" ;;
  "context show") echo fake-context ;;
  "context inspect") echo unix:///fake.sock ;;
  "image inspect") [ "${FAKE_IMAGE_CACHED:-1}" = 1 ] || exit 1 ;;
  "pull "*)
    [ "${FAKE_PULL_FAILS:-0}" = 1 ] && { echo "fake pull: manifest unknown" >&2; exit 1; }
    # The shell creates a redirect's file before it writes to it, and the test reads pull.pid as
    # soon as it exists; writing a temporary name and renaming means pull.pid never exists empty.
    if [ "${FAKE_PULL_MODE:-}" = hang ]; then
      echo "$$" > "$FAKE_DIR/pull.pid.tmp" && mv "$FAKE_DIR/pull.pid.tmp" "$FAKE_DIR/pull.pid"
      exec sleep 30
    fi ;;
  "ps "*)
    n=$(cat "$FAKE_DIR/ps.count" 2>/dev/null || echo 0); n=$((n + 1)); echo "$n" > "$FAKE_DIR/ps.count"
    [ "$n" -le "${FAKE_PS_SURVIVORS:-0}" ] && echo "c0ffee00c0ffee00" ;;
esac
exit 0
`

// fakeGo stands in for go test. It records argv, the admission environment, and the owner file
// it can see; writes a testcontainers session id where the fixture would; and in FAKE_GO_MODE=hang
// spawns a grandchild and waits, recording what it sees when TERM arrives.
const fakeGo = `#!/usr/bin/env bash
printf '%s\n' "$@" > "$FAKE_DIR/go.argv"
{
  echo "TESTCONTAINERS_RYUK_DISABLED=${TESTCONTAINERS_RYUK_DISABLED:-}"
  echo "SEMENGINE_NATS_IMAGE=${SEMENGINE_NATS_IMAGE:-}"
  echo "SEMENGINE_DOCKER_ADMISSION_TOKEN=${SEMENGINE_DOCKER_ADMISSION_TOKEN:-}"
  echo "SEMENGINE_DOCKER_ADMISSION_LOCK_DIR=${SEMENGINE_DOCKER_ADMISSION_LOCK_DIR:-}"
  echo "SEMENGINE_EVIDENCE_DIR=${SEMENGINE_EVIDENCE_DIR:-}"
} > "$FAKE_DIR/go.env"
cp "$SEMENGINE_DOCKER_ADMISSION_LOCK_DIR/owner" "$FAKE_DIR/go.owner"
echo org.testcontainers.sessionId=fake-session >> "$SEMENGINE_EVIDENCE_DIR/testcontainers-session"
if [ "${FAKE_GO_MODE:-ok}" = hang ]; then
  lock="$SEMENGINE_DOCKER_ADMISSION_LOCK_DIR"
  on_term() {
    echo term >> "$FAKE_DIR/go.signals"
    # A slow shutdown: the runner must still hold the lock after it, until the group is reaped.
    sleep 1
    [ -d "$lock" ] && echo lock-held-at-exit >> "$FAKE_DIR/go.signals"
    exit 143
  }
  trap on_term TERM
  # The grandchild stands in for a test binary: it records TERM itself, so a runner that
  # signals only the go pid and later KILLs the group is told apart from one that forwards.
  bash -c 'trap "echo grandchild-term >> \"$FAKE_DIR/go.signals\"; exit 143" TERM; sleep 300 & wait' &
  echo "$!" > "$FAKE_DIR/grandchild.pid"
  touch "$FAKE_DIR/go.ready"
  wait
fi
if [ "${FAKE_GO_MODE:-ok}" = ignore-term ]; then
  # A test binary that ignores TERM: only the runner's KILL escalation ends it.
  trap '' TERM
  bash -c 'trap "" TERM; sleep 300 & wait' &
  echo "$!" > "$FAKE_DIR/grandchild.pid"
  touch "$FAKE_DIR/go.ready"
  wait
fi
if [ "${FAKE_GO_MODE:-ok}" = interrupt-output ]; then
  # go test prints its failure summary after the interrupt; that output must reach the log.
  # The sleep is an & job, so it ignores INT; the handler ends it, as go test ends its binaries.
  on_int() {
    echo "--- FAIL: shutdown summary written after the interrupt"
    kill "$sleeper" 2>/dev/null
    exit 130
  }
  trap on_int INT
  sleep 300 &
  sleeper=$!
  touch "$FAKE_DIR/go.ready"
  wait
fi
if [ "${FAKE_GO_MODE:-ok}" = escape ]; then
  # A process that leaves go test's process group but keeps its stdout, the log's FIFO.
  python3 -c 'import os, time; os.setpgrp(); time.sleep(60)' &
  echo "$!" > "$FAKE_DIR/escaped.pid"
fi
if [ "${FAKE_GO_MODE:-ok}" = steal ]; then
  sed -i.bak 's/^token=.*/token=someone-else/' "$SEMENGINE_DOCKER_ADMISSION_LOCK_DIR/owner"
  rm -f "$SEMENGINE_DOCKER_ADMISSION_LOCK_DIR/owner.bak"
fi
echo "ok  	example.com/fake	0.123s"
exit "${FAKE_GO_STATUS:-0}"
`

type harness struct {
	root, fake, lock, evidence string
	env                        []string
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the runner package")
		}
		dir = parent
	}
}

// newHarness builds the fake toolchain and an environment scrubbed of every SEMENGINE_* and
// SEMSTREAMS_* variable, then adds only what the case sets.
func newHarness(t *testing.T, extra ...string) *harness {
	t.Helper()
	tmp := t.TempDir()
	h := &harness{
		root:     repoRoot(t),
		fake:     filepath.Join(tmp, "bin"),
		lock:     filepath.Join(tmp, "lock"),
		evidence: filepath.Join(tmp, "evidence"),
	}
	if err := os.MkdirAll(h.fake, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"docker": fakeDocker, "go": fakeGo} {
		if err := os.WriteFile(filepath.Join(h.fake, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "SEMENGINE_") || strings.HasPrefix(kv, "SEMSTREAMS_") || strings.HasPrefix(kv, "FAKE_") {
			continue
		}
		if strings.HasPrefix(kv, "PATH=") {
			kv = "PATH=" + h.fake + ":" + strings.TrimPrefix(kv, "PATH=")
		}
		h.env = append(h.env, kv)
	}
	h.env = append(h.env,
		"FAKE_DIR="+h.fake,
		"SEMENGINE_DOCKER_ADMISSION_LOCK_DIR="+h.lock,
		"SEMENGINE_EVIDENCE_DIR="+h.evidence,
	)
	h.env = append(h.env, extra...)
	return h
}

func (h *harness) command(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "bash", append([]string{filepath.Join(h.root, "scripts", "test-integration.sh")}, args...)...)
	cmd.Env = h.env
	cmd.Dir = h.root
	return cmd
}

type result struct {
	status         int
	stdout, stderr string
}

func (h *harness) run(t *testing.T, args ...string) result {
	t.Helper()
	cmd := h.command(t.Context(), args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	status := 0
	if errors.As(err, &exitErr) {
		status = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("run runner: %v", err)
	}
	return result{status: status, stdout: stdout.String(), stderr: stderr.String()}
}

func (h *harness) read(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(h.fake, name))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return string(data)
}

func (h *harness) evidenceFile(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(h.evidence, name))
	if err != nil {
		t.Fatalf("evidence %s: %v", name, err)
	}
	return string(data)
}

// writeOwner plants a lock held by someone else, in the shared owner-record format: SemStreams'
// six keys, then each of extra as a line of its own (a SemEngine runner's identity_utc).
func (h *harness) writeOwner(t *testing.T, host string, pid int, identity string, extra ...string) {
	t.Helper()
	if err := os.Mkdir(h.lock, 0o755); err != nil {
		t.Fatal(err)
	}
	owner := fmt.Sprintf("host=%s\npid=%d\nstarted=%d\nidentity=%s\ntoken=foreign-token\ncommand=scripts/run-integration-tests.sh\n",
		host, pid, time.Now().Unix(), identity)
	for _, line := range extra {
		owner += line + "\n"
	}
	if err := os.WriteFile(filepath.Join(h.lock, "owner"), []byte(owner), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hostname(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("hostname").Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

// startIdentity is the owner identity exactly as both runners record it: ps's start time with
// leading blanks stripped (`sed 's/^[[:space:]]*//'`). macOS pads the column with trailing blanks,
// and they are part of the recorded value, so they are kept here too.
func startIdentity(t *testing.T, pid int) string {
	t.Helper()
	identity, err := psStartIdentity(pid)
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func psStartIdentity(pid int) (string, error) {
	out, err := exec.Command("ps", "-o", "lstart=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return "", fmt.Errorf("ps lstart %d: %w", pid, err)
	}
	return trimIdentity(out), nil
}

func trimIdentity(psOut []byte) string {
	return strings.TrimLeft(strings.TrimSuffix(string(psOut), "\n"), " \t")
}

// awaitLaterStartIdentity returns once a process started now gets a start identity other than
// identity. ps reports start times to the second, so this takes at most about a second; start
// times only move forward, so every process started afterwards differs from identity too. The
// budget only bounds a host whose ps never moves on.
func awaitLaterStartIdentity(ctx context.Context, identity string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_, err := probe.Await(ctx, func(ctx context.Context) (string, error) {
		// The shell asks ps about itself, so the identity is that of a process started just now.
		out, err := exec.CommandContext(ctx, "sh", "-c", "ps -o lstart= -p $$").Output()
		return trimIdentity(out), err
	}, func(now string) bool { return now != "" && now != identity })
	return err
}

// deadPIDAttempts bounds how many fresh children deadPID starts before it gives up. A reaped pid
// is reused only when the host cycles through its pid space between the child's exit and the
// check, so one retry is almost always enough; the bound keeps a pathological host from looping.
const deadPIDAttempts = 5

// deadPID returns the pid of a process that has exited and been reaped, and the start identity
// the runner would have recorded for it, read while it ran. The child is kept running until a
// process started now would get a different identity, so if the pid is reused after this check
// the new process's identity cannot equal the returned one, and the runner quarantines the owner
// on the identity mismatch instead of respecting it as live. If every pid it got was already
// reused, it fails the test with the pids it saw: a reused pid is reported, never hidden.
func deadPID(t *testing.T) (int, string) {
	t.Helper()
	var reused []int
	for range deadPIDAttempts {
		cmd := exec.Command("cat") // runs until its stdin closes
		stdin, err := cmd.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		pid := cmd.Process.Pid
		identity, err := psStartIdentity(pid)
		if err == nil {
			err = awaitLaterStartIdentity(t.Context(), identity)
		}
		_ = stdin.Close()
		if waitErr := cmd.Wait(); err == nil {
			err = waitErr
		}
		if err != nil {
			t.Fatalf("deadPID: child %d: %v", pid, err)
		}
		// ESRCH is "no such process"; any other answer, EPERM included, means the pid is in use.
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return pid, identity
		}
		reused = append(reused, pid)
	}
	t.Fatalf("deadPID: each of %d reaped children's pids was reused before the test could use it: %v", deadPIDAttempts, reused)
	return 0, ""
}

func pidAlive(pid int) bool { return syscall.Kill(pid, 0) == nil }

// R1: a live same-host owner refuses the run before any Docker call, and SEMSTREAMS_* variables
// (here a wait budget and a different lock path) are not read. Both records name this process
// with its real identity: SemStreams' six keys are judged by it, and a record whose identity_utc
// is unknown is judged by that, which is never stale.
func TestR1BusyLockRefusesBeforeDocker(t *testing.T) {
	t.Parallel()
	for name, extra := range map[string][]string{
		"live owner":           nil,
		"identity_utc unknown": {"identity_utc=unknown"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, "SEMSTREAMS_INTEGRATION_LOCK_WAIT_SECONDS=30", "SEMSTREAMS_INTEGRATION_LOCK_DIR=/nonexistent/semstreams.lock")
			h.writeOwner(t, hostname(t), os.Getpid(), startIdentity(t, os.Getpid()), extra...)
			r := h.run(t, "./internal/harness/natsfixture/")
			if r.status != 1 {
				t.Fatalf("status %d, want 1\nstdout:\n%s\nstderr:\n%s", r.status, r.stdout, r.stderr)
			}
			for _, want := range []string{"lock owner host=" + hostname(t), fmt.Sprintf("pid=%d", os.Getpid()), "command=scripts/run-integration-tests.sh"} {
				if !strings.Contains(r.stderr, want) {
					t.Errorf("stderr does not report the owner (%q):\n%s", want, r.stderr)
				}
			}
			if log := h.read(t, "docker.log"); log != "" {
				t.Errorf("docker was invoked while the lock was busy:\n%s", log)
			}
			if h.read(t, "go.argv") != "" {
				t.Error("go test ran while the lock was busy")
			}
			if owner, err := os.ReadFile(filepath.Join(h.lock, "owner")); err != nil || !strings.Contains(string(owner), "token=foreign-token") {
				t.Errorf("the foreign owner record was disturbed: %q, %v", owner, err)
			}
		})
	}
}

// R1 (foreign host): an owner on another host is never judged stale here, whatever its pid.
func TestR1ForeignHostOwnerIsRespected(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	pid, identity := deadPID(t)
	h.writeOwner(t, "some-other-host.invalid", pid, identity)
	r := h.run(t, "./internal/harness/natsfixture/")
	if r.status != 1 || !strings.Contains(r.stderr, "host=some-other-host.invalid") {
		t.Fatalf("status %d, want 1 naming the foreign owner\nstderr:\n%s", r.status, r.stderr)
	}
	if _, err := os.Stat(filepath.Join(h.lock, "owner")); err != nil {
		t.Errorf("the foreign lock was removed: %v", err)
	}
}

// T1 (#126): a second runner, started from a shell in another time zone and locale, reads the
// first runner, still running, as the live owner it is, and refuses rather than quarantine its
// lock. Each runner sets both, so the test does not depend on the host's own zone. Both locales
// are non-C: on macOS, whose ps prints the start time in the locale, a writer or a judge that
// reads it without LC_ALL=C records or reads text the other side does not. A Linux host with no
// such locale runs both in C, and only the time zones differ there.
func TestR1LiveOwnerInAnotherTimeZoneOrLocaleIsRespected(t *testing.T) {
	t.Parallel()
	a := newHarness(t, "FAKE_GO_MODE=hang", "TZ=EST5", "LC_ALL=en_GB.UTF-8")
	cmdA := a.command(t.Context(), "./internal/harness/natsfixture/")
	waitedA, stdoutA, stderrA := a.startRunner(t, cmdA, "go.ready")
	var errA error
	reapedA := false
	// A is stopped with TERM, its graceful path, on every path out of the test, while the test's
	// context (which would SIGKILL the runner alone and orphan its go test group) is still live.
	defer func() {
		if !reapedA {
			_ = cmdA.Process.Signal(syscall.SIGTERM)
			<-waitedA
		}
	}()
	// runningA reports whether A is still running: Wait has not returned and its pid takes a signal.
	runningA := func() bool {
		select {
		case errA = <-waitedA:
			reapedA = true
			return false
		default:
			return pidAlive(cmdA.Process.Pid)
		}
	}
	if !runningA() {
		t.Fatalf("runner A stopped running before runner B started (wait: %v)", errA)
	}
	ownerA, err := os.ReadFile(filepath.Join(a.lock, "owner"))
	if err != nil {
		t.Fatalf("runner A holds no owner record: %v", err)
	}
	// The record's mode is the one a new file gets under the umask runner A inherits from this
	// process, which a file os.Create makes here gets too.
	newFile, err := os.Create(filepath.Join(t.TempDir(), "new-file"))
	if err != nil {
		t.Fatal(err)
	}
	if err := newFile.Close(); err != nil {
		t.Fatal(err)
	}
	ownerInfo, err := os.Stat(filepath.Join(a.lock, "owner"))
	if err != nil {
		t.Fatal(err)
	}
	newInfo, err := os.Stat(newFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ownerInfo.Mode().Perm(), newInfo.Mode().Perm(); got != want {
		t.Errorf("owner record mode %v, want %v, the mode a new file gets under the runner's umask", got, want)
	}
	tokenA := ""
	for _, line := range strings.Split(string(ownerA), "\n") {
		if v, ok := strings.CutPrefix(line, "token="); ok {
			tokenA = v
		}
	}
	if tokenA == "" {
		t.Fatalf("runner A's owner record holds no token:\n%s", ownerA)
	}

	// B has its own fakes and evidence and the default FAKE_GO_MODE, and A's lock directory: the
	// later value in the environment wins (os/exec). No wait budget is set, so B decides at once.
	b := newHarness(t, "SEMENGINE_DOCKER_ADMISSION_LOCK_DIR="+a.lock, "TZ=JST-9", "LC_ALL=ja_JP.UTF-8")
	// A watchdog, not a synchronisation: a B that refuses exits at once and a B that acquires runs
	// to completion, so the deadline ends only a B that does neither.
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	cmdB := b.command(ctx, "./internal/harness/natsfixture/")
	var stdoutB, stderrB bytes.Buffer
	cmdB.Stdout, cmdB.Stderr = &stdoutB, &stderrB
	statusB := exitCode(cmdB.Run())
	if statusB != 1 || !strings.Contains(stderrB.String(), fmt.Sprintf("pid=%d ", cmdA.Process.Pid)) {
		t.Errorf("runner B status %d, want 1 naming runner A (pid %d) as the lock owner\nstdout:\n%s\nstderr:\n%s",
			statusB, cmdA.Process.Pid, stdoutB.String(), stderrB.String())
	}
	if owner, err := os.ReadFile(filepath.Join(a.lock, "owner")); err != nil || !strings.Contains(string(owner), "\ntoken="+tokenA+"\n") {
		t.Errorf("the owner record no longer holds runner A's token %q: %q, %v", tokenA, owner, err)
	}
	if !runningA() {
		t.Fatalf("runner A stopped running while runner B judged its lock (wait: %v)", errA)
	}
	if err := cmdA.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	errA, reapedA = <-waitedA, true
	if code := exitCode(errA); code != 143 {
		t.Errorf("runner A exit %d, want 143\nstdout:\n%s\nstderr:\n%s", code, stdoutA, stderrA)
	}
}

// publicationHook is sourced by runner A's bash through BASH_ENV. Its printf is the builtin,
// except that on the first call whose format is the owner record's command line, a call made
// inside publication, it writes publication.ready into A's fake directory and then blocks reading
// the FIFO named by FAKE_GATE. A failed read returns as a resumed one, so the hook never changes
// the script's flow, only when it continues.
const publicationHook = `printf() {
  builtin printf "$@" || return
  if [ "$1" = 'command=%s\n' ] && [ -z "${publication_paused:-}" ]; then
    publication_paused=1
    : > "$FAKE_DIR/publication.ready"
    read -r _ < "$FAKE_GATE" || true
  fi
}
`

// T2 (#126, D7): a runner that finds the lock while its owner is still publishing the record
// respects the owner. Runner A pauses inside publication, after its command= line and before its
// identity_utc line, as in Codex's probe (PR #127 comment 6059322575). Runner B, in another time
// zone, must not read a partly written record as SemStreams' six keys and quarantine A on a start
// time it reads differently. A then publishes its whole record, runs, and releases the lock,
// leaving nothing beside it.
func TestR1ContenderDuringPublicationRespectsTheOwner(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	hook, gatePath := filepath.Join(dir, "publication-hook.bash"), filepath.Join(dir, "gate")
	if err := os.WriteFile(hook, []byte(publicationHook), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(gatePath, 0o600); err != nil {
		t.Fatal(err)
	}
	// Opened for reading and writing, so neither this open nor A's open for reading waits for the
	// other side. It stays open until A is reaped, so a resume line written before A opens the
	// FIFO is still there when A reads.
	gate, err := os.OpenFile(gatePath, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer gate.Close()

	a := newHarness(t, "TZ=EST5", "LC_ALL=C", "BASH_ENV="+hook, "FAKE_GATE="+gatePath)
	// A watchdog, not a synchronisation: the deadline ends an A that the gate never releases, and
	// WaitDelay bounds Wait if a child of A still holds A's output open after that.
	ctxA, cancelA := context.WithTimeout(t.Context(), time.Minute)
	defer cancelA()
	cmdA := a.command(ctxA, "./internal/harness/natsfixture/")
	cmdA.WaitDelay = 10 * time.Second
	waitedA, stdoutA, stderrA := a.startRunner(t, cmdA, "publication.ready")
	pidA := cmdA.Process.Pid
	var errA error
	reapedA, resumed := false, false
	resumeA := func() {
		if !resumed {
			resumed = true
			if _, err := gate.Write([]byte("resume\n")); err != nil {
				t.Errorf("resume runner A: %v", err)
			}
		}
	}
	// Every path out resumes A and waits for it, within ctxA's deadline.
	defer func() {
		resumeA()
		if !reapedA {
			<-waitedA
		}
	}()
	// runningA reports whether A is still running: Wait has not returned and its pid takes a signal.
	runningA := func() bool {
		select {
		case errA = <-waitedA:
			reapedA = true
			return false
		default:
			return pidAlive(pidA)
		}
	}
	if !runningA() {
		t.Fatalf("runner A stopped running at the publication gate (wait: %v)", errA)
	}

	// The premise: ps prints A's start time, and prints it differently in A's time zone and in
	// B's, so a judge that compares A's recorded identity with its own reading calls A stale. A
	// host where ps is denied, or ignores TZ, fails here instead of passing without the race.
	lstart := func(tz string) (string, error) {
		cmd := exec.Command("ps", "-o", "lstart=", "-p", strconv.Itoa(pidA))
		cmd.Env = append(os.Environ(), tz, "LC_ALL=C")
		out, err := cmd.Output()
		return trimIdentity(out), err
	}
	inA, errInA := lstart("TZ=EST5")
	inB, errInB := lstart("TZ=JST-9")
	if errInA != nil || errInB != nil || inA == "" || inB == "" || inA == inB {
		t.Fatalf("premise: ps must print runner A's start time in both time zones, and differently: TZ=EST5 %q (%v), TZ=JST-9 %q (%v)",
			inA, errInA, inB, errInB)
	}

	// B has its own fakes and evidence, A's lock directory, and no wait budget, so it decides at
	// once; its deadline is a watchdog, as in T1.
	b := newHarness(t, "SEMENGINE_DOCKER_ADMISSION_LOCK_DIR="+a.lock, "TZ=JST-9", "LC_ALL=C")
	ctxB, cancelB := context.WithTimeout(t.Context(), time.Minute)
	defer cancelB()
	cmdB := b.command(ctxB, "./internal/harness/natsfixture/")
	var stdoutB, stderrB bytes.Buffer
	cmdB.Stdout, cmdB.Stderr = &stdoutB, &stderrB
	statusB := exitCode(cmdB.Run())
	if want := "host lock is busy and no wait budget was requested: " + a.lock; statusB != 1 || !strings.Contains(stderrB.String(), want) {
		t.Errorf("runner B status %d, want 1 reporting %q\nstdout:\n%s\nstderr:\n%s", statusB, want, stdoutB.String(), stderrB.String())
	}
	if strings.Contains(stdoutB.String()+stderrB.String(), "cleaned stale lock") {
		t.Errorf("runner B quarantined the lock of runner A (pid %d) while A was publishing its record", pidA)
	}
	if log := b.read(t, "docker.log"); log != "" {
		t.Errorf("runner B called Docker:\n%s", log)
	}
	if !runningA() {
		t.Fatalf("runner A stopped running while runner B judged its lock (wait: %v)", errA)
	}

	resumeA()
	errA, reapedA = <-waitedA, true
	if code := exitCode(errA); code != 0 {
		t.Errorf("runner A exit %d, want 0\nstdout:\n%s\nstderr:\n%s", code, stdoutA, stderrA)
	}
	if strings.Contains(stderrA.String(), "lock ownership changed") {
		t.Errorf("runner A lost its lock while publishing its record:\n%s", stderrA)
	}
	// A's token is the one it exported to go test; its evidence must be its whole record.
	tokenA := ""
	for _, line := range strings.Split(a.read(t, "go.env"), "\n") {
		if v, ok := strings.CutPrefix(line, "SEMENGINE_DOCKER_ADMISSION_TOKEN="); ok {
			tokenA = v
		}
	}
	evidence, err := os.ReadFile(filepath.Join(a.evidence, "lock-owner"))
	var keys []string
	owner := map[string]string{}
	for _, line := range strings.Split(strings.TrimSuffix(string(evidence), "\n"), "\n") {
		k, v, _ := strings.Cut(line, "=")
		keys, owner[k] = append(keys, k), v
	}
	wantKeys := append(slices.Clone(semstreamsOwnerKeys), "identity_utc")
	if err != nil || !slices.Equal(keys, wantKeys) || tokenA == "" || owner["token"] != tokenA || owner["pid"] != strconv.Itoa(pidA) {
		t.Errorf("runner A's lock-owner evidence is not its whole record: keys %q (want %q), token %q (want %q), pid %q (want %d), read error %v",
			keys, wantKeys, owner["token"], tokenA, owner["pid"], pidA, err)
	}
	if left, err := filepath.Glob(a.lock + "*"); err != nil || len(left) != 0 {
		t.Errorf("left at the lock path or beside it after release: %q, %v", left, err)
	}
}

// fakeMktemp refuses the owner record's template, a name holding ".owner.", as a full or
// read-only temporary directory would, and hands every other call to the system's mktemp.
const fakeMktemp = `#!/usr/bin/env bash
case "$*" in
  *.owner.*) echo "fake mktemp: cannot create $*" >&2; exit 1 ;;
esac
command -p mktemp "$@"
`

// T3 (#126, D7): a runner that has created the lock directory and cannot write its record beside
// it removes the lock directory, leaves nothing beside it, and exits non-zero before any Docker
// call.
func TestR1OwnerRecordThatCannotBePublishedReleasesTheLock(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	if err := os.WriteFile(filepath.Join(h.fake, "mktemp"), []byte(fakeMktemp), 0o755); err != nil {
		t.Fatal(err)
	}
	r := h.run(t, "./internal/harness/natsfixture/")
	if r.status == 0 || !strings.Contains(r.stderr, "could not publish") || !strings.Contains(r.stderr, "owner record") {
		t.Fatalf("status %d, want non-zero reporting that it could not publish the owner record\nstdout:\n%s\nstderr:\n%s",
			r.status, r.stdout, r.stderr)
	}
	if log := h.read(t, "docker.log"); log != "" {
		t.Errorf("docker was invoked without a published owner record:\n%s", log)
	}
	if left, err := filepath.Glob(h.lock + "*"); err != nil || len(left) != 0 {
		t.Errorf("left at the lock path or beside it: %q, %v", left, err)
	}
}

// R2: a same-host owner whose pid is dead, or whose pid now belongs to a different process, is
// quarantined and removed; the run then acquires, completes, and releases.
func TestR2StaleLockIsQuarantined(t *testing.T) {
	t.Parallel()
	// plant takes the subtest's t: it runs inside a parallel subtest, where a failure reported
	// against the parent is a panic, not a failed subtest.
	for name, plant := range map[string]func(t *testing.T, h *harness){
		"dead pid": func(t *testing.T, h *harness) {
			pid, identity := deadPID(t)
			h.writeOwner(t, hostname(t), pid, identity)
		},
		"changed identity": func(t *testing.T, h *harness) {
			h.writeOwner(t, hostname(t), os.Getpid(), "Mon Jan  1 00:00:00 1990")
		},
		// A record with identity_utc is judged by it, not by its identity, which here is live.
		"changed identity_utc": func(t *testing.T, h *harness) {
			h.writeOwner(t, hostname(t), os.Getpid(), startIdentity(t, os.Getpid()), "identity_utc=Mon Jan  1 00:00:00 1990")
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			plant(t, h)
			r := h.run(t, "./internal/harness/natsfixture/")
			if r.status != 0 {
				t.Fatalf("status %d, want 0\nstdout:\n%s\nstderr:\n%s", r.status, r.stdout, r.stderr)
			}
			if !strings.Contains(r.stdout, "cleaned stale lock") {
				t.Errorf("stdout does not record the quarantine:\n%s", r.stdout)
			}
			leftovers, _ := filepath.Glob(h.lock + "*")
			if len(leftovers) != 0 {
				t.Errorf("lock or quarantine directories remain: %v", leftovers)
			}
			if !strings.Contains(h.read(t, "go.owner"), "command=semengine ") {
				t.Error("go test did not run under this runner's own lock record")
			}
		})
	}
}

// R3: SIGTERM to the runner reaches go test and its grandchild through the process group; the
// lock is held until the group is reaped, then released; the exit status is 143; the evidence
// records the signal.
func TestR3TermReachesTheProcessGroup(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "FAKE_GO_MODE=hang")
	cmd := h.command(t.Context(), "./internal/harness/natsfixture/")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	// Ready means fake go has spawned its grandchild; a runner that exits first has failed, so the
	// wait ends on either observation rather than running out the test's clock.
	ready := filepath.Join(h.fake, "go.ready")
	state, err := probe.Await(t.Context(), func(context.Context) (string, error) {
		select {
		case err := <-waited:
			waited <- err
			return "runner exited", nil
		default:
		}
		if _, err := os.Stat(ready); err != nil {
			return "waiting", err
		}
		return "ready", nil
	}, func(s string) bool { return s != "waiting" })
	if err != nil || state != "ready" {
		_ = cmd.Process.Kill()
		t.Fatalf("fake go never became ready (%s): %v\nstderr:\n%s", state, err, stderr.String())
	}
	grandchild, err := strconv.Atoi(strings.TrimSpace(h.read(t, "grandchild.pid")))
	if err != nil || !pidAlive(grandchild) {
		t.Fatalf("grandchild pid %q not running: %v", h.read(t, "grandchild.pid"), err)
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	err = <-waited
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 143 {
		t.Fatalf("runner exit = %v, want status 143\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	signals := h.read(t, "go.signals")
	if !strings.Contains(signals, "term") {
		t.Errorf("go test did not receive TERM; recorded %q", signals)
	}
	if !strings.Contains(signals, "grandchild-term") {
		t.Errorf("the grandchild (a test binary's stand-in) never received TERM; recorded %q", signals)
	}
	if !strings.Contains(signals, "lock-held-at-exit") {
		t.Errorf("the lock was released before go test finished exiting; recorded %q", signals)
	}
	if pidAlive(grandchild) {
		t.Errorf("grandchild %d outlived the runner", grandchild)
		_ = syscall.Kill(grandchild, syscall.SIGKILL)
	}
	if _, err := os.Stat(h.lock); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("lock not released after the group was reaped: %v", err)
	}
	record := h.evidenceFile(t, "runner.env")
	for _, want := range []string{"signal=TERM", "go_test_status=143", "leak_check=clean"} {
		if !strings.Contains(record, want) {
			t.Errorf("evidence runner.env lacks %q:\n%s", want, record)
		}
	}
}

// R4: the canonical argv, the exported environment, the image pin, and the owner record.
func TestR4CanonicalInvocationAndOwnerFormat(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "SEMSTREAMS_INTEGRATION_LOCK_DIR=/nonexistent/semstreams.lock", "TESTCONTAINERS_RYUK_DISABLED=true")
	r := h.run(t, "./internal/harness/natsfixture/")
	if r.status != 0 {
		t.Fatalf("status %d\nstdout:\n%s\nstderr:\n%s", r.status, r.stdout, r.stderr)
	}
	argv := strings.Split(strings.TrimSpace(h.read(t, "go.argv")), "\n")
	want := []string{"test", "-race", "-failfast", "-tags=integration", "-count=1", "-p", "2", "-timeout", "10m",
		"-coverprofile=" + filepath.Join(h.evidence, "integration.coverprofile"), "./internal/harness/natsfixture/"}
	if !slices.Equal(argv, want) {
		t.Errorf("go argv\n got %q\nwant %q", argv, want)
	}

	env := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(h.read(t, "go.env")), "\n") {
		k, v, _ := strings.Cut(line, "=")
		env[k] = v
	}
	if env["TESTCONTAINERS_RYUK_DISABLED"] != "false" {
		t.Errorf("TESTCONTAINERS_RYUK_DISABLED=%q, want false even when the caller set true", env["TESTCONTAINERS_RYUK_DISABLED"])
	}
	if pin := readPin(t, h.root); env["SEMENGINE_NATS_IMAGE"] != pin {
		t.Errorf("SEMENGINE_NATS_IMAGE=%q, want the .nats-image pin %q", env["SEMENGINE_NATS_IMAGE"], pin)
	}
	if env["SEMENGINE_DOCKER_ADMISSION_LOCK_DIR"] != h.lock || env["SEMENGINE_EVIDENCE_DIR"] != h.evidence {
		t.Errorf("lock dir %q and evidence dir %q not exported as given", env["SEMENGINE_DOCKER_ADMISSION_LOCK_DIR"], env["SEMENGINE_EVIDENCE_DIR"])
	}

	var keys []string
	owner := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(h.read(t, "go.owner")), "\n") {
		k, v, _ := strings.Cut(line, "=")
		keys, owner[k] = append(keys, k), v
	}
	if want := append(slices.Clone(semstreamsOwnerKeys), "identity_utc"); !slices.Equal(keys, want) {
		t.Errorf("owner keys %q, want SemStreams' followed by identity_utc: %q", keys, want)
	}
	// ps prints the runner's start time on every host this runs on, so unknown is a failed read.
	// TrimSpace above strips the last line's trailing blanks: no value is checked byte for byte.
	if v := owner["identity_utc"]; v == "" || v == "unknown" {
		t.Errorf("owner identity_utc %q, want the runner's start time", v)
	}
	if env["SEMENGINE_DOCKER_ADMISSION_TOKEN"] == "" || owner["token"] != env["SEMENGINE_DOCKER_ADMISSION_TOKEN"] {
		t.Errorf("exported token %q does not match the owner record's %q", env["SEMENGINE_DOCKER_ADMISSION_TOKEN"], owner["token"])
	}
	if wantCmd := "semengine " + filepath.Join(h.root, "scripts", "test-integration.sh"); owner["command"] != wantCmd {
		t.Errorf("owner command %q, want %q", owner["command"], wantCmd)
	}
	if owner["host"] != hostname(t) {
		t.Errorf("owner host %q, want %q", owner["host"], hostname(t))
	}
	if _, err := os.Stat(h.lock); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("lock not released after a clean run: %v", err)
	}
	record := h.evidenceFile(t, "runner.env")
	for _, want := range []string{"go_test_status=0", "leak_check=clean", "docker_context=fake-context", "nats_digest_cached=yes"} {
		if !strings.Contains(record, want) {
			t.Errorf("evidence runner.env lacks %q:\n%s", want, record)
		}
	}
	if !strings.Contains(h.evidenceFile(t, "per-package.txt"), "example.com/fake") {
		t.Error("per-package wall time not recorded")
	}
}

// Release is token-checked: if the owner record no longer carries this run's token, the runner
// leaves the lock alone rather than delete someone else's.
func TestReleaseRequiresMatchingToken(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "FAKE_GO_MODE=steal")
	r := h.run(t, "./internal/harness/natsfixture/")
	if !strings.Contains(r.stderr, "lock ownership changed; refusing to remove") {
		t.Errorf("stderr does not report the refusal:\n%s", r.stderr)
	}
	owner, err := os.ReadFile(filepath.Join(h.lock, "owner"))
	if err != nil || !strings.Contains(string(owner), "token=someone-else") {
		t.Fatalf("the other owner's lock was removed or altered: %q, %v", owner, err)
	}
}

// A failed bounded pull stops the run before go test, releases the lock, and says why.
func TestPullFailureReleasesLock(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "FAKE_IMAGE_CACHED=0", "FAKE_PULL_FAILS=1")
	r := h.run(t, "./internal/harness/natsfixture/")
	if r.status == 0 || !strings.Contains(r.stderr, "pull failed") || !strings.Contains(r.stderr, "manifest unknown") {
		t.Fatalf("status %d, want a reported pull failure\nstderr:\n%s", r.status, r.stderr)
	}
	if h.read(t, "go.argv") != "" {
		t.Error("go test ran without the image")
	}
	if _, err := os.Stat(h.lock); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("lock not released after the pull failure: %v", err)
	}
	if !strings.Contains(h.read(t, "docker.log"), "pull "+readPin(t, h.root)) {
		t.Errorf("pull was not by the pinned reference:\n%s", h.read(t, "docker.log"))
	}
}

// Leak check: containers labelled with the run's session that are still present after the
// bounded wait are removed by ID and fail the run; a label that clears within the wait (Ryuk
// reaping) passes. The runner never lists by name.
func TestLeakCheckBySession(t *testing.T) {
	t.Parallel()
	t.Run("clears within the wait", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t, "FAKE_PS_SURVIVORS=2")
		if r := h.run(t, "./internal/harness/natsfixture/"); r.status != 0 {
			t.Fatalf("status %d\nstderr:\n%s", r.status, r.stderr)
		}
		if !strings.Contains(h.evidenceFile(t, "runner.env"), "leak_check=clean") {
			t.Error("a session that cleared within the wait was not recorded clean")
		}
	})
	t.Run("survivor is removed by id and fails the run", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t, "FAKE_PS_SURVIVORS=1000")
		r := h.run(t, "./internal/harness/natsfixture/")
		if r.status == 0 {
			t.Fatal("a run that left a container behind passed")
		}
		log := h.read(t, "docker.log")
		if !strings.Contains(log, "rm -f c0ffee00c0ffee00") {
			t.Errorf("survivor not removed by id:\n%s", log)
		}
		if !strings.Contains(log, "ps -aq --no-trunc --filter label=org.testcontainers.sessionId=fake-session") {
			t.Errorf("leak listing was not by the session label:\n%s", log)
		}
		if strings.Contains(log, "name=") {
			t.Errorf("runner selected Docker resources by name:\n%s", log)
		}
		if !strings.Contains(h.evidenceFile(t, "runner.env"), "leak_check=survivors") {
			t.Error("survivors not recorded in evidence")
		}
	})
}

func readPin(t *testing.T, root string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".nats-image"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			return line
		}
	}
	t.Fatal(".nats-image holds no pin")
	return ""
}

// startRunner starts the runner and waits until fake go reports ready (the file named) or the
// runner exits first, which fails the test.
func (h *harness) startRunner(t *testing.T, cmd *exec.Cmd, readyFile string) (<-chan error, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	ready := filepath.Join(h.fake, readyFile)
	state, err := probe.Await(t.Context(), func(context.Context) (string, error) {
		select {
		case err := <-waited:
			waited <- err
			return "runner exited", nil
		default:
		}
		if _, err := os.Stat(ready); err != nil {
			return "waiting", err
		}
		return "ready", nil
	}, func(s string) bool { return s != "waiting" })
	if err != nil || state != "ready" {
		_ = cmd.Process.Kill()
		t.Fatalf("%s never appeared (%s): %v\nstderr:\n%s", readyFile, state, err, stderr.String())
	}
	return waited, &stdout, &stderr
}

func exitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	if err != nil {
		return -1
	}
	return 0
}

// H2: a signal during the bounded image pull kills and reaps the pull before the lock is
// released. A pull left running would hold registry and daemon work after the run that owned
// it has gone.
func TestSignalDuringPullReapsThePull(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "FAKE_IMAGE_CACHED=0", "FAKE_PULL_MODE=hang")
	cmd := h.command(t.Context(), "./internal/harness/natsfixture/")
	waited, stdout, stderr := h.startRunner(t, cmd, "pull.pid")
	pull, err := strconv.Atoi(strings.TrimSpace(h.read(t, "pull.pid")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Kill(pull, syscall.SIGKILL) })
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if code := exitCode(<-waited); code != 143 {
		t.Fatalf("runner exit %d, want 143\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if pidAlive(pull) {
		t.Fatalf("docker pull (pid %d) still running after the runner exited", pull)
	}
	if _, err := os.Stat(h.lock); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lock not released: %v", err)
	}
	if h.read(t, "go.argv") != "" {
		t.Fatal("go test ran after the pull was interrupted")
	}
}

// M3: a terminal Ctrl-C reaches the runner's whole foreground process group, the log's tee
// included. Output go test writes after the interrupt (its failure summary) still lands in
// go-test.log and per-package.txt.
func TestTerminalInterruptKeepsTheLog(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "FAKE_GO_MODE=interrupt-output")
	cmd := h.command(t.Context(), "./internal/harness/natsfixture/")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // a terminal's foreground group
	waited, stdout, stderr := h.startRunner(t, cmd, "go.ready")
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	if code := exitCode(<-waited); code != 130 {
		t.Fatalf("runner exit %d, want 130\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	const line = "--- FAIL: shutdown summary written after the interrupt"
	if log := h.evidenceFile(t, "go-test.log"); !strings.Contains(log, line) {
		t.Fatalf("go-test.log lost the output written after the interrupt:\n%s", log)
	}
	if per := h.evidenceFile(t, "per-package.txt"); !strings.Contains(per, line) {
		t.Fatalf("per-package.txt lost the summary:\n%s", per)
	}
}

// M4: a runner launched with SIGINT ignored (an `&` job of a non-interactive shell) cannot trap
// it. It says so, records it, and still runs; TERM remains the scripted interrupt.
func TestIgnoredInterruptIsDetected(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	script := filepath.Join(h.root, "scripts", "test-integration.sh")
	cmd := exec.CommandContext(t.Context(), "bash", "-c", `trap '' INT; exec bash "$0" "$@"`, script, "./internal/harness/natsfixture/")
	cmd.Env, cmd.Dir = h.env, h.root
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if code := exitCode(cmd.Run()); code != 0 {
		t.Fatalf("runner exit %d\nstderr:\n%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "WARN") || !strings.Contains(stderr.String(), "SIGINT") {
		t.Errorf("no warning that SIGINT is ignored:\n%s", stderr.String())
	}
	if rec := h.evidenceFile(t, "runner.env"); !strings.Contains(rec, "int_ignored_on_entry=yes") {
		t.Errorf("runner.env does not record the ignored SIGINT:\n%s", rec)
	}
	// The ordinary launch records the opposite, so the field is evidence either way.
	plain := newHarness(t)
	if r := plain.run(t, "./internal/harness/natsfixture/"); r.status != 0 || strings.Contains(r.stderr, "WARN") {
		t.Fatalf("plain run: status %d\n%s", r.status, r.stderr)
	}
	if rec := plain.evidenceFile(t, "runner.env"); !strings.Contains(rec, "int_ignored_on_entry=no") {
		t.Errorf("runner.env lacks int_ignored_on_entry=no:\n%s", rec)
	}
}

// M10: a go test group that ignores TERM is killed once the grace passes, and the run still
// reaps it, leak-checks, and releases the lock. SEMENGINE_TEST_SIGNAL_GRACE_SECONDS exists only
// so this test need not wait out the production 20s.
func TestKillEscalationEndsAGroupThatIgnoresTerm(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "FAKE_GO_MODE=ignore-term", "SEMENGINE_TEST_SIGNAL_GRACE_SECONDS=1")
	cmd := h.command(t.Context(), "./internal/harness/natsfixture/")
	waited, stdout, stderr := h.startRunner(t, cmd, "go.ready")
	grandchild, err := strconv.Atoi(strings.TrimSpace(h.read(t, "grandchild.pid")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Kill(grandchild, syscall.SIGKILL) })
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	// A watchdog for the failure, not a synchronisation: with a 1s grace the runner exits within
	// a few seconds; without the knob it would take the production 20s.
	watchdog := time.NewTimer(10 * time.Second)
	defer watchdog.Stop()
	select {
	case err := <-waited:
		if code := exitCode(err); code != 143 {
			t.Fatalf("runner exit %d, want 143\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
		}
	case <-watchdog.C:
		_ = cmd.Process.Kill()
		t.Fatal("runner still waiting on a TERM-ignoring group 10s after a 1s grace")
	}
	if pidAlive(grandchild) {
		t.Fatalf("grandchild %d survived the KILL escalation", grandchild)
	}
	rec := h.evidenceFile(t, "runner.env")
	for _, want := range []string{"signal=TERM", "group_killed_ms=", "group_reaped_ms=", "leak_check=clean", "signal_grace_s=1"} {
		if !strings.Contains(rec, want) {
			t.Errorf("runner.env lacks %q:\n%s", want, rec)
		}
	}
	if _, err := os.Stat(h.lock); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lock not released: %v", err)
	}
}

// A4: SEMENGINE_NATS_IMAGE replaces the pin only with a stated reason. A stale export in a
// developer's shell is refused before the lock, not silently run.
func TestImageOverrideNeedsAReason(t *testing.T) {
	t.Parallel()
	other := "nats" + "@sha256:" + strings.Repeat("0", 64) // assembled: T-B3 keeps image literals in .nats-image
	refused := newHarness(t, "SEMENGINE_NATS_IMAGE="+other)
	r := refused.run(t, "./internal/harness/natsfixture/")
	if r.status == 0 || !strings.Contains(r.stderr, "SEMENGINE_NATS_IMAGE_OVERRIDE_REASON") {
		t.Fatalf("override without a reason: status %d\n%s", r.status, r.stderr)
	}
	if refused.read(t, "docker.log") != "" {
		t.Fatal("a refused override reached Docker")
	}
	if _, err := os.Stat(refused.lock); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a refused override took the lock")
	}

	h := newHarness(t, "SEMENGINE_NATS_IMAGE="+other, "SEMENGINE_NATS_IMAGE_OVERRIDE_REASON=forced-failure protocol item 3")
	r = h.run(t, "./internal/harness/natsfixture/")
	if r.status != 0 {
		t.Fatalf("override with a reason: status %d\n%s", r.status, r.stderr)
	}
	if !strings.Contains(r.stderr, "WARN") || !strings.Contains(r.stderr, other) {
		t.Errorf("no WARN naming the override:\n%s", r.stderr)
	}
	rec := h.evidenceFile(t, "runner.env")
	for _, want := range []string{"image_override=" + other, "image_override_reason=forced-failure protocol item 3"} {
		if !strings.Contains(rec, want) {
			t.Errorf("runner.env lacks %q:\n%s", want, rec)
		}
	}
	if !strings.Contains(h.read(t, "go.env"), "SEMENGINE_NATS_IMAGE="+other) {
		t.Error("go test did not receive the override")
	}
}

// R1 (round 2): a writer that left the go test process group keeps the log's FIFO open, so the
// tee never sees EOF. The runner bounds its wait for tee, kills it, records the log as
// incomplete, and still releases the shared lock promptly.
func TestEscapedWriterDoesNotHoldTheLock(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "FAKE_GO_MODE=escape")
	cmd := h.command(t.Context(), "./internal/harness/natsfixture/")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	t.Cleanup(func() {
		if pid, err := strconv.Atoi(strings.TrimSpace(h.read(t, "escaped.pid"))); err == nil {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	})
	// A watchdog for the failure, not a synchronisation: the escaped writer lives 60s, and a
	// runner that waits for it would hold the lock that long.
	watchdog := time.NewTimer(20 * time.Second)
	defer watchdog.Stop()
	select {
	case err := <-waited:
		if code := exitCode(err); code != 0 {
			t.Fatalf("runner exit %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
		}
	case <-watchdog.C:
		_ = cmd.Process.Kill()
		t.Fatal("runner still waiting for its log 20s after go test exited; the lock is held by an escaped writer")
	}
	if _, err := os.Stat(h.lock); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lock not released: %v", err)
	}
	if rec := h.evidenceFile(t, "runner.env"); !strings.Contains(rec, "log_incomplete=yes") {
		t.Fatalf("runner.env does not record the incomplete log:\n%s", rec)
	}
	if !strings.Contains(stderr.String(), "WARN") {
		t.Errorf("no warning that the log is incomplete:\n%s", stderr.String())
	}
}

// The grace knob is reserved for the contract tests: a real run, on the shared lock, refuses it
// before taking the lock, so a stale export cannot shorten a real run's shutdown grace.
func TestSignalGraceKnobRefusedOutsideContractTests(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "SEMENGINE_TEST_SIGNAL_GRACE_SECONDS=1")
	var env []string
	for _, kv := range h.env {
		if !strings.HasPrefix(kv, "SEMENGINE_DOCKER_ADMISSION_LOCK_DIR=") {
			env = append(env, kv)
		}
	}
	h.env = env
	r := h.run(t, "./internal/harness/natsfixture/")
	if r.status != 2 || !strings.Contains(r.stderr, "SEMENGINE_TEST_SIGNAL_GRACE_SECONDS") {
		t.Fatalf("grace knob on the shared lock: status %d, want 2\n%s", r.status, r.stderr)
	}
	if h.read(t, "docker.log") != "" || h.read(t, "go.argv") != "" {
		t.Fatal("a refused run reached Docker or go test")
	}
	if !strings.Contains(r.stderr, "contract tests") {
		t.Errorf("refusal does not say what the knob is for:\n%s", r.stderr)
	}
}
