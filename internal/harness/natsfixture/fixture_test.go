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
)

// Restart refuses before a successful Start and holds nothing afterwards (nats-fixture › "Restart
// before Start"); TestRestartRefusedOnceStopHasBegun covers the other half against Docker.
func TestRestartRefusesBeforeStart(t *testing.T) {
	var nilCtx context.Context
	f := New(t)
	if err := f.Restart(t.Context()); !errors.Is(err, errNotStarted) {
		t.Fatalf("Restart before Start = %v, want the not-started refusal", err)
	}
	if err := f.Restart(nilCtx); err == nil {
		t.Fatal("Restart with a nil context returned nil")
	}
	if rem := f.remaining(); len(rem) != 0 {
		t.Fatalf("a refused Restart holds %v", rem)
	}
}

// A nil context and a nil JetStream are each refused by name, and the refusals leave the fixture
// unused. The nil-context call passes the nil JetStream too, so only the text tells the two apart.
func TestNilContextsAndUnstartedJetStreamAreRefused(t *testing.T) {
	f := New(t)
	var nilCtx context.Context
	_, createNilJS := CreateStream(t.Context(), f.JetStream(), jetstream.FileStorage, "s")
	_, createNilCtx := CreateStream(nilCtx, f.JetStream(), jetstream.FileStorage, "s")
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"CreateStream on an unstarted fixture's JetStream", createNilJS, "nil JetStream"},
		{"CreateStream nil ctx", createNilCtx, "nil context"},
		{"Start nil ctx", f.Start(nilCtx), "nil context"},
		{"Stop nil ctx", f.Stop(nilCtx), "nil context"},
	} {
		if tc.err == nil || !strings.Contains(tc.err.Error(), tc.want) {
			t.Errorf("%s = %v, want an error naming %q", tc.name, tc.err, tc.want)
		}
	}
	if f.used || len(f.remaining()) != 0 {
		t.Errorf("refusals consumed the fixture or left it holding %v", f.remaining())
	}
}

func TestPreCancelledStartIsRefusedWithoutConsuming(t *testing.T) {
	f := New(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := f.Start(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Start = %v, want context.Canceled", err)
	}
	if f.used {
		t.Fatal("a pre-cancelled Start consumed the fixture")
	}
}

func TestErrorReportsEveryField(t *testing.T) {
	cause, cleanup := errors.New("dial refused"), errors.New("terminate: daemon gone")
	e := &phaseError{phase: "connect", containerID: "0123456789abcdef", parentErr: context.Canceled, cause: cause, cleanup: cleanup}
	msg := e.Error()
	for _, want := range []string{"phase connect", "container 0123456789ab", "parent context context canceled", "dial refused", "cleanup: terminate: daemon gone"} {
		if !strings.Contains(msg, want) {
			t.Errorf("%q lacks %q", msg, want)
		}
	}
	if !errors.Is(e, cause) || !errors.Is(e, cleanup) || !errors.Is(e, context.Canceled) {
		t.Error("Unwrap hides the cause, the cleanup error, or the parent context's error")
	}
	live := (&phaseError{phase: "start", cause: cause}).Error()
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

// M5: connect honours its context. A broker that accepts the TCP connection and
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
	nc, err := connect(ctx, "nats://"+ln.Addr().String())
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
		nc, err := connect(ctx, "nats://"+ln.Addr().String())
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
		if f.used {
			t.Errorf("max payload %d: the refusal consumed the fixture", n)
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
	// Start reads the pin first, before any Docker call: a pin without a digest fails the image
	// phase holding nothing. The embedded value is swapped for this test only; no test in this
	// package runs in parallel.
	t.Run("Start refuses before Docker", func(t *testing.T) {
		embedded := imagePin
		imagePin = "nats" + ":2.15.0-alpine\n"
		t.Cleanup(func() { imagePin = embedded })
		f := New(t)
		err := f.Start(t.Context())
		if err == nil || !strings.Contains(err.Error(), "phase image") || !strings.Contains(err.Error(), "not a digest reference") {
			t.Fatalf("Start = %v, want an image-phase error naming the missing digest", err)
		}
		if rem := f.remaining(); len(rem) != 0 {
			t.Fatalf("a refused image left the fixture holding %v", rem)
		}
	})
}
