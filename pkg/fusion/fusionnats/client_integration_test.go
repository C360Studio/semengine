//go:build integration

package fusionnats_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/inference"
	"github.com/c360studio/semengine/internal/graphmutation"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/internal/harness/payloadfixture"
	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/fusion"
	"github.com/c360studio/semengine/pkg/fusion/fusionnats"
	"github.com/c360studio/semengine/pkg/lifecycle"
	graphindex "github.com/c360studio/semengine/processor/graph-index"
	graphingest "github.com/c360studio/semengine/processor/graph-ingest"
	graphlexical "github.com/c360studio/semengine/processor/graph-lexical"
	graphquery "github.com/c360studio/semengine/processor/graph-query"
	"github.com/c360studio/semengine/types"
	"github.com/c360studio/semengine/vocabulary"
)

// shared is this package's NATS server. It starts at the first reset; TestMain removes it after the tests.
var shared natsfixture.Shared

func TestMain(m *testing.M) { os.Exit(run(m)) }

func run(m *testing.M) int {
	code := m.Run()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := shared.Stop(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return code
}

const (
	requestTimeout = time.Minute

	// Entity IDs under the authority graph-ingest runs with below (c360, test).
	alpha  = "c360.test.fusion.alpha.doc.001"
	beta   = "c360.test.fusion.alpha.doc.002"
	omega  = "c360.test.fusion.omega.doc.003"
	absent = "c360.test.fusion.alpha.doc.999"
	poison = "c360.test.fusion.alpha.doc.009"

	// titlePredicate is a display name to graph-index (registered as a label below) and text to
	// graph-lexical (its ".title" suffix is one of the default text suffixes).
	titlePredicate = "test.doc.title"
	linkPredicate  = "test.link.to"
)

var testEntityType = message.Type{Domain: "test", Category: "fusion", Version: "v1"}

// stack is real graph-ingest, graph-index and graph-query on one connection to the shared server,
// each built with its factory; startWordIndex adds graph-lexical.
type stack struct {
	client    *natsclient.Client
	mutations *graphmutation.Client
}

func startStack(t *testing.T) stack {
	t.Helper()
	client := natsfixture.Open(t, shared.Reset(t), openClient)
	js, err := client.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := natsfixture.CreateStream(t.Context(), js, jetstream.FileStorage, "ENTITY", "entity.>"); err != nil {
		t.Fatalf("create ENTITY stream: %v", err)
	}
	// graph-index reads its label predicates once when it starts.
	vocabulary.Register(titlePredicate, vocabulary.WithAlias(vocabulary.AliasTypeLabel, 0))

	registry := payloadfixture.NewWithSubset(t, message.RegisterPayloads, inference.RegisterPayloads, lifecycle.RegisterPayloads)
	payloadfixture.RegisterTestType(t, registry, testEntityType)
	ingestConfig, err := json.Marshal(graphingest.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	ingest, err := graphingest.CreateGraphIngest(ingestConfig, component.Dependencies{
		NATSClient: client, PayloadRegistry: registry, MetricsRegistry: metric.NewMetricsRegistry(),
		Platform: types.PlatformMeta{Org: "c360", Platform: "test"},
	})
	if err != nil {
		t.Fatalf("CreateGraphIngest: %v", err)
	}
	startLifecycle(t, "graph-ingest", ingest)
	index, err := graphindex.CreateGraphIndex(json.RawMessage(`{}`), component.Dependencies{
		NATSClient: client, MetricsRegistry: metric.NewMetricsRegistry(),
	})
	if err != nil {
		t.Fatalf("CreateGraphIndex: %v", err)
	}
	startLifecycle(t, "graph-index", index)
	query, err := graphquery.CreateGraphQuery(json.RawMessage(`{}`), component.Dependencies{
		NATSClient: client, MetricsRegistry: metric.NewMetricsRegistry(),
	})
	if err != nil {
		t.Fatalf("CreateGraphQuery: %v", err)
	}
	startLifecycle(t, "graph-query", query)

	mutations, err := graphmutation.NewClient(client, requestTimeout)
	if err != nil {
		t.Fatal(err)
	}
	return stack{client: client, mutations: mutations}
}

// startWordIndex starts graph-lexical, the word index an NL resolve asks, on the stack's
// connection.
func (s stack) startWordIndex(t *testing.T) {
	t.Helper()
	lexical, err := graphlexical.CreateGraphLexical(json.RawMessage(`{}`), component.Dependencies{
		NATSClient: s.client, MetricsRegistry: metric.NewMetricsRegistry(),
	})
	if err != nil {
		t.Fatalf("CreateGraphLexical: %v", err)
	}
	startLifecycle(t, "graph-lexical", lexical)
}

func openClient(ctx context.Context, url string) (*natsclient.Client, func(context.Context) error, error) {
	client, err := natsclient.NewClient(url,
		natsclient.WithTimeout(15*time.Second),
		natsclient.WithMaxReconnects(0),
		natsclient.WithHealthInterval(0),
	)
	if err != nil {
		return nil, nil, err
	}
	if err := client.Connect(ctx); err != nil {
		return nil, nil, errors.Join(err, client.Close(ctx))
	}
	if err := client.WaitForConnection(ctx); err != nil {
		return nil, nil, errors.Join(err, client.Close(ctx))
	}
	return client, client.Close, nil
}

// startLifecycle starts built and stops it when the test ends; the components stop in reverse
// start order, graph-query first.
func startLifecycle(t *testing.T, name string, built component.Discoverable) {
	t.Helper()
	owner := built.(component.LifecycleComponent)
	if err := owner.Initialize(); err != nil {
		t.Fatalf("Initialize %s: %v", name, err)
	}
	if err := owner.Start(t.Context()); err != nil {
		t.Fatalf("start %s: %v", name, err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := owner.Stop(ctx); err != nil {
			t.Errorf("stop %s: %v", name, err)
		}
	})
}

func verbRow(t *testing.T, responder, name string) graph.QueryVerb {
	t.Helper()
	for _, verb := range graph.QueryVerbs() {
		if verb.Responder == responder && verb.Name == name {
			return verb
		}
	}
	t.Fatalf("the verb table declares no %s for %s", name, responder)
	return graph.QueryVerb{}
}

// create writes an entity through graph-ingest and returns its kv_revision. The call returns only
// after the write committed. Each statement is a predicate and an object.
func (s stack) create(t *testing.T, id string, statements ...[2]string) uint64 {
	t.Helper()
	now := time.Now()
	triples := []message.Triple{{
		Subject: id, Predicate: "core.identity.type", Object: "doc", Source: "test", Timestamp: now, Confidence: 1,
	}}
	for _, statement := range statements {
		triples = append(triples, message.Triple{
			Subject: id, Predicate: statement[0], Object: statement[1], Source: "test", Timestamp: now, Confidence: 1,
		})
	}
	created, err := s.mutations.Create(t.Context(), graph.CreateEntityRequest{
		Entity:  &graph.EntityState{ID: id, MessageType: testEntityType, UpdatedAt: now},
		Triples: triples,
	})
	if err != nil {
		t.Fatalf("create %s: %v", id, err)
	}
	return created.KVRevision
}

// seed writes alpha -> beta and omega -> beta, alpha and omega titled with "widget". It waits until
// graph-index has indexed the last write (alias read with min_revision; every index read after it
// reflects every earlier write) and, when graph-lexical runs, until its word index has too (a word
// search with min_revision answering both widgets is the event).
func (s stack) seed(t *testing.T, wordIndex bool) {
	t.Helper()
	s.create(t, beta, [2]string{titlePredicate, "Gadget Beta"})
	s.create(t, alpha, [2]string{titlePredicate, "Widget Alpha"}, [2]string{linkPredicate, beta})
	last := s.create(t, omega, [2]string{titlePredicate, "Widget Omega"}, [2]string{linkPredicate, beta})

	alias := verbRow(t, graph.ResponderGraphIndex, "alias").Subject
	request := fmt.Appendf(nil, `{"alias":"nobody-holds-this","min_revision":%d}`, last)
	if _, err := probe.Await(t.Context(), func(ctx context.Context) (bool, error) {
		_, err := s.client.RequestClassified(ctx, alias, request, requestTimeout)
		return err == nil, err
	}, func(reached bool) bool { return reached }); err != nil {
		t.Fatalf("revision %d indexed: %v", last, err)
	}

	if !wordIndex {
		return
	}
	search := verbRow(t, graph.ResponderGraphLexical, "search").Subject
	request = fmt.Appendf(nil, `{"query":"widget","min_revision":%d}`, last)
	if _, err := probe.Await(t.Context(), func(ctx context.Context) (int, error) {
		raw, err := s.client.RequestClassified(ctx, search, request, requestTimeout)
		if err != nil {
			return 0, err
		}
		var reply graph.QueryResponse[graph.LexicalSearchData]
		if err := json.Unmarshal(raw, &reply); err != nil {
			return 0, err
		}
		return len(reply.Data.Results), nil
	}, func(hits int) bool { return hits == 2 }); err != nil {
		t.Fatalf("both widgets in the word index: %v", err)
	}
}

func newClient(t *testing.T, s stack) *fusionnats.Client {
	t.Helper()
	c, err := fusionnats.New(s.client, requestTimeout)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func seedIDs(r fusion.Resolution) []string {
	out := []string{}
	for _, seed := range r.Seeds {
		out = append(out, seed.ID)
	}
	return out
}

func entityIDs(h fusion.Hydration) []string {
	out := []string{}
	for _, e := range h.Entities {
		out = append(out, e.ID)
	}
	return out
}

func hasTriple(e *fusion.Entity, predicate, object string) bool {
	for _, triple := range e.Triples {
		if triple.Predicate == predicate && triple.Object == object {
			return true
		}
	}
	return false
}

// TestRealRespondersAnswer drives every RetrievalClient method over real graph-ingest, graph-index,
// graph-lexical and graph-query: the subjects, the request shapes and each row's reply decode meet
// the responders this tree ships.
func TestRealRespondersAnswer(t *testing.T) {
	s := startStack(t)
	s.startWordIndex(t)
	s.seed(t, true)
	c := newClient(t, s)
	ctx := t.Context()

	symbol, err := c.Resolve(ctx, fusion.ResolveQuery{Mode: fusion.ResolveModeSymbol, Query: "Widget Alpha", Limit: 10})
	if err != nil {
		t.Fatalf("Resolve symbol: %v", err)
	}
	if got := seedIDs(symbol); !reflect.DeepEqual(got, []string{alpha}) {
		t.Errorf("Resolve symbol seeds = %v, want [%s]", got, alpha)
	}

	prefix, err := c.Resolve(ctx, fusion.ResolveQuery{Mode: fusion.ResolveModePrefix, Query: "c360.test.fusion.alpha", Limit: 10})
	if err != nil {
		t.Fatalf("Resolve prefix: %v", err)
	}
	if got := seedIDs(prefix); !reflect.DeepEqual(got, []string{alpha, beta}) {
		t.Errorf("Resolve prefix seeds = %v, want [%s %s]", got, alpha, beta)
	}

	// Unscoped, "widget" hits alpha and omega (seed's wait); the scope keeps alpha only.
	nl, err := c.Resolve(ctx, fusion.ResolveQuery{
		Mode: fusion.ResolveModeNL, Query: "widget", Scope: []string{"c360.test.fusion.alpha"}, Limit: 10,
	})
	if err != nil {
		t.Fatalf("Resolve NL: %v", err)
	}
	if got := seedIDs(nl); !reflect.DeepEqual(got, []string{alpha}) {
		t.Errorf("Resolve NL scoped seeds = %v, want [%s]", got, alpha)
	}
	if len(nl.Seeds) == 1 && (!nl.Seeds[0].HasSimilarity || nl.Seeds[0].Similarity <= 0) {
		t.Errorf("Resolve NL seed = %+v, want a positive score", nl.Seeds[0])
	}
	// The question reaches the word index unchanged, so its query rules apply: name: is reserved.
	_, err = c.Resolve(ctx, fusion.ResolveQuery{Mode: fusion.ResolveModeNL, Query: "name:widget", Limit: 10})
	var refusal *errs.ClassifiedError
	if !errors.As(err, &refusal) || refusal.Class != errs.ErrorInvalid || refusal.Code != graph.ErrorCodeInvalidRequest {
		t.Errorf("Resolve NL name:widget error = %v, want the word index's invalid %s", err, graph.ErrorCodeInvalidRequest)
	}

	entity, err := c.Entity(ctx, alpha)
	if err != nil {
		t.Fatalf("Entity: %v", err)
	}
	if entity == nil || entity.ID != alpha || !hasTriple(entity, titlePredicate, "Widget Alpha") ||
		!hasTriple(entity, linkPredicate, beta) {
		t.Errorf("Entity(%s) = %+v, want its title and link triples", alpha, entity)
	}
	missing, err := c.Entity(ctx, absent)
	if err != nil || missing != nil {
		t.Errorf("Entity(%s) = %+v, %v; want nil, nil", absent, missing, err)
	}

	hydration, err := c.Entities(ctx, []string{omega, absent, alpha})
	if err != nil {
		t.Fatalf("Entities: %v", err)
	}
	if got := entityIDs(hydration); !reflect.DeepEqual(got, []string{omega, alpha}) {
		t.Errorf("Entities = %v, want [%s %s] in request order", got, omega, alpha)
	}
	wantUnhydrated := []fusion.Unhydrated{{Handle: absent, Reason: "not_found"}}
	if !reflect.DeepEqual(hydration.Unhydrated, wantUnhydrated) {
		t.Errorf("Unhydrated = %+v, want %+v", hydration.Unhydrated, wantUnhydrated)
	}

	out, err := c.Neighbors(ctx, alpha, nil, fusion.Outgoing)
	if err != nil {
		t.Fatalf("Neighbors outgoing: %v", err)
	}
	if want := []fusion.Edge{{Predicate: linkPredicate, Target: beta}}; !reflect.DeepEqual(out, want) {
		t.Errorf("Neighbors(%s, outgoing) = %+v, want %+v", alpha, out, want)
	}
	in, err := c.Neighbors(ctx, beta, []string{linkPredicate}, fusion.Incoming)
	if err != nil {
		t.Fatalf("Neighbors incoming: %v", err)
	}
	if want := []fusion.Edge{{Predicate: linkPredicate, Target: alpha}, {Predicate: linkPredicate, Target: omega}}; !reflect.DeepEqual(in, want) {
		t.Errorf("Neighbors(%s, incoming) = %+v, want %+v", beta, in, want)
	}

	names, err := c.Names(ctx, "Widget Omega", 5)
	if err != nil {
		t.Fatalf("Names: %v", err)
	}
	if !reflect.DeepEqual(names, []string{"Widget Omega"}) {
		t.Errorf("Names = %v, want [Widget Omega]", names)
	}
}

// TestNLResolveWithoutWordIndex: with no graph-lexical composed nothing answers the word search,
// and an NL Fuse fails transient internal naming the subject, never an empty answer.
func TestNLResolveWithoutWordIndex(t *testing.T) {
	s := startStack(t)
	s.seed(t, false)
	eng, err := fusion.NewEngine(newClient(t, s), fusion.NewBodyResolver(fusion.MapStoreResolver{}), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	search := verbRow(t, graph.ResponderGraphLexical, "search").Subject

	resp, err := eng.Fuse(t.Context(), fusion.Request{Query: "widget"}, nlLens{})
	var refusal *errs.ClassifiedError
	if !errors.As(err, &refusal) {
		t.Fatalf("NL widget answered %+v (error %v), want a classified error", resp, err)
	}
	if refusal.Class != errs.ErrorTransient || refusal.Code != graph.ErrorCodeInternal ||
		!strings.Contains(refusal.Error(), search) {
		t.Fatalf("NL widget refused %v (class %v, code %q), want transient %q naming %s",
			refusal, refusal.Class, refusal.Code, graph.ErrorCodeInternal, search)
	}
	if !natsclient.IsNoResponders(err) {
		t.Errorf("NL widget error %v does not keep the no-responders cause", err)
	}
	if !reflect.DeepEqual(resp, fusion.Response{}) {
		t.Errorf("NL widget returned %+v beside its error, want no response", resp)
	}
}

// TestPoisonedSeedOverRealResponders: bytes the stored-value rule refuses, Put under one key after
// graph-ingest's Start returned, are found by the read path, not the boot sweep. graph-ingest
// refuses the whole batch for them; Entities lists the poisoned seed unhydrated error and hydrates
// the other two in request order (fusion spec, "One poisoned seed").
func TestPoisonedSeedOverRealResponders(t *testing.T) {
	s := startStack(t)
	s.create(t, beta, [2]string{titlePredicate, "Gadget Beta"})
	s.create(t, alpha, [2]string{titlePredicate, "Widget Alpha"})

	js, err := s.client.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	kv, err := js.KeyValue(t.Context(), graph.BucketEntityStates)
	if err != nil {
		t.Fatal(err)
	}
	// A two-part predicate fails the stored-value decode. Put returns once the value is stored, so
	// every read after it sees the poison.
	if _, err := kv.Put(t.Context(), poison,
		[]byte(`{"id":"`+poison+`","triples":[{"subject":"`+poison+`","predicate":"legacy.predicate","object":1}]}`)); err != nil { // predicate-audit:invalid {"kind":"stored-predicate","value":"legacy.predicate","reason":"arity"}
		t.Fatalf("put poison: %v", err)
	}

	// The precondition: graph-query's batch, as graph-ingest answers it, is refused whole.
	batch := verbRow(t, graph.ResponderGraphQuery, "batch").Subject
	raw, err := s.client.RequestClassified(t.Context(), batch,
		[]byte(`{"ids":["`+alpha+`","`+poison+`","`+beta+`"]}`), requestTimeout)
	var refusal *errs.ClassifiedError
	if !errors.As(err, &refusal) || refusal.Code != "graph_state_reset_required" {
		t.Fatalf("batch with poison = %s, %v; want a graph_state_reset_required refusal", raw, err)
	}

	hydration, err := newClient(t, s).Entities(t.Context(), []string{alpha, poison, beta})
	if err != nil {
		t.Fatalf("Entities: %v", err)
	}
	if got := entityIDs(hydration); !reflect.DeepEqual(got, []string{alpha, beta}) {
		t.Errorf("Entities = %v, want [%s %s] in request order", got, alpha, beta)
	}
	want := []fusion.Unhydrated{{Handle: poison, Reason: "error"}}
	if !reflect.DeepEqual(hydration.Unhydrated, want) {
		t.Errorf("Unhydrated = %+v, want %+v", hydration.Unhydrated, want)
	}
}

// TestDamagedEntityUnderPrefixOverRealResponders: a damaged value under a prefix, Put after
// graph-ingest's Start returned, makes graph-query's full-entity prefix page refuse, yet Fuse with a
// prefix lens answers the two good entities and lists the damaged one unhydrated error (fusion spec,
// "A prefix resolve lists IDs, then reads them").
func TestDamagedEntityUnderPrefixOverRealResponders(t *testing.T) {
	const prefix = "c360.test.fusion.alpha"
	s := startStack(t)
	s.create(t, alpha, [2]string{titlePredicate, "Widget Alpha"})
	s.create(t, beta, [2]string{titlePredicate, "Gadget Beta"})

	js, err := s.client.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	kv, err := js.KeyValue(t.Context(), graph.BucketEntityStates)
	if err != nil {
		t.Fatal(err)
	}
	// A two-part predicate fails the stored-value decode. Put returns once the value is stored, so
	// every read after it sees the damage.
	if _, err := kv.Put(t.Context(), poison,
		[]byte(`{"id":"`+poison+`","triples":[{"subject":"`+poison+`","predicate":"legacy.predicate","object":1}]}`)); err != nil { // predicate-audit:invalid {"kind":"stored-predicate","value":"legacy.predicate","reason":"arity"}
		t.Fatalf("put poison: %v", err)
	}

	// The precondition: graph-query's full-entity prefix page, as graph-ingest answers it, is
	// refused whole.
	prefixSubject := verbRow(t, graph.ResponderGraphQuery, "prefix").Subject
	raw, err := s.client.RequestClassified(t.Context(), prefixSubject, []byte(`{"prefix":"`+prefix+`"}`), requestTimeout)
	var refusal *errs.ClassifiedError
	if !errors.As(err, &refusal) || refusal.Code != "graph_state_reset_required" {
		t.Fatalf("prefix with a damaged entity = %s, %v; want a graph_state_reset_required refusal", raw, err)
	}

	eng, err := fusion.NewEngine(newClient(t, s), fusion.NewBodyResolver(fusion.MapStoreResolver{}), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	resp, err := eng.Fuse(t.Context(), fusion.Request{Query: prefix}, idPrefixLens{})
	if err != nil {
		t.Fatalf("Fuse: %v", err)
	}
	var names []string
	for _, node := range resp.Nodes {
		names = append(names, node.Name)
	}
	slices.Sort(names)
	if want := []string{alpha, beta}; !reflect.DeepEqual(names, want) {
		t.Errorf("nodes = %v, want %v", names, want)
	}
	want := []fusion.Unhydrated{{Handle: poison, Reason: fusion.UnhydratedError}}
	if !reflect.DeepEqual(resp.Unhydrated, want) {
		t.Errorf("Unhydrated = %+v, want %+v", resp.Unhydrated, want)
	}
}

// nlLens resolves every query as natural language, walks no edges and names each node by its
// entity ID.
type nlLens struct{}

func (nlLens) Name() string                          { return "nl" }
func (nlLens) ResolveMode(string) fusion.ResolveMode { return fusion.ResolveModeNL }
func (nlLens) Edges() []fusion.EdgeSpec              { return nil }
func (nlLens) Label(e *fusion.Entity) string         { return e.ID }
func (nlLens) Kind(*fusion.Entity) string            { return "entity" }
func (nlLens) Location(*fusion.Entity) fusion.Locator {
	return fusion.Locator{}
}
func (nlLens) Hydrate(context.Context, *fusion.Entity) (*message.StorageReference, error) {
	return nil, nil
}

// idPrefixLens resolves every query as an ID prefix, walks no edges and names each node by its
// entity ID, so a Fuse call reads only the prefix listing and the seeds.
type idPrefixLens struct{}

func (idPrefixLens) Name() string                          { return "id-prefix" }
func (idPrefixLens) ResolveMode(string) fusion.ResolveMode { return fusion.ResolveModePrefix }
func (idPrefixLens) Edges() []fusion.EdgeSpec              { return nil }
func (idPrefixLens) Label(e *fusion.Entity) string         { return e.ID }
func (idPrefixLens) Kind(*fusion.Entity) string            { return "entity" }
func (idPrefixLens) Location(*fusion.Entity) fusion.Locator {
	return fusion.Locator{}
}
func (idPrefixLens) Hydrate(context.Context, *fusion.Entity) (*message.StorageReference, error) {
	return nil, nil
}
