package graphingest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/vocabulary"
	"github.com/nats-io/nats.go/jetstream"
)

// The stream lane's write (replaceEntity) is a compare-and-set that the bucket retries after a
// create conflict or a create no stream received (jetstream.ErrNoStreamResponse). Each retry must
// start from the arrival as it came and the stored state it reads (PR #93 comment 6060120246).
// These tests make the entity's first create fail once and read what the retry stored.

// createFailsOnce is the in-memory entity bucket with one failing create: the first create of
// key runs beforeFail, when set (another lane's write), keeps the value it was given and returns
// err. Every other create is mockKVBucket's own.
type createFailsOnce struct {
	*mockKVBucket
	key        string
	err        error
	beforeFail func()
	failed     atomic.Bool
	abandoned  []byte
}

func (b *createFailsOnce) Create(ctx context.Context, key string, value []byte, opts ...jetstream.KVCreateOpt) (uint64, error) {
	if key == b.key && b.failed.CompareAndSwap(false, true) {
		if b.beforeFail != nil {
			b.beforeFail()
		}
		b.abandoned = value
		return 0, b.err
	}
	return b.mockKVBucket.Create(ctx, key, value, opts...)
}

// newRetryingStreamLane builds graph-ingest with hierarchy on, over a bucket whose first create
// of the drone's key fails with err, and the drone's arrival.
func newRetryingStreamLane(t *testing.T, err error) (*Component, *createFailsOnce, *graph.EntityState) {
	t.Helper()
	const id = "c360.platform.robotics.mav1.drone.001"
	c, mock := createTestComponentWithMockKVBucket(t)
	c.config.EnableHierarchy = true
	buildHierarchyInference(t, c)
	bucket := &createFailsOnce{mockKVBucket: mock, key: id, err: err}
	c.entityBucket = c.natsClient.NewKVStore(bucket)

	now := time.Now()
	arrival := &graph.EntityState{
		ID:          id,
		MessageType: testEntityType(),
		Triples: withTestMetadata(
			message.Triple{Subject: id, Predicate: "robotics.status.armed", Object: true, Timestamp: now},
		),
		UpdatedAt: now,
	}
	return c, bucket, arrival
}

func decodeEntity(t *testing.T, key string, data []byte) graph.EntityState {
	t.Helper()
	var state graph.EntityState
	if err := graph.UnmarshalEntityState(key, data, &state); err != nil {
		t.Fatalf("decode entity: %v", err)
	}
	return state
}

// statementKey is a stored statement's identity for these tests: two statements with the same
// key are the same statement stored twice.
func statementKey(tr message.Triple) string {
	return fmt.Sprintf("%s|%s|%v|%s|%s", tr.Subject, tr.Predicate, tr.Object, tr.Source, tr.Timestamp.UTC().Format(time.RFC3339Nano))
}

func isHierarchyPredicate(predicate string) bool {
	switch predicate {
	case vocabulary.HierarchyTypeMember, vocabulary.HierarchySystemMember,
		vocabulary.HierarchyDomainMember, vocabulary.HierarchyTypeSibling:
		return true
	}
	return false
}

// TestReplaceEntityRetryAfterCreateErrorStoresNoStatementTwice: the birth's create fails once
// with jetstream.ErrNoStreamResponse, the one error other than a conflict after which the bucket
// runs the callback again (no stream stored the write; task 4.18), so the key is still absent and
// the bucket retries the birth. The entity is born with each statement once, hierarchy statements
// included.
func TestReplaceEntityRetryAfterCreateErrorStoresNoStatementTwice(t *testing.T) {
	c, bucket, arrival := newRetryingStreamLane(t, jetstream.ErrNoStreamResponse)

	if err := c.mergeEntityOnLane(t.Context(), arrival, false); err != nil {
		t.Fatalf("mergeEntityOnLane: %v", err)
	}
	if !bucket.failed.Load() {
		t.Fatal("the drone's create never failed, so no retry was exercised")
	}

	stored := storedEntity(t, c, arrival.ID)
	hierarchy := 0
	seen := make(map[string]int)
	for _, tr := range stored.Triples {
		if isHierarchyPredicate(tr.Predicate) {
			hierarchy++
		}
		seen[statementKey(tr)]++
	}
	if hierarchy == 0 {
		t.Fatalf("birth carries no hierarchy statement, so hierarchy did not run: %v", stored.Triples)
	}
	for key, n := range seen {
		if n > 1 {
			t.Errorf("statement stored %d times: %s", n, key)
		}
	}
}

