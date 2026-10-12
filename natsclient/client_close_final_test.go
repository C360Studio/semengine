package natsclient

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/pkg/errs"
	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The tests in this file are task 3.7c2 (design D3: natsclient-close-is-final,
// natsclient-close-honours-each-context, natsclient-status-ownership and
// natsclient-close-reports-drain-timeout), Codex F21-F25 at 7ce1940 (PR #48 comment 5980134911).

// heldCall blocks every caller of hold until Release; entered closes when the first caller
// arrives and returned when it leaves. Release is registered in t.Cleanup before anything can
// call hold, so a failed assertion never leaves a caller blocked.
type heldCall struct {
	entered, release, returned         chan struct{}
	enterOnce, releaseOnce, returnOnce sync.Once
}

func newHeldCall(t *testing.T) *heldCall {
	t.Helper()
	h := &heldCall{entered: make(chan struct{}), release: make(chan struct{}), returned: make(chan struct{})}
	t.Cleanup(h.Release)
	return h
}

func (h *heldCall) hold() {
	h.enterOnce.Do(func() { close(h.entered) })
	<-h.release
	h.returnOnce.Do(func() { close(h.returned) })
}

func (h *heldCall) Release() { h.releaseOnce.Do(func() { close(h.release) }) }

// await waits for ch to close, failing the test after lifecycleBound.
func await(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(lifecycleBound):
		t.Fatalf("%s: not within %v", what, lifecycleBound)
	}
}

// awaitErr waits for one result on ch, failing the test after lifecycleBound.
func awaitErr(t *testing.T, ch <-chan error, what string) error {
	t.Helper()
	select {
	case err := <-ch:
		return err
	case <-time.After(lifecycleBound):
		t.Fatalf("%s: did not return within %v", what, lifecycleBound)
		return nil
	}
}

// awaitValue waits for one value on ch, failing the test after lifecycleBound.
func awaitValue[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(lifecycleBound):
		t.Fatalf("%s: not within %v", what, lifecycleBound)
		var zero T
		return zero
	}
}

// releaseInCleanup returns an idempotent close of a fake's hold channel and registers it in
// t.Cleanup, so a test that fails before its normal-path release leaves no goroutine waiting on
// the fake. Inside a synctest bubble the cleanup runs before the bubble waits for its goroutines.
func releaseInCleanup(t *testing.T, ch chan struct{}) func() {
	var once sync.Once
	release := func() { once.Do(func() { close(ch) }) }
	t.Cleanup(release)
	return release
}

// closeAsync runs Close(ctx) on its own goroutine.
func closeAsync(ctx context.Context, c *Client) <-chan error {
	out := make(chan error, 1)
	go func() { out <- c.Close(ctx) }()
	return out
}

// endedContext returns a context that has already ended.
func endedContext(t *testing.T) context.Context {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	return ctx
}

// joinState is what requireJoinPending observes of Close's join, in-package.
type joinState struct{ done, blocked bool }

// Wait points of Close's joiner, for requireJoinPending.
const (
	joinOnOwnedWork     = "client-owned work"
	joinOnEventHandlers = "the connection's event handlers"
)

// requireJoinPending requires that Close's join, which every nil Close waits for, is still open.
// It waits (bounded by lifecycleBound, a failure bound) until the joiner is observed blocked at the
// named wait point, the one the test's held work keeps it at, or has finished; then it requires
// that the joiner has not finished. No real-clock interval decides the outcome (design D8, R1b): a
// joiner observed blocked at that point has not closed the join, and cannot while the work is
// held.
func requireJoinPending(t *testing.T, c *Client, waitPoint, what string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), lifecycleBound)
	defer cancel()
	st, err := probe.Await(ctx, func(context.Context) (joinState, error) {
		c.mu.RLock()
		defer c.mu.RUnlock()
		if c.joined == nil {
			return joinState{}, nil
		}
		select {
		case <-c.joined:
			return joinState{done: true}, nil
		default:
		}
		if waitPoint == joinOnEventHandlers {
			return joinState{blocked: c.awaitingClosed != nil}, nil
		}
		return joinState{blocked: c.idleWait != nil}, nil
	}, func(s joinState) bool { return s.done || s.blocked })
	require.NoError(t, err, "Close's joiner never waited for %s", waitPoint)
	require.False(t, st.done, "Close's join completed before %s", what)
}

// requireJoinOpen requires that Close's join has not completed, at a moment the test has already
// made it impossible for a correct client to complete it (the first Close's drain is held, so its
// cleanup has not finished).
func requireJoinOpen(t *testing.T, c *Client, what string) {
	t.Helper()
	c.mu.RLock()
	joined := c.joined
	c.mu.RUnlock()
	require.NotNil(t, joined, "Close has not begun")
	select {
	case <-joined:
		t.Fatalf("Close's join completed before %s", what)
	default:
	}
}

// requireLiveCloseNil calls Close with a live context and requires nil.
func requireLiveCloseNil(t *testing.T, c *Client, what string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), lifecycleBound)
	defer cancel()
	require.NoError(t, c.Close(ctx), what)
}

// closeInCleanup registers a bounded Close of c.
func closeInCleanup(t *testing.T, c *Client) {
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), closeBudget)
		defer cancel()
		_ = c.Close(ctx)
	})
}

