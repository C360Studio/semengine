package natsclient

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/pkg/errs"
)

// Tests of the change subscribe-registers-interest, design D7: T1 to T8. T1 runs against the
// embedded server. T2 to T8 run against a scripted server on a net.Pipe, which binds no address:
// T2 to T7 inside a synctest bubble, each in a test process of its own (ownProcessBubble), so a
// withheld PONG costs no wall time; T8 on real time, outside a bubble.

// scriptedServer is the server end of a net.Pipe. It answers nats.go's handshake, records every
// later line the client sends, answers or withholds each later PING as the test says, and
// delivers a message when told.
type scriptedServer struct {
	conn net.Conn

	mu       sync.Mutex
	lines    []string      // every line read after the handshake, in order
	answer   bool          // answer each later PING as it is read
	withheld int           // PINGs read and not yet answered
	pinged   chan struct{} // receives once for each later PING read
}

// serveScripted starts a scripted server on conn that withholds every PING after the handshake.
// Its goroutine ends when either end of the pipe is closed.
func serveScripted(conn net.Conn) *scriptedServer {
	s := &scriptedServer{conn: conn, pinged: make(chan struct{}, 64)}
	go s.serve()
	return s
}

func (s *scriptedServer) serve() {
	defer func() { _ = s.conn.Close() }()
	s.write(`INFO {"server_id":"scripted","version":"2.14.0","proto":1,"max_payload":1048576}` + "\r\n")
	r := bufio.NewReader(s.conn)
	handshake := true
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimSpace(line)
		if handshake {
			// nats.go sends CONNECT, then PING, and waits for this PONG before Connect returns.
			if line == "PING" {
				s.write("PONG\r\n")
				handshake = false
			}
			continue
		}
		s.mu.Lock()
		s.lines = append(s.lines, line)
		if line == "PING" {
			select {
			case s.pinged <- struct{}{}:
			default:
			}
			if s.answer {
				s.writeLocked("PONG\r\n")
			} else {
				s.withheld++
			}
		}
		s.mu.Unlock()
	}
}

// answerPings answers every withheld PING, in the order read, and every later one as it is read.
// nats.go matches each PONG to its oldest outstanding PING, a timed-out one included, so the
// withheld ones are answered first.
func (s *scriptedServer) answerPings() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.answer = true
	for ; s.withheld > 0; s.withheld-- {
		s.writeLocked("PONG\r\n")
	}
}

// deliver sends one empty message on subject to the subscription with sid.
func (s *scriptedServer) deliver(subject, sid string) {
	s.write(fmt.Sprintf("MSG %s %s 0\r\n\r\n", subject, sid))
}

// hangUp closes the server's end of the pipe: the client reads end of file.
func (s *scriptedServer) hangUp() { _ = s.conn.Close() }

func (s *scriptedServer) write(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writeLocked(text)
}

// writeLocked writes under mu, so lines from the serve goroutine and the test never interleave.
// A write after either end has closed fails; the test reads what the client saw instead.
func (s *scriptedServer) writeLocked(text string) {
	_, _ = s.conn.Write([]byte(text))
}

// read returns every line read after the handshake.
func (s *scriptedServer) read() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.lines...)
}

// sid returns the sid of the first SUB read for subject, or "" when none was read.
func (s *scriptedServer) sid(subject string) string {
	for _, line := range s.read() {
		if f := strings.Fields(line); len(f) >= 3 && f[0] == "SUB" && f[1] == subject {
			return f[len(f)-1]
		}
	}
	return ""
}

// readUnsub reports whether an UNSUB for sid was read.
func (s *scriptedServer) readUnsub(sid string) bool {
	for _, line := range s.read() {
		if f := strings.Fields(line); len(f) >= 2 && f[0] == "UNSUB" && f[1] == sid {
			return true
		}
	}
	return false
}

// subLines returns every SUB line read.
func (s *scriptedServer) subLines() []string {
	var subs []string
	for _, line := range s.read() {
		if strings.HasPrefix(line, "SUB ") {
			subs = append(subs, line)
		}
	}
	return subs
}

