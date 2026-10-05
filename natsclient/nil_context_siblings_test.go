package natsclient

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/pkg/errs"
)

// The siblings of the entry points in Codex's F32 probe (reviewer_nil_context_probe_test.go): every
// exported, context-taking method on the request/reply path and the KV bucket acquisition path
// refuses a nil context with an invalid-data error, on a connected client, without panicking.
func TestExportedNilContextRefusedOnRequestAndBucketPaths(t *testing.T) {
	c, err := NewClient(embeddedJetStreamURL(t), WithHealthInterval(0))
	require.NoError(t, err)
	require.NoError(t, c.Connect(t.Context()))
	closeInCleanup(t, c)

	const subject = "nilctx.siblings"
	var calls atomic.Int64
	_, err = c.SubscribeForRequests(t.Context(), subject, func(context.Context, []byte) ([]byte, error) {
		calls.Add(1)
		return []byte("ok"), nil
	})
	require.NoError(t, err)
	require.NoError(t, c.GetConnection().FlushTimeout(10*time.Second))
	// Positive control: the connected client answers with a real context.
	data, err := c.RequestClassified(t.Context(), subject, nil, 10*time.Second)
	require.NoError(t, err)
	require.Equal(t, "ok", string(data))

	for _, tc := range []struct {
		name string
		call func(context.Context) error
	}{
		{"RequestClassified", func(ctx context.Context) error {
			_, err := c.RequestClassified(ctx, subject, nil, 10*time.Second)
			return err
		}},
		{"RequestWithRetry", func(ctx context.Context) error {
			_, err := c.RequestWithRetry(ctx, subject, nil, 10*time.Second, DefaultRetryConfig())
			return err
		}},
		{"RequestWithRetryClassified", func(ctx context.Context) error {
			_, err := c.RequestWithRetryClassified(ctx, subject, nil, 10*time.Second, DefaultRetryConfig())
			return err
		}},
		{"RequestReadyClassified", func(ctx context.Context) error {
			_, err := c.RequestReadyClassified(ctx, subject, nil, 0, 0)
			return err
		}},
		{"Reply", func(ctx context.Context) error { return c.Reply(ctx, "nilctx.reply", nil) }},
		{"ReplyWithHeaders", func(ctx context.Context) error {
			return c.ReplyWithHeaders(ctx, "nilctx.reply", nil, map[string]string{"k": "v"})
		}},
		{"Publish", func(ctx context.Context) error { return c.Publish(ctx, "nilctx.publish", nil) }},
		{"SubscribeForRequests", func(ctx context.Context) error {
			sub, err := c.SubscribeForRequests(ctx, "nilctx.subscribe", func(context.Context, []byte) ([]byte, error) {
				return nil, nil
			})
			if sub != nil {
				return errors.New("a refused subscribe returned a subscription")
			}
			return err
		}},
		{"GetKeyValueBucket", func(ctx context.Context) error {
			_, err := c.GetKeyValueBucket(ctx, "NILCTX")
			return err
		}},
		{"CreateKeyValueBucket", func(ctx context.Context) error {
			_, err := c.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{Bucket: "NILCTX"})
			return err
		}},
		{"WaitForBucket", func(ctx context.Context) error {
			_, err := c.WaitForBucket(ctx, "NILCTX", time.Second)
			return err
		}},
		{"DeleteKeyValueBucket", func(ctx context.Context) error { return c.DeleteKeyValueBucket(ctx, "NILCTX") }},
		{"ListKeyValueBuckets", func(ctx context.Context) error {
			_, err := c.ListKeyValueBuckets(ctx)
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if p := recover(); p != nil {
					t.Errorf("nil context panicked: %v", p)
				}
			}()
			var nilCtx context.Context
			err := tc.call(nilCtx)
			if !errs.IsInvalid(err) {
				t.Errorf("nil context: err = %v, want an invalid-data refusal", err)
			}
		})
	}

	// No refused call reached the responder: a request sent after them on the same connection is
	// delivered after anything they sent, and it is the only one since the positive control.
	_, err = c.RequestClassified(t.Context(), subject, nil, 10*time.Second)
	require.NoError(t, err)
	require.Equal(t, int64(2), calls.Load())
}

