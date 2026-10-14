package natsfixture

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/testcontainers/testcontainers-go"
)

// startedWith returns a fixture that looks started to its resource methods, with dependency
// calls captured instead of sent: enough to prove what the fixture asks the broker for.
func startedWith(t *testing.T) *Fixture {
	t.Helper()
	f := &Fixture{deps: defaultDeps(t.Logf), calls: map[string]int{}, containerID: "c0ffee"}
	f.js = struct{ jetstream.JetStream }{} // never called: every JetStream use below goes through deps
	return f
}

// Every stream the fixture creates declares its bounds (nats-fixture › "Bounded test
// streams"); the values are restated here, not read from the implementation.
func TestCreateStreamDeclaresBounds(t *testing.T) {
	f := startedWith(t)
	var got jetstream.StreamConfig
	f.deps.createStream = func(_ context.Context, _ jetstream.JetStream, cfg jetstream.StreamConfig) (jetstream.Stream, error) {
		got = cfg
		return nil, nil
	}
	if _, err := f.CreateStream(t.Context(), "orders", "orders.>"); err != nil {
		t.Fatal(err)
	}
	if got.Name != "orders" || len(got.Subjects) != 1 || got.MaxAge != time.Hour || got.MaxBytes != 64<<20 || got.Discard != jetstream.DiscardOld {
		t.Fatalf("stream config %+v lacks the declared bounds", got)
	}
}

// A memory-backed stream is bounded like a file-backed one (nats-fixture › "Bounded test
// streams"); only the storage differs.
func TestCreateMemoryStreamDeclaresBoundsAndMemoryStorage(t *testing.T) {
	f := startedWith(t)
	var got jetstream.StreamConfig
	f.deps.createStream = func(_ context.Context, _ jetstream.JetStream, cfg jetstream.StreamConfig) (jetstream.Stream, error) {
		got = cfg
		return nil, nil
	}
	if _, err := f.CreateMemoryStream(t.Context(), "scratch", "scratch.>"); err != nil {
		t.Fatal(err)
	}
	if got.Name != "scratch" || len(got.Subjects) != 1 || got.MaxAge != time.Hour || got.MaxBytes != 64<<20 ||
		got.Discard != jetstream.DiscardOld || got.Storage != jetstream.MemoryStorage {
		t.Fatalf("stream config %+v lacks memory storage or the declared bounds", got)
	}
	var nilCtx context.Context
	if _, err := f.CreateMemoryStream(nilCtx, "s"); err == nil {
		t.Fatal("CreateMemoryStream accepted a nil context")
	}
}

// Restart refuses before a successful Start with no Docker call (nats-fixture › "Restart before
// Start"); TestRestartRefusedOnceStopHasBegun covers the other half against Docker.
func TestRestartRefusesWithoutDockerCall(t *testing.T) {
	var nilCtx context.Context
	t.Run("before Start", func(t *testing.T) {
		f := New(t)
		if err := f.Restart(t.Context()); err == nil {
			t.Fatal("Restart before Start returned nil")
		}
		if err := f.Restart(nilCtx); err == nil {
			t.Fatal("Restart with a nil context returned nil")
		}
		if f.totalCalls() != 0 {
			t.Fatalf("refused Restart made calls: %v", f.callCounts())
		}
	})
}

