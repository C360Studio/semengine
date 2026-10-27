package graphingest

// The applied-sequence record is generation-aware (graph-ingest-recovery, "Replay protection uses
// the generation read before the bind"; design D8 row #15, task 4.3). A stream deleted and created
// again starts a new generation whose sequences restart at 1, so a sequence is compared only with the
// last one applied from the same generation. The scenario "Stream recreated with lower sequences" runs
// on a broker in guard_generation_integration_test.go; these tests drive processIngest and the
// consumer setup directly.

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/metric"
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

// keyCount is the number of keys in m.
func keyCount(m *mockKVBucket) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.data)
}