// embeddedJetStreamURL starts an in-process NATS server with JetStream on an ephemeral port.
func embeddedJetStreamURL(t *testing.T) string {
	t.Helper()
	server, err := natsserver.NewServer(&natsserver.Options{
		Host: "127.0.0.1", Port: -1, NoLog: true, NoSigs: true, JetStream: true, StoreDir: t.TempDir(),
	})
	require.NoError(t, err)
	server.Start()
	t.Cleanup(func() {
		server.Shutdown()
		server.WaitForShutdown()
	})
	require.True(t, server.ReadyForConnections(lifecycleBound), "embedded server not ready")
	return server.ClientURL()
}

// boundedAutoCreate declares the finite bounds an auto-created stream must carry.
func boundedAutoCreate() *StreamAutoCreateConfig {
	return &StreamAutoCreateConfig{Storage: "memory", MaxAge: time.Hour, MaxBytes: 1 << 20}
}

// claimCount reads how many local consumer claims the client holds.
func claimCount(c *Client) int {
	c.internalClaimsMu.Lock()
	defer c.internalClaimsMu.Unlock()
	return len(c.internalClaims)
}

// 1. Codex TestReviewerForceCloseKeepsNativeCallbackJoin, extended to SubscribeForRequests: a
// handler passed to Subscribe or SubscribeForRequests is running when Close force-closes the
// connection under an ended context. A later Close with a live context must not return nil until
// the handler has returned.
func TestClientCloseJoinsSubscribeHandlerAfterForcedClose(t *testing.T) {
	for _, api := range []string{"Subscribe", "SubscribeForRequests"} {
		t.Run(api, func(t *testing.T) {
			c, err := NewClient(embeddedServerURL(t), WithHealthInterval(0))
			require.NoError(t, err)
			require.NoError(t, c.Connect(t.Context()))
			closeInCleanup(t, c)
			held := newHeldCall(t)
			subject := "close.final." + api
			switch api {
			case "Subscribe":
				_, err = c.Subscribe(t.Context(), subject, func(context.Context, *nats.Msg) { held.hold() })
			default:
				_, err = c.SubscribeForRequests(t.Context(), subject, func(context.Context, []byte) ([]byte, error) {
					held.hold()
					return nil, nil
				})
			}
			require.NoError(t, err)
			nc := c.GetConnection()
			require.NoError(t, nc.FlushTimeout(lifecycleBound))
			require.NoError(t, nc.Publish(subject, nil))
			await(t, held.entered, "the handler entering")

			require.ErrorIs(t, c.Close(endedContext(t)), context.Canceled, "Close under an ended context")
			requireJoinPending(t, c, joinOnOwnedWork, "the handler returned")

			held.Release()
			await(t, held.returned, "the handler returning")
			requireLiveCloseNil(t, c, "Close after the handler returned")
		})
	}
}

// startedNativeConsumer is Codex's reviewerStartedConsumer: its Consume delivers one message on
// another goroutine before returning the handle, and that goroutine closes the handle's Closed
// once the handler returns.
type startedNativeConsumer struct {
	jetstream.Consumer
	info            *jetstream.ConsumerInfo
	handle          *controlledNativeConsumeContext
	entered         chan struct{}
	returnHandle    chan struct{}
	callbackEntered <-chan struct{}
}

func (c *startedNativeConsumer) Info(context.Context) (*jetstream.ConsumerInfo, error) {
	return c.info, nil
}

func (c *startedNativeConsumer) Consume(handler jetstream.MessageHandler, _ ...jetstream.PullConsumeOpt) (jetstream.ConsumeContext, error) {
	go func() { handler(&headersOnlyMessage{}); close(c.handle.closed) }()
	<-c.callbackEntered
	close(c.entered)
	<-c.returnHandle
	return c.handle, nil
}

type headersOnlyMessage struct{ jetstream.Msg }

func (*headersOnlyMessage) Headers() nats.Header { return nil }

// consumeVia starts a consumer through the internal or the port API.
func consumeVia(
	ctx context.Context, c *Client, port bool, cfg StreamConsumerConfig,
	handler func(context.Context, jetstream.Msg),
) (jetstream.ConsumeContext, error) {
	if port {
		return c.ConsumeStreamWithConfig(ctx, PortConsumerContext{Component: "review", Port: "input"}, cfg, handler)
	}
	return c.ConsumeInternalStreamWithConfig(ctx, cfg, handler)
}

