package graph

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
)

// TestQueryVerbsDeclareEachRespondersVerbs: graph-ingest answers exactly
// entity, batch, prefix and prefixIDs on graph.ingest.query.*, and each entry names its
// subject and responder. There is no suffix verb. graph-index's eight verbs
// follow on graph.index.query.*. graph-query's forwards follow on
// graph.query.*, with the derived-index verbs four of them forward to, and
// graph-lexical's search on graph.lexical.query.*; no other entry exists.
func TestQueryVerbsDeclareEachRespondersVerbs(t *testing.T) {
	// The marks are literals per verb, read from what each handler replies with
	// (graph-transport-boundary, "The verb table declares each reply's shape"):
	// graph-ingest's replies are bare, graph-index's are enveloped, a forward
	// answers as the verb it forwards to, and the two verbs graph-query composes
	// from an index reply are enveloped. The spatial and temporal verbs have no
	// handler in the tree yet; their marks are the shapes the table declares for them.
	want := []QueryVerb{
		{Name: "entity", Subject: "graph.ingest.query.entity", Responder: "graph-ingest",
			Enveloped: false, Reply: reflect.TypeFor[ExactEntity]()},
		{Name: "batch", Subject: "graph.ingest.query.batch", Responder: "graph-ingest",
			Enveloped: false, Reply: reflect.TypeFor[EntityBatchResponse]()},
		{Name: "prefix", Subject: "graph.ingest.query.prefix", Responder: "graph-ingest",
			Enveloped: false, Reply: reflect.TypeFor[PrefixQueryResponse]()},
		{Name: "prefixIDs", Subject: "graph.ingest.query.prefixIDs", Responder: "graph-ingest",
			Enveloped: false, Reply: reflect.TypeFor[PrefixIDsResponse]()},
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
		{Name: "search", Subject: "graph.embedding.query.search", Responder: "graph-embedding",
			Enveloped: true, Reply: reflect.TypeFor[SemanticData]()},
		{Name: "similar", Subject: "graph.embedding.query.similar", Responder: "graph-embedding",
			Enveloped: true, Reply: reflect.TypeFor[SimilarData]()},
		{Name: "bounds", Subject: "graph.spatial.query.bounds", Responder: "graph-index-spatial",
			Enveloped: true, Reply: reflect.TypeFor[SpatialData]()},
		{Name: "range", Subject: "graph.temporal.query.range", Responder: "graph-index-temporal",
			Enveloped: true, Reply: reflect.TypeFor[TemporalData]()},
		{Name: "search", Subject: "graph.lexical.query.search", Responder: "graph-lexical",
			Enveloped: true, Reply: reflect.TypeFor[LexicalSearchData]()},
		{Name: "entity", Subject: "graph.query.entity", Responder: "graph-query",
			Enveloped: false, Reply: reflect.TypeFor[ExactEntity]()},
		{Name: "batch", Subject: "graph.query.batch", Responder: "graph-query",
			Enveloped: false, Reply: reflect.TypeFor[EntityBatchResponse]()},
		{Name: "prefix", Subject: "graph.query.prefix", Responder: "graph-query",
			Enveloped: false, Reply: reflect.TypeFor[PrefixQueryResponse]()},
		{Name: "prefixIDs", Subject: "graph.query.prefixIDs", Responder: "graph-query",
			Enveloped: false, Reply: reflect.TypeFor[PrefixIDsResponse]()},
		{Name: "byName", Subject: "graph.query.byName", Responder: "graph-query",
			Enveloped: true, Reply: reflect.TypeFor[NameData]()},
		{Name: "predicate", Subject: "graph.query.predicate", Responder: "graph-query",
			Enveloped: true, Reply: reflect.TypeFor[PredicateData]()},
		{Name: "entityByAlias", Subject: "graph.query.entityByAlias", Responder: "graph-query",
			Enveloped: true, Reply: reflect.TypeFor[ExactEntity]()},
		{Name: "relationships", Subject: "graph.query.relationships", Responder: "graph-query",
			Enveloped: true, Reply: reflect.TypeFor[[]RelationshipEdge]()},
		{Name: "pathSearch", Subject: "graph.query.pathSearch", Responder: "graph-query",
			Enveloped: true, Reply: reflect.TypeFor[PathSearchData]()},
		{Name: "hierarchyStats", Subject: "graph.query.hierarchyStats", Responder: "graph-query",
			Enveloped: false, Reply: reflect.TypeFor[HierarchyStatsData]()},
		{Name: "summary", Subject: "graph.query.summary", Responder: "graph-query",
			Enveloped: true, Reply: reflect.TypeFor[SummaryData]()},
		{Name: "localSearch", Subject: "graph.query.localSearch", Responder: "graph-query",
			Enveloped: true, Reply: reflect.TypeFor[LocalSearchData]()},
		{Name: "globalSearch", Subject: "graph.query.globalSearch", Responder: "graph-query",
			Enveloped: true, Reply: reflect.TypeFor[GlobalSearchData]()},
		{Name: "searchGraph", Subject: "graph.query.searchGraph", Responder: "graph-query",
			Enveloped: true, Reply: reflect.TypeFor[GlobalSearchData]()},
		{Name: "semantic", Subject: "graph.query.semantic", Responder: "graph-query",
			Enveloped: true, Reply: reflect.TypeFor[SemanticData]()},
		{Name: "similar", Subject: "graph.query.similar", Responder: "graph-query",
			Enveloped: true, Reply: reflect.TypeFor[SimilarData]()},
		{Name: "spatial", Subject: "graph.query.spatial", Responder: "graph-query",
			Enveloped: true, Reply: reflect.TypeFor[SpatialData]()},
		{Name: "temporal", Subject: "graph.query.temporal", Responder: "graph-query",
			Enveloped: true, Reply: reflect.TypeFor[TemporalData]()},
	}

	if ResponderGraphIngest != "graph-ingest" || ResponderGraphIndex != "graph-index" || ResponderGraphQuery != "graph-query" {
		t.Fatalf("responders = %q, %q, %q; want graph-ingest, graph-index, graph-query",
			ResponderGraphIngest, ResponderGraphIndex, ResponderGraphQuery)
	}
	if ResponderGraphEmbedding != "graph-embedding" || ResponderGraphIndexSpatial != "graph-index-spatial" ||
		ResponderGraphIndexTemporal != "graph-index-temporal" || ResponderGraphLexical != "graph-lexical" {
		t.Fatalf("responders = %q, %q, %q, %q; want graph-embedding, graph-index-spatial, graph-index-temporal, graph-lexical",
			ResponderGraphEmbedding, ResponderGraphIndexSpatial, ResponderGraphIndexTemporal, ResponderGraphLexical)
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

// TestPredicateRequestWireNames holds the keys graph-index's predicate verb reads, and that
// an empty value still arrives as a value: a nil Value means no value filter.
func TestPredicateRequestWireNames(t *testing.T) {
	empty := ""
	for _, tc := range []struct {
		request PredicateRequest
		want    string
	}{
		{PredicateRequest{Predicate: "test.rel.knows"}, `{"predicate":"test.rel.knows"}`},
		{PredicateRequest{Predicate: "test.rel.knows", Value: &empty, Limit: 3, MinRevisionField: MinRevisionField{MinRevision: 4}},
			`{"predicate":"test.rel.knows","value":"","limit":3,"min_revision":4}`},
	} {
		got, err := json.Marshal(tc.request)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != tc.want {
			t.Errorf("PredicateRequest = %s, want %s", got, tc.want)
		}
		var back PredicateRequest
		if err := json.Unmarshal(got, &back); err != nil {
			t.Fatal(err)
		}
		if (back.Value == nil) != (tc.request.Value == nil) {
			t.Errorf("%s decodes Value %v, want nil %v", got, back.Value, tc.request.Value == nil)
		}
	}
}

// TestPathSearchWireNames holds the keys of the pathSearch request and reply and the limits callers read.
func TestPathSearchWireNames(t *testing.T) {
	request, err := json.Marshal(PathSearchRequest{
		StartEntity: "start", Direction: "both", MaxDepth: 2, MaxNodes: 3, Predicates: []string{"test.rel.knows"},
		MinRevisionField: MinRevisionField{MinRevision: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"start_entity":"start","direction":"both","max_depth":2,"max_nodes":3,"predicates":["test.rel.knows"],"min_revision":4}`; string(request) != want {
		t.Errorf("PathSearchRequest = %s, want %s", request, want)
	}
	reply, err := json.Marshal(PathSearchData{
		Entities: []PathEntity{{EntityID: "from", Depth: 0}, {EntityID: "to", Depth: 1}},
		Edges:    []RelationshipEdge{{Predicate: "test.rel.knows", FromEntityID: "from", ToEntityID: "to"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"entities":[{"entity_id":"from","depth":0},{"entity_id":"to","depth":1}],` +
		`"edges":[{"predicate":"test.rel.knows","from_entity_id":"from","to_entity_id":"to"}],"truncated":false}`
	if string(reply) != want {
		t.Errorf("PathSearchData = %s, want %s", reply, want)
	}
	if MaxPathSearchDepth != 10 || MaxPathSearchNodes != 500 || DefaultPathSearchNodes != 100 {
		t.Errorf("limits = %d, %d, %d, want 10, 500, 100", MaxPathSearchDepth, MaxPathSearchNodes, DefaultPathSearchNodes)
	}
}

// TestQueryWireNames holds the spellings callers and responders meet on the wire:
// a relationships row and min_revision with its refusal code.
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
	if ErrorCodeNotEnabled != "not_enabled" {
		t.Fatalf("ErrorCodeNotEnabled = %q, want not_enabled", ErrorCodeNotEnabled)
	}
}
