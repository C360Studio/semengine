package graphingest

// The applied-sequence record is generation-aware (graph-ingest-recovery, "Replay protection is
// generation-aware"; design D8 row #15, task 4.3). A stream deleted and created again starts a new
// generation whose sequences restart at 1, so a sequence is compared only with the last one applied
// from the same generation. The scenario "Stream recreated with lower sequences" runs on a broker in
// guard_generation_integration_test.go; these tests drive processIngest and the consumer setup
// directly.

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	generationEntity = "acme.ops.robotics.gcs.drone.001"
	generationOther  = "acme.ops.robotics.gcs.drone.002"
	// Two generations of the ENTITY stream, as the server's creation times in nanoseconds.
	generationFirst  = uint64(1_700_000_000_000_000_000)
	generationSecond = uint64(1_700_000_100_000_000_000)
	noBucketLogged   = "no applied-sequence bucket"
)

// gatheredCounter reads the unlabelled counter name from registry. A registry that does not gather
// it fails the test and reads as zero, so the rest of the test runs.
func gatheredCounter(t *testing.T, registry *metric.MetricsRegistry, name string) float64 {
	t.Helper()
	family, ok := gatherFamilies(t, registry.PrometheusRegistry())[name]
	if !ok || len(family.GetMetric()) != 1 {
		t.Errorf("the registry does not gather %s", name)
		return 0
	}
	return family.GetMetric()[0].GetCounter().GetValue()
}

// generationWork is an input for id from the ENTITY stream's generation at seq, setting the
// entity's test.state.value to value.
func generationWork(id string, generation, seq uint64, value string, msg *keyedIngestTestMsg) ingestWork {
	entity := &graph.EntityState{
		ID:          id,
		MessageType: testEntityType(),
		Triples: withTestMetadata(message.Triple{
			Subject: id, Predicate: "test.state.value", Object: value, Timestamp: time.Now(), Confidence: 1.0,
		}),
	}
	return ingestWork{entity: entity, msg: msg, consumer: testConsumer, heartbeat: testHeartbeat, entityID: id, stream: "ENTITY", generation: generation, seq: seq}
}

// storedValue reads id's test.state.value and the revision of its stored entry.
func storedValue(t *testing.T, c *Component, id string) (any, uint64) {
	t.Helper()
	state, revision, err := c.readEntity(t.Context(), id)
	if err != nil {
		t.Fatalf("read %s: %v", id, err)
	}
	for _, tr := range state.Triples {
		if tr.Predicate == "test.state.value" {
			return tr.Object, revision
		}
	}
	return nil, revision
}

// deliver runs one input through processIngest and returns how it was settled.
func deliver(t *testing.T, c *Component, work ingestWork) string {
	t.Helper()
	if err := c.processIngest(t.Context(), 0, work); err != nil {
		t.Fatalf("processIngest seq %d of generation %d: %v", work.seq, work.generation, err)
	}
	msg := work.msg.(*keyedIngestTestMsg)
	switch {
	case msg.ack.Load() && !msg.nak.Load() && !msg.term.Load():
		return "ack"
	case msg.nak.Load() && !msg.ack.Load() && !msg.term.Load():
		return "nak"
	case msg.term.Load() && !msg.ack.Load() && !msg.nak.Load():
		return "term"
	}
	return "mixed"
}

