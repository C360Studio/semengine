//go:build integration

package graphingest

import (
	"cmp"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/internal/harness/prochost"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/natsclient"
)

// The helpers the settlement process-kill test starts (design D8, "The settlement process-kill test"), and what the
// test passes each in its environment.
const (
	// settlementHeld runs graph-ingest with every input held after its entity write: the first run.
	settlementHeld = "graph-ingest-held-after-apply"
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

// TestHelperProcess runs a settlement helper in a child the test started with prochost.Start; in a normal run each
// Helper call returns at once.
func TestHelperProcess(t *testing.T) {
	prochost.Helper(settlementHeld, func() { runSettlementHelper(t, true) })
	prochost.Helper(settlementPlain, func() { runSettlementHelper(t, false) })
}

// runSettlementHelper starts graph-ingest on the broker named in the environment, creates the ready file, and parks
// until a signal ends the process; it never stops graph-ingest. With holdApplied, graph-ingest's logger handler pauses
// on "Entity ingested" and is never released (fix-301 design D2, "The pause"), so an input it applies is never given
// its applied-sequence record or acknowledged. A failure fails TestHelperProcess, and the child exits.
func runSettlementHelper(t *testing.T, holdApplied bool) {
	ctx := t.Context()
	client, _, err := openFixtureClient(ctx, os.Getenv(helperNATSURL))
	require.NoError(t, err)
	var handler slog.Handler
	if holdApplied {
		handler = newPauseOnRecord("Entity ingested") // nothing releases it
	}
	startSettlementIngest(ctx, t, client, handler)
	ready := os.Getenv(helperReady)
	require.NoError(t, os.WriteFile(ready+".tmp", nil, 0o600))
	require.NoError(t, os.Rename(ready+".tmp", ready))
	terminated := make(chan os.Signal, 1)
	signal.Notify(terminated, syscall.SIGTERM)
	<-terminated
}

// startSettlementIngest starts graph-ingest with its default configuration on client under the c360.test authority,
// as startIngestOn does. With handler set, it is the handler of graph-ingest's logger.
func startSettlementIngest(ctx context.Context, t *testing.T, client *natsclient.Client,
	handler slog.Handler) *graphIngestTestOwner {
	t.Helper()
	cfgJSON, err := json.Marshal(DefaultConfig())
	require.NoError(t, err)
	deps := testDependencies(t, client, withAuthority("c360", "test"))
	if handler != nil {
		deps.Logger = slog.New(handler)
	}
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

// startSettlementHelper starts the helper name against url and waits until it reports graph-ingest started.
func startSettlementHelper(ctx context.Context, t *testing.T, url, name string) *prochost.Process {
	t.Helper()
	ready := filepath.Join(t.TempDir(), "ready")
	p, err := prochost.Start(t, name, helperNATSURL+"="+url, helperReady+"="+ready)
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
func ingestConsumer(ctx context.Context, js jetstream.JetStream) (ingestDelivery, error) {
	consumer, err := js.Consumer(ctx, entityStream.name, restartConsumer)
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
		require.NoError(t, graph.UnmarshalEntityState(key, entry.Value(), &entity))
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
// test"; task 4.4, Q18). graph-ingest runs in a helper process on a file-backed stream. In the first run its logger
// handler pauses for good on "Entity ingested", after the entity write and before the applied-sequence record, so the
// input it applies stays unacknowledged (fix-301 design D2, "The pause"); the test kills that process and starts
// a second on the same stream and buckets, which is delivered the input again and acknowledges it. The graph is then
// as one application left it.
func TestIntegration_Settlement_ProcessKilledBetweenApplyAndAck(t *testing.T) {
	ctx, cancelOperation := context.WithTimeout(t.Context(), 180*time.Second)
	defer cancelOperation()
	const (
		entityID = "c360.test.wire.settlement.entity.001"
		guardKey = "c360.test.wire.settlement.entity.001/ENTITY" // entity ID, "/", stream name
	)

	url := shared.Reset(t)
	publisher := natsfixture.Open(t, url, openFixtureClient)
	js, err := publisher.JetStream()
	require.NoError(t, err)
	_, err = natsfixture.CreateStream(ctx, js, jetstream.FileStorage, entityStream.name, entityStream.subjects...)
	require.NoError(t, err)

	first := startSettlementHelper(ctx, t, url, settlementHeld)
	entities, err := js.KeyValue(ctx, graph.BucketEntityStates)
	require.NoError(t, err)
	guardRecords, err := js.KeyValue(ctx, graph.BucketGraphIngestAppliedSeq)
	require.NoError(t, err)
	seq := publishTriples(ctx, t, publisher, entityID, message.Triple{
		Subject: entityID, Predicate: settlementPredicate, Object: settlementObject, Timestamp: fixtureTime, Confidence: 1.0,
	})

	awaitSettlement(ctx, t, first, "the entity's write", settlementApplyBound,
		func(ctx context.Context) (jetstream.KeyValueEntry, error) { return entities.Get(ctx, entityID) },
		func(entry jetstream.KeyValueEntry) bool { return entry != nil })
	delivery, err := ingestConsumer(ctx, js)
	require.NoError(t, err)
	require.GreaterOrEqual(t, delivery.DeliveredStream, seq, "the input must have been delivered")
	require.Less(t, delivery.AckFloor, seq, "the input must not be acknowledged while its application is held")
	require.Equal(t, 1, delivery.AckPending, "the input must be pending acknowledgement")
	_, err = guardRecords.Get(ctx, guardKey)
	require.ErrorIs(t, err, jetstream.ErrKeyNotFound, "the held application must not have written its record")
	applied := graphState(ctx, t, entities)
	requirePublishedStatementOnce(t, applied[entityID])

	require.True(t, first.Alive(), "the first helper must still be running when the test kills it")
	require.NoError(t, first.Kill())
	waitCtx, cancelWait := context.WithTimeout(ctx, settlementStartBound)
	status, err := first.Wait(waitCtx)
	cancelWait()
	require.NoError(t, err)
	require.Equal(t, syscall.SIGKILL, status.Signal, "the first helper must have ended by the kill")

	second := startSettlementHelper(ctx, t, url, settlementPlain)
	delivery = awaitSettlement(ctx, t, second, "the redelivery's acknowledgement", settlementRedeliveryBound,
		func(ctx context.Context) (ingestDelivery, error) { return ingestConsumer(ctx, js) },
		func(d ingestDelivery) bool { return d.AckFloor >= seq && d.AckPending == 0 })
	require.GreaterOrEqual(t, delivery.Deliveries, uint64(2), "the input must have been delivered again")
	_, err = guardRecords.Get(ctx, guardKey)
	require.NoError(t, err, "the acknowledgement must follow the input's applied-sequence record")
	require.Equal(t, applied, graphState(ctx, t, entities),
		"the graph after the redelivery must equal the graph after one application")
}
