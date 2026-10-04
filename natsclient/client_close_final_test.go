package natsclient

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"testing/synctest"
	"time"

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

// stillWaitingBound is the deadline of a Close that must not return nil yet: a correct client
// returns context.DeadlineExceeded when it passes, because a held handler or callback keeps the
// join open. The bound only decides how long the check takes; no outcome depends on it.
const stillWaitingBound = 100 * time.Millisecond

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

// requireCloseStillWaiting calls Close with a short live deadline. A client that still owns
// running work must report the deadline, never nil.
func requireCloseStillWaiting(t *testing.T, c *Client, what string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), stillWaitingBound)
	defer cancel()
	require.ErrorIs(t, c.Close(ctx), context.DeadlineExceeded, "a live Close returned before %s", what)
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

// runIfCommitFree runs fn to completion when the client's critical section is free at the
// moment the commit seam fires. The seam is called by the code under test between its ownership
// check and its status write; if that check and write are one step, the section is held by the
// seam's caller and fn cannot run in between, so it is left for the test to run afterwards.
func runIfCommitFree(c *Client, fn func()) bool {
	if !c.mu.TryLock() {
		return false
	}
	c.mu.Unlock()
	fn()
	return true
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
			requireCloseStillWaiting(t, c, "the handler returned")

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
				close(native.returnHandle)
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
// unchanged), plus two cases: a loser held after its started check and before its Connecting
// write, and a loser whose failure write is held at the commit seam. In every case the winner's
// status, failure count and circuit stand.
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

	t.Run("held-before-connecting", func(t *testing.T) {
		c, err := NewClient(embeddedServerURL(t), WithHealthInterval(0))
		require.NoError(t, err)
		closeInCleanup(t, c)
		held, release := make(chan struct{}), make(chan struct{})
		var holdOnce, releaseOnce sync.Once
		t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
		c.opHook = func(op string) {
			first := false
			holdOnce.Do(func() { first = op == "dial" })
			if first {
				close(held)
				<-release
			}
		}
		dialEntered, dialRelease := make(chan struct{}), make(chan struct{})
		var dialOnce sync.Once
		t.Cleanup(func() { dialOnce.Do(func() { close(dialRelease) }) })
		loser := make(chan error, 1)
		go func() {
			loser <- c.connectWith(t.Context(), func(string, ...nats.Option) (*nats.Conn, error) {
				close(dialEntered)
				<-dialRelease
				return nil, errors.New("controlled dial failure")
			})
		}()
		await(t, held, "the loser reaching its dial report")
		require.NoError(t, c.Connect(t.Context()), "the winner")
		require.Equal(t, StatusConnected, c.Status())

		releaseOnce.Do(func() { close(release) })
		select {
		case <-dialEntered:
		case err := <-loser:
			loser <- err
		case <-time.After(lifecycleBound):
			t.Fatal("the loser neither dialled nor returned")
		}
		require.Equal(t, StatusConnected, c.Status(), "the loser showed its own status over the winner's")
		dialOnce.Do(func() { close(dialRelease) })
		require.Error(t, awaitErr(t, loser, "the loser"))
		require.Equal(t, StatusConnected, c.Status())
		require.Zero(t, c.Failures(), "the loser recorded a failure against the winner's connection")
	})

	t.Run("held-before-failure-write", func(t *testing.T) {
		c, err := NewClient(embeddedServerURL(t), WithHealthInterval(0))
		require.NoError(t, err)
		closeInCleanup(t, c)
		var hookOnce sync.Once
		winnerRan := false
		var winnerErr error
		c.commitHook = func(site string) {
			if site != "connect failure" {
				return
			}
			hookOnce.Do(func() {
				winnerRan = runIfCommitFree(c, func() { winnerErr = c.Connect(t.Context()) })
			})
		}
		err = c.connectWith(t.Context(), func(string, ...nats.Option) (*nats.Conn, error) {
			return nil, errors.New("controlled dial failure")
		})
		require.Error(t, err, "the loser")
		if !winnerRan {
			winnerErr = c.Connect(t.Context())
		}
		require.NoError(t, winnerErr, "the winner")
		require.Equal(t, StatusConnected, c.Status(), "the loser's failure write landed after the winner installed")
		require.Zero(t, c.Failures(), "the loser's failure was counted against the winner's connection")
	})
}

