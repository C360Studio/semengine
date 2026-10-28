//go:build integration

package graphingest

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/natsclient"
)

// longApplyAckWait is the acknowledgement wait of graph-ingest's consumer and of the oracle in the long-apply test.
// The port declares no heartbeat_interval, so graph-ingest's is a quarter of it.
const longApplyAckWait = 2 * time.Second

// startLongApplyIngest starts graph-ingest on client under the c360.test authority with the entity_stream port
// declaring ack_wait 2s and no heartbeat_interval, and pause as its logger's handler.
func startLongApplyIngest(ctx context.Context, t *testing.T, client *natsclient.Client,
	pause *pauseOnRecord) *graphIngestTestOwner {
	t.Helper()
	cfgJSON, err := json.Marshal(withEntityStreamTiming(t, DefaultConfig(), longApplyAckWait.String(), ""))
	require.NoError(t, err)
	deps := testDependencies(t, client, withAuthority("c360", "test"))
	deps.Logger = slog.New(pause)
	created, err := CreateGraphIngest(cfgJSON, deps)
	require.NoError(t, err)
	c := created.(*Component)
	owner := newGraphIngestTestOwner(c)
	defer owner.provisionalFinish(ctx, t)
	require.NoError(t, c.Initialize())
	registerMergeTestPayload(t, c)
	require.NoError(t, c.Start(owner.startContext(ctx)))
	owner.transfer()
	return owner
}

// TestIntegration_Settlement_LongApplyIsNotRedelivered holds graph-ingest-recovery, "Settlement order", scenario
// "Long apply" (design D8, "Long apply"; task 4.4 part 2): an input whose application starts at once and outlasts the
// consumer's acknowledgement wait is not delivered again while it is applied. graph-ingest's consumer has a 2-second
// wait; the application is held after its entity write and before its applied-sequence record, by graph-ingest's
// logger handler pausing on "Entity ingested" (fix-301 design D2, "The pause"). The oracle that says the wait has
// passed is a consumer of the test's own on the same stream and subject, with the same wait, created once the
// application is held: by the time it has delivered the input three times, at least two of its waits have passed
// since graph-ingest's delivery, and both consumers' timers run on one server. graph-ingest's consumer must then show
// one delivery, none redelivered, and one acknowledgement pending. Once released, the input is acknowledged after one
// delivery and the graph is as one application left it.
func TestIntegration_Settlement_LongApplyIsNotRedelivered(t *testing.T) {
	ctx, cancelOperation := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancelOperation()
	const (
		entityID = "c360.test.wire.settlement.entity.003"
		guardKey = "c360.test.wire.settlement.entity.003/ENTITY" // entity ID, "/", stream name
		oracle   = "long-apply-oracle"
	)

	client := newFixtureClient(t, entityStream)
	js, err := client.JetStream()
	require.NoError(t, err)

	pause := newPauseOnRecord("Entity ingested")
	owner := startLongApplyIngest(ctx, t, client, pause)
	defer owner.finish(ctx, t)
	defer pause.Release() // before owner.finish: Stop must not wait on a held application
	entities, err := js.KeyValue(ctx, graph.BucketEntityStates)
	require.NoError(t, err)
	guardRecords, err := js.KeyValue(ctx, graph.BucketGraphIngestAppliedSeq)
	require.NoError(t, err)

	// (1) One input, and its application held after its entity write.
	seq := publishTriples(ctx, t, client, entityID, message.Triple{
		Subject: entityID, Predicate: settlementPredicate, Object: settlementObject, Timestamp: fixtureTime, Confidence: 1.0,
	})
	pause.awaitEntered(ctx, t, settlementApplyBound)

	// (2) and (3) The oracle, created only now, delivers the input three times, never acknowledged.
	oracleConsumer, err := js.CreateOrUpdateConsumer(ctx, entityStream.name, jetstream.ConsumerConfig{
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

	// (4) graph-ingest's delivery: one, not redelivered, pending. The entity write precedes the held record
	// (pause.entered, awaited at (1)), and the applied-sequence record follows the release.
	delivery, err := ingestConsumer(ctx, js)
	require.NoError(t, err)
	t.Logf("graph-ingest's consumer: %+v; the oracle's delivery counts: %v", delivery, oracleDeliveries)
	require.Equal(t, 0, delivery.Redelivered, "the input must not be delivered again while it is applied")
	require.Equal(t, uint64(1), delivery.Deliveries, "the input must have been delivered once")
	require.Equal(t, 1, delivery.AckPending, "the input must be pending acknowledgement")
	_, err = guardRecords.Get(ctx, guardKey)
	require.ErrorIs(t, err, jetstream.ErrKeyNotFound, "the held application must not have written its record")
	applied := graphState(ctx, t, entities)
	requirePublishedStatementOnce(t, applied[entityID])

	// (5) Released, the input is acknowledged after its one delivery.
	pause.Release()
	delivery = awaitSettlement(ctx, t, nil, "the input's acknowledgement", settlementApplyBound,
		func(ctx context.Context) (ingestDelivery, error) { return ingestConsumer(ctx, js) },
		func(d ingestDelivery) bool { return d.AckFloor >= seq && d.AckPending == 0 })
	require.Equal(t, uint64(1), delivery.Deliveries, "the input must have been delivered once in total")
	_, err = guardRecords.Get(ctx, guardKey)
	require.NoError(t, err, "the acknowledgement must follow the input's applied-sequence record")
	require.Equal(t, applied, graphState(ctx, t, entities), "the graph must be as one application left it")
}
