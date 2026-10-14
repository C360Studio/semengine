//go:build integration

package natsfixture

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/c360studio/semengine/internal/harness/lifecycletest"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/testcontainers/testcontainers-go"
)

// These include the owner tests S1-1, S1-2 and S1-6..S1-8 of design "Lifecycle ownership proof"
// (S1-3 and S1-5 went with Consume, test-infra-audit task 3.2). They need Docker:
// `go test -tags integration ./internal/harness/natsfixture/`.

// stopBound is the fresh finite authority these tests give a controlled Stop.
const stopBound = 60 * time.Second

// containerExists asks the Docker CLI, an oracle independent of the fixture.
func containerExists(t *testing.T, id string) bool {
	t.Helper()
	if id == "" {
		t.Fatal("containerExists: empty id")
	}
	out, err := exec.Command("docker", "inspect", "--format", "{{.Id}}", id).CombinedOutput()
	if err == nil {
		return true
	}
	if strings.Contains(strings.ToLower(string(out)), "no such") {
		return false
	}
	t.Fatalf("docker inspect %s: %v\n%s", id, err, out)
	return false
}

func startFixture(t *testing.T) *Fixture {
	t.Helper()
	f := New(t)
	if err := f.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	return f
}

func stop(t *testing.T, f *Fixture) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), stopBound)
	defer cancel()
	return f.Stop(ctx)
}

// logTB records what the fixture writes to the test log, and passes it on.
type logTB struct {
	testing.TB
	mu   sync.Mutex
	logs []string
}

func (l *logTB) Logf(format string, args ...any) {
	l.mu.Lock()
	l.logs = append(l.logs, fmt.Sprintf(format, args...))
	l.mu.Unlock()
	l.TB.Logf(format, args...)
}

// containerLogs counts the container logs written to the test log, each a log holding the
// server's ready line.
func (l *logTB) containerLogs() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, line := range l.logs {
		if strings.Contains(line, readyLog) {
			n++
		}
	}
	return n
}

// conn is the fixture's own connection, read in-package: the fixture exports no accessor for it,
// because Stop closes it.
func conn(f *Fixture) *nats.Conn {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.nc
}

