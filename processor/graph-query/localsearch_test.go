package graphquery

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/c360studio/semengine/graph"
	semtypes "github.com/c360studio/semengine/pkg/types"
)

func TestLocalSearchRequestDecoding(t *testing.T) {
	request, group, err := decodeLocalSearchRequest([]byte(`{"entity_id":"acme.ops.src.docs.note.n00","query":"Alpha","level":2,"limit":100,"min_revision":7}`))
	want := graph.LocalSearchRequest{EntityID: "acme.ops.src.docs.note.n00", Query: "Alpha", Level: 2, Limit: 100, MinRevisionField: graph.MinRevisionField{MinRevision: 7}}
	if err != nil || request != want || group != (localGroup{key: "2.acme.ops", id: "acme.ops"}) {
		t.Errorf("decode = %+v, %+v, %v; want %+v, the group acme.ops under 2.acme.ops", request, group, err, want)
	}
	// The group at each level is the ID's first four, three or two parts, the record's key the level
	// and a dot before it.
	for level, id := range []string{"acme.ops.src.docs", "acme.ops.src", "acme.ops"} {
		raw, err := json.Marshal(graph.LocalSearchRequest{EntityID: "acme.ops.src.docs.note.n00", Query: "a", Level: level})
		if err != nil {
			t.Fatal(err)
		}
		_, group, err := decodeLocalSearchRequest(raw)
		if wantKey := string(rune('0'+level)) + "." + id; err != nil || group.id != id || group.key != wantKey {
			t.Errorf("level %d: group = %+v, %v; want %s under %s", level, group, err, id, wantKey)
		}
	}
	// What the query holds is the word index's to judge: a reserved form and many words pass here.
	for _, query := range []string{"pump*", "name:retry", strings.Repeat("w ", 100)} {
		raw, err := json.Marshal(graph.LocalSearchRequest{EntityID: "acme.ops.src.docs.note.n00", Query: query})
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := decodeLocalSearchRequest(raw); err != nil {
			t.Errorf("query %q refused before the word index: %v", query, err)
		}
	}

	for request, field := range map[string]string{
		`{"entity_id":"acme.ops.src.docs.note.n00","query":"a","extra":3}`: "extra",
		`{"entity_id":"acme.ops.src.docs.note","query":"a"}`:               "entity_id",
		`{"query":"a"}`: "entity_id",
		`{"entity_id":"acme.ops.src.docs.note.n00","query":"a","level":3}`:   "level",
		`{"entity_id":"acme.ops.src.docs.note.n00","query":"a","level":-1}`:  "level",
		`{"entity_id":"acme.ops.src.docs.note.n00","query":" \t"}`:           "query",
		`{"entity_id":"acme.ops.src.docs.note.n00"}`:                         "query",
		`{"entity_id":"acme.ops.src.docs.note.n00","query":"a","limit":101}`: "limit",
		`{"entity_id":"acme.ops.src.docs.note.n00","query":"a","limit":-1}`:  "limit",
		`{"entity_id":"acme.ops.src.docs.note.n00","query":"a"} {}`:          "after",
		`not json`: "invalid",
	} {
		if _, _, err := decodeLocalSearchRequest([]byte(request)); err == nil || !strings.Contains(err.Error(), field) {
			t.Errorf("decodeLocalSearchRequest(%s) = %v, want an error naming %q", request, err, field)
		}
	}
}

func FuzzDecodeLocalSearchRequest(f *testing.F) {
	for _, seed := range []string{
		`{"entity_id":"acme.ops.src.docs.note.n00","query":"Alpha beta ALPHA","level":1,"min_revision":7}`,
		`{"entity_id":"acme.ops.src.docs.note.n00","query":"pump*","limit":1}`,
		`{"entity_id":"acme.ops.src.docs.note.n00","query":"a","limit":100}`,
		`{"entity_id":"acme.ops.src.docs.note.n00","query":"a","limit":101}`,
		`{"entity_id":"acme.ops.src.docs.note.n00","query":"İSTANBUL straße","level":2}`,
		`{"entity_id":"acme.ops.src.docs.note.n00","query":" "}`,
		`{"entity_id":"acme.ops.src.docs.note","query":"a"}`,
		`{"entity_id":"acme.ops.src.docs.note.n00","query":"a","level":3}`,
		`{"entity_id":"acme.ops.src.docs.note.n00","query":"a","extra":1}`,
		`{"entity_id":"acme.ops.src.docs.note.n00","query":"a"}x`,
		``,
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		request, group, err := decodeLocalSearchRequest(data)
		if err != nil {
			return
		}
		if err := semtypes.ValidateEntityID(request.EntityID); err != nil {
			t.Errorf("accepted entity_id %q: %v", request.EntityID, err)
		}
		if request.Level < 0 || request.Level > 2 {
			t.Errorf("accepted level %d", request.Level)
		}
		if request.Limit < 0 || request.Limit > 100 {
			t.Errorf("accepted limit %d", request.Limit)
		}
		if strings.TrimSpace(request.Query) == "" {
			t.Errorf("accepted the blank query %q", request.Query)
		}
		// The scope is a whole-part prefix of the entity's ID, with 4 - level parts, and the key names it.
		if err := semtypes.ValidateEntityIDPrefix(group.id); err != nil || !strings.HasPrefix(request.EntityID, group.id+".") ||
			strings.Count(group.id, ".") != 3-request.Level || group.key != string(rune('0'+request.Level))+"."+group.id {
			t.Errorf("entity %q at level %d gave the group %+v", request.EntityID, request.Level, group)
		}
	})
}

