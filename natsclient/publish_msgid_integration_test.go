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
// (drop-in PublishToStream behavior). The window's expiry is a separate proof,
// TestIntegration_PublishToStreamWithMsgID_StoresAgainAfterWindow.
func TestIntegration_PublishToStreamWithMsgID_Dedup(t *testing.T) {
	// Why the window's expiry cannot fail this proof (design D8 R1c): every publish and
	// read below runs under proofCtx, which ends failureBound after it starts. The server
	// stores the first evt-1 after proofCtx starts and handles the second before that
	// publish returns, so the two are at most failureBound apart, and the window is six
	// times that. A host too slow to finish inside failureBound fails on the context with a
	// deadline error; it can never reach the count assertion with the window expired.
	const duplicateWindow = 6 * failureBound

	natsURL := startFixture(t).URL()

	client, err := NewClient(natsURL)
	require.NoError(t, err)
	require.NoError(t, client.Connect(t.Context()))
	defer closeClient(t, client)

	stream, err := client.EnsureStream(t.Context(), jetstream.StreamConfig{
		Name:       "MSGID_STREAM",
		Subjects:   []string{"msgid.>"},
		Storage:    jetstream.MemoryStorage,
		Duplicates: duplicateWindow,
		MaxAge:     testStreamMaxAge,
		MaxBytes:   testStreamMaxBytes,
	})
	require.NoError(t, err)

	proofCtx, cancelProof := context.WithTimeout(t.Context(), failureBound)
	defer cancelProof()
	msgCount := func() uint64 {
		info, ierr := stream.Info(proofCtx)
		require.NoError(t, ierr)
		return info.State.Msgs
	}

	// Same msgID twice → deduped to one stored message. PublishMsg returns a
	// PubAck with Duplicate=true (no error) on the second call.
	require.NoError(t, client.PublishToStreamWithMsgID(proofCtx, "msgid.test", []byte("first"), "evt-1"))
	require.NoError(t, client.PublishToStreamWithMsgID(proofCtx, "msgid.test", []byte("first-again"), "evt-1"))
	assert.Equal(t, uint64(1), msgCount(),
		"same Nats-Msg-Id within the window must dedup to one stored message")

	// Distinct msgID → a new message.
	require.NoError(t, client.PublishToStreamWithMsgID(proofCtx, "msgid.test", []byte("second"), "evt-2"))
	assert.Equal(t, uint64(2), msgCount(), "distinct Nats-Msg-Id must store a new message")

	// Empty msgID → no dedup; two identical publishes both store.
	require.NoError(t, client.PublishToStreamWithMsgID(proofCtx, "msgid.test", []byte("x"), ""))
	require.NoError(t, client.PublishToStreamWithMsgID(proofCtx, "msgid.test", []byte("x"), ""))
	assert.Equal(t, uint64(4), msgCount(), "empty msgID must not dedup")
}

// TestIntegration_PublishToStreamWithMsgID_StoresAgainAfterWindow is the expiry
// half of the T1 contract: once the stream's duplicate window has passed, the
// same Nats-Msg-Id stores again. Expiry is the wall-clock behaviour under test
// (design D8 R1c), so the window is the pin's 250 ms and nothing here asserts a
// duplicate inside it: a slow host can only make the first retry land after the
// window, which this check accepts. The retries are observed with probe.Await
// over the server's stream state, bounded by the window plus failureBound.
func TestIntegration_PublishToStreamWithMsgID_StoresAgainAfterWindow(t *testing.T) {
	const duplicateWindow = 250 * time.Millisecond

	natsURL := startFixture(t).URL()

	client, err := NewClient(natsURL)
	require.NoError(t, err)
	require.NoError(t, client.Connect(t.Context()))
	defer closeClient(t, client)

	stream, err := client.EnsureStream(t.Context(), jetstream.StreamConfig{
		Name:       "MSGID_EXPIRY_STREAM",
		Subjects:   []string{"msgidexpiry.>"},
		Storage:    jetstream.MemoryStorage,
		Duplicates: duplicateWindow,
		MaxAge:     testStreamMaxAge,
		MaxBytes:   testStreamMaxBytes,
	})
	require.NoError(t, err)

	expiryCtx, cancelExpiry := context.WithTimeout(t.Context(), duplicateWindow+failureBound)
	defer cancelExpiry()
	require.NoError(t, client.PublishToStreamWithMsgID(expiryCtx, "msgidexpiry.test", []byte("first"), "evt-1"))
	stored, err := probe.Await(expiryCtx, func(ctx context.Context) (uint64, error) {
		if err := client.PublishToStreamWithMsgID(ctx, "msgidexpiry.test", []byte("after-window"), "evt-1"); err != nil {
			return 0, err
		}
		info, err := stream.Info(ctx)
		if err != nil {
			return 0, err
		}
		return info.State.Msgs, nil
	}, func(msgs uint64) bool { return msgs == 2 })
	require.NoError(t, err, "the same Nats-Msg-Id must store again after the configured window (stored %d)", stored)
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
