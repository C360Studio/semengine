package fusion_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/c360studio/semengine/pkg/fusion"
)

// TestContractVersionIsTwo: every response, answered or deferred, carries
// contract_version "2" and no index key, the wire change that removed the
// readiness envelope (ADR-109).
func TestContractVersionIsTwo(t *testing.T) {
	seed := entity("acme.ops.code.repo.symbol.OnEvent", "OnEvent", "on_event.go")
	cases := map[string]*fakeGraph{
		"answered": {
			seeds:    map[string][]string{"OnEvent": {seed.ID}},
			entities: map[string]*fusion.Entity{seed.ID: seed},
		},
		"miss":     {names: []string{"OnEvents"}},
		"deferred": {resolveErr: indexNotReady()},
	}
	for name, g := range cases {
		t.Run(name, func(t *testing.T) {
			eng := mustEngine(t, g, fusion.NewBodyResolver(fusion.MapStoreResolver{}))
			resp, err := eng.Fuse(t.Context(), fusion.Request{Query: "OnEvent"}, refLens{})
			if err != nil {
				t.Fatalf("Fuse: %v", err)
			}
			raw, err := json.Marshal(resp)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var keyed map[string]json.RawMessage
			if err := json.Unmarshal(raw, &keyed); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got := string(keyed["contract_version"]); got != `"2"` {
				t.Errorf("contract_version = %s, want \"2\":\n%s", got, raw)
			}
			if _, ok := keyed["index"]; ok {
				t.Errorf("response carries an index key:\n%s", raw)
			}
		})
	}
}

// TestFuse_AnsweredResponseIsNotMarkedDeferred: the Deferred flag must not become
// decorative by being set everywhere.
func TestFuse_AnsweredResponseIsNotMarkedDeferred(t *testing.T) {
	ent := entity("acme.ops.code.repo.symbol.OnEvent", "OnEvent", "on_event.go")
	g := &fakeGraph{
		seeds:    map[string][]string{"OnEvent": {ent.ID}},
		entities: map[string]*fusion.Entity{ent.ID: ent},
	}
	eng := mustEngine(t, g, fusion.NewBodyResolver(fusion.MapStoreResolver{}))
	resp, err := eng.Fuse(t.Context(), fusion.Request{Query: "OnEvent"}, refLens{})
	if err != nil {
		t.Fatalf("Fuse: %v", err)
	}
	if len(resp.Nodes) == 0 {
		t.Fatal("precondition: an answered response holds nodes")
	}
	if resp.Deferred || resp.DeferReason != "" {
		t.Errorf("Deferred=%v DeferReason=%q, want an answer that does not claim it deferred", resp.Deferred, resp.DeferReason)
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "deferred") {
		t.Errorf("the answered wire shape must carry no deferred key: %s", raw)
	}
}
