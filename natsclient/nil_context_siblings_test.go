package natsclient

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
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

// Every other exported, error-returning natsclient entry that takes a context refuses a nil one,
// without panicking and before touching any state (Codex F32, "cover these sibling entry points
// together"), with the same invalid-data error as the rest. KVStore.KeysByFilter,
// ValidateHeartbeatDeliveryPolicy and Subscription.Drain refused nil before the sweep with an
// unclassified error; they now use the same class (early check E2).
func TestExportedNilContextRefusedOnEveryOtherEntry(t *testing.T) {
	c, err := NewClient(embeddedJetStreamURL(t), WithHealthInterval(0))
	require.NoError(t, err)
	require.NoError(t, c.Connect(t.Context()))
	closeInCleanup(t, c)

	bucket, err := c.CreateKeyValueBucket(t.Context(), jetstream.KeyValueConfig{Bucket: "NILCTX_KV", Storage: jetstream.MemoryStorage})
	require.NoError(t, err)
	kv := c.NewKVStore(bucket)
	_, err = kv.Put(t.Context(), "k", []byte(`{"a":1}`))
	require.NoError(t, err)
	js, err := c.JetStream()
	require.NoError(t, err)
	streamCfg := jetstream.StreamConfig{
		Name: "NILCTX_STREAM", Subjects: []string{"nilctx.stream.>"},
		Storage: jetstream.MemoryStorage, MaxAge: time.Hour, MaxBytes: 1 << 20,
	}
	stream, err := c.EnsureStream(t.Context(), streamCfg)
	require.NoError(t, err)
	consumer, err := stream.CreateOrUpdateConsumer(t.Context(), jetstream.ConsumerConfig{AckPolicy: jetstream.AckExplicitPolicy})
	require.NoError(t, err)
	sub, err := c.Subscribe(t.Context(), "nilctx.sub", func(context.Context, *nats.Msg) {})
	require.NoError(t, err)
	collector := accountTestCollector(t, (&accountAwareLister{}).source())
	publisher := newTestPublisher(t, newFakeReportStore(1), defaultSource())
	owner := PortConsumerContext{Component: "nilctx", Port: "in"}
	consumerCfg := StreamConsumerConfig{StreamName: "NILCTX_STREAM", ConsumerName: "nilctx", FilterSubject: "nilctx.stream.a"}
	noop := func(context.Context, jetstream.Msg) {}
	updateBytes := func(b []byte) ([]byte, error) { return b, nil }

	for _, tc := range []struct {
		name string
		call func(context.Context) error
	}{
		// KVStore
		{"KVStore.AssertNoLifecycleRetention", func(ctx context.Context) error { return kv.AssertNoLifecycleRetention(ctx, "NILCTX_KV") }},
		{"KVStore.Create", func(ctx context.Context) error { _, err := kv.Create(ctx, "new", nil); return err }},
		{"KVStore.Delete", func(ctx context.Context) error { return kv.Delete(ctx, "k") }},
		{"KVStore.DeleteAtRevision", func(ctx context.Context) error { return kv.DeleteAtRevision(ctx, "k", 1) }},
		{"KVStore.Get", func(ctx context.Context) error { _, err := kv.Get(ctx, "k"); return err }},
		{"KVStore.Keys", func(ctx context.Context) error { _, err := kv.Keys(ctx); return err }},
		{"KVStore.KeysByFilter", func(ctx context.Context) error { _, err := kv.KeysByFilter(ctx, "k"); return err }},
		{"KVStore.KeysByPrefix", func(ctx context.Context) error { _, err := kv.KeysByPrefix(ctx, "k"); return err }},
		{"KVStore.Put", func(ctx context.Context) error { _, err := kv.Put(ctx, "k", nil); return err }},
		{"KVStore.Update", func(ctx context.Context) error { _, err := kv.Update(ctx, "k", nil, 1); return err }},
		{"KVStore.UpdateJSON", func(ctx context.Context) error {
			return kv.UpdateJSON(ctx, "k", func(map[string]any) error { return nil })
		}},
		{"KVStore.UpdateWithRetry", func(ctx context.Context) error { return kv.UpdateWithRetry(ctx, "k", updateBytes) }},
		{"KVStore.UpdateWithRetryRead", func(ctx context.Context) error {
			_, err := kv.UpdateWithRetryRead(ctx, "k", func(b []byte, _ uint64) ([]byte, error) { return b, nil })
			return err
		}},
		{"KVStore.UpdateWithRetryRev", func(ctx context.Context) error {
			_, err := kv.UpdateWithRetryRev(ctx, "k", updateBytes)
			return err
		}},
		{"KVStore.Watch", func(ctx context.Context) error { _, err := kv.Watch(ctx, "k"); return err }},
		// Retention and bucket helpers
		{"BucketRetention", func(ctx context.Context) error { _, _, err := BucketRetention(ctx, bucket); return err }},
		{"BucketLastSeq", func(ctx context.Context) error { _, err := BucketLastSeq(ctx, bucket); return err }},
		{"FilteredKeys", func(ctx context.Context) error { _, err := FilteredKeys(ctx, bucket, "k"); return err }},
		{"ReconcileNoLifecycleRetention", func(ctx context.Context) error {
			return ReconcileNoLifecycleRetention(ctx, js, "NILCTX_KV", nil)
		}},
		// Streams and consumers
		{"CreateStream", func(ctx context.Context) error { _, err := c.CreateStream(ctx, streamCfg); return err }},
		{"EnsureStream", func(ctx context.Context) error { _, err := c.EnsureStream(ctx, streamCfg); return err }},
		{"GetStream", func(ctx context.Context) error { _, err := c.GetStream(ctx, "NILCTX_STREAM"); return err }},
		{"PublishToStream", func(ctx context.Context) error { return c.PublishToStream(ctx, "nilctx.stream.a", nil) }},
		{"PublishToStreamWithAck", func(ctx context.Context) error {
			_, err := c.PublishToStreamWithAck(ctx, "nilctx.stream.a", nil)
			return err
		}},
		{"PublishToStreamWithMsgID", func(ctx context.Context) error {
			return c.PublishToStreamWithMsgID(ctx, "nilctx.stream.a", nil, "id-1")
		}},
		{"PublishBatchToStream", func(ctx context.Context) error {
			return c.PublishBatchToStream(ctx, "nilctx.stream.a", [][]byte{nil})
		}},
		{"ConsumeInternalStreamWithConfig", func(ctx context.Context) error {
			_, err := c.ConsumeInternalStreamWithConfig(ctx, consumerCfg, noop)
			return err
		}},
		{"ConsumeStreamWithConfig", func(ctx context.Context) error {
			_, err := c.ConsumeStreamWithConfig(ctx, owner, consumerCfg, noop)
			return err
		}},
		{"ConsumeStreamWithConfigContexts", func(ctx context.Context) error {
			_, err := c.ConsumeStreamWithConfigContexts(ctx, ctx, owner, consumerCfg, noop)
			return err
		}},
		{"ObserveDirectPortConsumerPolicy", func(ctx context.Context) error {
			_, err := c.ObserveDirectPortConsumerPolicy(ctx, owner, jetstream.ConsumerConfig{}, consumer)
			return err
		}},
		{"ValidateHeartbeatDeliveryPolicy", func(ctx context.Context) error {
			_, err := ValidateHeartbeatDeliveryPolicy(ctx, consumerCfg, time.Second, DeliveryRetryPolicy{}, nil)
			return err
		}},
		// Subscriptions and connection
		{"Subscribe", func(ctx context.Context) error {
			s, err := c.Subscribe(ctx, "nilctx.sub2", func(context.Context, *nats.Msg) {})
			if s != nil {
				return errors.New("a refused subscribe returned a subscription")
			}
			return err
		}},
		{"Subscription.Drain", func(ctx context.Context) error { return sub.Drain(ctx) }},
		{"WaitForConnection", func(ctx context.Context) error { return c.WaitForConnection(ctx) }},
		// Storage reporting
		{"StorageInventoryCollector.Collect", func(ctx context.Context) error { _, err := collector.Collect(ctx); return err }},
		{"StorageReportPublisher.Publish", func(ctx context.Context) error {
			_, err := publisher.Publish(ctx, StorageInventory{})
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
			switch {
			case err == nil:
				t.Error("nil context accepted")
			case !errs.IsInvalid(err):
				t.Errorf("nil context: err = %v, want an invalid-data refusal", err)
			}
		})
	}

	// No refused call changed state: the key still holds its value at its first revision.
	entry, err := kv.Get(t.Context(), "k")
	require.NoError(t, err)
	require.Equal(t, `{"a":1}`, string(entry.Value))
	require.Equal(t, uint64(1), entry.Revision)
}