// TestFixtureRoundTrip is the adopter's path: start, use JetStream and KV, stop, and find
// everything gone.
func TestFixtureRoundTrip(t *testing.T) {
	f := startFixture(t)
	ctx := t.Context()
	if !strings.HasPrefix(f.URL(), "nats://") || conn(f).Status() != nats.CONNECTED {
		t.Fatalf("URL %q status %v", f.URL(), conn(f).Status())
	}
	stream := "orders"
	if _, err := f.CreateStream(ctx, stream, stream+".>"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.JetStream().Publish(ctx, stream+".1", []byte("one")); err != nil {
		t.Fatal(err)
	}
	kv, err := f.JetStream().CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "kv"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kv.Put(ctx, "k", []byte("v")); err != nil {
		t.Fatal(err)
	}
	if e, err := kv.Get(ctx, "k"); err != nil || string(e.Value()) != "v" {
		t.Fatalf("KV round trip: %v %v", e, err)
	}
	id := f.containerID
	if err := stop(t, f); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if rem := f.remaining(); len(rem) != 0 {
		t.Fatalf("remaining after Stop: %v", rem)
	}
	if containerExists(t, id) {
		t.Fatalf("container %s exists after Stop", id)
	}
}

// failpoints wrap a real dependency: fail returns an error after the real call, so the resource
// the call creates exists; block runs the real call, then parks until released.
func failAfter[T any](real func(context.Context, testcontainers.Container) (T, error), err error) func(context.Context, testcontainers.Container) (T, error) {
	return func(ctx context.Context, c testcontainers.Container) (T, error) {
		v, _ := real(ctx, c)
		return v, err
	}
}

// S1-1: a failure at each phase after the container exists removes it, names the phase and the
// container, writes the container's log to the test log, terminates exactly once per attempt, and
// leaves Stop nothing to do (nats-fixture › "Failure after the container exists").
func TestS1_1PartialStartupCleanup(t *testing.T) {
	injected := errors.New("injected failure")
	for _, tc := range []struct {
		phase    Phase
		inject   func(d *deps)
		attempts int
	}{
		{PhaseHost, func(d *deps) { d.host = failAfter(d.host, injected) }, 1},
		// A persistent mapped-port failure earns exactly one replacement, then fails.
		{PhaseMappedPort, func(d *deps) { d.mappedPort = failAfter(d.mappedPort, injected) }, 2},
		{PhaseConnect, func(d *deps) {
			real := d.connect
			d.connect = func(ctx context.Context, url string) (*nats.Conn, error) {
				nc, err := real(ctx, url)
				if err == nil {
					nc.Close()
				}
				return nil, injected
			}
		}, 1},
		{PhaseJetStream, func(d *deps) { d.jsReady = func(context.Context, jetstream.JetStream) error { return injected } }, 1},
	} {
		t.Run(string(tc.phase), func(t *testing.T) {
			tb := &logTB{TB: t}
			f := New(tb)
			tc.inject(&f.deps)
			err := f.Start(t.Context())
			var fe *Error
			if !errors.As(err, &fe) || fe.Phase != tc.phase || !errors.Is(err, injected) {
				t.Fatalf("Start = %v, want an *Error at phase %s wrapping the injected cause", err, tc.phase)
			}
			if fe.ContainerID == "" || fe.ParentErr != nil || fe.Cleanup != nil {
				t.Fatalf("error fields: container %q parent %v cleanup %v", fe.ContainerID, fe.ParentErr, fe.Cleanup)
			}
			attempts := fixtureErrors(err)
			if len(attempts) != tc.attempts {
				t.Fatalf("%d attempt error(s), want %d: %v", len(attempts), tc.attempts, err)
			}
			for i, a := range attempts {
				if a.Attempt != i+1 || a.Phase != tc.phase || a.ContainerID == "" {
					t.Errorf("attempt %d error: attempt %d phase %s container %q", i+1, a.Attempt, a.Phase, a.ContainerID)
				} else if containerExists(t, a.ContainerID) {
					t.Errorf("attempt %d container %s still exists", a.Attempt, a.ContainerID)
				}
			}
			if got := tb.containerLogs(); got != tc.attempts {
				t.Errorf("%d container log(s) in the test log, want one per attempt (%d)", got, tc.attempts)
			}
			calls := f.callCounts()
			if calls["terminate"] != tc.attempts || calls["start"] != tc.attempts {
				t.Errorf("calls %v, want start and terminate %d each", calls, tc.attempts)
			}
			if err := stop(t, f); err != nil || !maps(calls, f.callCounts()) {
				t.Errorf("Stop after a rolled-back Start = %v, calls %v -> %v; want nil with no call", err, calls, f.callCounts())
			}
		})
	}

	// A resource operation after a successful Start reports its own phase and leaves the fixture
	// owned; Stop then removes everything.
	t.Run(string(PhaseCreateStream), func(t *testing.T) {
		f := startFixture(t)
		f.deps.createStream = func(context.Context, jetstream.JetStream, jetstream.StreamConfig) (jetstream.Stream, error) {
			return nil, injected
		}
		_, err := f.CreateStream(t.Context(), "never", "never.>")
		var fe *Error
		if !errors.As(err, &fe) || fe.Phase != PhaseCreateStream || fe.ContainerID == "" {
			t.Fatalf("CreateStream = %v, want an *Error at phase %s", err, PhaseCreateStream)
		}
		id := f.containerID
		if err := stop(t, f); err != nil {
			t.Fatalf("Stop: %v", err)
		}
		if containerExists(t, id) {
			t.Fatal("container survives Stop after a failed CreateStream")
		}
	})

	// A transient mapped-port failure is the one case replaced: the second container serves, and
	// the first is gone with its log in the test log.
	t.Run("mapped-port replacement", func(t *testing.T) {
		tb := &logTB{TB: t}
		f := New(tb)
		var started []string
		realStart := f.deps.start
		f.deps.start = func(ctx context.Context, req testcontainers.GenericContainerRequest) (testcontainers.Container, error) {
			c, err := realStart(ctx, req)
			if c != nil {
				started = append(started, c.GetContainerID())
			}
			return c, err
		}
		real, failed := f.deps.mappedPort, false
		f.deps.mappedPort = func(ctx context.Context, c testcontainers.Container) (string, error) {
			if !failed {
				failed = true
				return "", injected
			}
			return real(ctx, c)
		}
		if err := f.Start(t.Context()); err != nil {
			t.Fatalf("Start with one transient mapped-port failure: %v", err)
		}
		if len(started) != 2 || started[1] != f.containerID || containerExists(t, started[0]) {
			t.Fatalf("containers started %v, serving %s: want a replacement serving and the first gone", started, f.containerID)
		}
		if got := tb.containerLogs(); got != 1 {
			t.Fatalf("%d container log(s) in the test log, want the first attempt's", got)
		}
	})
}

func maps(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// S1-2: cancelling the test context while a phase is in I/O fails Start with
// context.Canceled, reports the parent state, removes every attempted container,
// and does not start another attempt after cancellation. A mapped-port failure
// may already have earned one replacement while the parent was live.
func TestS1_2CancellationDuringIO(t *testing.T) {
	for _, tc := range []struct {
		name           string
		phase          Phase
		failMappedPort bool
	}{
		{name: string(PhaseStart), phase: PhaseStart},
		{name: string(PhaseConnect), phase: PhaseConnect},
		{name: "connect after mapped-port failure", phase: PhaseConnect, failMappedPort: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := New(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var startedAfterCancel, signaled atomic.Bool
			realStart := f.deps.start
			f.deps.start = func(startCtx context.Context, req testcontainers.GenericContainerRequest) (testcontainers.Container, error) {
				if ctx.Err() != nil {
					startedAfterCancel.Store(true)
				}
				return realStart(startCtx, req)
			}
			if tc.failMappedPort {
				real := f.deps.mappedPort
				failed := false
				f.deps.mappedPort = func(ctx context.Context, c testcontainers.Container) (string, error) {
					if !failed {
						failed = true
						return "", errors.New("injected mapped-port failure")
					}
					return real(ctx, c)
				}
			}
			entered, release := make(chan struct{}), make(chan struct{})
			signal := func() {
				if signaled.CompareAndSwap(false, true) {
					close(entered)
				}
			}
			switch tc.phase {
			case PhaseStart:
				real := f.deps.start
				f.deps.start = func(ctx context.Context, req testcontainers.GenericContainerRequest) (testcontainers.Container, error) {
					c, err := real(ctx, req)
					signal()
					<-release
					return c, err
				}
			case PhaseConnect:
				real := f.deps.connect
				f.deps.connect = func(ctx context.Context, url string) (*nats.Conn, error) {
					nc, err := real(ctx, url)
					signal()
					<-release
					return nc, err
				}
			}
			result := make(chan error, 1)
			go func() { result <- f.Start(ctx) }()
			select {
			case <-entered:
			case err := <-result:
				t.Fatalf("Start ended before phase %s: %v", tc.phase, err)
			}
			cancel()
			close(release)
			err := <-result
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("Start = %v, want context.Canceled", err)
			}
			attempts := fixtureErrors(err)
			if len(attempts) < 1 || len(attempts) > 2 ||
				tc.phase == PhaseStart && len(attempts) != 1 ||
				tc.failMappedPort && len(attempts) != 2 {
				t.Fatalf("unexpected attempts: %v", err)
			}
			for i, a := range attempts {
				if a.Attempt != i+1 || a.ContainerID == "" {
					t.Fatalf("attempt %d error is incomplete: %v", i+1, a)
				}
				if containerExists(t, a.ContainerID) {
					t.Fatalf("attempt %d container %s survives cancellation", a.Attempt, a.ContainerID)
				}
				if i < len(attempts)-1 && (a.Phase != PhaseMappedPort || a.ParentErr != nil || a.Cleanup != nil) {
					t.Fatalf("attempt %d was not a cleaned-up mapped-port failure with live parent: %v", a.Attempt, a)
				}
			}
			if last := attempts[len(attempts)-1]; last.Phase != tc.phase || !errors.Is(last.ParentErr, context.Canceled) || last.Cleanup != nil {
				t.Fatalf("terminal attempt = %v (parent %v, cleanup %v), want cancellation at phase %s", last, last.ParentErr, last.Cleanup, tc.phase)
			}
			if startedAfterCancel.Load() {
				t.Fatal("Start called the container start hook after cancellation")
			}
		})
	}
}

// S1-6: a Stop after a successful Stop is a no-op: nil, and no dependency call.
func TestS1_6RepeatedStop(t *testing.T) {
	f := startFixture(t)
	if err := stop(t, f); err != nil {
		t.Fatal(err)
	}
	before := f.callCounts()
	if err := stop(t, f); err != nil {
		t.Fatalf("repeated Stop = %v", err)
	}
	if after := f.callCounts(); !maps(before, after) {
		t.Fatalf("repeated Stop made calls: %v -> %v", before, after)
	}
}

// S1-7: a second Start is refused and leaves the running fixture untouched; the portable floor
// passes over the fixture with no restart promised.
func TestS1_7Restart(t *testing.T) {
	f := startFixture(t)
	before, calls := f.remaining(), f.callCounts()
	if err := f.Start(t.Context()); !errors.Is(err, ErrAlreadyUsed) {
		t.Fatalf("second Start = %v, want ErrAlreadyUsed", err)
	}
	if !slices.Equal(before, f.remaining()) || !maps(calls, f.callCounts()) || conn(f).Status() != nats.CONNECTED {
		t.Fatal("refused second Start changed the fixture")
	}
	t.Run("lifecycletest", func(t *testing.T) {
		lifecycletest.Run(t, func() lifecycletest.Owner { return owner{New(t)} }, mustFailFixture(t), lifecycletest.Promise{Restart: false})
	})
}

// errStartRefused is the start hook's injected failure in the fixture's must-fail factory.
var errStartRefused = errors.New("injected: container start refused")

// mustFailFixture is the fixture's must-fail factory: a fixture whose start hook creates the real
// container and then returns an error, as GenericContainer can (generic.go:89,94). Start must fail
// at PhaseStart and roll the container back, so the failed-start check judges that rollback, not a
// start that never reached Docker.
func mustFailFixture(t *testing.T) lifecycletest.Factory {
	return func() lifecycletest.Owner {
		f := New(t)
		realStart := f.deps.start
		f.deps.start = func(ctx context.Context, req testcontainers.GenericContainerRequest) (testcontainers.Container, error) {
			c, err := realStart(ctx, req)
			if c == nil {
				// No container means no rollback to judge: the check would pass on a start that
				// never reached Docker.
				t.Errorf("must-fail factory: the real start created no container: %v", err)
			}
			return c, errors.Join(err, errStartRefused)
		}
		return owner{f}
	}
}

// nats-fixture › "Start hook fails": Start returns an *Error at PhaseStart naming the container it
// rolled back, the fixture reports nothing unresolved, and a following Stop returns nil with no
// call.
func TestMustFailFixture(t *testing.T) {
	o := mustFailFixture(t)().(owner)
	err := o.f.Start(t.Context())
	var fe *Error
	if !errors.As(err, &fe) || fe.Phase != PhaseStart || !errors.Is(err, errStartRefused) || fe.ContainerID == "" || fe.Cleanup != nil {
		t.Fatalf("Start = %v, want an *Error at phase %s with a clean rollback", err, PhaseStart)
	}
	if containerExists(t, fe.ContainerID) {
		t.Fatalf("container %s survives the failed Start", fe.ContainerID)
	}
	if rem := o.f.remaining(); len(rem) != 0 {
		t.Fatalf("failed Start holds %v", rem)
	}
	calls := o.f.callCounts()
	if err := stop(t, o.f); err != nil || !maps(calls, o.f.callCounts()) {
		t.Fatalf("Stop after the failed Start = %v, calls %v -> %v; want nil with no call", err, calls, o.f.callCounts())
	}
}

// owner adapts a fixture to the lifecycle floor, reporting its retained state.
type owner struct{ f *Fixture }

func (o owner) Start(ctx context.Context) error { return o.f.Start(ctx) }
func (o owner) Stop(ctx context.Context) error  { return o.f.Stop(ctx) }
func (o owner) Observe() lifecycletest.Observation {
	return lifecycletest.Observation{Unresolved: o.f.remaining(), Calls: o.f.callCounts()}
}

// S1-8: two fixtures in one test are disjoint, and each Stop removes only its own.
func TestS1_8Isolation(t *testing.T) {
	a, b := startFixture(t), startFixture(t)
	if a.containerID == b.containerID || a.URL() == b.URL() {
		t.Fatalf("fixtures collide: %s %s / %s %s", a.containerID, a.URL(), b.containerID, b.URL())
	}
	aID, aURL, bID := a.containerID, a.URL(), b.containerID
	if err := stop(t, a); err != nil {
		t.Fatal(err)
	}
	if !containerExists(t, bID) {
		t.Fatal("stopping one fixture removed the other's container")
	}
	if _, err := b.JetStream().AccountInfo(t.Context()); err != nil {
		t.Fatalf("the other fixture stopped answering: %v", err)
	}
	t.Logf("isolated: %s at %s and %s at %s", shortID(aID), aURL, shortID(bID), b.URL())
}

// restart restarts the fixture under a bounded context and fails the test on error.
func restart(t *testing.T, f *Fixture) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), stopBound)
	defer cancel()
	if err := f.Restart(ctx); err != nil {
		t.Fatalf("Restart: %v", err)
	}
}

