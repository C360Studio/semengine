//go:build integration

package natsclient

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/pkg/errs"
)

// The tests of Client.WaitConsumerHandlers (design D11 of
// setup-04a-04-graph-roots-objectstore-component; transport-client "A caller can wait for one
// consumer's handlers").

// heldWait bounds a wait the test expects to block because a handler is held: the call can only
// end by this context. Its length decides nothing but how long the test runs.
const heldWait = 200 * time.Millisecond

// heldHandler is a consumer handler whose first invocation reports that it entered and then
// waits for release; later invocations report on later. Every invocation acknowledges its
// message.
type heldHandler struct {
	entered chan struct{}
	later   chan struct{}
	release chan struct{}
	once    sync.Once
	first   sync.Once
}

func newHeldHandler() *heldHandler {
	return &heldHandler{
		entered: make(chan struct{}),
		later:   make(chan struct{}, 8),
		release: make(chan struct{}),
	}
}

func (h *heldHandler) handle(_ context.Context, msg jetstream.Msg) {
	held := false
	h.first.Do(func() { held = true })
	if held {
		close(h.entered)
		<-h.release
	} else {
		h.later <- struct{}{}
	}
	_ = msg.Ack()
}

func (h *heldHandler) releaseHandler() { h.once.Do(func() { close(h.release) }) }

func (h *heldHandler) awaitEntered(t *testing.T) {
	t.Helper()
	select {
	case <-h.entered:
	case <-time.After(failureBound):
		t.Fatal("the handler never received the first message")
	}
}

// expectHeld calls WaitConsumerHandlers under a context of heldWait and requires the call to end
// by that context: a handler is held, so the wait must not return nil.
func expectHeld(t *testing.T, client *Client, handle jetstream.ConsumeContext) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), heldWait)
	defer cancel()
	err := client.WaitConsumerHandlers(ctx, handle)
	require.Error(t, err, "the wait returned nil while a handler is still running")
	require.ErrorIs(t, err, context.DeadlineExceeded, "the wait ended other than by its context")
}

// expectReturned calls WaitConsumerHandlers under failureBound and requires nil.
func expectReturned(t *testing.T, client *Client, handle jetstream.ConsumeContext) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), failureBound)
	defer cancel()
	require.NoError(t, client.WaitConsumerHandlers(ctx, handle))
}

func TestWaitConsumerHandlersWaitsForRunningHandler(t *testing.T) {
	tc := newFixtureClient(t)
	tc.stream(t, "W1_RUNNING", "w1.running.>")
	cfg := StreamConsumerConfig{
		StreamName: "W1_RUNNING", ConsumerName: "w1-running",
		FilterSubject: "w1.running.>", AckPolicy: "explicit", DeliverPolicy: "all",
	}
	owner := PortConsumerContext{Component: "w1-owner", Port: "input"}
	held := newHeldHandler()
	defer held.releaseHandler()
	handle, err := tc.Client.ConsumeStreamWithConfig(t.Context(), owner, cfg, held.handle)
	require.NoError(t, err)

	require.NoError(t, tc.Client.PublishToStream(t.Context(), "w1.running.work", []byte("work")))
	held.awaitEntered(t)
	handle.Stop()

	expectHeld(t, tc.Client, handle)

	held.releaseHandler()
	expectReturned(t, tc.Client, handle)

	// The durable claim is released by the time the wait returns nil: the same durable starts
	// again on the same client at once.
	again, err := tc.Client.ConsumeStreamWithConfig(t.Context(), owner, cfg, func(context.Context, jetstream.Msg) {})
	require.NoError(t, err, "a consumer with the same durable after the wait returned nil")
	again.Drain()
	expectReturned(t, tc.Client, again)
}

