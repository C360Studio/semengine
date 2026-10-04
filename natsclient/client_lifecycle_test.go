package natsclient

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/c360studio/semengine/internal/harness/lifecycletest"
	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/pkg/errs"
	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lifecycleBound caps every wait in these tests that a correct client ends sooner; it is reached
// only when the client is wrong.
const lifecycleBound = 10 * time.Second

// clientOwner adapts Client to the lifecycle suite (design D3). Unresolved lists what a started
// Client retains: the nats.Conn, the JetStream handle, the subscriptions on that connection, the
// internal consumer claims, each kind of client-owned work still running (the health monitor, the
// metrics poller, the claim-release goroutines, the two timer callbacks, client-owned
// subscriptions and the async publish error handler), the two timers while armed, message handler
// invocations still running, and the connection event handlers Close is still waiting for. The
// last two are visible after Close has cleared the connection. Calls counts the external operations the client reports through its opHook seam:
// each dial and each drain.
type clientOwner struct {
	c     *Client
	mu    sync.Mutex
	calls map[string]int
}

func newClientOwner(c *Client) *clientOwner {
	o := &clientOwner{c: c, calls: map[string]int{}}
	c.opHook = func(op string) {
		o.mu.Lock()
		defer o.mu.Unlock()
		o.calls[op]++
	}
	return o
}

func (o *clientOwner) Start(ctx context.Context) error { return o.c.Connect(ctx) }
func (o *clientOwner) Stop(ctx context.Context) error  { return o.c.Close(ctx) }

func (o *clientOwner) Observe() lifecycletest.Observation {
	o.mu.Lock()
	calls := maps.Clone(o.calls)
	o.mu.Unlock()

	c := o.c
	var held []string
	c.mu.RLock()
	if c.conn != nil {
		held = append(held, "nats connection")
		if c.conn.NumSubscriptions() > 0 {
			held = append(held, "subscriptions")
		}
	}
	if c.js != nil {
		held = append(held, "jetstream handle")
	}
	held = append(held, slices.Sorted(maps.Keys(c.running))...)
	// What the monitor and poller publish before they start: still set after a nil Close means
	// one of them started outside startBackground.
	if c.healthDone != nil {
		held = append(held, "health monitor done channel")
	}
	if c.metricsCancel != nil {
		held = append(held, "metrics poller cancel")
	}
	if c.awaitingClosed != nil {
		held = append(held, "connection event handlers")
	}
	c.mu.RUnlock()
	if c.handlersRunning.Load() > 0 {
		held = append(held, "message handler")
	}
	c.timersMu.Lock()
	if c.lossTimer != nil {
		held = append(held, "connection-loss timer armed")
	}
	if c.circuitTimer != nil {
		held = append(held, "circuit-test timer armed")
	}
	c.timersMu.Unlock()
	c.internalClaimsMu.Lock()
	if len(c.internalClaims) > 0 {
		held = append(held, "internal consumer claims")
	}
	c.internalClaimsMu.Unlock()
	return lifecycletest.Observation{Unresolved: held, Calls: calls}
}

// embeddedServerURL starts an in-process NATS server on an ephemeral loopback port and returns
// its client URL.
func embeddedServerURL(t *testing.T) string {
	t.Helper()
	server, err := natsserver.NewServer(&natsserver.Options{Host: "127.0.0.1", Port: -1, NoLog: true, NoSigs: true})
	require.NoError(t, err)
	server.Start()
	t.Cleanup(func() {
		server.Shutdown()
		server.WaitForShutdown()
	})
	require.True(t, server.ReadyForConnections(lifecycleBound), "embedded server not ready")
	return server.ClientURL()
}

// recordingHandler keeps every log record and, when hold is set, blocks the first record with
// that message until release is called. The client logs through it from Connect's goroutine,
// so the hold is a deterministic pause point inside Connect.
type recordingHandler struct {
	hold    string
	entered chan struct{}
	release chan struct{}

	enterOnce, releaseOnce sync.Once
	mu                     sync.Mutex
	records                []loggedRecord
}

type loggedRecord struct {
	level slog.Level
	msg   string
	attrs map[string]string
}

