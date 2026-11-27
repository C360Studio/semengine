//go:build integration

package graphindex

import (
	"context"
	"crypto/sha256"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/vocabulary"
)

// The tests in this file read an entity's record from GRAPH_INDEX_RECORDS and its rows from the
// index buckets themselves. A row belongs to an entity when the entity's six tokens stand in its
// key, which every key layout places its owner in: OUTGOING's key is the entity, INCOMING's is
// target.source.hex(predicate), PREDICATE's predicate.entity, NAME's and ALIAS's
// hash.entity.hex(predicate). No test entity here is another's target, so every row whose key holds
// an entity's tokens is that entity's own.

// indexBuckets is the five index buckets.
var indexBuckets = slices.Sorted(maps.Keys(indexRowLabels))

// storedRecord is id's record as GRAPH_INDEX_RECORDS holds it, and whether there is one.
func storedRecord(t *testing.T, client *natsclient.Client, id string) (entityRecord, bool) {
	t.Helper()
	reader, err := kvcatalog.OpenCatalogReader(t.Context(), client, graph.BucketGraphIndexRecords)
	if err != nil {
		t.Fatalf("open %s: %v", graph.BucketGraphIndexRecords, err)
	}
	entry, err := reader.Get(t.Context(), id)
	if natsclient.IsKVNotFoundError(err) {
		return entityRecord{}, false
	}
	if err != nil {
		t.Fatalf("get the record of %s: %v", id, err)
	}
	r, err := decodeRecord(entry.Value())
	if err != nil {
		t.Fatalf("decode the record of %s: %v", id, err)
	}
	return r, true
}

// holdsTokens reports whether key's dot-separated tokens include id's six in a row.
func holdsTokens(key, id string) bool {
	keyTokens, idTokens := strings.Split(key, "."), strings.Split(id, ".")
	for i := 0; i+len(idTokens) <= len(keyTokens); i++ {
		if slices.Equal(keyTokens[i:i+len(idTokens)], idTokens) {
			return true
		}
	}
	return false
}

// ownedRows is every index row whose key holds id's tokens, by place, with its value: a listing of
// every key of each index bucket, not graph-index's owner filters.
func ownedRows(t *testing.T, client *natsclient.Client, id string) map[rowID]string {
	t.Helper()
	return ownedRowsOf(t, client, id)[id]
}

// ownedRowsOf is ownedRows of each of ids, from one listing of each index bucket.
func ownedRowsOf(t *testing.T, client *natsclient.Client, ids ...string) map[string]map[rowID]string {
	t.Helper()
	rows := map[string]map[rowID]string{}
	for _, id := range ids {
		rows[id] = map[rowID]string{}
	}
	for _, bucket := range indexBuckets {
		reader, err := kvcatalog.OpenCatalogReader(t.Context(), client, bucket)
		if err != nil {
			t.Fatalf("open %s: %v", bucket, err)
		}
		keys, err := natsclient.FilteredKeys(t.Context(), reader, ">")
		if err != nil {
			t.Fatalf("keys of %s: %v", bucket, err)
		}
		for _, key := range keys {
			for _, id := range ids {
				if !holdsTokens(key, id) {
					continue
				}
				entry, err := reader.Get(t.Context(), key)
				if natsclient.IsKVNotFoundError(err) {
					continue // deleted since the listing
				}
				if err != nil {
					t.Fatalf("get %s/%s: %v", bucket, key, err)
				}
				rows[id][rowID{bucket, key}] = string(entry.Value())
			}
		}
	}
	return rows
}

// recordNames reports, for each stored row, whether r names it; with exact, whether r names exactly
// the stored rows with each value's hash. It returns what disagrees.
func recordNames(r entityRecord, stored map[rowID]string, exact bool) []string {
	var problems []string
	named := map[rowID]recordRow{}
	for _, row := range r.rows {
		named[row.id()] = row
	}
	for id, value := range stored {
		row, ok := named[id]
		switch {
		case !ok:
			problems = append(problems, "unnamed row "+id.bucket+"/"+id.key)
		case exact && row.hash != sha256.Sum256([]byte(value)):
			problems = append(problems, "wrong hash for "+id.bucket+"/"+id.key)
		}
	}
	if exact {
		for id := range named {
			if _, ok := stored[id]; !ok {
				problems = append(problems, "named row not stored "+id.bucket+"/"+id.key)
			}
		}
	}
	slices.Sort(problems)
	return problems
}

