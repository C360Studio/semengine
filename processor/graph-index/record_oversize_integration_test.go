//go:build integration

package graphindex

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
)

// oversizePayload is the max_payload of the server TestOversizeRecordFallsBackToListing starts:
// small enough that an entity's record outgrows it while the entity's own value still fits.
const oversizePayload = 8192

// relPredicate is the i-th relationship predicate of the oversize test. Its long property makes
// each relationship's two rows (INCOMING, which holds the predicate in hex, and PREDICATE) far
// larger in a record than the statement is in the entity's value.
func relPredicate(i int) string {
	return fmt.Sprintf("test.rel%02d.%s", i, strings.Repeat("x", 60))
}

// relTarget is the target of the i-th relationship; no target is a stored entity.
func relTarget(i int) string { return entityID(100 + i) }

// relValue is src's value holding the relationships rels, in that order.
func relValue(t *testing.T, src string, rels []int) []byte {
	t.Helper()
	stmts := make([]stmt, 0, len(rels))
	for _, i := range rels {
		stmts = append(stmts, stmt{relPredicate(i), relTarget(i)})
	}
	return entityValue(t, src, stmts...)
}

// relPlaces is every row src owns while it holds rels, spelled from the key layouts: OUTGOING's
// key is the entity, INCOMING's target.source.hex(predicate), PREDICATE's predicate.entity.
func relPlaces(src string, rels []int) []rowID {
	places := []rowID{{graph.BucketOutgoingIndex, src}}
	for _, i := range rels {
		places = append(places,
			rowID{graph.BucketIncomingIndex, relTarget(i) + "." + src + "." + hexOf(relPredicate(i))},
			rowID{graph.BucketPredicateIndex, relPredicate(i) + "." + src})
	}
	return slices.SortedFunc(slices.Values(places), compareIDs)
}

// recordBytes is the length of a record naming places, by the encoding record.go documents: each
// string behind an 8-byte length, each number 8 bytes, each hash 32, and one generation for
// ENTITY_STATES and each index bucket.
func recordBytes(places []rowID, pending bool) int {
	state := "clean"
	if pending {
		state = "pending"
	}
	n := 8 + len(recordFormat) + 8 + len(state) + 8
	for _, bucket := range append([]string{graph.BucketEntityStates}, indexBuckets...) {
		n += 8 + len(bucket) + 8
	}
	n += 8
	for _, place := range places {
		n += 8 + len(place.bucket) + 8 + len(place.key) + 32
	}
	return n
}