// 2. Codex TestReviewerRefusedConsumerRetainsJoinUntilClosed, assertions unchanged: native Consume
// delivers before returning its handle and Close wins. The refused setup must keep its claim and
// its place in Close's join until the handler has returned.
func TestClientRefusedConsumerKeepsOwnershipUntilHandlersReturn(t *testing.T) {
	for _, port := range []bool{false, true} {
		name := "internal"
		if port {
			name = "port"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				c, err := NewClient("nats://unused")
				require.NoError(t, err)
				entered := make(chan struct{})
				release := make(chan struct{})
				var once sync.Once
				unblock := func() { once.Do(func() { close(release) }) }
				defer unblock()
				handle := &controlledNativeConsumeContext{closed: make(chan struct{})}
				native := &startedNativeConsumer{
					info:   &jetstream.ConsumerInfo{Stream: "S_REVIEW", Name: "review", Config: jetstream.ConsumerConfig{Durable: "review", MaxAckPending: 17}},
					handle: handle, entered: make(chan struct{}), returnHandle: make(chan struct{}), callbackEntered: entered,
				}
				c.mu.Lock()
				c.js = &streamOnlyJetStream{fakeJetStream: &fakeJetStream{}, stream: &consumerOnlyStream{consumer: native}}
				c.mu.Unlock()
				c.setStatus(StatusConnected)
				returnHandle := releaseInCleanup(t, native.returnHandle)
				type result struct {
					handle jetstream.ConsumeContext
					err    error
				}
				results := make(chan result, 1)
				go func() {
					cfg := StreamConsumerConfig{StreamName: "S_REVIEW", ConsumerName: "review", MaxAckPending: 17}
					h, e := consumeVia(t.Context(), c, port, cfg, func(context.Context, jetstream.Msg) { close(entered); <-release })
					results <- result{h, e}
				}()
				select {
				case <-native.entered:
				case <-time.After(10 * time.Second):
					t.Fatal("native Consume did not deliver callback")
				}
				ended, cancel := context.WithCancel(t.Context())
				cancel()
				require.ErrorIs(t, c.Close(ended), context.Canceled)
				returnHandle()
				synctest.Wait()
				var early *result
				select {
				case r := <-results:
					early = &r
					select {
					case <-handle.closed:
						t.Fatal("diagnostic setup lost held callback")
					default:
						t.Errorf("refused consumer returned handle=%v err=%v while callback is running and native Closed is open (Stop=%v)", r.handle, r.err, handle.stopped.Load())
					}
				default:
				}
				if claimCount(c) == 0 {
					t.Error("local consumer claim was released before native Closed")
				}
				later := make(chan error, 1)
				live, cancelLive := context.WithTimeout(t.Context(), time.Hour)
				defer cancelLive()
				go func() { later <- c.Close(live) }()
				synctest.Wait()
				laterReturned := false
				select {
				case e := <-later:
					laterReturned = true
					if e == nil {
						t.Error("later Close returned nil while refused consumer callback still runs")
					}
				default:
				}
				unblock()
				if early == nil {
					select {
					case r := <-results:
						require.ErrorIs(t, r.err, nats.ErrConnectionClosed)
					case <-time.After(2 * time.Hour):
						t.Fatal("consumer did not finish after callback release")
					}
				} else {
					require.ErrorIs(t, early.err, nats.ErrConnectionClosed)
				}
				if !laterReturned {
					select {
					case e := <-later:
						require.NoError(t, e)
					case <-time.After(2 * time.Hour):
						t.Fatal("Close did not join after release")
					}
				}
			})
		})
	}
}

// 3. Codex TestReviewerConcurrentCloseDuringDrain, assertions unchanged: while the first Close
// drains a connection whose native callback is held, a second Close with an ended context
// returns at once.
func TestClientCloseHonoursItsContextDuringAnotherDrain(t *testing.T) {
	c, err := NewClient(embeddedServerURL(t), WithHealthInterval(0))
	require.NoError(t, err)
	require.NoError(t, c.Connect(t.Context()))
	callback, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	nc := c.GetConnection()
	_, err = nc.Subscribe("review.held", func(*nats.Msg) { close(callback); <-release })
	require.NoError(t, err)
	require.NoError(t, nc.FlushTimeout(10*time.Second))
	require.NoError(t, nc.Publish("review.held", nil))
	select {
	case <-callback:
	case <-time.After(10 * time.Second):
		t.Fatal("callback did not enter")
	}
	firstCtx, cancelFirst := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancelFirst()
	draining := nc.StatusChanged(nats.DRAINING_SUBS)
	defer nc.RemoveStatusListener(draining)
	first := make(chan error, 1)
	go func() { first <- c.Close(firstCtx) }()
	select {
	case <-draining:
	case <-time.After(10 * time.Second):
		t.Fatal("first Close did not begin drain")
	}
	ended, cancelEnded := context.WithCancel(t.Context())
	cancelEnded()
	second := make(chan error, 1)
	go func() { second <- c.Close(ended) }()
	var secondErr error
	returned := false
	select {
	case secondErr = <-second:
		returned = true
	case <-time.After(10 * time.Second):
	}
	t.Logf("second Close returned while first drain was held=%v; error=%v", returned, secondErr)
	once.Do(func() { close(release) })
	select {
	case err = <-first:
	case <-time.After(10 * time.Second):
		t.Fatal("first Close did not join after release")
	}
	if !returned {
		select {
		case secondErr = <-second:
		case <-time.After(10 * time.Second):
			t.Fatal("second Close did not return after release")
		}
	}
	t.Logf("after release: first=%v second=%v", err, secondErr)
	require.True(t, returned, "canceled Close waited on another caller's drain")
}

// 4. Codex TestReviewerLosingDialPreservesWinner (dial-error, canceled-dial, assertions
// unchanged): a Connect whose dial loses to a winner leaves the winner's status standing.
func TestClientLosingConnectLeavesWinnerStatus(t *testing.T) {
	for _, mode := range []string{"dial-error", "canceled-dial"} {
		t.Run(mode, func(t *testing.T) {
			c, err := NewClient(embeddedServerURL(t), WithHealthInterval(0))
			require.NoError(t, err)
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				_ = c.Close(ctx)
			}()
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			defer once.Do(func() { close(release) })
			dialCtx, cancelDial := context.WithCancel(t.Context())
			defer cancelDial()
			answer := make(chan error, 1)
			go func() {
				answer <- c.connectWith(dialCtx, func(string, ...nats.Option) (*nats.Conn, error) {
					close(entered)
					<-release
					return nil, errors.New("controlled dial failure")
				})
			}()
			select {
			case <-entered:
			case <-time.After(10 * time.Second):
				t.Fatal("dial did not enter")
			}
			require.NoError(t, c.Connect(t.Context()))
			winner := c.GetConnection()
			require.True(t, winner.IsConnected())
			require.Equal(t, StatusConnected, c.Status())
			if mode == "canceled-dial" {
				cancelDial()
			}
			once.Do(func() { close(release) })
			select {
			case err = <-answer:
			case <-time.After(10 * time.Second):
				t.Fatal("losing dial did not return")
			}
			t.Logf("loser=%v; same connection=%v; native connected=%v; client status=%v", err, winner == c.GetConnection(), winner.IsConnected(), c.Status())
			require.Equal(t, StatusConnected, c.Status(), "losing dial changed the winner's status")
		})
	}
}