// TestReplaceEntityRetryAfterLostBirthMergesOnlyTheArrival: another lane stores the key, then
// this lane's create fails with the conflict error, so the retry merges into the other lane's
// entity. The stored entity carries none of the hierarchy statements this lane added to its
// abandoned birth. (The profile is not compared: the merge stamps the unprofiled entity's
// profile from the same arrival, so it is the same statement as the abandoned birth's.)
func TestReplaceEntityRetryAfterLostBirthMergesOnlyTheArrival(t *testing.T) {
	c, bucket, arrival := newRetryingStreamLane(t, jetstream.ErrKeyExists)
	bucket.beforeFail = func() {
		otherLane := graph.EntityState{
			ID:          arrival.ID,
			MessageType: testEntityType(),
			Triples: withTestMetadata(
				message.Triple{Subject: arrival.ID, Predicate: "robotics.status.mode", Object: "manual", Timestamp: time.Now()},
			),
			UpdatedAt: time.Now(),
		}
		data, err := graph.MarshalEntityState(&otherLane)
		if err != nil {
			t.Errorf("encode the other lane's entity: %v", err)
			return
		}
		if _, err := bucket.mockKVBucket.Put(t.Context(), arrival.ID, data); err != nil {
			t.Errorf("store the other lane's entity: %v", err)
		}
	}

	// Read before the write, which must not change the arrival but did before #91's repair.
	arrivalPredicates := make(map[string]bool)
	for _, tr := range arrival.Triples {
		arrivalPredicates[tr.Predicate] = true
	}

	if err := c.mergeEntityOnLane(t.Context(), arrival, false); err != nil {
		t.Fatalf("mergeEntityOnLane: %v", err)
	}
	if !bucket.failed.Load() {
		t.Fatal("the drone's create never failed, so no retry was exercised")
	}

	// The hierarchy statements this lane added to the arrival in its abandoned birth.
	added := make(map[string]bool)
	for _, tr := range decodeEntity(t, arrival.ID, bucket.abandoned).Triples {
		if !arrivalPredicates[tr.Predicate] && isHierarchyPredicate(tr.Predicate) {
			added[statementKey(tr)] = true
		}
	}
	if len(added) == 0 {
		t.Fatal("abandoned birth added no hierarchy statement, want at least one")
	}

	for _, tr := range storedEntity(t, c, arrival.ID).Triples {
		if added[statementKey(tr)] {
			t.Errorf("the merge stored a statement from this lane's abandoned birth: %s", statementKey(tr))
		}
	}
}

// The write seam refuses a stored value it cannot change (spec graph-entity-writes, "The write
// path refuses a stored value it cannot change"; design D23). On the stream lane and on both
// append lanes, the canonical request and the in-process one, a refused write stores nothing and
// the refusal is recorded at the revision the seam read. Each lane settles as it does for any
// poisoned stored value: the error is fatal/graph_state_reset_required, the stream message is
// negatively acknowledged (never terminated, so the arrival survives a repair), and the canonical
// append replies failed for the subject.
//
// The test plants the stored value at seamReadRevision. Right after the seam's read it plays
// another writer that stores the same value again at the next revision, so a record taken from a
// later read, or a write retried after one, shows.

const (
	seamKey          = "acme.ops.test.system.widget.seam-key"
	seamOtherEntity  = "acme.ops.test.system.widget.other-id"
	seamReadRevision = uint64(7)
)

// seamHeld is a statement the stored value carries, and seamAdded one it does not.
var (
	seamHeld = message.Triple{
		Subject: seamKey, Predicate: "test.state.value", Object: "held",
		Source: fixtureSource, Timestamp: fixtureTime, Confidence: 1.0,
	}
	seamAdded = message.Triple{
		Subject: seamKey, Predicate: "test.evidence.value", Object: "added",
		Source: fixtureSource, Timestamp: fixtureTime, Confidence: 1.0,
	}
)

