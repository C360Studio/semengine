//go:build integration

package natsfixture

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

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
	if _, err := CreateStream(ctx, f.JetStream(), jetstream.FileStorage, stream, stream+".>"); err != nil {
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

// Every stream CreateStream creates declares its bounds, as the broker reports them (nats-fixture ›
// "Bounded test streams"); the values are restated here, not read from the implementation. Only
// the storage differs between the two.
func TestStreamsDeclareBounds(t *testing.T) {
	f := startFixture(t)
	ctx := t.Context()
	file, err := CreateStream(ctx, f.JetStream(), jetstream.FileStorage, "orders", "orders.>")
	if err != nil {
		t.Fatal(err)
	}
	mem, err := CreateStream(ctx, f.JetStream(), jetstream.MemoryStorage, "scratch", "scratch.>")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		stream  jetstream.Stream
		storage jetstream.StorageType
	}{{file, jetstream.FileStorage}, {mem, jetstream.MemoryStorage}} {
		info, err := tc.stream.Info(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if c := info.Config; c.Storage != tc.storage || c.MaxAge != time.Hour || c.MaxBytes != 64<<20 || c.Discard != jetstream.DiscardOld {
			t.Errorf("stream %s as the broker reports it: storage %v max age %v max bytes %d discard %v; want %v, 1h, 64 MiB, old",
				c.Name, c.Storage, c.MaxAge, c.MaxBytes, c.Discard, tc.storage)
		}
	}
}

// errNotFound is the error testcontainers' MappedPort returns when Docker Desktop has left the
// container's port list empty (#180).
var errNotFound = errors.New(`port "4222/tcp" not found`)

// stopHoldsNothing reports whether Stop under an ended context returns nil. Stop returns nil with
// no Docker call when the fixture holds nothing, and refuses with the context's error when it
// holds anything, so nil means nothing was held and no call was made.
func stopHoldsNothing(t *testing.T, f *Fixture) bool {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	return f.Stop(ctx) == nil
}

// deadPort returns a port nothing listens on: the operating system handed it out and it was
// closed again at once.
func deadPort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

// S1-1: a failure in a phase after the container exists removes it, names the phase and the
// container, writes the container's log to the test log once, and leaves Stop nothing to do
// (nats-fixture › "Failure after the container exists", "A missing mapped port fails Start"). The
// fixture's one test seam, the mapped-port read, drives both rows: it fails the read, or it reports
// a port nothing listens on, so the real connect fails.
func TestS1_1PartialStartupCleanup(t *testing.T) {
	for _, tc := range []struct {
		phase string
		read  func(t *testing.T) (string, error)
	}{
		{"mapped-port", func(*testing.T) (string, error) { return "", errNotFound }},
		{"connect", func(t *testing.T) (string, error) { return deadPort(t), nil }},
	} {
		t.Run(tc.phase, func(t *testing.T) {
			tb := &logTB{TB: t}
			f := New(tb)
			var id string
			f.readPort = func(_ context.Context, c testcontainers.Container) (string, error) {
				id = c.GetContainerID()
				return tc.read(t)
			}
			err := f.Start(t.Context())
			if err == nil || id == "" {
				t.Fatalf("Start = %v with container %q, want a failure after the container exists", err, id)
			}
			for _, want := range []string{"phase " + tc.phase, "container " + shortID(id), "(parent context live)"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("Start error %q lacks %q", err, want)
				}
			}
			if strings.Contains(err.Error(), "cleanup:") {
				t.Errorf("Start error %q reports a failed cleanup", err)
			}
			if containerExists(t, id) {
				t.Errorf("container %s still exists", id)
			}
			if got := processContainers(t); len(got) != 0 {
				t.Errorf("containers of this process after the failed Start: %v, want none", got)
			}
			if got := tb.containerLogs(); got != 1 {
				t.Errorf("%d container log(s) in the test log, want one: one container started", got)
			}
			if !stopHoldsNothing(t, f) {
				t.Errorf("after a rolled-back Start the fixture still holds %v", f.remaining())
			}
		})
	}

}

