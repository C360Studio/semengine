//go:build integration

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/natsclient"
)

// The message logger's subscriptions against the package's shared NATS server (design D2, D3;
// D7 U-c). Each test uses one fixture client both to publish and, through the logger, to
// subscribe: a SUB the logger sends reaches the server before any later PUB on the same
// connection, so a publish after Start needs no wait for the subscription. Counts are read after
// a Stop that returned nil: the publishing connection is flushed first, so the server has sent
// every message to the connection, and Stop drains each subscription, which handles every message
// already delivered to it. A count read then is final, with no window to wait out.

// messageSample is the Debug record the logger writes after it stores a message (pin
// message_logger.go:1001, :1027); holdingHandler holds the first one.
const messageSample = "Message sample"

// subscriptionStats is the part of GetStatistics these tests read, decoded from its JSON as the
// /stats route serves it, so the expected values are spelled here and not read back from the
// logger's own types.
type subscriptionStats struct {
	TotalMessages     int64    `json:"total_messages"`
	MonitoredSubjects []string `json:"monitored_subjects"`
	SubjectOverlaps   []struct {
		Broader    string `json:"broader"`
		Covered    string `json:"covered"`
		Resolution string `json:"resolution"`
	} `json:"subject_overlaps"`
	Degraded       bool   `json:"subject_reconciliation_degraded"`
	DegradedReason string `json:"subject_reconciliation_error"`
}

func readSubscriptionStats(t *testing.T, ml *MessageLogger) subscriptionStats {
	t.Helper()
	raw, err := json.Marshal(ml.GetStatistics())
	require.NoError(t, err)
	var stats subscriptionStats
	require.NoError(t, json.Unmarshal(raw, &stats))
	return stats
}

// capturedMessage is one stored entry as a test compares it: the subject and the payload bytes.
type capturedMessage struct {
	Subject string
	Payload string
}

// capturedMessages lists the stored entries oldest first (GetLogEntries returns newest first).
func capturedMessages(ml *MessageLogger) []capturedMessage {
	entries := ml.GetLogEntries(0)
	out := make([]capturedMessage, 0, len(entries))
	for i := len(entries) - 1; i >= 0; i-- {
		out = append(out, capturedMessage{Subject: entries[i].Subject, Payload: string(entries[i].RawData)})
	}
	return out
}

// startSubscriptionLogger builds a logger over client from config and starts it under a bounded
// context. A Stop under fresh bounded authority is registered as the test's cleanup, before the
// Start, so a logger a failed assertion leaves running is stopped; a Stop after one that returned
// nil returns nil (D2).
func startSubscriptionLogger(t *testing.T, client *natsclient.Client, config string, logger *slog.Logger) *MessageLogger {
	t.Helper()
	ml := newTestMessageLogger(t, client, config, logger)
	stopLoggerOnCleanup(t, ml)
	startCtx, cancel := context.WithTimeout(t.Context(), stopBound)
	defer cancel()
	require.NoError(t, ml.Start(startCtx))
	return ml
}

func stopLoggerOnCleanup(t *testing.T, ml *MessageLogger) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), stopBound)
		defer cancel()
		if err := ml.Stop(ctx); err != nil {
			t.Errorf("cleanup Stop: %v", err)
		}
	})
}

// publishAndFlush publishes each payload to its subject and flushes the connection, so the server
// has processed every publish, and sent its deliveries to this connection, when it returns.
func publishAndFlush(t *testing.T, client *natsclient.Client, messages ...capturedMessage) {
	t.Helper()
	for _, m := range messages {
		require.NoError(t, client.Publish(t.Context(), m.Subject, []byte(m.Payload)))
	}
	require.NoError(t, client.GetConnection().Flush())
}

func stopLogger(t *testing.T, ml *MessageLogger) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), stopBound)
	defer cancel()
	return ml.Stop(ctx)
}

// awaitSignal waits for ch under the test's context, bounded at stopBound.
func awaitSignal(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	timer := time.NewTimer(stopBound)
	defer timer.Stop()
	select {
	case <-ch:
	case <-timer.C:
		t.Fatalf("%s: not within %s", what, stopBound)
	case <-t.Context().Done():
		t.Fatalf("%s: test context ended", what)
	}
}

