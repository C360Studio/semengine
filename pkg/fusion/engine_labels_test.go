package fusion_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/pkg/fusion"
)

// Tests for design D4: every left-out piece of an answer is declared, and every
// Neighbors result is ordered before a cap cuts it. Expected values come from
// how each fixture is built, never from the engine's walk.

// labelEntity builds an entity whose display name differs from its ID, so a test
// can tell a name from an ID on the wire.
func labelEntity(id string) *fusion.Entity {
	return entity(id, "name-of-"+id, id+".go")
}

// fuseWant runs one Fuse for want over g with the reference lens.
func fuseWant(t *testing.T, g *fakeGraph, want fusion.Want) fusion.Response {
	t.Helper()
	eng := fusion.NewEngine(g, nil)
	resp, err := eng.Fuse(t.Context(), fusion.Request{Query: "q", Want: []fusion.Want{want}}, refLens{})
	if err != nil {
		t.Fatalf("Fuse: %v", err)
	}
	return resp
}

// targetsGraph is seed S with outgoing reference edges to the given targets, each
// a readable entity unless left out of readable.
func targetsGraph(targets []string, readable map[string]bool) *fakeGraph {
	entities := map[string]*fusion.Entity{"S": labelEntity("S")}
	var edges []fusion.Edge
	for _, id := range targets {
		if readable == nil || readable[id] {
			entities[id] = labelEntity(id)
		}
		edges = append(edges, edge(id))
	}
	return facetGraph(map[string][]fusion.Edge{"S": edges}, nil, entities)
}

func numbered(n int) []string {
	out := make([]string, n)
	for i := range n {
		out[i] = fmt.Sprintf("t%02d", i)
	}
	return out
}

func refNames(refs []fusion.Ref) []string {
	out := make([]string, len(refs))
	for i, r := range refs {
		out[i] = r.Name
	}
	return out
}

func names(ids []string) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = "name-of-" + id
	}
	return out
}

// TestEdgesSortedBeforeCap: two clients list the same 13 edges, one in order and
// one reversed with a duplicate; both nodes hold the same 12 refs, the first 12
// targets by (predicate, target), each once.
func TestEdgesSortedBeforeCap(t *testing.T) {
	targets := numbered(13)
	forward := targetsGraph(targets, nil)
	reversed := targetsGraph(targets, nil)
	rev := slices.Clone(forward.out["S"])
	slices.Reverse(rev)
	reversed.out = map[string][]fusion.Edge{"S": append(rev, edge("t00"))}

	want := names(targets[:12])
	for name, g := range map[string]*fakeGraph{"forward": forward, "reversed with a duplicate": reversed} {
		t.Run(name, func(t *testing.T) {
			resp := fuseWant(t, g, fusion.WantRelations)
			if got := refNames(resp.Nodes[0].Relations["referent"]); !slices.Equal(got, want) {
				t.Errorf("refs = %v, want %v", got, want)
			}
		})
	}
}

// TestRelationsCapLabelled: 13 edges for one role give 12 refs and
// relations_truncated; exactly 12 give 12 refs and no key.
func TestRelationsCapLabelled(t *testing.T) {
	over := fuseWant(t, targetsGraph(numbered(13), nil), fusion.WantRelations).Nodes[0]
	if len(over.Relations["referent"]) != 12 || !over.RelationsTruncated {
		t.Errorf("13 edges: %d refs, relations_truncated=%v; want 12, true",
			len(over.Relations["referent"]), over.RelationsTruncated)
	}
	exact := fuseWant(t, targetsGraph(numbered(12), nil), fusion.WantRelations).Nodes[0]
	raw, err := json.Marshal(exact)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(exact.Relations["referent"]) != 12 || strings.Contains(string(raw), "relations_truncated") {
		t.Errorf("12 edges: %d refs, want 12 and no relations_truncated key: %s", len(exact.Relations["referent"]), raw)
	}
}

// TestRelationsFaultLabelled: a relations Neighbors read that fails with an
// unclassified error is skipped, and the node ships labelled. Replaces the pin's
// TestEngine_NeighborGenuineError_Swallowed.
func TestRelationsFaultLabelled(t *testing.T) {
	g := targetsGraph(numbered(2), nil)
	g.neighborsErr = errors.New("neighbors backend down")
	resp := fuseWant(t, g, fusion.WantRelations)
	if resp.Deferred || len(resp.Nodes) != 1 {
		t.Fatalf("want one answered node, got Deferred=%v nodes=%d", resp.Deferred, len(resp.Nodes))
	}
	if !resp.Nodes[0].RelationsTruncated {
		t.Error("a skipped Neighbors read must set relations_truncated")
	}
}