func newRecordingHandler(hold string) *recordingHandler {
	return &recordingHandler{hold: hold, entered: make(chan struct{}), release: make(chan struct{})}
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h *recordingHandler) WithGroup(string) slog.Handler            { return h }

func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	rec := loggedRecord{level: r.Level, msg: r.Message, attrs: map[string]string{}}
	r.Attrs(func(a slog.Attr) bool {
		rec.attrs[a.Key] = a.Value.String()
		return true
	})
	h.mu.Lock()
	h.records = append(h.records, rec)
	h.mu.Unlock()
	if h.hold != "" && r.Message == h.hold {
		first := false
		h.enterOnce.Do(func() { first = true; close(h.entered) })
		if first {
			<-h.release
		}
	}
	return nil
}

func (h *recordingHandler) releaseHold() { h.releaseOnce.Do(func() { close(h.release) }) }

func (h *recordingHandler) find(match func(loggedRecord) bool) []loggedRecord {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []loggedRecord
	for _, r := range h.records {
		if match(r) {
			out = append(out, r)
		}
	}
	return out
}

// droppedWork reports the debug records that name kind as work dropped because the client was
// closing.
func (h *recordingHandler) droppedWork(kind string) []loggedRecord {
	return h.find(func(r loggedRecord) bool {
		return r.level == slog.LevelDebug && r.attrs["work"] == kind
	})
}

// recoverCall runs fn and reports the value it panicked with, or its error.
func recoverCall(fn func() error) (panicked any, err error) {
	defer func() { panicked = recover() }()
	return nil, fn()
}

// blockingLossCallback plants an onConnectionLost that signals entered and then blocks until
// release. The release is registered in t.Cleanup before the callback exists, so a failing test
// never leaves it blocked.
type blockingLossCallback struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func plantBlockingLossCallback(t *testing.T) *blockingLossCallback {
	t.Helper()
	cb := &blockingLossCallback{entered: make(chan struct{}, 1), release: make(chan struct{})}
	t.Cleanup(cb.Release)
	return cb
}

func (cb *blockingLossCallback) Release() { cb.once.Do(func() { close(cb.release) }) }

func (cb *blockingLossCallback) fn(error) {
	select {
	case cb.entered <- struct{}{}:
	default:
	}
	<-cb.release
}

// awaitEntered waits on the bubble's clock for the planted callback to start.
func (cb *blockingLossCallback) awaitEntered(t *testing.T) {
	t.Helper()
	select {
	case <-cb.entered:
	case <-time.After(time.Minute):
		t.Fatal("the connection-loss callback did not start")
	}
}

// TestClientCloseRefusesNilContext is natsclient-nil-context-refused for Close: on a connected
// client Close(nil) returns an error before acting, and the connection stays open.
func TestClientCloseRefusesNilContext(t *testing.T) {
	c, err := NewClient(embeddedServerURL(t), WithHealthInterval(0))
	require.NoError(t, err)
	require.NoError(t, c.Connect(t.Context()))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), closeBudget)
		defer cancel()
		_ = c.Close(ctx)
	})
	conn := c.GetConnection()

	var nilCtx context.Context
	panicked, err := recoverCall(func() error { return c.Close(nilCtx) })
	require.Nil(t, panicked, "Close(nil) panicked")
	require.Error(t, err, "Close(nil) returned nil")
	require.Same(t, conn, c.GetConnection())
	require.True(t, conn.IsConnected(), "Close(nil) touched the connection")
	require.Equal(t, StatusConnected, c.Status())
}

// TestClientConnectRefusesNilContext is natsclient-nil-context-refused for Connect: Connect(nil)
// returns an error and never dials.
func TestClientConnectRefusesNilContext(t *testing.T) {
	url := embeddedServerURL(t)
	c, err := NewClient(url, WithHealthInterval(0))
	require.NoError(t, err)
	var dials atomic.Int32
	var dialled *nats.Conn
	dial := func(u string, opts ...nats.Option) (*nats.Conn, error) {
		dials.Add(1)
		conn, err := nats.Connect(u, opts...)
		dialled = conn
		return conn, err
	}
	t.Cleanup(func() {
		if dialled != nil {
			dialled.Close()
		}
	})

	var nilCtx context.Context
	panicked, err := recoverCall(func() error { return c.connectWith(nilCtx, dial) })
	require.Nil(t, panicked, "Connect(nil) panicked")
	require.Error(t, err, "Connect(nil) returned nil")
	require.Zero(t, dials.Load(), "Connect(nil) dialled")
	require.Nil(t, c.GetConnection())
}

