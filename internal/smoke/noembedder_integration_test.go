//go:build integration

package smoke

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/readiness"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/fusion"
	"github.com/c360studio/semengine/pkg/fusion/fusionnats"
)

// fleetLens reads the fixed entities for fusion: name from namePredicate, kind from the ID's type
// position, the two link predicates as relations. mode is the one resolve mode it asks for.
type fleetLens struct{ mode fusion.ResolveMode }

func (l fleetLens) Name() string                          { return "fleet-" + string(l.mode) }
func (l fleetLens) ResolveMode(string) fusion.ResolveMode { return l.mode }
func (fleetLens) Edges() []fusion.EdgeSpec {
	return []fusion.EdgeSpec{
		{Predicate: nextPredicate, OutgoingRole: "next", IncomingRole: "previous"},
		{Predicate: homePredicate, OutgoingRole: "home", IncomingRole: "resident"},
	}
}
func (fleetLens) Label(e *fusion.Entity) string          { return e.First(namePredicate) }
func (fleetLens) Kind(e *fusion.Entity) string           { return strings.Split(e.ID, ".")[4] }
func (fleetLens) Location(*fusion.Entity) fusion.Locator { return fusion.Locator{} }
func (fleetLens) Hydrate(context.Context, *fusion.Entity) (*message.StorageReference, error) {
	return nil, nil
}

// fusionEngine is fusion over fusionnats on o's connection, as a consumer builds it: no body
// resolver, so no lens here hydrates a body.
func (o observer) fusionEngine(t *testing.T) *fusion.Engine {
	t.Helper()
	client, err := fusionnats.New(o.client, requestTimeout)
	if err != nil {
		t.Fatalf("fusionnats.New: %v", err)
	}
	engine, err := fusion.NewEngine(client, nil, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("fusion.NewEngine: %v", err)
	}
	return engine
}

// requireFused fails unless resp, as a consumer reads it on the wire, is want.
func requireFused(t *testing.T, what string, resp fusion.Response, err error, want fusion.Response) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: Fuse: %v", what, err)
	}
	got, gerr := json.Marshal(resp)
	wanted, werr := json.Marshal(want)
	if gerr != nil || werr != nil {
		t.Fatalf("%s: marshal: %v, %v", what, gerr, werr)
	}
	if string(got) != string(wanted) {
		t.Errorf("%s answered\n  %s\nwant\n  %s", what, got, wanted)
	}
}