// S1-2: cancelling the test context while Start is in I/O fails Start with context.Canceled,
// reports the parent state, removes the container, and starts no other container. The mapped-port
// read succeeds and is then held until the test has cancelled: work that finishes after
// cancellation is not accepted, so Start fails in that phase rather than the next.
func TestS1_2CancellationDuringIO(t *testing.T) {
	tb := &logTB{TB: t}
	f := New(tb)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	entered, release := make(chan struct{}), make(chan struct{})
	var id string
	f.readPort = func(ctx context.Context, c testcontainers.Container) (string, error) {
		id = c.GetContainerID()
		port, err := readMappedPort(ctx, c)
		close(entered)
		<-release
		return port, err
	}
	result := make(chan error, 1)
	go func() { result <- f.Start(ctx) }()
	select {
	case <-entered:
	case err := <-result:
		t.Fatalf("Start ended before the mapped-port read: %v", err)
	}
	cancel()
	close(release)
	err := <-result
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Start = %v, want context.Canceled", err)
	}
	for _, want := range []string{"phase mapped-port", "container " + shortID(id), "(parent context context canceled)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Start error %q lacks %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "cleanup:") {
		t.Errorf("Start error %q reports a failed cleanup", err)
	}
	if containerExists(t, id) {
		t.Fatalf("container %s survives cancellation", id)
	}
	if got := processContainers(t); len(got) != 0 {
		t.Fatalf("containers of this process after the cancelled Start: %v, want none", got)
	}
	if got := tb.containerLogs(); got != 1 {
		t.Fatalf("%d container log(s) in the test log, want one: one container started", got)
	}
}

// S1-6: a Stop after a successful Stop is a no-op: nil, and no Docker call.
func TestS1_6RepeatedStop(t *testing.T) {
	f := startFixture(t)
	if err := stop(t, f); err != nil {
		t.Fatal(err)
	}
	if !stopHoldsNothing(t, f) {
		t.Fatalf("after Stop the fixture still holds %v", f.remaining())
	}
}

// S1-7: a second Start is refused and leaves the running fixture untouched.
func TestS1_7Restart(t *testing.T) {
	f := startFixture(t)
	id, before := f.containerID, f.remaining()
	if err := f.Start(t.Context()); !errors.Is(err, ErrAlreadyUsed) {
		t.Fatalf("second Start = %v, want ErrAlreadyUsed", err)
	}
	if !slices.Equal(before, f.remaining()) || conn(f).Status() != nats.CONNECTED {
		t.Fatal("refused second Start changed the fixture")
	}
	if got := processContainers(t); !slices.Equal(got, []string{id}) {
		t.Fatalf("containers of this process after a refused second Start: %v, want only %s", got, id)
	}
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
	if _, err := CreateStream(ctx, f.JetStream(), jetstream.FileStorage, file, file+".>"); err != nil {
		t.Fatal(err)
	}
	ms, err := CreateStream(ctx, f.JetStream(), jetstream.MemoryStorage, mem, mem+".>")
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

	if got := processContainers(t); f.containerID != id || !slices.Equal(got, []string{id}) {
		t.Fatalf("Restart replaced the container: %s -> %s, containers of this process %v", id, f.containerID, got)
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

// nats-fixture › "Restart before Start", its second half: once Stop has begun, Restart refuses and
// starts no container. The container is removed behind the fixture's back with the Docker CLI, so
// Stop's own removal fails: Stop has begun, and keeps the container handle for a later Stop.
func TestRestartRefusedOnceStopHasBegun(t *testing.T) {
	f := startFixture(t)
	id := f.containerID
	if out, err := exec.Command("docker", "rm", "-f", id).CombinedOutput(); err != nil {
		t.Fatalf("docker rm -f %s: %v\n%s", shortID(id), err, out)
	}
	// Registered after New's cleanup, so it runs first: the container is gone, and New's Stop
	// would otherwise fail the test trying to remove it again.
	t.Cleanup(func() {
		f.mu.Lock()
		f.container, f.containerID = nil, ""
		f.mu.Unlock()
	})
	if err := stop(t, f); err == nil {
		t.Fatal("Stop of a container already removed returned nil")
	}
	if rem := f.remaining(); !slices.Equal(rem, []string{"container " + shortID(id)}) {
		t.Fatalf("after the failed Stop the fixture holds %v, want only the container", rem)
	}
	ctx, cancel := context.WithTimeout(t.Context(), stopBound)
	defer cancel()
	if err := f.Restart(ctx); !errors.Is(err, errNotStarted) {
		t.Fatalf("Restart after Stop began = %v, want the not-started refusal", err)
	}
	if got := processContainers(t); len(got) != 0 {
		t.Fatalf("containers of this process after the refused Restart: %v, want none", got)
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