func TestWaitConsumerHandlersAfterConnectionLoss(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), restartBound)
	defer cancel()
	// Own server: the test takes the broker away under a running handler.
	f := natsfixture.New(t)
	require.NoError(t, f.Start(ctx))
	_, err := natsfixture.CreateStream(ctx, f.JetStream(), jetstream.FileStorage, "W2_LOSS", "w2.loss.>")
	require.NoError(t, err)
	// newFixtureClientAt's client does not reconnect, so losing the broker closes its connection
	// for good.
	tc := newFixtureClientAt(t, f.URL())
	cfg := StreamConsumerConfig{
		StreamName: "W2_LOSS", ConsumerName: "w2-loss",
		FilterSubject: "w2.loss.>", AckPolicy: "explicit", DeliverPolicy: "all",
	}
	held := newHeldHandler()
	defer held.releaseHandler()
	handle, err := tc.Client.ConsumeInternalStreamWithConfig(ctx, cfg, held.handle)
	require.NoError(t, err)

	require.NoError(t, tc.Client.PublishToStream(ctx, "w2.loss.work", []byte("work")))
	held.awaitEntered(t)

	closedConn := tc.Client.GetConnection().StatusChanged(nats.CLOSED)
	require.NoError(t, f.Restart(ctx))
	select {
	case <-closedConn:
	case <-ctx.Done():
		t.Fatal("the client's connection never closed after the broker went away")
	}
	// The trigger, and the probe at nats.go v1.54.0: the consume loop reports its end while the
	// handler is still held.
	select {
	case <-handle.Closed():
	case <-ctx.Done():
		t.Fatal("the consume loop's Closed() never fired; the scenario needs it to")
	}

	expectHeld(t, tc.Client, handle)

	held.releaseHandler()
	expectReturned(t, tc.Client, handle)
	// The claim is released by the time the wait returns nil (the client's connection is gone,
	// so a second consumer cannot start to show it).
	tc.Client.internalClaimsMu.Lock()
	_, claimed := tc.Client.internalClaims[internalConsumerIdentity{stream: cfg.StreamName, durable: cfg.ConsumerName}]
	tc.Client.internalClaimsMu.Unlock()
	require.False(t, claimed, "the durable claim is still held after the wait returned nil")
}

func TestWaitConsumerHandlersContextEndsFirst(t *testing.T) {
	tc := newFixtureClient(t)
	tc.stream(t, "W3_CTX", "w3.ctx.>")
	cfg := StreamConsumerConfig{
		StreamName: "W3_CTX", ConsumerName: "w3-ctx",
		FilterSubject: "w3.ctx.>", AckPolicy: "explicit", DeliverPolicy: "all",
	}
	owner := PortConsumerContext{Component: "w3-owner", Port: "input"}
	held := newHeldHandler()
	defer held.releaseHandler()
	handle, err := tc.Client.ConsumeStreamWithConfig(t.Context(), owner, cfg, held.handle)
	require.NoError(t, err)

	require.NoError(t, tc.Client.PublishToStream(t.Context(), "w3.ctx.first", []byte("first")))
	held.awaitEntered(t)

	waitCtx, waitCancel := context.WithTimeout(t.Context(), heldWait)
	err = tc.Client.WaitConsumerHandlers(waitCtx, handle)
	waitCancel()
	require.Error(t, err, "the wait returned nil while a handler is still running")
	require.ErrorIs(t, err, waitCtx.Err(), "the error does not match the context's")

	// The wait stopped nothing: once the held handler returns, a message published now is
	// delivered to the handler.
	held.releaseHandler()
	require.NoError(t, tc.Client.PublishToStream(t.Context(), "w3.ctx.second", []byte("second")))
	select {
	case <-held.later:
	case <-time.After(failureBound):
		t.Fatal("a message published after the wait ended was not delivered: the wait stopped the consumer")
	}

	handle.Stop()
	expectReturned(t, tc.Client, handle)
}

// foreignHandle is a ConsumeContext nats.go never made, and not comparable: a map key or an ==
// on it would panic.
type foreignHandle struct{ tags []string }

func (foreignHandle) Stop()                   {}
func (foreignHandle) Drain()                  {}
func (foreignHandle) Closed() <-chan struct{} { return nil }

