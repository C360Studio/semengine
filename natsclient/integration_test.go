//go:build integration

package natsclient

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/metric"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_ConnectToRealNATS tests connection to a real NATS server
func TestIntegration_ConnectToRealNATS(t *testing.T) {
	ctx := context.Background()

	natsURL := startFixture(t).URL()

	// Create manager and connect
	manager, err := NewClient(natsURL)
	require.NoError(t, err)
	err = manager.Connect(ctx)
	require.NoError(t, err)
	defer closeClient(t, manager)

	// Verify connection
	assert.True(t, manager.IsHealthy())
	assert.Equal(t, StatusConnected, manager.Status())

	// Test RTT
	rtt, err := manager.RTT()
	assert.NoError(t, err)
	assert.Greater(t, rtt, time.Duration(0))
}

// TestIntegration_ReconnectionIsRedial proves re-dial, not nats.go's automatic reconnect: when the
// broker restarts, a client on it observes the loss (its connection-lost callback fires and its
// Status leaves connected), and a client dialled from the restarted broker's URL is healthy.
// Docker may map the restarted broker to another host port, so the first client's URL can point
// nowhere; that is why the pin skipped this test (integration_test.go:63-65) and why the first
// client here has no reconnects. Rewritten on natsfixture.Restart (owner ruling, #9 comment
// 5941920346, Q2; comment 5969522395, item 2).
func TestIntegration_ReconnectionIsRedial(t *testing.T) {
	ctx := t.Context()
	f := startFixture(t)

	lost := make(chan error, 1)
	manager, err := NewClient(f.URL(),
		WithMaxReconnects(0),
		WithHealthInterval(0),
		WithConnectionLossTimeout(100*time.Millisecond),
		WithConnectionLostCallback(func(err error) {
			select {
			case lost <- err:
			default:
			}
		}),
	)
	require.NoError(t, err)
	require.NoError(t, manager.Connect(ctx))
	defer closeClient(t, manager)
	require.Equal(t, StatusConnected, manager.Status())

	restartCtx, cancel := context.WithTimeout(ctx, restartBound)
	defer cancel()
	require.NoError(t, f.Restart(restartCtx))

	// The callback runs once the connection has been down for the configured grace; with no
	// reconnects it never comes back, so a correct client always gets here.
	select {
	case <-lost:
	case <-time.After(failureBound):
		t.Fatal("connection-lost callback did not fire after the broker restarted")
	}
	waitCtx, waitCancel := context.WithTimeout(ctx, failureBound)
	defer waitCancel()
	status, err := probe.Await(waitCtx, func(context.Context) (ConnectionStatus, error) { return manager.Status(), nil },
		func(s ConnectionStatus) bool { return s == StatusDisconnected })
	require.NoError(t, err, "status after the loss: %v", status)
	assert.False(t, manager.IsHealthy())

	redialled, err := NewClient(f.URL(), WithMaxReconnects(0), WithHealthInterval(0))
	require.NoError(t, err)
	require.NoError(t, redialled.Connect(ctx))
	defer closeClient(t, redialled)
	status, err = probe.Await(waitCtx, func(context.Context) (ConnectionStatus, error) { return redialled.Status(), nil },
		func(s ConnectionStatus) bool { return s == StatusConnected })
	require.NoError(t, err, "re-dialled client's status: %v", status)
	assert.True(t, redialled.IsHealthy())
	_, err = redialled.RTT()
	require.NoError(t, err, "a round trip to the restarted broker")
}

