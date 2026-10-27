package graph

import (
	"slices"
	"testing"
)

// TestQueryVerbsDeclareEachRespondersVerbs holds design D20: graph-ingest answers
// exactly entity, batch and prefix on graph.ingest.query.*, and each entry
// names its subject and responder. There is no suffix verb (design D19).
// graph-index's eight verbs follow on graph.index.query.* (change 4, design
// D6), the subjects the SemStreams pin's graph-index subscribed to
// (pin:processor/graph-index/query.go:31-80); no other entry exists.
func TestQueryVerbsDeclareEachRespondersVerbs(t *testing.T) {
	want := []QueryVerb{
		{Name: "entity", Subject: "graph.ingest.query.entity", Responder: "graph-ingest"},
		{Name: "batch", Subject: "graph.ingest.query.batch", Responder: "graph-ingest"},
		{Name: "prefix", Subject: "graph.ingest.query.prefix", Responder: "graph-ingest"},
		{Name: "outgoing", Subject: "graph.index.query.outgoing", Responder: "graph-index"},
		{Name: "incoming", Subject: "graph.index.query.incoming", Responder: "graph-index"},
		{Name: "alias", Subject: "graph.index.query.alias", Responder: "graph-index"},
		{Name: "predicate", Subject: "graph.index.query.predicate", Responder: "graph-index"},
		{Name: "predicateList", Subject: "graph.index.query.predicateList", Responder: "graph-index"},
		{Name: "predicateStats", Subject: "graph.index.query.predicateStats", Responder: "graph-index"},
		{Name: "predicateCompound", Subject: "graph.index.query.predicateCompound", Responder: "graph-index"},
		{Name: "byName", Subject: "graph.index.query.byName", Responder: "graph-index"},
	}

	if ResponderGraphIngest != "graph-ingest" || ResponderGraphIndex != "graph-index" {
		t.Fatalf("responders = %q, %q; want graph-ingest, graph-index", ResponderGraphIngest, ResponderGraphIndex)
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
