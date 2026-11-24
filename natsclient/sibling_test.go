package natsclient

import (
	"context"
	"log/slog"
	"net"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/metric"
)

// clientWorkFrames are the frames of the goroutines a Client runs for itself: every goroutine
// started through startBackground (the metrics poller and claim releases among them), the join a
// Close starts, the wait for a connection's closed handler, and the metrics poller.
var clientWorkFrames = []string{
	"natsclient.(*Client).startBackground.func1(",
	"natsclient.(*Client).join(",
	"natsclient.(*Client).awaitClosedHandler(",
	"natsclient.(*jetstreamMetrics).runPoller(",
}

// clientWorkStacks returns the stacks of goroutines in one of frames.
func clientWorkStacks(frames ...string) []string {
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	var found []string
	for _, stack := range strings.Split(string(buf), "\n\n") {
		for _, frame := range frames {
			if strings.Contains(stack, frame) {
				found = append(found, stack)
				break
			}
		}
	}
	return found
}

// noMoreClientWork is a scan for natsfixture.AwaitNoStacks: "" while no more goroutines are in
// frames than the baseline count, else their stacks. The baseline is read before the sibling is
// made, because the test's parent Client runs the same frames.
func noMoreClientWork(baseline int, frames ...string) func() string {
	return func() string {
		if stacks := clientWorkStacks(frames...); len(stacks) > baseline {
			return strings.Join(stacks, "\n\n")
		}
		return ""
	}
}

// connectSiblingScripted connects sibling to a scripted server of its own on a net.Pipe and
// registers its Close, under a bounded context, on t's cleanup.
func connectSiblingScripted(t *testing.T, sibling *Client) *scriptedServer {
	t.Helper()
	cli, srv := net.Pipe()
	server := serveScripted(srv)
	sibling.netDial = (&pipeDialer{conn: cli}).Dial
	t.Cleanup(func() {
		server.answerPings()
		ctx, cancel := context.WithTimeout(context.Background(), closeBudget)
		defer cancel()
		_ = sibling.Close(ctx)
		_ = cli.Close()
	})
	require.NoError(t, sibling.Connect(t.Context()))
	return server
}

// gatherSeries returns every semengine series in registry, one string per metric with its labels
// and value, so two reads can be compared. The Go runtime and process series the registry also
// holds change on their own and are left out.
func gatherSeries(t *testing.T, registry *metric.MetricsRegistry) []string {
	t.Helper()
	families, err := registry.PrometheusRegistry().Gather()
	require.NoError(t, err)
	var series []string
	for _, family := range families {
		if !strings.HasPrefix(family.GetName(), "semengine_") {
			continue
		}
		for _, m := range family.GetMetric() {
			series = append(series, family.GetName()+" "+m.String())
		}
	}
	return series
}

// Every setting a Client is built with but its metrics and its exit policy reaches the sibling.
// Each value differs from NewClient's default, so a sibling built from defaults fails.
func TestSiblingCopiesEverySetting(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	parent, err := NewClient("nats://unused-one,nats://unused-two",
		WithMaxReconnects(7),
		WithReconnectWait(123*time.Millisecond),
		WithPingInterval(456*time.Millisecond),
		WithWriteTimeout(789*time.Millisecond),
		WithTimeout(1011*time.Millisecond),
		WithCredentials("sibling-user", "sibling-pass"),
		WithName("sibling-name"),
		WithLogger(logger),
	)
	require.NoError(t, err)
	parent.drainTimeout = 1213 * time.Millisecond
	parent.requestHandlerTimeout = 1415 * time.Millisecond

	sibling, err := parent.NewSibling()
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), closeBudget)
		defer cancel()
		_ = sibling.Close(ctx)
		_ = parent.Close(ctx)
	})

	require.Equal(t, "nats://unused-one,nats://unused-two", sibling.URLs())
	require.Equal(t, 7, sibling.maxReconnects)
	require.Equal(t, 123*time.Millisecond, sibling.reconnectWait)
	require.Equal(t, 456*time.Millisecond, sibling.pingInterval)
	require.Equal(t, 789*time.Millisecond, sibling.writeTimeout)
	require.Equal(t, 1011*time.Millisecond, sibling.timeout)
	require.Equal(t, 1213*time.Millisecond, sibling.drainTimeout)
	require.Equal(t, 1415*time.Millisecond, sibling.requestHandlerTimeout)
	require.Equal(t, "sibling-user", sibling.username)
	require.Equal(t, "sibling-pass", sibling.password)
	require.Equal(t, "sibling-name", sibling.clientName)
	require.Same(t, logger, sibling.logger)
	require.NotNil(t, sibling.netDial, "a sibling must be able to dial")
	require.Equal(t, StatusDisconnected, sibling.Status())
	require.Nil(t, sibling.GetConnection(), "a sibling starts unconnected")
}

