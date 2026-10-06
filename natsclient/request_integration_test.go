//go:build integration

package natsclient

import (
	"context"
	"testing"
	"time"

	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_Request tests the basic request/reply pattern
func TestIntegration_Request(t *testing.T) {
	ctx := context.Background()

	// Start NATS container
	natsURL := startFixture(t).URL()

	// Create and connect client
	client, err := NewClient(natsURL)
	require.NoError(t, err)
	err = client.Connect(ctx)
	require.NoError(t, err)
	defer closeClient(t, client)

	// Set up a responder
	subject := "test.request"
	expectedResponse := []byte("pong")

	_, err = client.SubscribeForRequests(ctx, subject, func(_ context.Context, data []byte) ([]byte, error) {
		assert.Equal(t, []byte("ping"), data)
		return expectedResponse, nil
	})
	require.NoError(t, err)

	// The subscription is registered once the broker has answered a flush on
	// the same connection.
	flushClient(t, client)

	// Send request
	response, err := client.Request(ctx, subject, []byte("ping"), failureBound)
	require.NoError(t, err)
	assert.Equal(t, expectedResponse, response)
}

// TestIntegration_Request_Timeout tests request timeout behavior
func TestIntegration_Request_Timeout(t *testing.T) {
	ctx := context.Background()

	// Start NATS container
	natsURL := startFixture(t).URL()

	// Create and connect client
	client, err := NewClient(natsURL)
	require.NoError(t, err)
	err = client.Connect(ctx)
	require.NoError(t, err)
	defer closeClient(t, client)

	// Send request to non-existent subject (no responder)
	// NATS may return "no responders" immediately or timeout depending on server config
	_, err = client.Request(ctx, "nonexistent.subject", []byte("test"), 100*time.Millisecond)

	// Should return an error (either timeout or "no responders")
	assert.Error(t, err)
	// The error should be either a timeout or "no responders" error
	errStr := err.Error()
	assert.True(t, errStr == "nats: no responders available for request" ||
		errStr == "context deadline exceeded" ||
		errStr == "nats: timeout",
		"Expected timeout or no responders error, got: %s", errStr)
}

// TestIntegration_Request_DefaultTimeout tests default timeout is applied
func TestIntegration_Request_DefaultTimeout(t *testing.T) {
	ctx := context.Background()

	// Start NATS container
	natsURL := startFixture(t).URL()

	// Create and connect client
	client, err := NewClient(natsURL)
	require.NoError(t, err)
	err = client.Connect(ctx)
	require.NoError(t, err)
	defer closeClient(t, client)

	// Request with 0 timeout should use default (5s)
	// We won't wait for full timeout, just verify it doesn't fail immediately
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	_, err = client.Request(ctx, "nonexistent.subject", []byte("test"), 0)
	assert.Error(t, err) // Will error due to context cancellation
}

// TestIntegration_RequestWithHeaders tests request with custom headers
func TestIntegration_RequestWithHeaders(t *testing.T) {
	ctx := context.Background()

	// Start NATS container
	natsURL := startFixture(t).URL()

	// Create and connect client
	client, err := NewClient(natsURL)
	require.NoError(t, err)
	err = client.Connect(ctx)
	require.NoError(t, err)
	defer closeClient(t, client)

	// Set up responder using SubscribeForRequests
	// Note: Headers are preserved in the underlying message
	subject := "test.headers"
	_, err = client.SubscribeForRequests(ctx, subject, func(_ context.Context, _ []byte) ([]byte, error) {
		// Return a fixed response
		return []byte("headers-response"), nil
	})
	require.NoError(t, err)

	flushClient(t, client)

	// Send request with headers
	headers := map[string]string{
		"X-Custom": "test-value",
	}
	response, err := client.RequestWithHeaders(ctx, subject, []byte("data"), headers, failureBound)
	require.NoError(t, err)
	assert.Equal(t, []byte("headers-response"), response.Data)
}

// TestIntegration_Request_NotConnected tests behavior when not connected
func TestIntegration_Request_NotConnected(t *testing.T) {
	// Create client without connecting
	client, err := NewClient("nats://unused")
	require.NoError(t, err)

	ctx := context.Background()
	_, err = client.Request(ctx, "test.subject", []byte("data"), time.Second)
	assert.Equal(t, ErrNotConnected, err)
}

// TestIntegration_RequestWithHeaders_NotConnected tests headers request when not connected
func TestIntegration_RequestWithHeaders_NotConnected(t *testing.T) {
	// Create client without connecting
	client, err := NewClient("nats://unused")
	require.NoError(t, err)

	ctx := context.Background()
	_, err = client.RequestWithHeaders(ctx, "test.subject", []byte("data"), nil, time.Second)
	assert.Equal(t, ErrNotConnected, err)
}

// TestIntegration_Reply tests the Reply function
func TestIntegration_Reply(t *testing.T) {
	ctx := context.Background()

	// Start NATS container
	natsURL := startFixture(t).URL()

	// Create and connect client
	client, err := NewClient(natsURL)
	require.NoError(t, err)
	err = client.Connect(ctx)
	require.NoError(t, err)
	defer closeClient(t, client)

	// Reply to empty subject should be no-op
	err = client.Reply(ctx, "", []byte("data"))
	assert.NoError(t, err)

	// Reply to valid subject
	err = client.Reply(ctx, "reply.subject", []byte("data"))
	assert.NoError(t, err)
}

// TestIntegration_ReplyWithHeaders tests ReplyWithHeaders function
func TestIntegration_ReplyWithHeaders(t *testing.T) {
	ctx := context.Background()

	// Start NATS container
	natsURL := startFixture(t).URL()

	// Create and connect client
	client, err := NewClient(natsURL)
	require.NoError(t, err)
	err = client.Connect(ctx)
	require.NoError(t, err)
	defer closeClient(t, client)

	// Reply with headers to empty subject should be no-op
	err = client.ReplyWithHeaders(ctx, "", []byte("data"), map[string]string{"X-Test": "value"})
	assert.NoError(t, err)

	// Reply with headers to valid subject
	err = client.ReplyWithHeaders(ctx, "reply.subject", []byte("data"), map[string]string{"X-Test": "value"})
	assert.NoError(t, err)
}

// TestIntegration_SubscribeForRequests tests the request handler subscription
func TestIntegration_SubscribeForRequests(t *testing.T) {
	ctx := context.Background()

	// Start NATS container
	natsURL := startFixture(t).URL()

	// Create and connect client
	client, err := NewClient(natsURL)
	require.NoError(t, err)
	err = client.Connect(ctx)
	require.NoError(t, err)
	defer closeClient(t, client)

	// Subscribe to requests
	subject := "test.service"
	handlerCalled := make(chan bool, 1)

	_, err = client.SubscribeForRequests(ctx, subject, func(_ context.Context, _ []byte) ([]byte, error) {
		handlerCalled <- true
		return []byte("response"), nil
	})
	require.NoError(t, err)

	flushClient(t, client)

	// Send a request
	response, err := client.Request(ctx, subject, []byte("request"), failureBound)
	require.NoError(t, err)
	assert.Equal(t, []byte("response"), response)

	// Verify handler was called
	select {
	case <-handlerCalled:
		// Success
	case <-time.After(failureBound):
		t.Fatal("Handler was not called")
	}
}

// TestIntegration_SubscribeForRequests_HandlerTimeoutConfigurable proves, through a
// real broker, that the per-message handler context carries the CONFIGURED deadline,
// not the 30s default. The exact value is proven at the derivation seam by
// TestRequestHandlerDeadlineIsTheConfiguredTimeout; here the handler hands its
// deadline back over a channel and the test brackets it by events: the handler
// started after the request was sent and before the reply arrived, so its deadline
// lies between those two instants plus the configured timeout. No bound depends on
// how fast the host runs. WithRequestHandlerTimeout was dropped as dead surface
// (task 3.7a); the timeout is set through the environment variable it overrode.
func TestIntegration_SubscribeForRequests_HandlerTimeoutConfigurable(t *testing.T) {
	ctx := context.Background()

	natsURL := startFixture(t).URL()

	const configured = 5 * time.Second
	t.Setenv(requestHandlerTimeoutEnv, configured.String())
	client, err := NewClient(natsURL)
	require.NoError(t, err)
	err = client.Connect(ctx)
	require.NoError(t, err)
	defer closeClient(t, client)

	type handlerDeadline struct {
		at time.Time
		ok bool
	}
	seen := make(chan handlerDeadline, 1)
	subject := "test.handler.timeout"
	_, err = client.SubscribeForRequests(ctx, subject, func(hctx context.Context, _ []byte) ([]byte, error) {
		at, ok := hctx.Deadline()
		select {
		case seen <- handlerDeadline{at: at, ok: ok}:
		default: // only the first request is examined
		}
		return []byte("ok"), nil
	})
	require.NoError(t, err)

	flushClient(t, client)

	sent := time.Now()
	resp, err := client.Request(ctx, subject, []byte("x"), failureBound)
	replied := time.Now()
	require.NoError(t, err)
	require.Equal(t, "ok", string(resp))

	got := <-seen
	require.True(t, got.ok, "the handler context carries a deadline")
	// Both instants and the deadline carry monotonic clock readings, so these
	// comparisons are unaffected by wall-clock changes.
	assert.False(t, got.at.Before(sent.Add(configured)),
		"the handler started after the request was sent; deadline %v is earlier than sent+%v", got.at, configured)
	assert.False(t, got.at.After(replied.Add(configured)),
		"the handler started before the reply arrived; deadline %v is later than replied+%v (the 30s default?)",
		got.at, configured)
}

// TestIntegration_SubscribeForRequests_Error tests error handling in request handler
func TestIntegration_SubscribeForRequests_Error(t *testing.T) {
	ctx := context.Background()

	// Start NATS container
	natsURL := startFixture(t).URL()

	// Create and connect client
	client, err := NewClient(natsURL)
	require.NoError(t, err)
	err = client.Connect(ctx)
	require.NoError(t, err)
	defer closeClient(t, client)

	// Subscribe with handler that returns error
	subject := "test.error"
	_, err = client.SubscribeForRequests(ctx, subject, func(_ context.Context, _ []byte) ([]byte, error) {
		return nil, assert.AnError
	})
	require.NoError(t, err)

	flushClient(t, client)

	// ADR-060: a handler error is the {message} envelope + X-Status header;
	// RequestClassified surfaces it as a classified error carrying the message.
	_, err = client.RequestClassified(ctx, subject, []byte("request"), failureBound)
	require.Error(t, err)
	assert.Contains(t, err.Error(), assert.AnError.Error())
}

// TestIntegration_SubscribeForRequests_NotConnected tests subscription when not connected
func TestIntegration_SubscribeForRequests_NotConnected(t *testing.T) {
	// Create client without connecting
	client, err := NewClient("nats://unused")
	require.NoError(t, err)

	ctx := context.Background()
	_, err = client.SubscribeForRequests(ctx, "test.subject", func(_ context.Context, _ []byte) ([]byte, error) {
		return nil, nil
	})
	assert.Equal(t, ErrNotConnected, err)
}

// TestIntegration_RequestWithRetry_Success tests retry succeeds when responder starts late
func TestIntegration_RequestWithRetry_Success(t *testing.T) {
	ctx := context.Background()

	// Start NATS container
	natsURL := startFixture(t).URL()

	// Create and connect client
	client, err := NewClient(natsURL)
	require.NoError(t, err)
	err = client.Connect(ctx)
	require.NoError(t, err)
	defer closeClient(t, client)

	subject := "test.retry"
	expectedResponse := []byte("delayed-response")

	// The responder subscribes once the first attempt has failed for want of one
	// (the client counts the failure), so only a retry can succeed: the "not ready
	// yet" scenario. The pin waited 150 ms instead.
	subscribed := make(chan error, 1)
	go func() {
		waitCtx, cancel := context.WithTimeout(ctx, failureBound)
		defer cancel()
		if _, err := probe.Await(waitCtx, func(context.Context) (int32, error) { return client.Failures(), nil },
			func(failures int32) bool { return failures >= 1 }); err != nil {
			subscribed <- err
			return
		}
		_, err := client.SubscribeForRequests(ctx, subject, func(_ context.Context, data []byte) ([]byte, error) {
			return expectedResponse, nil
		})
		subscribed <- err
	}()

	// Ten retries span about 5.6 s of backoff, so the late subscription lands inside
	// the retry window on a slow host; a correct run ends at the first retry after it.
	config := DefaultRetryConfig()
	config.InitialBackoff = 50 * time.Millisecond
	config.MaxRetries = 10
	config.BackoffMultiplier = 1.5

	// Send request that should retry until responder is ready
	response, err := client.RequestWithRetry(
		ctx,
		subject,
		[]byte("request"),
		2*time.Second,
		config,
	)

	require.NoError(t, <-subscribed, "late responder")
	require.NoError(t, err, "retry should succeed when responder becomes available")
	assert.Equal(t, expectedResponse, response)
}

// TestIntegration_RequestWithRetry_NoResponder tests all retries exhausted when no responder exists
func TestIntegration_RequestWithRetry_NoResponder(t *testing.T) {
	ctx := context.Background()

	// Start NATS container
	natsURL := startFixture(t).URL()

	// Create and connect client
	client, err := NewClient(natsURL)
	require.NoError(t, err)
	err = client.Connect(ctx)
	require.NoError(t, err)
	defer closeClient(t, client)

	// Configure retry with short backoff for faster test
	config := DefaultRetryConfig()
	config.InitialBackoff = 20 * time.Millisecond
	config.MaxRetries = 3
	config.MaxBackoff = 100 * time.Millisecond

	// Send request to subject with no responder
	_, err = client.RequestWithRetry(
		ctx,
		"nonexistent.subject",
		[]byte("request"),
		100*time.Millisecond,
		config,
	)

	// Should fail with "no responders" or timeout error
	require.Error(t, err)
	errStr := err.Error()
	assert.True(t,
		errStr == "nats: no responders available for request" ||
			errStr == "context deadline exceeded" ||
			errStr == "nats: timeout",
		"Expected no responders or timeout error, got: %s", errStr)

	// Note: NATS returns "no responders" immediately (no wait), so with retries
	// and backoff the timing is: retry backoffs (20ms + 40ms + 60ms) ~120ms
	// We don't assert on timing as it varies with server configuration
}