// TestClientSecondConnectRefused is natsclient-connect-refuses-second-start: a second Connect on
// a connected client returns an error, never dials, and leaves the first connection in place.
func TestClientSecondConnectRefused(t *testing.T) {
	c, err := NewClient(embeddedServerURL(t), WithHealthInterval(0))
	require.NoError(t, err)
	require.NoError(t, c.Connect(t.Context()))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), closeBudget)
		defer cancel()
		_ = c.Close(ctx)
	})
	first := c.GetConnection()

	var dials atomic.Int32
	var dialled []*nats.Conn
	t.Cleanup(func() {
		for _, conn := range dialled {
			conn.Close()
		}
	})
	err = c.connectWith(t.Context(), func(u string, opts ...nats.Option) (*nats.Conn, error) {
		dials.Add(1)
		conn, err := nats.Connect(u, opts...)
		if conn != nil {
			dialled = append(dialled, conn)
		}
		return conn, err
	})
	require.Error(t, err, "a second Connect returned nil")
	require.Zero(t, dials.Load(), "a second Connect dialled")
	require.Same(t, first, c.GetConnection(), "a second Connect replaced the connection")
	require.True(t, first.IsConnected(), "a second Connect closed the first connection")
	require.Equal(t, StatusConnected, c.Status())
}

// TestClientConnectThatLosesAdmissionLeavesStatus: two Connects pass the started check; the one
// held at its opHook "dial" report, after that check and before it writes the status, resumes
// only once the other has connected. It must be refused and leave the status the winner set:
// writing Connecting over a connected client is a silent wrong state, and so is its rejected
// connection's closed handler writing Disconnected. The loser's dial runs the client's own
// closed handler and then signals, so the check waits for that handler, not for luck.
func TestClientConnectThatLosesAdmissionLeavesStatus(t *testing.T) {
	c, err := NewClient(embeddedServerURL(t), WithHealthInterval(0))
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), closeBudget)
		defer cancel()
		_ = c.Close(ctx)
	})
	held, release := make(chan struct{}), make(chan struct{})
	var releaseOnce, holdOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	c.opHook = func(op string) {
		if op != "dial" {
			return
		}
		first := false
		holdOnce.Do(func() { first = true })
		if first {
			close(held)
			<-release
		}
	}

	loserClosed := make(chan struct{})
	loserDial := func(url string, opts ...nats.Option) (*nats.Conn, error) {
		return nats.Connect(url, append(opts, nats.ClosedHandler(func(nc *nats.Conn) {
			c.handleClosed(nc)
			close(loserClosed)
		}))...)
	}
	loser := make(chan error, 1)
	go func() { loser <- c.connectWith(t.Context(), loserDial) }()
	select {
	case <-held:
	case <-time.After(lifecycleBound):
		t.Fatal("the first Connect never reached its dial report")
	}
	require.NoError(t, c.Connect(t.Context()), "the Connect that wins admission")
	winner := c.GetConnection()
	require.Equal(t, StatusConnected, c.Status())

	releaseOnce.Do(func() { close(release) })
	select {
	case err := <-loser:
		require.ErrorIs(t, err, errs.ErrAlreadyStarted, "the Connect that lost admission")
	case <-time.After(lifecycleBound):
		t.Fatal("the losing Connect did not return")
	}
	select {
	case <-loserClosed:
	case <-time.After(lifecycleBound):
		t.Fatal("the rejected connection's closed handler never ran")
	}
	require.Same(t, winner, c.GetConnection(), "the losing Connect replaced the connection")
	require.True(t, winner.IsConnected(), "the losing Connect closed the winner's connection")
	require.Equal(t, StatusConnected, c.Status(), "the losing Connect left its own status behind")
}

