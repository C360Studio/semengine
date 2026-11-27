//go:build integration

package graphindex

import (
	"maps"
	"slices"
	"testing"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/vocabulary"
)

// bucketCreated is the creation time on the server, in nanoseconds since the Unix epoch, of the
// stream behind each of ENTITY_STATES and the five index buckets: read from the stream itself,
// named KV_<bucket> as NATS names a bucket's stream.
func bucketCreated(t *testing.T, client *natsclient.Client) map[string]uint64 {
	t.Helper()
	js, err := client.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	created := map[string]uint64{}
	for _, bucket := range append([]string{graph.BucketEntityStates}, indexBuckets...) {
		stream, err := js.Stream(t.Context(), "KV_"+bucket)
		if err != nil {
			t.Fatalf("stream of %s: %v", bucket, err)
		}
		info, err := stream.Info(t.Context())
		if err != nil {
			t.Fatalf("stream info of %s: %v", bucket, err)
		}
		created[bucket] = uint64(info.Created.UnixNano())
	}
	return created
}

// requireGenerations requires id's record to carry exactly want as its generations.
func requireGenerations(t *testing.T, client *natsclient.Client, id string, want map[string]uint64) {
	t.Helper()
	r, ok := storedRecord(t, client, id)
	if !ok {
		t.Fatalf("%s has no record", id)
	}
	if !maps.Equal(r.generations, want) {
		t.Errorf("generations in the record of %s = %v, want %v, the buckets' creation times", id, r.generations, want)
	}
}

// TestRestartRebuildsRecreatedIndexBucket deletes INCOMING_INDEX while graph-index is stopped. A
// graph-index then starts its rebuild and is stopped partway: some entities applied whole, one cut
// after its pending record and one row, the rest not reached. After the next start every incoming
// read answers the sources it answered before the deletion, and every record carries the new
// generation.
//
// The cut rebuild is driven by a graph-index that opened its buckets (recreating INCOMING_INDEX)
// and runs its applies itself, through writeRows and a prefix of one apply's operations, because a
// stop must land at a known point of the rebuild and nothing else orders it there.
func TestRestartRebuildsRecreatedIndexBucket(t *testing.T) {
	client := fixtureClient(t)
	first := startIndex(t, client, `{}`)
	targets := []string{entityID(10), entityID(11)}
	var sources []string
	values := map[string][]byte{}
	var lastRev uint64
	for i := range 6 {
		id := entityID(i + 1)
		sources = append(sources, id)
		values[id] = entityValue(t, id, stmt{linkTo, targets[i%2]}, stmt{armed, true},
			stmt{vocabulary.DCTermsTitle, "Source"}, stmt{alias, "source-" + id})
		lastRev = first.put(t, id, values[id])
	}
	first.awaitIndexed(t, lastRev)
	reads := []indexRead{}
	for _, target := range targets {
		reads = append(reads, indexRead{"incoming", map[string]string{"entity_id": target}})
	}
	for _, id := range sources {
		reads = append(reads, indexRead{"outgoing", map[string]string{"entity_id": id}})
	}
	before := readAll(t, first, reads, lastRev)
	// "The same sources" means something only if the first start answered them.
	in := answer[graph.IncomingRelationshipsData](t, client, "incoming", map[string]string{"entity_id": targets[0]}, lastRev)
	if len(in.Relationships) != 3 {
		t.Fatalf("incoming %s before the deletion = %v, want three sources", targets[0], in.Relationships)
	}
	oldGenerations := bucketCreated(t, client)
	for _, id := range sources {
		requireGenerations(t, client, id, oldGenerations)
	}
	stopComponent(t, first.c)
	deleteBucket(t, client, graph.BucketIncomingIndex)

	w := openWriter(t, client)
	newGenerations := bucketCreated(t, client)
	if newGenerations[graph.BucketIncomingIndex] == oldGenerations[graph.BucketIncomingIndex] {
		t.Fatal("the recreated INCOMING_INDEX has the deleted one's creation time")
	}
	for _, id := range sources[:2] {
		if _, err := w.writeRows(t.Context(), id, plannedFor(t, w, id, values[id])); err != nil {
			t.Fatalf("rebuild %s: %v", id, err)
		}
	}
	cut := prepareFor(t, w, sources[2], values[sources[2]])
	if len(cut.ops) < 2 || cut.ops[0].kind != opPending {
		t.Fatalf("the rebuild of %s under the new generation starts with %v, want its pending record and rows", sources[2], cut.ops)
	}
	runPrefix(t, w, cut, 2)
	// The cut is where the test put it: two records rebuilt, one pending, three of the old generation.
	for i, id := range sources {
		r, _ := storedRecord(t, client, id)
		rebuilt := maps.Equal(r.generations, newGenerations)
		if rebuilt != (i <= 2) || r.pending != (i == 2) {
			t.Fatalf("record of %s after the cut rebuild: new generation %v, pending %v", id, rebuilt, r.pending)
		}
	}

	second := restart(t, client, `{}`)
	second.awaitIndexed(t, lastRev) // the floor orders the reads below after every apply
	after := readAll(t, second, reads, lastRev)
	for _, read := range slices.Sorted(maps.Keys(before)) {
		if after[read] != before[read] {
			t.Errorf("%s after the rebuild = %s, want %s, as before the deletion", read, after[read], before[read])
		}
	}
	for _, id := range sources {
		requireCleanRecord(t, client, id)
		requireGenerations(t, client, id, newGenerations)
	}
	if n := second.c.failedTotal(); n != 0 {
		t.Errorf("failed entities after the rebuild = %d, want 0", n)
	}
}

