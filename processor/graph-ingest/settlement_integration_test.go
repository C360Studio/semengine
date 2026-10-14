//go:build integration

package graphingest

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/internal/harness/prochost"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/natsclient"
)

// The helpers the settlement process-kill test starts (design D8, "The settlement process-kill test"), and what the
// test passes each in its environment.
const (
	// settlementHeld runs graph-ingest with every applied-sequence write held: the first run.
	settlementHeld = "graph-ingest-guard-write-held"
	// settlementPlain runs graph-ingest unchanged: the second run.
	settlementPlain = "graph-ingest"
	// helperNATSURL is the fixture broker's URL; helperReady is the file the helper creates once Start returned.
	helperNATSURL = "HELPER_NATS_URL"
	helperReady   = "HELPER_READY"
)

// Failure bounds for the settlement tests' waits; none paces a run. A redelivery comes once the consumer's
// acknowledgement wait has passed since the first delivery, and graph-ingest's consumer keeps its own 30-second
// default (these tests declare no ack_wait), so that wait is bounded at twice it.
const (
	settlementStartBound      = 30 * time.Second
	settlementApplyBound      = 20 * time.Second
	settlementRedeliveryBound = 60 * time.Second
)

// settlementPredicate and settlementObject are the one statement each settlement test publishes.
const (
	settlementPredicate = "test.content.body"
	settlementObject    = "applied once"
)

// errGuardWriteRefused is the durable-record failure test's fault on the applied-sequence write.
var errGuardWriteRefused = errors.New("injected: applied-sequence write refused")

// TestHelperProcess runs a settlement helper in a child the test started with prochost.Start; in a normal run each
// Helper call returns at once.
func TestHelperProcess(t *testing.T) {
	prochost.Helper(settlementHeld, func() { runSettlementHelper(t, true) })
	prochost.Helper(settlementPlain, func() { runSettlementHelper(t, false) })
}

// runSettlementHelper starts graph-ingest on the broker named in the environment, creates the ready file, and parks
// until a signal ends the process; it never stops graph-ingest. With holdGuardWrite, the applied-sequence bucket is a
// heldWriteBucket, so an input it applies is never acknowledged. A failure fails TestHelperProcess, and the child
// exits.
func runSettlementHelper(t *testing.T, holdGuardWrite bool) {
	ctx := t.Context()
	client, _, err := openFixtureClient(ctx, os.Getenv(helperNATSURL))
	require.NoError(t, err)
	var wrap func(jetstream.KeyValue) jetstream.KeyValue
	if holdGuardWrite {
		wrap = func(bucket jetstream.KeyValue) jetstream.KeyValue {
			return &heldWriteBucket{KeyValue: bucket, held: make(chan struct{})}
		}
	}
	startSettlementIngest(ctx, t, client, wrap)
	ready := os.Getenv(helperReady)
	require.NoError(t, os.WriteFile(ready+".tmp", nil, 0o600))
	require.NoError(t, os.Rename(ready+".tmp", ready))
	terminated := make(chan os.Signal, 1)
	signal.Notify(terminated, syscall.SIGTERM)
	<-terminated
}

// heldWriteBucket is the first run's applied-sequence bucket: Put never returns, so graph-ingest applies an input and
// then waits on its record, unacknowledged, until the test kills the process. Every other method is the bucket's.
type heldWriteBucket struct {
	jetstream.KeyValue
	held chan struct{} // nothing closes it
}

func (b *heldWriteBucket) Put(context.Context, string, []byte) (uint64, error) {
	<-b.held
	return 0, errors.New("heldWriteBucket: a held write was released")
}

// startSettlementIngest starts graph-ingest with its default configuration on client under the c360.test authority,
// as startIngestOn does. With wrap set, the applied-sequence bucket Start acquired is replaced by wrap(bucket) through
// the consumeStream seam: on Start's goroutine, after the bucket is acquired and before the one stream input's consumer
// binds. No input reaches a lane before the replacement, and the delivery that hands inputs to the lanes starts after
// it, so the lanes read the field without a race.
func startSettlementIngest(ctx context.Context, t *testing.T, client *natsclient.Client,
	wrap func(jetstream.KeyValue) jetstream.KeyValue) *graphIngestTestOwner {
	t.Helper()
	cfgJSON, err := json.Marshal(DefaultConfig())
	require.NoError(t, err)
	created, err := CreateGraphIngest(cfgJSON, testDependencies(t, client, withAuthority("c360", "test")))
	require.NoError(t, err)
	c := created.(*Component)
	owner := newGraphIngestTestOwner(c)
	defer owner.provisionalFinish(ctx, t)
	require.NoError(t, c.Initialize())
	registerMergeTestPayload(t, c)
	if wrap != nil {
		c.consumeStream = func(ctx context.Context, port natsclient.PortConsumerContext,
			cfg natsclient.StreamConsumerConfig, handler func(context.Context, jetstream.Msg),
		) (jetstream.ConsumeContext, error) {
			bucket, err := kvcatalog.EnsureCatalogBucket(ctx, c.natsClient, graph.BucketGraphIngestAppliedSeq)
			if err != nil {
				return nil, err
			}
			c.ingestGuardBucket = c.natsClient.NewKVStore(wrap(bucket))
			return c.natsClient.ConsumeStreamWithConfig(ctx, port, cfg, handler)
		}
	}
	require.NoError(t, c.Start(owner.startContext(ctx)))
	owner.transfer()
	return owner
}