// TestMessageLoggerMonitorsOnlyConfiguredSubjects carries the pin's explicit mode
// (message_logger_registry_test.go:19-29, subscription_integration_test.go:17-56) onto the shared
// server: with explicit monitor_subjects the logger monitors exactly those, sorted and
// deduplicated, and stores what is published on them and nothing else.
func TestMessageLoggerMonitorsOnlyConfiguredSubjects(t *testing.T) {
	client := fixtureClient(t)
	ml := startSubscriptionLogger(t, client,
		`{"monitor_subjects":["logger.explicit.b","logger.explicit.a","logger.explicit.b"]}`, nil)

	stats := readSubscriptionStats(t, ml)
	require.Equal(t, []string{"logger.explicit.a", "logger.explicit.b"}, stats.MonitoredSubjects)
	require.Empty(t, stats.SubjectOverlaps)
	require.False(t, stats.Degraded, "a normal boot is not degraded: %q", stats.DegradedReason)

	want := []capturedMessage{
		{Subject: "logger.explicit.a", Payload: `{"n":1}`},
		{Subject: "logger.explicit.b", Payload: `{"n":2}`},
	}
	publishAndFlush(t, client,
		want[0],
		capturedMessage{Subject: "logger.unmonitored", Payload: `{"n":0}`},
		want[1],
	)
	require.NoError(t, stopLogger(t, ml))

	require.Equal(t, want, capturedMessages(ml))
	require.Equal(t, int64(2), readSubscriptionStats(t, ml).TotalMessages)
}

// TestMessageLoggerKeepsCapturingAfterStartContextEnds is D2's "Ending the Start context after
// Start returned stops nothing": the Start context is cancelled once Start returns, and a message
// published afterwards is stored and counted. natsclient makes a subscription's context the parent
// of every message's context (natsclient/client.go:1036-1047) and the logger drops a message whose
// context has ended (pin message_logger.go:945-951), so a logger that subscribes under the Start
// context, or derives its run context from it, stores nothing here.
func TestMessageLoggerKeepsCapturingAfterStartContextEnds(t *testing.T) {
	client := fixtureClient(t)
	ml := newTestMessageLogger(t, client, `{"monitor_subjects":["logger.after-start"]}`, nil)
	stopLoggerOnCleanup(t, ml)

	startCtx, cancelStart := context.WithCancel(t.Context())
	require.NoError(t, ml.Start(startCtx))
	cancelStart()

	want := capturedMessage{Subject: "logger.after-start", Payload: `{"phase":"running"}`}
	publishAndFlush(t, client, want)
	stopErr := stopLogger(t, ml)

	require.Equal(t, []capturedMessage{want}, capturedMessages(ml),
		"a message published after the Start context ended must be stored")
	require.Equal(t, int64(1), readSubscriptionStats(t, ml).TotalMessages)
	require.NoError(t, stopErr)
}

// TestMessageLoggerStopDrainsInFlightMessage carries pin message_logger_lifecycle_test.go:102-145
// (D2, Stop step 2): a message being handled when Stop begins, and one queued behind it, are both
// stored before Stop returns nil. The handler holds message 1's "Message sample" record, which the
// logger writes after storing it, so message 1's callback has not returned; message 2 is published
// and flushed behind it, then Stop begins. Message 2 reaching the handler during the drain is
// measured, not promised (design D2, drain probe: 200 of 200 runs at nats.go v1.54.0). A Stop that
// ends the run context before it drains drops message 2: the logger drops a message whose context
// has ended.
func TestMessageLoggerStopDrainsInFlightMessage(t *testing.T) {
	client := fixtureClient(t)
	handler := newHoldingHandler(t, messageSample)
	ml := startSubscriptionLogger(t, client, `{"monitor_subjects":["logger.drain"]}`, slog.New(handler))

	first := capturedMessage{Subject: "logger.drain", Payload: `{"n":1}`}
	second := capturedMessage{Subject: "logger.drain", Payload: `{"n":2}`}
	publishAndFlush(t, client, first)
	awaitSignal(t, handler.heldRecord(), "message 1's sample record held")
	publishAndFlush(t, client, second)

	stopCtx, cancel := context.WithTimeout(t.Context(), stopBound)
	defer cancel()
	observed := probe.Observe(stopCtx)
	stopResult := make(chan error, 1)
	go func() { stopResult <- ml.Stop(observed) }()
	// Stop waits on its context while the held callback keeps the drain from finishing.
	awaitSignal(t, observed.Observed(), "Stop waiting on its context")
	select {
	case err := <-stopResult:
		t.Fatalf("Stop returned while message 1's callback was held: %v", err)
	default:
	}

	handler.releaseHeld()
	require.NoError(t, <-stopResult, "Stop returns nil once the drain has handled every delivered message")
	require.Equal(t, []capturedMessage{first, second}, capturedMessages(ml))
	require.Equal(t, int64(2), readSubscriptionStats(t, ml).TotalMessages)
}

