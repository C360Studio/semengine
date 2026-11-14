package fusion_test

import (
	"encoding/json"
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
