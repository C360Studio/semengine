package graph

import (
	"encoding/json"
	"testing"
)

// TestGlobalSearchTypesWire proves the wire spelling of the globalSearch request and reply: the request
// is query, level, limit and min_revision, and a reply always holds text and hints, and an entity or
// a walk only beside a hint that was honoured, the entity's bytes unchanged. A hint carries a reason
// and a code only when it was not honoured. A text group carries its record only when it was read.
func TestGlobalSearchTypesWire(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  string
	}{
		{"request", GlobalSearchRequest{Query: "sensor-001", MinRevisionField: MinRevisionField{MinRevision: 7}}, `{"query":"sensor-001","min_revision":7}`},
		{"request without revision", GlobalSearchRequest{Query: "sensor-001"}, `{"query":"sensor-001"}`},
		{"request with level and limit", GlobalSearchRequest{Query: "pump", Level: 1, Limit: 25}, `{"query":"pump","level":1,"limit":25}`},
		{
			"text with a verified entity",
			GlobalSearchData{
				Text:   &TextSearchData{Groups: []TextSearchGroup{}, Matches: []TextSearchMatch{}},
				Entity: json.RawMessage(`{"id":"a.b.c.d.e.f","triples":[]}`),
				Hints:  []GlobalSearchHint{{Hint: "entity", Detail: "pump-7", Honoured: true}},
			},
			`{"text":{"level":0,"groups":[],"matches":[],"matched":0,"missing":0,"bodies_not_indexed":0,"truncated":false,"unreadable_groups":0,"answer":""},` +
				`"entity":{"id":"a.b.c.d.e.f","triples":[]},"hints":[{"hint":"entity","detail":"pump-7","honoured":true}]}`,
		},
		{
			"text with a walk and hints not honoured",
			GlobalSearchData{
				Text: &TextSearchData{Groups: []TextSearchGroup{}, Matches: []TextSearchMatch{}},
				Path: &PathSearchData{Entities: []PathEntity{{EntityID: "a.b.c.d.e.f"}}, Edges: []RelationshipEdge{}},
				Hints: []GlobalSearchHint{
					{Hint: "path", Detail: "pump-7", Honoured: true},
					{Hint: "aggregation", Detail: "count", Reason: "global search computes no aggregation"},
					{Hint: "entity", Detail: "pump-99", Reason: "not found: pump-99", Code: "entity_not_found"},
				},
			},
			`{"text":{"level":0,"groups":[],"matches":[],"matched":0,"missing":0,"bodies_not_indexed":0,"truncated":false,"unreadable_groups":0,"answer":""},` +
				`"path":{"entities":[{"entity_id":"a.b.c.d.e.f","depth":0}],"edges":[],"truncated":false},"hints":[` +
				`{"hint":"path","detail":"pump-7","honoured":true},` +
				`{"hint":"aggregation","detail":"count","honoured":false,"reason":"global search computes no aggregation"},` +
				`{"hint":"entity","detail":"pump-99","honoured":false,"reason":"not found: pump-99","code":"entity_not_found"}]}`,
		},
		{
			"text with no hint",
			GlobalSearchData{Text: &TextSearchData{Groups: []TextSearchGroup{}, Matches: []TextSearchMatch{}}, Hints: []GlobalSearchHint{}},
			`{"text":{"level":0,"groups":[],"matches":[],"matched":0,"missing":0,"bodies_not_indexed":0,"truncated":false,"unreadable_groups":0,"answer":""},"hints":[]}`,
		},
		{
			"text",
			GlobalSearchData{Text: &TextSearchData{
				Level: 1,
				Groups: []TextSearchGroup{
					{CommunityID: "a.b.c", Matches: 1, RecordState: RecordRead, Record: &LocalSearchRecord{MemberCount: 2, Keywords: []string{"pump"}}},
					{CommunityID: "a.b.x", Matches: 1, RecordState: RecordUnreadable},
				},
				Matches: []TextSearchMatch{{ID: "a.b.c.d.e.f", Label: "Pump 7", Score: 0.5, Entity: json.RawMessage(`{"id":"a.b.c.d.e.f"}`)}, {ID: "a.b.x.d.e.g", Score: 0.5, Entity: json.RawMessage(`{}`)}},
				Matched: 40, Excluded: []string{"test"}, Missing: 1, BodiesNotIndexed: 3, Truncated: true,
				UnreadableGroups: 1, Answer: "Found 40.",
			}, Hints: []GlobalSearchHint{}},
			`{"text":{"level":1,"groups":[` +
				`{"community_id":"a.b.c","matches":1,"record_state":"read","record":{"member_count":2,"keywords":["pump"]}},` +
				`{"community_id":"a.b.x","matches":1,"record_state":"unreadable"}],` +
				`"matches":[{"id":"a.b.c.d.e.f","label":"Pump 7","score":0.5,"entity":{"id":"a.b.c.d.e.f"}},{"id":"a.b.x.d.e.g","score":0.5,"entity":{}}],` +
				`"matched":40,"excluded":["test"],"missing":1,"bodies_not_indexed":3,"truncated":true,` +
				`"unreadable_groups":1,"answer":"Found 40."},"hints":[]}`,
		},
		{
			"text with nothing excluded",
			TextSearchData{Groups: []TextSearchGroup{}, Matches: []TextSearchMatch{}},
			`{"level":0,"groups":[],"matches":[],"matched":0,"missing":0,"bodies_not_indexed":0,"truncated":false,` +
				`"unreadable_groups":0,"answer":""}`,
		},
	}
	for _, tc := range cases {
		got, err := json.Marshal(tc.value)
		if err != nil {
			t.Fatalf("%s: marshal: %v", tc.name, err)
		}
		if string(got) != tc.want {
			t.Errorf("%s = %s, want %s", tc.name, got, tc.want)
		}
	}
}