// seamWrite is one write through the seam: a lane writing triple to seamKey. It checks the lane's
// refusal, the error and the settlement its caller sees.
type seamWrite struct {
	name   string
	triple message.Triple
	write  func(t *testing.T, c *Component, triple message.Triple)
}

func streamLaneWrite(t *testing.T, c *Component, triple message.Triple) {
	t.Helper()
	c.ingestGuardMem = []*laneGuard{newLaneGuard(16)}
	msg := &keyedIngestTestMsg{}
	err := c.processIngest(t.Context(), 0, ingestWork{
		entity: &graph.EntityState{ID: seamKey, MessageType: testEntityType(), Triples: []message.Triple{triple}},
		msg:    msg, consumer: testConsumer, heartbeat: testHeartbeat,
		entityID: seamKey,
		stream:   "ENTITY",
		seq:      1,
	})
	assertIngestResetRequired(t, err)
	if !msg.nak.Load() || msg.term.Load() || msg.ack.Load() {
		t.Fatalf("settlement: nak %v, term %v, ack %v; want only a negative acknowledgement",
			msg.nak.Load(), msg.term.Load(), msg.ack.Load())
	}
}

func canonicalAppendWrite(t *testing.T, c *Component, triple message.Triple) {
	t.Helper()
	data, err := c.handleCanonicalAppend(t.Context(), mustCanonicalJSON(t, graph.AppendTriplesRequest{
		Triples: []message.Triple{triple},
	}))
	if err != nil {
		t.Fatalf("handleCanonicalAppend: %v", err)
	}
	var response graph.AppendTriplesResponse
	if err := json.Unmarshal(data, &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Results) != 1 {
		t.Fatalf("results = %#v, want one", response.Results)
	}
	result := response.Results[0]
	if result.EntityID != seamKey || result.Outcome != graph.MutationFailed || result.KVRevision != 0 ||
		result.Error == nil || *result.Error != (graph.MutationFailure{Class: "fatal", Code: graph.ErrorCodeGraphStateResetRequired}) {
		t.Fatalf("append result = %#v (error %+v), want failed with fatal/%s",
			result, result.Error, graph.ErrorCodeGraphStateResetRequired)
	}
}

// assertWriteSeamRefuses runs each write over stored, planted at seamKey, and checks that it
// wrote nothing and that the refusal is recorded for reason at seamReadRevision.
func assertWriteSeamRefuses(t *testing.T, stored []byte, reason graph.StateResetReason, writes []seamWrite) {
	t.Helper()
	for _, write := range writes {
		t.Run(write.name, func(t *testing.T) {
			c, bucket := poisonScopingTestComponent(t)
			bucket.data[seamKey] = mockKVData{value: stored, revision: seamReadRevision}
			bucket.getFunc = func(_ context.Context, key string) (jetstream.KeyValueEntry, error) {
				bucket.mu.Lock()
				defer bucket.mu.Unlock()
				entry, ok := bucket.data[key]
				if !ok {
					return nil, jetstream.ErrKeyNotFound
				}
				if key == seamKey && entry.revision == seamReadRevision {
					// The other writer: the same value again, after this read.
					bucket.data[key] = mockKVData{value: entry.value, revision: seamReadRevision + 1}
				}
				return &mockKVEntry{key: key, data: entry.value, revision: entry.revision}, nil
			}

			write.write(t, c, write.triple)

			bucket.mu.Lock()
			after := bucket.data[seamKey]
			bucket.mu.Unlock()
			if !bytes.Equal(after.value, stored) || after.revision != seamReadRevision+1 {
				t.Fatalf("the refused write wrote: stored %q at revision %d, want %q at %d (the other writer's)",
					after.value, after.revision, stored, seamReadRevision+1)
			}
			record, inventoried := poisonInventoryEntry(c, seamKey)
			if !inventoried {
				t.Fatal("the refusal is not recorded in the poison inventory")
			}
			if record.revision != seamReadRevision {
				t.Fatalf("refusal recorded at revision %d, want %d, the revision the seam read",
					record.revision, seamReadRevision)
			}
			if record.contractErr.EntityID != seamKey || record.contractErr.Reason != reason {
				t.Fatalf("refusal recorded for %q with reason %q, want %q with %q",
					record.contractErr.EntityID, record.contractErr.Reason, seamKey, reason)
			}
		})
	}
}

