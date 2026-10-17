//go:build integration

package natsclient

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/retry"
)

// TestIntegration_RequestClassified_RoundTripPreservesClass is the
// load-bearing gh#93 contract: a handler that returns
// errs.WrapInvalid(...) MUST surface to the caller as a classified
// error where errs.IsInvalid(err) == true. This is the regression
// net that catches Phase 1 wire-format gaps.
func TestIntegration_RequestClassified_RoundTripPreservesClass(t *testing.T) {
	ctx := context.Background()

	natsURL := startFixture(t).URL()

	client, err := NewClient(natsURL)
	require.NoError(t, err)
	require.NoError(t, client.Connect(ctx))
	defer closeClient(t, client)

	cases := []struct {
		name        string
		handlerErr  error
		isInvalid   bool
		isTransient bool
		isFatal     bool
		wantInMsg   string
	}{
		{
			name:       "invalid_class_round_trips",
			handlerErr: errs.WrapInvalid(errors.New("bad request shape"), "Test", "Handle", "validate"),
			isInvalid:  true,
			wantInMsg:  "bad request shape",
		},
		{
			name:        "transient_class_round_trips",
			handlerErr:  errs.WrapTransient(errors.New("kv temporarily unavailable"), "Test", "Handle", "fetch"),
			isTransient: true,
			wantInMsg:   "kv temporarily unavailable",
		},
		{
			name:       "fatal_class_round_trips",
			handlerErr: errs.WrapFatal(errors.New("kv permanently unreachable"), "Test", "Handle", "fetch"),
			isFatal:    true,
			wantInMsg:  "kv permanently unreachable",
		},
		{
			// An unclassified error is fatal (owner ruling 1, #167 comment 6191970843).
			name:       "unclassified_plain_error_defaults_fatal",
			handlerErr: errors.New("marshal failed"),
			isFatal:    true,
			wantInMsg:  "marshal failed",
		},
	}

	for i, tc := range cases {
		tc := tc
		i := i
		t.Run(tc.name, func(t *testing.T) {
			subject := "test.classified." + tc.name
			_, err = client.SubscribeForRequests(ctx, subject, func(_ context.Context, _ []byte) ([]byte, error) {
				return nil, tc.handlerErr
			})
			require.NoError(t, err)

			// The subscription is registered once the broker has answered a flush on
			// the same connection.
			flushClient(t, client)

			data, err := client.RequestClassified(ctx, subject, []byte("ping"), failureBound)
			if data != nil {
				t.Errorf("data should be nil on classified error; got %q", data)
			}
			if err == nil {
				t.Fatal("expected classified error")
			}
			if errs.IsInvalid(err) != tc.isInvalid {
				t.Errorf("IsInvalid=%v, want %v (err=%v)", errs.IsInvalid(err), tc.isInvalid, err)
			}
			if errs.IsTransient(err) != tc.isTransient {
				t.Errorf("IsTransient=%v, want %v (err=%v)", errs.IsTransient(err), tc.isTransient, err)
			}
			if errs.IsFatal(err) != tc.isFatal {
				t.Errorf("IsFatal=%v, want %v (err=%v)", errs.IsFatal(err), tc.isFatal, err)
			}
			if !strings.Contains(err.Error(), tc.wantInMsg) {
				t.Errorf("case %d err=%q, want substring %q", i, err.Error(), tc.wantInMsg)
			}
		})
	}
}

// ADR-060 PR-D removed TestIntegration_LegacyBodyPrefix_StillReadable: the
// legacy "error: " body shape is gone. A handler failure is now the
// {message, detail} envelope + X-Status header (covered by the unit-level
// TestRespondError_RoundTrip and the header-classified tests above).

// TestIntegration_RequestClassified_SuccessPath confirms the
// non-error path: handler returns (data, nil), caller gets the same
// data, nil error.
func TestIntegration_RequestClassified_SuccessPath(t *testing.T) {
	ctx := context.Background()

	natsURL := startFixture(t).URL()

	client, err := NewClient(natsURL)
	require.NoError(t, err)
	require.NoError(t, client.Connect(ctx))
	defer closeClient(t, client)

	subject := "test.classified.success"
	want := []byte(`{"ok":true}`)
	_, err = client.SubscribeForRequests(ctx, subject, func(_ context.Context, _ []byte) ([]byte, error) {
		return want, nil
	})
	require.NoError(t, err)
	flushClient(t, client)

	data, err := client.RequestClassified(ctx, subject, []byte("ping"), failureBound)
	require.NoError(t, err)
	if string(data) != string(want) {
		t.Fatalf("data = %q, want %q", data, want)
	}
}