// pipeDialer hands nats.go the client end of the pipe once; every later dial, a reconnect's,
// fails, so a hung-up connection stays lost.
type pipeDialer struct {
	conn net.Conn
	used atomic.Bool
}

func (d *pipeDialer) Dial(string, string) (net.Conn, error) {
	if d.used.Swap(true) {
		return nil, errors.New("scripted server: no second connection")
	}
	return d.conn, nil
}

// connectScripted returns a client connected, through connectWith's dial seam, to a scripted
// server that withholds every PING after the handshake. Health monitoring is off, so the only
// PINGs are the ones the test causes. Cleanup answers every PING and closes the client.
func connectScripted(t *testing.T) (*Client, *scriptedServer) {
	t.Helper()
	cli, srv := net.Pipe()
	server := serveScripted(srv)
	c, err := NewClient("nats://scripted", WithHealthInterval(0), WithLogger(slog.New(slog.DiscardHandler)))
	require.NoError(t, err)
	dialer := &pipeDialer{conn: cli}
	t.Cleanup(func() {
		server.answerPings()
		ctx, cancel := context.WithTimeout(context.Background(), closeBudget)
		defer cancel()
		_ = c.Close(ctx)
		_ = cli.Close()
	})
	require.NoError(t, c.connectWith(t.Context(), func(url string, opts ...nats.Option) (*nats.Conn, error) {
		// SkipHostLookup: the resolver's shared state, made inside the first bubble that looks a
		// name up, may not be used from another bubble, and the pipe has no host to look up.
		return nats.Connect(url, append(opts, nats.SetCustomDialer(dialer), nats.SkipHostLookup())...)
	}))
	return c, server
}

// subscribeResult is one call's return.
type subscribeResult struct {
	sub *Subscription
	err error
}

// subscribeAsync calls Subscribe on its own goroutine and returns the channel its result arrives on.
func subscribeAsync(
	ctx context.Context, c *Client, subject string, handler func(context.Context, *nats.Msg),
) <-chan subscribeResult {
	res := make(chan subscribeResult, 1)
	go func() {
		sub, err := c.Subscribe(ctx, subject, handler)
		res <- subscribeResult{sub, err}
	}()
	return res
}

// requireRefused fails the test at once unless the call returned no subscription and an error.
func requireRefused(t *testing.T, sub *Subscription, err error) {
	t.Helper()
	if sub != nil || err == nil {
		t.Fatalf("the call returned a subscription: %t, and error: %v; want no subscription and an error",
			sub != nil, err)
	}
}

// requireTransient fails unless err is classified transient. The class is read with errors.As, not
// errs.IsTransient, whose text match would pass an unclassified error.
func requireTransient(t *testing.T, err error) {
	t.Helper()
	var ce *errs.ClassifiedError
	require.True(t, errors.As(err, &ce), "error %v is not a *errs.ClassifiedError", err)
	require.Equal(t, errs.ErrorTransient, ce.Class, "error %v", err)
}

func noopHandler(context.Context, *nats.Msg) {}

// T1. It can fail only at one CPU: the probe P1 missed 831 and 800 of 2,000 requests at -cpu=1
// (without and with -race) when nothing waited for the server to read the SUB, and #144's test
// failed 0 of 500 times at the default CPU count. task test:unit runs at one CPU.
func TestSubscribeRegistrationAnswersAnotherConnection(t *testing.T) {
	const iterations = 200
	url := embeddedServerURL(t)
	c, err := NewClient(url, WithLogger(slog.New(slog.DiscardHandler)))
	require.NoError(t, err)
	require.NoError(t, c.Connect(t.Context()))
	closeInCleanup(t, c)
	requester, err := nats.Connect(url)
	require.NoError(t, err)
	t.Cleanup(requester.Close)

	calls := []struct {
		name      string
		subscribe func(subject string) (*Subscription, error)
	}{
		{"Subscribe", func(subject string) (*Subscription, error) {
			return c.Subscribe(t.Context(), subject, func(_ context.Context, msg *nats.Msg) {
				if err := msg.Respond([]byte("answered")); err != nil {
					t.Errorf("respond on %s: %v", msg.Subject, err)
				}
			})
		}},
		{"SubscribeForRequests", func(subject string) (*Subscription, error) {
			return c.SubscribeForRequests(t.Context(), subject, func(context.Context, []byte) ([]byte, error) {
				return []byte("answered"), nil
			})
		}},
	}
	for _, call := range calls {
		t.Run(call.name, func(t *testing.T) {
			misses := 0
			for i := range iterations {
				subject := fmt.Sprintf("t1.%s.%d", call.name, i)
				sub, err := call.subscribe(subject)
				require.NoError(t, err)
				reply, err := requester.Request(subject, nil, lifecycleBound)
				switch {
				case errors.Is(err, nats.ErrNoResponders):
					misses++
				case err != nil:
					t.Fatalf("request %d on %s: %v", i, subject, err)
				default:
					require.Equal(t, "answered", string(reply.Data), "request %d on %s", i, subject)
				}
				require.NoError(t, sub.Unsubscribe())
			}
			if misses > 0 {
				t.Fatalf("%d of %d requests sent right after %s returned got \"no responders\"",
					misses, iterations, call.name)
			}
		})
	}
}

