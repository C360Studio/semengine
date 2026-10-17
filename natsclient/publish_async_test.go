package natsclient

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPublishToStreamAsync_NotConnected verifies the async enqueue honors the
// same connection guard as the synchronous path: a disconnected client returns
// ErrNotConnected and a nil future, without touching JetStream.
func TestPublishToStreamAsync_NotConnected(t *testing.T) {
	client, err := NewClient("nats://unused")
	require.NoError(t, err)

	future, err := client.publishToStreamAsync(context.Background(), "test.subject", []byte("data"))
	assert.Equal(t, ErrNotConnected, err)
	assert.Nil(t, future)
}

// TestPublishToStreamAsync_CancelledContext verifies a cancelled context is
// honored before enqueue (PublishMsgAsync takes no ctx), returning the context
// error and a nil future.
func TestPublishToStreamAsync_CancelledContext(t *testing.T) {
	client, err := NewClient("nats://unused")
	require.NoError(t, err)
	// Force Connected so the ctx check (which sits after the connection gate) is
	// the one that fires, not ErrNotConnected.
	client.setStatus(StatusConnected)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	future, err := client.publishToStreamAsync(ctx, "test.subject", []byte("data"))
	assert.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, future)
}

// TestAsyncPublishErrHandler_NilMsg verifies the handler's defensive nil-guard on
// the message argument. jetstream-go currently always passes a non-nil paf.msg,
// but the handler must not panic if that ever changes.
func TestAsyncPublishErrHandler_NilMsg(t *testing.T) {
	client, err := NewClient("nats://unused")
	require.NoError(t, err)

	assert.NotPanics(t, func() {
		client.asyncPublishErrHandler(nil, nil, errors.New("ack failed"))
	})
}

// TestPublishBatchToStream_Empty verifies an empty batch is a no-op returning nil
// without touching the connection.
func TestPublishBatchToStream_Empty(t *testing.T) {
	client, err := NewClient("nats://unused")
	require.NoError(t, err)

	assert.NoError(t, client.PublishBatchToStream(context.Background(), "test.subject", nil))
}

// TestPublishBatchToStream_NotConnected verifies a non-empty batch on a
// disconnected client surfaces the enqueue guard error.
func TestPublishBatchToStream_NotConnected(t *testing.T) {
	client, err := NewClient("nats://unused")
	require.NoError(t, err)

	err = client.PublishBatchToStream(context.Background(), "test.subject", [][]byte{[]byte("a")})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotConnected)
}
