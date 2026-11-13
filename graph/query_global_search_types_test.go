package graph

import (
	"encoding/json"
	"testing"
)

// TestGlobalSearchTypesWire proves the wire spelling of the globalSearch request and reply: the request
// is query and min_revision, and a reply holds strategy and exactly the one result it names, the
// entity's bytes unchanged.
func TestGlobalSearchTypesWire(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  string
	}{
		{"request", GlobalSearchRequest{Query: "sensor-001", MinRevisionField: MinRevisionField{MinRevision: 7}}, `{"query":"sensor-001","min_revision":7}`},
		{"request without revision", GlobalSearchRequest{Query: "sensor-001"}, `{"query":"sensor-001"}`},
		{
			"entity",
			GlobalSearchData{Strategy: "entity", Entity: json.RawMessage(`{"id":"a.b.c.d.e.f","triples":[]}`)},
			`{"strategy":"entity","entity":{"id":"a.b.c.d.e.f","triples":[]}}`,
		},
		{
			"path",
			GlobalSearchData{Strategy: "path", Path: &PathSearchData{Entities: []PathEntity{{EntityID: "a.b.c.d.e.f"}}, Edges: []RelationshipEdge{}}},
			`{"strategy":"path","path":{"entities":[{"entity_id":"a.b.c.d.e.f","depth":0}],"edges":[],"truncated":false}}`,
		},
		{
			"temporal",
			GlobalSearchData{Strategy: "temporal", Temporal: &TemporalData{Results: []TemporalHit{{EntityID: "a.b.c.d.e.f"}}, Truncated: true}},
			`{"strategy":"temporal","temporal":{"results":[{"entity_id":"a.b.c.d.e.f"}],"truncated":true}}`,
		},
		{
			"spatial",
			GlobalSearchData{Strategy: "spatial", Spatial: &SpatialData{Results: []SpatialHit{}}},
			`{"strategy":"spatial","spatial":{"results":[],"truncated":false}}`,
		},
		{
			"text",
			GlobalSearchData{Strategy: "text", Text: &TextSearchData{
				Level:           1,
				Groups:          []TextSearchGroup{{CommunityID: "a.b.c", MemberCount: 2, Keywords: []string{"pump"}}},
				Matches:         []TextSearchMatch{{ID: "a.b.c.d.e.f", Label: "Pump 7", Score: 0.5, Entity: json.RawMessage(`{"id":"a.b.c.d.e.f"}`)}, {ID: "a.b.c.d.e.g", Score: 0.5, Entity: json.RawMessage(`{}`)}},
				EntitiesScanned: 3, EntitiesTruncated: true, UnreadableGroups: 1, UnhonouredHints: []string{"path"}, Answer: "Found 2.",
			}},
			`{"strategy":"text","text":{"level":1,"groups":[{"community_id":"a.b.c","member_count":2,"keywords":["pump"]}],` +
				`"matches":[{"id":"a.b.c.d.e.f","label":"Pump 7","score":0.5,"entity":{"id":"a.b.c.d.e.f"}},{"id":"a.b.c.d.e.g","score":0.5,"entity":{}}],` +
				`"entities_scanned":3,"entities_truncated":true,"unreadable_groups":1,"unhonoured_hints":["path"],"answer":"Found 2."}}`,
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
