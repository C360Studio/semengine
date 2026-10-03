//go:build integration

package natsclient

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_PublishToStreamAsync_PipelinesAndDrains verifies the async
// publish path: N messages enqueue without blocking on individual acks, every
// returned future resolves via Ok() (no Err()), PublishAsyncComplete closes once
// drained, PublishAsyncPending returns to 0, and all N are stored (gh#470).
func TestIntegration_PublishToStreamAsync_PipelinesAndDrains(t *testing.T) {
	ctx := context.Background()

	natsURL := startFixture(t).URL()

	client, err := NewClient(natsURL)
	require.NoError(t, err)
	require.NoError(t, client.Connect(ctx))
	defer closeClient(t, client)

	stream, err := client.EnsureStream(ctx, jetstream.StreamConfig{
		Name:     "ASYNC_STREAM",
		Subjects: []string{"async.>"},
		Storage:  jetstream.MemoryStorage,
		MaxAge:   testStreamMaxAge,
		MaxBytes: testStreamMaxBytes,
	})
	require.NoError(t, err)

	const n = 500
	futures := make([]jetstream.PubAckFuture, 0, n)
	for i := 0; i < n; i++ {
		f, perr := client.publishToStreamAsync(ctx, "async.test", []byte(strconv.Itoa(i)))
		require.NoError(t, perr)
		require.NotNil(t, f)
		futures = append(futures, f)
	}

	// Drain: block until every outstanding async publish is acked.
	js := jetStreamOf(t, client)
	select {
	case <-js.PublishAsyncComplete():
	case <-time.After(failureBound):
		t.Fatalf("PublishAsyncComplete did not close; %d still pending", js.PublishAsyncPending())
	}

	assert.Equal(t, 0, js.PublishAsyncPending(), "no async publishes should remain pending after drain")

	// Every future resolved successfully.
	for i, f := range futures {
		select {
		case <-f.Ok():
		case ackErr := <-f.Err():
			t.Fatalf("future %d resolved with error: %v", i, ackErr)
		default:
			t.Fatalf("future %d did not resolve after drain", i)
		}
	}

	info, err := stream.Info(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(n), info.State.Msgs, "all async publishes must be stored")
}

// TestIntegration_PublishToStreamAsync_Ordering verifies per-subject ordering from
// a single caller/connection: an async-published monotonic sequence is stored (and
// consumed) in publish order.
func TestIntegration_PublishToStreamAsync_Ordering(t *testing.T) {
	ctx := context.Background()

	natsURL := startFixture(t).URL()

	client, err := NewClient(natsURL)
	require.NoError(t, err)
	require.NoError(t, client.Connect(ctx))
	defer closeClient(t, client)

	_, err = client.EnsureStream(ctx, jetstream.StreamConfig{
		Name:     "ASYNC_ORDER_STREAM",
		Subjects: []string{"asyncorder.>"},
		Storage:  jetstream.MemoryStorage,
		MaxAge:   testStreamMaxAge,
		MaxBytes: testStreamMaxBytes,
	})
	require.NoError(t, err)

	const n = 200
	for i := 0; i < n; i++ {
		_, perr := client.publishToStreamAsync(ctx, "asyncorder.seq", []byte(strconv.Itoa(i)))
		require.NoError(t, perr)
	}
	select {
	case <-jetStreamOf(t, client).PublishAsyncComplete():
	case <-time.After(failureBound):
		t.Fatal("drain timeout")
	}

	// Consume in stream order and assert the sequence matches publish order.
	got := make([]int, 0, n)
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(n)
	handle, err := client.ConsumeStreamWithConfig(ctx, PortConsumerContext{Component: "integration", Port: "input"}, StreamConsumerConfig{
		StreamName:    "ASYNC_ORDER_STREAM",
		ConsumerName:  "order-consumer",
		FilterSubject: "asyncorder.>",
		DeliverPolicy: "all",
		AckPolicy:     "explicit",
	}, func(_ context.Context, msg jetstream.Msg) {
		v, _ := strconv.Atoi(string(msg.Data()))
		mu.Lock()
		got = append(got, v)
		mu.Unlock()
		msg.Ack()
		wg.Done()
	})
	require.NoError(t, err)
	defer drainNativeConsume(t, handle)

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(failureBound):
		t.Fatalf("consume timeout, got %d/%d", len(got), n)
	}

	want := make([]int, n)
	for i := range want {
		want[i] = i
	}
	assert.Equal(t, want, got, "async publishes must be stored/consumed in publish order")
}

