package graphindex

import (
	"context"
	"encoding/hex"
	"errors"
	"math"
	"slices"
	"testing"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/pkg/errs"
)

// TestQueryRefusesInvalidRequestsBeforeAnyRead: each malformed request on each verb is refused
// with class invalid, code invalid_request. The component is built but never started, so it has
// no follower and no buckets: a request that got past validation to the readiness check or a
// bucket read would panic rather than return (the pin's TestQueryInvalidRequest_Integration and its
// "...HasNoBucketIO" tests).
func TestQueryRefusesInvalidRequestsBeforeAnyRead(t *testing.T) {
	c, err := createFromJSON(t, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	handlers := c.queryHandlers()
	cases := map[string][]string{
		"outgoing": {`not json`, `{}`, `{"entity_id":"acme.ops.test.system.widget"}`,
			`{"entity_id":"acme.ops.test.system.widget.*"}`, `{"entity_id":"acme.ops.test.system.widget.a b"}`},
		"incoming": {`not json`, `{}`, `{"entity_id":"acme.ops.test.system.widget"}`,
			`{"entity_id":"acme.ops.test.system.widget.>"}`},
		"alias":          {`not json`, `{}`, `{"alias":"two words"}`, `{"alias":"alpha.>"}`, `{"alias":"alpha.*"}`},
		"predicate":      {`not json`, `{}`, `{"predicate":"test.link"}`, `{"predicate":"test.link.*"}`, `{"predicate":"test.link.to.far"}`},
		"predicateList":  {`not json`, `{"unknown":1}`, `{"namespace":"test.link.to"}`, `{"namespace":"*"}`, `{"namespace":"te st"}`},
		"predicateStats": {`not json`, `{}`, `{"predicate":"test.link"}`},
		"predicateCompound": {`not json`, `{"predicates":[],"operator":"AND"}`,
			`{"predicates":["test.link.to"],"operator":"XOR"}`, `{"predicates":["test.link.to"]}`,
			`{"predicates":["test.link.to","test.link"],"operator":"OR"}`},
		"byName": {`not json`, `{}`, `{"name":""}`},
	}
	if len(cases) != len(handlers) {
		t.Fatalf("cases cover %d verbs, graph-index has %d handlers", len(cases), len(handlers))
	}
	for verb, bodies := range cases {
		handler, ok := handlers[verb]
		if !ok {
			t.Fatalf("no handler for %s", verb)
		}
		for _, body := range bodies {
			reply, err := handler(context.Background(), []byte(body))
			var classified *errs.ClassifiedError
			if !errors.As(err, &classified) || classified.Class != errs.ErrorInvalid ||
				classified.Code != graph.ErrorCodeInvalidRequest {
				t.Errorf("%s %s: reply %q err %v, want class invalid, code %s", verb, body, reply, err,
					graph.ErrorCodeInvalidRequest)
			}
		}
	}
}

// TestRankNameMatches (pin name_index_test.go:24-65): exact case ranks first, then lower
// priority, then entity ID; one match per entity, its best; the limit applies after ranking.
func TestRankNameMatches(t *testing.T) {
	got := rankNameMatches([]graph.NameIndexItem{
		{EntityID: "test.graph.index.name.entity.pref", Name: "foo", Predicate: "skos.core.pref-label", Priority: 0},
		{EntityID: "test.graph.index.name.entity.exact", Name: "Foo", Predicate: "dc.terms.title", Priority: 1},
		{EntityID: "test.graph.index.name.entity.upper", Name: "FOO", Predicate: "dc.terms.title", Priority: 1},
	}, "Foo", 0)
	want := []graph.NameMatch{
		{EntityID: "test.graph.index.name.entity.exact", MatchedName: "Foo", Predicate: "dc.terms.title", ExactCase: true},
		{EntityID: "test.graph.index.name.entity.pref", MatchedName: "foo", Predicate: "skos.core.pref-label"},
		{EntityID: "test.graph.index.name.entity.upper", MatchedName: "FOO", Predicate: "dc.terms.title"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("ranking:\n got %+v\nwant %+v", got, want)
	}

	got = rankNameMatches([]graph.NameIndexItem{
		{EntityID: "test.graph.index.name.entity.e1", Name: "FOO", Predicate: "dc.terms.title", Priority: 1},
		{EntityID: "test.graph.index.name.entity.e1", Name: "Foo", Predicate: "skos.core.pref-label", Priority: 0},
	}, "Foo", 0)
	if len(got) != 1 || got[0].Predicate != "skos.core.pref-label" || !got[0].ExactCase {
		t.Errorf("one entity under two predicates = %+v, want its exact-case pref-label match only", got)
	}

	got = rankNameMatches([]graph.NameIndexItem{
		{EntityID: "test.graph.index.name.entity.e3", Name: "x", Priority: 1},
		{EntityID: "test.graph.index.name.entity.e1", Name: "x", Priority: 1},
		{EntityID: "test.graph.index.name.entity.e2", Name: "x", Priority: 1},
	}, "x", 2)
	if len(got) != 2 || got[0].EntityID != "test.graph.index.name.entity.e1" || got[1].EntityID != "test.graph.index.name.entity.e2" {
		t.Errorf("limit 2 = %+v, want e1 then e2", got)
	}
}

// TestRankNameMatchesExtremePriorities: priorities are plain ints with no narrower range
// (vocabulary.WithAlias), so the lowest and highest still rank lowest first, whichever order
// they arrive in (#429: a subtracting comparator overflows here and reverses them).
func TestRankNameMatchesExtremePriorities(t *testing.T) {
	high, low := "acme.ops.test.system.widget.002", "acme.ops.test.system.widget.001"
	for _, items := range [][]graph.NameIndexItem{
		{{EntityID: high, Name: "same", Priority: math.MinInt}, {EntityID: low, Name: "same", Priority: math.MaxInt}},
		{{EntityID: low, Name: "same", Priority: math.MaxInt}, {EntityID: high, Name: "same", Priority: math.MinInt}},
	} {
		got := rankNameMatches(items, "same", 0)
		if len(got) != 2 || got[0].EntityID != high || got[1].EntityID != low {
			t.Errorf("ranking %+v = %+v, want the MinInt entity %s before the MaxInt entity %s", items, got, high, low)
		}
	}
}

// TestReverseReadsSkipMalformedRows: a stored INCOMING_INDEX or NAME_INDEX tail that is not
// entity6.hex(predicate) with a canonical predicate, and a PREDICATE_INDEX key that is not
// predicate3.entity6, is never read as a row (the pin's
// TestPublicReverseQueriesSkipNoncanonicalPredicateRows and
// TestPredicateQueriesRejectStoredIdentityAndNamespaceAmbiguity). Expected keys are spelled from
// the layouts, not built by the package.
func TestReverseReadsSkipMalformedRows(t *testing.T) {
	source := "acme.ops.test.system.widget.001"
	hexOf := func(s string) string { return hex.EncodeToString([]byte(s)) }

	entity, predicate, ok := entityAndPredicate(source + "." + hexOf("test.link.to"))
	if !ok || entity != source || predicate != "test.link.to" {
		t.Errorf("canonical tail = %q %q %v, want %s test.link.to", entity, predicate, ok, source)
	}
	for _, tail := range []string{
		source + "." + hexOf("test.link.to.far"), // four-part predicate
		source + "." + hexOf("test.link"),        // two-part predicate
		source + ".not-hex",
		source,                                          // no predicate token
		"acme.ops.test.system." + hexOf("test.link.to"), // five-part entity
		source + "." + hexOf("test.link.to") + ".extra",
	} {
		if e, p, ok := entityAndPredicate(tail); ok {
			t.Errorf("tail %q read as %q %q", tail, e, p)
		}
	}
	if _, ok := incomingEntryFromKey("acme.ops.test.system.widget.010."+source+"."+hexOf("test.link.to"),
		"acme.ops.test.system.widget.011."); ok {
		t.Error("a key under another target's prefix was read as an incoming row")
	}

	summaries, malformed := predicateSummaries([]string{
		"test.link.to." + source,
		"test.link.to." + source, // a listing may repeat a key
		"test.link.to.acme.ops.test.system.widget.002",
		"test.state.armed." + source,
		"test.link.to.far." + source,               // ten tokens
		"test.link.to.acme.ops.test.system.widget", // eight tokens
	})
	want := []graph.PredicateSummary{{Predicate: "test.link.to", EntityCount: 2}, {Predicate: "test.state.armed", EntityCount: 1}}
	if !slices.Equal(summaries, want) {
		t.Errorf("predicate summaries = %+v, want %+v", summaries, want)
	}
	wantMalformed := []string{"test.link.to.far." + source, "test.link.to.acme.ops.test.system.widget"}
	if !slices.Equal(malformed, wantMalformed) {
		t.Errorf("malformed keys = %q, want %q", malformed, wantMalformed)
	}
}
