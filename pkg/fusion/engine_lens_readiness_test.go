package fusion_test

import (
	"context"
	"errors"
	"testing"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/fusion"
)

// indexNotReady builds the classified readiness transient a graph read emits when the
// serving index is not SOUND — its watcher is unavailable, it is degraded by an
// unresolved required write, or its initial build has not completed
// (graph.ErrorCodeIndexNotReady), mirroring processor/graph-index/query.go and
// the graph-query responders that preserve classified index errors.
//
// ADR-084 narrowed this: the transient no longer fires on ordinary catch-up lag, so the
// window in which Fuse's internal reads hit it is now a health window rather than a
// load window. The DEGRADE behavior under test is unchanged and matters more, not less
// — the transient is rarer now, which makes handling it inconsistently harder to catch
// in the wild.
func indexNotReady() error {
	return errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeIndexNotReady,
		errors.New("index not ready: initial build has not completed"))
}

// resetRequired builds the fatal refusal graph-ingest returns for a poisoned
// entity (processor/graph-ingest/query.go), as the reply reconstructs it.
func resetRequired() error {
	return errs.ClassifiedCode(errs.ErrorFatal, graph.ErrorCodeGraphStateResetRequired,
		errors.New("graph_state_reset_required: unreadable_entity_state"))
}

// TestDeferredOnIndexNotReady: Fuse asks no status (ADR-109 D1); an index_not_ready
// reply to Resolve, Entities or a relations Neighbors read defers the whole answer
// with defer_reason index_not_ready, no nodes and no miss, and no error. Ported
// from the pin's TestEngine_ReadinessTransient_Degrades without the status gate.
func TestDeferredOnIndexNotReady(t *testing.T) {
	seed := entity("acme.ops.code.repo.symbol.OnEvent", "OnEvent", "on_event.go")

	cases := map[string]struct {
		graph *fakeGraph
		req   fusion.Request
	}{
		"resolve": {
			graph: &fakeGraph{resolveErr: indexNotReady()},
			req:   fusion.Request{Query: "OnEvent"},
		},
		"entities": {
			graph: &fakeGraph{
				seeds:  map[string][]string{"OnEvent": {seed.ID}},
				entErr: indexNotReady(),
			},
			req: fusion.Request{Query: "OnEvent"},
		},
		"relations neighbors": {
			graph: &fakeGraph{
				seeds:        map[string][]string{"OnEvent": {seed.ID}},
				entities:     map[string]*fusion.Entity{seed.ID: seed},
				neighborsErr: indexNotReady(),
			},
			req: fusion.Request{Query: "OnEvent", Want: []fusion.Want{fusion.WantRelations}},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			eng := fusion.NewEngine(tc.graph, fusion.NewBodyResolver(fusion.MapStoreResolver{}))

			resp, err := eng.Fuse(t.Context(), tc.req, refLens{})
			if err != nil {
				t.Fatalf("index_not_ready must defer, not fail; got err %v", err)
			}
			if !resp.Deferred || resp.DeferReason != "index_not_ready" {
				t.Errorf("Deferred=%v DeferReason=%q, want true/index_not_ready", resp.Deferred, resp.DeferReason)
			}
			if len(resp.Nodes) != 0 || len(resp.Misses) != 0 {
				t.Errorf("a defer must hold no nodes and no misses, got %d nodes / %d misses",
					len(resp.Nodes), len(resp.Misses))
			}
			if resp.ContractVersion != "2" {
				t.Errorf("ContractVersion = %q, want 2", resp.ContractVersion)
			}
			if resp.Provenance != fusion.ProvenanceDeterministic {
				t.Errorf("Provenance = %q, want deterministic", resp.Provenance)
			}
		})
	}
}

// TestFatalReplyReturnsError: a fatal seed read fails the call. At the pin a
// reset deferred hard_stop through the status gate; with no gate, a
// graph_state_reset_required reply to Entities (one poisoned seed refuses the
// whole batch) or Resolve is Fuse's error, class and code intact.
func TestFatalReplyReturnsError(t *testing.T) {
	seed := "acme.ops.code.repo.symbol.OnEvent"
	cases := map[string]*fakeGraph{
		"resolve":  {resolveErr: resetRequired()},
		"entities": {seeds: map[string][]string{"OnEvent": {seed}}, entErr: resetRequired()},
	}
	for name, g := range cases {
		t.Run(name, func(t *testing.T) {
			eng := fusion.NewEngine(g, fusion.NewBodyResolver(fusion.MapStoreResolver{}))
			resp, err := eng.Fuse(t.Context(), fusion.Request{Query: "OnEvent"}, refLens{})
			if err == nil {
				t.Fatalf("a fatal seed read must fail the call, got response %+v", resp)
			}
			if !errs.IsFatal(err) || !graph.IsStateContractError(err) {
				t.Errorf("err = %v, want the fatal graph_state_reset_required reply", err)
			}
		})
	}
}