// TestClientCloseJoinsConnectionLossCallback is natsclient-close-joins-its-goroutines (a): a
// connection-loss timer armed by handleDisconnect runs a planted callback that blocks. Close
// under an ended context reports that context; a second Close with a live context returns only
// once the callback has returned, and then returns nil.
func TestClientCloseJoinsConnectionLossCallback(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cb := plantBlockingLossCallback(t)
		c, err := NewClient("nats://unused",
			WithConnectionLossTimeout(50*time.Millisecond), WithConnectionLostCallback(cb.fn))
		require.NoError(t, err)
		c.handleDisconnect(nil, errors.New("broker gone"))
		cb.awaitEntered(t)

		ended, cancel := context.WithCancel(t.Context())
		cancel()
		require.ErrorIs(t, c.Close(ended), context.Canceled, "Close under an ended context while the callback runs")

		live, cancelLive := context.WithTimeout(t.Context(), time.Hour)
		defer cancelLive()
		second := make(chan error, 1)
		go func() { second <- c.Close(live) }()
		synctest.Wait()
		select {
		case err := <-second:
			t.Fatalf("second Close returned %v while the callback still runs", err)
		default:
		}
		cb.Release()
		select {
		case err := <-second:
			require.NoError(t, err, "second Close after the callback returned")
		case <-time.After(2 * time.Hour):
			t.Fatal("second Close did not return after the callback was released")
		}
	})
}

// TestClientCloseDropsLateDisconnect is natsclient-close-joins-its-goroutines (b): once Close
// has begun, a disconnect arms no connection-loss timer and the drop is logged at debug level.
// Close is held open by a planted callback, so the disconnect races a Close that is waiting.
func TestClientCloseDropsLateDisconnect(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cb := plantBlockingLossCallback(t)
		var fired atomic.Int32
		h := newRecordingHandler("")
		c, err := NewClient("nats://unused",
			WithLogger(slog.New(h)),
			WithConnectionLossTimeout(50*time.Millisecond),
			WithConnectionLostCallback(func(e error) {
				fired.Add(1)
				cb.fn(e)
			}))
		require.NoError(t, err)
		owner := newClientOwner(c)
		c.handleDisconnect(nil, errors.New("first loss"))
		cb.awaitEntered(t)

		live, cancelLive := context.WithTimeout(t.Context(), time.Hour)
		defer cancelLive()
		closed := make(chan error, 1)
		go func() { closed <- c.Close(live) }()
		synctest.Wait()

		c.handleDisconnect(nil, errors.New("loss after Close began"))
		require.NotContains(t, owner.Observe().Unresolved, "connection-loss timer armed",
			"a disconnect after Close began armed the connection-loss timer")
		require.Len(t, h.droppedWork("connection-loss timer"), 1, "the dropped disconnect was not logged at debug level")

		// A loss timer that fires once Close has begun: its body is refused, not run and not counted.
		c.connectionLossFired(func() bool { return false }, errors.New("timer fired after Close began"))
		require.Equal(t, []string{"connection-loss timer"}, owner.Observe().Unresolved,
			"only the planted callback is running; the late timer was counted")
		require.Len(t, h.droppedWork("connection-loss timer"), 2, "the dropped timer body was not logged at debug level")

		cb.Release()
		select {
		case err := <-closed:
			require.NoError(t, err)
		case <-time.After(2 * time.Hour):
			t.Fatal("Close did not return after the callback was released")
		}
		<-time.After(time.Second)
		synctest.Wait()
		require.Equal(t, int32(1), fired.Load(), "a callback ran after Close began")
		require.Empty(t, owner.Observe().Unresolved)
	})
}