// Restart keeps the container's writable layer: a message acknowledged on a file-backed stream is
// readable at its sequence afterwards, one on a memory-backed stream is not (design P3, proven
// here). Stop then removes both streams and the container.
func TestRestartKeepsFileStreamLosesMemoryStream(t *testing.T) {
	f := startFixture(t)
	ctx := t.Context()
	file, mem := "file", "mem"
	if _, err := f.CreateStream(ctx, file, file+".>"); err != nil {
		t.Fatal(err)
	}
	ms, err := f.CreateMemoryStream(ctx, mem, mem+".>")
	if err != nil {
		t.Fatal(err)
	}
	if info, err := ms.Info(ctx); err != nil || info.Config.Storage != jetstream.MemoryStorage ||
		info.Config.MaxAge != time.Hour || info.Config.MaxBytes != 64<<20 || info.Config.Discard != jetstream.DiscardOld {
		t.Fatalf("memory stream as the broker reports it: %+v %v", info, err)
	}
	fileAck, err := f.JetStream().Publish(ctx, file+".1", []byte("kept"))
	if err != nil {
		t.Fatal(err)
	}
	memAck, err := f.JetStream().Publish(ctx, mem+".1", []byte("lost"))
	if err != nil {
		t.Fatal(err)
	}
	id := f.containerID

	restart(t, f)

	if f.containerID != id || f.callCounts()["start"] != 1 {
		t.Fatalf("Restart replaced the container: %s -> %s, start calls %d", id, f.containerID, f.callCounts()["start"])
	}
	js := f.JetStream()
	fs, err := js.Stream(ctx, file)
	if err != nil {
		t.Fatalf("file stream after restart: %v", err)
	}
	msg, err := fs.GetMsg(ctx, fileAck.Sequence)
	if err != nil || string(msg.Data) != "kept" {
		t.Fatalf("file-backed message at sequence %d after restart: %v %v", fileAck.Sequence, msg, err)
	}
	switch s, err := js.Stream(ctx, mem); {
	case errors.Is(err, jetstream.ErrStreamNotFound):
	case err != nil:
		t.Fatalf("memory stream after restart: %v", err)
	default:
		if _, err := s.GetMsg(ctx, memAck.Sequence); !errors.Is(err, jetstream.ErrMsgNotFound) {
			t.Fatalf("memory-backed message at sequence %d after restart: %v, want ErrMsgNotFound", memAck.Sequence, err)
		}
	}
	if err := stop(t, f); err != nil {
		t.Fatalf("Stop after Restart: %v", err)
	}
	if rem := f.remaining(); len(rem) != 0 || containerExists(t, id) {
		t.Fatalf("after Stop: remaining %v", rem)
	}
}