// keptHandlerConsumer keeps the handler native Consume was given and returns handle; the test
// delivers through the kept handler when it chooses.
type keptHandlerConsumer struct {
	jetstream.Consumer
	info    *jetstream.ConsumerInfo
	handle  *controlledNativeConsumeContext
	handler chan jetstream.MessageHandler
}

func newKeptHandlerConsumer(stream, durable string, ack jetstream.AckPolicy) *keptHandlerConsumer {
	return &keptHandlerConsumer{
		info: &jetstream.ConsumerInfo{Stream: stream, Name: durable,
			Config: jetstream.ConsumerConfig{Durable: durable, MaxAckPending: 17, AckPolicy: ack}},
		handle:  &controlledNativeConsumeContext{closed: make(chan struct{})},
		handler: make(chan jetstream.MessageHandler, 1),
	}
}

func (c *keptHandlerConsumer) Info(context.Context) (*jetstream.ConsumerInfo, error) {
	return c.info, nil
}

func (c *keptHandlerConsumer) Consume(handler jetstream.MessageHandler, _ ...jetstream.PullConsumeOpt) (jetstream.ConsumeContext, error) {
	c.handler <- handler
	return c.handle, nil
}

// fakeConsumerClient returns a client whose JetStream serves native as its only consumer.
func fakeConsumerClient(t *testing.T, native jetstream.Consumer, opts ...ClientOption) *Client {
	c, err := NewClient("nats://unused", opts...)
	require.NoError(t, err)
	c.mu.Lock()
	c.js = &streamOnlyJetStream{fakeJetStream: &fakeJetStream{}, stream: &consumerOnlyStream{consumer: native}}
	c.mu.Unlock()
	c.setStatus(StatusConnected)
	return c
}

// 6. H1: a consumer handler is running when Close force-closes the connection; the caller then
// calls the handle's Closed(), which the native library closes at once for an invalid
// subscription. Close must still not return nil, nor release the claim, before the handler
// returns. Real JetStream on an embedded server, and a twin whose fake Closed() closes early.
func TestClientCloseJoinsConsumerHandlerWhenClosedReportsEarly(t *testing.T) {
	t.Run("jetstream", func(t *testing.T) {
		c, err := NewClient(embeddedJetStreamURL(t), WithHealthInterval(0))
		require.NoError(t, err)
		require.NoError(t, c.Connect(t.Context()))
		closeInCleanup(t, c)
		held := newHeldCall(t)
		cfg := StreamConsumerConfig{StreamName: "EARLY", ConsumerName: "early", FilterSubject: "early.>",
			AutoCreate: true, AutoCreateConfig: boundedAutoCreate()}
		handle, err := c.ConsumeInternalStreamWithConfig(t.Context(), cfg, func(_ context.Context, msg jetstream.Msg) {
			held.hold()
			_ = msg.Ack()
		})
		require.NoError(t, err)
		require.NoError(t, c.PublishToStream(t.Context(), "early.one", []byte("x")))
		await(t, held.entered, "the consumer handler entering")

		require.ErrorIs(t, c.Close(endedContext(t)), context.Canceled, "Close under an ended context")
		_ = handle.Closed() // the caller following the Consume doc comment
		requireJoinPending(t, c, joinOnOwnedWork, "the consumer handler returned")
		require.Equal(t, 1, claimCount(c), "the claim was released while the handler runs")

		held.Release()
		requireLiveCloseNil(t, c, "Close after the handler returned")
		require.Zero(t, claimCount(c), "the claim outlived a nil Close")
	})

	t.Run("fake", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			native := newKeptHandlerConsumer("S_EARLY", "early", jetstream.AckExplicitPolicy)
			c := fakeConsumerClient(t, native)
			endNative := releaseInCleanup(t, native.handle.closed)
			held := newHeldCall(t)
			_, err := c.ConsumeInternalStreamWithConfig(t.Context(),
				StreamConsumerConfig{StreamName: "S_EARLY", ConsumerName: "early"},
				func(context.Context, jetstream.Msg) { held.hold() })
			require.NoError(t, err)
			deliver := awaitValue(t, native.handler, "native Consume receiving the handler")
			go deliver(&mockMsg{subject: "early.one"})
			await(t, held.entered, "the consumer handler entering")

			require.ErrorIs(t, c.Close(endedContext(t)), context.Canceled)
			endNative() // native Closed() reports the end while the handler runs
			synctest.Wait()
			live, cancel := context.WithTimeout(t.Context(), time.Hour)
			defer cancel()
			later := closeAsync(live, c)
			synctest.Wait()
			select {
			case err := <-later:
				t.Fatalf("Close returned %v while the consumer handler runs", err)
			default:
			}
			require.Equal(t, 1, claimCount(c), "the claim was released while the handler runs")
			held.Release()
			require.NoError(t, awaitErr(t, later, "Close after the handler returned"))
			require.Zero(t, claimCount(c))
		})
	})
}

