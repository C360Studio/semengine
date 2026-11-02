package graph

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
)

// TestQueryVerbsDeclareEachRespondersVerbs holds design D20: graph-ingest answers
// exactly entity, batch and prefix on graph.ingest.query.*, and each entry
// names its subject and responder. There is no suffix verb (design D19).
// graph-index's eight verbs follow on graph.index.query.* (change 4, design
// D6), the subjects the SemStreams pin's graph-index subscribed to
// (pin:processor/graph-index/query.go:31-80). graph-query's forwards follow on
// graph.query.* (design D1); no other entry exists.
func TestQueryVerbsDeclareEachRespondersVerbs(t *testing.T) {
	// The marks are literals per verb, read from what each handler replies with
	// (graph-transport-boundary, "The verb table declares each reply's shape"):
	// graph-ingest's replies are bare, graph-index's are enveloped, and a forward
	// answers as the verb it forwards to.
	want := []QueryVerb{
		{Name: "entity", Subject: "graph.ingest.query.entity", Responder: "graph-ingest",
			Enveloped: false, Reply: reflect.TypeFor[ExactEntity]()},
		{Name: "batch", Subject: "graph.ingest.query.batch", Responder: "graph-ingest",
			Enveloped: false, Reply: reflect.TypeFor[EntityBatchResponse]()},
		{Name: "prefix", Subject: "graph.ingest.query.prefix", Responder: "graph-ingest",
			Enveloped: false, Reply: reflect.TypeFor[PrefixQueryResponse]()},
		{Name: "outgoing", Subject: "graph.index.query.outgoing", Responder: "graph-index",
			Enveloped: true, Reply: reflect.TypeFor[OutgoingRelationshipsData]()},
		{Name: "incoming", Subject: "graph.index.query.incoming", Responder: "graph-index",
			Enveloped: true, Reply: reflect.TypeFor[IncomingRelationshipsData]()},
		{Name: "alias", Subject: "graph.index.query.alias", Responder: "graph-index",
			Enveloped: true, Reply: reflect.TypeFor[AliasData]()},
		{Name: "predicate", Subject: "graph.index.query.predicate", Responder: "graph-index",
			Enveloped: true, Reply: reflect.TypeFor[PredicateData]()},
		{Name: "predicateList", Subject: "graph.index.query.predicateList", Responder: "graph-index",
			Enveloped: true, Reply: reflect.TypeFor[PredicateListData]()},
		{Name: "predicateStats", Subject: "graph.index.query.predicateStats", Responder: "graph-index",
			Enveloped: true, Reply: reflect.TypeFor[PredicateStatsData]()},
		{Name: "predicateCompound", Subject: "graph.index.query.predicateCompound", Responder: "graph-index",
			Enveloped: true, Reply: reflect.TypeFor[CompoundPredicateData]()},
		{Name: "byName", Subject: "graph.index.query.byName", Responder: "graph-index",
			Enveloped: true, Reply: reflect.TypeFor[NameData]()},
		{Name: "entity", Subject: "graph.query.entity", Responder: "graph-query",
			Enveloped: false, Reply: reflect.TypeFor[ExactEntity]()},
		{Name: "batch", Subject: "graph.query.batch", Responder: "graph-query",
			Enveloped: false, Reply: reflect.TypeFor[EntityBatchResponse]()},
		{Name: "prefix", Subject: "graph.query.prefix", Responder: "graph-query",
			Enveloped: false, Reply: reflect.TypeFor[PrefixQueryResponse]()},
		{Name: "byName", Subject: "graph.query.byName", Responder: "graph-query",
			Enveloped: true, Reply: reflect.TypeFor[NameData]()},
	}

	if ResponderGraphIngest != "graph-ingest" || ResponderGraphIndex != "graph-index" || ResponderGraphQuery != "graph-query" {
		t.Fatalf("responders = %q, %q, %q; want graph-ingest, graph-index, graph-query",
			ResponderGraphIngest, ResponderGraphIndex, ResponderGraphQuery)
	}

	got := QueryVerbs()
	if !slices.Equal(got, want) {
		t.Fatalf("QueryVerbs() =\n  %+v\nwant\n  %+v", got, want)
	}

	// The table is a declaration: a caller that edits its copy changes nothing.
	got[0].Subject = "graph.ingest.query.changed"
	if again := QueryVerbs(); !slices.Equal(again, want) {
		t.Fatalf("editing a returned table changed the declaration: %+v", again)
	}
}

// TestQueryWireNames holds the spellings callers and responders meet on the wire:
// a relationships row (design D5) and min_revision with its refusal code (design D3).
func TestQueryWireNames(t *testing.T) {
	row, err := json.Marshal(RelationshipEdge{Predicate: "test.rel.knows", FromEntityID: "from", ToEntityID: "to"})
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"predicate":"test.rel.knows","from_entity_id":"from","to_entity_id":"to"}`; string(row) != want {
		t.Fatalf("RelationshipEdge = %s, want %s", row, want)
	}

	// Embedded, as a request embeds it: its key sits at the request's top level,
	// and zero, meaning none, sends no key.
	var request struct {
		MinRevisionField
	}
	unset, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if string(unset) != `{}` {
		t.Fatalf("unset MinRevisionField = %s, want {}", unset)
	}
	if err := json.Unmarshal([]byte(`{"min_revision":7}`), &request); err != nil {
		t.Fatal(err)
	}
	if request.MinRevision != 7 {
		t.Fatalf("MinRevision = %d, want 7", request.MinRevision)
	}

	if ErrorCodeRevisionNotReached != "revision_not_reached" {
		t.Fatalf("ErrorCodeRevisionNotReached = %q, want revision_not_reached", ErrorCodeRevisionNotReached)
	}
}
