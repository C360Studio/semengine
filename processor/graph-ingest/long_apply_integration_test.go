//go:build integration

package graphingest

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/natsclient"
)

// longApplyAckWait is the acknowledgement wait of graph-ingest's consumer and of the oracle in the long-apply test.
// The port declares no heartbeat_interval, so graph-ingest's is a quarter of it.
const longApplyAckWait = 2 * time.Second

// heldApplyBucket is the long-apply test's applied-sequence bucket: the process-kill test's held write, which also
// reports its arrival and ends. Put closes entered on the first write, then waits until the test closes release, when
// it makes the write, or until its context ends. Every other method is the bucket's.
type heldApplyBucket struct {
	jetstream.KeyValue
	enterOnce sync.Once
	entered   chan struct{}
	release   chan struct{}
}

func (b *heldApplyBucket) Put(ctx context.Context, key string, value []byte) (uint64, error) {
	b.enterOnce.Do(func() { close(b.entered) })
	select {
	case <-b.release:
		return b.KeyValue.Put(ctx, key, value)
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// progressCountingMsg is a delivery to graph-ingest that counts the in-progress signals sent for it.
type progressCountingMsg struct {
	jetstream.Msg
	inProgress *atomic.Int64
}

func (m progressCountingMsg) InProgress() error {
	m.inProgress.Add(1)
	return m.Msg.InProgress()
}

// startLongApplyIngest starts graph-ingest on client under the c360.test authority with the entity_stream port
// declaring ack_wait 2s and no heartbeat_interval. Through the consumeStream seam, as startSettlementIngest does, the
// applied-sequence bucket is replaced by held, and each delivery is handed on as a progressCountingMsg counting into
// inProgress. The replaced store has no operation timeout: the held write outlasts the five seconds KVStore gives a
// write by default, and on release it must make the write on graph-ingest's own context.
func startLongApplyIngest(ctx context.Context, t *testing.T, client *natsclient.Client, held *heldApplyBucket,
	inProgress *atomic.Int64) *graphIngestTestOwner {
	t.Helper()
	cfgJSON, err := json.Marshal(withEntityStreamTiming(t, DefaultConfig(), longApplyAckWait.String(), ""))
	require.NoError(t, err)
	created, err := CreateGraphIngest(cfgJSON, testDependencies(t, client, withAuthority("c360", "test")))
	require.NoError(t, err)
	c := created.(*Component)
	owner := newGraphIngestTestOwner(c)
	defer owner.provisionalFinish(ctx, t)
	require.NoError(t, c.Initialize())
	registerMergeTestPayload(t, c)
	c.consumeStream = func(ctx context.Context, port natsclient.PortConsumerContext,
		cfg natsclient.StreamConsumerConfig, handler func(context.Context, jetstream.Msg),
	) (jetstream.ConsumeContext, error) {
		bucket, err := kvcatalog.EnsureCatalogBucket(ctx, c.natsClient, graph.BucketGraphIngestAppliedSeq)
		if err != nil {
			return nil, err
		}
		held.KeyValue = bucket
		c.ingestGuardBucket = c.natsClient.NewKVStore(held, func(o *natsclient.KVOptions) { o.Timeout = 0 })
		return c.natsClient.ConsumeStreamWithConfig(ctx, port, cfg, func(ctx context.Context, msg jetstream.Msg) {
			handler(ctx, progressCountingMsg{Msg: msg, inProgress: inProgress})
		})
	}
	require.NoError(t, c.Start(owner.startContext(ctx)))
	owner.transfer()
	return owner
}

// TestIntegration_Settlement_LongApplyIsNotRedelivered holds graph-ingest-recovery, "Settlement order", scenario
// "Long apply" (design D8, "Long apply"; task 4.4 part 2): an input whose application starts at once and outlasts the
// consumer's acknowledgement wait is not delivered again while it is applied. graph-ingest's consumer has a 2-second
// wait; the application is held at its applied-sequence write. The oracle that says the wait has passed is a consumer
// of the test's own on the same stream and subject, with the same wait, created once the write has begun: by the time
// it has delivered the input three times, at least two of its waits have passed since graph-ingest's delivery, and
// both consumers' timers run on one server. graph-ingest's consumer must then show one delivery, none redelivered,
// one acknowledgement pending, and in-progress signals sent. Once released, the input is acknowledged after one
// delivery and the graph is as one application left it.
func TestIntegration_Settlement_LongApplyIsNotRedelivered(t *testing.T) {
	ctx, cancelOperation := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancelOperation()
	const (
		entityID = "c360.test.wire.settlement.entity.003"
		guardKey = "c360.test.wire.settlement.entity.003/ENTITY" // entity ID, "/", stream name
		oracle   = "long-apply-oracle"
	)

	f := natsfixture.New(t)
	require.NoError(t, f.Start(ctx))
	_, err := f.CreateStream(ctx, entityStream.name, entityStream.subjects...)
	require.NoError(t, err)
	client := natsfixture.Open(t, f, openFixtureClient)

	held := &heldApplyBucket{entered: make(chan struct{}), release: make(chan struct{})}
	var inProgress atomic.Int64
	owner := startLongApplyIngest(ctx, t, client, held, &inProgress)
	defer owner.finish(ctx, t)
	var releaseOnce sync.Once
	releaseHeld := func() { releaseOnce.Do(func() { close(held.release) }) }
	defer releaseHeld() // before owner.finish: Stop must not wait on a held write
	entities, err := f.JetStream().KeyValue(ctx, graph.BucketEntityStates)
	require.NoError(t, err)
	guardRecords, err := f.JetStream().KeyValue(ctx, graph.BucketGraphIngestAppliedSeq)
	require.NoError(t, err)

	// (1) One input, and its application held at the applied-sequence write.
	seq := publishTriples(ctx, t, client, entityID, message.Triple{
		Subject: entityID, Predicate: settlementPredicate, Object: settlementObject, Timestamp: fixtureTime, Confidence: 1.0,
	})
	enteredCtx, cancelEntered := context.WithTimeout(ctx, settlementApplyBound)
	defer cancelEntered()
	select {
	case <-held.entered:
	case <-enteredCtx.Done():
		t.Fatalf("graph-ingest made no applied-sequence write within %s", settlementApplyBound)
	}

	// (2) and (3) The oracle, created only now, delivers the input three times, never acknowledged.
	oracleConsumer, err := f.JetStream().CreateOrUpdateConsumer(ctx, entityStream.name, jetstream.ConsumerConfig{
		Durable: oracle, FilterSubject: entityStream.subjects[0], DeliverPolicy: jetstream.DeliverAllPolicy,
		AckPolicy: jetstream.AckExplicitPolicy, AckWait: longApplyAckWait, MaxDeliver: 3,
	})
	require.NoError(t, err)
	fetchCtx, cancelFetch := context.WithTimeout(ctx, 4*settlementApplyBound)
	defer cancelFetch()
	var oracleDeliveries []uint64
	for len(oracleDeliveries) == 0 || oracleDeliveries[len(oracleDeliveries)-1] < 3 {
		msg, err := oracleConsumer.Next(jetstream.FetchContext(fetchCtx))
		require.NoError(t, err, "the oracle's deliveries so far: %v", oracleDeliveries)
		meta, err := msg.Metadata()
		require.NoError(t, err)
		require.Equal(t, seq, meta.Sequence.Stream, "the oracle must deliver the one input")
		oracleDeliveries = append(oracleDeliveries, meta.NumDelivered)
	}

	// (4) graph-ingest's delivery: one, not redelivered, pending, and kept in progress.
	delivery, err := ingestConsumer(ctx, f)
	require.NoError(t, err)
	signals := inProgress.Load()
	t.Logf("graph-ingest's consumer: %+v; in-progress signals: %d; the oracle's delivery counts: %v",
		delivery, signals, oracleDeliveries)
	require.Equal(t, 0, delivery.Redelivered, "the input must not be delivered again while it is applied")
	require.Equal(t, uint64(1), delivery.Deliveries, "the input must have been delivered once")
	require.Equal(t, 1, delivery.AckPending, "the input must be pending acknowledgement")
	require.GreaterOrEqual(t, signals, int64(1), "the application must have signalled that it is in progress")
	_, err = guardRecords.Get(ctx, guardKey)
	require.ErrorIs(t, err, jetstream.ErrKeyNotFound, "the held write must not have reached the bucket")
	applied := graphState(ctx, t, entities)
	requirePublishedStatementOnce(t, applied[entityID])

	// (5) Released, the input is acknowledged after its one delivery.
	releaseHeld()
	delivery = awaitSettlement(ctx, t, nil, "the input's acknowledgement", settlementApplyBound,
		func(ctx context.Context) (ingestDelivery, error) { return ingestConsumer(ctx, f) },
		func(d ingestDelivery) bool { return d.AckFloor >= seq && d.AckPending == 0 })
	require.Equal(t, uint64(1), delivery.Deliveries, "the input must have been delivered once in total")
	_, err = guardRecords.Get(ctx, guardKey)
	require.NoError(t, err, "the acknowledgement must follow the input's applied-sequence record")
	require.Equal(t, applied, graphState(ctx, t, entities), "the graph must be as one application left it")
}