// startSettlementHelper starts the helper name against f and waits until it reports graph-ingest started.
func startSettlementHelper(ctx context.Context, t *testing.T, f *natsfixture.Fixture, name string) *prochost.Process {
	t.Helper()
	ready := filepath.Join(t.TempDir(), "ready")
	p, err := prochost.Start(t, name, helperNATSURL+"="+f.URL(), helperReady+"="+ready)
	require.NoError(t, err)
	awaitSettlement(ctx, t, p, "graph-ingest started in helper "+name, settlementStartBound,
		func(context.Context) (bool, error) {
			_, err := os.Stat(ready)
			return err == nil, nil
		},
		func(started bool) bool { return started })
	return p
}

// awaitSettlement polls observe within bound until done accepts what it saw. With helper set, it stops at once and
// fails the test if the helper exits first: a helper that has exited cannot bring the condition about.
func awaitSettlement[T any](ctx context.Context, t *testing.T, helper *prochost.Process, what string,
	bound time.Duration, observe func(context.Context) (T, error), done func(T) bool) T {
	t.Helper()
	type observation struct {
		value  T
		exited bool
	}
	waitCtx, cancel := context.WithTimeout(ctx, bound)
	defer cancel()
	seen, err := probe.Await(waitCtx, func(ctx context.Context) (observation, error) {
		if helper != nil && !helper.Alive() {
			return observation{exited: true}, nil
		}
		value, err := observe(ctx)
		return observation{value: value}, err
	}, func(o observation) bool { return o.exited || done(o.value) })
	require.NoError(t, err, "waiting for %s", what)
	require.False(t, seen.exited, "the helper exited before %s", what)
	return seen.value
}

// ingestDelivery is what the broker reports of graph-ingest's durable consumer on the ENTITY stream, in fields a
// failed wait prints.
type ingestDelivery struct {
	Deliveries      uint64 // deliveries the consumer made, redeliveries included
	DeliveredStream uint64 // the highest stream sequence delivered
	AckFloor        uint64 // every input up to this stream sequence is acknowledged
	AckPending      int    // inputs delivered and not yet acknowledged
	Redelivered     int    // of those, the ones delivered more than once
}

// ingestConsumer reads graph-ingest's durable consumer on the ENTITY stream from the broker.
func ingestConsumer(ctx context.Context, f *natsfixture.Fixture) (ingestDelivery, error) {
	consumer, err := f.JetStream().Consumer(ctx, entityStream.name, restartConsumer)
	if err != nil {
		return ingestDelivery{}, err
	}
	info, err := consumer.Info(ctx)
	if err != nil {
		return ingestDelivery{}, err
	}
	return ingestDelivery{
		Deliveries: info.Delivered.Consumer, DeliveredStream: info.Delivered.Stream, AckFloor: info.AckFloor.Stream,
		AckPending: info.NumAckPending, Redelivered: info.NumRedelivered,
	}, nil
}

// graphState reads every entity in ENTITY_STATES, by key, with UpdatedAt zeroed and its statements grouped by their
// (subject, predicate, source) set, each set's statements in their stored order. Neither is state an input applied
// again may not change. UpdatedAt is the store's write time, not a statement (design D15, "Statement metadata"). A
// set's position among the others is where the replace rule put it: the stored sets an arrival does not carry stay
// ahead of the sets it applies (graph.ReplaceBySource), so an input applied again moves its sets to the end. The order
// inside a set, which GetTriple's last tie-break reads, is the arrival's both times.
func graphState(ctx context.Context, t *testing.T, entities jetstream.KeyValue) map[string]graph.EntityState {
	t.Helper()
	keys, err := entities.Keys(ctx)
	require.NoError(t, err)
	state := make(map[string]graph.EntityState, len(keys))
	for _, key := range keys {
		entry, err := entities.Get(ctx, key)
		require.NoError(t, err)
		var entity graph.EntityState
		require.NoError(t, graph.UnmarshalEntityState(entry.Value(), &entity))
		entity.UpdatedAt = time.Time{}
		slices.SortStableFunc(entity.Triples, func(a, b message.Triple) int {
			return cmp.Or(strings.Compare(a.Subject, b.Subject), strings.Compare(a.Predicate, b.Predicate),
				strings.Compare(a.Source, b.Source))
		})
		state[key] = entity
	}
	return state
}