// After Restart, URL() is the container's current binding, read from Docker independently of the
// fixture, on the host port it had before the restart, and a fresh dial to it reaches the restarted
// broker.
func TestRestartURLDials(t *testing.T) {
	f := startFixture(t)
	old := f.URL()
	restart(t, f)
	out, err := exec.Command("docker", "port", f.containerID, clientPort).Output()
	if err != nil {
		t.Fatalf("docker port: %v", err)
	}
	var ports []string
	for _, line := range strings.Fields(string(out)) {
		if i := strings.LastIndex(line, ":"); i >= 0 {
			ports = append(ports, line[i+1:])
		}
	}
	if !slices.ContainsFunc(ports, func(p string) bool { return strings.HasSuffix(f.URL(), ":"+p) }) {
		t.Fatalf("URL %q is not the container's current binding %v", f.URL(), ports)
	}
	if f.URL() != old {
		t.Fatalf("URL %s -> %s across Restart, want the host port kept", old, f.URL())
	}
	nc, err := nats.Connect(f.URL(), nats.MaxReconnects(0))
	if err != nil {
		t.Fatalf("dial %s after restart: %v", f.URL(), err)
	}
	defer nc.Close()
	flushCtx, cancel := context.WithTimeout(t.Context(), stopBound) // nats.go demands a deadline here
	defer cancel()
	if err := nc.FlushWithContext(flushCtx); err != nil {
		t.Fatalf("round trip on the new binding: %v", err)
	}
	if conn(f).Status() != nats.CONNECTED {
		t.Fatalf("fixture connection %v after restart", conn(f).Status())
	}
}

