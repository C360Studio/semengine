//go:build integration

package natsclient

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readyTestClient(ctx context.Context, t *testing.T) *Client {
	t.Helper()
	natsURL := startFixture(t).URL()
	client, err := NewClient(natsURL)
	require.NoError(t, err)
	require.NoError(t, client.Connect(ctx))
	t.Cleanup(func() { closeClient(t, client) })
	return client
}

// A responder that subscribes AFTER the reader starts must still be found: this
// is the cold-start race gated-dag hit (gh#420). A single-shot short request
// would fail; the readiness read retries within its budget and succeeds once the
// responder is up. RequestReady was dropped as dead surface (task 3.7a); these
// tests call requestMsgReady, which it wrapped and RequestReadyClassified calls.
func TestIntegration_RequestReady_ResponderAppearsLate(t *testing.T) {
	ctx := context.Background()
	client := readyTestClient(ctx, t)
	const subject = "test.ready.late"

	// A monitor that never replies sees the first attempt; the responder subscribes
	// only after that, so the first attempt can only time out and success proves the
	// loop RETRIED. The pin subscribed the responder after a 700 ms sleep instead.
	// The monitor unsubscribes once it has seen the first attempt: its unread
	// messages would otherwise hold the client's drain open at Close.
	monitor, err := client.GetConnection().SubscribeSync(subject)
	require.NoError(t, err)
	t.Cleanup(func() { _ = monitor.Unsubscribe() })
	flushClient(t, client)
	subscribed := make(chan error, 1)
	go func() {
		waitCtx, cancel := context.WithTimeout(ctx, failureBound)
		defer cancel()
		if _, err := monitor.NextMsgWithContext(waitCtx); err != nil {
			subscribed <- err
			return
		}
		if err := monitor.Unsubscribe(); err != nil {
			subscribed <- err
			return
		}
		_, err := client.SubscribeForRequests(ctx, subject, func(_ context.Context, _ []byte) ([]byte, error) {
			return []byte("pong"), nil
		})
		subscribed <- err
	}()

	start := time.Now()
	// probe 300ms, budget 10s (generous).
	resp, err := client.requestMsgReady(ctx, subject, []byte("ping"), 300*time.Millisecond, 10*time.Second)
	elapsed := time.Since(start)

	require.NoError(t, <-subscribed, "the monitor saw the first attempt and the responder subscribed")
	require.NoError(t, err, "readiness read must succeed once the responder appears")
	assert.Equal(t, []byte("pong"), resp.Data)
	// Well under the budget — proves it converged on the responder, not the cap.
	assert.Less(t, elapsed, 5*time.Second, "should return shortly after the responder appears, not near the budget")
}

// No responder ever appears: the read must surface an error BOUNDED by the
// budget, not hang to a full per-attempt query timeout (the gh#420 30s hang).
func TestIntegration_RequestReady_NeverReady_BoundedByBudget(t *testing.T) {
	ctx := context.Background()
	client := readyTestClient(ctx, t)

	start := time.Now()
	_, err := client.requestMsgReady(ctx, "test.ready.absent", []byte("ping"), 300*time.Millisecond, 1*time.Second)
	elapsed := time.Since(start)

	require.Error(t, err)
	// Bounded by the budget (1s) with a ≥3× ceiling for CI/container jitter.
	// The point is it does NOT hang past the budget.
	assert.Less(t, elapsed, 3*time.Second, "must be bounded by the readiness budget, not a full query timeout")
}

// An immediately-available responder returns on the first attempt.
func TestIntegration_RequestReady_ImmediateResponder(t *testing.T) {
	ctx := context.Background()
	client := readyTestClient(ctx, t)
	const subject = "test.ready.immediate"

	_, err := client.SubscribeForRequests(ctx, subject, func(_ context.Context, _ []byte) ([]byte, error) {
		return []byte("pong"), nil
	})
	require.NoError(t, err)

	// No sleep: the readiness read tolerates not-yet-propagated interest by retrying —
	// that is the whole point. A present responder returns quickly.
	resp, err := client.requestMsgReady(ctx, subject, []byte("ping"), 300*time.Millisecond, 10*time.Second)
	require.NoError(t, err)
	assert.Equal(t, []byte("pong"), resp.Data)
}

// A reply — even a handler-ERROR reply — means the responder is UP, so the loop
// must STOP (not retry to the budget) and return the classified error promptly.
func TestIntegration_RequestReadyClassified_HandlerErrorStopsLoop(t *testing.T) {
	ctx := context.Background()
	client := readyTestClient(ctx, t)
	const subject = "test.ready.handlererr"

	const marker = "handler-rejected-marker"
	_, err := client.SubscribeForRequests(ctx, subject, func(_ context.Context, _ []byte) ([]byte, error) {
		return nil, errors.New(marker) // handler error → classified error reply
	})
	require.NoError(t, err)

	start := time.Now()
	// Budget is large; if the loop wrongly retried the handler error it would burn
	// the whole budget. It must return promptly because a reply WAS received.
	_, err = client.RequestReadyClassified(ctx, subject, []byte("ping"), 300*time.Millisecond, 10*time.Second)
	elapsed := time.Since(start)

	require.Error(t, err)
	// The error must be the round-tripped HANDLER error (proving a reply was
	// received + classified), not a transport/budget error (which would say
	// "readiness budget" or "no responders").
	assert.Contains(t, err.Error(), marker, "must return the classified handler error, not a transport error")
	assert.NotContains(t, err.Error(), "readiness budget", "must not have retried to budget exhaustion")
	assert.Less(t, elapsed, 3*time.Second, "a received (error) reply means responder is up — must not retry to the budget")
}