// requireCleanRecord requires id to have a clean record, under this server's generations, naming
// exactly its stored rows.
func requireCleanRecord(t *testing.T, client *natsclient.Client, id string) {
	t.Helper()
	r, ok := storedRecord(t, client, id)
	if !ok || r.pending {
		t.Fatalf("record of %s: present %v, pending %v; want a clean record", id, ok, r.pending)
	}
	if problems := recordNames(r, ownedRows(t, client, id), true); len(problems) > 0 {
		t.Errorf("clean record of %s disagrees with its rows: %v", id, problems)
	}
}

// recordSequence is GRAPH_INDEX_RECORDS' last stream sequence, which every record write moves.
func recordSequence(t *testing.T, client *natsclient.Client) uint64 {
	t.Helper()
	reader, err := kvcatalog.OpenCatalogReader(t.Context(), client, graph.BucketGraphIndexRecords)
	if err != nil {
		t.Fatalf("open %s: %v", graph.BucketGraphIndexRecords, err)
	}
	seq, err := natsclient.BucketLastSeq(t.Context(), reader)
	if err != nil {
		t.Fatalf("last sequence of %s: %v", graph.BucketGraphIndexRecords, err)
	}
	return seq
}

// deleteBucket deletes a KV bucket, as an operator would.
func deleteBucket(t *testing.T, client *natsclient.Client, bucket string) {
	t.Helper()
	js, err := client.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	if err := js.DeleteKeyValue(t.Context(), bucket); err != nil {
		t.Fatalf("delete %s: %v", bucket, err)
	}
}

// TestEqualReofferSkipsIndexing writes every indexed entity again with equal statements, so each
// gets a new revision. graph-index lists nothing and writes no index row or record for them, and
// every read answers as before.
func TestEqualReofferSkipsIndexing(t *testing.T) {
	h := startIndex(t, fixtureClient(t), `{}`)
	title := vocabulary.DCTermsTitle
	a, b := entityID(1), entityID(2)
	values := map[string][]byte{
		a: entityValue(t, a, stmt{linkTo, b}, stmt{title, "Alpha"}, stmt{alias, "alpha-1"}),
		b: entityValue(t, b, stmt{linkTo, a}, stmt{armed, true}),
	}
	var rev uint64
	for _, id := range []string{a, b} {
		rev = h.put(t, id, values[id])
	}
	h.awaitIndexed(t, rev)
	reads := []indexRead{
		{"outgoing", map[string]string{"entity_id": a}}, {"incoming", map[string]string{"entity_id": a}},
		{"outgoing", map[string]string{"entity_id": b}}, {"incoming", map[string]string{"entity_id": b}},
		{"alias", map[string]string{"alias": "alpha-1"}}, {"byName", map[string]string{"name": "Alpha"}},
		{"predicate", map[string]string{"predicate": armed}},
	}
	before := readAll(t, h, reads, rev)
	if got := resolveAlias(t, h, "alpha-1", rev); got != a {
		t.Fatalf("alias alpha-1 before the re-offer = %q, want %q", got, a)
	}
	opsBefore, sequencesBefore, recordsBefore := kvOperations(t, h.c), indexSequences(t, h.client), recordSequence(t, h.client)

	for _, id := range []string{a, b} {
		rev = h.put(t, id, values[id])
	}
	// The lane counts its KV operations before it completes the revision, so the floor reaching the
	// last re-offer orders the reads below after both applies.
	h.awaitIndexed(t, rev)
	opsAfter := kvOperations(t, h.c)
	if n := listings(opsAfter) - listings(opsBefore); n != 0 {
		t.Errorf("the re-offer made %v owner listings, want 0", n)
	}
	if n := rowWrites(opsAfter) - rowWrites(opsBefore); n != 0 {
		t.Errorf("the re-offer made %v index row writes, want 0", n)
	}
	if got := indexSequences(t, h.client); !maps.Equal(got, sequencesBefore) {
		t.Errorf("index stream sequences after the re-offer = %v, want %v, unchanged", got, sequencesBefore)
	}
	if got := recordSequence(t, h.client); got != recordsBefore {
		t.Errorf("record stream sequence after the re-offer = %d, want %d, unchanged", got, recordsBefore)
	}
	after := readAll(t, h, reads, rev)
	for _, read := range slices.Sorted(maps.Keys(before)) {
		if after[read] != before[read] {
			t.Errorf("%s after the re-offer = %s, want %s", read, after[read], before[read])
		}
	}
}