// TestMessageLoggerFailedStopIsRetried is D2's "A failed Stop is retried" (M3), flipping pin
// message_logger_subscription_integration_test.go:62-63 and lifecycle_test.go:147-178, where a
// failed Stop was terminal and forgot its subscriptions.
//
// The two 200 ms windows are the only real-clock windows in these tests (design D7, as
// docs/testing.md § Porting a test from SemStreams requires). The first Stop's deadline cannot fail
// a correct logger: the held callback keeps the drain from finishing, so it can only end at the
// deadline. The second window is an observation: under a logger that forgets its subscriptions
// after a failed Stop, the second Stop returns nil at once; a correct one waits for the drain,
// which waits for the held callback (drain probe: a second Drain returned while the callback was
// held in 0 of 200 runs). If the first Stop's deadline ends before it reached its drain, that
// mutant passes the run; such a pass is inconclusive, not a survivor.
func TestMessageLoggerFailedStopIsRetried(t *testing.T) {
	const window = 200 * time.Millisecond
	client := fixtureClient(t)
	handler := newHoldingHandler(t, messageSample)
	ml := startSubscriptionLogger(t, client, `{"monitor_subjects":["logger.retry-stop"]}`, slog.New(handler))

	held := capturedMessage{Subject: "logger.retry-stop", Payload: `{"n":1}`}
	publishAndFlush(t, client, held)
	awaitSignal(t, handler.heldRecord(), "the sample record held")

	firstCtx, cancelFirst := context.WithTimeout(t.Context(), window)
	defer cancelFirst()
	require.ErrorIs(t, ml.Stop(firstCtx), context.DeadlineExceeded,
		"a Stop whose drain cannot finish returns its context's error")

	secondCtx, cancelSecond := context.WithTimeout(t.Context(), stopBound)
	defer cancelSecond()
	secondResult := make(chan error, 1)
	go func() { secondResult <- ml.Stop(secondCtx) }()
	observation := time.NewTimer(window)
	defer observation.Stop()
	select {
	case err := <-secondResult:
		t.Fatalf("the second Stop returned (%v) while the callback was held: the failed Stop forgot its subscription", err)
	case <-observation.C:
	}

	handler.releaseHeld()
	require.NoError(t, <-secondResult, "the second Stop finishes the cleanup the first left")
	require.NoError(t, stopLogger(t, ml), "a Stop after one that returned nil returns nil")
	require.Equal(t, []capturedMessage{held}, capturedMessages(ml))
}

// TestMessageLoggerIgnoresPublishesAfterStop carries pin
// message_logger_subscription_integration_test.go:59-63: after a Stop that returned nil, a
// message published on a monitored subject is not stored or counted. A plain subscription on the
// same connection receives that message, so the server did deliver it to the connection the logger
// subscribed on; the logger, having unsubscribed, does not see it.
func TestMessageLoggerIgnoresPublishesAfterStop(t *testing.T) {
	client := fixtureClient(t)
	ml := startSubscriptionLogger(t, client, `{"monitor_subjects":["logger.after-stop"]}`, nil)

	before := capturedMessage{Subject: "logger.after-stop", Payload: `{"phase":"running"}`}
	publishAndFlush(t, client, before)
	require.NoError(t, stopLogger(t, ml))

	control, err := client.GetConnection().SubscribeSync("logger.after-stop")
	require.NoError(t, err)
	t.Cleanup(func() { _ = control.Unsubscribe() })
	publishAndFlush(t, client, capturedMessage{Subject: "logger.after-stop", Payload: `{"phase":"stopped"}`})
	delivered, err := control.NextMsg(stopBound)
	require.NoError(t, err, "the server delivers the later publish to the connection")
	require.JSONEq(t, `{"phase":"stopped"}`, string(delivered.Data))

	require.Equal(t, []capturedMessage{before}, capturedMessages(ml), "a stopped logger stores no later publish")
	require.Equal(t, int64(1), readSubscriptionStats(t, ml).TotalMessages)
}

