package fusion_test

import (
	"encoding/json"
	"testing"

	"github.com/c360studio/semengine/pkg/fusion"
)

// Tests for design D3 (R1): a natural-language answer carries the search reply's
// unreadable_records and failed_records; a symbol or prefix answer carries neither.
// refLens reads a query with a space as natural language and a bare token as a
// symbol.

const nlQuery = "on event handler"

// searchCountKeys fuses req over g and returns the response with its top-level
// JSON keys.
func searchCountKeys(t *testing.T, g *fakeGraph, query string) (fusion.Response, map[string]json.RawMessage) {
	t.Helper()
	eng := fusion.NewEngine(g, nil)
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

// TestNLAnswerCarriesSearchCounts: an NL resolve with failed_records 1 gives an
// answer carrying failed_records 1 beside its nodes and no unreadable_records key;
// the same counts ride an all-seeds-unhydrated deferral and an index_not_ready
// deferral from the batch read that followed the search.
func TestNLAnswerCarriesSearchCounts(t *testing.T) {
	seed := entity("acme.ops.code.repo.symbol.OnEvent", "OnEvent", "on_event.go")

	t.Run("nodes", func(t *testing.T) {
		g := &fakeGraph{
			seeds:    map[string][]string{nlQuery: {seed.ID}},
			entities: map[string]*fusion.Entity{seed.ID: seed},
			failed:   1,
		}
		resp, keys := searchCountKeys(t, g, nlQuery)
		if len(resp.Nodes) != 1 || string(keys["failed_records"]) != "1" {
			t.Errorf("nodes=%d failed_records=%s, want 1 and 1", len(resp.Nodes), keys["failed_records"])
		}
		if _, ok := keys["unreadable_records"]; ok {
			t.Errorf("unreadable_records present at 0: %s", keys["unreadable_records"])
		}
	})

	t.Run("deferrals after the search answered", func(t *testing.T) {
		for name, g := range map[string]*fakeGraph{
			"all seeds unhydrated": {seeds: map[string][]string{nlQuery: {seed.ID}}, unreadable: 2, failed: 1},
			"batch not ready":      {seeds: map[string][]string{nlQuery: {seed.ID}}, entErr: indexNotReady(), unreadable: 2, failed: 1},
		} {
			t.Run(name, func(t *testing.T) {
				resp, _ := searchCountKeys(t, g, nlQuery)
				if !resp.Deferred || resp.UnreadableRecords != 2 || resp.FailedRecords != 1 {
					t.Errorf("Deferred=%v unreadable=%d failed=%d, want true, 2, 1",
						resp.Deferred, resp.UnreadableRecords, resp.FailedRecords)
				}
			})
		}
	})
}

// TestNLMissCarriesSearchCounts: an NL resolve that found no seeds while 3 records
// were unreadable gives a miss carrying unreadable_records 3.
func TestNLMissCarriesSearchCounts(t *testing.T) {
	g := &fakeGraph{names: []string{"OnEvent"}, unreadable: 3}
	resp, keys := searchCountKeys(t, g, nlQuery)
	if len(resp.Misses) != 1 || string(keys["unreadable_records"]) != "3" {
		t.Errorf("misses=%d unreadable_records=%s, want 1 and 3", len(resp.Misses), keys["unreadable_records"])
	}
}

// TestSymbolAnswerOmitsCounts: after a symbol resolve the response has neither
// key, even when the client reports counts.
func TestSymbolAnswerOmitsCounts(t *testing.T) {
	seed := entity("acme.ops.code.repo.symbol.OnEvent", "OnEvent", "on_event.go")
	for name, g := range map[string]*fakeGraph{
		"answer": {seeds: map[string][]string{"OnEvent": {seed.ID}}, entities: map[string]*fusion.Entity{seed.ID: seed}, unreadable: 3, failed: 1},
		"miss":   {names: []string{"OnEvents"}, unreadable: 3, failed: 1},
	} {
		t.Run(name, func(t *testing.T) {
			_, keys := searchCountKeys(t, g, "OnEvent")
			for _, k := range []string{"unreadable_records", "failed_records"} {
				if v, ok := keys[k]; ok {
					t.Errorf("%s = %s after a symbol resolve, want no key", k, v)
				}
			}
		})
	}
}