// TestFatalWalkReadSkipped: a fatal walk read is skipped like any other, because
// poison refuses only the poisoned entities (ADR-109 D1) and the reply cannot tell
// that refusal from a reset. The spec's scenario is a relation target; a
// relations Neighbors read and the paths, impact and graph walks are checked the
// same way. In the walks index_not_ready is skipped too (F23); in a relations
// Neighbors read it defers (TestDeferredOnIndexNotReady), so that case is fatal
// only.
func TestFatalWalkReadSkipped(t *testing.T) {
	seed := entity("acme.ops.code.repo.symbol.OnEvent", "OnEvent", "on_event.go")
	good := entity("acme.ops.code.repo.symbol.Good", "Good", "good.go")
	poisoned := "acme.ops.code.repo.symbol.Poisoned"

	t.Run("relation target", func(t *testing.T) {
		g := &fakeGraph{
			seeds:    map[string][]string{"OnEvent": {seed.ID}},
			entities: map[string]*fusion.Entity{seed.ID: seed, good.ID: good},
			out: map[string][]fusion.Edge{seed.ID: {
				{Predicate: "ref.relationship.references", Target: poisoned},
				{Predicate: "ref.relationship.references", Target: good.ID},
			}},
			entityErr: map[string]error{poisoned: resetRequired()},
		}
		eng := fusion.NewEngine(g, fusion.NewBodyResolver(fusion.MapStoreResolver{}))
		resp, err := eng.Fuse(t.Context(), fusion.Request{Query: "OnEvent", Want: []fusion.Want{fusion.WantRelations}}, refLens{})
		if err != nil {
			t.Fatalf("a fatal relation-target read must be skipped, got err %v", err)
		}
		if resp.Deferred || len(resp.Nodes) != 1 {
			t.Fatalf("want one answered node, got Deferred=%v nodes=%+v", resp.Deferred, resp.Nodes)
		}
		node := resp.Nodes[0]
		refs := node.Relations["referent"]
		if len(refs) != 1 || refs[0].Name != "Good" {
			t.Errorf("refs = %+v, want only Good (the poisoned target left out)", refs)
		}
		if !node.RelationsTruncated {
			t.Error("relations_truncated must be set when a target read was skipped")
		}
	})

	walks := map[string]struct {
		want      fusion.Want
		fatalOnly bool
		check     func(t *testing.T, resp fusion.Response)
	}{
		"relations neighbors": {want: fusion.WantRelations, fatalOnly: true, check: func(t *testing.T, resp fusion.Response) {
			if !resp.Nodes[0].RelationsTruncated {
				t.Error("a skipped relations Neighbors read must set relations_truncated")
			}
		}},
		"paths": {want: fusion.WantPaths, check: func(t *testing.T, resp fusion.Response) {
			if len(resp.Paths) != 0 || !resp.PathsTruncated {
				t.Errorf("paths = %+v, paths_truncated=%v; want none past a skipped first read, labelled",
					resp.Paths, resp.PathsTruncated)
			}
		}},
		"impact": {want: fusion.WantImpact, check: func(t *testing.T, resp fusion.Response) {
			if resp.Impact == nil || !resp.Impact.Truncated {
				t.Errorf("impact = %+v, want truncated", resp.Impact)
			}
		}},
		"graph": {want: fusion.WantGraph, check: func(t *testing.T, resp fusion.Response) {
			if resp.Graph == nil || !resp.Graph.Truncated {
				t.Errorf("graph = %+v, want truncated", resp.Graph)
			}
		}},
	}
	replies := map[string]func() error{"fatal": resetRequired, "index_not_ready": indexNotReady}
	for name, tc := range walks {
		for reply, replyErr := range replies {
			if tc.fatalOnly && reply != "fatal" {
				continue
			}
			t.Run(name+"/"+reply, func(t *testing.T) {
				g := &fakeGraph{
					seeds:        map[string][]string{"OnEvent": {seed.ID}},
					entities:     map[string]*fusion.Entity{seed.ID: seed},
					neighborsErr: replyErr(),
				}
				eng := fusion.NewEngine(g, fusion.NewBodyResolver(fusion.MapStoreResolver{}))
				resp, err := eng.Fuse(t.Context(), fusion.Request{Query: "OnEvent", Want: []fusion.Want{tc.want}}, refLens{})
				if err != nil {
					t.Fatalf("a %s %s read must be skipped, got err %v", reply, name, err)
				}
				if resp.Deferred || len(resp.Nodes) != 1 {
					t.Fatalf("want one answered node, got Deferred=%v nodes=%d", resp.Deferred, len(resp.Nodes))
				}
				tc.check(t, resp)
			})
		}
	}
}

