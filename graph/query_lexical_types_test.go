package graph

import (
	"encoding/json"
	"testing"
)

// TestLexicalSearchTypesWire holds the keys of the word search verb's request and reply: query,
// limit, scope and min_revision, the last two only when set, and results of entity_id and score,
// matched, excluded only when the query excludes a word, and bodies_not_indexed, with an empty
// result set as [] and both counts always sent. A
// strict decoder reads each request literal back into the same value.
func TestLexicalSearchTypesWire(t *testing.T) {
	requests := []struct {
		name  string
		value LexicalSearchRequest
		want  string
	}{
		{"every field", LexicalSearchRequest{Query: "pump failure", Limit: 5, Scope: []string{"acme.ops", ""},
			MinRevisionField: MinRevisionField{MinRevision: 7}},
			`{"query":"pump failure","limit":5,"scope":["acme.ops",""],"min_revision":7}`},
		{"unscoped", LexicalSearchRequest{Query: "pump"}, `{"query":"pump","limit":0}`},
	}
	for _, tc := range requests {
		got, err := json.Marshal(tc.value)
		if err != nil || string(got) != tc.want {
			t.Errorf("%s = %s (%v), want %s", tc.name, got, err, tc.want)
		}
		var back LexicalSearchRequest
		if err := decodeStrictly(tc.want, &back); err != nil {
			t.Fatalf("%s: strict decode of %s: %v", tc.name, tc.want, err)
		}
		if again, err := json.Marshal(back); err != nil || string(again) != tc.want {
			t.Errorf("%s: decoded and marshalled again = %s (%v), want %s", tc.name, again, err, tc.want)
		}
	}

	replies := []struct {
		name  string
		value LexicalSearchData
		want  string
	}{
		{"hits", LexicalSearchData{Results: []LexicalHit{{EntityID: "acme.ops.site.a.pump.001", Score: 1.5}},
			Matched: 3, BodiesNotIndexed: 2},
			`{"results":[{"entity_id":"acme.ops.site.a.pump.001","score":1.5}],"matched":3,"bodies_not_indexed":2}`},
		{"excluded", LexicalSearchData{Results: []LexicalHit{}, Excluded: []string{"race", "docker compose"}},
			`{"results":[],"matched":0,"excluded":["race","docker compose"],"bodies_not_indexed":0}`},
		{"none", LexicalSearchData{Results: []LexicalHit{}}, `{"results":[],"matched":0,"bodies_not_indexed":0}`},
	}
	for _, tc := range replies {
		if got, err := json.Marshal(tc.value); err != nil || string(got) != tc.want {
			t.Errorf("%s = %s (%v), want %s", tc.name, got, err, tc.want)
		}
	}
}