// The container's log keeps every boot's lines, and testcontainers' own wait counts them all
// (wait/log.go:210), so it passes at once on a restart. Restart connects only after the restarted
// server has logged one more ready line than before, and reports a later phase's failure by name
// without replacing the container.
func TestRestartConnectsOnlyAfterANewReadyLine(t *testing.T) {
	f := startedWith(t)
	f.container, f.nc, f.url = struct{ testcontainers.Container }{}, &nats.Conn{}, "nats://fixture-host:1"
	boots, booting, reads := 1, false, 0
	f.deps.drain = func(context.Context, *nats.Conn) error { return nil }
	f.deps.stopContainer = func(context.Context, testcontainers.Container) error { return nil }
	f.deps.startContainer = func(context.Context, testcontainers.Container) error {
		booting = true // started, but not yet ready: the line comes on the third read
		return nil
	}
	f.deps.logs = func(context.Context, testcontainers.Container) ([]byte, error) {
		if booting {
			if reads++; reads == 3 {
				boots, booting = boots+1, false
			}
		}
		return []byte(strings.Repeat(readyLog+"\n", boots)), nil
	}
	f.deps.mappedPort = func(context.Context, testcontainers.Container) (string, error) { return "2", nil }
	refused := errors.New("connection refused")
	bootsAtConnect, dialled := 0, ""
	f.deps.connect = func(_ context.Context, url string) (*nats.Conn, error) {
		bootsAtConnect, dialled = boots, url
		return nil, refused
	}
	err := f.Restart(t.Context())
	var fe *Error
	if !errors.As(err, &fe) || fe.Phase != PhaseConnect || !errors.Is(err, refused) || fe.ContainerID != "c0ffee" {
		t.Fatalf("Restart = %v, want an *Error at phase %s wrapping the cause", err, PhaseConnect)
	}
	if bootsAtConnect != 2 {
		t.Fatalf("connect ran with %d ready line(s) in the log; want 2, the restarted server's included", bootsAtConnect)
	}
	if dialled != "nats://fixture-host:2" {
		t.Fatalf("dialled %q, want the re-read mapped port on the same host", dialled)
	}
	if calls := f.callCounts(); calls["start"] != 0 || calls["terminate"] != 0 || calls["restart"] != 1 {
		t.Fatalf("calls %v: a failed Restart replaced or removed the container", calls)
	}
	if err := f.Restart(t.Context()); !errors.Is(err, errNotStarted) {
		t.Fatalf("Restart after a failed Restart = %v, want the not-started refusal", err)
	}
}

// The Restart fault matrix's later phases (task 2.2; TestRestartFaultMatrix covers the container
// hooks against Docker): a failure re-reading the mapped port or waiting for JetStream returns an
// *Error naming that phase and wrapping the cause, runs no later phase, publishes no new binding,
// replaces no container, and leaves the fixture refusing a further Restart.
func TestRestartLaterPhaseFailuresAreNamed(t *testing.T) {
	injected := errors.New("injected failure")
	for _, tc := range []struct {
		phase Phase
		set   func(d *deps)
		// later is the dependency count that must stay zero: the next phase never ran.
		later string
	}{
		{PhaseMappedPort, func(d *deps) {
			d.mappedPort = func(context.Context, testcontainers.Container) (string, error) { return "", injected }
		}, "connect"},
		{PhaseJetStream, func(d *deps) {
			d.jsReady = func(context.Context, jetstream.JetStream) error { return injected }
		}, ""},
	} {
		t.Run(string(tc.phase), func(t *testing.T) {
			f := restartOnFakes(t)
			tc.set(&f.deps)

			err := f.Restart(t.Context())
			var fe *Error
			if !errors.As(err, &fe) || fe.Phase != tc.phase || !errors.Is(err, injected) || fe.ContainerID != "c0ffee" || fe.ParentErr != nil {
				t.Fatalf("Restart = %v, want an *Error at phase %s wrapping the injected cause", err, tc.phase)
			}
			calls := f.callCounts()
			if tc.later != "" && calls[tc.later] != 0 {
				t.Fatalf("a phase after %s ran: %s called %d times", tc.phase, tc.later, calls[tc.later])
			}
			if calls["start"] != 0 || calls["terminate"] != 0 || calls["restart"] != 1 {
				t.Fatalf("calls %v: a failed Restart replaced or removed the container", calls)
			}
			if f.URL() != "" || f.JetStream() != nil {
				t.Fatalf("after a failed Restart: URL %q, JetStream %v; want no binding published", f.URL(), f.JetStream())
			}
			if err := f.Restart(t.Context()); !errors.Is(err, errNotStarted) {
				t.Fatalf("Restart after a failed Restart = %v, want the not-started refusal", err)
			}
		})
	}
}

// A create that fails returns a typed error naming the phase and the container.
func TestFailedCreateIsTyped(t *testing.T) {
	f := startedWith(t)
	cause := errors.New("timeout")
	f.deps.createStream = func(context.Context, jetstream.JetStream, jetstream.StreamConfig) (jetstream.Stream, error) {
		return nil, cause
	}
	_, err := f.CreateStream(t.Context(), "s")
	var fe *Error
	if !errors.As(err, &fe) || fe.Phase != PhaseCreateStream || !errors.Is(err, cause) || fe.ContainerID != "c0ffee" {
		t.Fatalf("CreateStream = %v, want a typed create-stream error", err)
	}
}