// TestClientCloseStopsCircuitTimer: Close stops a pending circuit-test timer (design D3, the
// timers Close closes), so testCircuit never runs once Close has returned.
func TestClientCloseStopsCircuitTimer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newRecordingHandler("")
		c, err := NewClient("nats://unused", WithLogger(slog.New(h)))
		require.NoError(t, err)
		owner := newClientOwner(c)
		for range c.circuitThreshold {
			c.recordFailure()
		}
		require.Equal(t, StatusCircuitOpen, c.Status())

		require.NoError(t, c.Close(t.Context()))
		require.Empty(t, owner.Observe().Unresolved)

		// Once Close has begun, arming the circuit timer is refused, and a timer that fired is
		// refused before its test runs; both drops are logged. (Failures recorded after Close no
		// longer open the circuit: TestClientFailuresAfterCloseLeaveStatusDisconnected.)
		c.armCircuitTimer(time.Second)
		require.Empty(t, owner.Observe().Unresolved, "a circuit timer armed after Close")
		c.circuitTimerFired(func() bool { return false })
		require.Empty(t, owner.Observe().Unresolved, "a circuit timer that fired after Close was counted")
		require.Len(t, h.droppedWork("circuit-test timer"), 2, "the dropped arm and timer body were not both logged")

		<-time.After(time.Hour)
		synctest.Wait()
		require.Empty(t, h.find(func(r loggedRecord) bool {
			return r.msg == "Testing circuit breaker - attempting to close circuit"
		}), "the circuit test ran after Close returned")
	})
}

// TestClientCloseUnderEndedContextAlwaysReportsIt: a Close whose context has ended returns that
// context's error even when the join has already finished, so a nil return always means the
// caller's context was live (lifecycle-suite, abort cause). Both cases of Close's wait are ready
// on every call here, so a Close that trusted the select's pick would return nil about half the
// time; 64 calls make a survivor a 2^-64 event.
func TestClientCloseUnderEndedContextAlwaysReportsIt(t *testing.T) {
	c, err := NewClient("nats://unused")
	require.NoError(t, err)
	live, cancelLive := context.WithTimeout(t.Context(), lifecycleBound)
	defer cancelLive()
	require.NoError(t, c.Close(live), "the first Close, with nothing running")

	ended, cancel := context.WithCancel(t.Context())
	cancel()
	for i := range 64 {
		require.ErrorIs(t, c.Close(ended), context.Canceled, "Close %d under an ended context after the join", i)
	}
	require.NoError(t, c.Close(live), "a later Close with a live context")
}

// TestClientCloseUnderShortDeadlineClosesConnection is task 4.3's bounded Close (transport-client,
// "Close under an ended context"): a client-owned handler holds the drain past the end of Close's
// context. Close returns the context's error while the handler still runs, the connection is
// closed by then, and a second Close with a live context waits for the handler and then returns
// nil. The handler is held until the test releases it, so the context's end is the only way the
// first Close can return; the 200 ms deadline is the subject under test, not a wait for an event.
// The deadline case is the scenario. Close shortens its drain timer to the deadline, so there
// either of its two timer branches may close the connection; the cancelled case, ended once the
// drain is observed to have begun, reaches only the context branch.
func TestClientCloseUnderShortDeadlineClosesConnection(t *testing.T) {
	for _, tc := range []struct {
		name string
		want error
	}{
		{"deadline", context.DeadlineExceeded},
		{"cancelled-during-drain", context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := NewClient(embeddedServerURL(t), WithHealthInterval(0))
			require.NoError(t, err)
			require.NoError(t, c.Connect(t.Context()))
			closeInCleanup(t, c)
			held := newHeldCall(t)
			_, err = c.Subscribe(t.Context(), "close.short", func(context.Context, *nats.Msg) { held.hold() })
			require.NoError(t, err)
			nc := c.GetConnection()
			require.NoError(t, nc.FlushTimeout(lifecycleBound))
			require.NoError(t, nc.Publish("close.short", nil))
			await(t, held.entered, "the handler entering")
			draining := nc.StatusChanged(nats.DRAINING_SUBS)
			defer nc.RemoveStatusListener(draining)

			var first <-chan error
			if tc.want == context.DeadlineExceeded {
				short, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
				defer cancel()
				first = closeAsync(short, c)
			} else {
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				first = closeAsync(ctx, c)
				awaitValue(t, draining, "the drain beginning")
				cancel()
			}
			err = awaitErr(t, first, "Close whose context ends while the handler runs")
			require.ErrorIs(t, err, tc.want, "Close whose context ended")
			// Close has closed the connection by the time it returns, but nats.go's own drain
			// goroutine, finding no subscriptions left once the connection closed, can still move its
			// status to DRAINING_PUBS and then close it again (nats.go v1.54.0 nats.go:6378-6390), so
			// IsClosed can read false for that moment. The wait is bounded by lifecycleBound; without
			// Close's force close the native drain holds the connection open for the client's 30 s
			// drain timeout, past that bound.
			closedCtx, cancelClosed := context.WithTimeout(t.Context(), lifecycleBound)
			defer cancelClosed()
			_, err = probe.Await(closedCtx, func(context.Context) (bool, error) { return nc.IsClosed(), nil },
				func(closed bool) bool { return closed })
			require.NoError(t, err, "the connection is still open after Close returned on its context")

			live, cancelLive := context.WithTimeout(t.Context(), 2*lifecycleBound)
			defer cancelLive()
			watched := newDoneWatchContext(live)
			second := closeAsync(watched, c)
			select {
			case err := <-second:
				t.Fatalf("a second Close returned %v while the handler still runs", err)
			case <-watched.waiting:
			case <-time.After(lifecycleBound):
				t.Fatalf("the second Close neither returned nor waited on its context within %v", lifecycleBound)
			}
			requireJoinPending(t, c, joinOnOwnedWork, "the held handler returned")
			select {
			case err := <-second:
				t.Fatalf("a second Close returned %v while the handler still runs", err)
			default:
			}
			held.Release()
			require.NoError(t, awaitErr(t, second, "the second Close after the handler returned"))
		})
	}
}

