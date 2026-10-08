package graphingest

import (
	"context"
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