// TestEndedContextReturnsError: when the caller's context ends, Fuse returns the
// context's error whichever read failed and whatever the read's reply said: the
// first row of design D1's table is checked before index_not_ready, the fatal
// class and the skip.
func TestEndedContextReturnsError(t *testing.T) {
	seed := entity("acme.ops.code.repo.symbol.OnEvent", "OnEvent", "on_event.go")
	target := entity("acme.ops.code.repo.symbol.Target", "Target", "target.go")
	edges := map[string][]fusion.Edge{seed.ID: {{Predicate: "ref.relationship.references", Target: target.ID}}}

	cases := map[string]struct {
		want  fusion.Want
		setup func(g *fakeGraph, cancel context.CancelFunc)
	}{
		"entities replies index_not_ready": {want: fusion.WantBody, setup: func(g *fakeGraph, cancel context.CancelFunc) {
			g.entitiesHook = func() error { cancel(); return indexNotReady() }
		}},
		"relation target replies fatal": {want: fusion.WantRelations, setup: func(g *fakeGraph, cancel context.CancelFunc) {
			g.out = edges
			g.entityHook = func(string) error { cancel(); return resetRequired() }
		}},
		"impact file read replies an error": {want: fusion.WantImpact, setup: func(g *fakeGraph, cancel context.CancelFunc) {
			g.in = edges
			g.entityHook = func(string) error { cancel(); return errors.New("transport closed") }
		}},
		"graph counterpart read replies an error": {want: fusion.WantGraph, setup: func(g *fakeGraph, cancel context.CancelFunc) {
			g.in = edges
			g.entityHook = func(string) error { cancel(); return errors.New("transport closed") }
		}},
		"paths name read replies an error": {want: fusion.WantPaths, setup: func(g *fakeGraph, cancel context.CancelFunc) {
			g.out = edges
			g.entityHook = func(string) error { cancel(); return errors.New("transport closed") }
		}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			g := &fakeGraph{
				seeds:    map[string][]string{"OnEvent": {seed.ID}},
				entities: map[string]*fusion.Entity{seed.ID: seed, target.ID: target},
			}
			tc.setup(g, cancel)
			eng := fusion.NewEngine(g, fusion.NewBodyResolver(fusion.MapStoreResolver{}))
			resp, err := eng.Fuse(ctx, fusion.Request{Query: "OnEvent", Want: []fusion.Want{tc.want}}, refLens{})
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("err = %v (response %+v), want context.Canceled", err, resp)
			}
		})
	}
}

// TestEngine_GenuineError_Propagates: a NON-transient (non-IndexNotReady) internal
// read error is a real backend fault and MUST still propagate as an error — never
// defer (that would mask a real failure as "not ready").
func TestEngine_GenuineError_Propagates(t *testing.T) {
	cases := map[string]*fakeGraph{
		"resolve genuine error": {resolveErr: errors.New("graph down")},
		"entities genuine error": {
			seeds:  map[string][]string{"OnEvent": {"acme.ops.code.repo.symbol.OnEvent"}},
			entErr: errors.New("batch down"),
		},
	}
	for name, g := range cases {
		t.Run(name, func(t *testing.T) {
			eng := fusion.NewEngine(g, fusion.NewBodyResolver(fusion.MapStoreResolver{}))
			if _, err := eng.Fuse(context.Background(), fusion.Request{Query: "OnEvent"}, refLens{}); err == nil {
				t.Error("expected a genuine backend error to propagate, not defer")
			}
		})
	}
}