// T2. The server never answers the PING and the context has no deadline: the call returns once
// DefaultRequestTimeout has passed, and the subscription is removed.
func TestSubscribeRegistrationBoundedByDefaultRequestTimeout(t *testing.T) {
	ownProcessBubble(t, func(t *testing.T) {
		c, server := connectScripted(t)
		const subject = "t2.withheld"

		start := time.Now()
		sub, err := c.Subscribe(t.Context(), subject, noopHandler)
		elapsed := time.Since(start)

		requireRefused(t, sub, err)
		require.Equal(t, DefaultRequestTimeout, elapsed, "the call returned after %v on the bubble's clock", elapsed)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		requireTransient(t, err)
		synctest.Wait()
		sid := server.sid(subject)
		require.NotEmpty(t, sid, "the server read no SUB for %s: %q", subject, server.read())
		require.True(t, server.readUnsub(sid), "the server read no UNSUB %s: %q", sid, server.read())
	})
}

// T3. The caller cancels the context while the round trip waits: the call returns at once, and the
// subscription is removed.
func TestSubscribeRegistrationCancelledDuringRoundTrip(t *testing.T) {
	ownProcessBubble(t, func(t *testing.T) {
		c, server := connectScripted(t)
		const subject = "t3.cancelled"
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		res := subscribeAsync(ctx, c, subject, noopHandler)
		synctest.Wait()
		start := time.Now()
		cancel()
		r := awaitValue(t, res, "the call returning after its context was cancelled")
		elapsed := time.Since(start)

		requireRefused(t, r.sub, r.err)
		require.ErrorIs(t, r.err, context.Canceled)
		requireTransient(t, r.err)
		require.Zero(t, elapsed, "the call returned %v after its context was cancelled", elapsed)
		synctest.Wait()
		sid := server.sid(subject)
		require.NotEmpty(t, sid, "the server read no SUB for %s: %q", subject, server.read())
		require.True(t, server.readUnsub(sid), "the server read no UNSUB %s: %q", sid, server.read())
	})
}

// T4. A context that has already ended, on a connected client: the call refuses it and sends no SUB.
// Both facts are checked, so a failure reports each.
func TestSubscribeRegistrationEndedContextSendsNoSub(t *testing.T) {
	ownProcessBubble(t, func(t *testing.T) {
		c, server := connectScripted(t)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		sub, err := c.Subscribe(ctx, "t4.ended", noopHandler)

		if assert.ErrorIs(t, err, context.Canceled, "the call returned a subscription: %t", sub != nil) {
			assert.Nil(t, sub)
			requireTransient(t, err)
		}
		synctest.Wait()
		assert.Empty(t, server.subLines(), "the server read a SUB")
	})
}