func TestWriteSeamRefusesValueUnderAnotherKey(t *testing.T) {
	// A canonical entity, but another one than its key names. It carries seamHeld, so an append
	// of seamHeld would add nothing.
	stored, err := graph.MarshalEntityState(&graph.EntityState{
		ID: seamOtherEntity, MessageType: testEntityType(), Triples: []message.Triple{seamHeld},
	})
	if err != nil {
		t.Fatalf("encode the stored value: %v", err)
	}
	assertWriteSeamRefuses(t, stored, graph.GraphStateReasonNoncanonicalEntityID, []seamWrite{
		{name: "stream lane", triple: seamAdded, write: streamLaneWrite},
		{name: "canonical append", triple: seamAdded, write: canonicalAppendWrite},
		{name: "canonical append that adds nothing", triple: seamHeld, write: canonicalAppendWrite},
	})
}

// Absence is revision 0. An empty value at a nonzero revision is no birth on the stream lane and
// no absent entity on the append lane. It carries no statement, so no append over it adds nothing.
func TestWriteSeamRefusesEmptyStoredValue(t *testing.T) {
	assertWriteSeamRefuses(t, []byte{}, graph.GraphStateReasonUnreadableEntity, []seamWrite{
		{name: "stream lane", triple: seamAdded, write: streamLaneWrite},
		{name: "canonical append", triple: seamAdded, write: canonicalAppendWrite},
	})
}

// seamStoredWithout is a value at seamKey holding seamHeld and a statement of another predicate
// with no field, "source" or "timestamp", as a writer outside graph-ingest could store it: graph's
// encoder, which plants it, does not check statement metadata (checkpoint 3 H3).
func seamStoredWithout(t *testing.T, field string) []byte {
	t.Helper()
	bare := message.Triple{
		Subject: seamKey, Predicate: "test.state.legacy", Object: "old",
		Source: fixtureSource, Timestamp: fixtureTime, Confidence: 1.0,
	}
	switch field {
	case "source":
		bare.Source = ""
	case "timestamp":
		bare.Timestamp = time.Time{}
	default:
		t.Fatalf("seamStoredWithout: field %q is neither source nor timestamp", field)
	}
	value, err := graph.MarshalEntityState(&graph.EntityState{
		ID: seamKey, MessageType: testEntityType(), Triples: []message.Triple{seamHeld, bare},
	})
	if err != nil {
		t.Fatalf("encode the stored value: %v", err)
	}
	return value
}

// A value that does not decode, a write whose result the write gate refuses for a statement it
// keeps from the stored value, and one whose result keeps a statement with no source or no
// timestamp (checkpoint 3 H3), are refused; the record is at the revision the seam read, not at a
// later read's. An append that adds nothing over a value only the full rule refuses is not here:
// it reports unchanged, the cost design D23 declares
// (TestAppendAddingNothingOverStatementWithoutMetadataIsUnchanged).
func TestWriteSeamRecordsStoredPoisonAtTheRevisionRead(t *testing.T) {
	writes := []seamWrite{
		{name: "stream lane", triple: seamAdded, write: streamLaneWrite},
		{name: "canonical append", triple: seamAdded, write: canonicalAppendWrite},
	}
	t.Run("value that does not decode", func(t *testing.T) {
		assertWriteSeamRefuses(t, []byte(`{"id":`), graph.GraphStateReasonUnreadableEntity, writes)
	})
	t.Run("statement the write gate refuses", func(t *testing.T) {
		assertWriteSeamRefuses(t, guardTestPoisonBytes(seamKey), graph.GraphStateReasonNoncanonicalPredicate, writes)
	})
	for _, field := range []string{"source", "timestamp"} {
		t.Run("statement without a "+field, func(t *testing.T) {
			assertWriteSeamRefuses(t, seamStoredWithout(t, field), graph.GraphStateReasonMissingStatementMetadata, writes)
		})
	}
}