// TestDeletingRecordsRebuildsEveryEntity deletes one INCOMING row by hand while graph-index is
// stopped. A start with the records in place does not repair it: the source's clean record still
// matches its plan, so the source is skipped. Deleting GRAPH_INDEX_RECORDS while graph-index is
// stopped is the reset: after the next start's first build the row answers again, and every entity
// is recorded again.
func TestDeletingRecordsRebuildsEveryEntity(t *testing.T) {
	client := fixtureClient(t)
	first := startIndex(t, client, `{}`)
	src, other, target := entityID(1), entityID(2), entityID(10)
	first.put(t, src, entityValue(t, src, stmt{linkTo, target}, stmt{alias, "source-1"}))
	rev := first.put(t, other, entityValue(t, other, stmt{linkTo, target}, stmt{armed, true}))
	first.awaitIndexed(t, rev)
	both := []string{linkTo + "<" + src, linkTo + "<" + other}
	incomingOf := func(h *harness) []string {
		t.Helper()
		in := answer[graph.IncomingRelationshipsData](t, h.client, "incoming", map[string]string{"entity_id": target}, rev)
		return sortedIDs(incomingSources(in.Relationships))
	}
	requireRelationship(t, "incoming before the hand edit", incomingOf(first), both)
	stopComponent(t, first.c)

	incoming, err := kvcatalog.EnsureCatalogBucket(t.Context(), client, graph.BucketIncomingIndex)
	if err != nil {
		t.Fatalf("open %s: %v", graph.BucketIncomingIndex, err)
	}
	if err := incoming.Delete(t.Context(), target+"."+src+"."+hexOf(linkTo)); err != nil {
		t.Fatalf("delete the INCOMING row by hand: %v", err)
	}

	second := restart(t, client, `{}`)
	second.awaitIndexed(t, rev) // the floor orders the read after every apply
	requireRelationship(t, "incoming after a start with the records in place", incomingOf(second),
		[]string{linkTo + "<" + other})
	stopComponent(t, second.c)

	deleteBucket(t, client, graph.BucketGraphIndexRecords)
	third := restart(t, client, `{}`)
	third.awaitIndexed(t, rev)
	requireRelationship(t, "incoming after the reset", incomingOf(third), both)
	for _, id := range []string{src, other} {
		requireCleanRecord(t, client, id)
	}
	if n := third.c.failedTotal(); n != 0 {
		t.Errorf("failed entities after the reset = %d, want 0", n)
	}
}