// call. Restart returns an *Error naming exactly that phase, runs no later phase, creates no second
// container, and Stop still removes the one container.
func TestRestartFaultMatrix(t *testing.T) {
	injected := errors.New("injected failure")
	hook := func(real func(context.Context, testcontainers.Container) error, after bool) func(context.Context, testcontainers.Container) error {
		return func(ctx context.Context, c testcontainers.Container) error {
			if after {
				_ = real(ctx, c)
			}
			return injected
		}
	}
	for _, tc := range []struct {
		name  string
		phase Phase
		set   func(d *deps)
		// later is the dependency count that must not move: the next phase never ran.
		later string
	}{
		{"stop-container before", PhaseStopContainer, func(d *deps) { d.stopContainer = hook(d.stopContainer, false) }, "startContainer"},
		{"stop-container after", PhaseStopContainer, func(d *deps) { d.stopContainer = hook(d.stopContainer, true) }, "startContainer"},
		{"start-container before", PhaseStartContainer, func(d *deps) { d.startContainer = hook(d.startContainer, false) }, "mappedPort"},
		{"start-container after", PhaseStartContainer, func(d *deps) { d.startContainer = hook(d.startContainer, true) }, "mappedPort"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := startFixture(t)
			id := f.containerID
			tc.set(&f.deps)
			before := f.callCounts()
			ctx, cancel := context.WithTimeout(t.Context(), stopBound)
			defer cancel()
			err := f.Restart(ctx)
			var fe *Error
			if !errors.As(err, &fe) || fe.Phase != tc.phase || !errors.Is(err, injected) || fe.ContainerID != id || fe.ParentErr != nil {
				t.Fatalf("Restart = %v, want an *Error at phase %s on container %s wrapping the injected cause", err, tc.phase, shortID(id))
			}
			calls := f.callCounts()
			if calls[tc.later] != before[tc.later] {
				t.Fatalf("a phase after %s ran: %s calls %d -> %d", tc.phase, tc.later, before[tc.later], calls[tc.later])
			}
			if calls["start"] != 1 {
				t.Fatalf("start called %d times; Restart must not replace the container", calls["start"])
			}
			if got := processContainers(t); len(got) != 1 || got[0] != id {
				t.Fatalf("containers of this fixture after a failed Restart: %v, want only %s", got, id)
			}
			if err := f.Restart(ctx); !errors.Is(err, errNotStarted) {
				t.Fatalf("Restart after a failed Restart = %v, want the not-started refusal", err)
			}
			if err := stop(t, f); err != nil {
				t.Fatalf("Stop after a failed Restart: %v", err)
			}
			if rem := f.remaining(); len(rem) != 0 || containerExists(t, id) {
				t.Fatalf("after Stop: remaining %v, container exists %t", rem, containerExists(t, id))
			}
		})
	}
}