// An append whose every statement is already stored writes nothing and is unchanged at the
// revision it read, even over a value holding a statement with no source or no timestamp: the
// write path's read check stays the trusted decode plus the key check, so a write declined before
// the metadata check of its result is not refused for it, and nothing is recorded (design D23;
// ADR-110 Decision 6; checkpoint 3 H3).
func TestAppendAddingNothingOverStatementWithoutMetadataIsUnchanged(t *testing.T) {
	for _, field := range []string{"source", "timestamp"} {
		t.Run("statement without a "+field, func(t *testing.T) {
			c, bucket := poisonScopingTestComponent(t)
			stored := seamStoredWithout(t, field)
			bucket.data[seamKey] = mockKVData{value: stored, revision: seamReadRevision}

			data, err := c.handleCanonicalAppend(t.Context(), mustCanonicalJSON(t, graph.AppendTriplesRequest{
				Triples: []message.Triple{seamHeld},
			}))
			if err != nil {
				t.Fatalf("handleCanonicalAppend: %v", err)
			}
			var response graph.AppendTriplesResponse
			if err := json.Unmarshal(data, &response); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if len(response.Results) != 1 || response.Results[0].Outcome != graph.MutationUnchanged ||
				response.Results[0].KVRevision != seamReadRevision {
				t.Fatalf("results = %#v, want one unchanged at revision %d", response.Results, seamReadRevision)
			}
			bucket.mu.Lock()
			after := bucket.data[seamKey]
			bucket.mu.Unlock()
			if !bytes.Equal(after.value, stored) || after.revision != seamReadRevision {
				t.Fatalf("the append wrote: revision %d, want the planted value at %d", after.revision, seamReadRevision)
			}
			if _, recorded := poisonInventoryEntry(c, seamKey); recorded {
				t.Fatal("an append that wrote nothing recorded the stored value")
			}
		})
	}
}

// A stream arrival from a statement's own source, carrying its predicate, replaces the stored set
// of that predicate and source whole, so a stored statement with no timestamp in that set is
// replaced and the write applies: the metadata check is of the merge's result, never of the
// stored statements before the replace (design D23; checkpoint 3 H3). A stored set with no
// timestamp is older than any arrival's. On an unprofiled entity the write also stamps the profile
// (SemStreams ADR-054).
func TestStreamArrivalReplacesStoredStatementWithoutTimestamp(t *testing.T) {
	const (
		id      = "acme.ops.test.system.widget.no-time"
		source  = "source-a"
		planted = uint64(41)
	)
	bare := wmStatement(id, "p0", source, time.Time{})
	arrival := wmStatement(id, "p2", source, wmTime(2))
	profile := message.Triple{
		Subject: id, Predicate: vocabulary.EntityIndexingProfile, Object: vocabulary.IndexingProfileControl,
		Source: "graph-ingest-indexing-profile", Timestamp: wmTime(1), Confidence: 1,
	}
	for _, tc := range []struct {
		name   string
		stored []message.Triple
	}{
		{name: "profiled entity", stored: []message.Triple{bare, profile}},
		{name: "unprofiled entity", stored: []message.Triple{bare}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, bucket := createTestComponentWithMockKVBucket(t, withAuthority("acme", "ops"))
			value, err := graph.MarshalEntityState(&graph.EntityState{ID: id, MessageType: testEntityType(), Triples: tc.stored})
			if err != nil {
				t.Fatalf("encode the stored entity: %v", err)
			}
			bucket.data[id] = mockKVData{value: value, revision: planted}

			c.ingestGuardMem = []*laneGuard{newLaneGuard(16)}
			msg := &keyedIngestTestMsg{}
			err = c.processIngest(t.Context(), 0, ingestWork{
				entity: &graph.EntityState{ID: id, MessageType: testEntityType(), Triples: []message.Triple{arrival}},
				msg:    msg, consumer: testConsumer, heartbeat: testHeartbeat, entityID: id, stream: "ENTITY", seq: 1,
			})
			if err != nil {
				t.Fatalf("processIngest: %v", err)
			}
			if !msg.ack.Load() || msg.nak.Load() || msg.term.Load() {
				t.Fatalf("settlement: ack %v, nak %v, term %v; want only an acknowledgement",
					msg.ack.Load(), msg.nak.Load(), msg.term.Load())
			}
			bucket.mu.Lock()
			after := bucket.data[id]
			bucket.mu.Unlock()
			if after.revision != planted+1 {
				t.Fatalf("stored at revision %d, want %d: the arrival applies", after.revision, planted+1)
			}
			var stored graph.EntityState
			if err := json.Unmarshal(after.value, &stored); err != nil {
				t.Fatalf("decode the stored entity: %v", err)
			}
			var held []string
			for _, triple := range stored.Triples {
				if triple.Timestamp.IsZero() {
					t.Errorf("stored statement %+v has no timestamp", triple)
				}
				if triple.Predicate == writeModesPredicate {
					held = append(held, heldEntry(triple.Object, triple.Source, triple.Timestamp))
				}
			}
			if want := []string{"p2/source-a/2"}; !slices.Equal(held, want) {
				t.Errorf("stored statements of %s: %v, want %v", writeModesPredicate, held, want)
			}
		})
	}
}

