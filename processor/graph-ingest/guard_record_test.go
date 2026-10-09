package graphingest

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/metric"
	"github.com/nats-io/nats.go/jetstream"
)

// The two entities under test and the first one's guard key, written out from guardKey's grammar
// (entity ID, "/", stream name) rather than computed by it.
const (
	guardRecordID     = "acme.ops.robotics.gcs.drone.001"
	guardRecordOther  = "acme.ops.robotics.gcs.drone.002"
	guardRecordStream = "ENTITY"
	guardRecordKey    = "acme.ops.robotics.gcs.drone.001/ENTITY"
	guardRecordLogged = "applied-sequence record cannot be decoded"
)

// guardRecordRefusals reads semengine_graph_ingest_guard_record_refusals_total from registry. A
// registry that does not gather it fails the test and reads as zero, so the rest of the test runs.
func guardRecordRefusals(t *testing.T, registry *metric.MetricsRegistry) float64 {
	t.Helper()
	family, ok := gatherFamilies(t, registry.PrometheusRegistry())["semengine_graph_ingest_guard_record_refusals_total"]
	if !ok || len(family.GetMetric()) != 1 {
		t.Error("the registry does not gather semengine_graph_ingest_guard_record_refusals_total")
		return 0
	}
	return family.GetMetric()[0].GetCounter().GetValue()
}

func guardRecordWork(id string, seq uint64, msg *keyedIngestTestMsg) ingestWork {
	entity := &graph.EntityState{
		ID:          id,
		MessageType: testEntityType(),
		Triples: withTestMetadata(message.Triple{
			Subject: id, Predicate: "test.state.value", Object: "ready", Timestamp: time.Now(), Confidence: 1.0,
		}),
	}
	return ingestWork{entity: entity, msg: msg, entityID: id, stream: guardRecordStream, seq: seq}
}

// TestCorruptGuardRecordIsRefused holds graph-ingest-recovery, "A record that cannot be decoded"
// (design D21): a stored applied-sequence record graph-ingest cannot decode is refused. The input
// is neither applied nor acknowledged, so it is delivered again; each refusal is counted, and the
// key is logged once. Once the key is deleted the redelivery applies and is acknowledged. When the
// same key is later corrupted again, that is a new refusal, logged again.
func TestCorruptGuardRecordIsRefused(t *testing.T) {
	registry := metric.NewMetricsRegistry()
	c, entities := createTestComponentWithMockKVBucket(t, withAuthority("acme", "ops"), withMetricsRegistry(registry))
	var logs bytes.Buffer
	c.logger = slog.New(slog.NewTextHandler(&logs, nil))
	guard := newMockKVBucket()
	c.ingestGuardBucket = c.natsClient.NewKVStore(guard)
	// The lane's cache holds one entry, so applying a second entity sends the first entity's next
	// arrival back to the stored record.
	c.ingestGuardMem = []*laneGuard{newLaneGuard(1)}

	corrupt := func() {
		t.Helper()
		guard.mu.Lock()
		guard.data[guardRecordKey] = mockKVData{value: []byte{0, 0, 1}, revision: 1}
		guard.mu.Unlock()
	}
	entityStored := func(id string) bool {
		t.Helper()
		entities.mu.Lock()
		defer entities.mu.Unlock()
		_, ok := entities.data[id]
		return ok
	}
	loggedLines := func() int { return strings.Count(logs.String(), guardRecordLogged) }
	refused := func(name string, seq uint64) {
		t.Helper()
		msg := &keyedIngestTestMsg{}
		if err := c.processIngest(t.Context(), 0, guardRecordWork(guardRecordID, seq, msg)); err == nil {
			t.Errorf("%s: processIngest returned nil for an input whose guard record cannot be decoded", name)
		}
		if !msg.nak.Load() || msg.ack.Load() || msg.term.Load() {
			t.Errorf("%s: settlement nak %v, ack %v, term %v; want only a negative acknowledgement",
				name, msg.nak.Load(), msg.ack.Load(), msg.term.Load())
		}
	}
	applied := func(name, id string, seq uint64) {
		t.Helper()
		msg := &keyedIngestTestMsg{}
		if err := c.processIngest(t.Context(), 0, guardRecordWork(id, seq, msg)); err != nil {
			t.Errorf("%s: processIngest: %v", name, err)
		}
		if !msg.ack.Load() || msg.nak.Load() || msg.term.Load() {
			t.Errorf("%s: settlement ack %v, nak %v, term %v; want only an acknowledgement",
				name, msg.ack.Load(), msg.nak.Load(), msg.term.Load())
		}
		if !entityStored(id) {
			t.Errorf("%s: entity %s was not stored", name, id)
		}
	}

	corrupt()
	refused("first delivery", 1)
	if entityStored(guardRecordID) {
		t.Fatal("the entity was stored although its guard record cannot be decoded")
	}
	if got := guardRecordRefusals(t, registry); got != 1 {
		t.Errorf("refusals after the first delivery = %v, want 1", got)
	}
	if got := loggedLines(); got != 1 {
		t.Errorf("refusal logged %d times after the first delivery, want 1", got)
	}
	if !strings.Contains(logs.String(), "key="+guardRecordKey) {
		t.Errorf("the refusal log does not name the key %s:\n%s", guardRecordKey, logs.String())
	}

	refused("redelivery", 1)
	if got := guardRecordRefusals(t, registry); got != 2 {
		t.Errorf("refusals after the redelivery = %v, want 2", got)
	}
	if got := loggedLines(); got != 1 {
		t.Errorf("refusal logged %d times after the redelivery, want once for the key", got)
	}

	if err := guard.Delete(context.Background(), guardRecordKey); err != nil {
		t.Fatalf("delete the guard record: %v", err)
	}
	applied("redelivery after the key is deleted", guardRecordID, 1)
	guard.mu.Lock()
	stamped := guard.data[guardRecordKey].value
	guard.mu.Unlock()
	if want := []byte{0, 0, 0, 0, 0, 0, 0, 1}; !bytes.Equal(stamped, want) {
		t.Errorf("guard record after the apply = %v, want %v", stamped, want)
	}
	if got := guardRecordRefusals(t, registry); got != 2 {
		t.Errorf("refusals after the apply = %v, want 2", got)
	}

	// The key is corrupted again. Applying another entity evicts the first from the lane's cache,
	// so the first entity's next arrival reads the stored record: a new refusal, logged again.
	corrupt()
	applied("another entity", guardRecordOther, 1)
	refused("arrival after the key is corrupted again", 2)
	if got := guardRecordRefusals(t, registry); got != 3 {
		t.Errorf("refusals after the key is corrupted again = %v, want 3", got)
	}
	if got := loggedLines(); got != 2 {
		t.Errorf("refusal logged %d times after the key is corrupted again, want 2", got)
	}

	// Repaired this time by writing a record that decodes: the arrival applies, and the key's next
	// corruption is again a new refusal, logged again.
	if _, err := guard.Put(context.Background(), guardRecordKey, []byte{0, 0, 0, 0, 0, 0, 0, 1}); err != nil {
		t.Fatalf("write a decodable guard record: %v", err)
	}
	applied("arrival after a decodable record is written", guardRecordID, 3)
	corrupt()
	applied("another entity again", guardRecordOther, 2)
	refused("arrival after the key is corrupted a third time", 4)
	if got := loggedLines(); got != 3 {
		t.Errorf("refusal logged %d times after the key is corrupted a third time, want 3", got)
	}
}