// TestRepairSkipClearsFailed: an entity whose clean record matches is in the failed set, as after a
// failed ENTITY_STATES read. The repair pass reads it, finds its rows current, and clears it with no
// listing and no write; queries are no longer withheld.
func TestRepairSkipClearsFailed(t *testing.T) {
	h := startIndex(t, fixtureClient(t), `{}`)
	id := entityID(1)
	h.awaitIndexed(t, h.put(t, id, entityValue(t, id, stmt{linkTo, entityID(2)}, stmt{alias, "alpha-1"})))
	requireCleanRecord(t, h.client, id)
	// The lane's mark for a failed read of ENTITY_STATES (reconcileEntity).
	h.c.markEntityFailed(id)
	if err := h.f.QueryReady(t.Context()); err == nil {
		t.Fatal("QueryReady = nil with an entity in the failed set")
	}
	opsBefore, sequencesBefore := kvOperations(t, h.c), indexSequences(t, h.client)

	h.c.repairFailedEntities(t.Context())
	// The lane clears the entity after its apply, so the failed count reaching 0 orders the counter
	// and sequence reads after it.
	await(t, "the repair pass clears the entity",
		func(context.Context) (int64, error) { return h.c.failedTotal(), nil },
		func(n int64) bool { return n == 0 })
	opsAfter := kvOperations(t, h.c)
	if n := listings(opsAfter) - listings(opsBefore); n != 0 {
		t.Errorf("the repair made %v owner listings, want 0", n)
	}
	if n := rowWrites(opsAfter) - rowWrites(opsBefore); n != 0 {
		t.Errorf("the repair made %v index row writes, want 0", n)
	}
	if got := indexSequences(t, h.client); !maps.Equal(got, sequencesBefore) {
		t.Errorf("index stream sequences after the repair = %v, want %v, unchanged", got, sequencesBefore)
	}
	if err := h.f.QueryReady(t.Context()); err != nil {
		t.Errorf("QueryReady after the repair: %v", err)
	}
}