// An arrival whose every set is older than the stored set of its predicate and source applies
// nothing (graph-entity-writes, "Every set is older"; design D23). Over a profiled entity the
// write is declined: the value and the revision the test planted stay, and the stream lane still
// acknowledges the arrival and counts and logs each set. Over an unprofiled entity the arrival
// writes once, to stamp the profile (SemStreams ADR-054) at the arrival's time. The arrival goes through
// processIngest, the stream lane's per-message handler, so its settlement is observed; the
// stored revision is planted by the test, so "unchanged" is not read off the code under test.
func TestStreamArrivalAllOlderWritesNothing(t *testing.T) {
	const (
		id      = "acme.ops.test.system.widget.all-older"
		other   = "test.state.other"
		source  = "source-a"
		planted = uint64(41)
	)
	storedSets := []message.Triple{
		wmStatement(id, "p3", source, wmTime(3)),
		{Subject: id, Predicate: other, Object: "q3", Source: source, Timestamp: wmTime(3), Confidence: 1},
	}
	// Both sets are older than the stored ones; the latest, wmTime(2), is the arrival's time.
	arrival := []message.Triple{
		wmStatement(id, "p1", source, wmTime(1)),
		{Subject: id, Predicate: other, Object: "q2", Source: source, Timestamp: wmTime(2), Confidence: 1},
	}
	profile := message.Triple{
		Subject: id, Predicate: vocabulary.EntityIndexingProfile, Object: vocabulary.IndexingProfileControl,
		Source: "graph-ingest-indexing-profile", Timestamp: wmTime(3), Confidence: 1,
	}

	// arrive plants the stored entity at the planted revision, delivers the arrival, and returns
	// the bucket's entry for id afterwards, with how far stale_sets_total and
	// entities_updated_total rose. It requires the arrival acknowledged and each set logged.
	arrive := func(t *testing.T, stored []message.Triple) (before, after mockKVData, staleRose, updatedRose float64) {
		t.Helper()
		registry := metric.NewMetricsRegistry()
		c, bucket := createTestComponentWithMockKVBucket(t, withAuthority("acme", "ops"), withMetricsRegistry(registry))
		var logs bytes.Buffer
		c.logger = slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
		value, err := graph.MarshalEntityState(&graph.EntityState{
			ID: id, MessageType: testEntityType(), Triples: stored, UpdatedAt: wmTime(3),
			StorageRef: &message.StorageReference{StorageInstance: "store", Key: "ref-stored"},
		})
		if err != nil {
			t.Fatalf("encode the stored entity: %v", err)
		}
		before = mockKVData{value: value, revision: planted}
		bucket.data[id] = before
		updated := func() float64 {
			family, ok := gatherFamilies(t, registry.PrometheusRegistry())["semengine_datamanager_entities_updated_total"]
			if !ok || len(family.GetMetric()) != 1 {
				t.Fatal("the registry does not gather semengine_datamanager_entities_updated_total")
			}
			return family.GetMetric()[0].GetCounter().GetValue()
		}
		staleBefore, updatedBefore := staleSetsTotal(t, registry), updated()

		c.ingestGuardMem = []*laneGuard{newLaneGuard(16)}
		msg := &keyedIngestTestMsg{}
		err = c.processIngest(t.Context(), 0, ingestWork{
			entity: &graph.EntityState{ID: id, MessageType: testEntityType(), Triples: slices.Clone(arrival),
				StorageRef: &message.StorageReference{StorageInstance: "store", Key: "ref-arrival"}},
			msg: msg, consumer: testConsumer, heartbeat: testHeartbeat, entityID: id, stream: "ENTITY", seq: 1,
		})
		if err != nil {
			t.Fatalf("processIngest: %v", err)
		}
		if !msg.ack.Load() || msg.nak.Load() || msg.term.Load() {
			t.Fatalf("settlement: ack %v, nak %v, term %v; want only an acknowledgement",
				msg.ack.Load(), msg.nak.Load(), msg.term.Load())
		}
		for _, predicate := range []string{writeModesPredicate, other} {
			logged := false
			for line := range strings.Lines(logs.String()) {
				logged = logged || strings.Contains(line, "level=DEBUG") && strings.Contains(line, "entity_id="+id) &&
					strings.Contains(line, "predicate="+predicate+" ") && strings.Contains(line, "source="+source)
			}
			if !logged {
				t.Errorf("no debug line names the set not applied of %s from %s", predicate, source)
			}
		}
		bucket.mu.Lock()
		after = bucket.data[id]
		bucket.mu.Unlock()
		return before, after, staleSetsTotal(t, registry) - staleBefore, updated() - updatedBefore
	}

	t.Run("profiled entity", func(t *testing.T) {
		before, after, staleRose, updatedRose := arrive(t, append(slices.Clone(storedSets), profile))
		if after.revision != planted || !bytes.Equal(after.value, before.value) {
			t.Errorf("the arrival wrote: stored at revision %d, value changed %v; want revision %d and the planted value",
				after.revision, !bytes.Equal(after.value, before.value), planted)
		}
		if staleRose != 2 {
			t.Errorf("stale_sets_total rose by %v, want 2: each set is counted though nothing is written", staleRose)
		}
		if updatedRose != 0 {
			t.Errorf("entities_updated_total rose by %v, want 0: no entity was updated", updatedRose)
		}
	})

	t.Run("unprofiled entity", func(t *testing.T) {
		_, after, staleRose, updatedRose := arrive(t, slices.Clone(storedSets))
		if after.revision != planted+1 {
			t.Fatalf("stored at revision %d, want %d: one write, to stamp the profile", after.revision, planted+1)
		}
		var stored graph.EntityState
		if err := json.Unmarshal(after.value, &stored); err != nil {
			t.Fatalf("decode the stored entity: %v", err)
		}
		var profiles []message.Triple
		held := map[string]string{}
		for _, triple := range stored.Triples {
			switch triple.Predicate {
			case vocabulary.EntityIndexingProfile:
				profiles = append(profiles, triple)
			case writeModesPredicate, other:
				held[triple.Predicate] = heldEntry(triple.Object, triple.Source, triple.Timestamp)
			}
		}
		if len(profiles) != 1 || !profiles[0].Timestamp.Equal(wmTime(2)) {
			t.Errorf("profile statements %+v, want one stamped at the arrival's time %v", profiles, wmTime(2))
		}
		want := map[string]string{writeModesPredicate: "p3/source-a/3", other: "q3/source-a/3"}
		if !maps.Equal(held, want) {
			t.Errorf("stored sets %v, want %v: an older set is not applied", held, want)
		}
		if stored.StorageRef == nil || stored.StorageRef.Key != "ref-stored" {
			t.Errorf("storage reference %+v, want ref-stored: a message with a skipped set keeps it", stored.StorageRef)
		}
		if staleRose != 2 {
			t.Errorf("stale_sets_total rose by %v, want 2", staleRose)
		}
		if updatedRose != 1 {
			t.Errorf("entities_updated_total rose by %v, want 1", updatedRose)
		}
	})
}