// An already-cancelled context sends nothing from either retrying request method: each attempt,
// the first included, checks the context first (Codex F32), and the refusal is a transient
// classified error that still matches context.Canceled.
func TestRetryRequestEndedContextNeverDispatches(t *testing.T) {
	c, err := NewClient(embeddedServerURL(t), WithHealthInterval(0))
	require.NoError(t, err)
	require.NoError(t, c.Connect(t.Context()))
	closeInCleanup(t, c)
	const subject = "endedctx.dispatch"
	var calls atomic.Int64
	_, err = c.SubscribeForRequests(t.Context(), subject, func(context.Context, []byte) ([]byte, error) {
		calls.Add(1)
		return []byte("handled"), nil
	})
	require.NoError(t, err)
	require.NoError(t, c.GetConnection().FlushTimeout(10*time.Second))

	ended, cancel := context.WithCancel(t.Context())
	cancel()
	failuresBefore := c.GetStatus().FailureCount
	_, err = c.RequestWithRetry(ended, subject, nil, 10*time.Second, DefaultRetryConfig())
	require.ErrorIs(t, err, context.Canceled)
	require.True(t, errs.IsTransient(err), "RequestWithRetry: %v", err)
	_, err = c.RequestWithRetryClassified(ended, subject, nil, 10*time.Second, DefaultRetryConfig())
	require.ErrorIs(t, err, context.Canceled)
	require.True(t, errs.IsTransient(err), "RequestWithRetryClassified: %v", err)
	// No attempt was made, so the caller's cancellation is not a transport failure: it must not
	// count toward the shared circuit breaker that fast-fails unrelated calls.
	require.Equal(t, failuresBefore, c.GetStatus().FailureCount, "an ended context recorded a transport failure")

	// Sent after the refused calls on the same connection, so delivered after anything they sent.
	data, err := c.RequestWithRetryClassified(t.Context(), subject, nil, 10*time.Second, RetryConfig{})
	require.NoError(t, err)
	require.Equal(t, "handled", string(data))
	require.Equal(t, int64(1), calls.Load(), "only the request with a live context reached the handler")
}

// A negative MaxRetries is refused as invalid by both retrying request methods and sends nothing
// (Codex F31). The probe in reviewer_full_nats_probe_test.go proves the refusal; this proves no
// request reached the handler.
func TestNegativeMaxRetriesNeverDispatches(t *testing.T) {
	c, err := NewClient(embeddedServerURL(t), WithHealthInterval(0))
	require.NoError(t, err)
	require.NoError(t, c.Connect(t.Context()))
	closeInCleanup(t, c)
	const subject = "negretry.dispatch"
	var calls atomic.Int64
	_, err = c.SubscribeForRequests(t.Context(), subject, func(context.Context, []byte) ([]byte, error) {
		calls.Add(1)
		return []byte("handled"), nil
	})
	require.NoError(t, err)
	require.NoError(t, c.GetConnection().FlushTimeout(10*time.Second))

	negative := RetryConfig{MaxRetries: -1}
	_, err = c.RequestWithRetry(t.Context(), subject, nil, 10*time.Second, negative)
	require.True(t, errs.IsInvalid(err), "RequestWithRetry: %v", err)
	_, err = c.RequestWithRetryClassified(t.Context(), subject, nil, 10*time.Second, negative)
	require.True(t, errs.IsInvalid(err), "RequestWithRetryClassified: %v", err)

	// Sent after the refused calls on the same connection, so delivered after anything they sent.
	data, err := c.RequestWithRetryClassified(t.Context(), subject, nil, 10*time.Second, RetryConfig{})
	require.NoError(t, err)
	require.Equal(t, "handled", string(data))
	require.Equal(t, int64(1), calls.Load(), "only the valid request reached the handler")
}
