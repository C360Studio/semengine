//go:build integration

package natsclient

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_ConsumeStreamWithConfigContexts_CancelledSetupDoesNotStartConsumer(t *testing.T) {
	ctx := context.Background()
	natsURL := startFixture(t).URL()

	client, err := NewClient(natsURL)
	require.NoError(t, err)
	require.NoError(t, client.Connect(ctx))
	defer closeClient(t, client)

	_, err = client.EnsureStream(ctx, jetstream.StreamConfig{
		Name:     "CONTEXT_SPLIT_STREAM",
		Subjects: []string{"context.split.>"},
		Storage:  jetstream.MemoryStorage,
		MaxAge:   testStreamMaxAge,
		MaxBytes: testStreamMaxBytes,
	})
	require.NoError(t, err)

	setupCtx, cancelSetup := context.WithCancel(ctx)
	cancelSetup()
	handlerCtx, cancelHandler := context.WithCancel(ctx)
	defer cancelHandler()

	handle, err := client.ConsumeStreamWithConfigContexts(setupCtx, handlerCtx,
		PortConsumerContext{Component: "integration", Port: "input"}, StreamConsumerConfig{
			StreamName:    "CONTEXT_SPLIT_STREAM",
			ConsumerName:  "cancelled-setup",
			FilterSubject: "context.split.>",
			DeliverPolicy: "all",
			AckPolicy:     "explicit",
		}, func(context.Context, jetstream.Msg) {
			t.Error("handler ran despite cancelled consumer setup")
		})
	require.Nil(t, handle)
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled), "setup error = %v, want context cancellation", err)
	select {
	case <-handlerCtx.Done():
		t.Fatal("setup cancellation leaked into independent handler lifecycle")
	default:
	}

}

// TestIntegration_EnsureStream tests stream creation
func TestIntegration_EnsureStream(t *testing.T) {
	ctx := context.Background()

	// Start NATS container with JetStream
	natsURL := startFixture(t).URL()

	// Create and connect client
	client, err := NewClient(natsURL)
	require.NoError(t, err)
	err = client.Connect(ctx)
	require.NoError(t, err)
	defer closeClient(t, client)

	// Create stream
	cfg := jetstream.StreamConfig{
		Name:     "TEST_STREAM",
		Subjects: []string{"test.>"},
		Storage:  jetstream.MemoryStorage,
		MaxAge:   testStreamMaxAge,
		MaxBytes: testStreamMaxBytes,
	}

	stream, err := client.EnsureStream(ctx, cfg)
	require.NoError(t, err)
	assert.NotNil(t, stream)

	// Verify stream info
	info, err := stream.Info(ctx)
	require.NoError(t, err)
	assert.Equal(t, "TEST_STREAM", info.Config.Name)
	assert.Contains(t, info.Config.Subjects, "test.>")
}

// TestIntegration_EnsureStream_Existing tests that existing stream is returned
func TestIntegration_EnsureStream_Existing(t *testing.T) {
	ctx := context.Background()

	// Start NATS container with JetStream
	natsURL := startFixture(t).URL()

	// Create and connect client
	client, err := NewClient(natsURL)
	require.NoError(t, err)
	err = client.Connect(ctx)
	require.NoError(t, err)
	defer closeClient(t, client)

	cfg := jetstream.StreamConfig{
		Name:     "EXISTING_STREAM",
		Subjects: []string{"existing.>"},
		Storage:  jetstream.MemoryStorage,
		MaxAge:   testStreamMaxAge,
		MaxBytes: testStreamMaxBytes,
	}

	// Create stream first time
	stream1, err := client.EnsureStream(ctx, cfg)
	require.NoError(t, err)

	// Create stream second time - should return existing
	stream2, err := client.EnsureStream(ctx, cfg)
	require.NoError(t, err)

	// Both should reference the same stream
	info1, _ := stream1.Info(ctx)
	info2, _ := stream2.Info(ctx)
	assert.Equal(t, info1.Config.Name, info2.Config.Name)
}