// nats-fixture › "Restart before Start", its second half: once Stop has begun, Restart refuses
// with no dependency call. A Stop whose container removal fails has begun and keeps the container
// for a later Stop; Restart must not stop and start that container again. Docker's start time for
// the container is the oracle that it was not restarted.
func TestRestartRefusedOnceStopHasBegun(t *testing.T) {
	f := startFixture(t)
	id := f.containerID
	startedAt := func() string {
		t.Helper()
		out, err := exec.Command("docker", "inspect", "--format", "{{.State.StartedAt}}", id).Output()
		if err != nil {
			t.Fatalf("docker inspect %s: %v", shortID(id), err)
		}
		return strings.TrimSpace(string(out))
	}
	first := startedAt()
	terminate := f.deps.terminate
	refused := errors.New("removal refused by the test")
	f.deps.terminate = func(context.Context, testcontainers.Container) error { return refused }
	if err := stop(t, f); !errors.Is(err, refused) {
		t.Fatalf("Stop with a refused removal = %v, want the refusal", err)
	}
	before := f.callCounts()
	ctx, cancel := context.WithTimeout(t.Context(), stopBound)
	defer cancel()
	if err := f.Restart(ctx); err == nil {
		t.Fatal("Restart after Stop began returned nil")
	}
	if calls := f.callCounts(); !maps(calls, before) {
		t.Fatalf("refused Restart made calls: %v -> %v", before, calls)
	}
	if got := startedAt(); got != first {
		t.Fatalf("container %s restarted after Stop began: started at %s, then %s", shortID(id), first, got)
	}
	f.deps.terminate = terminate
	if err := stop(t, f); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
	if containerExists(t, id) {
		t.Fatalf("container %s exists after the second Stop", shortID(id))
	}
}