// NewSibling after the parent's Close has begun returns nats.ErrConnectionClosed and no Client.
// Before the Close, the same parent gives a sibling, so the refusal is the Close's doing.
func TestSiblingRefusedAfterClose(t *testing.T) {
	parent, err := NewClient("nats://unused", WithLogger(slog.New(slog.DiscardHandler)))
	require.NoError(t, err)

	before, err := parent.NewSibling()
	require.NoError(t, err)
	require.NotNil(t, before)
	require.NoError(t, before.Close(t.Context()))

	require.NoError(t, parent.Close(t.Context()))

	after, err := parent.NewSibling()
	require.Nil(t, after)
	require.ErrorIs(t, err, nats.ErrConnectionClosed)
}

// A sibling of a Client with a metrics registry, a connection-lost callback and a connection-loss
// timeout gets none of them: no poller runs for it, the registry holds only the parent's series,
// and a permission refusal on it is logged and not counted. Both run against scripted servers on
// a net.Pipe, so no address is bound.
func TestSiblingHasNoMetricsAndNoLossCallback(t *testing.T) {
	registry := metric.NewMetricsRegistry()
	logs := newRefusalCountHandler(registry)
	lost := func(error) {}
	parent, _ := connectScripted(t,
		WithMetrics(registry),
		WithLogger(slog.New(logs)),
		WithConnectionLostCallback(lost),
		WithConnectionLossTimeout(time.Minute),
	)
	// The parent's poller is admitted before its Connect returns, so the frames compared below
	// are ones a Client of this configuration runs.
	parent.mu.RLock()
	parentPollers := parent.running[workMetricsPoller]
	parent.mu.RUnlock()
	require.Equal(t, 1, parentPollers, "the parent runs the metrics poller")

	baseline := len(clientWorkStacks(clientWorkFrames...))
	seriesBefore := gatherSeries(t, registry)

	sibling, err := parent.NewSibling()
	require.NoError(t, err)
	server := connectSiblingScripted(t, sibling)

	// Connect starts the poller's goroutine before it returns, and a goroutine started through
	// startBackground has that function's closure on its stack from its creation, so this scan is
	// ordered after any start. A poller never ends on its own, so one started stays above the
	// baseline until the bound.
	if err := natsfixture.AwaitNoStacks(t.Context(), lifecycleBound,
		noMoreClientWork(baseline, clientWorkFrames...)); err != nil {
		t.Fatalf("a Client goroutine started for the sibling: %v", err)
	}
	// Connect admits the poller before it returns, so this read is ordered after any admission.
	sibling.mu.RLock()
	siblingPollers := sibling.running[workMetricsPoller]
	sibling.mu.RUnlock()
	require.Zero(t, siblingPollers, "no metrics poller is admitted for the sibling")

	res := subscribeAsync(t.Context(), sibling, "refusal.sibling", noopHandler)
	await(t, server.pinged, "the sibling's server reading the subscription's PING")
	server.write(`-ERR 'Permissions Violation for Subscription to "refusal.sibling"'` + "\r\n")
	server.answerPings()
	r := awaitValue(t, res, "the subscription returning after the PONG")
	require.NoError(t, r.err)

	// The handler reads the registry while the sibling writes its NATS error record.
	read := awaitValue(t, logs.seen, "the sibling's NATS error record")
	require.NoError(t, read.err)
	require.Empty(t, read.counts, "a refusal on the sibling is not counted")
	require.Equal(t, seriesBefore, gatherSeries(t, registry), "the registry holds only the parent's series")

	require.Nil(t, sibling.jsMetrics, "a sibling holds no metrics")
	require.Nil(t, sibling.onConnectionLost, "a sibling holds no connection-lost callback")
	require.Zero(t, sibling.connectionLossTimeout, "a sibling holds no connection-loss timeout")
}
