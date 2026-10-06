//go:build integration

package natsclient

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/pkg/errs"
)

const responseBoundsTestMaxPayload int64 = 1024

type responseBoundsLogRecorder struct {
	mu      sync.Mutex
	records []slog.Record
}

func (r *responseBoundsLogRecorder) Enabled(context.Context, slog.Level) bool { return true }

func (r *responseBoundsLogRecorder) Handle(_ context.Context, record slog.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, record.Clone())
	return nil
}

func (r *responseBoundsLogRecorder) WithAttrs([]slog.Attr) slog.Handler { return r }
func (r *responseBoundsLogRecorder) WithGroup(string) slog.Handler      { return r }

func (r *responseBoundsLogRecorder) containsError(parts ...string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, record := range r.records {
		if record.Level != slog.LevelError {
			continue
		}
		text := record.Message
		record.Attrs(func(attr slog.Attr) bool {
			text += " " + attr.Key + "=" + attr.Value.String()
			return true
		})
		matched := true
		for _, part := range parts {
			if !strings.Contains(text, part) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func TestIntegration_ClientMaxPayloadObservesActiveConnection(t *testing.T) {
	testClient := newFixtureClient(t, natsfixture.WithMaxPayload(responseBoundsTestMaxPayload))

	maxPayload, err := testClient.Client.MaxPayload()
	require.NoError(t, err)
	assert.Equal(t, responseBoundsTestMaxPayload, maxPayload)
}

func TestIntegration_SubscribeForRequestsPublishesBeforeClassifyingResponseLimit(t *testing.T) {
	testClient := newFixtureClient(t, natsfixture.WithMaxPayload(responseBoundsTestMaxPayload))
	client := testClient.Client
	ctx := context.Background()

	tests := []struct {
		name              string
		pageFitMaxPayload int64
		responseBytes     int64
		wantRefusal       bool
	}{
		{
			name:              "below limit",
			pageFitMaxPayload: responseBoundsTestMaxPayload,
			responseBytes:     responseBoundsTestMaxPayload - 1,
		},
		{
			name:              "exact limit",
			pageFitMaxPayload: responseBoundsTestMaxPayload,
			responseBytes:     responseBoundsTestMaxPayload,
		},
		{
			name: "page fit observation becomes stale before publish",
			// Model an operation-owned page built against an earlier, larger
			// observation. The real broker below has a 1024-byte limit; only the
			// actual success publish can authoritatively reject this fitted page.
			pageFitMaxPayload: responseBoundsTestMaxPayload * 2,
			responseBytes:     responseBoundsTestMaxPayload + 1,
			wantRefusal:       true,
		},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			subject := "test.response.bounds." + string(rune('a'+index))
			observedLimit, err := client.MaxPayload()
			require.NoError(t, err)
			require.Equal(t, responseBoundsTestMaxPayload, observedLimit)
			require.LessOrEqual(t, test.responseBytes, test.pageFitMaxPayload,
				"the operation-owned page must fit its prior observation")

			response := []byte(strings.Repeat("x", int(test.responseBytes)))
			_, err = client.SubscribeForRequests(ctx, subject, func(context.Context, []byte) ([]byte, error) {
				return response, nil
			})
			require.NoError(t, err)
			require.NoError(t, client.GetConnection().Flush())

			data, requestErr := client.RequestClassified(ctx, subject, []byte("request"), failureBound)
			if !test.wantRefusal {
				require.NoError(t, requestErr)
				assert.Equal(t, response, data)
				return
			}

			require.Error(t, requestErr)
			assert.False(t, errors.Is(requestErr, nats.ErrTimeout), "oversize response must not become a timeout")
			assert.True(t, errs.IsInvalid(requestErr))
			var classified *errs.ClassifiedError
			require.ErrorAs(t, requestErr, &classified)
			assert.Equal(t, "response_too_large", classified.Code)
			assert.Equal(t, float64(test.responseBytes), classified.Detail["response_bytes"])
			assert.Equal(t, float64(responseBoundsTestMaxPayload), classified.Detail["max_payload"])
			assert.Nil(t, data, "the rejected success bytes must never be delivered as a response")
		})
	}
}

func TestIntegration_SubscribeForRequestsUsesSubscriptionConnectionForOversizeDiagnostic(t *testing.T) {
	testClient := newFixtureClient(t, natsfixture.WithMaxPayload(responseBoundsTestMaxPayload))
	client := testClient.Client
	ctx := context.Background()
	const subject = "test.response.bounds.subscription.connection"

	// The operation fitted this page against an earlier, larger observation.
	// The explicit barrier below forces the wrapper connection state to change
	// after page construction but before the responder attempts publication.
	priorPageFitMaxPayload := responseBoundsTestMaxPayload * 2
	response := []byte(strings.Repeat("x", int(responseBoundsTestMaxPayload+1)))
	require.LessOrEqual(t, int64(len(response)), priorPageFitMaxPayload)
	handlerReady := make(chan struct{})
	publishResponse := make(chan struct{})
	_, err := client.SubscribeForRequests(ctx, subject, func(context.Context, []byte) ([]byte, error) {
		close(handlerReady)
		<-publishResponse
		return response, nil
	})
	require.NoError(t, err)

	subscriptionConn := client.GetConnection()
	require.NoError(t, subscriptionConn.Flush())
	t.Cleanup(func() { client.SetConnection(subscriptionConn) })

	requestResult := make(chan error, 1)
	go func() {
		reply, requestErr := subscriptionConn.Request(subject, []byte("request"), failureBound)
		if requestErr == nil {
			_, requestErr = ClassifyReply(reply)
		}
		requestResult <- requestErr
	}()

	waitCtx, cancel := context.WithTimeout(ctx, failureBound)
	defer cancel()
	select {
	case <-handlerReady:
	case <-waitCtx.Done():
		t.Fatal("request handler did not reach the explicit publication barrier")
	}

	// Model the reconnect window after the request was delivered: Client's
	// active-connection observation is unavailable, while the message remains
	// bound to the exact connection that accepted the subscription.
	client.SetConnection(nil)
	_, err = client.MaxPayload()
	require.ErrorIs(t, err, ErrNotConnected)
	close(publishResponse)

	select {
	case requestErr := <-requestResult:
		require.Error(t, requestErr)
		assert.False(t, errors.Is(requestErr, nats.ErrTimeout),
			"an unavailable Client observation must not turn oversize refusal into timeout")
		assert.True(t, errs.IsInvalid(requestErr))
		var classified *errs.ClassifiedError
		require.ErrorAs(t, requestErr, &classified)
		assert.Equal(t, errorCodeResponseTooLarge, classified.Code)
		assert.Equal(t, float64(len(response)), classified.Detail["response_bytes"])
		assert.Equal(t, float64(responseBoundsTestMaxPayload), classified.Detail["max_payload"])
	case <-waitCtx.Done():
		t.Fatal("request did not receive the classified oversize refusal")
	}
}

func TestIntegration_SubscribeForRequestsLogsResponseLimitRefusalPublishFailure(t *testing.T) {
	const tinyMaxPayload int64 = 64
	testClient := newFixtureClient(t, natsfixture.WithMaxPayload(tinyMaxPayload))
	client := testClient.Client
	recorder := &responseBoundsLogRecorder{}
	client.logger = slog.New(recorder)
	ctx := context.Background()
	const subject = "test.response.bounds.refusal.publish.failure"

	_, err := client.SubscribeForRequests(ctx, subject, func(context.Context, []byte) ([]byte, error) {
		return []byte(strings.Repeat("x", int(tinyMaxPayload+1))), nil
	})
	require.NoError(t, err)
	flushCtx, cancelFlush := context.WithTimeout(ctx, failureBound)
	defer cancelFlush()
	require.NoError(t, client.GetConnection().FlushWithContext(flushCtx))

	// An observer on its own connection sees every reply to any inbox, the requester's
	// included, so it can show that none was published without relying on the request's
	// own outcome. Its subscription is registered at the broker before the request is sent.
	observer := dial(t, testClient.URL)
	replies, err := observer.SubscribeSync("_INBOX.>")
	require.NoError(t, err)
	require.NoError(t, observer.FlushWithContext(flushCtx))

	reqCtx, cancelRequest := context.WithCancel(ctx)
	defer cancelRequest()
	requestResult := make(chan error, 1)
	go func() {
		_, requestErr := client.GetConnection().RequestWithContext(reqCtx, subject, []byte("x"))
		requestResult <- requestErr
	}()
	logCtx, cancelLog := context.WithTimeout(ctx, failureBound)
	defer cancelLog()
	_, err = probe.Await(logCtx, func(context.Context) (bool, error) {
		return recorder.containsError("failed to publish response-too-large reply", "maximum payload exceeded"), nil
	}, func(logged bool) bool { return logged })
	require.NoError(t, err, "the refusal publish failure was not logged")

	// The record is written after the responder's last publish attempt. Flushing the
	// responder's connection makes the broker process everything it had published before
	// the record; flushing the observer's then delivers, ahead of its PONG, any reply the
	// broker routed to it. An empty pending queue after both is the observed absence of a
	// reply, made before the request is cancelled. The pin waited for a 150 ms request
	// timeout instead.
	require.NoError(t, client.GetConnection().FlushWithContext(flushCtx))
	require.NoError(t, observer.FlushWithContext(flushCtx))
	pending, _, err := replies.Pending()
	require.NoError(t, err)
	require.Zero(t, pending, "the responder must publish no reply after its refusal publish failed")

	cancelRequest()
	select {
	case requestErr := <-requestResult:
		require.ErrorIs(t, requestErr, context.Canceled, "with no reply published, only the cancellation ends the request")
	case <-time.After(failureBound):
		t.Fatal("request did not end after its context was cancelled")
	}
}
