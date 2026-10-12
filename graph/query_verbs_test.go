package graph

import (
	"reflect"
	"slices"
	"testing"
)

// TestQueryVerbsDeclareGraphIngestVerbs holds design D20: graph-ingest answers
// exactly entity, batch and prefix on graph.ingest.query.*, and each entry
// names its subject, responder, request type and reply type. There is no
// suffix verb (design D19).
func TestQueryVerbsDeclareGraphIngestVerbs(t *testing.T) {
	// A request without a Go type is written as its JSON fields; a named type
	// is spelled by reflect, so renaming the type fails here.
	want := []QueryVerb{
		{
			Name: "entity", Subject: "graph.ingest.query.entity", Responder: "graph-ingest",
			RequestType: "{id:string}", ReplyType: reflect.TypeFor[ExactEntity]().String(),
		},
		{
			Name: "batch", Subject: "graph.ingest.query.batch", Responder: "graph-ingest",
			RequestType: "{ids:[]string}", ReplyType: reflect.TypeFor[EntityBatchResponse]().String(),
		},
		{
			Name: "prefix", Subject: "graph.ingest.query.prefix", Responder: "graph-ingest",
			RequestType: reflect.TypeFor[PrefixQueryRequest]().String(),
			ReplyType:   reflect.TypeFor[PrefixQueryResponse]().String(),
		},
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