// 5. Codex F23 monitor ordering: the health monitor is held between its health check and its
// commit; Close finishes its cleanup and writes Disconnected; the monitor is released. The
// status must stay Disconnected.
func TestClientHealthMonitorCannotOverwriteClosedStatus(t *testing.T) {
	c, err := NewClient(embeddedServerURL(t), WithHealthInterval(time.Millisecond))
	require.NoError(t, err)
	held := newHeldCall(t)
	var holdOnce sync.Once
	c.commitHook = func(site string) {
		if site == "health monitor" {
			holdOnce.Do(held.hold)
		}
	}
	require.NoError(t, c.Connect(t.Context()))
	closeInCleanup(t, c)
	await(t, held.entered, "the monitor reaching its commit")

	ctx, cancel := context.WithTimeout(t.Context(), lifecycleBound)
	defer cancel()
	closed := closeAsync(ctx, c)
	require.Eventually(t, func() bool { return c.Status() == StatusDisconnected }, lifecycleBound, time.Millisecond,
		"Close did not finish its cleanup")
	held.Release()
	require.NoError(t, awaitErr(t, closed, "Close"))
	require.Equal(t, StatusDisconnected, c.Status(), "the monitor committed a status after Close")
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
		requireCloseStillWaiting(t, c, "the consumer handler returned")
		require.Equal(t, 1, claimCount(c), "the claim was released while the handler runs")

		held.Release()
		requireLiveCloseNil(t, c, "Close after the handler returned")
		require.Zero(t, claimCount(c), "the claim outlived a nil Close")
	})

	t.Run("fake", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			native := newKeptHandlerConsumer("S_EARLY", "early", jetstream.AckExplicitPolicy)
			c := fakeConsumerClient(t, native)
			held := newHeldCall(t)
			_, err := c.ConsumeInternalStreamWithConfig(t.Context(),
				StreamConsumerConfig{StreamName: "S_EARLY", ConsumerName: "early"},
				func(context.Context, jetstream.Msg) { held.hold() })
			require.NoError(t, err)
			deliver := <-native.handler
			go deliver(&mockMsg{subject: "early.one"})
			<-held.entered

			require.ErrorIs(t, c.Close(endedContext(t)), context.Canceled)
			close(native.handle.closed) // native Closed() reports the end while the handler runs
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
			require.NoError(t, <-later)
			require.Zero(t, claimCount(c))
		})
	})
}