func TestResourceMethodsRefuseBeforeStartAndNilContexts(t *testing.T) {
	f := New(t)
	var nilCtx context.Context
	checks := map[string]error{}
	_, checks["CreateStream before Start"] = f.CreateStream(t.Context(), "s")
	_, checks["CreateStream nil ctx"] = f.CreateStream(nilCtx, "s")
	checks["Start nil ctx"] = f.Start(nilCtx)
	checks["Stop nil ctx"] = f.Stop(nilCtx)
	for name, err := range checks {
		if err == nil {
			t.Errorf("%s returned nil", name)
		}
	}
	if f.totalCalls() != 0 {
		t.Errorf("refusals made calls: %v", f.callCounts())
	}
}

func TestPreCancelledStartIsRefusedWithoutConsuming(t *testing.T) {
	f := New(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := f.Start(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Start = %v, want context.Canceled", err)
	}
	if f.used || f.totalCalls() != 0 {
		t.Fatal("a pre-cancelled Start consumed the fixture or made a call")
	}
}

func TestErrorReportsEveryField(t *testing.T) {
	cause, cleanup := errors.New("dial refused"), errors.New("terminate: daemon gone")
	e := &Error{Phase: PhaseConnect, ContainerID: "0123456789abcdef", ParentErr: context.Canceled, Cause: cause, Cleanup: cleanup}
	msg := e.Error()
	for _, want := range []string{"phase connect", "container 0123456789ab", "parent context context canceled", "dial refused", "cleanup: terminate: daemon gone"} {
		if !strings.Contains(msg, want) {
			t.Errorf("%q lacks %q", msg, want)
		}
	}
	if !errors.Is(e, cause) || !errors.Is(e, cleanup) || !errors.Is(e, context.Canceled) {
		t.Error("Unwrap hides the cause, the cleanup error, or the parent context's error")
	}
	live := (&Error{Phase: PhaseStart, Cause: cause}).Error()
	if !strings.Contains(live, "parent context live") || strings.Contains(live, "container") {
		t.Errorf("%q", live)
	}
}

func TestRollbackIgnoresParentCancellationButStaysBounded(t *testing.T) {
	var nilCtx context.Context
	if err := rollback(nilCtx, func(context.Context) error { return nil }); err == nil {
		t.Fatal("rollback accepted a nil parent")
	}
	parent, cancel := context.WithCancel(t.Context())
	cancel()
	var sawErr error
	var sawDeadline bool
	if err := rollback(parent, func(ctx context.Context) error {
		sawErr = ctx.Err()
		_, sawDeadline = ctx.Deadline()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if sawErr != nil || !sawDeadline {
		t.Fatalf("rollback context: err %v deadline %t; want live and bounded after parent cancellation", sawErr, sawDeadline)
	}
}

// M5: the connect dependency honours its context. A broker that accepts the TCP connection and
// never speaks would otherwise hold Start for the whole dial timeout after the caller gave up.
func TestConnectHonoursItsContext(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			defer func() { _ = c.Close() }() // held open, silent, until the listener closes
		}
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	began := time.Now()
	nc, err := defaultDeps(t.Logf).connect(ctx, "nats://"+ln.Addr().String())
	if nc != nil {
		nc.Close()
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("connect = %v after %s, want context.DeadlineExceeded", err, time.Since(began))
	}
	if elapsed := time.Since(began); elapsed > dialTimeout/2 {
		t.Fatalf("connect returned after %s; its context ended at 100ms", elapsed)
	}
}

// connect leaves nothing behind (background-work rule): once it has returned, its dial has
// ended. The broker accepts and stays silent until connect has given up, then speaks; a dial still
// running would answer with CONNECT, an ended one has closed the connection.
func TestConnectJoinsItsDial(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	accepted := make(chan net.Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			close(accepted)
			return
		}
		accepted <- c
	}()
	ctx, cancel := context.WithCancel(t.Context())
	type result struct {
		nc  *nats.Conn
		err error
	}
	done := make(chan result, 1)
	go func() {
		nc, err := defaultDeps(t.Logf).connect(ctx, "nats://"+ln.Addr().String())
		done <- result{nc, err}
	}()
	srv, ok := <-accepted
	if !ok {
		t.Fatal("connect never dialled")
	}
	defer func() { _ = srv.Close() }()
	cancel()
	r := <-done
	if r.nc != nil {
		r.nc.Close()
	}
	if !errors.Is(r.err, context.Canceled) {
		t.Fatalf("connect = %v, want context.Canceled", r.err)
	}
	// A failure bound, not pacing: a closed connection answers the read at once.
	if err := srv.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.Write([]byte(`INFO {"server_id":"x","version":"2.14.0","proto":1,"max_payload":1048576}` + "\r\n")); err != nil {
		return // the client side is closed: nothing is dialling
	}
	buf := make([]byte, 64)
	n, err := srv.Read(buf)
	if err == nil {
		t.Fatalf("connect returned but its dial is still running: the client answered %q", buf[:n])
	}
	if errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatal("the client neither answered nor closed within the failure bound")
	}
}