// FaultKV over a real bucket (nats-fixture › "Fault-injecting key-value double"): a fail-after
// Update stands on the broker at the next revision; a fail-before Create leaves no key.
func TestFaultKVOverARealBucket(t *testing.T) {
	f := startFixture(t)
	ctx := t.Context()
	bucket, err := f.JetStream().CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "faults"})
	if err != nil {
		t.Fatal(err)
	}
	kv := NewFaultKV(bucket)
	rev, err := kv.Create(ctx, "k", []byte("v1"))
	if err != nil {
		t.Fatal(err)
	}
	injected := errors.New("injected")
	kv.FailAfter(KVUpdate, injected)
	if _, err := kv.Update(ctx, "k", []byte("v2"), rev); !errors.Is(err, injected) {
		t.Fatalf("Update = %v, want the injected error", err)
	}
	fresh, err := f.JetStream().KeyValue(ctx, bucket.Bucket())
	if err != nil {
		t.Fatal(err)
	}
	if e, err := fresh.Get(ctx, "k"); err != nil || string(e.Value()) != "v2" || e.Revision() != rev+1 {
		t.Fatalf("fresh read after a fail-after Update: %v %v, want v2 at revision %d", e, err, rev+1)
	}
	kv.FailBefore(KVCreate, injected)
	if _, err := kv.Create(ctx, "absent", []byte("v")); !errors.Is(err, injected) {
		t.Fatalf("Create = %v, want the injected error", err)
	}
	if _, err := fresh.Get(ctx, "absent"); !errors.Is(err, jetstream.ErrKeyNotFound) {
		t.Fatalf("key after a fail-before Create: %v, want ErrKeyNotFound", err)
	}
	if calls := kv.Calls(); calls[KVUpdate] != 1 || calls[KVCreate] != 1 {
		t.Fatalf("Calls() = %v, want one bucket Update and one bucket Create (the first)", calls)
	}
}