// TestRestartWithoutRecordsListsOnce starts graph-index over index rows that have no records, as
// left by a graph-index that kept none. Each entity is listed once and recorded, its reads answer as
// before, and the next start lists none.
func TestRestartWithoutRecordsListsOnce(t *testing.T) {
	client := fixtureClient(t)
	first := startIndex(t, client, `{}`)
	a, b, c := entityID(1), entityID(2), entityID(3)
	target := entityID(10)
	first.put(t, a, entityValue(t, a, stmt{linkTo, target}, stmt{vocabulary.DCTermsTitle, "Alpha"}, stmt{alias, "alpha-1"}))
	first.put(t, b, entityValue(t, b, stmt{linkTo, target}, stmt{armed, true}))
	rev := first.put(t, c, entityValue(t, c, stmt{alias, "gamma-1"}))
	first.awaitIndexed(t, rev)
	reads := []indexRead{
		{"outgoing", map[string]string{"entity_id": a}}, {"incoming", map[string]string{"entity_id": target}},
		{"alias", map[string]string{"alias": "alpha-1"}}, {"alias", map[string]string{"alias": "gamma-1"}},
		{"byName", map[string]string{"name": "Alpha"}}, {"predicate", map[string]string{"predicate": armed}},
	}
	before := readAll(t, first, reads, rev)
	if got := resolveAlias(t, first, "gamma-1", rev); got != c {
		t.Fatalf("alias gamma-1 on the first start = %q, want %q", got, c)
	}
	stopComponent(t, first.c)
	deleteBucket(t, client, graph.BucketGraphIndexRecords)

	second := restart(t, client, `{}`)
	second.awaitIndexed(t, rev) // the floor orders the counter and record reads after every apply
	// Four owner listings an entity: INCOMING, PREDICATE, NAME and ALIAS.
	if n := listings(kvOperations(t, second.c)); n != 3*4 {
		t.Errorf("start without records: %v owner listings, want %d, each entity once", n, 3*4)
	}
	for _, id := range []string{a, b, c} {
		requireCleanRecord(t, client, id)
	}
	after := readAll(t, second, reads, rev)
	for _, read := range slices.Sorted(maps.Keys(before)) {
		if after[read] != before[read] {
			t.Errorf("%s after recording = %s, want %s", read, after[read], before[read])
		}
	}
	stopComponent(t, second.c)

	third := restart(t, client, `{}`)
	third.awaitIndexed(t, rev)
	if ops := kvOperations(t, third.c); listings(ops) != 0 || rowWrites(ops) != 0 {
		t.Errorf("the start after recording: %v listings and %v row writes, want none", listings(ops), rowWrites(ops))
	}
}

// TestRestartAppliesNewAliasPredicate: a predicate becomes an alias predicate between two starts and
// no value changes. After the second start an alias request answers the entity carrying it; only
// that entity's alias row was written, and nothing was listed.
func TestRestartAppliesNewAliasPredicate(t *testing.T) {
	defer vocabulary.SnapshotRegistry()()
	client := fixtureClient(t)
	const serial = "test.alias.serial"
	first := startIndex(t, client, `{}`)
	carrier, other := entityID(1), entityID(2)
	first.put(t, carrier, entityValue(t, carrier, stmt{serial, "SN-1"}, stmt{linkTo, other}))
	rev := first.put(t, other, entityValue(t, other, stmt{armed, true}, stmt{alias, "other-1"}))
	first.awaitIndexed(t, rev)
	if got := resolveAlias(t, first, "SN-1", rev); got != "" {
		t.Fatalf("alias SN-1 before %s is an alias predicate = %q, want none", serial, got)
	}
	stopComponent(t, first.c)
	sequencesBefore := indexSequences(t, client)

	vocabulary.Register(serial, vocabulary.WithAlias(vocabulary.AliasTypeIdentity, 0))
	second := restart(t, client, `{}`)
	second.awaitIndexed(t, rev) // the floor orders the reads below after both applies
	if got := resolveAlias(t, second, "SN-1", rev); got != carrier {
		t.Errorf("alias SN-1 after the second start = %q, want %q", got, carrier)
	}
	if got := resolveAlias(t, second, "other-1", rev); got != other {
		t.Errorf("alias other-1 after the second start = %q, want %q", got, other)
	}
	ops := kvOperations(t, second.c)
	if n := listings(ops); n != 0 {
		t.Errorf("second start: %v owner listings, want 0", n)
	}
	if got, want := ops["put"]["alias"], 1.0; got != want || rowWrites(ops) != want {
		t.Errorf("second start: %v alias puts and %v index row writes, want one alias put and nothing else",
			got, rowWrites(ops))
	}
	want := maps.Clone(sequencesBefore)
	want[graph.BucketAliasIndex]++
	if got := indexSequences(t, client); !maps.Equal(got, want) {
		t.Errorf("index stream sequences after the second start = %v, want %v, the alias row only", got, want)
	}
	requireCleanRecord(t, client, carrier)
}

