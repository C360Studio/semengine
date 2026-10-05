package natsclient

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// capturingNativeConsumer returns its handle from Consume at once and keeps the native handler,
// so a test delivers messages when it chooses.
type capturingNativeConsumer struct {
	jetstream.Consumer
	info      *jetstream.ConsumerInfo
	handle    *controlledNativeConsumeContext
	mu        sync.Mutex
	handler   jetstream.MessageHandler
	closeOnce sync.Once
}

// end closes the handle's Closed channel, as nats.go does once delivery has stopped.
func (c *capturingNativeConsumer) end() { c.closeOnce.Do(func() { close(c.handle.closed) }) }

func (c *capturingNativeConsumer) Info(context.Context) (*jetstream.ConsumerInfo, error) {
	return c.info, nil
}

func (c *capturingNativeConsumer) Consume(handler jetstream.MessageHandler, _ ...jetstream.PullConsumeOpt) (jetstream.ConsumeContext, error) {
	c.mu.Lock()
	c.handler = handler
	c.mu.Unlock()
	return c.handle, nil
}

// deliver runs one message through the native handler on its own goroutine, as nats.go does.
func (c *capturingNativeConsumer) deliver() {
	c.mu.Lock()
	h := c.handler
	c.mu.Unlock()
	go h(&headersOnlyMessage{})
}

func newCapturingClient(t *testing.T) (*Client, *capturingNativeConsumer) {
	t.Helper()
	c, err := NewClient("nats://unused")
	require.NoError(t, err)
	native := &capturingNativeConsumer{
		info: &jetstream.ConsumerInfo{Stream: "S_CTX", Name: "ctx",
			Config: jetstream.ConsumerConfig{Durable: "ctx", MaxAckPending: 17}},
		handle: &controlledNativeConsumeContext{closed: make(chan struct{})},
	}
	c.mu.Lock()
	c.js = &streamOnlyJetStream{fakeJetStream: &fakeJetStream{}, stream: &consumerOnlyStream{consumer: native}}
	c.mu.Unlock()
	c.setStatus(StatusConnected)
	// Cleanups run last-registered first: delivery ends, then a bounded Close joins the rest, so a
	// failed assertion leaves nothing blocked in the bubble.
	closeInCleanup(t, c)
	t.Cleanup(native.end)
	return c, native
}

var consumeContextCases = []struct {
	name string
	port bool
}{{"internal", false}, {"port", true}}

// TestConsumeSetupDeadlineDoesNotEndLaterHandlers (owner ruling, #9 comment 5994720412 item 2):
// the context passed to ConsumeStreamWithConfig and ConsumeInternalStreamWithConfig bounds setup
// only. A handler invoked after that context's deadline receives a live context. At the pin the
// setup context parented every handler, so each handler after the deadline got an ended context
// while delivery continued.
func TestConsumeSetupDeadlineDoesNotEndLaterHandlers(t *testing.T) {
	for _, tc := range consumeContextCases {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				c, native := newCapturingClient(t)
				setupCtx, cancel := context.WithTimeout(t.Context(), time.Second)
				defer cancel()
				handlerErr := make(chan error, 1)
				cfg := StreamConsumerConfig{StreamName: "S_CTX", ConsumerName: "ctx", MaxAckPending: 17}
				_, err := consumeVia(setupCtx, c, tc.port, cfg, func(ctx context.Context, _ jetstream.Msg) {
					handlerErr <- ctx.Err()
				})
				require.NoError(t, err)

				<-time.After(2 * time.Second) // the bubble's clock: past the setup deadline
				require.Error(t, setupCtx.Err(), "the setup context must have ended")
				native.deliver()
				select {
				case err := <-handlerErr:
					assert.NoError(t, err, "a handler after the setup deadline must get a live context")
				case <-time.After(lifecycleBound):
					t.Fatal("the handler did not run")
				}
				native.end()
				requireLiveCloseNil(t, c, "Close after the handler returned")
			})
		})
	}
}

// TestConsumeHandlerContextEndsWhenCloseBegins (owner ruling, #9 comment 5994720412 item 2): the
// handlers' context is owned by the client and is cancelled when Close begins, and Close still
// joins the handler before it returns nil.
func TestConsumeHandlerContextEndsWhenCloseBegins(t *testing.T) {
	for _, tc := range consumeContextCases {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				c, native := newCapturingClient(t)
				entered := make(chan struct{})
				ended := make(chan struct{})
				returned := make(chan struct{})
				cfg := StreamConsumerConfig{StreamName: "S_CTX", ConsumerName: "ctx", MaxAckPending: 17}
				_, err := consumeVia(t.Context(), c, tc.port, cfg, func(ctx context.Context, _ jetstream.Msg) {
					defer close(returned)
					close(entered)
					select {
					case <-ctx.Done():
						close(ended)
					case <-time.After(lifecycleBound):
					}
				})
				require.NoError(t, err)
				native.deliver()
				await(t, entered, "the handler starting")

				closed := closeAsync(t.Context(), c)
				await(t, ended, "the handler's context ending once Close began")
				await(t, returned, "the handler returning")
				native.end()
				require.NoError(t, awaitErr(t, closed, "Close"), "Close joins the handler and returns nil")
			})
		})
	}
}
