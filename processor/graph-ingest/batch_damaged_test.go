package graphingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/cache"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"
	"pgregory.net/rapid"
)

// batchIDState is what the store holds under one requested ID before a batch reads it.
type batchIDState int

const (
	batchIDGood    batchIDState = iota // a readable value in the store
	batchIDCached                      // a readable value read once, so the cache holds it
	batchIDDamaged                     // bytes the stored-value rule refuses
	batchIDAbsent                      // nothing under the key
	batchIDEmpty                       // the empty string, which is never read
)

var batchIDStates = []batchIDState{batchIDGood, batchIDCached, batchIDDamaged, batchIDAbsent, batchIDEmpty}

func (s batchIDState) String() string {
	return [...]string{"good", "cached", "damaged", "absent", "empty"}[s]
}

// batchAccountingModel restates the batch reply rule in the requirement's words, one
// requested ID at a time and from its stored state alone: a readable entity is returned,
// a damaged one is named `damaged`, an absent one `not_found`, the empty ID `error`. A
// cached entity is readable. Repeats count once per request. Both lists come back sorted
// so the comparison ignores order, which the reply does not promise.
func batchAccountingModel(ids []string, states []batchIDState, requested []int) ([]string, []graph.MissingEntity) {
	entities := []string{}
	missing := []graph.MissingEntity{}
	for _, i := range requested {
		switch states[i] {
		case batchIDGood, batchIDCached:
			entities = append(entities, ids[i])
		case batchIDDamaged:
			missing = append(missing, graph.MissingEntity{ID: ids[i], Reason: graph.MissingDamaged})
		case batchIDAbsent:
			missing = append(missing, graph.MissingEntity{ID: ids[i], Reason: graph.MissingNotFound})
		case batchIDEmpty:
			missing = append(missing, graph.MissingEntity{ID: "", Reason: graph.MissingError})
		}
	}
	slices.Sort(entities)
	slices.SortFunc(missing, compareMissing)
	return entities, missing
}

func compareMissing(a, b graph.MissingEntity) int {
	if c := strings.Compare(a.ID, b.ID); c != 0 {
		return c
	}
	return strings.Compare(string(a.Reason), string(b.Reason))
}

// checkBatchAccounting stores states, sends one batch for requested (indices into states,
// repeats allowed) and checks the reply against batchAccountingModel: the entities and
// the missing list match it exactly, the reply is not a refusal, every damaged ID is
// recorded and no other ID is. A cached entity is removed from the store after the read
// that cached it, so only the cache can answer it.
func checkBatchAccounting(t *testing.T, tb fatalfer, states []batchIDState, requested []int) {
	tb.Helper()
	c, bucket := createTestComponentWithMockKVBucket(t, withAuthority("acme", "ops"))
	c.logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	entityCache, err := cache.NewSimple[graph.EntityState]()
	if err != nil {
		tb.Fatalf("new cache: %v", err)
	}
	defer func() { _ = entityCache.Close() }()
	c.entityCache = entityCache

	ids := make([]string, len(states))
	for i, state := range states {
		if state == batchIDEmpty {
			continue
		}
		ids[i] = fmt.Sprintf("acme.ops.test.system.widget.acct-%d", i)
		switch state {
		case batchIDGood, batchIDCached:
			seedValidBytes(bucket, ids[i], uint64(i+1))
		case batchIDDamaged:
			seedPoisonBytes(bucket, ids[i], uint64(i+1))
		}
	}
	for i, state := range states {
		if state != batchIDCached {
			continue
		}
		if _, err := c.handleQueryBatchNATS(context.Background(), batchRequest(tb, []string{ids[i]})); err != nil {
			tb.Fatalf("warm the cache for %s: %v", ids[i], err)
		}
		bucket.mu.Lock()
		delete(bucket.data, ids[i])
		bucket.mu.Unlock()
	}

	sent := make([]string, len(requested))
	for n, i := range requested {
		sent[n] = ids[i]
	}
	raw, err := c.handleQueryBatchNATS(context.Background(), batchRequest(tb, sent))
	if err != nil {
		tb.Fatalf("batch %q refused: %v", sent, err)
	}
	var resp graph.EntityBatchResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		tb.Fatalf("decode %s: %v", raw, err)
	}
	gotEntities := []string{}
	for _, e := range resp.Entities {
		gotEntities = append(gotEntities, e.ID)
	}
	slices.Sort(gotEntities)
	gotMissing := append([]graph.MissingEntity{}, resp.Missing...)
	slices.SortFunc(gotMissing, compareMissing)

	wantEntities, wantMissing := batchAccountingModel(ids, states, requested)
	if !slices.Equal(gotEntities, wantEntities) {
		tb.Fatalf("batch %q entities = %q, want %q", sent, gotEntities, wantEntities)
	}
	if !slices.Equal(gotMissing, wantMissing) {
		tb.Fatalf("batch %q missing = %v, want %v", sent, gotMissing, wantMissing)
	}
	for i, state := range states {
		if state == batchIDEmpty {
			continue
		}
		_, recorded := poisonInventoryEntry(c, ids[i])
		wantRecorded := state == batchIDDamaged && slices.Contains(requested, i)
		if recorded != wantRecorded {
			tb.Fatalf("%s (%s) recorded damaged = %v, want %v", ids[i], state, recorded, wantRecorded)
		}
	}
}