// TestIntegration_EnsureStream_NotConnected tests EnsureStream when not connected
func TestIntegration_EnsureStream_NotConnected(t *testing.T) {
	client, err := NewClient("nats://unused")
	require.NoError(t, err)

	ctx := context.Background()
	cfg := jetstream.StreamConfig{
		Name:     "TEST_STREAM",
		Subjects: []string{"test.>"},
		MaxAge:   testStreamMaxAge,
		MaxBytes: testStreamMaxBytes,
	}

	_, err = client.EnsureStream(ctx, cfg)
	assert.Equal(t, ErrNotConnected, err)
}

// TestIntegration_ConsumeStreamWithConfig tests basic stream consumption
func TestIntegration_ConsumeStreamWithConfig(t *testing.T) {
	ctx := context.Background()

	// Start NATS container with JetStream
	natsURL := startFixture(t).URL()

	// Create and connect client
	client, err := NewClient(natsURL)
	require.NoError(t, err)
	err = client.Connect(ctx)
	require.NoError(t, err)
	defer closeClient(t, client)

	// Create stream first
	streamCfg := jetstream.StreamConfig{
		Name:     "CONSUME_STREAM",
		Subjects: []string{"consume.>"},
		Storage:  jetstream.MemoryStorage,
		MaxAge:   testStreamMaxAge,
		MaxBytes: testStreamMaxBytes,
	}
	_, err = client.EnsureStream(ctx, streamCfg)
	require.NoError(t, err)

	// Publish some messages
	js, err := client.JetStream()
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		_, err = js.Publish(ctx, "consume.test", []byte("message"))
		require.NoError(t, err)
	}

	// Set up consumer
	var received atomic.Int32
	var wg sync.WaitGroup
	wg.Add(5)

	cfg := StreamConsumerConfig{
		StreamName:    "CONSUME_STREAM",
		ConsumerName:  "test-consumer",
		FilterSubject: "consume.>",
		DeliverPolicy: "all",
		AckPolicy:     "explicit",
	}

	handle, err := client.ConsumeStreamWithConfig(ctx, PortConsumerContext{Component: "integration", Port: "input"}, cfg, func(_ context.Context, msg jetstream.Msg) {
		received.Add(1)
		msg.Ack()
		wg.Done()
	})
	require.NoError(t, err)
	defer drainNativeConsume(t, handle)

	// Wait for messages to be received
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(failureBound):
		t.Fatalf("Timeout waiting for messages, received %d/5", received.Load())
	}

	assert.Equal(t, int32(5), received.Load())
}

// TestIntegration_ConsumeStreamWithConfig_AutoCreate tests auto-creation of stream
func TestIntegration_ConsumeStreamWithConfig_AutoCreate(t *testing.T) {
	ctx := context.Background()

	// Start NATS container with JetStream
	natsURL := startFixture(t).URL()

	// Create and connect client
	client, err := NewClient(natsURL)
	require.NoError(t, err)
	err = client.Connect(ctx)
	require.NoError(t, err)
	defer closeClient(t, client)

	// Configure consumer with auto-create (stream doesn't exist yet)
	cfg := StreamConsumerConfig{
		StreamName:    "AUTO_STREAM",
		ConsumerName:  "auto-consumer",
		FilterSubject: "auto.test.*",
		DeliverPolicy: "new",
		AckPolicy:     "explicit",
		AutoCreate:    true,
		AutoCreateConfig: &StreamAutoCreateConfig{
			Subjects:  []string{"auto.test.>"},
			Storage:   "memory",
			Retention: "limits",
			// Auto-create is stream PROVISIONING, so it declares bounds like every
			// other creation seam. Without these the seam refuses the creation.
			MaxAge:   testStreamMaxAge,
			MaxBytes: testStreamMaxBytes,
		},
	}

	var received atomic.Int32
	handle, err := client.ConsumeStreamWithConfig(ctx, PortConsumerContext{Component: "integration", Port: "input"}, cfg, func(_ context.Context, msg jetstream.Msg) {
		received.Add(1)
		msg.Ack()
	})
	require.NoError(t, err)
	defer drainNativeConsume(t, handle)

	// Verify stream was created
	js, err := client.JetStream()
	require.NoError(t, err)

	stream, err := js.Stream(ctx, "AUTO_STREAM")
	require.NoError(t, err)
	assert.NotNil(t, stream)

	// Publish message and verify it's received
	_, err = js.Publish(ctx, "auto.test.msg", []byte("test"))
	require.NoError(t, err)

	// Wait for the message.
	awaitCount(t, &received, 1)
}