// TestGuardRecordIsPerGeneration holds the scenario "Redelivery within one generation" and the rule
// that a newer generation is never suppressed by an older one's sequence. Within one generation an
// input whose sequence is not newer than the last applied is acknowledged and changes nothing, read
// from the lane's cache and, after another entity evicts it, from the stored record. An input from
// the next generation at a lower sequence is applied, and a redelivery within that generation is
// then dropped in turn.
func TestGuardRecordIsPerGeneration(t *testing.T) {
	registry := metric.NewMetricsRegistry()
	c, _ := createTestComponentWithMockKVBucket(t, withAuthority("acme", "ops"), withMetricsRegistry(registry))
	guard := newMockKVBucket()
	c.ingestGuardBucket = c.natsClient.NewKVStore(guard)
	// One cache entry per lane: applying another entity sends the first one's next input to the
	// stored record.
	c.ingestGuardMem = []*laneGuard{newLaneGuard(1)}
	dropped := func() float64 {
		return gatheredCounter(t, registry, "semengine_graph_ingest_redeliveries_dropped_total")
	}

	if got := deliver(t, c, generationWork(generationEntity, generationFirst, 4, "v4", &keyedIngestTestMsg{})); got != "ack" {
		t.Fatalf("seq 4 settled %s, want ack", got)
	}
	if got := deliver(t, c, generationWork(generationEntity, generationFirst, 5, "v5", &keyedIngestTestMsg{})); got != "ack" {
		t.Fatalf("seq 5 settled %s, want ack", got)
	}
	value, revision := storedValue(t, c, generationEntity)
	if value != "v5" {
		t.Fatalf("value after seq 5 = %v, want v5", value)
	}

	// Redelivery of seq 4 within the generation, judged from the lane's cache.
	if got := deliver(t, c, generationWork(generationEntity, generationFirst, 4, "v4", &keyedIngestTestMsg{})); got != "ack" {
		t.Errorf("redelivered seq 4 settled %s, want ack", got)
	}
	if v, r := storedValue(t, c, generationEntity); v != "v5" || r != revision {
		t.Errorf("after the redelivery from the cache: value %v at revision %d, want v5 at %d", v, r, revision)
	}
	if got := dropped(); got != 1 {
		t.Errorf("redeliveries dropped = %v, want 1", got)
	}

	// Another entity takes the cache's one entry, so the next judgement reads the stored record.
	if got := deliver(t, c, generationWork(generationOther, generationFirst, 6, "other", &keyedIngestTestMsg{})); got != "ack" {
		t.Fatalf("other entity settled %s, want ack", got)
	}
	if got := deliver(t, c, generationWork(generationEntity, generationFirst, 5, "v5", &keyedIngestTestMsg{})); got != "ack" {
		t.Errorf("redelivered seq 5 settled %s, want ack", got)
	}
	if v, r := storedValue(t, c, generationEntity); v != "v5" || r != revision {
		t.Errorf("after the redelivery from the stored record: value %v at revision %d, want v5 at %d", v, r, revision)
	}
	if got := dropped(); got != 2 {
		t.Errorf("redeliveries dropped = %v, want 2", got)
	}

	// The stream is recreated: its next generation restarts at sequence 1.
	if got := deliver(t, c, generationWork(generationEntity, generationSecond, 1, "current", &keyedIngestTestMsg{})); got != "ack" {
		t.Errorf("seq 1 of the next generation settled %s, want ack", got)
	}
	value, revision = storedValue(t, c, generationEntity)
	if value != "current" {
		t.Errorf("value after seq 1 of the next generation = %v, want current", value)
	}
	if got := dropped(); got != 2 {
		t.Errorf("redeliveries dropped after the next generation's input = %v, want 2", got)
	}

	// Within the next generation the guard holds again, from the stored record.
	if got := deliver(t, c, generationWork(generationOther, generationSecond, 2, "other", &keyedIngestTestMsg{})); got != "ack" {
		t.Fatalf("other entity in the next generation settled %s, want ack", got)
	}
	if got := deliver(t, c, generationWork(generationEntity, generationSecond, 1, "stale", &keyedIngestTestMsg{})); got != "ack" {
		t.Errorf("redelivered seq 1 of the next generation settled %s, want ack", got)
	}
	if v, r := storedValue(t, c, generationEntity); v != "current" || r != revision {
		t.Errorf("after the redelivery in the next generation: value %v at revision %d, want current at %d", v, r, revision)
	}
	if got := dropped(); got != 3 {
		t.Errorf("redeliveries dropped = %v, want 3", got)
	}
}