// TestMessageLoggerCapturesCoveredSubjectOnce is D3's covered-subject rule on the wire, replacing
// pin message_logger_registry_test.go:31-56: a desired subject another desired subject covers is
// not subscribed, the omission is reported in subject_overlaps, and a message on it is captured
// once. A logger that also subscribed the covered subject would store the message on
// logger.cover.x twice, once per subscription.
func TestMessageLoggerCapturesCoveredSubjectOnce(t *testing.T) {
	client := fixtureClient(t)
	ml := startSubscriptionLogger(t, client,
		`{"monitor_subjects":["logger.cover.*","logger.cover.>","logger.cover.>"]}`, nil)

	stats := readSubscriptionStats(t, ml)
	require.Equal(t, []string{"logger.cover.>"}, stats.MonitoredSubjects)
	require.Len(t, stats.SubjectOverlaps, 1)
	require.Equal(t, "logger.cover.>", stats.SubjectOverlaps[0].Broader)
	require.Equal(t, "logger.cover.*", stats.SubjectOverlaps[0].Covered)
	require.Equal(t, "covered subscription omitted", stats.SubjectOverlaps[0].Resolution)

	want := []capturedMessage{
		{Subject: "logger.cover.x", Payload: `{"n":1}`},
		{Subject: "logger.cover.x.y", Payload: `{"n":2}`},
	}
	publishAndFlush(t, client, want...)
	require.NoError(t, stopLogger(t, ml))

	require.Equal(t, want, capturedMessages(ml))
	require.Equal(t, int64(2), readSubscriptionStats(t, ml).TotalMessages)
}

// TestMessageLoggerRetriesSubscribeUntilConnected is D2's "Retry, declared", replacing pin
// message_logger_registry_test.go:58-73 (a subscribe that fails is retried and the degraded state
// clears) without its injected subscriber and retry clock. The logger starts on a client built for
// the shared server's URL but not yet connected, so every subscribe fails with ErrNotConnected
// (natsclient/client.go:1074-1078), the state a NATS outage during boot leaves. Start returns nil;
// /stats reports the subject unsubscribed and the logger's health check, run through the base
// service's production path, turns it unhealthy. Once the client connects, the retry subscribes:
// the degraded state clears, the logger is healthy again, and a message published then is stored.
// A logger whose health check is not wired stays healthy through the outage and fails here.
func TestMessageLoggerRetriesSubscribeUntilConnected(t *testing.T) {
	url := shared.Reset(t)
	client, err := natsclient.NewClient(url,
		natsclient.WithTimeout(clientTimeout),
		natsclient.WithMaxReconnects(0),
		natsclient.WithHealthInterval(0),
	)
	require.NoError(t, err)
	// Registered after the reset, so it runs before the reset's check that no connection is left.
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), stopBound)
		defer cancel()
		if err := client.Close(ctx); err != nil {
			t.Errorf("cleanup Close: %v", err)
		}
	})

	const subject = "logger.until-connected"
	ml := startSubscriptionLogger(t, client, `{"monitor_subjects":["`+subject+`"]}`, nil)

	stats := readSubscriptionStats(t, ml)
	require.True(t, stats.Degraded, "a subject that failed to subscribe is reported")
	require.NotEmpty(t, stats.DegradedReason)
	ml.performHealthCheck()
	require.False(t, ml.IsHealthy(), "a logger holding an unsubscribed subject is unhealthy")

	connectCtx, cancelConnect := context.WithTimeout(t.Context(), stopBound)
	defer cancelConnect()
	require.NoError(t, client.Connect(connectCtx))
	require.NoError(t, client.WaitForConnection(connectCtx))

	// The retry runs on its own every 100 ms and signals nothing a caller can wait on (no seam is
	// added for a test, #186), so the test polls the two production reads under a bounded context.
	recovered, err := probe.Await(connectCtx, func(context.Context) (subscriptionStats, error) {
		ml.performHealthCheck()
		s := readSubscriptionStats(t, ml)
		if s.Degraded || !ml.IsHealthy() {
			return s, fmt.Errorf("degraded=%t (%q) healthy=%t", s.Degraded, s.DegradedReason, ml.IsHealthy())
		}
		return s, nil
	}, func(subscriptionStats) bool { return true })
	require.NoError(t, err, "the retry subscribes once the client connects")
	require.Equal(t, []string{subject}, recovered.MonitoredSubjects)

	want := capturedMessage{Subject: subject, Payload: `{"phase":"connected"}`}
	publishAndFlush(t, client, want)
	require.NoError(t, stopLogger(t, ml))
	require.Equal(t, []capturedMessage{want}, capturedMessages(ml))
	require.Empty(t, (&messageLoggerOwner{ml: ml}).Observe().Unresolved,
		"a Stop that returned nil leaves no retry, subscription or base service running")
}