// TestIntegration_ConsumeStreamWithConfig_DeliverPolicies tests different deliver policies
func TestIntegration_ConsumeStreamWithConfig_DeliverPolicies(t *testing.T) {
	testCases := []struct {
		name          string
		deliverPolicy string
		publishBefore int
		publishAfter  int
		expectedMin   int32
		expectedMax   int32
	}{
		{
			name:          "deliver_all",
			deliverPolicy: "all",
			publishBefore: 3,
			publishAfter:  2,
			expectedMin:   5, // Should receive all
			expectedMax:   5,
		},
		{
			name:          "deliver_new",
			deliverPolicy: "new",
			publishBefore: 3,
			publishAfter:  2,
			expectedMin:   2, // Should receive only new
			expectedMax:   2,
		},
		{
			name:          "deliver_last",
			deliverPolicy: "last",
			publishBefore: 3,
			publishAfter:  2,
			expectedMin:   3, // Last before + 2 after
			expectedMax:   3,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			// Start NATS container with JetStream
			natsURL := startFixture(t).URL()

			// Create and connect client
			client, err := NewClient(natsURL)
			require.NoError(t, err)
			err = client.Connect(ctx)
			require.NoError(t, err)
			defer closeClient(t, client)

			streamName := "POLICY_" + tc.name
			subject := "policy." + tc.name + ".test"

			// Create stream
			streamCfg := jetstream.StreamConfig{
				Name:     streamName,
				Subjects: []string{"policy." + tc.name + ".>"},
				Storage:  jetstream.MemoryStorage,
				MaxAge:   testStreamMaxAge,
				MaxBytes: testStreamMaxBytes,
			}
			_, err = client.EnsureStream(ctx, streamCfg)
			require.NoError(t, err)

			// Publish messages before consumer
			js, err := client.JetStream()
			require.NoError(t, err)

			for i := 0; i < tc.publishBefore; i++ {
				_, err = js.Publish(ctx, subject, []byte("before"))
				require.NoError(t, err)
			}

			// Set up consumer
			var received atomic.Int32
			cfg := StreamConsumerConfig{
				StreamName:    streamName,
				ConsumerName:  tc.name + "-consumer",
				FilterSubject: "policy." + tc.name + ".>",
				DeliverPolicy: tc.deliverPolicy,
				AckPolicy:     "explicit",
			}

			handle, consumeErr := client.ConsumeStreamWithConfig(ctx, PortConsumerContext{Component: "integration", Port: "input"}, cfg, func(_ context.Context, msg jetstream.Msg) {
				received.Add(1)
				msg.Ack()
			})
			require.NoError(t, consumeErr)
			defer drainNativeConsume(t, handle)

			// The consumer exists on the server once ConsumeStreamWithConfig has
			// returned, so its deliver policy has already fixed its start: the pin's
			// 100 ms "ready" delay waited for nothing.

			// Publish messages after consumer
			for i := 0; i < tc.publishAfter; i++ {
				_, err = js.Publish(ctx, subject, []byte("after"))
				require.NoError(t, err)
			}

			// Processing is over once the server holds nothing for the consumer to
			// deliver or to wait for an ack on, and the handler has counted at least
			// the minimum. The pin waited 500 ms instead.
			consumer, err := js.Consumer(ctx, streamName, cfg.ConsumerName)
			require.NoError(t, err)
			awaitCtx, cancelAwait := context.WithTimeout(ctx, failureBound)
			defer cancelAwait()
			_, err = probe.Await(awaitCtx, func(ctx context.Context) (*jetstream.ConsumerInfo, error) {
				return consumer.Info(ctx)
			}, func(info *jetstream.ConsumerInfo) bool {
				return info.NumPending == 0 && info.NumAckPending == 0 && received.Load() >= tc.expectedMin
			})
			require.NoError(t, err, "consumer did not finish processing; received %d", received.Load())

			count := received.Load()
			assert.GreaterOrEqual(t, count, tc.expectedMin, "received fewer messages than expected")
			assert.LessOrEqual(t, count, tc.expectedMax, "received more messages than expected")
		})
	}
}