// An arrival whose only statement is the indexing profile carries nothing that can apply to a
// profiled entity: the profile is create-time immutable (SemStreams ADR-054), so the replace drops the
// arrival's, and no set is left. It writes nothing (graph-entity-writes, "Timestamp orders a
// replace": a message with at least one set, none of which applies; design D23; #150), whatever
// its age: older and newer than the stored profile alike, the value and the revision the test
// planted stay, so the arrival's storage reference is not taken, and the stream lane still
// acknowledges it. No set was older, so none is counted stale.
func TestStreamArrivalOnlyProfileWritesNothing(t *testing.T) {
	const (
		id      = "acme.ops.test.system.widget.profile-only"
		planted = uint64(41)
	)
	profileAt := func(at time.Time) message.Triple {
		return message.Triple{
			Subject: id, Predicate: vocabulary.EntityIndexingProfile, Object: vocabulary.IndexingProfileControl,
			Source: "graph-ingest-indexing-profile", Timestamp: at, Confidence: 1,
		}
	}
	for _, tc := range []struct {
		name string
		at   time.Time
	}{
		{name: "older than the stored profile", at: wmTime(1)},
		{name: "newer than the stored profile", at: wmTime(5)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registry := metric.NewMetricsRegistry()
			c, bucket := createTestComponentWithMockKVBucket(t, withAuthority("acme", "ops"), withMetricsRegistry(registry))
			value, err := graph.MarshalEntityState(&graph.EntityState{
				ID: id, MessageType: testEntityType(), UpdatedAt: wmTime(3),
				Triples:    []message.Triple{wmStatement(id, "p3", "source-a", wmTime(3)), profileAt(wmTime(3))},
				StorageRef: &message.StorageReference{StorageInstance: "store", Key: "ref-stored"},
			})
			if err != nil {
				t.Fatalf("encode the stored entity: %v", err)
			}
			bucket.data[id] = mockKVData{value: value, revision: planted}
			updated := func() float64 {
				family, ok := gatherFamilies(t, registry.PrometheusRegistry())["semengine_datamanager_entities_updated_total"]
				if !ok || len(family.GetMetric()) != 1 {
					t.Fatal("the registry does not gather semengine_datamanager_entities_updated_total")
				}
				return family.GetMetric()[0].GetCounter().GetValue()
			}
			staleBefore, updatedBefore := staleSetsTotal(t, registry), updated()

			c.ingestGuardMem = []*laneGuard{newLaneGuard(16)}
			msg := &keyedIngestTestMsg{}
			err = c.processIngest(t.Context(), 0, ingestWork{
				entity: &graph.EntityState{ID: id, MessageType: testEntityType(), Triples: []message.Triple{profileAt(tc.at)},
					StorageRef: &message.StorageReference{StorageInstance: "store", Key: "ref-arrival"}},
				msg: msg, consumer: testConsumer, heartbeat: testHeartbeat, entityID: id, stream: "ENTITY", seq: 1,
			})
			if err != nil {
				t.Fatalf("processIngest: %v", err)
			}
			if !msg.ack.Load() || msg.nak.Load() || msg.term.Load() {
				t.Fatalf("settlement: ack %v, nak %v, term %v; want only an acknowledgement",
					msg.ack.Load(), msg.nak.Load(), msg.term.Load())
			}
			bucket.mu.Lock()
			after := bucket.data[id]
			bucket.mu.Unlock()
			if after.revision != planted || !bytes.Equal(after.value, value) {
				t.Errorf("the arrival wrote: stored at revision %d, value changed %v; want revision %d and the planted value",
					after.revision, !bytes.Equal(after.value, value), planted)
			}
			if rose := updated() - updatedBefore; rose != 0 {
				t.Errorf("entities_updated_total rose by %v, want 0: no entity was updated", rose)
			}
			if rose := staleSetsTotal(t, registry) - staleBefore; rose != 0 {
				t.Errorf("stale_sets_total rose by %v, want 0: the arrival carried no older set", rose)
			}
		})
	}
}
