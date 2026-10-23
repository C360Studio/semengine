package natsclient

import (
	"context"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

// The tests in this file are design natsclient-handler-root T1-T3 (#311; owner ruling, #9
// comment 5994720412 item 2): SubscribeForRequests' context bounds its setup only, and each
// handler's context descends from a context the client owns, which ends when Close begins.
// They use a real connection to an embedded server, so they do not run inside synctest (#156).

// connectedRequestClient returns a client connected to an embedded server, closed on cleanup.
func connectedRequestClient(t *testing.T) *Client {
	t.Helper()
	c, err := NewClient(embeddedServerURL(t), WithHealthInterval(0))
	require.NoError(t, err)
	require.NoError(t, c.Connect(t.Context()))
	closeInCleanup(t, c)
	return c
}

// T1: a request that arrives after the setup context was cancelled gets a handler context
// that has not ended.
func TestRequestHandlerContextOutlivesSetupContext(t *testing.T) {
	c := connectedRequestClient(t)
	setupCtx, cancelSetup := context.WithCancel(t.Context())
	seen := make(chan error, 1)
	const subject = "handler.root.outlives"
	_, err := c.SubscribeForRequests(setupCtx, subject, func(ctx context.Context, _ []byte) ([]byte, error) {
		seen <- ctx.Err()
		return []byte("ok"), nil
	})
	require.NoError(t, err)
	cancelSetup()

	_, err = c.GetConnection().Request(subject, nil, lifecycleBound)
	require.NoError(t, err, "request after the setup context ended")
	require.NoError(t, awaitValue(t, seen, "the handler running"),
		"the handler's context ended with the setup context")
}

// heldRequestHandler blocks in a SubscribeForRequests handler: entered closes when it starts,
// ended when its context ends, and it returns only after the test releases it.
type heldRequestHandler struct {
	entered, ended chan struct{}
	held           *heldCall
}

func newHeldRequestHandler(t *testing.T) *heldRequestHandler {
	return &heldRequestHandler{entered: make(chan struct{}), ended: make(chan struct{}), held: newHeldCall(t)}
}

func (h *heldRequestHandler) handle(ctx context.Context, _ []byte) ([]byte, error) {
	close(h.entered)
	select {
	case <-ctx.Done():
		close(h.ended)
	case <-h.held.release:
	}
	h.held.hold()
	return nil, nil
}

// requireContextEndsOnClose begins Close while h runs and requires that h's context ends, that
// Close has not returned while h is held, and that Close then returns nil.
func requireContextEndsOnClose(t *testing.T, c *Client, h *heldRequestHandler) {
	t.Helper()
	closed := closeAsync(t.Context(), c)
	await(t, h.ended, "the handler's context ending once Close began")
	// h is held, so a correct Close cannot have returned yet; this check cannot fail on one.
	select {
	case err := <-closed:
		t.Fatalf("Close returned %v while the handler was still running", err)
	default:
	}
	h.held.Release()
	await(t, h.held.returned, "the handler returning")
	require.NoError(t, awaitErr(t, closed, "Close"), "Close joins the handler and returns nil")
}

// T2: Close beginning while a handler runs ends the handler's context, and Close returns nil
// only after the handler has returned.
func TestRequestHandlerContextEndsWhenCloseBegins(t *testing.T) {
	c := connectedRequestClient(t)
	h := newHeldRequestHandler(t)
	const subject = "handler.root.close"
	_, err := c.SubscribeForRequests(t.Context(), subject, h.handle)
	require.NoError(t, err)
	require.NoError(t, c.GetConnection().PublishMsg(&nats.Msg{Subject: subject}))
	await(t, h.entered, "the handler entering")

	requireContextEndsOnClose(t, c, h)
}

// T3: as T2, after Unsubscribe has ended the subscription's delivery while the handler still
// runs. Unsubscribe does not wait for the running handler.
func TestRequestHandlerContextEndsWhenCloseBeginsAfterUnsubscribe(t *testing.T) {
	c := connectedRequestClient(t)
	h := newHeldRequestHandler(t)
	const subject = "handler.root.unsubscribed"
	sub, err := c.SubscribeForRequests(t.Context(), subject, h.handle)
	require.NoError(t, err)
	require.NoError(t, c.GetConnection().PublishMsg(&nats.Msg{Subject: subject}))
	await(t, h.entered, "the handler entering")
	require.NoError(t, sub.Unsubscribe())

	requireContextEndsOnClose(t, c, h)
}