// TestNoGuardBucketLogsAndCountsFirstSeenInputs holds the degrade of design D8: a component with no
// applied-sequence bucket, which only one built without Start can be, applies each input it has not
// seen as first seen, and logs and counts each one. An input the lane's cache has seen is judged
// there, and is neither logged nor counted.
func TestNoGuardBucketLogsAndCountsFirstSeenInputs(t *testing.T) {
	registry := metric.NewMetricsRegistry()
	c, _ := createTestComponentWithMockKVBucket(t, withAuthority("acme", "ops"), withMetricsRegistry(registry))
	var logs bytes.Buffer
	c.logger = slog.New(slog.NewTextHandler(&logs, nil))
	c.ingestGuardMem = []*laneGuard{newLaneGuard(16)}
	if c.ingestGuardBucket != nil {
		t.Fatal("the hand-built component has an applied-sequence bucket")
	}
	const counter = "semengine_graph_ingest_unguarded_inputs_total"

	for _, id := range []string{generationEntity, generationOther} {
		if got := deliver(t, c, generationWork(id, generationFirst, 1, "v1", &keyedIngestTestMsg{})); got != "ack" {
			t.Errorf("%s settled %s, want ack", id, got)
		}
		if v, _ := storedValue(t, c, id); v != "v1" {
			t.Errorf("%s value = %v, want v1", id, v)
		}
	}
	if got := gatheredCounter(t, registry, counter); got != 2 {
		t.Errorf("%s after two first-seen inputs = %v, want 2", counter, got)
	}
	if got := strings.Count(logs.String(), noBucketLogged); got != 2 {
		t.Errorf("degrade logged %d times for two first-seen inputs, want 2:\n%s", got, logs.String())
	}
	for _, id := range []string{generationEntity, generationOther} {
		if !strings.Contains(logs.String(), "entity_id="+id) {
			t.Errorf("the degrade log does not name %s:\n%s", id, logs.String())
		}
	}

	if got := deliver(t, c, generationWork(generationEntity, generationFirst, 1, "v1", &keyedIngestTestMsg{})); got != "ack" {
		t.Errorf("redelivery settled %s, want ack", got)
	}
	if got := gatheredCounter(t, registry, counter); got != 2 {
		t.Errorf("%s after a redelivery the cache judged = %v, want 2", counter, got)
	}
	if got := strings.Count(logs.String(), noBucketLogged); got != 2 {
		t.Errorf("degrade logged %d times after a redelivery the cache judged, want 2", got)
	}
}

// TestStreamWithoutCreationTimeIsRefused: the generation is the stream's creation time on the
// server. A stream that reports none would make every generation look the same, so graph-ingest
// refuses to bind a consumer to it and setup fails.
func TestStreamWithoutCreationTimeIsRefused(t *testing.T) {
	c, _ := createTestComponentWithMockKVBucket(t, withAuthority("acme", "ops"))
	if err := c.buildIngestPool(t.Context()); err != nil {
		t.Fatalf("buildIngestPool: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := c.ingestPool.Shutdown(ctx); err != nil {
			t.Errorf("ingest pool Shutdown: %v", err)
		}
	})
	c.waitForStreamInput = func(context.Context, string) (time.Time, error) { return time.Time{}, nil }
	bound := 0
	c.consumeStream = func(
		context.Context,
		natsclient.PortConsumerContext,
		natsclient.StreamConsumerConfig,
		func(context.Context, jetstream.Msg),
	) (jetstream.ConsumeContext, error) {
		bound++
		return nil, nil
	}
	err := c.setupSubscriptions(t.Context(), t.Context())
	if err == nil || !strings.Contains(err.Error(), "no creation time") {
		t.Errorf("setupSubscriptions error = %v, want a refusal naming the missing creation time", err)
	}
	if bound != 0 {
		t.Errorf("%d consumers were bound to a stream with no creation time, want 0", bound)
	}
}

// heldEntity is the entity of the input the consumer-setup tests deliver, under the c360.test
// authority their component is built with.
const heldEntity = "c360.test.held.generation.entity.001"