// TestIntegration_RequestWithRetryClassified_RoundTripPreservesClass
// is the gh#192 matrix-gap-closure regression net. Mirrors the
// RequestClassified case table but drives through the retry-aware
// entry point — proves the retry loop preserves the classified
// contract instead of returning the legacy text-body shape (which
// is the Footgun semteams shipped a `json.Valid` workaround for in
// cmd/semteams/tools/addsource/executor.go).
func TestIntegration_RequestWithRetryClassified_RoundTripPreservesClass(t *testing.T) {
	ctx := context.Background()

	natsURL := startFixture(t).URL()

	client, err := NewClient(natsURL)
	require.NoError(t, err)
	require.NoError(t, client.Connect(ctx))
	defer closeClient(t, client)

	cases := []struct {
		name        string
		handlerErr  error
		isInvalid   bool
		isTransient bool
		isFatal     bool
		wantInMsg   string
	}{
		{
			name:       "invalid_class_round_trips",
			handlerErr: errs.WrapInvalid(errors.New("bad mutation shape"), "Test", "Handle", "validate"),
			isInvalid:  true,
			wantInMsg:  "bad mutation shape",
		},
		{
			name:        "transient_class_round_trips",
			handlerErr:  errs.WrapTransient(errors.New("kv temporarily unavailable"), "Test", "Handle", "write"),
			isTransient: true,
			wantInMsg:   "kv temporarily unavailable",
		},
		{
			name:       "fatal_class_round_trips",
			handlerErr: errs.WrapFatal(errors.New("kv permanently unreachable"), "Test", "Handle", "write"),
			isFatal:    true,
			wantInMsg:  "kv permanently unreachable",
		},
	}

	cfg := retry.DefaultConfig()
	cfg.InitialDelay = 20 * time.Millisecond
	cfg.MaxAttempts = 3

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			subject := "test.retry_classified." + tc.name
			_, err = client.SubscribeForRequests(ctx, subject, func(_ context.Context, _ []byte) ([]byte, error) {
				return nil, tc.handlerErr
			})
			require.NoError(t, err)
			flushClient(t, client)

			data, err := client.RequestWithRetryClassified(ctx, subject, []byte("write"), failureBound, cfg)
			if data != nil {
				t.Errorf("data should be nil on classified error; got %q", data)
			}
			if err == nil {
				t.Fatal("expected classified error")
			}
			if errs.IsInvalid(err) != tc.isInvalid {
				t.Errorf("IsInvalid=%v, want %v (err=%v)", errs.IsInvalid(err), tc.isInvalid, err)
			}
			if errs.IsTransient(err) != tc.isTransient {
				t.Errorf("IsTransient=%v, want %v (err=%v)", errs.IsTransient(err), tc.isTransient, err)
			}
			if errs.IsFatal(err) != tc.isFatal {
				t.Errorf("IsFatal=%v, want %v (err=%v)", errs.IsFatal(err), tc.isFatal, err)
			}
			if !strings.Contains(err.Error(), tc.wantInMsg) {
				t.Errorf("err=%q, want substring %q", err.Error(), tc.wantInMsg)
			}
		})
	}
}

// TestIntegration_RequestWithRetryClassified_SuccessPath confirms
// the non-error path through the retry-aware entry point: handler
// returns (data, nil), caller gets the same data, nil error.
func TestIntegration_RequestWithRetryClassified_SuccessPath(t *testing.T) {
	ctx := context.Background()

	natsURL := startFixture(t).URL()

	client, err := NewClient(natsURL)
	require.NoError(t, err)
	require.NoError(t, client.Connect(ctx))
	defer closeClient(t, client)

	subject := "test.retry_classified.success"
	want := []byte(`{"ok":true,"id":"abc123"}`)
	_, err = client.SubscribeForRequests(ctx, subject, func(_ context.Context, _ []byte) ([]byte, error) {
		return want, nil
	})
	require.NoError(t, err)
	flushClient(t, client)

	data, err := client.RequestWithRetryClassified(ctx, subject, []byte("write"), failureBound, retry.DefaultConfig())
	require.NoError(t, err)
	if !bytes.Equal(data, want) {
		t.Fatalf("data = %q, want %q", data, want)
	}
}