// TestDamagedRemovalRecordsEmpty: an indexed entity is overwritten, while graph-index is down, with a
// value that does not decode. On the next start its rows are gone, it is reported damaged and it has
// a clean empty record; the start after that reports it again and lists or writes nothing for it.
func TestDamagedRemovalRecordsEmpty(t *testing.T) {
	client := fixtureClient(t)
	first := startIndex(t, client, `{}`)
	id := entityID(1)
	first.awaitIndexed(t, first.put(t, id, entityValue(t, id,
		stmt{linkTo, entityID(2)}, stmt{vocabulary.DCTermsTitle, "Alpha"}, stmt{alias, "alpha-1"})))
	if len(ownedRows(t, client, id)) == 0 {
		t.Fatal("the entity has no rows before it is damaged")
	}
	stopComponent(t, first.c)
	rev := first.put(t, id, damagedValue)

	for start := 1; start <= 2; start++ {
		h := restart(t, client, `{}`)
		h.awaitIndexed(t, rev) // the lane reports the damage and applies before it completes rev
		requireHealthNames(t, h, id)
		if rows := ownedRows(t, client, id); len(rows) != 0 {
			t.Errorf("start %d: rows of the damaged entity remain: %v", start, slices.Collect(maps.Keys(rows)))
		}
		r, ok := storedRecord(t, client, id)
		if !ok || r.pending || len(r.rows) != 0 {
			t.Errorf("start %d: record present %v, pending %v, %d rows; want a clean empty record", start, ok, r.pending, len(r.rows))
		}
		if start == 2 {
			if ops := kvOperations(t, h.c); listings(ops) != 0 || rowWrites(ops) != 0 {
				t.Errorf("start 2: %v listings and %v row writes, want none", listings(ops), rowWrites(ops))
			}
		}
		stopComponent(t, h.c)
	}
}

// TestRestartAppliesDeleteWhileDown deletes an indexed entity while graph-index is stopped. After the
// next start every read answers nothing for it, another entity's edge to it survives, and the start
// after that lists nothing.
func TestRestartAppliesDeleteWhileDown(t *testing.T) {
	client := fixtureClient(t)
	first := startIndex(t, client, `{}`)
	gone, other, last := entityID(1), entityID(2), entityID(3)
	first.put(t, gone, entityValue(t, gone,
		stmt{linkTo, entityID(10)}, stmt{armed, true}, stmt{vocabulary.DCTermsTitle, "Gone"}, stmt{alias, "gone-1"}))
	rev := first.put(t, other, entityValue(t, other, stmt{linkTo, gone}))
	first.awaitIndexed(t, rev)
	if got := resolveAlias(t, first, "gone-1", rev); got != gone {
		t.Fatalf("alias gone-1 before the delete = %q, want %q", got, gone)
	}
	stopComponent(t, first.c)
	if err := first.states.Delete(t.Context(), gone); err != nil {
		t.Fatalf("delete %s: %v", gone, err)
	}
	// Above the delete: the floor reaching it orders every read after the delete's apply.
	lastRev := first.put(t, last, entityValue(t, last, stmt{armed, false}))

	second := restart(t, client, `{}`)
	second.awaitIndexed(t, lastRev)
	out := answer[graph.OutgoingRelationshipsData](t, client, "outgoing", map[string]string{"entity_id": gone}, lastRev)
	if len(out.Relationships) != 0 {
		t.Errorf("outgoing of the deleted entity = %v, want none", out.Relationships)
	}
	in := answer[graph.IncomingRelationshipsData](t, client, "incoming", map[string]string{"entity_id": gone}, lastRev)
	if want := []graph.IncomingEntry{{FromEntityID: other, Predicate: linkTo}}; !slices.Equal(in.Relationships, want) {
		t.Errorf("incoming of the deleted entity = %v, want %v, the live source's edge", in.Relationships, want)
	}
	if got := resolveAlias(t, second, "gone-1", lastRev); got != "" {
		t.Errorf("alias gone-1 after the delete = %q, want none", got)
	}
	armedBy := answer[graph.PredicateData](t, client, "predicate", map[string]string{"predicate": armed}, lastRev)
	if !slices.Equal(armedBy.Entities, []string{last}) {
		t.Errorf("predicate %s = %v, want only %s", armed, armedBy.Entities, last)
	}
	if names := answer[graph.NameData](t, client, "byName", map[string]string{"name": "Gone"}, lastRev).Matches; len(names) != 0 {
		t.Errorf("byName Gone = %v, want none", names)
	}
	stopComponent(t, second.c)

	third := restart(t, client, `{}`)
	third.awaitIndexed(t, lastRev)
	if ops := kvOperations(t, third.c); listings(ops) != 0 || rowWrites(ops) != 0 {
		t.Errorf("the start after the delete: %v listings and %v row writes, want none", listings(ops), rowWrites(ops))
	}
}

