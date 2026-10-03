//go:build integration

package natsclient

import (
	"context"
	"testing"
	"time"

	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_PublishToStreamWithMsgID_Dedup verifies the ADR-055 §5 "T1"
// contract: a deterministic Nats-Msg-Id collapses re-publishes of the same
// logical event to a single stored message within the stream's duplicate
// window, distinct IDs each store, and an empty ID opts out of dedup entirely
// (drop-in PublishToStream behavior).
func TestIntegration_PublishToStreamWithMsgID_Dedup(t *testing.T) {
	ctx := context.Background()
	// The window must outlast two back-to-back publishes on a slow host, or a correct
	// server would store the duplicate (design D8 R1c); the pin's 250 ms is widened.
	const duplicateWindow = 2 * time.Second

	natsURL := startFixture(t).URL()

	client, err := NewClient(natsURL)
	require.NoError(t, err)
	require.NoError(t, client.Connect(ctx))
	defer closeClient(t, client)

	// Stream with an explicit duplicate-detection window.
	stream, err := client.EnsureStream(ctx, jetstream.StreamConfig{
		Name:       "MSGID_STREAM",
		Subjects:   []string{"msgid.>"},
		Storage:    jetstream.MemoryStorage,
		Duplicates: duplicateWindow,
		MaxAge:     testStreamMaxAge,
		MaxBytes:   testStreamMaxBytes,
	})
	require.NoError(t, err)

	msgCount := func() uint64 {
		info, ierr := stream.Info(ctx)
		require.NoError(t, ierr)
		return info.State.Msgs
	}

	// Same msgID twice → deduped to one stored message. PublishMsg returns a
	// PubAck with Duplicate=true (no error) on the second call.
	require.NoError(t, client.PublishToStreamWithMsgID(ctx, "msgid.test", []byte("first"), "evt-1"))
	require.NoError(t, client.PublishToStreamWithMsgID(ctx, "msgid.test", []byte("first-again"), "evt-1"))
	assert.Equal(t, uint64(1), msgCount(),
		"same Nats-Msg-Id within the window must dedup to one stored message")

	// Expiry itself is the wall-clock behavior under test. Poll the server's
	// authoritative stream state instead of sleeping for a guessed scheduler
	// margin: duplicate attempts remain collapsed until the configured window
	// actually expires, then the first accepted attempt advances the count.
	expiryCtx, cancelExpiry := context.WithTimeout(ctx, duplicateWindow+failureBound)
	defer cancelExpiry()
	stored, err := probe.Await(expiryCtx, func(ctx context.Context) (uint64, error) {
		if err := client.PublishToStreamWithMsgID(ctx, "msgid.test", []byte("after-window"), "evt-1"); err != nil {
			return 0, err
		}
		info, err := stream.Info(ctx)
		if err != nil {
			return 0, err
		}
		return info.State.Msgs, nil
	}, func(msgs uint64) bool { return msgs == 2 })
	require.NoError(t, err, "the same Nats-Msg-Id must store again after the configured window (stored %d)", stored)

	// Distinct msgID → a new message.
	require.NoError(t, client.PublishToStreamWithMsgID(ctx, "msgid.test", []byte("second"), "evt-2"))
	assert.Equal(t, uint64(3), msgCount(), "distinct Nats-Msg-Id must store a new message")

	// Empty msgID → no dedup; two identical publishes both store.
	require.NoError(t, client.PublishToStreamWithMsgID(ctx, "msgid.test", []byte("x"), ""))
	require.NoError(t, client.PublishToStreamWithMsgID(ctx, "msgid.test", []byte("x"), ""))
	assert.Equal(t, uint64(5), msgCount(), "empty msgID must not dedup")
}

// TestIntegration_PublishToStreamWithMsgID_NotConnected verifies the circuit
// guard mirrors PublishToStream / PublishToStreamWithAck.
func TestIntegration_PublishToStreamWithMsgID_NotConnected(t *testing.T) {
	client, err := NewClient("nats://unused")
	require.NoError(t, err)

	ctx := context.Background()
	err = client.PublishToStreamWithMsgID(ctx, "test.subject", []byte("data"), "id-1")
	assert.Equal(t, ErrNotConnected, err)
}
