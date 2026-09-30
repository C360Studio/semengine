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
// oracle: both runners must write and read exactly these keys, in this order, or neither can judge
// the other's lock.
var semstreamsOwnerKeys = []string{"host", "pid", "started", "identity", "token", "command"}

// fakeDocker records every invocation and answers the handful of subcommands the runner uses.
// FAKE_IMAGE_CACHED=0 makes the image absent, FAKE_PULL_FAILS=1 fails the pull, and
// FAKE_PS_SURVIVORS=N makes `docker ps` report one container for the first N calls.
const fakeDocker = `#!/usr/bin/env bash
echo "$*" >> "$FAKE_DIR/docker.log"
case "$1 ${2:-}" in
  "info "*) echo "fake info" ;;
  "version "*) echo "fake version" ;;
  "context show") echo fake-context ;;
  "context inspect") echo unix:///fake.sock ;;
  "image inspect") [ "${FAKE_IMAGE_CACHED:-1}" = 1 ] || exit 1 ;;
  "pull "*) [ "${FAKE_PULL_FAILS:-0}" = 1 ] && { echo "fake pull: manifest unknown" >&2; exit 1; } ;;
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

// writeOwner plants a lock held by someone else, in the shared owner-record format.
func (h *harness) writeOwner(t *testing.T, host string, pid int, identity string) {
	t.Helper()
	if err := os.Mkdir(h.lock, 0o755); err != nil {
		t.Fatal(err)
	}
	owner := fmt.Sprintf("host=%s\npid=%d\nstarted=%d\nidentity=%s\ntoken=foreign-token\ncommand=scripts/run-integration-tests.sh\n",
		host, pid, time.Now().Unix(), identity)
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
	out, err := exec.Command("ps", "-o", "lstart=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		t.Fatalf("ps lstart %d: %v", pid, err)
	}
	return strings.TrimLeft(strings.TrimSuffix(string(out), "\n"), " \t")
}

// deadPID returns the pid of a process that has exited and been reaped.
func deadPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	if err := syscall.Kill(pid, 0); err == nil {
		t.Skipf("pid %d was reused before the test could use it", pid)
	}
	return pid
}

func pidAlive(pid int) bool { return syscall.Kill(pid, 0) == nil }

// R1: a live same-host owner refuses the run before any Docker call, and SEMSTREAMS_* variables
// (here a wait budget and a different lock path) are not read.
func TestR1BusyLockRefusesBeforeDocker(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "SEMSTREAMS_INTEGRATION_LOCK_WAIT_SECONDS=30", "SEMSTREAMS_INTEGRATION_LOCK_DIR=/nonexistent/semstreams.lock")
	h.writeOwner(t, hostname(t), os.Getpid(), startIdentity(t, os.Getpid()))
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
}

// R1 (foreign host): an owner on another host is never judged stale here, whatever its pid.
func TestR1ForeignHostOwnerIsRespected(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.writeOwner(t, "some-other-host.invalid", deadPID(t), "unknown")
	r := h.run(t, "./internal/harness/natsfixture/")
	if r.status != 1 || !strings.Contains(r.stderr, "host=some-other-host.invalid") {
		t.Fatalf("status %d, want 1 naming the foreign owner\nstderr:\n%s", r.status, r.stderr)
	}
	if _, err := os.Stat(filepath.Join(h.lock, "owner")); err != nil {
		t.Errorf("the foreign lock was removed: %v", err)
	}
}

// R2: a same-host owner whose pid is dead, or whose pid now belongs to a different process, is
// quarantined and removed; the run then acquires, completes, and releases.
func TestR2StaleLockIsQuarantined(t *testing.T) {
	t.Parallel()
	for name, plant := range map[string]func(h *harness){
		"dead pid":         func(h *harness) { h.writeOwner(t, hostname(t), deadPID(t), "unknown") },
		"changed identity": func(h *harness) { h.writeOwner(t, hostname(t), os.Getpid(), "Mon Jan  1 00:00:00 1990") },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			plant(h)
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
	if !slices.Equal(keys, semstreamsOwnerKeys) {
		t.Errorf("owner keys %q, want SemStreams' %q", keys, semstreamsOwnerKeys)
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
