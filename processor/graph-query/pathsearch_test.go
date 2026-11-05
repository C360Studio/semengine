package graphquery

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/c360studio/semengine/graph"
)

// modelNode is the generated graph's node i, a valid entity ID.
func modelNode(i int) string { return fmt.Sprintf("c360.test.query.system.drone.%03d", i) }

// modelGraph is the edges a graph holds, in the order its index would send them, repeats included.
type modelGraph []graph.RelationshipEdge

// modelCrossing is one way the request lets a walk cross an edge.
type modelCrossing struct {
	from, to string
	incoming bool
	edge     graph.RelationshipEdge
}

func (g modelGraph) crossings(request graph.PathSearchRequest) []modelCrossing {
	var crossings []modelCrossing
	for _, edge := range g {
		if len(request.Predicates) > 0 && !slices.Contains(request.Predicates, edge.Predicate) {
			continue
		}
		if request.Direction != "incoming" {
			crossings = append(crossings, modelCrossing{from: edge.FromEntityID, to: edge.ToEntityID, edge: edge})
		}
		if request.Direction != "outgoing" {
			crossings = append(crossings, modelCrossing{from: edge.ToEntityID, to: edge.FromEntityID, incoming: true, edge: edge})
		}
	}
	return crossings
}

// modelOrder is the requirement written as levels, sharing nothing with the walk: distances by
// relaxation (repeat until no distance shrinks), then each level in order of the earliest entity that
// reaches it and its ID, each entity's edge the smallest by predicate, outgoing before incoming, from
// that earliest entity (spec, "Path search answers within its limits").
func (g modelGraph) order(request graph.PathSearchRequest) ([]graph.PathEntity, []graph.RelationshipEdge) {
	crossings := g.crossings(request)
	distance := map[string]int{request.StartEntity: 0}
	for changed := true; changed; {
		changed = false
		for _, c := range crossings {
			from, reached := distance[c.from]
			if !reached || from >= request.MaxDepth {
				continue
			}
			if to, known := distance[c.to]; !known || from+1 < to {
				distance[c.to] = from + 1
				changed = true
			}
		}
	}

	entities := []graph.PathEntity{{EntityID: request.StartEntity, Depth: 0}}
	edges := []graph.RelationshipEdge{}
	positionOf := func(id string) int {
		return slices.IndexFunc(entities, func(e graph.PathEntity) bool { return e.EntityID == id })
	}
	for level := 1; level <= request.MaxDepth; level++ {
		type arrival struct {
			position int
			id       string
			by       modelCrossing
		}
		best := map[string]arrival{}
		for _, c := range crossings {
			if distance[c.to] != level || c.to == request.StartEntity {
				continue
			}
			from, reached := distance[c.from]
			if !reached || from != level-1 {
				continue
			}
			candidate := arrival{position: positionOf(c.from), id: c.to, by: c}
			current, known := best[c.to]
			if !known || cmp.Or(
				cmp.Compare(candidate.position, current.position),
				strings.Compare(candidate.by.edge.Predicate, current.by.edge.Predicate),
				cmp.Compare(boolRank(candidate.by.incoming), boolRank(current.by.incoming)),
			) < 0 {
				best[c.to] = candidate
			}
		}
		arrivals := make([]arrival, 0, len(best))
		for _, a := range best {
			arrivals = append(arrivals, a)
		}
		slices.SortFunc(arrivals, func(a, b arrival) int {
			return cmp.Or(cmp.Compare(a.position, b.position), strings.Compare(a.id, b.id))
		})
		for _, a := range arrivals {
			entities = append(entities, graph.PathEntity{EntityID: a.id, Depth: level})
			edges = append(edges, a.by.edge)
		}
	}
	return entities, edges
}

func boolRank(b bool) int {
	if b {
		return 1
	}
	return 0
}

// expectedLimit reads max_nodes as the requirement states it: 0 means 100.
func expectedLimit(request graph.PathSearchRequest) int {
	if request.MaxNodes == 0 {
		return 100
	}
	return request.MaxNodes
}

// modelAnswer is the reply the requirement states for g and request.
func (g modelGraph) answer(request graph.PathSearchRequest) graph.PathSearchData {
	entities, edges := g.order(request)
	limit := expectedLimit(request)
	if len(entities) <= limit {
		return graph.PathSearchData{Entities: entities, Edges: edges}
	}
	return graph.PathSearchData{Entities: entities[:limit], Edges: edges[:limit-1], Truncated: true}
}