// T5. The connection is lost while the round trip waits: the call reports ErrNotConnected, not
// Close's nats.ErrConnectionClosed, and the connection holds no more subscriptions than before.
func TestSubscribeRegistrationConnectionLostDuringRoundTrip(t *testing.T) {
	ownProcessBubble(t, func(t *testing.T) {
		c, server := connectScripted(t)
		conn := c.GetConnection()
		before := conn.NumSubscriptions()

		res := subscribeAsync(t.Context(), c, "t5.lost", noopHandler)
		synctest.Wait()
		server.hangUp()
		// The client reads the end of file before the test goes on. Without this wait, a call
		// that has already returned lets cleanup's Close start nats.go's drain goroutine on the
		// connection being lost; it then sleeps past the bubble's end (nats.go:6355), which
		// panics the run.
		synctest.Wait()
		r := awaitValue(t, res, "the call returning after the connection was lost")

		requireRefused(t, r.sub, r.err)
		require.NotErrorIs(t, r.err, nats.ErrConnectionClosed)
		require.ErrorIs(t, r.err, ErrNotConnected)
		requireTransient(t, r.err)
		require.Equal(t, before, conn.NumSubscriptions(), "subscriptions on the connection")
	})
}

// T6. A message reaches the handler during the round trip and the handler is held when
// DefaultRequestTimeout passes: the call returns while the handler is held, and Close joins it.
func TestSubscribeRegistrationReturnsWhileHandlerHeld(t *testing.T) {
	ownProcessBubble(t, func(t *testing.T) {
		// Close's context stays live through cleanup (its cancel runs after connectScripted's),
		// so a failed assertion never turns this Close into a forced one.
		closeCtx, cancelClose := context.WithTimeout(context.Background(), closeBudget)
		t.Cleanup(cancelClose)
		c, server := connectScripted(t)
		const subject = "t6.held"
		held := newHeldCall(t)

		res := subscribeAsync(t.Context(), c, subject, func(context.Context, *nats.Msg) { held.hold() })
		synctest.Wait()
		sid := server.sid(subject)
		require.NotEmpty(t, sid, "the server read no SUB for %s: %q", subject, server.read())
		server.deliver(subject, sid)
		await(t, held.entered, "the handler starting")
		r := awaitValue(t, res, "the call returning while its handler is held")

		requireRefused(t, r.sub, r.err)
		require.ErrorIs(t, r.err, context.DeadlineExceeded)
		requireTransient(t, r.err)

		server.answerPings()
		closed := make(chan error, 1)
		go func() { closed <- c.Close(closeCtx) }()
		synctest.Wait()
		select {
		case err := <-closed:
			t.Fatalf("Close returned %v while the handler was held", err)
		default:
		}
		held.Release()
		require.NoError(t, awaitErr(t, closed, "Close"))
		select {
		case <-held.returned:
		default:
			t.Fatal("Close returned nil before the handler returned")
		}
	})
}

// T7. As T6 up to the held handler; then Close begins and the server answers every PING: the call
// returns Close's refusal although its round trip completed, while the handler is held, and Close
// joins the handler.
func TestSubscribeRegistrationCloseBeginsDuringRoundTrip(t *testing.T) {
	ownProcessBubble(t, func(t *testing.T) {
		// Close's context stays live through cleanup (its cancel runs after connectScripted's),
		// so a failed assertion never turns this Close into a forced one.
		closeCtx, cancelClose := context.WithTimeout(context.Background(), closeBudget)
		t.Cleanup(cancelClose)
		c, server := connectScripted(t)
		const subject = "t7.closing"
		held := newHeldCall(t)

		res := subscribeAsync(t.Context(), c, subject, func(context.Context, *nats.Msg) { held.hold() })
		synctest.Wait()
		sid := server.sid(subject)
		require.NotEmpty(t, sid, "the server read no SUB for %s: %q", subject, server.read())
		server.deliver(subject, sid)
		await(t, held.entered, "the handler starting")
		closed := make(chan error, 1)
		go func() { closed <- c.Close(closeCtx) }()
		synctest.Wait()
		server.answerPings()
		r := awaitValue(t, res, "the call returning while its handler is held")

		require.ErrorIs(t, r.err, nats.ErrConnectionClosed, "the call returned a subscription: %t", r.sub != nil)
		require.Nil(t, r.sub)
		synctest.Wait()
		select {
		case err := <-closed:
			t.Fatalf("Close returned %v while the handler was held", err)
		default:
		}
		held.Release()
		require.NoError(t, awaitErr(t, closed, "Close"))
		select {
		case <-held.returned:
		default:
			t.Fatal("Close returned nil before the handler returned")
		}
	})
}