func TestWaitConsumerHandlersRefusesUnknownHandle(t *testing.T) {
	tc := newFixtureClient(t)
	tc.stream(t, "W4_UNKNOWN", "w4.unknown.>")
	other := newFixtureClientAt(t, tc.URL)
	cfg := StreamConsumerConfig{
		StreamName: "W4_UNKNOWN", ConsumerName: "w4-unknown",
		FilterSubject: "w4.unknown.>", AckPolicy: "explicit", DeliverPolicy: "all",
	}
	foreign, err := other.Client.ConsumeStreamWithConfig(t.Context(),
		PortConsumerContext{Component: "w4-other", Port: "input"}, cfg, func(context.Context, jetstream.Msg) {})
	require.NoError(t, err)
	defer func() {
		foreign.Drain()
		expectReturned(t, other.Client, foreign)
	}()

	for name, handle := range map[string]jetstream.ConsumeContext{
		"nil":            nil,
		"another client": foreign,
		"not comparable": foreignHandle{tags: []string{"x"}},
	} {
		t.Run(name, func(t *testing.T) {
			// The foreign handle is live, so a call that waited on it would end only by this
			// context; the refusal must come first, as invalid input.
			ctx, cancel := context.WithTimeout(t.Context(), failureBound)
			defer cancel()
			var err error
			require.NotPanics(t, func() { err = tc.Client.WaitConsumerHandlers(ctx, handle) })
			require.Error(t, err, "an unknown handle was not refused")
			require.True(t, errs.IsInvalid(err), "want invalid input, got %v", err)
			require.False(t, errors.Is(err, context.DeadlineExceeded), "the call waited instead of refusing")
		})
	}
}

func TestWaitConsumerHandlersAfterClose(t *testing.T) {
	tc := newFixtureClient(t)
	tc.stream(t, "W5_CLOSE", "w5.close.>")
	cfg := StreamConsumerConfig{
		StreamName: "W5_CLOSE", ConsumerName: "w5-close",
		FilterSubject: "w5.close.>", AckPolicy: "explicit", DeliverPolicy: "all",
	}
	delivered := make(chan struct{}, 1)
	handle, err := tc.Client.ConsumeStreamWithConfigContexts(t.Context(), t.Context(),
		PortConsumerContext{Component: "w5-owner", Port: "input"}, cfg,
		func(_ context.Context, msg jetstream.Msg) {
			delivered <- struct{}{}
			_ = msg.Ack()
		})
	require.NoError(t, err)
	require.NoError(t, tc.Client.PublishToStream(t.Context(), "w5.close.work", []byte("work")))
	select {
	case <-delivered:
	case <-time.After(failureBound):
		t.Fatal("the handler never received the message")
	}

	// The handle is never stopped: Close stops it and joins its handlers.
	closeClient(t, tc.Client)
	expectReturned(t, tc.Client, handle)
}

func TestWaitConsumerHandlersRefusesNilContext(t *testing.T) {
	tc := newFixtureClient(t)
	tc.stream(t, "W6_NILCTX", "w6.nilctx.>")
	cfg := StreamConsumerConfig{
		StreamName: "W6_NILCTX", ConsumerName: "w6-nilctx",
		FilterSubject: "w6.nilctx.>", AckPolicy: "explicit", DeliverPolicy: "all",
	}
	// A live handle of this client: only the nil context can be refused.
	handle, err := tc.Client.ConsumeStreamWithConfig(t.Context(),
		PortConsumerContext{Component: "w6-owner", Port: "input"}, cfg, func(context.Context, jetstream.Msg) {})
	require.NoError(t, err)
	defer func() {
		handle.Drain()
		expectReturned(t, tc.Client, handle)
	}()

	var nilCtx context.Context
	err = tc.Client.WaitConsumerHandlers(nilCtx, handle)
	require.Error(t, err, "a nil context was not refused")
	require.True(t, errs.IsInvalid(err), "want invalid input, got %v", err)
}