// 7. H2: a Connect after Close refuses before it dials and writes no status.
func TestClientConnectAfterCloseRefusesBeforeDial(t *testing.T) {
	url, accepted := silentListener(t)
	c, err := NewClient(url, WithHealthInterval(0))
	require.NoError(t, err)
	requireLiveCloseNil(t, c, "Close on a new client")

	require.ErrorIs(t, c.Connect(t.Context()), nats.ErrConnectionClosed, "Connect after Close")
	// A Connect that dialled the listener would have waited for an answer that never comes, so
	// the accept is in by the time it returns.
	select {
	case <-accepted:
		t.Error("Connect after Close dialled: the listener accepted a connection")
	default:
	}
	require.Equal(t, StatusDisconnected, c.Status())
}

// 8. H3: failures recorded by an operation still in flight after Close leave the status
// Disconnected and arm nothing.
func TestClientFailuresAfterCloseLeaveStatusDisconnected(t *testing.T) {
	c, err := NewClient("nats://unused")
	require.NoError(t, err)
	owner := newClientOwner(c)
	requireLiveCloseNil(t, c, "Close")
	for range c.circuitThreshold {
		c.recordFailure()
	}
	require.Equal(t, StatusDisconnected, c.Status(), "failures after Close changed the status")
	require.Empty(t, owner.Observe().Unresolved)
}

// 11. H6: the connection's error handler is running (held on its "NATS error" record, raised by a
// slow consumer) when Close is called. Close must not return nil until the connection's event
// handlers have finished.
func TestClientCloseJoinsConnectionEventHandlers(t *testing.T) {
	h := newRecordingHandler("NATS error")
	t.Cleanup(h.releaseHold)
	c, err := NewClient(embeddedServerURL(t), WithHealthInterval(0), WithLogger(slog.New(h)))
	require.NoError(t, err)
	require.NoError(t, c.Connect(t.Context()))
	closeInCleanup(t, c)
	nc := c.GetConnection()
	sub, err := nc.SubscribeSync("events.slow")
	require.NoError(t, err)
	require.NoError(t, sub.SetPendingLimits(1, 1<<20))
	for range 3 {
		require.NoError(t, nc.Publish("events.slow", []byte("x")))
	}
	require.NoError(t, nc.FlushTimeout(lifecycleBound))
	await(t, h.entered, "the error handler entering")
	require.NoError(t, sub.Unsubscribe())

	require.ErrorIs(t, c.Close(endedContext(t)), context.Canceled, "Close under an ended context")
	requireJoinPending(t, c, joinOnEventHandlers, "the error handler returned")
	h.releaseHold()
	requireLiveCloseNil(t, c, "Close after the error handler returned")
}

// jsErrors reads the JetStream error metric for one operation.
func jsErrors(c *Client, operation string) float64 {
	return testutil.ToFloat64(c.jsMetrics.errors.WithLabelValues(operation))
}

// 13. N7, M-4: once Close has begun, the async publish error handler still counts its metric and
// logs, but records no failure and changes no status.
func TestClientAsyncPublishErrorAfterCloseRecordsMetricOnly(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, err := NewClient("nats://unused", WithMetrics(metric.NewMetricsRegistry()))
		require.NoError(t, err)
		require.NoError(t, c.Close(t.Context()))
		msg := &nats.Msg{Subject: "async.after.close"}
		for range c.circuitThreshold {
			c.asyncPublishErrHandler(nil, msg, errors.New("ack failed"))
		}
		synctest.Wait()
		require.Zero(t, c.Failures(), "an async ack failure after Close was counted")
		require.Equal(t, StatusDisconnected, c.Status())
		require.Equal(t, float64(c.circuitThreshold), jsErrors(c, "publish_async"),
			"the publish_async metric stopped counting after Close")
	})
}

// tableState builds a client in one lifecycle state and returns what the operations need.
type tableClient struct {
	c          *Client
	release    func()       // releases the held drain in the closing state
	firstClose <-chan error // the draining Close in the closing state
}