// requirePublishedStatementOnce fails unless entity holds the statement the test published, once.
func requirePublishedStatementOnce(t *testing.T, entity graph.EntityState) {
	t.Helper()
	var objects []any
	for _, triple := range entity.Triples {
		if triple.Predicate == settlementPredicate {
			objects = append(objects, triple.Object)
		}
	}
	require.Equal(t, []any{settlementObject}, objects, "the entity must hold the published statement once")
}

// TestIntegration_Settlement_ProcessKilledBetweenApplyAndAck holds graph-ingest-recovery, "Recovery on a file stream
// is redelivery", scenario "Process killed between apply and acknowledgement" (design D8, "The settlement process-kill
// test"; task 4.4, Q18). graph-ingest runs in a helper process on a file-backed stream. In the first run its
// applied-sequence write is held, so the input it applies stays unacknowledged; the test kills that process and starts
// a second on the same stream and buckets, which is delivered the input again and acknowledges it. The graph is then
// as one application left it.
func TestIntegration_Settlement_ProcessKilledBetweenApplyAndAck(t *testing.T) {
	ctx, cancelOperation := context.WithTimeout(t.Context(), 180*time.Second)
	defer cancelOperation()
	const (
		entityID = "c360.test.wire.settlement.entity.001"
		guardKey = "c360.test.wire.settlement.entity.001/ENTITY" // entity ID, "/", stream name
	)

	f := natsfixture.New(t)
	require.NoError(t, f.Start(ctx))
	_, err := natsfixture.CreateStream(ctx, f.JetStream(), jetstream.FileStorage, entityStream.name, entityStream.subjects...)
	require.NoError(t, err)
	publisher := natsfixture.Open(t, f.URL(), openFixtureClient)

	first := startSettlementHelper(ctx, t, f, settlementHeld)
	entities, err := f.JetStream().KeyValue(ctx, graph.BucketEntityStates)
	require.NoError(t, err)
	guardRecords, err := f.JetStream().KeyValue(ctx, graph.BucketGraphIngestAppliedSeq)
	require.NoError(t, err)
	seq := publishTriples(ctx, t, publisher, entityID, message.Triple{
		Subject: entityID, Predicate: settlementPredicate, Object: settlementObject, Timestamp: fixtureTime, Confidence: 1.0,
	})

	awaitSettlement(ctx, t, first, "the entity's write", settlementApplyBound,
		func(ctx context.Context) (jetstream.KeyValueEntry, error) { return entities.Get(ctx, entityID) },
		func(entry jetstream.KeyValueEntry) bool { return entry != nil })
	delivery, err := ingestConsumer(ctx, f)
	require.NoError(t, err)
	require.GreaterOrEqual(t, delivery.DeliveredStream, seq, "the input must have been delivered")
	require.Less(t, delivery.AckFloor, seq, "the input must not be acknowledged while its record is held")
	require.Equal(t, 1, delivery.AckPending, "the input must be pending acknowledgement")
	_, err = guardRecords.Get(ctx, guardKey)
	require.ErrorIs(t, err, jetstream.ErrKeyNotFound, "the held write must not have reached the bucket")
	applied := graphState(ctx, t, entities)
	requirePublishedStatementOnce(t, applied[entityID])

	require.True(t, first.Alive(), "the first helper must still be running when the test kills it")
	require.NoError(t, first.Kill())
	waitCtx, cancelWait := context.WithTimeout(ctx, settlementStartBound)
	status, err := first.Wait(waitCtx)
	cancelWait()
	require.NoError(t, err)
	require.Equal(t, syscall.SIGKILL, status.Signal, "the first helper must have ended by the kill")

	second := startSettlementHelper(ctx, t, f, settlementPlain)
	delivery = awaitSettlement(ctx, t, second, "the redelivery's acknowledgement", settlementRedeliveryBound,
		func(ctx context.Context) (ingestDelivery, error) { return ingestConsumer(ctx, f) },
		func(d ingestDelivery) bool { return d.AckFloor >= seq && d.AckPending == 0 })
	require.GreaterOrEqual(t, delivery.Deliveries, uint64(2), "the input must have been delivered again")
	_, err = guardRecords.Get(ctx, guardKey)
	require.NoError(t, err, "the acknowledgement must follow the input's applied-sequence record")
	require.Equal(t, applied, graphState(ctx, t, entities),
		"the graph after the redelivery must equal the graph after one application")
}

