//go:build integration

package graphgateway

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/c360studio/semengine/graph"
)

// TestAnswerCarriesDamagedCount: a root field over an enveloped reply that carries
// damaged_entities answers with top-level extensions.damagedEntities, the reply's count, and
// data holding exactly the selected fields; a reply at 0, a bare reply, and a reply the gateway
// refuses carry no such key.
func TestAnswerCarriesDamagedCount(t *testing.T) {
	client := fixtureClient(t)
	g := startGateway(t, client, `{}`)
	envelope := func(data string, damaged int) []byte {
		return fmt.Appendf(nil, `{"data":%s,"indexed_revision":1,"damaged_entities":%d,"producer":"graph-query",`+
			`"timestamp":"2026-11-18T00:00:00Z"}`, data, damaged)
	}
	start := fmt.Sprintf(`{"entities":[{"entity_id":%q,"depth":0}],"edges":[],"truncated":false}`, droneA)
	unaligned := fmt.Sprintf(`{"entities":[{"entity_id":%q,"depth":0},{"entity_id":%q,"depth":1}],"edges":[],`+
		`"truncated":false}`, droneA, droneB)
	// The case's name travels as the request's first predicate, so one stub answers every case.
	paths := map[string][]byte{
		"pathSearch at 1":      envelope(start, 1),
		"pathSearch at 0":      envelope(start, 0),
		"a refused pathSearch": envelope(unaligned, 2),
	}
	stub(t, client, verb(t, graph.ResponderGraphQuery, "pathSearch").Subject, func(_ context.Context, data []byte) ([]byte, error) {
		var request graph.PathSearchRequest
		if err := json.Unmarshal(data, &request); err != nil {
			return nil, err
		}
		return paths[request.Predicates[0]], nil
	})
	stub(t, client, verb(t, graph.ResponderGraphQuery, "predicate").Subject, func(context.Context, []byte) ([]byte, error) {
		return envelope(fmt.Sprintf(`{"entities":[%q]}`, droneA), 3), nil
	})
	stub(t, client, verb(t, graph.ResponderGraphQuery, "entity").Subject, func(context.Context, []byte) ([]byte, error) {
		return []byte(`{"entity":` + storedEntity + `,"kv_revision":3,"damaged_entities":4}`), nil
	})

	pathQuery := `query($p: [String]) { pathSearch(startEntity: "` + droneA + `", direction: "outgoing", maxDepth: 1,
		predicates: $p) { entities { id } } }`
	for _, tc := range []struct {
		name, query string
		vars        map[string]any
		data        string // the data the answer holds, or "" for a field refused null
		damaged     any    // extensions.damagedEntities as decoded, or nil for no key
	}{
		{"pathSearch at 1", pathQuery, map[string]any{"p": []any{"pathSearch at 1"}},
			fmt.Sprintf(`{"pathSearch":{"entities":[{"id":%q}]}}`, droneA), 1.0},
		{"entitiesByPredicate at 3", `{ entitiesByPredicate(predicate: "` + linkTo + `") { entities } }`, nil,
			fmt.Sprintf(`{"entitiesByPredicate":{"entities":[%q]}}`, droneA), 3.0},
		{"pathSearch at 0", pathQuery, map[string]any{"p": []any{"pathSearch at 0"}},
			fmt.Sprintf(`{"pathSearch":{"entities":[{"id":%q}]}}`, droneA), nil},
		{"a bare entity reply", `{ entity(id: "` + droneA + `") { kvRevision } }`, nil, `{"entity":{"kvRevision":3}}`, nil},
		{"a refused pathSearch", pathQuery, map[string]any{"p": []any{"a refused pathSearch"}}, "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := g.send(t, t.Context(), tc.query, tc.vars)
			got := decodeAnswer(t, rec)
			if tc.data == "" {
				requireNullField(t, got, 200, "pathSearch", "0 edges for 2 entities")
			} else {
				requireData(t, got, tc.data)
			}
			var top struct {
				Extensions map[string]any `json:"extensions"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &top); err != nil {
				t.Fatal(err)
			}
			value, present := top.Extensions["damagedEntities"]
			switch {
			case tc.damaged == nil && present:
				t.Errorf("extensions.damagedEntities = %v, want no key: %s", value, got.raw)
			case tc.damaged != nil && value != tc.damaged:
				t.Errorf("extensions.damagedEntities = %v (present %v), want %v: %s", value, present, tc.damaged, got.raw)
			}
			if tc.damaged == nil && top.Extensions != nil {
				t.Errorf("extensions = %v, want none: %s", top.Extensions, got.raw)
			}
		})
	}
}