// TestGuardRecordOfAnyOtherLengthIsRefused: graph-ingest writes its applied-sequence record as
// eight bytes, so a record of any other length, shorter or longer, cannot be decoded and is
// refused; a longer one is not read by its first eight bytes.
func TestGuardRecordOfAnyOtherLengthIsRefused(t *testing.T) {
	for _, size := range []int{0, 7, 9, 16} {
		registry := metric.NewMetricsRegistry()
		c, entities := createTestComponentWithMockKVBucket(t, withAuthority("acme", "ops"), withMetricsRegistry(registry))
		guard := newMockKVBucket()
		guard.data[guardRecordKey] = mockKVData{value: make([]byte, size), revision: 1}
		c.ingestGuardBucket = c.natsClient.NewKVStore(guard)
		c.ingestGuardMem = []*laneGuard{newLaneGuard(16)}

		msg := &keyedIngestTestMsg{}
		if err := c.processIngest(t.Context(), 0, guardRecordWork(guardRecordID, 1, msg)); err == nil {
			t.Errorf("%d-byte record: processIngest returned nil", size)
		}
		if !msg.nak.Load() || msg.ack.Load() || msg.term.Load() {
			t.Errorf("%d-byte record: settlement nak %v, ack %v, term %v; want only a negative acknowledgement",
				size, msg.nak.Load(), msg.ack.Load(), msg.term.Load())
		}
		entities.mu.Lock()
		_, stored := entities.data[guardRecordID]
		entities.mu.Unlock()
		if stored {
			t.Errorf("%d-byte record: the entity was stored", size)
		}
		if got := guardRecordRefusals(t, registry); got != 1 {
			t.Errorf("%d-byte record: refusals = %v, want 1", size, got)
		}
	}
}

// TestGuardReadFailureIsNotARefusal: a guard record that cannot be read is not one that cannot be
// decoded. The input is still neither applied nor acknowledged, but each delivery is warned about
// and none is counted as a refusal.
func TestGuardReadFailureIsNotARefusal(t *testing.T) {
	registry := metric.NewMetricsRegistry()
	c, entities := createTestComponentWithMockKVBucket(t, withAuthority("acme", "ops"), withMetricsRegistry(registry))
	var logs bytes.Buffer
	c.logger = slog.New(slog.NewTextHandler(&logs, nil))
	guard := newMockKVBucket()
	guard.getFunc = func(context.Context, string) (jetstream.KeyValueEntry, error) {
		return nil, errInjectedRefusal
	}
	c.ingestGuardBucket = c.natsClient.NewKVStore(guard)
	c.ingestGuardMem = []*laneGuard{newLaneGuard(16)}

	for delivery := 1; delivery <= 2; delivery++ {
		msg := &keyedIngestTestMsg{}
		if err := c.processIngest(t.Context(), 0, guardRecordWork(guardRecordID, 1, msg)); !errors.Is(err, errInjectedRefusal) {
			t.Errorf("delivery %d: processIngest error = %v, want the read failure", delivery, err)
		}
		if !msg.nak.Load() || msg.ack.Load() || msg.term.Load() {
			t.Errorf("delivery %d: settlement nak %v, ack %v, term %v; want only a negative acknowledgement",
				delivery, msg.nak.Load(), msg.ack.Load(), msg.term.Load())
		}
	}
	entities.mu.Lock()
	_, stored := entities.data[guardRecordID]
	entities.mu.Unlock()
	if stored {
		t.Error("the entity was stored although its guard record could not be read")
	}
	if got := guardRecordRefusals(t, registry); got != 0 {
		t.Errorf("refusals after two failed reads = %v, want 0", got)
	}
	if got := strings.Count(logs.String(), "redelivery-guard read failed"); got != 2 {
		t.Errorf("read failure warned %d times over two deliveries, want 2", got)
	}
	if strings.Contains(logs.String(), guardRecordLogged) {
		t.Errorf("a read failure was logged as an undecodable record:\n%s", logs.String())
	}
}