// TestIntegration_PublishBatchToStream verifies the batch helper stores every
// message in publish order and returns nil on all-acked.
func TestIntegration_PublishBatchToStream(t *testing.T) {
	ctx := context.Background()

	natsURL := startFixture(t).URL()

	client, err := NewClient(natsURL)
	require.NoError(t, err)
	require.NoError(t, client.Connect(ctx))
	defer closeClient(t, client)

	_, err = client.EnsureStream(ctx, jetstream.StreamConfig{
		Name:     "BATCH_STREAM",
		Subjects: []string{"batch.>"},
		Storage:  jetstream.MemoryStorage,
		MaxAge:   testStreamMaxAge,
		MaxBytes: testStreamMaxBytes,
	})
	require.NoError(t, err)

	const m = 300
	msgs := make([][]byte, m)
	for i := range msgs {
		msgs[i] = []byte(fmt.Sprintf("msg-%d", i))
	}

	require.NoError(t, client.PublishBatchToStream(ctx, "batch.test", msgs))
	assert.Equal(t, 0, jetStreamOf(t, client).PublishAsyncPending(), "batch must fully drain before returning")

	// Consume and assert order.
	got := make([]string, 0, m)
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(m)
	handle, err := client.ConsumeStreamWithConfig(ctx, PortConsumerContext{Component: "integration", Port: "input"}, StreamConsumerConfig{
		StreamName:    "BATCH_STREAM",
		ConsumerName:  "batch-consumer",
		FilterSubject: "batch.>",
		DeliverPolicy: "all",
		AckPolicy:     "explicit",
	}, func(_ context.Context, msg jetstream.Msg) {
		mu.Lock()
		got = append(got, string(msg.Data()))
		mu.Unlock()
		msg.Ack()
		wg.Done()
	})
	require.NoError(t, err)
	defer drainNativeConsume(t, handle)

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(failureBound):
		t.Fatalf("consume timeout, got %d/%d", len(got), m)
	}

	want := make([]string, m)
	for i := range want {
		want[i] = fmt.Sprintf("msg-%d", i)
	}
	assert.Equal(t, want, got, "batch must be stored/consumed in publish order")
}

// TestIntegration_PublishToStreamAsync_StampsTrace verifies the async path
// preserves the synchronous path's trace invariant: a trace context is injected
// (auto-generated when absent). Asserted on the raw consumed message headers. The
// pin's message-ID half went with PublishToStreamAsyncWithMsgID (task 3.7a).
func TestIntegration_PublishToStreamAsync_StampsTrace(t *testing.T) {
	ctx := context.Background()

	natsURL := startFixture(t).URL()

	client, err := NewClient(natsURL)
	require.NoError(t, err)
	require.NoError(t, client.Connect(ctx))
	defer closeClient(t, client)

	_, err = client.EnsureStream(ctx, jetstream.StreamConfig{
		Name:     "ASYNC_HDR_STREAM",
		Subjects: []string{"asynchdr.>"},
		Storage:  jetstream.MemoryStorage,
		MaxAge:   testStreamMaxAge,
		MaxBytes: testStreamMaxBytes,
	})
	require.NoError(t, err)

	_, err = client.publishToStreamAsync(ctx, "asynchdr.test", []byte("payload"))
	require.NoError(t, err)
	awaitAsyncComplete(t, client)

	var hdr map[string][]string
	var wg sync.WaitGroup
	wg.Add(1)
	handle, err := client.ConsumeStreamWithConfig(ctx, PortConsumerContext{Component: "integration", Port: "input"}, StreamConsumerConfig{
		StreamName:    "ASYNC_HDR_STREAM",
		ConsumerName:  "hdr-consumer",
		FilterSubject: "asynchdr.>",
		DeliverPolicy: "all",
		AckPolicy:     "explicit",
	}, func(_ context.Context, msg jetstream.Msg) {
		hdr = msg.Headers()
		msg.Ack()
		wg.Done()
	})
	require.NoError(t, err)
	defer drainNativeConsume(t, handle)

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(failureBound):
		t.Fatal("consume timeout")
	}

	require.NotNil(t, hdr)
	assert.NotEmpty(t, hdr[TraceIDHeader], "async publish must inject a trace ID header")
	assert.NotEmpty(t, hdr[TraceparentHeader], "async publish must inject a W3C traceparent header")
}