// doneWatchContext closes waiting the first time Done is called, so a test can tell a Close that
// has reached its wait on the caller's context from one that has not yet run.
type doneWatchContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func newDoneWatchContext(parent context.Context) *doneWatchContext {
	return &doneWatchContext{Context: parent, waiting: make(chan struct{})}
}

func (c *doneWatchContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}

// streamOnlyJetStream serves one stream from Stream; the embedded fake panics on anything else.
type streamOnlyJetStream struct {
	*fakeJetStream
	stream jetstream.Stream
}

func (j *streamOnlyJetStream) Stream(context.Context, string) (jetstream.Stream, error) {
	return j.stream, nil
}

// consumerOnlyStream returns one consumer from CreateOrUpdateConsumer; the embedded nil Stream
// panics on anything else.
type consumerOnlyStream struct {
	jetstream.Stream
	consumer jetstream.Consumer
}

func (s *consumerOnlyStream) CreateOrUpdateConsumer(context.Context, jetstream.ConsumerConfig) (jetstream.Consumer, error) {
	return s.consumer, nil
}

// TestClientCloseJoinsInternalClaimRelease is the claim-release site of
// ConsumeInternalStreamWithConfig (design D3): Close joins the release, which waits for the
// consumer's Closed channel.
func TestClientCloseJoinsInternalClaimRelease(t *testing.T) {
	c, err := NewClient("nats://unused")
	require.NoError(t, err)
	owner := newClientOwner(c)
	handle := &controlledNativeConsumeContext{closed: make(chan struct{})}
	var closeOnce sync.Once
	release := func() { closeOnce.Do(func() { close(handle.closed) }) }
	t.Cleanup(release)
	native := &controlledNativeConsumer{
		consumeEntered: make(chan struct{}),
		consumeRelease: make(chan struct{}),
		handle:         handle,
		info:           &jetstream.ConsumerInfo{Stream: "S_INTERNAL", Name: "internal"},
	}
	close(native.consumeRelease)
	c.mu.Lock()
	c.js = &streamOnlyJetStream{fakeJetStream: &fakeJetStream{}, stream: &consumerOnlyStream{consumer: native}}
	c.mu.Unlock()
	c.setStatus(StatusConnected)

	_, err = c.ConsumeInternalStreamWithConfig(t.Context(),
		StreamConsumerConfig{StreamName: "S_INTERNAL", ConsumerName: "internal"},
		func(context.Context, jetstream.Msg) {})
	require.NoError(t, err)
	require.Equal(t, []string{"jetstream handle", "claim release", "internal consumer claims"}, owner.Observe().Unresolved)

	ended, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, c.Close(ended), context.Canceled, "Close while the claim release waits on Closed")
	release()
	live, cancelLive := context.WithTimeout(t.Context(), lifecycleBound)
	defer cancelLive()
	require.NoError(t, c.Close(live), "Close once the consumer closed")
	require.Empty(t, owner.Observe().Unresolved, "after Close returned nil")
}