// A reply ends the retrying call, an error reply included: the handler's error comes back
// classified after one attempt and is never retried, though the config allows more attempts.
func TestIntegration_RequestWithRetryClassified_ErrorReplyIsNotRetried(t *testing.T) {
	client := newFixtureClient(t).Client

	const subject = "test.retry_classified.error_not_retried"
	var calls atomic.Int64
	_, err := client.SubscribeForRequests(t.Context(), subject, func(context.Context, []byte) ([]byte, error) {
		calls.Add(1)
		return nil, errs.WrapInvalid(errors.New("refused write"), "Test", "Handle", "validate")
	})
	require.NoError(t, err)
	flushClient(t, client)

	cfg := retry.Config{MaxAttempts: 3, InitialDelay: time.Millisecond, MaxDelay: time.Millisecond, Multiplier: 1}
	data, err := client.RequestWithRetryClassified(t.Context(), subject, []byte("write"), failureBound, cfg)
	require.Nil(t, data)
	require.True(t, errs.IsInvalid(err), "want the handler's invalid error, got %v", err)
	require.Contains(t, err.Error(), "refused write")
	// Every attempt the call made was answered before it returned, so the count is final here.
	require.Equal(t, int64(1), calls.Load(), "an error reply was retried")
}

// RequestWithHeaders returns the reply as received: a handler error arrives as a reply carrying
// X-Status: error with a nil error, and ClassifyReply on that reply returns the handler's class and
// code.
func TestIntegration_RequestWithHeaders_ErrorReplyKeepsHeaders(t *testing.T) {
	client := newFixtureClient(t).Client

	const subject = "test.headers.error_reply"
	_, err := client.SubscribeForRequests(t.Context(), subject, func(context.Context, []byte) ([]byte, error) {
		return nil, errs.ClassifiedCode(errs.ErrorInvalid, "entity_not_found", errors.New("no such entity"))
	})
	require.NoError(t, err)
	flushClient(t, client)

	msg, err := client.RequestWithHeaders(t.Context(), subject, []byte("write"), nil, failureBound)
	require.NoError(t, err)
	require.NotNil(t, msg)
	require.Equal(t, "error", msg.Header.Get("X-Status"))

	data, err := ClassifyReply(msg)
	require.Nil(t, data)
	require.True(t, errs.IsInvalid(err), "want an invalid error, got %v", err)
	var ce *errs.ClassifiedError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, "entity_not_found", ce.Code)
	require.Contains(t, err.Error(), "no such entity")
}

// TestIntegration_RequestWithRetryClassified_RetriesThenSucceeds
// pins that the retry loop actually retries: the responder is subscribed
// from the start, leaves its first request unanswered and answers the
// second, so only a retry can succeed, and its handler count proves the
// retry from the responder's side. Catches a refactor that accidentally
// bypasses retry (e.g. delegating to RequestClassified directly).
func TestIntegration_RequestWithRetryClassified_RetriesThenSucceeds(t *testing.T) {
	ctx := context.Background()

	natsURL := startFixture(t).URL()

	client, err := NewClient(natsURL)
	require.NoError(t, err)
	require.NoError(t, client.Connect(ctx))
	defer closeClient(t, client)

	subject := "test.retry_classified.drops_first"
	want := []byte(`{"ack":"second-attempt"}`)

	var received atomic.Int32
	sub, err := client.GetConnection().Subscribe(subject, func(msg *nats.Msg) {
		if received.Add(1) == 1 {
			return // the first attempt gets no reply and times out
		}
		_ = msg.Respond(want) // a failed respond fails the call below
	})
	require.NoError(t, err)
	defer func() { require.NoError(t, sub.Unsubscribe()) }()
	flushClient(t, client)

	// The per-attempt timeout is what the first attempt waits; the second is answered at once.
	// MaxAttempts 3 leaves one spare attempt, so a call that stopped retrying early fails rather
	// than one that retried once too often passing.
	cfg := retry.Config{MaxAttempts: 3, InitialDelay: 20 * time.Millisecond, MaxDelay: 100 * time.Millisecond, Multiplier: 2}

	data, err := client.RequestWithRetryClassified(ctx, subject, []byte("write"), time.Second, cfg)
	require.NoError(t, err, "retry+classified should succeed on the second attempt")
	if !bytes.Equal(data, want) {
		t.Fatalf("data = %q, want %q", data, want)
	}
	require.Equal(t, int32(2), received.Load(), "requests the responder received")
}