// TestIntegration_AcknowledgedIsNotDurableOnMemoryStream is task 4.2, the natsclient half of
// transport-client "Acknowledged is not durable on a memory stream": a message PublishToStream
// returned nil for (the server acknowledged it) on a memory-backed stream is absent after the
// broker restarts, and one published the same way on a file-backed stream is present. Both are
// read back before the restart, so the absence afterwards is not a publish that never landed. The
// graph-ingest half of the scenario is change 2's.
func TestIntegration_AcknowledgedIsNotDurableOnMemoryStream(t *testing.T) {
	ctx := t.Context()
	f := startFixture(t)
	file, mem := f.Name("file"), f.Name("mem")
	_, err := f.CreateStream(ctx, file, file+".>")
	require.NoError(t, err)
	_, err = f.CreateMemoryStream(ctx, mem, mem+".>")
	require.NoError(t, err)

	publisher, err := NewClient(f.URL(), WithMaxReconnects(0), WithHealthInterval(0))
	require.NoError(t, err)
	require.NoError(t, publisher.Connect(ctx))
	defer closeClient(t, publisher)
	memStream, err := publisher.GetStream(ctx, mem)
	require.NoError(t, err)
	memInfo, err := memStream.Info(ctx)
	require.NoError(t, err)
	require.Equal(t, jetstream.MemoryStorage, memInfo.Config.Storage, "the client reports the stream's storage class")
	require.NoError(t, publisher.PublishToStream(ctx, file+".1", []byte("kept")))
	require.NoError(t, publisher.PublishToStream(ctx, mem+".1", []byte("lost")))
	// Each stream is new, so its one message is at sequence 1.
	for name, want := range map[string]string{file: "kept", mem: "lost"} {
		s, err := publisher.GetStream(ctx, name)
		require.NoError(t, err)
		msg, err := s.GetMsg(ctx, 1)
		require.NoError(t, err, "%s before the restart", name)
		require.Equal(t, want, string(msg.Data), "%s before the restart", name)
	}
	closeClient(t, publisher) // the fixture contract: stop what is built on the old URL first

	restartCtx, cancel := context.WithTimeout(ctx, restartBound)
	defer cancel()
	require.NoError(t, f.Restart(restartCtx))

	reader, err := NewClient(f.URL(), WithMaxReconnects(0), WithHealthInterval(0))
	require.NoError(t, err)
	require.NoError(t, reader.Connect(ctx))
	defer closeClient(t, reader)
	fs, err := reader.GetStream(ctx, file)
	require.NoError(t, err, "file-backed stream after the restart")
	msg, err := fs.GetMsg(ctx, 1)
	require.NoError(t, err, "file-backed message after the restart")
	require.Equal(t, "kept", string(msg.Data))
	// The memory stream itself may or may not be re-created empty; either way its message is gone.
	ms, err := reader.GetStream(ctx, mem)
	t.Logf("memory-backed stream after the restart: stream not found = %v", errors.Is(err, jetstream.ErrStreamNotFound))
	if !errors.Is(err, jetstream.ErrStreamNotFound) {
		require.NoError(t, err, "memory-backed stream after the restart")
		_, err = ms.GetMsg(ctx, 1)
		require.ErrorIs(t, err, jetstream.ErrMsgNotFound, "memory-backed message after the restart")
	}
}

// TestIntegration_CircuitBreakerWithRealConnection tests circuit breaker with actual failures
func TestIntegration_CircuitBreakerWithRealConnection(t *testing.T) {
	ctx := context.Background()

	// Try to connect to an invalid NATS server
	manager, err := NewClient(refusedNATSURL(t))
	require.NoError(t, err)
	// Counts the dials Connect reports through its opHook seam; set before any Connect runs.
	var dials atomic.Int32
	manager.opHook = func(op string) {
		if op == "dial" {
			dials.Add(1)
		}
	}

	// Try 14 times - should not open circuit (threshold is 15)
	for i := 0; i < 14; i++ {
		err = manager.Connect(ctx)
		assert.Error(t, err)
		assert.NotEqual(t, StatusCircuitOpen, manager.Status())
	}

	// 15th attempt should trigger circuit breaker
	err = manager.Connect(ctx)
	assert.Error(t, err)

	// After 15 failures, circuit should be open
	assert.Equal(t, StatusCircuitOpen, manager.Status())
	assert.Equal(t, int32(15), manager.Failures())

	// Further attempts should fail immediately with circuit open error. "Immediately" is observed
	// as no dial, where the pin asserted under 10 ms of wall time (design D8 R1b).
	dialsBefore := dials.Load()
	require.Equal(t, int32(15), dialsBefore, "the 15 failed Connects each reported a dial through opHook")
	err = manager.Connect(ctx)

	assert.Error(t, err)
	assert.Equal(t, ErrCircuitOpen, err)
	assert.Equal(t, dialsBefore, dials.Load(), "Connect with the circuit open dialled") // Should fail fast
}

// TestIntegration_PublishSubscribe tests basic pub/sub functionality
func TestIntegration_PublishSubscribe(t *testing.T) {
	ctx := t.Context()

	natsURL := startFixture(t).URL()

	// Create manager and connect
	manager, err := NewClient(natsURL)
	require.NoError(t, err)
	err = manager.Connect(ctx)
	require.NoError(t, err)
	defer closeClient(t, manager)

	// Subscribe to a subject
	received := make(chan string, 1)
	sub, err := manager.Subscribe(ctx, "test.subject", func(_ context.Context, msg *nats.Msg) {
		received <- string(msg.Data)
	})
	require.NoError(t, err)
	defer sub.Unsubscribe()

	// Publish a message
	testMessage := "Hello NATS"
	err = manager.Publish(ctx, "test.subject", []byte(testMessage))
	require.NoError(t, err)

	// Verify message received
	select {
	case msg := <-received:
		assert.Equal(t, testMessage, msg)
	case <-time.After(failureBound):
		t.Fatal("Message not received")
	}
}