// TestSearchAnswer checks the local search answer template against literal texts.
func TestSearchAnswer(t *testing.T) {
	const summary = "Community of 7000 entities including 7000 notes. Key themes: alpha, beta."
	match := graph.TextSearchMatch{ID: "acme.ops.src.docs.note.a", Score: 2, Entity: json.RawMessage(`{}`)}
	data := graph.LocalSearchData{
		CommunityID: "acme.ops.src.docs",
		Record:      &graph.LocalSearchRecord{MemberCount: 7000, CommunitySummary: summary},
		RecordState: graph.RecordRead,
		Matches:     make([]graph.TextSearchMatch, 10),
		Matched:     40,
		Truncated:   true,
	}
	for i := range data.Matches {
		data.Matches[i] = match
	}
	if got, want := localSearchAnswer(data), "Found 40 matching entities in acme.ops.src.docs. Showing the first 10. "+summary; got != want {
		t.Errorf("answer = %q\nwant     %q", got, want)
	}
	data.Record, data.RecordState = nil, graph.RecordNotReady
	if got, want := localSearchAnswer(data), "Found 40 matching entities in acme.ops.src.docs. Showing the first 10."; got != want {
		t.Errorf("answer without a record = %q, want %q", got, want)
	}
	data.Matches, data.Matched, data.Truncated = data.Matches[:3], 3, false
	if got, want := localSearchAnswer(data), "Found 3 matching entities in acme.ops.src.docs."; got != want {
		t.Errorf("a whole answer = %q, want %q", got, want)
	}
	data.Record, data.RecordState = &graph.LocalSearchRecord{MemberCount: 3}, graph.RecordRead
	if got, want := localSearchAnswer(data), "Found 3 matching entities in acme.ops.src.docs."; got != want {
		t.Errorf("a record with no summary = %q, want %q", got, want)
	}
}

func TestLocalSearchFitKeepsTheBestThatFit(t *testing.T) {
	entity := func(id string) graph.TextSearchMatch {
		return graph.TextSearchMatch{ID: id, Score: 1, Entity: json.RawMessage(`{"id":"` + id + `"}`)}
	}
	data := graph.LocalSearchData{
		CommunityID: "acme.ops.src.docs",
		RecordState: graph.RecordNone,
		Matches:     []graph.TextSearchMatch{entity("a"), entity("b"), entity("c")},
		Matched:     3,
	}
	encode := func(d graph.LocalSearchData) ([]byte, error) { return json.Marshal(d) }
	const (
		head = `{"community_id":"acme.ops.src.docs","level":0,"record_state":"none","matches":[`
		a    = `{"id":"a","score":1,"entity":{"id":"a"}}`
		b    = `{"id":"b","score":1,"entity":{"id":"b"}}`
		c    = `{"id":"c","score":1,"entity":{"id":"c"}}`
		all  = head + a + `,` + b + `,` + c + `],"matched":3,"missing":0,"bodies_not_indexed":0,"truncated":false,"answer":"Found 3 matching entities in acme.ops.src.docs."}`
		two  = head + a + `,` + b + `],"matched":3,"missing":0,"bodies_not_indexed":0,"truncated":true,"answer":"Found 3 matching entities in acme.ops.src.docs. Showing the first 2."}`
		one  = head + a + `],"matched":3,"missing":0,"bodies_not_indexed":0,"truncated":true,"answer":"Found 3 matching entities in acme.ops.src.docs. Showing the first 1."}`
	)
	for limit, want := range map[int]string{len(all): all, len(two): two, len(two) - 1: one} {
		got, truncated, err := fitLocalSearch(data, limit, encode)
		if err != nil || string(got) != want || truncated != (limit < len(all)) {
			t.Errorf("fit at %d = %s, %t, %v\nwant %s, %t", limit, got, truncated, err, want, limit < len(all))
		}
	}
	data.Truncated = true // the word index matched more than it returned
	if got, truncated, err := fitLocalSearch(data, len(all)+100, encode); err != nil || !truncated || !strings.Contains(string(got), `"truncated":true`) {
		t.Errorf("fit of a limited answer = %s, %t, %v; want it still truncated", got, truncated, err)
	}
	if got, _, err := fitLocalSearch(data, 10, encode); err == nil {
		t.Errorf("fit at 10 bytes = %s, want an error", got)
	}
}