// heldInputComponent builds, inside the calling bubble, a component over mock entity and
// applied-sequence buckets with its ingest pool, and returns it with the two buckets. The pool is
// shut down before the bubble ends.
func heldInputComponent(t *testing.T) (*Component, *mockKVBucket, *mockKVBucket) {
	t.Helper()
	c, entities := createTestComponentWithMockKVBucket(t, withAuthority("c360", "test"))
	registerMergeTestPayload(t, c)
	guard := newMockKVBucket()
	c.ingestGuardBucket = c.natsClient.NewKVStore(guard)
	if err := c.buildIngestPool(t.Context()); err != nil {
		t.Fatalf("buildIngestPool: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := c.ingestPool.Shutdown(ctx); err != nil {
			t.Errorf("ingest pool Shutdown: %v", err)
		}
	})
	return c, entities, guard
}

// heldInputMsg is an input the consume callback accepts: a test.merge.v1 message for heldEntity
// at sequence 1 of the ENTITY stream.
func heldInputMsg(t *testing.T) *streamLaneMsg {
	t.Helper()
	payload := &mergeTestGraphable{entityID: heldEntity, triples: []message.Triple{
		{Subject: heldEntity, Predicate: "test.state.value", Object: "held", Timestamp: time.Now(), Confidence: 1},
	}}
	return newStreamLaneMsg(streamLaneData(t, payload, "held-producer"), 1)
}

// handlerOnItsOwnGoroutine is a consumeStream that runs the callback on msg on a goroutine of its
// own, with handlerCtx, and binds at once with no handle.
func handlerOnItsOwnGoroutine(handlerCtx context.Context, msg jetstream.Msg) func(
	context.Context, natsclient.PortConsumerContext, natsclient.StreamConsumerConfig, func(context.Context, jetstream.Msg),
) (jetstream.ConsumeContext, error) {
	return func(
		_ context.Context,
		_ natsclient.PortConsumerContext,
		_ natsclient.StreamConsumerConfig,
		handler func(context.Context, jetstream.Msg),
	) (jetstream.ConsumeContext, error) {
		go handler(handlerCtx, msg)
		return nil, nil
	}
}

// onlyDisposition returns msg's one disposition, or "none" when it has none yet. A second
// disposition fails the test.
func onlyDisposition(t *testing.T, msg *streamLaneMsg) string {
	t.Helper()
	select {
	case disposition := <-msg.settled:
		select {
		case extra := <-msg.settled:
			t.Errorf("a second disposition after %s: %s", disposition, extra)
		default:
		}
		return disposition
	default:
		return "none"
	}
}

// keyCount is the number of keys in m.
func keyCount(m *mockKVBucket) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.data)
}