// TestRelationsUnreadTargetLabelled: a target whose read fails, or finds nothing,
// is left out and the node is labelled; with every target readable it is not.
func TestRelationsUnreadTargetLabelled(t *testing.T) {
	targets := []string{"a", "b"}
	cases := map[string]struct {
		mutate func(g *fakeGraph)
		want   []string
		label  bool
	}{
		"all readable": {mutate: func(*fakeGraph) {}, want: names(targets), label: false},
		"read fails": {mutate: func(g *fakeGraph) {
			g.entityErr = map[string]error{"a": errors.New("entity read failed")}
		}, want: names([]string{"b"}), label: true},
		"read finds nothing": {mutate: func(g *fakeGraph) { delete(g.entities, "a") },
			want: names([]string{"b"}), label: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			g := targetsGraph(targets, nil)
			tc.mutate(g)
			node := fuseWant(t, g, fusion.WantRelations).Nodes[0]
			if got := refNames(node.Relations["referent"]); !slices.Equal(got, tc.want) {
				t.Errorf("refs = %v, want %v", got, tc.want)
			}
			if node.RelationsTruncated != tc.label {
				t.Errorf("relations_truncated = %v, want %v", node.RelationsTruncated, tc.label)
			}
		})
	}
}

// TestPathsCapLabelled: 11 leaf paths against the cap of 10 give 10 paths and
// paths_truncated; exactly 10 give 10 and no key.
func TestPathsCapLabelled(t *testing.T) {
	over := fuseWant(t, targetsGraph(numbered(11), nil), fusion.WantPaths)
	if len(over.Paths) != 10 || !over.PathsTruncated {
		t.Errorf("11 leaves: %d paths, paths_truncated=%v; want 10, true", len(over.Paths), over.PathsTruncated)
	}
	exact := fuseWant(t, targetsGraph(numbered(10), nil), fusion.WantPaths)
	raw, err := json.Marshal(exact)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(exact.Paths) != 10 || strings.Contains(string(raw), "paths_truncated") {
		t.Errorf("10 leaves: %d paths, want 10 and no paths_truncated key: %s", len(exact.Paths), raw)
	}
}

// TestPathStopsAtUnreadNode: S → B → C. When C cannot be read or reads as absent,
// the path is S, B marked truncated; when B cannot be read, no path is emitted and
// paths_truncated is set. No path ever holds an entity ID.
func TestPathStopsAtUnreadNode(t *testing.T) {
	build := func() *fakeGraph {
		g := facetGraph(map[string][]fusion.Edge{"S": {edge("B")}, "B": {edge("C")}}, nil,
			map[string]*fusion.Entity{"S": labelEntity("S"), "B": labelEntity("B"), "C": labelEntity("C")})
		return g
	}
	cases := map[string]struct {
		mutate    func(g *fakeGraph)
		want      []fusion.Path
		truncated bool
	}{
		"all readable": {mutate: func(*fakeGraph) {},
			want: []fusion.Path{{Names: names([]string{"S", "B", "C"})}}},
		"third node read fails": {mutate: func(g *fakeGraph) {
			g.entityErr = map[string]error{"C": errors.New("entity read failed")}
		}, want: []fusion.Path{{Names: names([]string{"S", "B"}), Truncated: true}}},
		"third node absent": {mutate: func(g *fakeGraph) { delete(g.entities, "C") },
			want: []fusion.Path{{Names: names([]string{"S", "B"}), Truncated: true}}},
		"second node read fails": {mutate: func(g *fakeGraph) {
			g.entityErr = map[string]error{"B": errors.New("entity read failed")}
		}, want: nil, truncated: true},
		"second node absent": {mutate: func(g *fakeGraph) { delete(g.entities, "B") },
			want: nil, truncated: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			g := build()
			tc.mutate(g)
			resp := fuseWant(t, g, fusion.WantPaths)
			if !slices.EqualFunc(resp.Paths, tc.want, func(a, b fusion.Path) bool {
				return a.Truncated == b.Truncated && slices.Equal(a.Names, b.Names)
			}) {
				t.Errorf("paths = %+v, want %+v", resp.Paths, tc.want)
			}
			if resp.PathsTruncated != tc.truncated {
				t.Errorf("paths_truncated = %v, want %v", resp.PathsTruncated, tc.truncated)
			}
			for _, p := range resp.Paths {
				for _, n := range p.Names {
					if n == "S" || n == "B" || n == "C" {
						t.Errorf("path %v holds the entity ID %q", p.Names, n)
					}
				}
			}
		})
	}
}