// TestClientConcurrentClosesEachHonourTheirContext is natsclient-close-joins-its-goroutines (c):
// two Close calls wait on the same planted callback. The one whose context ends reports it while
// the other still waits; the other returns nil once the callback is released.
func TestClientConcurrentClosesEachHonourTheirContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cb := plantBlockingLossCallback(t)
		c, err := NewClient("nats://unused",
			WithConnectionLossTimeout(50*time.Millisecond), WithConnectionLostCallback(cb.fn))
		require.NoError(t, err)
		c.handleDisconnect(nil, errors.New("broker gone"))
		cb.awaitEntered(t)

		ctxA, cancelA := context.WithCancel(t.Context())
		defer cancelA()
		ctxB, cancelB := context.WithTimeout(t.Context(), time.Hour)
		defer cancelB()
		a, b := make(chan error, 1), make(chan error, 1)
		go func() { a <- c.Close(ctxA) }()
		go func() { b <- c.Close(ctxB) }()
		synctest.Wait()
		select {
		case err := <-a:
			t.Fatalf("Close A returned %v while the callback runs", err)
		case err := <-b:
			t.Fatalf("Close B returned %v while the callback runs", err)
		default:
		}

		cancelA()
		synctest.Wait()
		select {
		case err := <-a:
			require.ErrorIs(t, err, context.Canceled)
		default:
			t.Fatal("Close A did not return when its context ended")
		}
		select {
		case err := <-b:
			t.Fatalf("Close B returned %v before the callback was released", err)
		default:
		}

		cb.Release()
		select {
		case err := <-b:
			require.NoError(t, err)
		case <-time.After(2 * time.Hour):
			t.Fatal("Close B did not return after the callback was released")
		}
	})
}

// TestClientCloseFromInsideCallbackReportsContext is natsclient-close-joins-its-goroutines (d):
// Close called from inside onConnectionLost waits on a join that includes its own goroutine, so
// with a bounded context it returns that context's error and never nil.
func TestClientCloseFromInsideCallbackReportsContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		inner := make(chan error, 1)
		var c *Client
		c, err := NewClient("nats://unused",
			WithConnectionLossTimeout(50*time.Millisecond),
			WithConnectionLostCallback(func(error) {
				ctx, cancel := context.WithTimeout(t.Context(), time.Second)
				defer cancel()
				inner <- c.Close(ctx)
			}))
		require.NoError(t, err)
		c.handleDisconnect(nil, errors.New("broker gone"))

		select {
		case err := <-inner:
			require.ErrorIs(t, err, context.DeadlineExceeded, "Close from inside the callback")
		case <-time.After(time.Minute):
			t.Fatal("Close from inside the callback did not return")
		}
		live, cancel := context.WithTimeout(t.Context(), time.Hour)
		defer cancel()
		require.NoError(t, c.Close(live), "Close after the callback returned")
	})
}

// TestClientCloseDuringConnectStartsNothing is natsclient-close-joins-its-goroutines (e): Connect
// is held, through its logger, on the "Successfully connected to NATS" record, after it has
// admitted the connection and before it starts the health monitor and the metrics poller. Close
// runs and returns while Connect is held; once Connect is released and both have returned,
// neither the monitor nor the poller runs and the client holds nothing.
func TestClientCloseDuringConnectStartsNothing(t *testing.T) {
	url := embeddedServerURL(t)
	h := newRecordingHandler("Successfully connected to NATS")
	t.Cleanup(h.releaseHold)
	c, err := NewClient(url, WithLogger(slog.New(h)), WithMetrics(metric.NewMetricsRegistry()))
	require.NoError(t, err)
	owner := newClientOwner(c)

	connected := make(chan error, 1)
	go func() { connected <- c.Connect(t.Context()) }()
	select {
	case <-h.entered:
	case <-time.After(lifecycleBound):
		t.Fatal("Connect never logged that it connected")
	}

	closeCtx, cancel := context.WithTimeout(t.Context(), lifecycleBound)
	defer cancel()
	require.NoError(t, c.Close(closeCtx), "Close while Connect is held")

	h.releaseHold()
	var connectErr error
	select {
	case connectErr = <-connected:
	case <-time.After(lifecycleBound):
		t.Fatal("Connect did not return after it was released")
	}
	assert.Empty(t, owner.Observe().Unresolved, "after Close overtook Connect")
	assert.ErrorIs(t, connectErr, nats.ErrConnectionClosed, "Connect that Close overtook reports the close")
}

