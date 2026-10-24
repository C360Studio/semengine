package natsclient

import (
	"context"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

// The tests in this file are design natsclient-subscribe-handler-root T1-T4 (#378; owner
// ruling, #9 comment 5994720412 item 2): Subscribe's context bounds its setup only, and each
// handler's context descends from a context the client owns, which ends when Close begins.
// They reuse request_handler_root_test.go's helpers and a real connection to an embedded
// server, so they do not run inside synctest (#156).

// T1: a message that arrives after the setup context was cancelled gets a handler context
// that has not ended.
func TestSubscribeHandlerContextOutlivesSetupContext(t *testing.T) {
	c := connectedRequestClient(t)
	setupCtx, cancelSetup := context.WithCancel(t.Context())
	seen := make(chan error, 1)
	const subject = "subscribe.root.outlives"
	_, err := c.Subscribe(setupCtx, subject, func(ctx context.Context, _ *nats.Msg) {
		seen <- ctx.Err()
	})
	require.NoError(t, err)
	cancelSetup()

	require.NoError(t, c.GetConnection().Publish(subject, nil), "publish after the setup context ended")
	require.NoError(t, awaitValue(t, seen, "the handler running"),
		"the handler's context ended with the setup context")
}

// subscribeHeld adapts h to Subscribe's handler shape.
func subscribeHeld(h *heldRequestHandler) func(context.Context, *nats.Msg) {
	return func(ctx context.Context, msg *nats.Msg) {
		_, _ = h.handle(ctx, msg.Data)
	}
}

// T2: Close beginning while a handler runs ends the handler's context, and Close returns nil
// only after the handler has returned.
func TestSubscribeHandlerContextEndsWhenCloseBegins(t *testing.T) {
	c := connectedRequestClient(t)
	h := newHeldRequestHandler(t)
	const subject = "subscribe.root.close"
	_, err := c.Subscribe(t.Context(), subject, subscribeHeld(h))
	require.NoError(t, err)
	require.NoError(t, c.GetConnection().Publish(subject, nil))
	await(t, h.entered, "the handler entering")

	requireContextEndsOnClose(t, c, h)
}

// T3: as T2, after Unsubscribe has ended the subscription's delivery while the handler still
// runs. Unsubscribe does not wait for the running handler.
func TestSubscribeHandlerContextEndsWhenCloseBeginsAfterUnsubscribe(t *testing.T) {
	c := connectedRequestClient(t)
	h := newHeldRequestHandler(t)
	const subject = "subscribe.root.unsubscribed"
	sub, err := c.Subscribe(t.Context(), subject, subscribeHeld(h))
	require.NoError(t, err)
	require.NoError(t, c.GetConnection().Publish(subject, nil))
	await(t, h.entered, "the handler entering")
	require.NoError(t, sub.Unsubscribe())

	requireContextEndsOnClose(t, c, h)
}

// handlerTrace is the trace a Subscribe handler's context carried for one message.
type handlerTrace struct {
	data string
	tc   *TraceContext
	ok   bool
}

// T4: a handler's context carries no value of the setup context. A setup context carrying a
// trace gives a header-free message's handler no trace, and a message with trace headers
// gives its handler the headers' trace. Both messages go on the raw connection: Client.Publish
// adds a new trace when its context has none (design, T4 note).
func TestSubscribeHandlerContextCarriesOnlyTheMessageTrace(t *testing.T) {
	c := connectedRequestClient(t)
	setupTrace := NewTraceContext()
	setupCtx := ContextWithTrace(t.Context(), setupTrace)
	seen := make(chan handlerTrace, 2)
	const subject = "subscribe.root.trace"
	_, err := c.Subscribe(setupCtx, subject, func(ctx context.Context, msg *nats.Msg) {
		tc, ok := TraceContextFromContext(ctx)
		seen <- handlerTrace{data: string(msg.Data), tc: tc, ok: ok}
	})
	require.NoError(t, err)

	conn := c.GetConnection()
	require.NoError(t, conn.Publish(subject, []byte("bare")))
	first := awaitValue(t, seen, "the header-free message's handler")
	require.Equal(t, "bare", first.data)
	require.False(t, first.ok, "the header-free message's handler carries trace %+v; setup trace %+v",
		first.tc, setupTrace)

	msgTrace := NewTraceContext()
	traced := &nats.Msg{Subject: subject, Data: []byte("traced")}
	InjectTrace(ContextWithTrace(t.Context(), msgTrace), traced)
	require.NoError(t, conn.PublishMsg(traced))
	second := awaitValue(t, seen, "the traced message's handler")
	require.Equal(t, "traced", second.data)
	require.True(t, second.ok, "the traced message's handler carries no trace")
	require.Equal(t, msgTrace.TraceID, second.tc.TraceID, "the trace comes from the message's headers")
	require.Equal(t, msgTrace.SpanID, second.tc.SpanID, "the span comes from the message's headers")
}