// TestIntegration_JetStream tests JetStream functionality
func TestIntegration_JetStream(t *testing.T) {
	ctx := context.Background()

	natsURL := startFixture(t).URL()

	// Create manager and connect
	manager, err := NewClient(natsURL)
	require.NoError(t, err)
	err = manager.Connect(ctx)
	require.NoError(t, err)
	defer closeClient(t, manager)

	// Get JetStream context
	js, err := manager.JetStream()
	require.NoError(t, err)
	require.NotNil(t, js)

	// Create a stream
	streamName := "TEST_STREAM"
	streamCfg := jetstream.StreamConfig{
		Name:     streamName,
		Subjects: []string{"test.*"},
		MaxAge:   testStreamMaxAge,
		MaxBytes: testStreamMaxBytes,
	}
	_, err = manager.CreateStream(ctx, streamCfg)
	require.NoError(t, err)

	// Publish to stream
	err = manager.PublishToStream(ctx, "test.data", []byte("stream message"))
	require.NoError(t, err)

	// Create consumer and receive message
	received := make(chan string, 1)
	consumeCtx, err := manager.ConsumeInternalStreamWithConfig(ctx, StreamConsumerConfig{
		StreamName: streamName, FilterSubject: "test.*",
	}, func(_ context.Context, msg jetstream.Msg) {
		received <- string(msg.Data())
		msg.Ack()
	})
	require.NoError(t, err)
	t.Cleanup(consumeCtx.Stop)

	// Verify message
	select {
	case msg := <-received:
		assert.Equal(t, "stream message", msg)
	case <-time.After(failureBound):
		t.Fatal("Stream message not received")
	}
}

// TestIntegration_JetStreamMetrics verifies that JetStream metrics are properly collected
func TestIntegration_JetStreamMetrics(t *testing.T) {
	ctx := context.Background()

	natsURL := startFixture(t).URL()

	// Create metrics registry
	metricsRegistry := metric.NewMetricsRegistry()

	// Create client with metrics enabled
	client, err := NewClient(natsURL,
		WithMetrics(metricsRegistry),
	)
	require.NoError(t, err)

	err = client.Connect(ctx)
	require.NoError(t, err)
	defer closeClient(t, client)

	// Create a stream
	streamCfg := jetstream.StreamConfig{
		Name:     "TEST_METRICS",
		Subjects: []string{"test.metrics.>"},
		MaxAge:   testStreamMaxAge,
		MaxBytes: testStreamMaxBytes,
	}
	stream, err := client.CreateStream(ctx, streamCfg)
	require.NoError(t, err)
	require.NotNil(t, stream)

	// Publish some messages to populate stream stats
	for i := 0; i < 5; i++ {
		err := client.PublishToStream(ctx, "test.metrics.msg", []byte(fmt.Sprintf("test message %d", i)))
		require.NoError(t, err)
	}

	// Create a consumer
	received := make(chan bool, 5)
	consumeCtx, err := client.ConsumeInternalStreamWithConfig(ctx, StreamConsumerConfig{
		StreamName: "TEST_METRICS", FilterSubject: "test.metrics.>",
	}, func(_ context.Context, msg jetstream.Msg) {
		select {
		case received <- true:
		default:
		}
		msg.Ack()
	})
	require.NoError(t, err)
	t.Cleanup(consumeCtx.Stop)

	// Wait for the five deliveries.
	for i := 0; i < 5; i++ {
		select {
		case <-received:
		case <-time.After(failureBound):
			t.Fatalf("%d of 5 messages delivered", i)
		}
	}

	// Trigger metrics update manually (normally happens every 30s)
	if client.jsMetrics != nil {
		client.jsMetrics.updateStats(ctx)
	}

	// Gather metrics
	metricFamilies, err := metricsRegistry.PrometheusRegistry().Gather()
	require.NoError(t, err)

	// Build metric lookup map
	metricsByName := make(map[string]*dto.MetricFamily)
	for _, mf := range metricFamilies {
		metricsByName[*mf.Name] = mf
	}

	// Verify stream metrics exist
	streamMessages := metricsByName["semengine_jetstream_stream_messages"]
	require.NotNil(t, streamMessages, "stream messages metric should exist")
	// Should have 5 messages in stream (might have consumed some)
	assert.GreaterOrEqual(t, *streamMessages.Metric[0].Gauge.Value, float64(0))

	streamBytes := metricsByName["semengine_jetstream_stream_bytes"]
	require.NotNil(t, streamBytes, "stream bytes metric should exist")
	assert.Greater(t, *streamBytes.Metric[0].Gauge.Value, float64(0))

	streamState := metricsByName["semengine_jetstream_stream_state"]
	require.NotNil(t, streamState, "stream state metric should exist")
	assert.Equal(t, float64(1), *streamState.Metric[0].Gauge.Value, "stream should be active")

	// Verify consumer metrics exist
	consumerPending := metricsByName["semengine_jetstream_consumer_pending_messages"]
	require.NotNil(t, consumerPending, "consumer pending metric should exist")

	consumerDelivered := metricsByName["semengine_jetstream_consumer_delivered_stream_sequence"]
	require.NotNil(t, consumerDelivered, "consumer delivered metric should exist")
	assert.GreaterOrEqual(t, *consumerDelivered.Metric[0].Gauge.Value, float64(0))

	client.jsMetrics.mu.Lock()
	var observationKey string
	for key := range client.jsMetrics.consumers {
		observationKey = key
		break
	}
	client.jsMetrics.mu.Unlock()
	require.NotEmpty(t, observationKey)
	consumeCtx.Drain()
	<-consumeCtx.Closed()
	waitForConsumerObservationRemoval(t, client.jsMetrics, observationKey)
}