// TestClientCloseJoinsClaimRelease: a port consumer's claim release waits for the consumer's
// Closed channel, and Close joins it (design D3, the claim-release site). Close under an ended
// context reports it while the release waits; once Closed fires, Close returns nil with the
// claim released. A consumer started once Close has begun is refused before native Consume is
// called (design D3, consumer setup that meets Close): its claim is released, and its claim
// release is dropped, logged and not counted.
func TestClientCloseJoinsClaimRelease(t *testing.T) {
	h := newRecordingHandler("")
	c, err := NewClient("nats://unused", WithLogger(slog.New(h)), WithMetrics(metric.NewMetricsRegistry()))
	require.NoError(t, err)
	owner := newClientOwner(c)
	startConsumer := func(durable string) (*controlledNativeConsumer, func(), error) {
		identity := internalConsumerIdentity{stream: "S_CLAIM", durable: durable}
		claim, err := c.reserveInternalConsumer(identity, "ConsumeStreamWithConfig")
		require.NoError(t, err)
		handle := &controlledNativeConsumeContext{closed: make(chan struct{})}
		var closeOnce sync.Once
		release := func() { closeOnce.Do(func() { close(handle.closed) }) }
		t.Cleanup(release)
		native := &controlledNativeConsumer{
			consumeEntered: make(chan struct{}),
			consumeRelease: make(chan struct{}),
			handle:         handle,
			info: &jetstream.ConsumerInfo{
				Stream: identity.stream, Name: durable,
				Config: jetstream.ConsumerConfig{Durable: durable, MaxAckPending: 17},
			},
		}
		close(native.consumeRelease)
		cfg := StreamConsumerConfig{StreamName: identity.stream, ConsumerName: durable, MaxAckPending: 17}
		_, err = c.startPortConsumer(t.Context(), t.Context(), "ConsumeStreamWithConfig",
			PortConsumerContext{Component: "claim-test", Port: "input"}, cfg,
			&guardedConsumer{Consumer: native}, identity, claim, func(context.Context, jetstream.Msg) {})
		return native, release, err
	}

	_, releaseFirst, err := startConsumer("first")
	require.NoError(t, err)
	require.Equal(t, []string{"claim release", "internal consumer claims"}, owner.Observe().Unresolved)

	ended, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, c.Close(ended), context.Canceled, "Close while the claim release waits on Closed")

	releaseFirst()
	live, cancelLive := context.WithTimeout(t.Context(), lifecycleBound)
	defer cancelLive()
	require.NoError(t, c.Close(live), "Close once the consumer closed")
	require.Empty(t, owner.Observe().Unresolved, "after Close returned nil")

	late, _, err := startConsumer("late")
	require.ErrorIs(t, err, nats.ErrConnectionClosed, "a consumer started once Close has begun")
	select {
	case <-late.consumeEntered:
		t.Fatal("native Consume was called once Close had begun")
	default:
	}
	require.Len(t, h.droppedWork("claim release"), 1, "the dropped claim release was not logged at debug level")
	require.Empty(t, owner.Observe().Unresolved, "the refused consumer kept its claim")
}

// TestClientLifecycleSuite runs the lifecycle suite on Client (design D3) against an embedded
// server, with a must-fail factory whose URL is a refused local port.
func TestClientLifecycleSuite(t *testing.T) {
	url := embeddedServerURL(t)
	refused := refusedNATSURL(t)
	factory := func() lifecycletest.Owner {
		c, err := NewClient(url, WithMetrics(metric.NewMetricsRegistry()))
		require.NoError(t, err)
		return newClientOwner(c)
	}
	mustFail := func() lifecycletest.Owner {
		c, err := NewClient(refused)
		require.NoError(t, err)
		return newClientOwner(c)
	}
	lifecycletest.Run(t, factory, mustFail, lifecycletest.Promise{})
}