// checkWalk runs the walk over g, through a hop answering as an index does, and holds it to the model:
// the reply, the smallest revision of the hops it made, and each entity read at most once per direction
// and only below max_depth. Every run reaches every assertion: a walk that errs fails here, and the
// walk always hops at the start, since max_depth is at least 1.
func checkWalk(ctx context.Context, t rapid.TB, g modelGraph, request graph.PathSearchRequest, revisionOf func(call string) uint64) graph.PathSearchData {
	t.Helper()
	type hopped struct{ entity, direction string }
	var hops []hopped
	smallest := uint64(math.MaxUint64)
	hop := func(_ context.Context, entityID, direction string) ([]graph.RelationshipEdge, uint64, error) {
		hops = append(hops, hopped{entityID, direction})
		revision := revisionOf(entityID + " " + direction)
		smallest = min(smallest, revision)
		edges := []graph.RelationshipEdge{}
		for _, edge := range g {
			if (direction == "outgoing" && edge.FromEntityID == entityID) || (direction == "incoming" && edge.ToEntityID == entityID) {
				edges = append(edges, edge)
			}
		}
		return edges, revision, nil
	}

	got, revision, err := walkPaths(ctx, request, hop)
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	gotJSON, wantJSON := mustJSON(t, got), mustJSON(t, g.answer(request))
	if gotJSON != wantJSON {
		t.Fatalf("request %+v over %+v:\n got %s\nwant %s", request, g, gotJSON, wantJSON)
	}
	if revision != smallest {
		t.Fatalf("revision = %d, want the smallest of the hops, %d", revision, smallest)
	}

	seen := map[hopped]bool{}
	for _, h := range hops {
		if seen[h] {
			t.Fatalf("%s read twice %s", h.entity, h.direction)
		}
		seen[h] = true
		position := slices.IndexFunc(got.Entities, func(e graph.PathEntity) bool { return e.EntityID == h.entity })
		if position < 0 {
			t.Fatalf("hopped at %s, which the reply does not hold", h.entity)
		}
		if got.Entities[position].Depth >= request.MaxDepth {
			t.Fatalf("hopped at %s at depth %d with max_depth %d", h.entity, got.Entities[position].Depth, request.MaxDepth)
		}
	}
	bound := expectedLimit(request)
	if request.Direction == "both" {
		bound *= 2
	}
	if len(hops) > bound {
		t.Fatalf("%d hops, over the bound %d", len(hops), bound)
	}
	return got
}

func mustJSON(t rapid.TB, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(encoded)
}

func constantRevision(string) uint64 { return 7 }