// TestIntegration_ConsumeStreamWithConfig_AckPolicies tests different ack policies
func TestIntegration_ConsumeStreamWithConfig_AckPolicies(t *testing.T) {
	ctx := context.Background()

	// Start NATS container with JetStream
	natsURL := startFixture(t).URL()

	// Create and connect client
	client, err := NewClient(natsURL)
	require.NoError(t, err)
	err = client.Connect(ctx)
	require.NoError(t, err)
	defer closeClient(t, client)

	// Create stream
	streamCfg := jetstream.StreamConfig{
		Name:     "ACK_STREAM",
		Subjects: []string{"ack.>"},
		Storage:  jetstream.MemoryStorage,
		MaxAge:   testStreamMaxAge,
		MaxBytes: testStreamMaxBytes,
	}
	_, err = client.EnsureStream(ctx, streamCfg)
	require.NoError(t, err)

	// Test explicit ack
	var received atomic.Int32
	cfg := StreamConsumerConfig{
		StreamName:    "ACK_STREAM",
		ConsumerName:  "explicit-consumer",
		FilterSubject: "ack.explicit",
		DeliverPolicy: "all",
		AckPolicy:     "explicit",
	}

	handle, err := client.ConsumeStreamWithConfig(ctx, PortConsumerContext{Component: "integration", Port: "input"}, cfg, func(_ context.Context, msg jetstream.Msg) {
		received.Add(1)
		// Explicitly ack
		msg.Ack()
	})
	require.NoError(t, err)
	defer drainNativeConsume(t, handle)

	// Publish and verify
	js, err := client.JetStream()
	require.NoError(t, err)

	_, err = js.Publish(ctx, "ack.explicit", []byte("test"))
	require.NoError(t, err)

	awaitCount(t, &received, 1)
	assert.Equal(t, int32(1), received.Load())
}

// TestIntegration_ConsumeStreamWithConfig_Nak tests message Nak behavior
func TestIntegration_ConsumeStreamWithConfig_Nak(t *testing.T) {
	ctx := context.Background()

	// Start NATS container with JetStream
	natsURL := startFixture(t).URL()

	// Create and connect client
	client, err := NewClient(natsURL)
	require.NoError(t, err)
	err = client.Connect(ctx)
	require.NoError(t, err)
	defer closeClient(t, client)

	// Create stream
	streamCfg := jetstream.StreamConfig{
		Name:     "NAK_STREAM",
		Subjects: []string{"nak.>"},
		Storage:  jetstream.MemoryStorage,
		MaxAge:   testStreamMaxAge,
		MaxBytes: testStreamMaxBytes,
	}
	_, err = client.EnsureStream(ctx, streamCfg)
	require.NoError(t, err)

	// Consumer that Naks first delivery, then Acks
	var deliveryCount atomic.Int32
	cfg := StreamConsumerConfig{
		StreamName:    "NAK_STREAM",
		ConsumerName:  "nak-consumer",
		FilterSubject: "nak.test",
		DeliverPolicy: "all",
		AckPolicy:     "explicit",
		MaxDeliver:    3,
		// Longer than awaitCount's bound, so an AckWait expiry cannot stand in for
		// the Nak: only the Nak redelivers in time.
		AckWait: 3 * failureBound,
	}

	handle, err := client.ConsumeStreamWithConfig(ctx, PortConsumerContext{Component: "integration", Port: "input"}, cfg, func(_ context.Context, msg jetstream.Msg) {
		count := deliveryCount.Add(1)
		if count == 1 {
			// First delivery - Nak for redelivery
			msg.Nak()
		} else {
			// Second delivery - Ack
			msg.Ack()
		}
	})
	require.NoError(t, err)
	defer drainNativeConsume(t, handle)

	// Publish message
	js, err := client.JetStream()
	require.NoError(t, err)

	_, err = js.Publish(ctx, "nak.test", []byte("test"))
	require.NoError(t, err)

	// Wait for the redelivery: delivered at least twice (Nak then Ack).
	awaitCount(t, &deliveryCount, 2)
}

