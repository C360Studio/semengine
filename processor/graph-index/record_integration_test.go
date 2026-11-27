//go:build integration

package graphindex

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"testing"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/vocabulary"
)

// The tests in this file count graph-index's own KV operations through the counter
// semengine_graph_index_kv_operations_total on the registry the component was built with. They
// rely on its two labels only: operation, where "list" is an owner listing and "put" and "delete"
// are row writes, and kv_bucket, which names the index a row belongs to. An index's label is the
// lower-case first word of its bucket name, so indexRowLabels is spelled from the bucket names,
// not taken from the code under test.

// indexRowLabels maps each index bucket to its kv_bucket label.
var indexRowLabels = map[string]string{
	graph.BucketOutgoingIndex:  "outgoing",
	graph.BucketIncomingIndex:  "incoming",
	graph.BucketPredicateIndex: "predicate",
	graph.BucketNameIndex:      "name",
	graph.BucketAliasIndex:     "alias",
}

// kvOperations is kv_operations_total on c's registry, by operation and then by kv_bucket.
func kvOperations(t *testing.T, c *Component) map[string]map[string]float64 {
	t.Helper()
	families, err := c.metricsRegistry.PrometheusRegistry().Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	got := map[string]map[string]float64{}
	for _, family := range families {
		if family.GetName() != "semengine_graph_index_kv_operations_total" {
			continue
		}
		for _, m := range family.GetMetric() {
			labels := map[string]string{}
			for _, label := range m.GetLabel() {
				labels[label.GetName()] = label.GetValue()
			}
			operation, bucket := labels["operation"], labels["kv_bucket"]
			if got[operation] == nil {
				got[operation] = map[string]float64{}
			}
			got[operation][bucket] += m.GetCounter().GetValue()
		}
	}
	return got
}

// listings is the number of owner listings in ops, over every kv_bucket.
func listings(ops map[string]map[string]float64) float64 {
	var n float64
	for _, count := range ops["list"] {
		n += count
	}
	return n
}

// rowWrites is the number of row puts and deletes in ops, over the five index buckets.
func rowWrites(ops map[string]map[string]float64) float64 {
	var n float64
	for _, label := range indexRowLabels {
		n += ops["put"][label] + ops["delete"][label]
	}
	return n
}

// indexSequences is the last sequence of each index bucket's stream. Every put and every delete
// of a row appends to its bucket's stream, so equal sequences before and after mean no row was
// written, whatever the component counts.
func indexSequences(t *testing.T, client *natsclient.Client) map[string]uint64 {
	t.Helper()
	sequences := map[string]uint64{}
	for bucket := range indexRowLabels {
		reader, err := kvcatalog.OpenCatalogReader(t.Context(), client, bucket)
		if err != nil {
			t.Fatalf("open %s: %v", bucket, err)
		}
		seq, err := natsclient.BucketLastSeq(t.Context(), reader)
		if err != nil {
			t.Fatalf("last sequence of %s: %v", bucket, err)
		}
		sequences[bucket] = seq
	}
	return sequences
}

// indexRead is one public read: a verb and its request body.
type indexRead struct {
	verb string
	body any
}

// readAll sends every read and returns each reply's data as the wire spelled it, keyed by verb and
// request. Each reply carries at least minRevision.
func readAll(t *testing.T, h *harness, reads []indexRead, minRevision uint64) map[string]string {
	t.Helper()
	answers := map[string]string{}
	for _, r := range reads {
		body, err := json.Marshal(r.body)
		if err != nil {
			t.Fatal(err)
		}
		answers[r.verb+" "+string(body)] = string(answer[json.RawMessage](t, h.client, r.verb, r.body, minRevision))
	}
	return answers
}

// restart builds a second graph-index from raw on a registry of its own and starts it on client,
// as a process restart does; the test's cleanup stops it.
func restart(t *testing.T, client *natsclient.Client, raw string) *harness {
	t.Helper()
	states := ensureEntityStates(t, client)
	c, logs := newIndex(t, client, raw)
	startComponent(t, c)
	return &harness{c: c, f: c.follower.Load(), states: states, client: client, logs: logs}
}