// TestConsumerInputHeldUntilGenerationConfirmed holds design D8, "The generation belongs to the bound
// consumer" (Codex interim review R3, PR #93 comment 6112098987). Start reads the stream's creation
// time, binds the consumer, and reads it again. An input the callback accepts in between is neither
// submitted nor settled until the second read returns: applied with the generation when the two
// reads agree, negatively acknowledged when they differ, and negatively acknowledged when its
// delivery context ends first, so the hold never waits on a client whose Close waits on it.
func TestConsumerInputHeldUntilGenerationConfirmed(t *testing.T) {
	first := time.Unix(0, int64(generationFirst))
	second := time.Unix(0, int64(generationSecond))

	t.Run("confirmed", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			c, entities, guard := heldInputComponent(t)
			msg := heldInputMsg(t)
			release := make(chan struct{})
			reads := 0
			c.waitForStreamInput = func(ctx context.Context, _ string) (time.Time, error) {
				reads++
				if reads > 1 {
					select {
					case <-release:
					case <-ctx.Done():
						return time.Time{}, ctx.Err()
					}
				}
				return first, nil
			}
			c.consumeStream = handlerOnItsOwnGoroutine(t.Context(), msg)
			setupErr := make(chan error, 1)
			go func() { setupErr <- c.setupSubscriptions(t.Context(), t.Context()) }()

			synctest.Wait()
			if got := onlyDisposition(t, msg); got != "none" {
				t.Fatalf("the input was settled (%s) before Start read the generation again", got)
			}
			if n := keyCount(entities) + keyCount(guard); n != 0 {
				t.Fatalf("%d keys written before Start read the generation again, want 0", n)
			}

			close(release)
			if err := <-setupErr; err != nil {
				t.Fatalf("setupSubscriptions: %v", err)
			}
			synctest.Wait()
			if got := onlyDisposition(t, msg); got != "ack" {
				t.Errorf("disposition after the generation was confirmed = %s, want ack", got)
			}
			entry, err := guard.Get(t.Context(), guardKey(heldEntity, "ENTITY"))
			if err != nil {
				t.Fatalf("read the applied-sequence record: %v", err)
			}
			record, ok := decodeGuardRecord(entry.Value())
			if !ok || record != (guardRecord{generation: generationFirst, seq: 1}) {
				t.Errorf("applied-sequence record = %+v (decoded %t), want generation %d at sequence 1",
					record, ok, generationFirst)
			}
		})
	})

	t.Run("changed", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			c, entities, guard := heldInputComponent(t)
			msg := heldInputMsg(t)
			reads := 0
			c.waitForStreamInput = func(context.Context, string) (time.Time, error) {
				reads++
				if reads == 1 {
					return first, nil
				}
				return second, nil
			}
			c.consumeStream = handlerOnItsOwnGoroutine(t.Context(), msg)

			err := c.setupSubscriptions(t.Context(), t.Context())
			if err == nil || !strings.Contains(err.Error(), "ENTITY") {
				t.Errorf("setupSubscriptions error = %v, want a failure naming the stream ENTITY", err)
			}
			synctest.Wait()
			if got := onlyDisposition(t, msg); got != "nak" {
				t.Errorf("disposition after the generation changed = %s, want nak", got)
			}
			if n := keyCount(entities) + keyCount(guard); n != 0 {
				t.Errorf("%d keys written after the generation changed, want 0", n)
			}
		})
	})

	t.Run("delivery context ends", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			c, _, _ := heldInputComponent(t)
			msg := heldInputMsg(t)
			c.waitForStreamInput = func(context.Context, string) (time.Time, error) { return first, nil }
			handlerCtx, cancelHandlers := context.WithCancel(t.Context())
			defer cancelHandlers()
			clientClosed := errors.New("client closed while starting consumer")
			var ended string
			// natsclient's commitConsumer when the client's Close begins during setup: the running
			// handlers' context is cancelled, and setup waits until they have returned, or until its
			// own context ends (natsclient/owned_delivery.go:181-194).
			c.consumeStream = func(
				ctx context.Context,
				_ natsclient.PortConsumerContext,
				_ natsclient.StreamConsumerConfig,
				handler func(context.Context, jetstream.Msg),
			) (jetstream.ConsumeContext, error) {
				returned := make(chan struct{})
				go func() {
					defer close(returned)
					handler(handlerCtx, msg)
				}()
				synctest.Wait()
				cancelHandlers()
				select {
				case <-returned:
					ended = "the handler returned"
					return nil, clientClosed
				case <-ctx.Done():
					ended = "the setup context ended"
					return nil, ctx.Err()
				}
			}
			setupCtx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()

			err := c.setupSubscriptions(setupCtx, t.Context())
			if ended != "the handler returned" {
				t.Errorf("the setup's wait ended because %s, want: the handler returned", ended)
			}
			if !errors.Is(err, clientClosed) {
				t.Errorf("setupSubscriptions error = %v, want it to wrap the client's close", err)
			}
			synctest.Wait()
			if got := onlyDisposition(t, msg); got != "nak" {
				t.Errorf("disposition after the delivery context ended = %s, want nak", got)
			}
		})
	})
}
