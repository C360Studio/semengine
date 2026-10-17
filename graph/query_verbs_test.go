package graph

import (
	"slices"
	"testing"
)

// TestQueryVerbsDeclareGraphIngestVerbs holds design D20: graph-ingest answers
// exactly entity, batch and prefix on graph.ingest.query.*, and each entry
// names its subject and responder. There is no suffix verb (design D19).
func TestQueryVerbsDeclareGraphIngestVerbs(t *testing.T) {
	want := []QueryVerb{
		{Name: "entity", Subject: "graph.ingest.query.entity", Responder: "graph-ingest"},
		{Name: "batch", Subject: "graph.ingest.query.batch", Responder: "graph-ingest"},
		{Name: "prefix", Subject: "graph.ingest.query.prefix", Responder: "graph-ingest"},
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