// TestRecordBucketDeletedWhileRunningWithholds deletes GRAPH_INDEX_RECORDS while graph-index runs,
// then changes an indexed entity. No row is written, since the pending record cannot be put; a
// request answers index_not_ready; after a restart the change answers.
func TestRecordBucketDeletedWhileRunningWithholds(t *testing.T) {
	client := fixtureClient(t)
	h := startIndex(t, client, `{}`)
	id := entityID(1)
	h.awaitIndexed(t, h.put(t, id, entityValue(t, id, stmt{linkTo, entityID(2)}, stmt{alias, "alpha-1"})))
	deleteBucket(t, client, graph.BucketGraphIndexRecords)
	sequencesBefore := indexSequences(t, client)

	rev := h.put(t, id, entityValue(t, id, stmt{linkTo, entityID(3)}, stmt{alias, "beta-1"}))
	h.awaitIndexed(t, rev) // the lane marks the entity failed before it completes rev
	if got := indexSequences(t, client); !maps.Equal(got, sequencesBefore) {
		t.Errorf("index stream sequences after the change = %v, want %v: a row was written with no pending record",
			got, sequencesBefore)
	}
	if n := h.c.failedTotal(); n != 1 {
		t.Errorf("failed entities = %d, want 1", n)
	}
	_, err := query(t, client, "alias", withMinRevision(t, map[string]string{"alias": "beta-1"}, rev))
	requireNotReady(t, "alias", err)
	stopComponent(t, h.c)

	second := restart(t, client, `{}`)
	second.awaitIndexed(t, rev)
	if got := resolveAlias(t, second, "beta-1", rev); got != id {
		t.Errorf("alias beta-1 after the restart = %q, want %q", got, id)
	}
	if got := resolveAlias(t, second, "alpha-1", rev); got != "" {
		t.Errorf("alias alpha-1 after the restart = %q, want none", got)
	}
	requireCleanRecord(t, client, id)
}