// T8. Close begins while the round trip waits, with a context that has already ended, so Close
// closes the connection at once: the call reports Close's nats.ErrConnectionClosed, not
// ErrNotConnected. It runs on real time, outside a bubble: nats.go's drain goroutine outlives a
// Close that closes the connection itself, by up to 5 s (nats.go:6355, :6384), and nothing
// signals its end.
func TestSubscribeRegistrationCloseEndsConnectionDuringRoundTrip(t *testing.T) {
	c, server := connectScripted(t)

	res := subscribeAsync(t.Context(), c, "t8.close", noopHandler)
	select {
	case <-server.pinged: // the round trip waits for its PONG
	case r := <-res:
		requireRefused(t, r.sub, r.err)
		t.Fatalf("the call returned %v before the server read a PING", r.err)
	case <-time.After(lifecycleBound):
		t.Fatalf("the server read no PING within %v", lifecycleBound)
	}
	ended, cancel := context.WithCancel(t.Context())
	cancel()
	closeErr := c.Close(ended)
	r := awaitValue(t, res, "the call returning after Close closed the connection")

	require.ErrorIs(t, r.err, nats.ErrConnectionClosed, "the call returned a subscription: %t", r.sub != nil)
	require.NotErrorIs(t, r.err, ErrNotConnected)
	require.Nil(t, r.sub)
	require.ErrorIs(t, closeErr, context.Canceled)
}

// childTestEnv names the test a re-executed test binary runs in a bubble.
const childTestEnv = "SEMENGINE_NATSCLIENT_BUBBLE_TEST"

// ownProcessBubble runs f in a synctest bubble in a test binary of its own. nats.go keeps one
// process-wide sync.Pool of timers (nats.go timer.go:22); a timer made inside a bubble and handed
// to code outside it, or to another bubble, aborts the process. In its own process the bubble's
// timers reach no other test. The parent re-runs this binary for this one test, and fails unless
// that run exits 0 and reports the test as passed.
func ownProcessBubble(t *testing.T, f func(*testing.T)) {
	t.Helper()
	name := t.Name()
	if os.Getenv(childTestEnv) == name {
		synctest.Test(t, f)
		return
	}
	// -test.run splits its pattern at each slash, and a repeated subtest name gains a #01
	// suffix, so a subtest's run could select nothing and still exit 0.
	if strings.Contains(name, "/") {
		t.Fatalf("ownProcessBubble: %s is a subtest; call it from a top-level test", name)
	}
	exe, err := os.Executable()
	require.NoError(t, err)
	args := []string{"-test.run=^" + name + "$", "-test.count=1", "-test.v",
		fmt.Sprintf("-test.cpu=%d", runtime.GOMAXPROCS(0))}
	if f := flag.Lookup("test.gocoverdir"); f != nil && f.Value.String() != "" {
		args = append(args, "-test.gocoverdir="+f.Value.String())
	}
	if d, ok := t.Deadline(); ok {
		// The child's limit ends first, so its timeout panic and goroutine dump reach this
		// test's failure message, and the child does not outlive this process.
		// A limit of zero would mean none, so it fails here too.
		limit := time.Until(d) * 9 / 10
		if limit <= 0 {
			t.Fatalf("ownProcessBubble: %s has no time left to run", name)
		}
		args = append(args, "-test.timeout="+limit.String())
	}
	cmd := exec.CommandContext(t.Context(), exe, args...)
	cmd.Env = append(os.Environ(), childTestEnv+"="+name,
		"GORACE="+strings.TrimSpace(os.Getenv("GORACE")+" atexit_sleep_ms=0"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s in its own process: %v\n%s", name, err, out)
	}
	if passed := "--- PASS: " + name + " ("; !strings.Contains(string(out), passed) {
		t.Fatalf("%s in its own process: exit 0 but no %q line, so it ran no test\n%s", name, passed, out)
	}
}
