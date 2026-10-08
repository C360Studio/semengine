package graphingest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/vocabulary"
	"github.com/nats-io/nats.go/jetstream"
)

// The stream lane's write (replaceEntity) is a compare-and-set that the bucket retries after a
// create conflict or a create error it does not mark non-retryable. Each retry must start from
// the arrival as it came and the stored state it reads (PR #93 comment 6060120246). These tests
// make the entity's first create fail once and read what the retry stored.

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
	c.initHierarchyInference()
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

func decodeEntity(t *testing.T, data []byte) graph.EntityState {
	t.Helper()
	var state graph.EntityState
	if err := graph.UnmarshalEntityState(data, &state); err != nil {
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
// with an error that is not a conflict, so the key is still absent and the bucket retries the
// birth. The entity is born with each statement once, hierarchy statements included.
func TestReplaceEntityRetryAfterCreateErrorStoresNoStatementTwice(t *testing.T) {
	c, bucket, arrival := newRetryingStreamLane(t, errors.New("nats: stream temporarily unavailable"))

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
	for _, tr := range decodeEntity(t, bucket.abandoned).Triples {
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
		entity:   &graph.EntityState{ID: seamKey, MessageType: testEntityType(), Triples: []message.Triple{triple}},
		msg:      msg,
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

func inProcessAppendWrite(t *testing.T, c *Component, triple message.Triple) {
	t.Helper()
	deduplicated, committed, err := c.addTripleLane(t.Context(), triple, dedupLaneHierarchy)
	assertIngestResetRequired(t, err)
	if deduplicated || committed != 0 {
		t.Fatalf("in-process append: deduplicated %v, committed revision %d; want neither", deduplicated, committed)
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
		{name: "in-process append", triple: seamAdded, write: inProcessAppendWrite},
		{name: "in-process append that adds nothing", triple: seamHeld, write: inProcessAppendWrite},
	})
}

// Absence is revision 0. An empty value at a nonzero revision is no birth on the stream lane and
// no absent entity on the append lanes. It carries no statement, so no append over it adds nothing.
func TestWriteSeamRefusesEmptyStoredValue(t *testing.T) {
	assertWriteSeamRefuses(t, []byte{}, graph.GraphStateReasonUnreadableEntity, []seamWrite{
		{name: "stream lane", triple: seamAdded, write: streamLaneWrite},
		{name: "canonical append", triple: seamAdded, write: canonicalAppendWrite},
		{name: "in-process append", triple: seamAdded, write: inProcessAppendWrite},
	})
}

// A value that does not decode, and a write whose result the write gate refuses for a statement it
// keeps from the stored value, are refused as before; the record is now at the revision the seam
// read, not at a later read's. An append that adds nothing over a value only the full rule
// refuses is not here: it reports unchanged, the cost design D23 declares.
func TestWriteSeamRecordsStoredPoisonAtTheRevisionRead(t *testing.T) {
	writes := []seamWrite{
		{name: "stream lane", triple: seamAdded, write: streamLaneWrite},
		{name: "canonical append", triple: seamAdded, write: canonicalAppendWrite},
		{name: "in-process append", triple: seamAdded, write: inProcessAppendWrite},
	}
	t.Run("value that does not decode", func(t *testing.T) {
		assertWriteSeamRefuses(t, []byte(`{"id":`), graph.GraphStateReasonUnreadableEntity, writes)
	})
	t.Run("statement the write gate refuses", func(t *testing.T) {
		assertWriteSeamRefuses(t, guardTestPoisonBytes(seamKey), graph.GraphStateReasonNoncanonicalPredicate, writes)
	})
}
