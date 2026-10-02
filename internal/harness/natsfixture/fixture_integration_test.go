//go:build integration

package natsfixture

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/c360studio/semengine/internal/harness/lifecycletest"
	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/testcontainers/testcontainers-go"
)

// These are the owner tests S1-1..S1-8 of design "Lifecycle ownership proof". They need the
// integration runner: `task test:integration -- ./internal/harness/natsfixture/`.

// stopBound is the fresh finite authority these tests give a controlled Stop.
const stopBound = 60 * time.Second

// containerExists asks the Docker CLI, an oracle independent of the fixture's own absence check.
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

func readRecord(t *testing.T, f *Fixture) record {
	t.Helper()
	f.mu.Lock()
	dir, name := f.adm.evidenceDir, f.rec.Name
	f.mu.Unlock()
	data, err := os.ReadFile(filepath.Join(dir, "fixtures", name+".json"))
	if err != nil {
		t.Fatalf("evidence record: %v", err)
	}
	var r record
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatalf("evidence record: %v", err)
	}
	return r
}

// conn is the fixture's own connection, read in-package: the fixture exports no accessor for it,
// because Stop closes it.
func conn(f *Fixture) *nats.Conn {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.nc
}

func closed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// TestFixtureRoundTrip is the adopter's path: start, use JetStream and KV, stop, and find
// everything gone and recorded.
func TestFixtureRoundTrip(t *testing.T) {
	f := startFixture(t)
	ctx := t.Context()
	if !strings.HasPrefix(f.URL(), "nats://") || conn(f).Status() != nats.CONNECTED {
		t.Fatalf("URL %q status %v", f.URL(), conn(f).Status())
	}
	requireLeakCheckSees(t, f)
	stream := f.Name("orders")
	if _, err := f.CreateStream(ctx, stream, stream+".>"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.JetStream().Publish(ctx, stream+".1", []byte("one")); err != nil {
		t.Fatal(err)
	}
	kv, err := f.CreateKeyValue(ctx, f.Name("kv"))
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
	r := readRecord(t, f)
	if r.Container != id || r.MappedPort == "" || len(r.Stops) != 1 || r.Stops[0].Result != "ok" || len(r.Owned) != 2 {
		t.Fatalf("evidence record incomplete: %+v", r)
	}
	for _, p := range r.Attempts[0].Phases {
		t.Logf("start phase %-16s %5dms", p.Phase, p.ElapsedMS)
	}
}

// requireLeakCheckSees proves the session labels the fixture recorded, applied exactly as the
// runner's leak check applies them, select this fixture's live container. A label key that
// testcontainers does not use would make the leak check pass on anything; this is the check that
// fails instead.
func requireLeakCheckSees(t *testing.T, f *Fixture) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.adm.evidenceDir, "testcontainers-session"))
	if err != nil {
		t.Fatalf("session labels: %v", err)
	}
	var seen []string
	for _, label := range strings.Fields(string(data)) {
		out, err := exec.Command("docker", "ps", "-aq", "--no-trunc", "--filter", "label="+label).Output()
		if err != nil {
			t.Fatalf("docker ps --filter label=%s: %v", label, err)
		}
		seen = append(seen, strings.Fields(string(out))...)
	}
	if !slices.Contains(seen, f.containerID) {
		t.Fatalf("the runner's session filter %q does not select the fixture's container %s (selected %v)",
			strings.Fields(string(data)), f.containerID, seen)
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

// S1-1: a failure at each phase after the container exists removes it, records the phase, the
// container, and its log, terminates exactly once per attempt, and leaves Stop nothing to do.
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
			f := New(t)
			tc.inject(&f.deps)
			err := f.Start(t.Context())
			var fe *Error
			if !errors.As(err, &fe) || fe.Phase != tc.phase || !errors.Is(err, injected) {
				t.Fatalf("Start = %v, want an *Error at phase %s wrapping the injected cause", err, tc.phase)
			}
			if fe.ContainerID == "" || fe.ParentErr != nil || fe.Cleanup != nil {
				t.Fatalf("error fields: container %q parent %v cleanup %v", fe.ContainerID, fe.ParentErr, fe.Cleanup)
			}
			r := readRecord(t, f)
			if len(r.Attempts) != tc.attempts {
				t.Fatalf("%d attempt(s) recorded, want %d", len(r.Attempts), tc.attempts)
			}
			for _, a := range r.Attempts {
				if containerExists(t, a.Container) {
					t.Errorf("attempt %d container %s still exists", a.Attempt, a.Container)
				}
				last := a.Phases[len(a.Phases)-1]
				if last.Phase != string(tc.phase) || last.Error == "" || a.Logs == "" {
					t.Errorf("attempt %d record: last phase %+v logs %q", a.Attempt, last, a.Logs)
				}
				if logs, err := os.ReadFile(filepath.Join(f.adm.evidenceDir, "fixtures", a.Logs)); err != nil || !strings.Contains(string(logs), "Server is ready") {
					t.Errorf("attempt %d container log not captured: %v", a.Attempt, err)
				}
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

	// A transient mapped-port failure is the one case replaced: the second container serves.
	t.Run("mapped-port replacement", func(t *testing.T) {
		f := New(t)
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
		r := readRecord(t, f)
		if len(r.Attempts) != 2 || r.Attempts[0].Error == "" || containerExists(t, r.Attempts[0].Container) {
			t.Fatalf("replacement not recorded or first container kept: %+v", r.Attempts)
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

// S1-2: cancelling the test context while a phase is in I/O fails Start with context.Canceled,
// records the parent state, removes the container, and does not retry.
func TestS1_2CancellationDuringIO(t *testing.T) {
	for _, phase := range []Phase{PhaseStart, PhaseConnect} {
		t.Run(string(phase), func(t *testing.T) {
			f := New(t)
			entered, release := make(chan struct{}), make(chan struct{})
			switch phase {
			case PhaseStart:
				real := f.deps.start
				f.deps.start = func(ctx context.Context, req testcontainers.GenericContainerRequest) (testcontainers.Container, error) {
					c, err := real(ctx, req)
					close(entered)
					<-release
					return c, err
				}
			case PhaseConnect:
				real := f.deps.connect
				f.deps.connect = func(ctx context.Context, url string) (*nats.Conn, error) {
					nc, err := real(ctx, url)
					close(entered)
					<-release
					return nc, err
				}
			}
			ctx, cancel := context.WithCancel(t.Context())
			result := make(chan error, 1)
			go func() { result <- f.Start(ctx) }()
			<-entered
			cancel()
			close(release)
			err := <-result
			var fe *Error
			if !errors.Is(err, context.Canceled) || !errors.As(err, &fe) || fe.Phase != phase {
				t.Fatalf("Start = %v, want context.Canceled at phase %s", err, phase)
			}
			if !errors.Is(fe.ParentErr, context.Canceled) || fe.Cleanup != nil {
				t.Fatalf("parent state %v cleanup %v", fe.ParentErr, fe.Cleanup)
			}
			if containerExists(t, fe.ContainerID) {
				t.Fatalf("container %s survives a cancelled Start", fe.ContainerID)
			}
			if r := readRecord(t, f); len(r.Attempts) != 1 || r.Attempts[0].ParentErr == "" {
				t.Fatalf("want one attempt recording the parent state, got %+v", r.Attempts)
			}
			if n := f.callCounts()["start"]; n != 1 {
				t.Fatalf("start called %d times; a cancelled Start must not retry", n)
			}
		})
	}
}

// consumeBlocked starts a fixture with one stream and one consumer whose handler blocks in cb.
func consumeBlocked(t *testing.T) (*Fixture, *probe.Callback, jetstream.Consumer, string) {
	t.Helper()
	f := startFixture(t)
	cb := probe.NewCallback()
	t.Cleanup(cb.Release) // runs before the fixture's own cleanup Stop
	stream := f.Name("work")
	if _, err := f.CreateStream(t.Context(), stream, stream+".>"); err != nil {
		t.Fatal(err)
	}
	jc, err := f.Consume(t.Context(), stream, "worker", func(ctx context.Context, msg jetstream.Msg) {
		cb.Block(ctx)
		_ = msg.Ack()
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.JetStream().Publish(t.Context(), stream+".1", []byte("job")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-cb.Entered():
	case <-t.Context().Done():
		t.Fatal("callback never entered")
	}
	return f, cb, jc, stream
}

// awaitDeliveryStopped waits until Stop has stopped the consumer's delivery, the step just before
// it waits to join the handler.
func awaitDeliveryStopped(t *testing.T, f *Fixture) {
	t.Helper()
	_, err := probe.Await(t.Context(), func(context.Context) (bool, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if len(f.consumers) == 0 {
			return false, errors.New("no consumer owned")
		}
		c := f.consumers[0]
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.stopping, nil
	}, func(stopping bool) bool { return stopping })
	if err != nil {
		t.Fatal(err)
	}
}

// S1-3: Stop under live Start authority waits for a blocked callback. While it waits the
// callback's context is live, the consumer exists, and the connection is up; the join completes
// before Stop returns, and only then are consumer, stream, connection, and container removed.
func TestS1_3BlockedCallbackDelaysFinalisation(t *testing.T) {
	f, cb, jc, _ := consumeBlocked(t)
	id, nc := f.containerID, conn(f)
	stopCtx, cancel := context.WithTimeout(t.Context(), stopBound)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- f.Stop(stopCtx) }()
	awaitDeliveryStopped(t, f)

	if _, err := jc.Info(t.Context()); err != nil {
		t.Fatalf("consumer gone while its callback is blocked: %v", err)
	}
	if nc.Status() != nats.CONNECTED {
		t.Fatalf("connection %v while the callback is blocked", nc.Status())
	}
	if !containerExists(t, id) {
		t.Fatal("container gone while the callback is blocked")
	}
	select {
	case err := <-result:
		t.Fatalf("Stop returned %v before the callback was released", err)
	default:
	}

	cb.Release()
	err := <-result
	joined := closed(cb.Joined())
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !joined {
		t.Fatal("Stop returned before the callback had joined")
	}
	if cbErr := cb.ContextErrAtRelease(); cbErr != nil {
		t.Fatalf("callback context ended before its release: %v", cbErr)
	}
	if !nc.IsClosed() || containerExists(t, id) || len(f.remaining()) != 0 {
		t.Fatalf("after Stop: conn closed %t, container exists, remaining %v", nc.IsClosed(), f.remaining())
	}
	var order []string
	for _, p := range readRecord(t, f).Stops[0].Phases {
		order = append(order, strings.Fields(p.Phase)[0])
	}
	if want := []string{"publish-complete", "consumer", "stream", "connection", "container"}; !slices.Equal(order, want) {
		t.Fatalf("stop order %v, want %v", order, want)
	}
}

// S1-4: Stop waits for in-flight async publishes and records what the stream held before it
// deleted it.
func TestS1_4GracefulFinalisation(t *testing.T) {
	f := startFixture(t)
	stream := f.Name("events")
	if _, err := f.CreateStream(t.Context(), stream, stream+".>"); err != nil {
		t.Fatal(err)
	}
	const n = 200
	for i := range n {
		if _, err := f.JetStream().PublishAsync(fmt.Sprintf("%s.%d", stream, i), []byte("e")); err != nil {
			t.Fatal(err)
		}
	}
	if err := stop(t, f); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if got := readRecord(t, f).StreamMsgs[stream]; got != n {
		t.Fatalf("stream held %d messages when finalisation reached it, want %d", got, n)
	}
}

// S1-5: a Stop whose deadline expires while a callback is blocked returns DeadlineExceeded,
// retains every handle, and leaves the container; after release a later Stop completes.
func TestS1_5DeadlineIsNotAJoin(t *testing.T) {
	f, cb, _, stream := consumeBlocked(t)
	id := f.containerID
	short, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	err := f.Stop(short)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop = %v, want context.DeadlineExceeded", err)
	}
	rem := f.remaining()
	for _, want := range []string{"consumer " + stream + "/", "stream " + stream, "connection", "container "} {
		if !slices.ContainsFunc(rem, func(s string) bool { return strings.HasPrefix(s, want) }) {
			t.Errorf("remaining %v lacks %q", rem, want)
		}
	}
	if !containerExists(t, id) {
		t.Fatal("container removed although Stop did not complete")
	}
	cb.Release()
	if err := stop(t, f); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
	if len(f.remaining()) != 0 || containerExists(t, id) {
		t.Fatalf("after second Stop: remaining %v", f.remaining())
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
		lifecycletest.Run(t, func() lifecycletest.Owner { return owner{New(t)} }, lifecycletest.Promise{Restart: false})
	})
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
	if a.containerID == b.containerID || a.URL() == b.URL() || a.Name("x") == b.Name("x") {
		t.Fatalf("fixtures collide: %s %s / %s %s", a.containerID, a.URL(), b.containerID, b.URL())
	}
	for _, n := range []string{a.Name("x"), b.Name("x")} {
		if err := CheckName(n); err != nil {
			t.Fatal(err)
		}
	}
	bID := b.containerID
	if err := stop(t, a); err != nil {
		t.Fatal(err)
	}
	if !containerExists(t, bID) {
		t.Fatal("stopping one fixture removed the other's container")
	}
	if _, err := b.JetStream().AccountInfo(t.Context()); err != nil {
		t.Fatalf("the other fixture stopped answering: %v", err)
	}
	t.Logf("isolated: %s at %s and %s at %s", shortID(a.rec.Container), a.rec.MappedPort, shortID(bID), b.rec.MappedPort)
}

// stopRunning starts Stop in the background and returns its result channel once Stop has stopped
// the consumer's delivery, or fails if Stop returns first: a Stop that never reaches the join
// cannot be observed waiting on it.
func stopRunning(t *testing.T, f *Fixture) <-chan error {
	t.Helper()
	stopCtx, cancel := context.WithTimeout(t.Context(), stopBound)
	t.Cleanup(cancel)
	result := make(chan error, 1)
	go func() { result <- f.Stop(stopCtx) }()
	stopping := make(chan struct{})
	go func() {
		_, _ = probe.Await(stopCtx, func(context.Context) (bool, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			if len(f.consumers) == 0 {
				return false, nil
			}
			c := f.consumers[0]
			c.mu.Lock()
			defer c.mu.Unlock()
			return c.stopping, nil
		}, func(stopping bool) bool { return stopping })
		close(stopping)
	}()
	select {
	case err := <-result:
		t.Fatalf("Stop returned %v without stopping the consumer's delivery while its handler was blocked", err)
	case <-stopping:
	}
	return result
}

// H1: with the connection already closed (the broker went away; the fixture connects with
// MaxReconnects(0)), Stop still stops delivery, joins the running handler, and only then cancels
// its context. Only the server-side deletes are skipped: the container's removal takes them.
func TestStopJoinsHandlersOnAClosedConnection(t *testing.T) {
	f := startFixture(t)
	cb := probe.NewCallback()
	t.Cleanup(cb.Release)
	stream := f.Name("work")
	if _, err := f.CreateStream(t.Context(), stream, stream+".>"); err != nil {
		t.Fatal(err)
	}
	handlerCtx := make(chan context.Context, 1)
	if _, err := f.Consume(t.Context(), stream, "worker", func(ctx context.Context, msg jetstream.Msg) {
		handlerCtx <- ctx
		cb.Block(ctx)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.JetStream().Publish(t.Context(), stream+".1", []byte("job")); err != nil {
		t.Fatal(err)
	}
	<-cb.Entered()
	hctx := <-handlerCtx
	id := f.containerID
	conn(f).Close()

	result := stopRunning(t, f)
	select {
	case err := <-result:
		t.Fatalf("Stop returned %v while the handler was still running", err)
	default:
	}
	if err := hctx.Err(); err != nil {
		t.Fatalf("handler context ended before its handler returned: %v", err)
	}
	cb.Release()
	err := <-result
	joined := closed(cb.Joined())
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !joined {
		t.Fatal("Stop returned before the handler had joined")
	}
	if cbErr := cb.ContextErrAtRelease(); cbErr != nil {
		t.Fatalf("handler context ended before its release: %v", cbErr)
	}
	if hctx.Err() == nil {
		t.Fatal("Stop returned nil with the joined handler's context still live")
	}
	if rem := f.remaining(); len(rem) != 0 || containerExists(t, id) {
		t.Fatalf("after Stop: remaining %v", rem)
	}
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
	file, mem := f.Name("file"), f.Name("mem")
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
// fixture, and a fresh dial to it reaches the restarted broker.
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
	t.Logf("binding %s -> %s", old, f.URL())
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

// A consumer whose handler is running is ended before Restart returns: Restart waits for the
// handler, no handler of that consumer runs afterwards, and only a new Consume delivers again.
// Stop still succeeds and removes the consumer.
func TestRestartEndsConsumers(t *testing.T) {
	f := startFixture(t)
	cb := probe.NewCallback()
	t.Cleanup(cb.Release)
	stream := f.Name("work")
	if _, err := f.CreateStream(t.Context(), stream, stream+".>"); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	if _, err := f.Consume(t.Context(), stream, "worker", func(ctx context.Context, msg jetstream.Msg) {
		calls.Add(1)
		cb.Block(ctx)
		_ = msg.Ack()
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.JetStream().Publish(t.Context(), stream+".1", []byte("job")); err != nil {
		t.Fatal(err)
	}
	<-cb.Entered()

	ctx, cancel := context.WithTimeout(t.Context(), stopBound)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- f.Restart(ctx) }()
	awaitDeliveryStopped(t, f)
	select {
	case err := <-result:
		t.Fatalf("Restart returned %v while the handler was running", err)
	default:
	}
	cb.Release()
	err := <-result
	joined := closed(cb.Joined())
	if err != nil {
		t.Fatalf("Restart: %v", err)
	}
	if !joined {
		t.Fatal("Restart returned before the handler had joined")
	}

	// A fresh consumer receiving a message published after the restart is the positive control:
	// once it has, the ended consumer has had every chance to run its handler too.
	fresh := make(chan struct{}, 1)
	if _, err := f.Consume(t.Context(), stream, "fresh", func(_ context.Context, msg jetstream.Msg) {
		if string(msg.Data()) == "after" {
			select {
			case fresh <- struct{}{}:
			default:
			}
		}
		_ = msg.Ack()
	}); err != nil {
		t.Fatalf("Consume after restart: %v", err)
	}
	if _, err := f.JetStream().Publish(t.Context(), stream+".2", []byte("after")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-fresh:
	case <-t.Context().Done():
		t.Fatal("the new consumer never received the message published after the restart")
	}
	if n := calls.Load(); n != 1 {
		t.Fatalf("the ended consumer's handler ran %d times, want 1 (before the restart only)", n)
	}
	id := f.containerID
	if err := stop(t, f); err != nil {
		t.Fatalf("Stop after Restart: %v", err)
	}
	if rem := f.remaining(); len(rem) != 0 || containerExists(t, id) {
		t.Fatalf("after Stop: remaining %v", rem)
	}
}

// sessionContainersLike lists the containers of this run's testcontainers session whose name
// shares the fixture container's name up to its random suffix: every container this fixture could
// have created, read from Docker rather than from the fixture.
func sessionContainersLike(t *testing.T, f *Fixture, id string) []string {
	t.Helper()
	out, err := exec.Command("docker", "inspect", "--format", "{{.Name}}", id).Output()
	if err != nil {
		t.Fatalf("docker inspect %s: %v", id, err)
	}
	name := strings.TrimPrefix(strings.TrimSpace(string(out)), "/")
	prefix := name[:strings.LastIndex(name, "-")+1]
	data, err := os.ReadFile(filepath.Join(f.adm.evidenceDir, "testcontainers-session"))
	if err != nil {
		t.Fatalf("session labels: %v", err)
	}
	var ids []string
	for _, label := range strings.Fields(string(data)) {
		out, err := exec.Command("docker", "ps", "-a", "--no-trunc", "--filter", "label="+label, "--format", "{{.ID}} {{.Names}}").Output()
		if err != nil {
			t.Fatalf("docker ps --filter label=%s: %v", label, err)
		}
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if fields := strings.Fields(line); len(fields) == 2 && strings.HasPrefix(fields[1], prefix) && !slices.Contains(ids, fields[0]) {
				ids = append(ids, fields[0])
			}
		}
	}
	return ids
}

// Restart fault matrix (task 2.2): each container hook fails in turn, before and after its real
// call. Restart returns an *Error naming exactly that phase, runs no later phase, creates no second
// container, and Stop still removes the one container and observes it gone.
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
			if got := sessionContainersLike(t, f, id); len(got) != 1 || got[0] != id {
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

// M1: once Stop has begun, the fixture refuses to create anything; a resource created behind
// Stop's back would be owned by nobody when Stop returned nil.
func TestNoCreationOnceStopBegins(t *testing.T) {
	f, cb, _, _ := consumeBlocked(t)
	result := stopRunning(t, f)
	late := f.Name("late")
	// Refused at once, not queued behind Stop: Stop is parked on a handler only the test releases.
	_, err := f.CreateStream(t.Context(), late, late+".>")
	cb.Release()
	if stopErr := <-result; stopErr != nil {
		t.Fatalf("Stop: %v", stopErr)
	}
	if err == nil {
		t.Fatal("CreateStream succeeded after Stop began")
	}
	if rem := f.remaining(); len(rem) != 0 {
		t.Fatalf("Stop returned nil while still owning %v", rem)
	}
	if _, err := f.CreateKeyValue(t.Context(), f.Name("after")); err == nil {
		t.Fatal("CreateKeyValue succeeded after Stop")
	}
	if rem := f.remaining(); len(rem) != 0 {
		t.Fatalf("a refused creation is owned: %v", rem)
	}
}