// TestSecondWriterFailsRecordWrite: another writer puts an entity's record between an apply's
// record read and its pending put, as a second graph-index on the same store could. The apply's
// pending put is refused as a conflict, it writes no row, and it is not retried in place; once the
// entity is failed, the repair pass converges it from the stored record, which then names every row
// the entity owns.
//
// The apply is driven by its own two steps, prepareApply (the read) and runApply (the writes from
// the pending put on), because the second writer must land between them and nothing else orders it
// there.
func TestSecondWriterFailsRecordWrite(t *testing.T) {
	client := fixtureClient(t)
	h := startIndex(t, client, `{}`)
	id := entityID(1)
	valueA := entityValue(t, id, stmt{linkTo, entityID(2)}, stmt{alias, "alpha-1"})
	h.awaitIndexed(t, h.put(t, id, valueA))
	valueB := entityValue(t, id, stmt{linkTo, entityID(3)}, stmt{alias, "beta-1"})
	apply := prepareFor(t, h.c, id, valueB)
	if len(apply.ops) == 0 || apply.ops[0].kind != opPending {
		t.Fatalf("the apply from A to B starts with %v, want the pending record", apply.ops)
	}

	// The second writer: another graph-index's apply of B, cut after its pending put and one row.
	second := openWriter(t, client)
	secondApply := prepareFor(t, second, id, valueB)
	runPrefix(t, second, secondApply, 2)
	rowsBefore, recordBefore := ownedRows(t, client, id), recordSequence(t, client)
	sequencesBefore := indexSequences(t, client)

	err := h.c.runApply(t.Context(), apply)
	if !errors.Is(err, errRecordConflict) {
		t.Fatalf("runApply after the second writer: %v, want errRecordConflict", err)
	}
	if got := indexSequences(t, client); !maps.Equal(got, sequencesBefore) {
		t.Errorf("index stream sequences after the refused apply = %v, want %v: a row was written", got, sequencesBefore)
	}
	if got := recordSequence(t, client); got != recordBefore {
		t.Errorf("record stream sequence after the refused apply = %d, want %d", got, recordBefore)
	}
	attempts := 0
	retryErr := retryIndexWrites(t.Context(), func() error { attempts++; return h.c.runApply(t.Context(), apply) })
	if !errors.Is(retryErr, errRecordConflict) || attempts != 1 {
		t.Errorf("the lane's retry of the refused apply: %d attempts, %v; want one, a conflict", attempts, retryErr)
	}
	r, _ := storedRecord(t, client, id)
	if problems := recordNames(r, rowsBefore, false); len(problems) > 0 {
		t.Errorf("the second writer's pending record does not name every row: %v", problems)
	}

	// The lane fails an entity whose writes ended in a conflict (indexEntity); the store holds A, so
	// the repair converges the second writer's cut back to A.
	h.c.markEntityFailed(id)
	h.c.repairFailedEntities(t.Context())
	await(t, "the repair pass converges the entity",
		func(context.Context) (int64, error) { return h.c.failedTotal(), nil },
		func(n int64) bool { return n == 0 })
	requireCleanRecord(t, client, id)
	if got := resolveAlias(t, h, "alpha-1", 0); got != id {
		t.Errorf("alias alpha-1 after the repair = %q, want %q", got, id)
	}
	if got := resolveAlias(t, h, "beta-1", 0); got != "" {
		t.Errorf("alias beta-1 after the repair = %q, want none", got)
	}
}

// openWriter is a graph-index on client that has opened its buckets and read the vocabulary but
// runs nothing: a writer whose applies the test drives.
func openWriter(t *testing.T, client *natsclient.Client) *Component {
	t.Helper()
	c, _ := newIndex(t, client, `{}`)
	if err := c.openIndexBuckets(t.Context()); err != nil {
		t.Fatalf("open the index buckets: %v", err)
	}
	c.aliasPredicates = vocabulary.DiscoverAliasPredicates()
	c.namePredicates = vocabulary.DiscoverLabelPredicates()
	return c
}

// plannedFor is the rows c plans for id's stored value, none for nil (a deleted entity).
func plannedFor(t *testing.T, c *Component, id string, value []byte) []indexRow {
	t.Helper()
	if value == nil {
		return nil
	}
	var state graph.EntityState
	if err := graph.UnmarshalEntityState(id, value, &state); err != nil {
		t.Fatalf("decode the value of %s: %v", id, err)
	}
	plan, err := c.buildEntityIndexPlan(state, id)
	if err != nil {
		t.Fatalf("plan %s: %v", id, err)
	}
	rows, err := c.plannedRows(plan)
	if err != nil {
		t.Fatalf("rows of %s: %v", id, err)
	}
	return rows
}

// prepareFor is c's apply of value for id from what the store holds now.
func prepareFor(t *testing.T, c *Component, id string, value []byte) entityApply {
	t.Helper()
	apply, err := c.prepareApply(t.Context(), id, plannedFor(t, c, id, value))
	if err != nil {
		t.Fatalf("prepare the apply of %s: %v", id, err)
	}
	return apply
}

// runPrefix runs the first n of apply's operations, as an apply cut off after them would have.
func runPrefix(t *testing.T, c *Component, apply entityApply, n int) {
	t.Helper()
	cut := apply
	cut.ops = apply.ops[:n]
	if err := c.runApply(t.Context(), cut); err != nil {
		t.Fatalf("run %d of %d operations for %s: %v", n, len(apply.ops), apply.entityID, err)
	}
}