// TestPathSearchMatchesTheModel: named witnesses, each with its literal reply, then generated graphs,
// holding the walk to an independent model (spec, "Generated graphs"). Inputs interact: direction,
// filter, depth and limit each change which edge reaches which entity first.
func TestPathSearchMatchesTheModel(t *testing.T) {
	a, b, c, d, z := modelNode(1), modelNode(2), modelNode(3), modelNode(4), modelNode(26)
	edge := func(from, predicate, to string) graph.RelationshipEdge {
		return graph.RelationshipEdge{Predicate: predicate, FromEntityID: from, ToEntityID: to}
	}
	const p, q = "test.link.p", "test.link.q"
	entityJSON := func(id string, depth int) string { return fmt.Sprintf(`{"entity_id":%q,"depth":%d}`, id, depth) }
	edgeJSON := func(from, predicate, to string) string {
		return fmt.Sprintf(`{"predicate":%q,"from_entity_id":%q,"to_entity_id":%q}`, predicate, from, to)
	}
	reply := func(entities, edges []string, truncated bool) string {
		return fmt.Sprintf(`{"entities":[%s],"edges":[%s],"truncated":%t}`, strings.Join(entities, ","), strings.Join(edges, ","), truncated)
	}
	request := func(direction string, maxDepth, maxNodes int, predicates ...string) graph.PathSearchRequest {
		return graph.PathSearchRequest{StartEntity: a, Direction: direction, MaxDepth: maxDepth, MaxNodes: maxNodes, Predicates: predicates}
	}

	for _, tc := range []struct {
		name    string
		graph   modelGraph
		request graph.PathSearchRequest
		want    string
	}{
		{"an incoming edge says what the graph holds", modelGraph{edge(b, p, a)}, request("incoming", 1, 0),
			reply([]string{entityJSON(a, 0), entityJSON(b, 1)}, []string{edgeJSON(b, p, a)}, false)},
		{"both crosses a two-way pair by its outgoing edge", modelGraph{edge(b, p, a), edge(a, p, b)}, request("both", 1, 0),
			reply([]string{entityJSON(a, 0), entityJSON(b, 1)}, []string{edgeJSON(a, p, b)}, false)},
		{"the smaller predicate reaches first, whichever direction holds it", modelGraph{edge(a, q, b), edge(b, p, a)}, request("both", 1, 0),
			reply([]string{entityJSON(a, 0), entityJSON(b, 1)}, []string{edgeJSON(b, p, a)}, false)},
		{"a self loop and repeated rows add nothing", modelGraph{edge(a, p, a), edge(a, p, b), edge(a, p, b)}, request("outgoing", 1, 0),
			reply([]string{entityJSON(a, 0), entityJSON(b, 1)}, []string{edgeJSON(a, p, b)}, false)},
		{"a cycle ends", modelGraph{edge(a, p, b), edge(b, p, a)}, request("outgoing", 10, 0),
			reply([]string{entityJSON(a, 0), entityJSON(b, 1)}, []string{edgeJSON(a, p, b)}, false)},
		{"depth is the shortest distance", modelGraph{edge(a, p, b), edge(b, p, c), edge(a, p, c)}, request("outgoing", 2, 0),
			reply([]string{entityJSON(a, 0), entityJSON(b, 1), entityJSON(c, 1)}, []string{edgeJSON(a, p, b), edgeJSON(a, p, c)}, false)},
		{"breadth first: a level before the next, in order of who reached it", modelGraph{edge(a, p, z), edge(a, p, b), edge(z, p, c), edge(b, p, d)}, request("outgoing", 2, 0),
			reply([]string{entityJSON(a, 0), entityJSON(b, 1), entityJSON(z, 1), entityJSON(d, 2), entityJSON(c, 2)},
				[]string{edgeJSON(a, p, b), edgeJSON(a, p, z), edgeJSON(b, p, d), edgeJSON(z, p, c)}, false)},
		{"an entity at max_depth is kept and not read past", modelGraph{edge(a, p, b), edge(b, p, c)}, request("outgoing", 1, 0),
			reply([]string{entityJSON(a, 0), entityJSON(b, 1)}, []string{edgeJSON(a, p, b)}, false)},
		{"the filter applies after the hop", modelGraph{edge(a, q, b), edge(a, p, c)}, request("outgoing", 1, 0, p),
			reply([]string{entityJSON(a, 0), entityJSON(c, 1)}, []string{edgeJSON(a, p, c)}, false)},
		{"a filter nothing matches leaves the start", modelGraph{edge(a, q, b)}, request("outgoing", 1, 0, p),
			reply([]string{entityJSON(a, 0)}, nil, false)},
		{"the limit meets the graph exactly", modelGraph{edge(a, p, b), edge(a, p, c)}, request("outgoing", 1, 3),
			reply([]string{entityJSON(a, 0), entityJSON(b, 1), entityJSON(c, 1)}, []string{edgeJSON(a, p, b), edgeJSON(a, p, c)}, false)},
		{"the limit one lower", modelGraph{edge(a, p, b), edge(a, p, c)}, request("outgoing", 1, 2),
			reply([]string{entityJSON(a, 0), entityJSON(b, 1)}, []string{edgeJSON(a, p, b)}, true)},
		{"a limit of one with a neighbour", modelGraph{edge(a, p, b)}, request("outgoing", 1, 1),
			reply([]string{entityJSON(a, 0)}, nil, true)},
		{"a limit of one with no neighbour", modelGraph{edge(b, p, a)}, request("outgoing", 1, 1),
			reply([]string{entityJSON(a, 0)}, nil, false)},
		{"full, and a later entity finds nothing new", modelGraph{edge(a, p, b), edge(a, p, c), edge(b, p, c), edge(c, p, a)}, request("outgoing", 2, 3),
			reply([]string{entityJSON(a, 0), entityJSON(b, 1), entityJSON(c, 1)}, []string{edgeJSON(a, p, b), edgeJSON(a, p, c)}, false)},
		{"full, and a later entity finds one", modelGraph{edge(a, p, b), edge(b, p, c)}, request("outgoing", 2, 2),
			reply([]string{entityJSON(a, 0), entityJSON(b, 1)}, []string{edgeJSON(a, p, b)}, true)},
		{"full at max_depth, with an entity past it unseen", modelGraph{edge(a, p, b), edge(b, p, c)}, request("outgoing", 1, 2),
			reply([]string{entityJSON(a, 0), entityJSON(b, 1)}, []string{edgeJSON(a, p, b)}, false)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := checkWalk(t.Context(), t, tc.graph, tc.request, constantRevision)
			if encoded := mustJSON(t, got); encoded != tc.want {
				t.Errorf("reply\n got %s\nwant %s", encoded, tc.want)
			}
		})
	}

	t.Run("max_nodes 0 means 100", func(t *testing.T) {
		var star modelGraph
		for i := range 120 {
			star = append(star, edge(a, p, modelNode(100+i)))
		}
		got := checkWalk(t.Context(), t, star, request("outgoing", 1, 0), constantRevision)
		if len(got.Entities) != 100 || !got.Truncated {
			t.Errorf("%d entities, truncated %t; want 100, true", len(got.Entities), got.Truncated)
		}
	})

	t.Run("max_depth 10 stops at the tenth hop", func(t *testing.T) {
		var chain modelGraph
		for i := range graph.MaxPathSearchDepth + 1 {
			chain = append(chain, edge(modelNode(100+i), p, modelNode(101+i)))
		}
		g := request("outgoing", graph.MaxPathSearchDepth, 0)
		g.StartEntity = modelNode(100)
		got := checkWalk(t.Context(), t, chain, g, constantRevision)
		last := got.Entities[len(got.Entities)-1]
		if len(got.Entities) != graph.MaxPathSearchDepth+1 || last.EntityID != modelNode(110) || last.Depth != graph.MaxPathSearchDepth || got.Truncated {
			t.Errorf("%d entities, last %+v, truncated %t; want 11, %s at depth 10, false", len(got.Entities), last, got.Truncated, modelNode(110))
		}
	})

	for _, tc := range []struct {
		name      string
		leaves    int
		truncated bool
	}{
		{"max_nodes 500 holds an exact fit", graph.MaxPathSearchNodes - 1, false},
		{"max_nodes 500 declares one more", graph.MaxPathSearchNodes, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var star modelGraph
			for i := range tc.leaves {
				star = append(star, edge(a, p, modelNode(100+i)))
			}
			got := checkWalk(t.Context(), t, star, request("outgoing", 1, graph.MaxPathSearchNodes), constantRevision)
			if len(got.Entities) != graph.MaxPathSearchNodes || got.Truncated != tc.truncated {
				t.Errorf("%d entities, truncated %t; want 500, %t", len(got.Entities), got.Truncated, tc.truncated)
			}
		})
	}

	predicates := []string{p, q, "test.link.r"}
	rapid.Check(t, func(rt *rapid.T) {
		nodes := rapid.IntRange(1, 8).Draw(rt, "nodes")
		var g modelGraph
		for range rapid.IntRange(0, 24).Draw(rt, "edges") {
			g = append(g, edge(
				modelNode(rapid.IntRange(1, nodes).Draw(rt, "from")),
				rapid.SampledFrom(predicates).Draw(rt, "predicate"),
				modelNode(rapid.IntRange(1, nodes).Draw(rt, "to")),
			))
		}
		generated := graph.PathSearchRequest{
			StartEntity: modelNode(rapid.IntRange(1, nodes).Draw(rt, "start")),
			Direction:   rapid.SampledFrom([]string{"outgoing", "incoming", "both"}).Draw(rt, "direction"),
			MaxDepth:    rapid.IntRange(1, 10).Draw(rt, "max_depth"),
		}
		if rapid.Bool().Draw(rt, "filtered") {
			generated.Predicates = rapid.SliceOfNDistinct(rapid.SampledFrom(predicates), 1, 2, rapid.ID[string]).Draw(rt, "predicates")
		}
		// The limit's boundaries are drawn by construction: exactly the entities in reach, or one fewer.
		reach, _ := g.order(generated)
		switch rapid.SampledFrom([]string{"exact", "one lower", "any"}).Draw(rt, "limit") {
		case "exact":
			generated.MaxNodes = len(reach)
		case "one lower":
			generated.MaxNodes = max(1, len(reach)-1)
		default:
			generated.MaxNodes = rapid.IntRange(0, nodes+1).Draw(rt, "max_nodes")
		}
		revisions := map[string]uint64{}
		checkWalk(t.Context(), rt, g, generated, func(call string) uint64 {
			if _, known := revisions[call]; !known {
				revisions[call] = rapid.Uint64Range(1, 1000).Draw(rt, "revision")
			}
			return revisions[call]
		})
	})
}