func batchRequest(tb fatalfer, ids []string) []byte {
	tb.Helper()
	body, err := json.Marshal(graph.EntityBatchRequest{IDs: ids})
	if err != nil {
		tb.Fatalf("encode batch request: %v", err)
	}
	return body
}

// TestBatchAccountingNamedCases runs the cases the generated check must not leave to a
// draw through the same assertions, before it.
func TestBatchAccountingNamedCases(t *testing.T) {
	cases := []struct {
		name      string
		states    []batchIDState
		requested []int
	}{
		{"one damaged among good and absent", []batchIDState{batchIDGood, batchIDDamaged, batchIDGood, batchIDAbsent}, []int{0, 1, 2, 3}},
		{"every ID damaged", []batchIDState{batchIDDamaged, batchIDDamaged}, []int{0, 1}},
		{"a damaged ID requested twice", []batchIDState{batchIDDamaged, batchIDGood}, []int{0, 1, 0}},
		{"a cached entity beside a damaged one", []batchIDState{batchIDCached, batchIDDamaged}, []int{0, 1}},
		{"the empty ID beside a damaged one", []batchIDState{batchIDEmpty, batchIDDamaged}, []int{0, 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checkBatchAccounting(t, t, tc.states, tc.requested)
		})
	}
}

// Requirement: graph-transport-boundary/A batch reply accounts for every requested ID
//
// TestBatchAccountingMatchesModel draws up to six IDs, each good, cached, damaged, absent
// or empty, and a request of up to ten of them with repeats, then checks the reply
// against batchAccountingModel. Every assertion runs on every case: the entity and
// missing comparisons always, the recorded check once per drawn ID.
func TestBatchAccountingMatchesModel(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		states := rapid.SliceOfN(rapid.SampledFrom(batchIDStates), 1, 6).Draw(rt, "states")
		requested := rapid.SliceOfN(rapid.IntRange(0, len(states)-1), 1, 10).Draw(rt, "requested")
		checkBatchAccounting(t, rt, states, requested)
	})
}

// TestBatchDamageBesideFaultIsTransient: a damaged entity no longer refuses the batch, so
// it cannot outrank a read fault. A fault on another ID still fails the whole batch,
// transient and `internal`, with nothing returned.
func TestBatchDamageBesideFaultIsTransient(t *testing.T) {
	const (
		good    = "acme.ops.test.system.widget.fault-a"
		damaged = "acme.ops.test.system.widget.fault-b"
		faulty  = "acme.ops.test.system.widget.fault-d"
	)
	bucket := newMockKVBucket()
	bucket.getFunc = func(_ context.Context, key string) (jetstream.KeyValueEntry, error) {
		switch key {
		case good:
			return &mockKVEntry{key: key, data: guardTestValidBytes(key), revision: 1}, nil
		case damaged:
			return &mockKVEntry{key: key, data: guardTestPoisonBytes(key), revision: 2}, nil
		default:
			return nil, errors.New("backend unavailable")
		}
	}
	c := newIngestGuardTestComponent(t, bucket)

	data, err := c.handleQueryBatchNATS(t.Context(), batchRequest(t, []string{good, damaged, faulty}))
	if data != nil {
		t.Fatalf("reply = %s, want none", data)
	}
	var classified *errs.ClassifiedError
	if !errors.As(err, &classified) || classified.Class != errs.ErrorTransient || classified.Code != graph.ErrorCodeInternal {
		t.Fatalf("error = %T %v, want transient/%s", err, err, graph.ErrorCodeInternal)
	}
}