// 14. F25: every lifecycle state against every operation. Expected returns and statuses are
// written out from D3, never derived from the client.
func TestClientLifecycleOperationTable(t *testing.T) {
	url := embeddedJetStreamURL(t)
	type expect struct {
		err    error // nil, or what errors.Is must match
		status ConnectionStatus
	}
	closed := nats.ErrConnectionClosed
	notConnected := ErrNotConnected
	want := map[string]map[string]expect{
		"new": {
			"Connect": {nil, StatusConnected}, "CloseLive": {nil, StatusDisconnected},
			"CloseEnded": {context.Canceled, StatusDisconnected}, "Subscribe": {notConnected, StatusDisconnected},
			"SubscribeForRequests":            {notConnected, StatusDisconnected},
			"ConsumeStreamWithConfig":         {notConnected, StatusDisconnected},
			"ConsumeInternalStreamWithConfig": {notConnected, StatusDisconnected},
		},
		"connected": {
			"Connect": {errs.ErrAlreadyStarted, StatusConnected}, "CloseLive": {nil, StatusDisconnected},
			"CloseEnded": {context.Canceled, StatusDisconnected}, "Subscribe": {nil, StatusConnected},
			"SubscribeForRequests": {nil, StatusConnected}, "ConsumeStreamWithConfig": {nil, StatusConnected},
			"ConsumeInternalStreamWithConfig": {nil, StatusConnected},
		},
		"closing": {
			"Connect": {closed, StatusConnected}, "CloseLive": {nil, StatusDisconnected},
			"CloseEnded": {context.Canceled, StatusConnected}, "Subscribe": {closed, StatusConnected},
			"SubscribeForRequests": {closed, StatusConnected}, "ConsumeStreamWithConfig": {closed, StatusConnected},
			"ConsumeInternalStreamWithConfig": {closed, StatusConnected},
		},
		"closed": {
			"Connect": {closed, StatusDisconnected}, "CloseLive": {nil, StatusDisconnected},
			"CloseEnded": {context.Canceled, StatusDisconnected}, "Subscribe": {closed, StatusDisconnected},
			"SubscribeForRequests":            {closed, StatusDisconnected},
			"ConsumeStreamWithConfig":         {closed, StatusDisconnected},
			"ConsumeInternalStreamWithConfig": {closed, StatusDisconnected},
		},
	}
	stateNames := []string{"new", "connected", "closing", "closed"}
	opNames := []string{"Connect", "CloseLive", "CloseEnded", "Subscribe", "SubscribeForRequests",
		"ConsumeStreamWithConfig", "ConsumeInternalStreamWithConfig"}
	n := 0
	for _, state := range stateNames {
		for _, op := range opNames {
			n++
			id := n
			t.Run(state+"/"+op, func(t *testing.T) {
				tc := buildTableClient(t, url, state)
				err := runTableOp(t, tc, op, id)
				exp := want[state][op]
				if exp.err == nil {
					require.NoError(t, err)
				} else {
					require.ErrorIs(t, err, exp.err)
				}
				require.Equal(t, exp.status, tc.c.Status(), "status after %s in state %s", op, state)
				if tc.release != nil {
					tc.release()
					_ = awaitErr(t, tc.firstClose, "the draining Close")
				}
			})
		}
	}
}

func buildTableClient(t *testing.T, url, state string) *tableClient {
	t.Helper()
	c, err := NewClient(url, WithHealthInterval(0))
	require.NoError(t, err)
	tc := &tableClient{c: c}
	closeInCleanup(t, c)
	if state == "new" {
		return tc
	}
	require.NoError(t, c.Connect(t.Context()))
	dialled := c.GetConnection()
	switch state {
	case "closing":
		held := newHeldCall(t)
		_, err := c.Subscribe(t.Context(), "table.held", func(context.Context, *nats.Msg) { held.hold() })
		require.NoError(t, err)
		require.NoError(t, dialled.FlushTimeout(lifecycleBound))
		require.NoError(t, dialled.Publish("table.held", nil))
		await(t, held.entered, "the held handler entering")
		draining := dialled.StatusChanged(nats.DRAINING_SUBS)
		t.Cleanup(func() { dialled.RemoveStatusListener(draining) })
		ctx, cancel := context.WithTimeout(t.Context(), lifecycleBound)
		t.Cleanup(cancel)
		tc.firstClose = closeAsync(ctx, c)
		await(t, onceClosed(draining), "the first Close draining")
		tc.release = held.Release
	case "closed":
		requireLiveCloseNil(t, c, "Close")
	}
	return tc
}

// onceClosed turns a status-listener channel into one that closes on its first value.
func onceClosed(ch chan nats.Status) <-chan struct{} {
	out := make(chan struct{})
	go func() {
		<-ch
		close(out)
	}()
	return out
}

func runTableOp(t *testing.T, tc *tableClient, op string, id int) error {
	c := tc.c
	ctx := t.Context()
	stream := "TABLE_" + string(rune('A'+id%26)) + string(rune('A'+id/26))
	cfg := StreamConsumerConfig{StreamName: stream, ConsumerName: "table", FilterSubject: "table." + stream + ".>",
		AutoCreate: true, AutoCreateConfig: boundedAutoCreate()}
	noop := func(context.Context, jetstream.Msg) {}
	switch op {
	case "Connect":
		return c.Connect(ctx)
	case "CloseLive":
		live, cancel := context.WithTimeout(ctx, lifecycleBound)
		defer cancel()
		if tc.release == nil {
			return c.Close(live)
		}
		out := closeAsync(live, c)
		// A precondition, not the outcome: the held drain keeps the join open, so this live
		// Close cannot have returned nil yet. What a Close returns while handlers are held is
		// TestClientRefusedConsumerKeepsOwnershipUntilHandlersReturn's and test 1's to check.
		requireJoinOpen(t, c, "the held drain was released")
		tc.release()
		return awaitErr(t, out, "the live Close")
	case "CloseEnded":
		return c.Close(endedContext(t))
	case "Subscribe":
		_, err := c.Subscribe(ctx, "table.sub", func(context.Context, *nats.Msg) {})
		return err
	case "SubscribeForRequests":
		_, err := c.SubscribeForRequests(ctx, "table.req", func(context.Context, []byte) ([]byte, error) { return nil, nil })
		return err
	case "ConsumeStreamWithConfig":
		_, err := c.ConsumeStreamWithConfig(ctx, PortConsumerContext{Component: "table", Port: "input"}, cfg, noop)
		return err
	case "ConsumeInternalStreamWithConfig":
		_, err := c.ConsumeInternalStreamWithConfig(ctx, cfg, noop)
		return err
	}
	t.Fatalf("unknown operation %s", op)
	return nil
}