// TestIntegration_ConsumeStreamWithConfig_MissingStreamName tests validation
func TestIntegration_ConsumeStreamWithConfig_MissingStreamName(t *testing.T) {
	ctx := context.Background()

	// Start NATS container with JetStream
	natsURL := startFixture(t).URL()

	// Create and connect client
	client, err := NewClient(natsURL)
	require.NoError(t, err)
	err = client.Connect(ctx)
	require.NoError(t, err)
	defer closeClient(t, client)

	cfg := StreamConsumerConfig{
		// StreamName intentionally omitted
		ConsumerName: "test-consumer",
	}

	handle, err := client.ConsumeStreamWithConfig(ctx, PortConsumerContext{Component: "integration", Port: "input"}, cfg, func(_ context.Context, _ jetstream.Msg) {})
	assert.Nil(t, handle)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "stream name is required")
}

// TestIntegration_ConsumeStreamWithConfig_NotConnected tests behavior when not connected
func TestIntegration_ConsumeStreamWithConfig_NotConnected(t *testing.T) {
	client, err := NewClient("nats://unused")
	require.NoError(t, err)

	ctx := context.Background()
	cfg := StreamConsumerConfig{
		StreamName:   "TEST_STREAM",
		ConsumerName: "test-consumer",
	}

	handle, err := client.ConsumeStreamWithConfig(ctx, PortConsumerContext{Component: "integration", Port: "input"}, cfg, func(_ context.Context, _ jetstream.Msg) {})
	assert.Nil(t, handle)
	assert.Equal(t, ErrNotConnected, err)
}

// TestIntegration_NativeHandleStopsConsumer tests stopping an exact owned consumer.
func TestIntegration_NativeHandleStopsConsumer(t *testing.T) {
	ctx := context.Background()

	// Start NATS container with JetStream
	natsURL := startFixture(t).URL()

	// Create and connect client
	client, err := NewClient(natsURL)
	require.NoError(t, err)
	err = client.Connect(ctx)
	require.NoError(t, err)
	defer closeClient(t, client)

	// Create stream
	streamCfg := jetstream.StreamConfig{
		Name:     "STOP_STREAM",
		Subjects: []string{"stop.>"},
		Storage:  jetstream.MemoryStorage,
		MaxAge:   testStreamMaxAge,
		MaxBytes: testStreamMaxBytes,
	}
	_, err = client.EnsureStream(ctx, streamCfg)
	require.NoError(t, err)

	// Start consumer
	cfg := StreamConsumerConfig{
		StreamName:    "STOP_STREAM",
		ConsumerName:  "stop-consumer",
		FilterSubject: "stop.test",
		DeliverPolicy: "all",
		AckPolicy:     "explicit",
	}

	owner := PortConsumerContext{Component: "integration", Port: "input"}
	ack := func(_ context.Context, msg jetstream.Msg) { msg.Ack() }
	handle, err := client.ConsumeStreamWithConfig(ctx, owner, cfg, ack)
	require.NoError(t, err)

	// While the handle runs the client holds the durable's local claim, so a second
	// consumer of it is refused.
	_, err = client.ConsumeStreamWithConfig(ctx, owner, cfg, ack)
	require.ErrorContains(t, err, "already has a local owner", "test premise: the running consumer holds its claim")

	drainNativeConsume(t, handle)

	// Draining the native handle is the caller's stop; the client's part is to see the
	// handle end and give the durable up, so the same durable can be consumed again.
	awaitCtx, cancel := context.WithTimeout(t.Context(), failureBound)
	defer cancel()
	claims, err := probe.Await(awaitCtx, func(context.Context) (int, error) { return claimCount(client), nil },
		func(n int) bool { return n == 0 })
	require.NoError(t, err, "the client still holds %d consumer claims after the native drain", claims)
	again, err := client.ConsumeStreamWithConfig(ctx, owner, cfg, ack)
	require.NoError(t, err, "the drained durable could not be consumed again")
	drainNativeConsume(t, again)
}