// TestWarmRestartSkipsIndexedEntities indexes a store of entities that carry every kind of row
// (outgoing, incoming, predicate, name and alias), one of them deleted, stops graph-index and
// starts it again with nothing changed. The second start lists no index bucket and writes no index
// row: its own counter shows no list, put or delete, and no index bucket's stream moved. DataFlow
// counts each live entity read once, and every public read answers what it answered before the
// stop.
func TestWarmRestartSkipsIndexedEntities(t *testing.T) {
	client := fixtureClient(t)
	const raw = `{}`
	first := startIndex(t, client, raw)
	title := vocabulary.DCTermsTitle
	a, b, c, gone, last := entityID(1), entityID(2), entityID(3), entityID(4), entityID(5)

	first.put(t, a, entityValue(t, a, stmt{linkTo, b}, stmt{armed, true}, stmt{title, "Alpha"}, stmt{alias, "alpha-1"}))
	first.put(t, b, entityValue(t, b, stmt{linkTo, c}, stmt{linkTo, a}, stmt{disarmed, true}, stmt{title, "Beta"}))
	first.put(t, c, entityValue(t, c, stmt{armed, false}, stmt{alias, "gamma-1"}))
	first.put(t, gone, entityValue(t, gone, stmt{linkTo, a}, stmt{title, "Delta"}, stmt{alias, "delta-1"}))
	if err := first.states.Delete(t.Context(), gone); err != nil {
		t.Fatalf("delete %s: %v", gone, err)
	}
	// The last write's revision is above every other, the delete's included: the floor reaching it
	// orders every read below after every apply.
	lastRev := first.put(t, last, entityValue(t, last, stmt{linkTo, a}))
	first.awaitIndexed(t, lastRev)
	live := 4 // a, b, c and last

	reads := []indexRead{{"predicateList", map[string]string{}}}
	for _, id := range []string{a, b, c, gone, last} {
		reads = append(reads,
			indexRead{"outgoing", map[string]string{"entity_id": id}},
			indexRead{"incoming", map[string]string{"entity_id": id}})
	}
	for _, aliasValue := range []string{"alpha-1", "gamma-1", "delta-1"} {
		reads = append(reads, indexRead{"alias", map[string]string{"alias": aliasValue}})
	}
	for _, p := range []string{linkTo, armed, disarmed, title, alias} {
		reads = append(reads,
			indexRead{"predicate", map[string]string{"predicate": p}},
			indexRead{"predicateStats", map[string]string{"predicate": p}})
	}
	for _, name := range []string{"Alpha", "Beta", "Delta"} {
		reads = append(reads, indexRead{"byName", map[string]string{"name": name}})
	}
	reads = append(reads,
		indexRead{"predicateCompound", map[string]any{"predicates": []string{linkTo, armed}, "operator": "AND"}},
		indexRead{"predicateCompound", map[string]any{"predicates": []string{armed, disarmed}, "operator": "OR"}})
	before := readAll(t, first, reads, lastRev)
	// "As before" means something only if the first start indexed the store: each kind of row
	// answers, and the deleted entity answers nothing.
	out := answer[graph.OutgoingRelationshipsData](t, client, "outgoing", map[string]string{"entity_id": b}, lastRev)
	requireRelationship(t, "first start: outgoing "+b, outgoingTargets(out.Relationships), []string{linkTo + ">" + c, linkTo + ">" + a})
	in := answer[graph.IncomingRelationshipsData](t, client, "incoming", map[string]string{"entity_id": a}, lastRev)
	requireRelationship(t, "first start: incoming "+a, sortedIDs(incomingSources(in.Relationships)),
		sortedIDs([]string{linkTo + "<" + b, linkTo + "<" + last}))
	names := answer[graph.NameData](t, client, "byName", map[string]string{"name": "Alpha"}, lastRev).Matches
	requireRelationship(t, "first start: byName Alpha", nameIDs(names), []string{a})
	for aliasValue, want := range map[string]string{"alpha-1": a, "gamma-1": c, "delta-1": ""} {
		if got := resolveAlias(t, first, aliasValue, lastRev); got != want {
			t.Errorf("first start: alias %q = %q, want %q", aliasValue, got, want)
		}
	}
	sequencesBefore := indexSequences(t, client)
	stopComponent(t, first.c)

	second := restart(t, client, raw)
	// A lane counts its read and its KV operations before it completes the revision, so the floor
	// reaching the last revision orders the counter, DataFlow and sequence reads below after every
	// apply of the second start.
	second.awaitIndexed(t, lastRev)
	await(t, "the second start ready for queries",
		func(ctx context.Context) (error, error) { return second.f.QueryReady(ctx), nil },
		func(err error) bool { return err == nil })

	ops := kvOperations(t, second.c)
	if n := listings(ops); n != 0 {
		t.Errorf("second start: %v owner listings (kv_operations_total{operation=\"list\"} %v), want 0", n, ops["list"])
	}
	if n := rowWrites(ops); n != 0 {
		t.Errorf("second start: %v index row writes (kv_operations_total put %v, delete %v), want 0",
			n, ops["put"], ops["delete"])
	}
	if got := indexSequences(t, client); !maps.Equal(got, sequencesBefore) {
		t.Errorf("index stream sequences after the second start = %v, want %v, unchanged: a row was written",
			got, sequencesBefore)
	}
	if got := second.c.DataFlow().Messages; got != int64(live) {
		t.Errorf("second start DataFlow messages = %d, want %d, one per live entity read", got, live)
	}
	after := readAll(t, second, reads, lastRev)
	for _, read := range slices.Sorted(maps.Keys(before)) {
		if after[read] != before[read] {
			t.Errorf("%s after the restart = %s, want %s, as before the stop", read, after[read], before[read])
		}
	}
	if n := second.c.failedTotal(); n != 0 {
		t.Errorf("failed entities after the second start = %d, want 0", n)
	}
}