// 15. H8: the lifecycle adapter lists a running message handler after Close has cleared the
// connection, and lists nothing once a nil Close has returned.
func TestClientLifecycleAdapterListsHeldHandler(t *testing.T) {
	c, err := NewClient(embeddedServerURL(t), WithHealthInterval(0))
	require.NoError(t, err)
	owner := newClientOwner(c)
	require.NoError(t, c.Connect(t.Context()))
	closeInCleanup(t, c)
	held := newHeldCall(t)
	_, err = c.Subscribe(t.Context(), "adapter.held", func(context.Context, *nats.Msg) { held.hold() })
	require.NoError(t, err)
	nc := c.GetConnection()
	require.NoError(t, nc.FlushTimeout(lifecycleBound))
	require.NoError(t, nc.Publish("adapter.held", nil))
	await(t, held.entered, "the handler entering")

	require.ErrorIs(t, c.Close(endedContext(t)), context.Canceled)
	require.Contains(t, owner.Observe().Unresolved, "message handler",
		"the adapter does not list a handler that is still running")
	held.Release()
	requireLiveCloseNil(t, c, "Close after the handler returned")
	require.Empty(t, owner.Observe().Unresolved)
}

// lateRefusals reads the warn records of refused late deliveries.
func lateRefusals(h *recordingHandler) []loggedRecord {
	return h.find(func(r loggedRecord) bool {
		return r.level == slog.LevelWarn && r.msg == "NATS client refused a delivery after its subscription ended"
	})
}

// fakeInvalidSubscription reports itself invalid from creation, as a subscription that ended
// before the client set its closed handler does.
type fakeInvalidSubscription struct{}

func (fakeInvalidSubscription) Drain() error                  { return nil }
func (fakeInvalidSubscription) IsValid() bool                 { return false }
func (fakeInvalidSubscription) SetClosedHandler(func(string)) {}
func (fakeInvalidSubscription) Unsubscribe() error            { return nil }

// 16. F-4(a), F-2: a delivery the native library hands over after the client recorded that
// consumer's or subscription's end does not run the handler. It is not settled, it is logged at
// warn level with the subject and ack policy, and it is counted as late_delivery_refused.
func TestClientRefusesLateDeliveryAfterRecordedEnd(t *testing.T) {
	for _, tc := range []struct {
		name string
		port bool
		cfg  string
		ack  jetstream.AckPolicy
	}{
		{"ack-explicit", false, "explicit", jetstream.AckExplicitPolicy},
		{"ack-none", true, "none", jetstream.AckNonePolicy},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := newRecordingHandler("")
				native := newKeptHandlerConsumer("S_LATE", "late", tc.ack)
				c := fakeConsumerClient(t, native, WithLogger(slog.New(h)), WithMetrics(metric.NewMetricsRegistry()))
				endNative := releaseInCleanup(t, native.handle.closed)
				ran := false
				_, err := consumeVia(t.Context(), c, tc.port,
					StreamConsumerConfig{StreamName: "S_LATE", ConsumerName: "late", AckPolicy: tc.cfg, MaxAckPending: 17},
					func(context.Context, jetstream.Msg) { ran = true })
				require.NoError(t, err)
				deliver := awaitValue(t, native.handler, "native Consume receiving the handler")
				endNative() // the end, with no invocation running
				synctest.Wait()

				msg := &mockMsg{subject: "late.one"}
				deliver(msg)
				require.False(t, ran, "a delivery after the recorded end ran the handler")
				require.Zero(t, msg.ackCount.Load()+msg.nakCount.Load()+msg.termCount.Load(), "the refused delivery was settled")
				refusals := lateRefusals(h)
				require.Len(t, refusals, 1, "the refusal was not logged at warn level")
				assert.Equal(t, "late.one", refusals[0].attrs["subject"])
				assert.Equal(t, tc.ack.String(), refusals[0].attrs["ack_policy"])
				assert.Equal(t, "S_LATE", refusals[0].attrs["stream"])
				assert.Equal(t, "late", refusals[0].attrs["consumer"])
				require.Equal(t, float64(1), jsErrors(c, "late_delivery_refused"))
				require.NoError(t, c.Close(t.Context()))
			})
		})
	}

	t.Run("core", func(t *testing.T) {
		h := newRecordingHandler("")
		c, err := NewClient(embeddedServerURL(t), WithHealthInterval(0), WithLogger(slog.New(h)),
			WithMetrics(metric.NewMetricsRegistry()))
		require.NoError(t, err)
		require.NoError(t, c.Connect(t.Context()))
		closeInCleanup(t, c)
		var kept nats.MsgHandler
		ran := false
		_, err = c.subscribeWith(t.Context(), "late.core", func(context.Context, *nats.Msg) { ran = true },
			func(_ *nats.Conn, _ string, cb nats.MsgHandler) (nativeSubscription, error) {
				kept = cb
				return fakeInvalidSubscription{}, nil
			})
		require.NoError(t, err)
		kept(&nats.Msg{Subject: "late.core"})
		require.False(t, ran, "a delivery after the recorded end ran the handler")
		refusals := lateRefusals(h)
		require.Len(t, refusals, 1, "the refusal was not logged at warn level")
		assert.Equal(t, "late.core", refusals[0].attrs["subject"])
		assert.Equal(t, "core", refusals[0].attrs["ack_policy"])
		require.Equal(t, float64(1), jsErrors(c, "late_delivery_refused"))
	})
}

