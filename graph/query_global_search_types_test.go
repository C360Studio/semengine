package graph

import (
	"encoding/json"
	"testing"
)

// TestGlobalSearchTypesWire proves the wire spelling of the globalSearch request and reply (design D4,
// D5): the request is query and min_revision, and a reply holds strategy and exactly the one result it
// names, the entity's bytes unchanged.
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