// graphStatusKeys lists the GRAPH_STATUS keys, sorted.
func (o observer) graphStatusKeys(t *testing.T) []string {
	t.Helper()
	kv, err := o.js.KeyValue(t.Context(), graph.BucketGraphStatus)
	if err != nil {
		t.Fatalf("open %s: %v", graph.BucketGraphStatus, err)
	}
	lister, err := kv.ListKeys(t.Context())
	if err != nil {
		t.Fatalf("list %s: %v", graph.BucketGraphStatus, err)
	}
	var keys []string
	for key := range lister.Keys() {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// TestSmoke_NoEmbedderAnswers holds composed-stack, "The graph stack answers fusion without an
// embedder": graph-ingest, graph-index and graph-query boot from one configuration with no
// graph-embedding, and fusion over fusionnats answers symbol and prefix questions from them, misses
// an unknown name, and refuses a word question as not enabled, naming the semantic verb, never
// answering it empty.
func TestSmoke_NoEmbedderAnswers(t *testing.T) {
	url := shared.Reset(t)
	o := newObserver(t, url)
	s := startComposition(t, o, url, graphOnlyStack)
	f := fixture{platform: s.platform}
	// StartAll has returned, so the composition is fixed: the three components are listed.
	requireComponentsListed(t, s)

	var last uint64
	for _, e := range f.fixedEntities() {
		last = o.publish(t, e)
	}
	o.awaitAcknowledged(t, s, last)
	o.awaitIndexed(t, s)
	requireComponentsHealthy(t, s)
	// awaitIndexed read graph-ingest's and graph-index's entries, written by this child on a
	// server the reset emptied, so the listing is complete for both; nothing runs to write a third.
	if keys, want := o.graphStatusKeys(t), []string{readiness.KeyGraphIndex, readiness.KeyGraphIngest}; !slices.Equal(keys, want) {
		t.Fatalf("%s holds keys %v, want %v (no %s)", graph.BucketGraphStatus, keys, want, readiness.KeyGraphEmbedding)
	}

	engine := o.fusionEngine(t)
	wants := []fusion.Want{fusion.WantRelations, fusion.WantPaths}
	ref := func(name string) []fusion.Ref { return []fusion.Ref{{Name: name}} }
	node := func(local, name, kind string, relations map[string][]fusion.Ref) fusion.Node {
		return fusion.Node{Name: name, Kind: kind, Handle: f.id(local), Relations: relations}
	}
	path := func(truncated bool, names ...string) fusion.Path {
		return fusion.Path{Names: names, Truncated: truncated}
	}

	// Every expectation below is written from fixedEntities, never from a reply. The waits above
	// order these reads after the batch: graph-ingest acknowledged every input, and graph-index's
	// envelope reached the ENTITY_STATES revision read after that.

	// Symbol: graph-index's name lookup over namePredicate, the composition's display name.
	symbol := fusion.Request{Query: "Kestrel", Want: wants}
	resp, err := engine.Fuse(t.Context(), symbol, fleetLens{fusion.ResolveModeSymbol})
	requireFused(t, "symbol Kestrel", resp, err, fusion.Response{
		Provenance: fusion.ProvenanceDeterministic,
		Nodes: []fusion.Node{node(drone2, "Kestrel", "drone",
			map[string][]fusion.Ref{"next": ref("Osprey"), "previous": ref("Falcon")})},
		Paths:           []fusion.Path{path(false, "Kestrel", "Osprey", "Harrier", "North Field")},
		ContractVersion: fusion.ContractVersion,
	})

	// Prefix: every drone, in ID order (graph-ingest sorts the keys; the names score nothing
	// against an ID prefix). drone1's path is four hops, the walk's depth cap, so it is cut there.
	prefix := fusion.Request{Query: "c360." + f.platform + ".fleet.ops.drone", Want: wants}
	resp, err = engine.Fuse(t.Context(), prefix, fleetLens{fusion.ResolveModePrefix})
	southField := map[string][]fusion.Ref{"home": ref("South Field")}
	requireFused(t, "prefix drones", resp, err, fusion.Response{
		Provenance: fusion.ProvenanceDeterministic,
		Nodes: []fusion.Node{
			node(drone1, "Falcon", "drone", map[string][]fusion.Ref{"next": ref("Kestrel")}),
			node(drone2, "Kestrel", "drone", map[string][]fusion.Ref{"next": ref("Osprey"), "previous": ref("Falcon")}),
			node(drone3, "Osprey", "drone", map[string][]fusion.Ref{"next": ref("Harrier"), "previous": ref("Kestrel")}),
			node(drone4, "Harrier", "drone", map[string][]fusion.Ref{"home": ref("North Field"), "previous": ref("Osprey")}),
			node(drone5, "Merlin", "drone", southField),
			node(drone6, "Hobby", "drone", southField),
			node(drone7, "Lanner", "drone", southField),
			node(drone8, "Saker", "drone", southField),
		},
		Paths: []fusion.Path{
			path(true, "Falcon", "Kestrel", "Osprey", "Harrier", "North Field"),
			path(false, "Kestrel", "Osprey", "Harrier", "North Field"),
			path(false, "Osprey", "Harrier", "North Field"),
			path(false, "Harrier", "North Field"),
			path(false, "Merlin", "South Field"),
			path(false, "Hobby", "South Field"),
			path(false, "Lanner", "South Field"),
			path(false, "Saker", "South Field"),
		},
		ContractVersion: fusion.ContractVersion,
	})

	// An unknown name is a miss, with no near-match: no name starts with or holds it.
	resp, err = engine.Fuse(t.Context(), fusion.Request{Query: "Condor", Want: wants}, fleetLens{fusion.ResolveModeSymbol})
	requireFused(t, "symbol Condor", resp, err, fusion.Response{
		Provenance:      fusion.ProvenanceDeterministic,
		Misses:          []fusion.Miss{{Query: "Condor"}},
		ContractVersion: fusion.ContractVersion,
	})

	// A word question goes to graph.query.semantic. graph-query is configured with embedding_enabled
	// false, so it refuses at once, invalid not_enabled naming the verb, and the refusal reaches the
	// caller intact (graph-query, "Embedding verbs can be turned off"). Never an empty answer.
	resp, err = engine.Fuse(t.Context(), fusion.Request{Query: "falcon", Want: wants}, fleetLens{fusion.ResolveModeNL})
	var refusal *errs.ClassifiedError
	if !errors.As(err, &refusal) {
		t.Fatalf("NL falcon answered %+v (error %v), want a classified refusal", resp, err)
	}
	if refusal.Class != errs.ErrorInvalid || refusal.Code != graph.ErrorCodeNotEnabled ||
		!strings.Contains(refusal.Error(), "semantic is not enabled") {
		t.Fatalf("NL falcon refused %v (class %v, code %q), want invalid %q naming semantic",
			refusal, refusal.Class, refusal.Code, graph.ErrorCodeNotEnabled)
	}
	if !reflect.DeepEqual(resp, fusion.Response{}) {
		t.Fatalf("NL falcon returned %+v beside its refusal, want no response", resp)
	}
	// not_enabled is a refusal, not an error of graph-query's, so a word question leaves the stack
	// healthy.
	requireComponentsHealthy(t, s)

	stopStack(t, s)
}