// 17. F-4(b): as test 2, but the setup context ends while the handler is held after Close began.
// The setup returns that context's error; the claim stays held, and Close does not return nil,
// until the handler returns.
func TestClientRefusedConsumerSetupContextEndsWhileHandlerRuns(t *testing.T) {
	for _, port := range []bool{false, true} {
		name := "internal"
		if port {
			name = "port"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				entered := make(chan struct{})
				held := newHeldCall(t)
				handle := &controlledNativeConsumeContext{closed: make(chan struct{})}
				native := &startedNativeConsumer{
					info:   &jetstream.ConsumerInfo{Stream: "S_SETUP", Name: "setup", Config: jetstream.ConsumerConfig{Durable: "setup", MaxAckPending: 17}},
					handle: handle, entered: make(chan struct{}), returnHandle: make(chan struct{}), callbackEntered: entered,
				}
				c := fakeConsumerClient(t, native)
				returnHandle := releaseInCleanup(t, native.returnHandle)
				setupCtx, cancelSetup := context.WithCancel(t.Context())
				defer cancelSetup()
				type result struct {
					handle jetstream.ConsumeContext
					err    error
				}
				results := make(chan result, 1)
				go func() {
					cfg := StreamConsumerConfig{StreamName: "S_SETUP", ConsumerName: "setup", MaxAckPending: 17}
					h, e := consumeVia(setupCtx, c, port, cfg, func(context.Context, jetstream.Msg) {
						close(entered)
						held.hold()
					})
					results <- result{h, e}
				}()
				await(t, native.entered, "native Consume entering")
				require.ErrorIs(t, c.Close(endedContext(t)), context.Canceled)
				returnHandle()
				synctest.Wait()
				select {
				case r := <-results:
					t.Fatalf("the refused setup returned (%v, %v) while its handler runs and its context is live", r.handle, r.err)
				default:
				}

				cancelSetup()
				synctest.Wait()
				var r result
				select {
				case r = <-results:
				default:
					t.Fatal("the setup did not return when its context ended")
				}
				require.Nil(t, r.handle)
				require.ErrorIs(t, r.err, context.Canceled)
				require.Equal(t, 1, claimCount(c), "the claim was released while the handler runs")

				live, cancel := context.WithTimeout(t.Context(), time.Hour)
				defer cancel()
				later := closeAsync(live, c)
				synctest.Wait()
				select {
				case err := <-later:
					t.Fatalf("Close returned %v while the refused consumer's handler runs", err)
				default:
				}
				held.Release()
				require.NoError(t, awaitErr(t, later, "Close after the handler returned"))
				require.Zero(t, claimCount(c))
			})
		})
	}
}

// 18. F-4(c): an async publish error handler entered before Close is joined: a live Close does
// not return nil while it runs.
func TestClientCloseJoinsRunningAsyncPublishErrorHandler(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newRecordingHandler("Async publish ack failed")
		t.Cleanup(h.releaseHold)
		c, err := NewClient("nats://unused", WithLogger(slog.New(h)))
		require.NoError(t, err)
		done := make(chan struct{})
		go func() {
			defer close(done)
			c.asyncPublishErrHandler(nil, &nats.Msg{Subject: "async.held"}, errors.New("ack failed"))
		}()
		await(t, h.entered, "the async publish error handler entering")

		require.ErrorIs(t, c.Close(endedContext(t)), context.Canceled)
		short, cancel := context.WithTimeout(t.Context(), time.Minute)
		defer cancel()
		require.ErrorIs(t, c.Close(short), context.DeadlineExceeded,
			"a live Close returned while the async publish error handler runs")
		h.releaseHold()
		await(t, done, "the async publish error handler returning")
		require.NoError(t, c.Close(t.Context()))
	})
}

// 19. F-3, H9: a client-owned handler outlasts the drain. Whichever timer runs out first, the
// native drain's or the client's own, the first Close returns a transient error wrapping
// nats.ErrDrainTimeout, after the handler has returned.
func TestClientCloseReportsDrainTimeout(t *testing.T) {
	const short, long = 100 * time.Millisecond, 30 * time.Second
	for _, tc := range []struct {
		name                 string
		nativeTimer, ownTime time.Duration
	}{
		{"native-first", short, long},
		{"client-first", long, short},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := NewClient(embeddedServerURL(t), WithHealthInterval(0))
			require.NoError(t, err)
			c.drainTimeout = tc.nativeTimer // the connection's options capture it at Connect
			require.NoError(t, c.Connect(t.Context()))
			closeInCleanup(t, c)
			c.mu.Lock()
			c.drainTimeout = tc.ownTime
			c.mu.Unlock()
			held := newHeldCall(t)
			_, err = c.Subscribe(t.Context(), "drain.timeout", func(context.Context, *nats.Msg) { held.hold() })
			require.NoError(t, err)
			nc := c.GetConnection()
			require.NoError(t, nc.FlushTimeout(lifecycleBound))
			require.NoError(t, nc.Publish("drain.timeout", nil))
			await(t, held.entered, "the handler entering")
			closedStatus := nc.StatusChanged(nats.CLOSED)
			defer nc.RemoveStatusListener(closedStatus)

			ctx, cancel := context.WithTimeout(t.Context(), 2*long)
			defer cancel()
			first := closeAsync(ctx, c)
			await(t, onceClosed(closedStatus), "the connection closing after the drain timeout")
			held.Release()
			err = awaitErr(t, first, "the first Close")
			require.ErrorIs(t, err, nats.ErrDrainTimeout, "the first Close did not report the drain timeout")
			require.True(t, errs.IsTransient(err), "the drain timeout is not classified transient: %v", err)
		})
	}
}