// A max payload the broker cannot honour is refused by Start before any Docker call, not silently
// replaced by the broker default.
func TestNonPositiveMaxPayloadIsRefused(t *testing.T) {
	for _, n := range []int64{0, -1} {
		f := New(t, WithMaxPayload(n))
		// The refusal must name its cause.
		if err := f.Start(t.Context()); err == nil || !strings.Contains(err.Error(), "max payload") {
			t.Errorf("Start with max payload %d = %v, want a refusal naming the max payload", n, err)
		}
		if f.totalCalls() != 0 || f.used {
			t.Errorf("max payload %d: the refusal made calls %v or consumed the fixture", n, f.callCounts())
		}
	}
}

// harness-boundaries › "One image pin": the fixture takes its image from the pin file it embeds,
// which must hold exactly one digest reference, and refuses any other before a Docker call. Image
// strings are assembled so that this file spells no image (TestOneImagePin).
func TestImagePin(t *testing.T) {
	digest := "@sha256:" + strings.Repeat("0", 64)
	image, err := pinnedImage(imagePin)
	if err != nil || !strings.HasPrefix(image, "nats:") || !strings.Contains(image, "@sha256:") {
		t.Fatalf("the embedded pin: image %q, err %v; want one digest reference", image, err)
	}
	for _, tc := range []struct{ name, file, want string }{
		{"tag without a digest", "nats" + ":2.15.0-alpine\n", "not a digest reference"},
		{"another repository", "natsio/nats-server" + ":2.15.0" + digest + "\n", "not a digest reference"},
		{"two images", "nats" + ":1" + digest + "\nnats" + ":2" + digest + "\n", "holds 2"},
		{"comments only", "# nothing pinned\n\n", "holds 0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := pinnedImage(tc.file); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("pinnedImage = %q, %v; want an error naming %q", got, err, tc.want)
			}
		})
	}
	// Start reads the pin before any Docker call: a pin without a digest fails the image phase
	// with no dependency called. The embedded value is swapped for this test only; no test in this
	// package runs in parallel.
	t.Run("Start refuses before Docker", func(t *testing.T) {
		embedded := imagePin
		imagePin = "nats" + ":2.15.0-alpine\n"
		t.Cleanup(func() { imagePin = embedded })
		f := New(t)
		noDocker(t, f)
		err := f.Start(t.Context())
		var fe *Error
		if !errors.As(err, &fe) || fe.Phase != PhaseImage || !strings.Contains(err.Error(), "not a digest reference") {
			t.Fatalf("Start = %v, want an image-phase *Error naming the missing digest", err)
		}
		if calls := f.totalCalls(); calls != 0 {
			t.Fatalf("a refused image made %d dependency call(s): %v", calls, f.callCounts())
		}
	})
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

// M5: a Start whose context is cancelled while a dependency fails for its own reason still
// reports the cancellation to errors.Is: a caller asking "was I cancelled?" gets the truth.
func TestCancelledStartIsRecognisable(t *testing.T) {
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