// to a client dialled from URL, and what it delivers. nats.go refuses a publish above the announced
// limit before writing it, so the larger publish is refused on the broker's own figure.
func TestMaxPayloadIsSettable(t *testing.T) {
	const limit = 4096
	f := New(t, WithMaxPayload(limit))
	if err := f.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	nc, err := nats.Connect(f.URL(), nats.MaxReconnects(0))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer nc.Close()
	if got := nc.MaxPayload(); got != limit {
		t.Fatalf("broker announces max_payload %d, want %d", got, limit)
	}
	sub, err := nc.SubscribeSync("payload.bound")
	if err != nil {
		t.Fatal(err)
	}
	if err := nc.Publish("payload.bound", make([]byte, limit)); err != nil {
		t.Fatalf("publish of exactly %d bytes: %v", limit, err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), stopBound) // a failure bound, never reached when delivered
	defer cancel()
	msg, err := sub.NextMsgWithContext(ctx)
	if err != nil {
		t.Fatalf("the %d-byte message was not delivered: %v", limit, err)
	}
	if len(msg.Data) != limit {
		t.Fatalf("delivered %d bytes, want %d", len(msg.Data), limit)
	}
	if err := nc.Publish("payload.bound", make([]byte, limit+1)); !errors.Is(err, nats.ErrMaxPayload) {
		t.Fatalf("publish of %d bytes = %v, want nats.ErrMaxPayload", limit+1, err)
	}
}

// With no option the broker keeps its own default (1 MiB for nats-server), so the option is the
// only thing that moves it.
func TestMaxPayloadDefaultsToTheBroker(t *testing.T) {
	f := startFixture(t)
	nc, err := nats.Connect(f.URL(), nats.MaxReconnects(0))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer nc.Close()
	if got := nc.MaxPayload(); got != 1<<20 {
		t.Fatalf("broker announces max_payload %d with no option, want the nats-server default %d", got, 1<<20)
	}
}

// Requirement: nats-fixture/Connected value for a package's tests; Scenario: Close runs first, bounded
// A value opened through Open is closed while the fixture still serves: its own connection is
// still connected and the broker still answers the value's flush, under a deadline.
func TestOpenClosesBeforeTheFixtureStops(t *testing.T) {
	var (
		f                                *Fixture
		closeRan, hadDeadline, fixtureUp bool
		flushErr                         error
	)
	t.Run("open", func(t *testing.T) {
		f = startFixture(t)
		nc := Open(t, f, func(_ context.Context, url string) (*nats.Conn, func(context.Context) error, error) {
			nc, err := nats.Connect(url, nats.MaxReconnects(0))
			if err != nil {
				return nil, nil, err
			}
			return nc, func(ctx context.Context) error {
				closeRan = true
				_, hadDeadline = ctx.Deadline()
				own := conn(f)
				fixtureUp = own != nil && own.IsConnected()
				flushErr = nc.FlushWithContext(ctx)
				nc.Close()
				return nil
			}, nil
		})
		if !nc.IsConnected() {
			t.Fatal("Open returned a connection that is not connected")
		}
	})
	if !closeRan || !hadDeadline {
		t.Fatalf("close ran = %v with a deadline = %v; want both", closeRan, hadDeadline)
	}
	if !fixtureUp || flushErr != nil {
		t.Fatalf("at close the fixture's connection was up = %v and the broker answered with %v; want up and nil", fixtureUp, flushErr)
	}
	if rem := f.remaining(); len(rem) != 0 {
		t.Fatalf("the fixture's Stop left %v", rem)
	}
}