// TestIntegration_PublishToStreamAsync_EnqueueResetsCircuit verifies the documented
// breaker semantic (design §4): a successful async enqueue resets the failure
// count, giving a pure-async producer a reset path.
func TestIntegration_PublishToStreamAsync_EnqueueResetsCircuit(t *testing.T) {
	ctx := context.Background()

	natsURL := startFixture(t).URL()

	client, err := NewClient(natsURL)
	require.NoError(t, err)
	require.NoError(t, client.Connect(ctx))
	defer closeClient(t, client)

	_, err = client.EnsureStream(ctx, jetstream.StreamConfig{
		Name:     "ASYNC_RESET_STREAM",
		Subjects: []string{"asyncreset.>"},
		Storage:  jetstream.MemoryStorage,
		MaxAge:   testStreamMaxAge,
		MaxBytes: testStreamMaxBytes,
	})
	require.NoError(t, err)

	// Accumulate failures below the open threshold.
	for i := 0; i < 5; i++ {
		client.recordFailure()
	}
	require.Equal(t, int32(5), client.Failures())

	// A successful async enqueue must reset the failure count.
	f, err := client.publishToStreamAsync(ctx, "asyncreset.test", []byte("ok"))
	require.NoError(t, err)
	require.NotNil(t, f)
	assert.Equal(t, int32(0), client.Failures(),
		"a successful async enqueue must reset the circuit breaker failure count")
	awaitAsyncComplete(t, client)
}

// TestIntegration_PublishBatchToStream_CtxCancel verifies a batch whose context is
// already cancelled returns a context error rather than hanging.
func TestIntegration_PublishBatchToStream_CtxCancel(t *testing.T) {
	ctx := context.Background()

	natsURL := startFixture(t).URL()

	client, err := NewClient(natsURL)
	require.NoError(t, err)
	require.NoError(t, client.Connect(ctx))
	defer closeClient(t, client)

	_, err = client.EnsureStream(ctx, jetstream.StreamConfig{
		Name:     "BATCH_CANCEL_STREAM",
		Subjects: []string{"batchcancel.>"},
		Storage:  jetstream.MemoryStorage,
		MaxAge:   testStreamMaxAge,
		MaxBytes: testStreamMaxBytes,
	})
	require.NoError(t, err)

	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel()

	msgs := [][]byte{[]byte("a"), []byte("b"), []byte("c")}
	err = client.PublishBatchToStream(cancelledCtx, "batchcancel.test", msgs)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// jetStreamOf returns c's JetStream handle. Client.PublishAsyncComplete and
// PublishAsyncPending were dropped as dead surface (task 3.7a); they forwarded to
// this handle's own methods.
func jetStreamOf(t *testing.T, c *Client) jetstream.JetStream {
	t.Helper()
	js, err := c.JetStream()
	require.NoError(t, err)
	return js
}

// awaitAsyncComplete waits until every async publish on c's handle is
// acknowledged; the pin received from PublishAsyncComplete with no bound.
func awaitAsyncComplete(t *testing.T, c *Client) {
	t.Helper()
	select {
	case <-jetStreamOf(t, c).PublishAsyncComplete():
	case <-time.After(failureBound):
		t.Fatal("async publishes were not acknowledged")
	}
}