// TestImpactUnreadFileTruncates: A points at seed S; when A's entity read (for its
// file) fails, impact is truncated; when it succeeds or finds nothing, it is not.
func TestImpactUnreadFileTruncates(t *testing.T) {
	cases := map[string]struct {
		mutate    func(g *fakeGraph)
		truncated bool
	}{
		"readable":           {mutate: func(*fakeGraph) {}, truncated: false},
		"read fails":         {mutate: func(g *fakeGraph) { g.entityErr = map[string]error{"A": errors.New("entity read failed")} }, truncated: true},
		"read finds nothing": {mutate: func(g *fakeGraph) { delete(g.entities, "A") }, truncated: false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			g := facetGraph(nil, map[string][]fusion.Edge{"S": {edge("A")}},
				map[string]*fusion.Entity{"S": labelEntity("S"), "A": labelEntity("A")})
			tc.mutate(g)
			imp := fuseWant(t, g, fusion.WantImpact).Impact
			if imp == nil || imp.Nodes != 1 || imp.Truncated != tc.truncated {
				t.Errorf("impact = %+v, want 1 node, truncated %v", imp, tc.truncated)
			}
		})
	}
}

// TestGraphCounterpartFaultTruncates: an incoming counterpart whose read fails
// truncates the graph facet; one that reads as absent keeps its edge without
// evidence and does not.
func TestGraphCounterpartFaultTruncates(t *testing.T) {
	cases := map[string]struct {
		mutate    func(g *fakeGraph)
		truncated bool
	}{
		"read fails":         {mutate: func(g *fakeGraph) { g.entityErr = map[string]error{"A": errors.New("entity read failed")} }, truncated: true},
		"read finds nothing": {mutate: func(g *fakeGraph) { delete(g.entities, "A") }, truncated: false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			g := facetGraph(nil, map[string][]fusion.Edge{"S": {edge("A")}},
				map[string]*fusion.Entity{"S": labelEntity("S"), "A": labelEntity("A")})
			tc.mutate(g)
			proj := fuseWant(t, g, fusion.WantGraph).Graph
			if proj == nil || proj.Truncated != tc.truncated {
				t.Fatalf("graph = %+v, want truncated %v", proj, tc.truncated)
			}
			if len(proj.Edges) != 1 || len(proj.Edges[0].Evidence) != 0 {
				t.Errorf("edges = %+v, want the one incoming edge without evidence", proj.Edges)
			}
		})
	}
}

// TestGraphCapsIgnoreTripleOrder: the graph facet's edge cap (256) and fact cap
// (64) keep the same edges and facts whatever order the seed's triples are
// stored in: sort before limit. 257 declared edges to t000..t256 keep
// t000..t255; 67 facts v00..v66 keep v00..v63.
func TestGraphCapsIgnoreTripleOrder(t *testing.T) {
	var triples []message.Triple
	for i := range 257 {
		triples = append(triples, message.Triple{Predicate: refEdgePred, Object: fmt.Sprintf("t%03d", i)})
	}
	for i := range 67 {
		triples = append(triples, message.Triple{Predicate: "acme.fact.cap", Object: fmt.Sprintf("v%02d", i)})
	}
	reversed := slices.Clone(triples)
	slices.Reverse(reversed)

	wantEdges := make([]string, 256)
	for i := range wantEdges {
		wantEdges[i] = fmt.Sprintf("S|%s|t%03d", refEdgePred, i)
	}
	wantFacts := make([]string, 64)
	for i := range wantFacts {
		wantFacts[i] = fmt.Sprintf("%q", fmt.Sprintf("v%02d", i))
	}
	for name, order := range map[string][]message.Triple{"stored order": triples, "reversed": reversed} {
		t.Run(name, func(t *testing.T) {
			seed := &fusion.Entity{ID: "S", Triples: order}
			proj := fuseWant(t, facetGraph(nil, nil, map[string]*fusion.Entity{"S": seed}), fusion.WantGraph).Graph
			var edges []string
			for _, e := range proj.Edges {
				edges = append(edges, e.ID)
			}
			slices.Sort(edges)
			if !slices.Equal(edges, wantEdges) {
				t.Errorf("edges kept = %d, first missing from want: %v", len(edges), firstDiff(edges, wantEdges))
			}
			var facts []string
			for _, f := range proj.Nodes[0].Facts {
				facts = append(facts, string(f.Value))
			}
			slices.Sort(facts)
			if !slices.Equal(facts, wantFacts) || proj.Nodes[0].FactsDropped != 3 {
				t.Errorf("facts kept = %v (dropped %d), want %v (dropped 3)", facts, proj.Nodes[0].FactsDropped, wantFacts)
			}
		})
	}
}

// firstDiff names the first position where got and want differ.
func firstDiff(got, want []string) string {
	for i := range min(len(got), len(want)) {
		if got[i] != want[i] {
			return fmt.Sprintf("at %d got %s want %s", i, got[i], want[i])
		}
	}
	return fmt.Sprintf("lengths %d vs %d", len(got), len(want))
}