// refusedGuardWrite is the durable-record failure test's applied-sequence bucket: a FaultKV whose Put fault refuses
// one write. The refused write clears the fault, reports itself on refused, and returns only once the test closes
// release, so the test inspects the broker while the input is applied and its record refused. One refusal, not a
// fault held until the test clears it: graph-ingest's consumer delivers an input at most three times (MaxDeliver,
// component.GetConsumerConfig's default) and a negative acknowledgement asks for the next delivery at once, so a fault
// cleared from outside would race the delivery limit.
type refusedGuardWrite struct {
	*natsfixture.FaultKV
	refused chan struct{}
	release chan struct{}
}

func (b *refusedGuardWrite) Put(ctx context.Context, key string, value []byte) (uint64, error) {
	revision, err := b.FaultKV.Put(ctx, key, value)
	if errors.Is(err, errGuardWriteRefused) {
		b.FailBefore(natsfixture.KVPut, nil)
		close(b.refused)
		<-b.release
	}
	return revision, err
}

// TestIntegration_Settlement_FailedGuardWriteIsRedelivered holds graph-ingest-recovery, "Settlement order", scenario
// "Durable record fails" (design D8, row "Settlement (Q18)"), on the fixture broker: the entity write succeeds, the
// write of its applied-sequence record fails (FaultKV.FailBefore), and the input is not acknowledged and is delivered
// again. The redelivery's record is written, the redelivery is acknowledged, and the graph is as one application left
// it.
func TestIntegration_Settlement_FailedGuardWriteIsRedelivered(t *testing.T) {
	ctx, cancelOperation := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancelOperation()
	const (
		entityID = "c360.test.wire.settlement.entity.002"
		guardKey = "c360.test.wire.settlement.entity.002/ENTITY" // entity ID, "/", stream name
	)

	f := natsfixture.New(t)
	require.NoError(t, f.Start(ctx))
	_, err := natsfixture.CreateStream(ctx, f.JetStream(), jetstream.FileStorage, entityStream.name, entityStream.subjects...)
	require.NoError(t, err)
	client := natsfixture.Open(t, f.URL(), openFixtureClient)

	var guard *refusedGuardWrite
	owner := startSettlementIngest(ctx, t, client, func(bucket jetstream.KeyValue) jetstream.KeyValue {
		guard = &refusedGuardWrite{
			FaultKV: natsfixture.NewFaultKV(bucket), refused: make(chan struct{}), release: make(chan struct{}),
		}
		guard.FailBefore(natsfixture.KVPut, errGuardWriteRefused)
		return guard
	})
	defer owner.finish(ctx, t)
	require.NotNil(t, guard, "Start must have bound the stream input through the seam")
	var releaseOnce sync.Once
	releaseRefused := func() { releaseOnce.Do(func() { close(guard.release) }) }
	defer releaseRefused() // before owner.finish: Stop must not wait on a held write
	entities, err := f.JetStream().KeyValue(ctx, graph.BucketEntityStates)
	require.NoError(t, err)
	guardRecords, err := f.JetStream().KeyValue(ctx, graph.BucketGraphIngestAppliedSeq)
	require.NoError(t, err)
	seq := publishTriples(ctx, t, client, entityID, message.Triple{
		Subject: entityID, Predicate: settlementPredicate, Object: settlementObject, Timestamp: fixtureTime, Confidence: 1.0,
	})

	refusalCtx, cancelRefusal := context.WithTimeout(ctx, settlementApplyBound)
	defer cancelRefusal()
	select {
	case <-guard.refused:
	case <-refusalCtx.Done():
		t.Fatalf("graph-ingest made no applied-sequence write within %s", settlementApplyBound)
	}
	_, err = entities.Get(ctx, entityID)
	require.NoError(t, err, "the entity write must precede the applied-sequence write")
	delivery, err := ingestConsumer(ctx, f)
	require.NoError(t, err)
	require.Equal(t, uint64(1), delivery.Deliveries, "the input must have been delivered once")
	require.Less(t, delivery.AckFloor, seq, "the input must not be acknowledged while its record is refused")
	require.Equal(t, 1, delivery.AckPending, "the input must be pending acknowledgement")
	applied := graphState(ctx, t, entities)
	requirePublishedStatementOnce(t, applied[entityID])

	releaseRefused()
	delivery = awaitSettlement(ctx, t, nil, "the redelivery's acknowledgement", settlementApplyBound,
		func(ctx context.Context) (ingestDelivery, error) { return ingestConsumer(ctx, f) },
		func(d ingestDelivery) bool { return d.AckFloor >= seq && d.AckPending == 0 })
	require.Equal(t, uint64(2), delivery.Deliveries, "the input must have been delivered again, once")
	require.Equal(t, 1, guard.Calls()[natsfixture.KVPut], "only the redelivery's record write must reach the bucket")
	_, err = guardRecords.Get(ctx, guardKey)
	require.NoError(t, err, "the acknowledgement must follow the input's applied-sequence record")
	require.Equal(t, applied, graphState(ctx, t, entities),
		"the graph after the redelivery must equal the graph after one application")
}
