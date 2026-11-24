package fusion_test

import (
	"encoding/json"
	"maps"
	"slices"
	"testing"

	"github.com/c360studio/semengine/pkg/fusion"
)

// Tests that a natural-language answer carries the word index reply's
// bodies_not_indexed and excluded; a symbol or prefix answer carries neither.
// refLens reads a query with a space as natural language and a bare token as a
// symbol.

const nlQuery = "on event handler"

// searchCountKeys fuses req over g and returns the response with its top-level
// JSON keys.
func searchCountKeys(t *testing.T, g *fakeGraph, query string) (fusion.Response, map[string]json.RawMessage) {
	t.Helper()
	eng := mustEngine(t, g, nil)
	resp, err := eng.Fuse(t.Context(), fusion.Request{Query: query, Want: []fusion.Want{fusion.WantBody}}, refLens{})
	if err != nil {
		t.Fatalf("Fuse: %v", err)
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return resp, keys
}

// TestNLAnswerCarriesWordIndexCounts: an NL resolve reporting 2 bodies not
// indexed and the excluded word race gives an answer carrying both beside its
// nodes, and a miss carrying both; with neither reported, neither key appears.
// The same counts ride an all-seeds-unhydrated deferral and an index_not_ready
// deferral from the batch read that followed the search.
func TestNLAnswerCarriesWordIndexCounts(t *testing.T) {
	seed := entity("acme.ops.code.repo.symbol.OnEvent", "OnEvent", "on_event.go")
	excluded := []string{"race"}

	t.Run("nodes", func(t *testing.T) {
		g := &fakeGraph{
			seeds:            map[string][]string{nlQuery: {seed.ID}},
			entities:         map[string]*fusion.Entity{seed.ID: seed},
			bodiesNotIndexed: 2,
			excluded:         excluded,
		}
		resp, keys := searchCountKeys(t, g, nlQuery)
		if len(resp.Nodes) != 1 || string(keys["bodies_not_indexed"]) != "2" || string(keys["excluded"]) != `["race"]` {
			t.Errorf("nodes=%d bodies_not_indexed=%s excluded=%s, want 1, 2 and [\"race\"]",
				len(resp.Nodes), keys["bodies_not_indexed"], keys["excluded"])
		}
	})

	t.Run("miss", func(t *testing.T) {
		g := &fakeGraph{names: []string{"OnEvent"}, excluded: excluded}
		resp, keys := searchCountKeys(t, g, nlQuery)
		if len(resp.Misses) != 1 || string(keys["excluded"]) != `["race"]` {
			t.Errorf("misses=%d excluded=%s, want 1 and [\"race\"]", len(resp.Misses), keys["excluded"])
		}
		if v, ok := keys["bodies_not_indexed"]; ok {
			t.Errorf("bodies_not_indexed = %s at 0, want no key", v)
		}
	})

	t.Run("nothing left out", func(t *testing.T) {
		g := &fakeGraph{seeds: map[string][]string{nlQuery: {seed.ID}}, entities: map[string]*fusion.Entity{seed.ID: seed}}
		_, keys := searchCountKeys(t, g, nlQuery)
		for _, k := range []string{"bodies_not_indexed", "excluded"} {
			if v, ok := keys[k]; ok {
				t.Errorf("%s = %s when the reply reported none, want no key", k, v)
			}
		}
	})

	t.Run("deferrals after the search answered", func(t *testing.T) {
		for name, g := range map[string]*fakeGraph{
			"all seeds unhydrated": {seeds: map[string][]string{nlQuery: {seed.ID}}, bodiesNotIndexed: 2, excluded: excluded},
			"batch not ready":      {seeds: map[string][]string{nlQuery: {seed.ID}}, entErr: indexNotReady(), bodiesNotIndexed: 2, excluded: excluded},
		} {
			t.Run(name, func(t *testing.T) {
				resp, _ := searchCountKeys(t, g, nlQuery)
				if resp.Provenance != fusion.ProvenanceLexical {
					t.Errorf("Provenance = %q, want %q", resp.Provenance, fusion.ProvenanceLexical)
				}
				if !resp.Deferred || resp.BodiesNotIndexed != 2 || !slices.Equal(resp.Excluded, excluded) {
					t.Errorf("Deferred=%v bodies_not_indexed=%d excluded=%v, want true, 2, [race]",
						resp.Deferred, resp.BodiesNotIndexed, resp.Excluded)
				}
			})
		}
	})
}

// TestSymbolAnswerOmitsCounts: after a symbol resolve the response has neither
// key, even when the client reports counts.
func TestSymbolAnswerOmitsCounts(t *testing.T) {
	seed := entity("acme.ops.code.repo.symbol.OnEvent", "OnEvent", "on_event.go")
	excluded := []string{"race"}
	for name, g := range map[string]*fakeGraph{
		"answer": {seeds: map[string][]string{"OnEvent": {seed.ID}}, entities: map[string]*fusion.Entity{seed.ID: seed}, bodiesNotIndexed: 2, excluded: excluded},
		"miss":   {names: []string{"OnEvents"}, bodiesNotIndexed: 2, excluded: excluded},
	} {
		t.Run(name, func(t *testing.T) {
			_, keys := searchCountKeys(t, g, "OnEvent")
			for _, k := range []string{"bodies_not_indexed", "excluded"} {
				if v, ok := keys[k]; ok {
					t.Errorf("%s = %s after a symbol resolve, want no key", k, v)
				}
			}
		})
	}
}

// TestDeferralProvenanceFollowsMode: a deferral names the provenance of the
// resolve mode that was asked, whichever read withheld the answer, so a
// natural-language question is never reported as deterministic.
func TestDeferralProvenanceFollowsMode(t *testing.T) {
	seed := entity("acme.ops.code.repo.symbol.OnEvent", "OnEvent", "on_event.go")
	for name, tc := range map[string]struct {
		query string
		g     *fakeGraph
		want  []fusion.Want
		prov  fusion.Provenance
	}{
		"nl resolve not ready":     {nlQuery, &fakeGraph{resolveErr: indexNotReady()}, nil, fusion.ProvenanceLexical},
		"nl relations not ready":   {nlQuery, &fakeGraph{seeds: map[string][]string{nlQuery: {seed.ID}}, entities: map[string]*fusion.Entity{seed.ID: seed}, neighborsErr: indexNotReady()}, []fusion.Want{fusion.WantRelations}, fusion.ProvenanceLexical},
		"symbol resolve not ready": {"OnEvent", &fakeGraph{resolveErr: indexNotReady()}, nil, fusion.ProvenanceDeterministic},
		"symbol batch not ready":   {"OnEvent", &fakeGraph{seeds: map[string][]string{"OnEvent": {seed.ID}}, entErr: indexNotReady()}, nil, fusion.ProvenanceDeterministic},
	} {
		t.Run(name, func(t *testing.T) {
			resp, err := mustEngine(t, tc.g, nil).Fuse(t.Context(), fusion.Request{Query: tc.query, Want: tc.want}, refLens{})
			if err != nil {
				t.Fatalf("Fuse: %v", err)
			}
			if !resp.Deferred || resp.Provenance != tc.prov {
				t.Errorf("Deferred=%v Provenance=%q, want true, %q", resp.Deferred, resp.Provenance, tc.prov)
			}
		})
	}
}

// TestResolutionProvenanceWins: a response carries the provenance its resolve
// reported, whatever the mode would say, on an answer, a miss and a deferral
// from a read after the resolve; a resolve that reports none leaves the mode's.
func TestResolutionProvenanceWins(t *testing.T) {
	seed := entity("acme.ops.code.repo.symbol.OnEvent", "OnEvent", "on_event.go")
	found := func(query string, prov fusion.Provenance) *fakeGraph {
		return &fakeGraph{
			seeds: map[string][]string{query: {seed.ID}}, entities: map[string]*fusion.Entity{seed.ID: seed},
			provenance: prov,
		}
	}
	notReady := found(nlQuery, fusion.ProvenanceEmbedding)
	notReady.neighborsErr = indexNotReady()
	for name, tc := range map[string]struct {
		query string
		g     *fakeGraph
		want  []fusion.Want
		shape string
		prov  fusion.Provenance
	}{
		"nl answer reports embedding":   {nlQuery, found(nlQuery, fusion.ProvenanceEmbedding), nil, "nodes", fusion.ProvenanceEmbedding},
		"nl miss reports embedding":     {nlQuery, &fakeGraph{provenance: fusion.ProvenanceEmbedding}, nil, "miss", fusion.ProvenanceEmbedding},
		"nl deferral reports embedding": {nlQuery, notReady, []fusion.Want{fusion.WantRelations}, "deferred", fusion.ProvenanceEmbedding},
		"symbol answer reports lexical": {"OnEvent", found("OnEvent", fusion.ProvenanceLexical), nil, "nodes", fusion.ProvenanceLexical},
		"nl answer reports none":        {nlQuery, found(nlQuery, ""), nil, "nodes", fusion.ProvenanceLexical},
		"symbol answer reports none":    {"OnEvent", found("OnEvent", ""), nil, "nodes", fusion.ProvenanceDeterministic},
	} {
		t.Run(name, func(t *testing.T) {
			resp, err := mustEngine(t, tc.g, nil).Fuse(t.Context(), fusion.Request{Query: tc.query, Want: tc.want}, refLens{})
			if err != nil {
				t.Fatalf("Fuse: %v", err)
			}
			shape := "nodes"
			switch {
			case resp.Deferred:
				shape = "deferred"
			case len(resp.Misses) > 0:
				shape = "miss"
			}
			if shape != tc.shape || resp.Provenance != tc.prov {
				t.Errorf("a %s with provenance %q, want a %s with %q", shape, resp.Provenance, tc.shape, tc.prov)
			}
		})
	}
}

// mergeKey decodes the response's merge key, and reports whether it was there.
func mergeKey(t *testing.T, keys map[string]json.RawMessage) (map[string]any, bool) {
	t.Helper()
	raw, ok := keys["merge"]
	if !ok {
		return nil, false
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("merge %s: %v", raw, err)
	}
	return got, true
}

// TestNLAnswerCarriesMerge: an NL resolve reporting a merge answer puts it,
// as the merge key, on a response with nodes, on a miss, and on both kinds of
// deferral from the batch read that followed the search. A merge not honoured
// carries its detail, reason and code; an honoured one leaves out the empty
// reason and code. A symbol resolve's response, a failed NL resolve's deferral,
// and an NL resolve that reports no merge answer have no merge key.
func TestNLAnswerCarriesMerge(t *testing.T) {
	seed := entity("acme.ops.code.repo.symbol.OnEvent", "OnEvent", "on_event.go")
	failed := &fusion.Merge{Detail: "merged", Reason: "the embedding search failed", Code: "internal"}
	wantFailed := map[string]any{
		"honoured": false, "detail": "merged", "reason": "the embedding search failed", "code": "internal",
	}
	found := func(query string, merge *fusion.Merge) *fakeGraph {
		return &fakeGraph{
			seeds: map[string][]string{query: {seed.ID}}, entities: map[string]*fusion.Entity{seed.ID: seed},
			merge: merge,
		}
	}

	for name, tc := range map[string]struct {
		g     *fakeGraph
		shape string
		want  map[string]any
	}{
		"nodes, not honoured": {found(nlQuery, failed), "nodes", wantFailed},
		"nodes, honoured": {
			found(nlQuery, &fusion.Merge{Honoured: true, Detail: "merged"}), "nodes",
			map[string]any{"honoured": true, "detail": "merged"},
		},
		"miss":                 {&fakeGraph{names: []string{"OnEvent"}, merge: failed}, "miss", wantFailed},
		"all seeds unhydrated": {&fakeGraph{seeds: map[string][]string{nlQuery: {seed.ID}}, merge: failed}, "deferred", wantFailed},
		"batch not ready": {
			&fakeGraph{seeds: map[string][]string{nlQuery: {seed.ID}}, entErr: indexNotReady(), merge: failed}, "deferred", wantFailed,
		},
	} {
		t.Run(name, func(t *testing.T) {
			resp, keys := searchCountKeys(t, tc.g, nlQuery)
			shape := "nodes"
			switch {
			case resp.Deferred:
				shape = "deferred"
			case len(resp.Misses) > 0:
				shape = "miss"
			case len(resp.Nodes) == 0:
				shape = "empty"
			}
			if shape != tc.shape {
				t.Fatalf("a %s, want a %s", shape, tc.shape)
			}
			got, ok := mergeKey(t, keys)
			if !ok {
				t.Fatalf("no merge key, want %v", tc.want)
			}
			if !maps.Equal(got, tc.want) {
				t.Errorf("merge = %v, want %v", got, tc.want)
			}
		})
	}

	failedResolve := &fakeGraph{resolveErr: indexNotReady(), merge: failed}
	for name, tc := range map[string]struct {
		query string
		g     *fakeGraph
		nodes int
	}{
		"nl resolve with no merge": {nlQuery, found(nlQuery, nil), 1},
		"symbol resolve":           {"OnEvent", found("OnEvent", failed), 1},
		"failed nl resolve":        {nlQuery, failedResolve, 0},
	} {
		t.Run(name, func(t *testing.T) {
			resp, keys := searchCountKeys(t, tc.g, tc.query)
			if len(resp.Nodes) != tc.nodes {
				t.Fatalf("nodes = %d, want %d", len(resp.Nodes), tc.nodes)
			}
			if got, ok := mergeKey(t, keys); ok {
				t.Errorf("merge = %v, want no key", got)
			}
		})
	}
}