// TestLiveChangeUsesRecord changes an indexed entity from A to B while graph-index runs. Every
// public read answers B for it and nothing of A, another entity's edge to it survives, and the
// change lists no index bucket: kv_operations_total{operation="list"} does not move across it.
func TestLiveChangeUsesRecord(t *testing.T) {
	h := startIndex(t, fixtureClient(t), `{}`)
	title := vocabulary.DCTermsTitle
	src, other, targetA, targetB := entityID(1), entityID(2), entityID(10), entityID(11)

	h.put(t, other, entityValue(t, other, stmt{linkTo, src}, stmt{alias, "other-1"}))
	revA := h.put(t, src, entityValue(t, src,
		stmt{linkTo, targetA}, stmt{armed, true}, stmt{title, "Alpha"}, stmt{alias, "alpha-1"}))
	h.awaitIndexed(t, revA)
	listedBefore := listings(kvOperations(t, h.c))

	revB := h.put(t, src, entityValue(t, src,
		stmt{linkTo, targetB}, stmt{disarmed, true}, stmt{title, "Beta"}, stmt{alias, "beta-1"}))
	// The lane counts its KV operations before it completes the revision, so the floor reaching B
	// orders the counter read and the public reads after B's apply.
	h.awaitIndexed(t, revB)
	if listed := listings(kvOperations(t, h.c)) - listedBefore; listed != 0 {
		t.Errorf("the change A to B made %v owner listings, want 0", listed)
	}

	out := answer[graph.OutgoingRelationshipsData](t, h.client, "outgoing", map[string]string{"entity_id": src}, revB)
	if want := []graph.OutgoingEntry{{ToEntityID: targetB, Predicate: linkTo}}; !slices.Equal(out.Relationships, want) {
		t.Errorf("outgoing %s = %v, want %v", src, out.Relationships, want)
	}
	for target, want := range map[string][]graph.IncomingEntry{
		targetA: {},
		targetB: {{FromEntityID: src, Predicate: linkTo}},
		src:     {{FromEntityID: other, Predicate: linkTo}},
	} {
		in := answer[graph.IncomingRelationshipsData](t, h.client, "incoming", map[string]string{"entity_id": target}, revB)
		if !slices.Equal(in.Relationships, want) {
			t.Errorf("incoming %s = %v, want %v", target, in.Relationships, want)
		}
	}
	for predicate, want := range map[string][]string{
		linkTo: {src, other}, armed: {}, disarmed: {src}, title: {src}, alias: {src, other},
	} {
		got := answer[graph.PredicateData](t, h.client, "predicate", map[string]string{"predicate": predicate}, revB)
		if !slices.Equal(sortedIDs(got.Entities), sortedIDs(want)) {
			t.Errorf("predicate %s = %v, want %v", predicate, got.Entities, want)
		}
	}
	for name, want := range map[string][]string{"Alpha": {}, "Beta": {src}} {
		matches := answer[graph.NameData](t, h.client, "byName", map[string]string{"name": name}, revB).Matches
		if got := nameIDs(matches); !slices.Equal(got, want) {
			t.Errorf("byName %q = %v, want %v", name, got, want)
		}
	}
	for a, want := range map[string]string{"alpha-1": "", "beta-1": src, "other-1": other} {
		if got := resolveAlias(t, h, a, revB); got != want {
			t.Errorf("alias %q = %q, want %q", a, got, want)
		}
	}
	if n := h.c.failedTotal(); n != 0 {
		t.Errorf("failed entities = %d, want 0", n)
	}
}

// sortedIDs is ids sorted, so a comparison does not depend on reply order.
func sortedIDs(ids []string) []string {
	return slices.Sorted(slices.Values(ids))
}
