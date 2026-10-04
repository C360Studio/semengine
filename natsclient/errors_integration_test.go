//go:build integration

package natsclient

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/pkg/errs"
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
			name:        "unclassified_plain_error_defaults_transient",
			handlerErr:  errors.New("plain — pkg/errs Classify defaults to transient"),
			isTransient: true,
			wantInMsg:   "plain",
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

// TestIntegration_LegacyRequest_SuccessBodyUnchanged confirms a plain Request()
// against a success handler sees exactly the bytes the handler returned —
// SubscribeForRequests must NOT introduce headers or alter the success-path
// payload (unchanged by ADR-060: only the FAILURE body moved to the envelope).
func TestIntegration_LegacyRequest_SuccessBodyUnchanged(t *testing.T) {
	ctx := context.Background()

	natsURL := startFixture(t).URL()

	client, err := NewClient(natsURL)
	require.NoError(t, err)
	require.NoError(t, client.Connect(ctx))
	defer closeClient(t, client)

	subject := "test.legacy.success.body"
	want := []byte(`{"hello":"world","number":42}`)
	_, err = client.SubscribeForRequests(ctx, subject, func(_ context.Context, _ []byte) ([]byte, error) {
		return want, nil
	})
	require.NoError(t, err)
	flushClient(t, client)

	data, err := client.Request(ctx, subject, []byte("ping"), failureBound)
	require.NoError(t, err)
	if !bytes.Equal(data, want) {
		t.Fatalf("Request success body diverged from handler bytes:\n  got  %q\n  want %q", data, want)
	}
}

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

	retry := DefaultRetryConfig()
	retry.InitialBackoff = 20 * time.Millisecond
	retry.MaxRetries = 2

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			subject := "test.retry_classified." + tc.name
			_, err = client.SubscribeForRequests(ctx, subject, func(_ context.Context, _ []byte) ([]byte, error) {
				return nil, tc.handlerErr
			})
			require.NoError(t, err)
			flushClient(t, client)

			data, err := client.RequestWithRetryClassified(ctx, subject, []byte("write"), failureBound, retry)
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

	data, err := client.RequestWithRetryClassified(ctx, subject, []byte("write"), failureBound, DefaultRetryConfig())
	require.NoError(t, err)
	if !bytes.Equal(data, want) {
		t.Fatalf("data = %q, want %q", data, want)
	}
}

// TestIntegration_RequestWithRetryClassified_RetriesThenSucceeds
// pins that the retry loop actually retries — responder starts late,
// the request retries through the responder-not-ready window, then
// succeeds with the classified contract intact. Catches a refactor
// that accidentally bypasses retry (e.g. delegating to
// RequestClassified directly).
func TestIntegration_RequestWithRetryClassified_RetriesThenSucceeds(t *testing.T) {
	ctx := context.Background()

	natsURL := startFixture(t).URL()

	client, err := NewClient(natsURL)
	require.NoError(t, err)
	require.NoError(t, client.Connect(ctx))
	defer closeClient(t, client)

	subject := "test.retry_classified.late_responder"
	want := []byte(`{"ack":"after-delay"}`)

	// The responder subscribes once the first attempt has failed for want of one (the client
	// counts the failure), so only a retry can succeed. The pin waited 150 ms instead.
	subscribed := make(chan error, 1)
	go func() {
		waitCtx, cancel := context.WithTimeout(ctx, failureBound)
		defer cancel()
		if _, err := probe.Await(waitCtx, func(context.Context) (int32, error) { return client.Failures(), nil },
			func(failures int32) bool { return failures >= 1 }); err != nil {
			subscribed <- err
			return
		}
		_, err := client.SubscribeForRequests(ctx, subject, func(_ context.Context, _ []byte) ([]byte, error) {
			return want, nil
		})
		subscribed <- err
	}()

	// Ten retries span about 5.6 s of backoff, so the late subscription lands inside the retry
	// window on a slow host; a correct run ends at the first retry after it.
	retry := DefaultRetryConfig()
	retry.InitialBackoff = 50 * time.Millisecond
	retry.MaxRetries = 10
	retry.BackoffMultiplier = 1.5

	data, err := client.RequestWithRetryClassified(ctx, subject, []byte("write"), 2*time.Second, retry)
	require.NoError(t, <-subscribed, "late responder")
	require.NoError(t, err, "retry+classified should succeed once late responder comes up")
	if !bytes.Equal(data, want) {
		t.Fatalf("data = %q, want %q", data, want)
	}
}