// TestIntegration_NativeHandlesStopMultipleConsumers tests stopping multiple exact owned consumers.
func TestIntegration_NativeHandlesStopMultipleConsumers(t *testing.T) {
	ctx := context.Background()

	// Start NATS container with JetStream
	natsURL := startFixture(t).URL()

	// Create and connect client
	client, err := NewClient(natsURL)
	require.NoError(t, err)
	err = client.Connect(ctx)
	require.NoError(t, err)
	defer closeClient(t, client)

	// Create stream
	streamCfg := jetstream.StreamConfig{
		Name:     "STOPALL_STREAM",
		Subjects: []string{"stopall.>"},
		Storage:  jetstream.MemoryStorage,
		MaxAge:   testStreamMaxAge,
		MaxBytes: testStreamMaxBytes,
	}
	_, err = client.EnsureStream(ctx, streamCfg)
	require.NoError(t, err)

	// Start multiple consumers
	handles := make([]jetstream.ConsumeContext, 0, 3)
	for i := 0; i < 3; i++ {
		cfg := StreamConsumerConfig{
			StreamName:    "STOPALL_STREAM",
			ConsumerName:  "stopall-consumer-" + string(rune('a'+i)),
			FilterSubject: "stopall.test",
			DeliverPolicy: "all",
			AckPolicy:     "explicit",
		}

		handle, consumeErr := client.ConsumeStreamWithConfig(ctx, PortConsumerContext{Component: "integration", Port: "input"}, cfg, func(_ context.Context, msg jetstream.Msg) {
			msg.Ack()
		})
		require.NoError(t, consumeErr)
		handles = append(handles, handle)
	}

	for _, handle := range handles {
		drainNativeConsume(t, handle)
	}
}

func drainNativeConsume(t *testing.T, handle jetstream.ConsumeContext) {
	t.Helper()
	require.NotNil(t, handle)
	handle.Drain()
	select {
	case <-handle.Closed():
	case <-time.After(failureBound):
		t.Fatal("timed out waiting for native consumer handle to close")
	}
}

// TestIntegration_PublishToStreamWithAck tests publishing with acknowledgment
func TestIntegration_PublishToStreamWithAck(t *testing.T) {
	ctx := context.Background()

	// Start NATS container with JetStream
	natsURL := startFixture(t).URL()

	// Create and connect client
	client, err := NewClient(natsURL)
	require.NoError(t, err)
	err = client.Connect(ctx)
	require.NoError(t, err)
	defer closeClient(t, client)

	// Create stream
	streamCfg := jetstream.StreamConfig{
		Name:     "PUBACK_STREAM",
		Subjects: []string{"puback.>"},
		Storage:  jetstream.MemoryStorage,
		MaxAge:   testStreamMaxAge,
		MaxBytes: testStreamMaxBytes,
	}
	_, err = client.EnsureStream(ctx, streamCfg)
	require.NoError(t, err)

	// Publish with ack
	ack, err := client.PublishToStreamWithAck(ctx, "puback.test", []byte("test message"))
	require.NoError(t, err)
	assert.NotNil(t, ack)
	assert.Equal(t, "PUBACK_STREAM", ack.Stream)
	assert.GreaterOrEqual(t, ack.Sequence, uint64(1))
}

// TestIntegration_PublishToStreamWithAck_NotConnected tests publish when not connected
func TestIntegration_PublishToStreamWithAck_NotConnected(t *testing.T) {
	client, err := NewClient("nats://unused")
	require.NoError(t, err)

	ctx := context.Background()
	_, err = client.PublishToStreamWithAck(ctx, "test.subject", []byte("data"))
	assert.Equal(t, ErrNotConnected, err)
}

// TestIntegration_DefaultStreamConfig tests default configuration values.
//
// The bounds are asserted ABSENT deliberately. This used to return MaxAge 7 days
// with MaxBytes unset — the exact silent framework default the bounds requirement
// removed from the configuration path, handing out a retention window nobody chose
// and no size ceiling at all. A caller that auto-creates now states its own bounds
// or the seam refuses the creation.
func TestIntegration_DefaultStreamConfig(t *testing.T) {
	cfg := DefaultStreamConfig()

	assert.Equal(t, "file", cfg.Storage)
	assert.Equal(t, "limits", cfg.Retention)
	assert.Equal(t, 1, cfg.Replicas)
	assert.Zero(t, cfg.MaxAge, "no default retention window: a bound nobody chose is what this requirement ends")
	assert.Zero(t, cfg.MaxBytes, "and no default size ceiling")
}

// awaitCount waits until counter reaches at least want, the event the pin's
// sleeps before a count assertion stood in for (design D8 R1b).
func awaitCount(t *testing.T, counter *atomic.Int32, want int32) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), failureBound)
	defer cancel()
	got, err := probe.Await(ctx, func(context.Context) (int32, error) { return counter.Load(), nil },
		func(n int32) bool { return n >= want })
	require.NoError(t, err, "count reached %d, want at least %d", got, want)
}