// 7. H2: a Connect after Close refuses before it dials and writes no status.
func TestClientConnectAfterCloseRefusesBeforeDial(t *testing.T) {
	c, err := NewClient(embeddedServerURL(t), WithHealthInterval(0))
	require.NoError(t, err)
	owner := newClientOwner(c)
	requireLiveCloseNil(t, c, "Close on a new client")

	var dialled []ConnectionStatus
	err = c.connectWith(t.Context(), func(string, ...nats.Option) (*nats.Conn, error) {
		dialled = append(dialled, c.Status())
		return nil, errors.New("dial after Close")
	})
	require.ErrorIs(t, err, nats.ErrConnectionClosed, "Connect after Close")
	require.Empty(t, dialled, "Connect after Close dialled (status during the dial: %v)", dialled)
	require.Zero(t, owner.Observe().Calls["dial"], "Connect after Close reported a dial")
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

// 9. H4: a Connect that passed its started check before the winner installed is released while
// Close drains that connection with a native callback held. It returns at once.
func TestClientConnectDuringCloseDrainReturnsPromptly(t *testing.T) {
	c, err := NewClient(embeddedServerURL(t), WithHealthInterval(0))
	require.NoError(t, err)
	closeInCleanup(t, c)
	held, release := make(chan struct{}), make(chan struct{})
	var holdOnce, releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	c.opHook = func(op string) {
		first := false
		holdOnce.Do(func() { first = op == "dial" })
		if first {
			close(held)
			<-release
		}
	}
	late := make(chan error, 1)
	go func() { late <- c.Connect(t.Context()) }()
	await(t, held, "the late Connect reaching its dial report")
	require.NoError(t, c.Connect(t.Context()), "the winner")

	callback := newHeldCall(t)
	nc := c.GetConnection()
	_, err = nc.Subscribe("drain.held", func(*nats.Msg) { callback.hold() })
	require.NoError(t, err)
	require.NoError(t, nc.FlushTimeout(lifecycleBound))
	require.NoError(t, nc.Publish("drain.held", nil))
	await(t, callback.entered, "the native callback entering")
	draining := nc.StatusChanged(nats.DRAINING_SUBS)
	defer nc.RemoveStatusListener(draining)
	firstCtx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	first := closeAsync(firstCtx, c)
	select {
	case <-draining:
	case <-time.After(lifecycleBound):
		t.Fatal("the first Close did not begin its drain")
	}

	releaseOnce.Do(func() { close(release) })
	require.ErrorIs(t, awaitErr(t, late, "Connect during the drain"), nats.ErrConnectionClosed)
	callback.Release()
	_ = awaitErr(t, first, "the first Close")
}

// 10. H5: a connection event handler is held after its connection check; Close runs. The handler's
// status write must not land after Close.
func TestClientEventHandlerCannotCommitAfterClose(t *testing.T) {
	for _, tc := range []struct {
		name    string
		handler func(*Client)
	}{
		{"handleReconnect", func(c *Client) { c.handleReconnect(nil) }},
		{"handleDisconnect", func(c *Client) { c.handleDisconnect(nil, errors.New("gone")) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := NewClient("nats://unused")
			require.NoError(t, err)
			closed := false
			var closeErr error
			var hookOnce sync.Once
			c.commitHook = func(site string) {
				if site != "event handler" {
					return
				}
				hookOnce.Do(func() {
					closed = runIfCommitFree(c, func() {
						ctx, cancel := context.WithTimeout(t.Context(), lifecycleBound)
						defer cancel()
						closeErr = c.Close(ctx)
					})
				})
			}
			tc.handler(c)
			if !closed {
				requireLiveCloseNil(t, c, "Close after the handler")
			} else {
				require.NoError(t, closeErr)
			}
			require.Equal(t, StatusDisconnected, c.Status(), "the handler committed a status after Close")
		})
	}
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

	requireCloseStillWaiting(t, c, "the error handler returned")
	h.releaseHold()
	requireLiveCloseNil(t, c, "Close after the error handler returned")
}

// 12. Ownership by call: a Connect that loses to an installed connection does not return while
// its candidate connection's closed handler runs. Extends
// TestClientConnectThatLosesAdmissionLeavesStatus; the hold is the test's own closed handler.
func TestClientLosingConnectLeavesNoCandidateHandler(t *testing.T) {
	c, err := NewClient(embeddedServerURL(t), WithHealthInterval(0))
	require.NoError(t, err)
	closeInCleanup(t, c)
	held, release := make(chan struct{}), make(chan struct{})
	var holdOnce, releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	c.opHook = func(op string) {
		first := false
		holdOnce.Do(func() { first = op == "dial" })
		if first {
			close(held)
			<-release
		}
	}
	candidateClosed := newHeldCall(t)
	loserDial := func(url string, opts ...nats.Option) (*nats.Conn, error) {
		return nats.Connect(url, append(opts, nats.ClosedHandler(func(nc *nats.Conn) {
			candidateClosed.hold()
			c.handleClosed(nc)
		}))...)
	}
	loser := make(chan error, 1)
	go func() { loser <- c.connectWith(t.Context(), loserDial) }()
	await(t, held, "the loser reaching its dial report")
	require.NoError(t, c.Connect(t.Context()), "the winner")

	releaseOnce.Do(func() { close(release) })
	await(t, candidateClosed.entered, "the candidate's closed handler entering")
	select {
	case err := <-loser:
		t.Fatalf("the losing Connect returned %v while its candidate's closed handler runs", err)
	case <-time.After(stillWaitingBound):
	}
	candidateClosed.Release()
	require.ErrorIs(t, awaitErr(t, loser, "the losing Connect"), errs.ErrAlreadyStarted)
	require.Equal(t, StatusConnected, c.Status())
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
	owner      *clientOwner
	logs       *recordingHandler
	url        string
	release    func()       // releases the held drain in the closing state
	firstClose <-chan error // the draining Close in the closing state
	conn       *nats.Conn   // GetConnection() before the operation
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
			"SetConnection":                   {nil, StatusConnected},
		},
		"connected": {
			"Connect": {errs.ErrAlreadyStarted, StatusConnected}, "CloseLive": {nil, StatusDisconnected},
			"CloseEnded": {context.Canceled, StatusDisconnected}, "Subscribe": {nil, StatusConnected},
			"SubscribeForRequests": {nil, StatusConnected}, "ConsumeStreamWithConfig": {nil, StatusConnected},
			"ConsumeInternalStreamWithConfig": {nil, StatusConnected}, "SetConnection": {nil, StatusConnected},
		},
		"closing": {
			"Connect": {closed, StatusConnected}, "CloseLive": {nil, StatusDisconnected},
			"CloseEnded": {context.Canceled, StatusConnected}, "Subscribe": {closed, StatusConnected},
			"SubscribeForRequests": {closed, StatusConnected}, "ConsumeStreamWithConfig": {closed, StatusConnected},
			"ConsumeInternalStreamWithConfig": {closed, StatusConnected}, "SetConnection": {nil, StatusConnected},
		},
		"closed": {
			"Connect": {closed, StatusDisconnected}, "CloseLive": {nil, StatusDisconnected},
			"CloseEnded": {context.Canceled, StatusDisconnected}, "Subscribe": {closed, StatusDisconnected},
			"SubscribeForRequests":            {closed, StatusDisconnected},
			"ConsumeStreamWithConfig":         {closed, StatusDisconnected},
			"ConsumeInternalStreamWithConfig": {closed, StatusDisconnected}, "SetConnection": {nil, StatusDisconnected},
		},
	}
	// SetConnection installs the given connection only before Close has begun (owner answer 3);
	// once Close has begun it changes nothing and logs at warn level.
	installs := map[string]bool{"new": true, "connected": true, "closing": false, "closed": false}
	// Connect dials only before Close has begun.
	dials := map[string]bool{"new": true, "connected": false, "closing": false, "closed": false}

	stateNames := []string{"new", "connected", "closing", "closed"}
	opNames := []string{"Connect", "CloseLive", "CloseEnded", "Subscribe", "SubscribeForRequests",
		"ConsumeStreamWithConfig", "ConsumeInternalStreamWithConfig", "SetConnection"}
	n := 0
	for _, state := range stateNames {
		for _, op := range opNames {
			n++
			id := n
			t.Run(state+"/"+op, func(t *testing.T) {
				tc := buildTableClient(t, url, state)
				dialsBefore := tc.owner.Observe().Calls["dial"]
				var other *nats.Conn
				if op == "SetConnection" {
					var err error
					other, err = nats.Connect(url)
					require.NoError(t, err)
					t.Cleanup(other.Close)
				}
				err := runTableOp(t, tc, op, id, other)
				exp := want[state][op]
				if exp.err == nil {
					require.NoError(t, err)
				} else {
					require.ErrorIs(t, err, exp.err)
				}
				require.Equal(t, exp.status, tc.c.Status(), "status after %s in state %s", op, state)
				if op == "Connect" {
					dialled := tc.owner.Observe().Calls["dial"] > dialsBefore
					require.Equal(t, dials[state], dialled, "Connect dialled in state %s", state)
				}
				if op == "SetConnection" {
					if installs[state] {
						require.Same(t, other, tc.c.GetConnection())
					} else {
						require.NotSame(t, other, tc.c.GetConnection(), "SetConnection installed once Close had begun")
						require.NotEmpty(t, tc.logs.find(func(r loggedRecord) bool {
							return r.level == slog.LevelWarn && r.msg == "NATS client closing; SetConnection ignored"
						}), "SetConnection once Close had begun was not logged at warn level")
					}
				}
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
	h := newRecordingHandler("")
	c, err := NewClient(url, WithHealthInterval(0), WithLogger(slog.New(h)))
	require.NoError(t, err)
	tc := &tableClient{c: c, owner: newClientOwner(c), logs: h, url: url}
	closeInCleanup(t, c)
	if state == "new" {
		return tc
	}
	require.NoError(t, c.Connect(t.Context()))
	// The test owns the connection the client dialled once SetConnection may replace it.
	dialled := c.GetConnection()
	t.Cleanup(dialled.Close)
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
	tc.conn = c.GetConnection()
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

func runTableOp(t *testing.T, tc *tableClient, op string, id int, other *nats.Conn) error {
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
		select {
		case err := <-out:
			t.Fatalf("a live Close returned %v while the drain is held", err)
		case <-time.After(stillWaitingBound):
		}
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
	case "SetConnection":
		c.SetConnection(other)
		return nil
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
				ran := false
				_, err := consumeVia(t.Context(), c, tc.port,
					StreamConsumerConfig{StreamName: "S_LATE", ConsumerName: "late", AckPolicy: tc.cfg, MaxAckPending: 17},
					func(context.Context, jetstream.Msg) { ran = true })
				require.NoError(t, err)
				deliver := <-native.handler
				close(native.handle.closed) // the end, with no invocation running
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
				<-native.entered
				require.ErrorIs(t, c.Close(endedContext(t)), context.Canceled)
				close(native.returnHandle)
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
				require.NoError(t, <-later)
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
		<-h.entered

		require.ErrorIs(t, c.Close(endedContext(t)), context.Canceled)
		short, cancel := context.WithTimeout(t.Context(), time.Minute)
		defer cancel()
		require.ErrorIs(t, c.Close(short), context.DeadlineExceeded,
			"a live Close returned while the async publish error handler runs")
		h.releaseHold()
		<-done
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

// 20. M-2, R3-2: a client-owned subscription left on a connection replaced through SetConnection
// is ended and joined by Close, which leaves the replaced connection open (it belongs to the
// caller of SetConnection) and does not wait for that connection's event handlers. A replaced
// connection that is already closed makes Unsubscribe fail; Close still waits for the handler.
func TestClientCloseEndsSubscriptionOnReplacedConnection(t *testing.T) {
	for _, alreadyClosed := range []bool{false, true} {
		name := "replaced-connection-open"
		if alreadyClosed {
			name = "replaced-connection-closed"
		}
		t.Run(name, func(t *testing.T) {
			url := embeddedServerURL(t)
			c, err := NewClient(url, WithHealthInterval(0))
			require.NoError(t, err)
			require.NoError(t, c.Connect(t.Context()))
			closeInCleanup(t, c)
			first := c.GetConnection()
			t.Cleanup(first.Close)
			held := newHeldCall(t)
			_, err = c.Subscribe(t.Context(), "replaced.held", func(context.Context, *nats.Msg) { held.hold() })
			require.NoError(t, err)
			require.NoError(t, first.FlushTimeout(lifecycleBound))
			require.NoError(t, first.Publish("replaced.held", nil))
			await(t, held.entered, "the handler entering")

			second, err := nats.Connect(url)
			require.NoError(t, err)
			t.Cleanup(second.Close)
			c.SetConnection(second)
			if alreadyClosed {
				first.Close()
			}

			require.ErrorIs(t, c.Close(endedContext(t)), context.Canceled)
			requireCloseStillWaiting(t, c, "the handler on the replaced connection returned")
			held.Release()
			requireLiveCloseNil(t, c, "Close after the handler returned")
			require.Zero(t, first.NumSubscriptions(), "the subscription on the replaced connection is still open")
			require.Equal(t, !alreadyClosed, !first.IsClosed(), "Close closed the connection the SetConnection caller owns")
		})
	}
}