// TestOversizeRecordFallsBackToListing: a recorded entity grows until its record no longer fits
// under the server's payload limit less the allowance, changes while large, shrinks, and is
// applied once more. While its record would be too large it has none and each apply lists; the
// apply that shrinks it still starts from a listing of the large rows, so it keeps none either,
// and the next apply lists the small rows and records them. At every step the public reads answer
// its current relationships and a listing of the index buckets finds exactly its current rows.
func TestOversizeRecordFallsBackToListing(t *testing.T) {
	// Own server: a payload limit small enough for a record to outgrow it while the entity fits.
	f := natsfixture.New(t, natsfixture.WithMaxPayload(oversizePayload))
	if err := f.Start(t.Context()); err != nil {
		t.Fatalf("start the server: %v", err)
	}
	client := natsfixture.Open(t, f.URL(), openClient)
	h := startIndex(t, client, `{}`)
	bound := oversizePayload - recordPayloadAllowance

	// The allowance holds on the pinned server: a record of exactly the limit less the allowance,
	// under the longest entity ID, is accepted by both conditional puts a record write makes.
	probe := padEntityID("acme.ops.test.system.widget.", 'p')
	rev, err := h.c.recordBucket.Create(t.Context(), probe, make([]byte, bound))
	if err != nil {
		t.Fatalf("create a %d-byte record under a %d-byte key: %v", bound, len(probe), err)
	}
	if _, err := h.c.recordBucket.Update(t.Context(), probe, make([]byte, bound), rev); err != nil {
		t.Fatalf("update a %d-byte record under a %d-byte key: %v", bound, len(probe), err)
	}
	if err := h.c.recordBucket.Delete(t.Context(), probe); err != nil {
		t.Fatalf("delete the probe record: %v", err)
	}

	src := entityID(1)
	seq := func(from, to int) []int {
		rels := []int{}
		for i := from; i < to; i++ {
			rels = append(rels, i)
		}
		return rels
	}
	small, grown, changed, shrunk := seq(0, 2), seq(0, 17), seq(8, 38), []int{20, 40}
	// The grown record lies between the bound and the limit, so it is the allowance, not the limit
	// alone, that keeps it out; the changed one is over the limit itself.
	for _, step := range []struct {
		what     string
		rels     []int
		min, max int
	}{{"grown", grown, bound + 1, oversizePayload}, {"changed", changed, oversizePayload + 1, 1 << 30}} {
		for _, pending := range []bool{true, false} {
			if n := recordBytes(relPlaces(src, step.rels), pending); n < step.min || n > step.max {
				t.Fatalf("the %s record (pending %v) is %d bytes, want %d to %d: change the test's sizes",
					step.what, pending, n, step.min, step.max)
			}
		}
	}
	if n := recordBytes(relPlaces(src, small), true); n > bound {
		t.Fatalf("the small record is %d bytes, want at most %d", n, bound)
	}

	// held is every relationship any step gives src, so a read can show a dropped one answers nothing.
	held := slices.Compact(slices.Sorted(slices.Values(slices.Concat(small, grown, changed, shrunk))))
	requireCurrent := func(step string, rels []int, rev uint64) {
		t.Helper()
		want := relPlaces(src, rels)
		if got := slices.SortedFunc(maps.Keys(ownedRows(t, client, src)), compareIDs); !slices.Equal(got, want) {
			t.Errorf("%s: stored rows of %s = %v, want %v", step, src, got, want)
		}
		out := answer[graph.OutgoingRelationshipsData](t, client, "outgoing", map[string]string{"entity_id": src}, rev)
		wantOut := []string{}
		for _, i := range rels {
			wantOut = append(wantOut, relPredicate(i)+">"+relTarget(i))
		}
		requireRelationship(t, step+": outgoing "+src, outgoingTargets(out.Relationships), wantOut)
		for _, i := range held {
			wantIn := []string{}
			if slices.Contains(rels, i) {
				wantIn = []string{relPredicate(i) + "<" + src}
			}
			in := answer[graph.IncomingRelationshipsData](t, client, "incoming", map[string]string{"entity_id": relTarget(i)}, rev)
			requireRelationship(t, step+": incoming "+relTarget(i), incomingSources(in.Relationships), wantIn)
		}
		if n := h.c.failedTotal(); n != 0 {
			t.Errorf("%s: failed entities = %d, want 0", step, n)
		}
	}
	// apply writes src's value holding rels and returns the owner listings its apply made. The lane
	// counts its KV operations before it completes the revision, so the floor reaching rev orders
	// the counter and every read after the apply.
	apply := func(rels []int) (uint64, float64) {
		t.Helper()
		before := listings(kvOperations(t, h.c))
		rev := h.put(t, src, relValue(t, src, rels))
		h.awaitIndexed(t, rev)
		return rev, listings(kvOperations(t, h.c)) - before
	}
	requireNoRecord := func(step string) {
		t.Helper()
		if r, ok := storedRecord(t, client, src); ok {
			t.Errorf("%s: %s has a record (pending %v, %d rows), want none while it is too large",
				step, src, r.pending, len(r.rows))
		}
	}
	// One listing apply is the four owner listings: INCOMING, PREDICATE, NAME and ALIAS.
	const oneListing = 4

	rev, _ = apply(small)
	requireCurrent("small", small, rev)
	requireCleanRecord(t, client, src)

	for _, step := range []struct {
		what string
		rels []int
	}{{"grown", grown}, {"changed", changed}, {"shrunk", shrunk}} {
		var listed float64
		rev, listed = apply(step.rels)
		requireCurrent(step.what, step.rels, rev)
		requireNoRecord(step.what)
		if listed != oneListing {
			t.Errorf("%s: %v owner listings, want %d, one listing apply", step.what, listed, oneListing)
		}
	}

	// Applied once more, the shrunk entity's listing finds only its small rows, and its record fits.
	rev, listed := apply(shrunk)
	requireCurrent("shrunk, applied again", shrunk, rev)
	requireCleanRecord(t, client, src)
	if listed != oneListing {
		t.Errorf("shrunk, applied again: %v owner listings, want %d", listed, oneListing)
	}
	rev, listed = apply(shrunk)
	requireCurrent("shrunk, recorded", shrunk, rev)
	if listed != 0 {
		t.Errorf("an equal re-offer once recorded: %v owner listings, want 0", listed)
	}

	// A record that cannot be read cannot be deleted at a known revision: an apply whose record
	// would be too large then writes nothing, so the stored clean record still names every row.
	records, err := kvcatalog.EnsureCatalogBucket(t.Context(), client, graph.BucketGraphIndexRecords)
	if err != nil {
		t.Fatalf("open %s: %v", graph.BucketGraphIndexRecords, err)
	}
	w := openWriter(t, client)
	w.recordBucket = client.NewKVStore(refusedRecordReads{records})
	sequencesBefore, recordBefore := indexSequences(t, client), recordSequence(t, client)
	if _, err := w.writeRows(t.Context(), src, plannedFor(t, w, src, relValue(t, src, grown))); err == nil {
		t.Error("an apply too large to record, over a record that cannot be read: no error, want a failed write")
	}
	if got := indexSequences(t, client); !maps.Equal(got, sequencesBefore) {
		t.Errorf("index stream sequences after the refused apply = %v, want %v: a row was written", got, sequencesBefore)
	}
	if got := recordSequence(t, client); got != recordBefore {
		t.Errorf("record stream sequence after the refused apply = %d, want %d", got, recordBefore)
	}
	requireCleanRecord(t, client, src)
}

// refusedRecordReads is the record bucket with every read refused, as a read cut off by the
// connection would be.
type refusedRecordReads struct{ jetstream.KeyValue }

func (refusedRecordReads) Get(context.Context, string) (jetstream.KeyValueEntry, error) {
	return nil, errors.New("the test refuses every record read")
}
