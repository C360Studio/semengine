//go:build integration

package graphgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
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
	"github.com/c360studio/semengine/pkg/lifecycle"
	graphindex "github.com/c360studio/semengine/processor/graph-index"
	graphingest "github.com/c360studio/semengine/processor/graph-ingest"
	graphquery "github.com/c360studio/semengine/processor/graph-query"
	"github.com/c360studio/semengine/types"
)

// shared is this package's NATS server. It starts at the first reset; TestMain removes it after the tests.
var shared natsfixture.Shared

func TestMain(m *testing.M) { os.Exit(runIntegration(m)) }

func runIntegration(m *testing.M) int {
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
	// Valid entity IDs under the authority graph-ingest runs with below.
	droneA = "c360.test.gateway.system.drone.001"
	droneB = "c360.test.gateway.system.drone.002"
	droneC = "c360.test.gateway.system.drone.003"
	// No entity has this ID.
	absentDrone = "c360.test.gateway.system.drone.999"
	linkTo      = "test.link.to"
	// requestTimeout bounds the test's own requests to the stack, which must answer.
	requestTimeout = time.Minute
)

var testEntityType = message.Type{Domain: "test", Category: "gateway", Version: "v1"}

func fixtureClient(t *testing.T) *natsclient.Client {
	t.Helper()
	return natsfixture.Open(t, shared.Reset(t), openClient)
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

// verb returns the verb table's row for responder's name.
func verb(t *testing.T, responder, name string) graph.QueryVerb {
	t.Helper()
	for _, v := range graph.QueryVerbs() {
		if v.Responder == responder && v.Name == name {
			return v
		}
	}
	t.Fatalf("the verb table declares no %s for %s", name, responder)
	return graph.QueryVerb{}
}

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

// gateway is a started graph-gateway and the route the service manager would mount for it.
type gateway struct {
	c   *Component
	mux *http.ServeMux
}

// startGateway builds graph-gateway with config through its factory, starts it on client, mounts
// its route at /graph-gateway and stops it when the test ends. A held stub's release, if the test
// registers it after this call, runs before the Stop.
func startGateway(t *testing.T, client *natsclient.Client, config string) gateway {
	t.Helper()
	built, err := CreateGraphGateway(json.RawMessage(config), component.Dependencies{NATSClient: client})
	if err != nil {
		t.Fatalf("CreateGraphGateway: %v", err)
	}
	startLifecycle(t, key, built)
	mux := http.NewServeMux()
	built.(*Component).RegisterHTTPHandlers("/graph-gateway", mux)
	return gateway{c: built.(*Component), mux: mux}
}

// answer is a decoded GraphQL answer.
type answer struct {
	status int
	Data   map[string]any `json:"data"`
	Errors []struct {
		Message    string         `json:"message"`
		Path       []any          `json:"path"`
		Extensions map[string]any `json:"extensions"`
	} `json:"errors"`
	raw string
}

// post sends one GraphQL request through the mounted route.
func (g gateway) post(t *testing.T, query string, vars map[string]any) answer {
	t.Helper()
	return g.postContext(t, t.Context(), query, vars)
}

func (g gateway) postContext(t *testing.T, ctx context.Context, query string, vars map[string]any) answer {
	t.Helper()
	return decodeAnswer(t, g.send(t, ctx, query, vars))
}

// send sends one GraphQL request through the mounted route and returns the answer as written.
func (g gateway) send(t *testing.T, ctx context.Context, query string, vars map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	g.mux.ServeHTTP(rec, httptest.NewRequestWithContext(ctx, http.MethodPost, "/graph-gateway/graphql", bytes.NewReader(body)))
	return rec
}

func decodeAnswer(t *testing.T, rec *httptest.ResponseRecorder) answer {
	t.Helper()
	got := answer{status: rec.Code, raw: rec.Body.String()}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("answer %d %q is not JSON: %v", rec.Code, rec.Body, err)
	}
	return got
}

// requireData fails unless the answer is HTTP 200 with no errors and data equal to the literal JSON want.
func requireData(t *testing.T, got answer, want string) {
	t.Helper()
	if got.status != http.StatusOK || len(got.Errors) != 0 {
		t.Fatalf("answer %d %s, want 200 with no errors", got.status, got.raw)
	}
	var expected map[string]any
	if err := json.Unmarshal([]byte(want), &expected); err != nil {
		t.Fatalf("want %s: %v", want, err)
	}
	if !reflect.DeepEqual(got.Data, expected) {
		t.Errorf("data\n got %s\nwant %s", got.raw, want)
	}
}

// requireNullField fails unless the answer is HTTP status with data.field null and one error at
// [field] whose message holds text.
func requireNullField(t *testing.T, got answer, status int, field, text string) {
	t.Helper()
	if got.status != status {
		t.Fatalf("status %d (%s), want %d", got.status, got.raw, status)
	}
	if value, ok := got.Data[field]; !ok || value != nil || len(got.Data) != 1 {
		t.Errorf("data = %v, want only %s: null", got.Data, field)
	}
	if len(got.Errors) != 1 || !reflect.DeepEqual(got.Errors[0].Path, []any{field}) ||
		!strings.Contains(got.Errors[0].Message, text) {
		t.Fatalf("errors %s, want one at [%s] holding %q", got.raw, field, text)
	}
}

// stack is real graph-ingest, graph-index and graph-query behind the gateway, on one connection.
type stack struct {
	gateway
	client    *natsclient.Client
	mutations *graphmutation.Client
}

func startStack(t *testing.T) stack {
	t.Helper()
	client := fixtureClient(t)
	js, err := client.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := natsfixture.CreateStream(t.Context(), js, jetstream.FileStorage, "ENTITY", "entity.>"); err != nil {
		t.Fatalf("create ENTITY stream: %v", err)
	}
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
	return stack{gateway: startGateway(t, client, `{}`), client: client, mutations: mutations}
}

// create writes an entity through graph-ingest and returns its kv_revision. Each statement is a
// predicate and an object; an object that is an entity ID is an edge.
func (s stack) create(t *testing.T, id string, statements ...[2]string) uint64 {
	t.Helper()
	now := time.Now()
	triples := []message.Triple{{
		Subject: id, Predicate: "core.identity.type", Object: "drone", Source: "test", Timestamp: now, Confidence: 1,
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

// awaitIndexed waits until a direct read of graph-index reports revision reached: every later read
// of the index reflects that write.
func (s stack) awaitIndexed(t *testing.T, revision uint64) {
	t.Helper()
	request := fmt.Appendf(nil, `{"alias":"nobody-holds-this","min_revision":%d}`, revision)
	alias := verb(t, graph.ResponderGraphIndex, "alias").Subject
	if _, err := probe.Await(t.Context(), func(ctx context.Context) (bool, error) {
		_, err := s.client.RequestClassified(ctx, alias, request, requestTimeout)
		return err == nil, nil
	}, func(reached bool) bool { return reached }); err != nil {
		t.Fatalf("revision %d indexed: %v", revision, err)
	}
}

// sortTriples orders each entity's triples by predicate in place, so an answer compares with a
// literal whatever order graph-ingest stores them in.
func sortTriples(entities any) {
	for _, entity := range entities.([]any) {
		triples := entity.(map[string]any)["triples"].([]any)
		slices.SortFunc(triples, func(a, b any) int {
			return strings.Compare(a.(map[string]any)["predicate"].(string), b.(map[string]any)["predicate"].(string))
		})
	}
}

// TestGatewayAnswersFourOperations: on a real stack, each root field in the form semsource's UI and
// smoke assertions will send once they move to this gateway holds the seeded IDs and triples, and a
// root field name inside an argument routes nowhere but the field asked.
func TestGatewayAnswersFourOperations(t *testing.T) {
	s := startStack(t)
	revisionA := s.create(t, droneA, [2]string{linkTo, droneB})
	s.create(t, droneB, [2]string{linkTo, droneC})
	s.awaitIndexed(t, s.create(t, droneC))

	t.Run("entity", func(t *testing.T) {
		got := s.post(t, `query($id: String!) { entity(id: $id) { entity { id triples { predicate object } } kvRevision } }`,
			map[string]any{"id": droneA})
		if got.status == http.StatusOK && got.Data["entity"] != nil {
			sortTriples([]any{got.Data["entity"].(map[string]any)["entity"]})
		}
		requireData(t, got, fmt.Sprintf(`{"entity":{"entity":{"id":%q,"triples":[`+
			`{"predicate":"core.identity.type","object":"drone"},{"predicate":"entity.indexing.profile","object":"control"},`+
			`{"predicate":%q,"object":%q}]},"kvRevision":%d}}`,
			droneA, linkTo, droneB, revisionA))
	})

	t.Run("entitiesByPrefix", func(t *testing.T) {
		for _, prefix := range []string{"c360.test.gateway", ""} {
			got := s.post(t, `query($p: String!) { entitiesByPrefix(prefix: $p, limit: 2) { entities { id triples { predicate } } total next_cursor } }`,
				map[string]any{"p": prefix})
			if got.status == http.StatusOK && got.Data["entitiesByPrefix"] != nil {
				page := got.Data["entitiesByPrefix"].(map[string]any)
				sortTriples(page["entities"])
				if cursor, ok := page["next_cursor"].(string); !ok || cursor == "" {
					t.Errorf("prefix %q: next_cursor = %v, want a cursor after 2 of 3", prefix, page["next_cursor"])
				}
				page["next_cursor"] = "set"
			}
			requireData(t, got, fmt.Sprintf(`{"entitiesByPrefix":{"entities":[`+
				`{"id":%q,"triples":[{"predicate":"core.identity.type"},{"predicate":"entity.indexing.profile"},{"predicate":%q}]},`+
				`{"id":%q,"triples":[{"predicate":"core.identity.type"},{"predicate":"entity.indexing.profile"},{"predicate":%q}]}],"total":3,"next_cursor":"set"}}`,
				droneA, linkTo, droneB, linkTo))
		}
	})

	t.Run("entitiesByPredicate", func(t *testing.T) {
		requireData(t, s.post(t, `{ entitiesByPredicate(predicate: "test.link.to") { entities } }`, nil),
			fmt.Sprintf(`{"entitiesByPredicate":{"entities":[%q,%q]}}`, droneA, droneB))
		requireData(t, s.post(t, `query($v: String) { entitiesByPredicate(predicate: "test.link.to", value: $v) { entities } }`,
			map[string]any{"v": droneC}), fmt.Sprintf(`{"entitiesByPredicate":{"entities":[%q]}}`, droneB))
		requireData(t, s.post(t, `{ entitiesByPredicate(predicate: "test.link.to", value: "nothing") { entities } }`, nil),
			`{"entitiesByPredicate":{"entities":[]}}`)
	})

	t.Run("pathSearch", func(t *testing.T) {
		got := s.post(t, `query($start: String!, $predicates: [String]) {
			pathSearch(startEntity: $start, maxDepth: 2, direction: "OUTGOING", predicates: $predicates) {
				entities { id } paths { from predicate to } truncated } }`,
			map[string]any{"start": droneA, "predicates": []any{linkTo}})
		requireData(t, got, fmt.Sprintf(`{"pathSearch":{"entities":[{"id":%[1]q},{"id":%[2]q},{"id":%[3]q}],`+
			`"paths":[[],[{"from":%[1]q,"predicate":%[4]q,"to":%[2]q}],`+
			`[{"from":%[1]q,"predicate":%[4]q,"to":%[2]q},{"from":%[2]q,"predicate":%[4]q,"to":%[3]q}]],"truncated":false}}`,
			droneA, droneB, droneC, linkTo))
	})

	t.Run("a field name inside an argument", func(t *testing.T) {
		got := s.post(t, `{ entity(id: "x.y.pathsearch.trajectory.a.b") { kvRevision } }`, nil)
		requireNullField(t, got, http.StatusOK, "entity", "x.y.pathsearch.trajectory.a.b")
		if code := got.Errors[0].Extensions["code"]; code != graph.ErrorCodeEntityNotFound {
			t.Errorf("code = %v, want the entity verb's %s", code, graph.ErrorCodeEntityNotFound)
		}
	})
}

// TestPathsFollowEdgesBack: paths[k] leads from the start to entities[k] and paths[0] is empty, each
// step naming the edge as the graph holds it, whichever way the walk crossed it.
func TestPathsFollowEdgesBack(t *testing.T) {
	s := startStack(t)
	// C -to-> B -to-> A: from A, incoming, the walk crosses each edge against its direction.
	s.create(t, droneA)
	s.create(t, droneB, [2]string{linkTo, droneA})
	s.awaitIndexed(t, s.create(t, droneC, [2]string{linkTo, droneB}))
	query := `query($d: Int!) { pathSearch(startEntity: "` + droneA + `", direction: "INCOMING", maxDepth: $d) {
		entities { id depth } paths { from predicate to } } }`

	requireData(t, s.post(t, query, map[string]any{"d": 1}), fmt.Sprintf(`{"pathSearch":{`+
		`"entities":[{"id":%[1]q,"depth":0},{"id":%[2]q,"depth":1}],`+
		`"paths":[[],[{"from":%[2]q,"predicate":%[3]q,"to":%[1]q}]]}}`, droneA, droneB, linkTo))
	requireData(t, s.post(t, query, map[string]any{"d": 2}), fmt.Sprintf(`{"pathSearch":{`+
		`"entities":[{"id":%[1]q,"depth":0},{"id":%[2]q,"depth":1},{"id":%[4]q,"depth":2}],`+
		`"paths":[[],[{"from":%[2]q,"predicate":%[3]q,"to":%[1]q}],`+
		`[{"from":%[2]q,"predicate":%[3]q,"to":%[1]q},{"from":%[4]q,"predicate":%[3]q,"to":%[2]q}]]}}`,
		droneA, droneB, linkTo, droneC))
}

// TestPathReplyBreakingAlignmentRefused: a pathSearch reply whose edges cannot be followed back from
// each entity to the start (graph/query_path_types.go) is data.pathSearch null with an error at its
// path, HTTP 200, never paths answered in part.
func TestPathReplyBreakingAlignmentRefused(t *testing.T) {
	client := fixtureClient(t)
	g := startGateway(t, client, `{}`)
	edge := func(from, to string) string {
		return fmt.Sprintf(`{"predicate":%q,"from_entity_id":%q,"to_entity_id":%q}`, linkTo, from, to)
	}
	entities := func(ids ...string) string {
		out := make([]string, len(ids))
		for i, id := range ids {
			out[i] = fmt.Sprintf(`{"entity_id":%q,"depth":%d}`, id, min(i, 1))
		}
		return "[" + strings.Join(out, ",") + "]"
	}
	replies := map[string]string{
		"no start":            `{"entities":[],"edges":[],"truncated":false}`,
		"an edge short":       `{"entities":` + entities(droneA, droneB) + `,"edges":[],"truncated":false}`,
		"an edge elsewhere":   `{"entities":` + entities(droneA, droneB) + `,"edges":[` + edge(droneA, droneC) + `],"truncated":false}`,
		"from a later entity": `{"entities":` + entities(droneA, droneB, droneC) + `,"edges":[` + edge(droneC, droneB) + `,` + edge(droneA, droneC) + `],"truncated":false}`,
		"an entity twice":     `{"entities":` + entities(droneA, droneB, droneB) + `,"edges":[` + edge(droneA, droneB) + `,` + edge(droneA, droneB) + `],"truncated":false}`,
	}
	stub(t, client, verb(t, graph.ResponderGraphQuery, "pathSearch").Subject, func(_ context.Context, data []byte) ([]byte, error) {
		var request graph.PathSearchRequest
		if err := json.Unmarshal(data, &request); err != nil {
			return nil, err
		}
		return []byte(`{"data":` + replies[request.Predicates[0]] + `,"indexed_revision":1,"producer":"graph-query",` +
			`"timestamp":"2026-11-10T00:00:00Z"}`), nil
	})
	because := map[string]string{
		"no start": "no start entity", "an edge short": "0 edges for 2 entities", "an edge elsewhere": "does not reach",
		"from a later entity": "which no earlier entity is", "an entity twice": "listed twice",
	}
	for name := range replies {
		t.Run(name, func(t *testing.T) {
			got := g.post(t, `query($p: [String]) { pathSearch(startEntity: "`+droneA+`", direction: "outgoing", maxDepth: 2, predicates: $p) {
				entities { id } paths { from } } }`, map[string]any{"p": []any{name}})
			requireNullField(t, got, http.StatusOK, "pathSearch", because[name])
		})
	}
}

// TestDirectionIsCaseInsensitive: direction in any letter case answers as lower case does.
func TestDirectionIsCaseInsensitive(t *testing.T) {
	s := startStack(t)
	s.create(t, droneA)
	s.awaitIndexed(t, s.create(t, droneB, [2]string{linkTo, droneA}))
	want := fmt.Sprintf(`{"pathSearch":{"entities":[{"id":%q},{"id":%q}]}}`, droneA, droneB)
	for _, direction := range []string{"incoming", "INCOMING", "Incoming", "iNcOmInG", "both", "BOTH"} {
		t.Run(direction, func(t *testing.T) {
			requireData(t, s.post(t, `query($dir: String!) { pathSearch(startEntity: "`+droneA+`", direction: $dir, maxDepth: 1) {
				entities { id } } }`, map[string]any{"dir": direction}), want)
		})
	}
	// An outgoing walk from A finds nothing: the case changes the word, not the direction.
	requireData(t, s.post(t, `{ pathSearch(startEntity: "`+droneA+`", direction: "OUTGOING", maxDepth: 1) { entities { id } } }`, nil),
		fmt.Sprintf(`{"pathSearch":{"entities":[{"id":%q}]}}`, droneA))
}

// TestPredicatesRefuseANullItem: a null item in predicates is HTTP 400 naming the argument, and no
// request reaches NATS; a list without one is answered.
func TestPredicatesRefuseANullItem(t *testing.T) {
	client := fixtureClient(t)
	g := startGateway(t, client, `{}`)
	var asked atomic.Int32
	stub(t, client, verb(t, graph.ResponderGraphQuery, "pathSearch").Subject, func(context.Context, []byte) ([]byte, error) {
		asked.Add(1)
		return []byte(`{"data":{"entities":[{"entity_id":"` + droneA + `","depth":0}],"edges":[],"truncated":false},` +
			`"indexed_revision":1,"producer":"graph-query","timestamp":"2026-11-10T00:00:00Z"}`), nil
	})
	query := `query($p: [String]) { pathSearch(startEntity: "` + droneA + `", direction: "outgoing", maxDepth: 1, predicates: $p) { truncated } }`
	for _, predicates := range [][]any{{nil}, {linkTo, nil}} {
		got := g.post(t, query, map[string]any{"p": predicates})
		if got.status != http.StatusBadRequest || got.Data != nil || len(got.Errors) != 1 ||
			!strings.Contains(got.Errors[0].Message, "predicates") {
			t.Errorf("predicates %v: %d %s, want 400 naming predicates, no data", predicates, got.status, got.raw)
		}
	}
	requireData(t, g.post(t, query, map[string]any{"p": []any{linkTo}}), `{"pathSearch":{"truncated":false}}`)
	// Read after the answered request returned, which its one stub call preceded.
	if n := asked.Load(); n != 1 {
		t.Errorf("the stub was asked %d times, want once", n)
	}
}

// stub stands in for a graph.query.* verb. Its subscription ends with the client's Close.
func stub(t *testing.T, client *natsclient.Client, subject string, handler func(context.Context, []byte) ([]byte, error)) {
	t.Helper()
	if _, err := client.SubscribeForRequests(t.Context(), subject, handler); err != nil {
		t.Fatalf("subscribe %s: %v", subject, err)
	}
}

// storedFor is a stored value graph.UnmarshalEntityState accepts under the key id.
func storedFor(id string) string {
	return `{"id":"` + id + `","triples":[{"subject":"` + id + `","predicate":"core.identity.type",` +
		`"object":"drone","source":"test","timestamp":"2026-11-10T00:00:00Z","confidence":1}],` +
		`"message_type":{"domain":"test","category":"gateway","version":"v1"},"updated_at":"2026-11-10T00:00:00Z"}`
}

// storedEntity is droneA's stored value.
var storedEntity = storedFor(droneA)

// TestEntityZeroRevisionRefused: an entity reply with kv_revision 0, with no entity, or whose stored
// value graph.UnmarshalEntityState refuses (another entity's) leaves data.entity null with an error
// at its path, HTTP 200; each case's value is the one stored under the ID asked, so only the case's
// own fault can refuse it, and the same value at revision 3 is answered.
func TestEntityZeroRevisionRefused(t *testing.T) {
	client := fixtureClient(t)
	g := startGateway(t, client, `{}`)
	id := func(instance string) string { return "c360.test.gateway.system.drone." + instance }
	replies := map[string]string{
		id("010"): `{"entity":` + storedFor(id("010")) + `,"kv_revision":0}`,
		id("011"): `{"kv_revision":3}`,
		id("012"): `{"entity":null,"kv_revision":3}`,
		id("013"): `{"entity":` + storedFor(id("002")) + `,"kv_revision":3}`,
		id("014"): `{"entity":` + storedFor(id("014")) + `,"kv_revision":3}`,
	}
	stub(t, client, verb(t, graph.ResponderGraphQuery, "entity").Subject, func(_ context.Context, data []byte) ([]byte, error) {
		var request graph.EntityRequest
		if err := json.Unmarshal(data, &request); err != nil {
			return nil, err
		}
		return []byte(replies[request.ID]), nil
	})
	ask := func(instance string) answer {
		return g.post(t, `query($id: String!) { entity(id: $id) { kvRevision entity { id } } }`, map[string]any{"id": id(instance)})
	}
	for instance, text := range map[string]string{"010": "zero kv_revision", "011": "no entity", "012": "no entity"} {
		t.Run(instance, func(t *testing.T) { requireNullField(t, ask(instance), http.StatusOK, "entity", text) })
	}
	t.Run("another entity", func(t *testing.T) {
		got := ask("013")
		requireNullField(t, got, http.StatusOK, "entity", "refused")
		if code := got.Errors[0].Extensions["code"]; code != graph.ErrorCodeGraphStateResetRequired {
			t.Errorf("code = %v, want %s from graph.UnmarshalEntityState", code, graph.ErrorCodeGraphStateResetRequired)
		}
	})
	requireData(t, ask("014"), `{"entity":{"kvRevision":3,"entity":{"id":"`+id("014")+`"}}}`)
}

// TestPrefixReplyValidated: a prefix page holding an entity graph.ValidateDecodedEntityStates refuses
// (a triple with no source) is data.entitiesByPrefix null with an error at its path, HTTP 200; the
// page without it is answered.
func TestPrefixReplyValidated(t *testing.T) {
	client := fixtureClient(t)
	g := startGateway(t, client, `{}`)
	bad := strings.Replace(storedEntity, `"source":"test",`, ``, 1)
	stub(t, client, verb(t, graph.ResponderGraphQuery, "prefix").Subject, func(_ context.Context, data []byte) ([]byte, error) {
		var request graph.PrefixQueryRequest
		if err := json.Unmarshal(data, &request); err != nil {
			return nil, err
		}
		if request.Prefix == "bad" {
			return []byte(`{"entities":[` + storedEntity + `,` + bad + `],"total":2}`), nil
		}
		return []byte(`{"entities":[` + storedEntity + `],"total":1}`), nil
	})
	query := `query($p: String!) { entitiesByPrefix(prefix: $p) { entities { id } total next_cursor } }`
	requireNullField(t, g.post(t, query, map[string]any{"p": "bad"}), http.StatusOK, "entitiesByPrefix", "entity[1]")
	requireData(t, g.post(t, query, map[string]any{"p": "good"}),
		`{"entitiesByPrefix":{"entities":[{"id":"`+droneA+`"}],"total":1,"next_cursor":null}}`)
}

// TestClassifiedRefusalCarriesClassAndCode: an entity with no record is HTTP 200, data.entity null,
// and errors[0].extensions carries graph-ingest's class and code through graph-query (ADR-060).
func TestClassifiedRefusalCarriesClassAndCode(t *testing.T) {
	s := startStack(t)
	got := s.post(t, `{ entity(id: "`+absentDrone+`") { kvRevision } }`, nil)
	requireNullField(t, got, http.StatusOK, "entity", absentDrone)
	want := map[string]any{"class": "invalid", "code": graph.ErrorCodeEntityNotFound}
	if !reflect.DeepEqual(got.Errors[0].Extensions, want) {
		t.Errorf("extensions = %v, want %v", got.Errors[0].Extensions, want)
	}
}

// TestNoResponderNamesSubject: with nothing serving graph.query.*, each root field answers HTTP 500
// with a transient error naming its subject.
func TestNoResponderNamesSubject(t *testing.T) {
	g := startGateway(t, fixtureClient(t), `{}`)
	for field, query := range fieldQueries() {
		t.Run(field, func(t *testing.T) {
			got := g.post(t, query, nil)
			requireNullField(t, got, http.StatusInternalServerError, field, verb(t, graph.ResponderGraphQuery, rootVerbs[field]).Subject)
			if class := got.Errors[0].Extensions["class"]; class != "transient" {
				t.Errorf("class = %v, want transient", class)
			}
		})
	}
}

// fieldQueries is one valid request per root field.
func fieldQueries() map[string]string {
	return map[string]string{
		"entity":              `{ entity(id: "` + droneA + `") { kvRevision } }`,
		"entitiesByPrefix":    `{ entitiesByPrefix(prefix: "c360") { total } }`,
		"entitiesByPredicate": `{ entitiesByPredicate(predicate: "test.link.to") { entities } }`,
		"pathSearch":          `{ pathSearch(startEntity: "` + droneA + `", direction: "outgoing", maxDepth: 1) { truncated } }`,
	}
}

// holdAll stands in for every graph.query.* verb the gateway asks, holding each request until the
// test ends.
func holdAll(t *testing.T, client *natsclient.Client) *probe.Callback {
	t.Helper()
	held := probe.NewCallback()
	t.Cleanup(held.Release)
	for _, name := range rootVerbs {
		stub(t, client, verb(t, graph.ResponderGraphQuery, name).Subject, func(ctx context.Context, _ []byte) ([]byte, error) {
			held.Block(ctx)
			return nil, errors.New("released")
		})
	}
	return held
}

// TestDeadlineIs504: a request whose verb does not answer within query_timeout_ms is HTTP 504
// "request timeout", the root field null, for each root field.
func TestDeadlineIs504(t *testing.T) {
	client := fixtureClient(t)
	g := startGateway(t, client, `{"query_timeout_ms":100}`)
	holdAll(t, client)
	for field, query := range fieldQueries() {
		t.Run(field, func(t *testing.T) {
			requireNullField(t, g.post(t, query, nil), http.StatusGatewayTimeout, field, "request timeout")
		})
	}
}

// TestQueryTimeoutIsRead: with query_timeout_ms 50 and a verb that never answers, the answer is HTTP
// 504 well before the 60 s default would end the wait. The bound is wall clock because the timeout is
// the behaviour under test; 20 s leaves a loaded host 400 times the configured 50 ms.
func TestQueryTimeoutIsRead(t *testing.T) {
	client := fixtureClient(t)
	g := startGateway(t, client, `{"query_timeout_ms":50}`)
	holdAll(t, client)
	began := time.Now()
	got := g.post(t, fieldQueries()["entity"], nil)
	if elapsed := time.Since(began); elapsed > 20*time.Second {
		t.Errorf("answered after %s, want the configured 50 ms, not the default", elapsed)
	}
	requireNullField(t, got, http.StatusGatewayTimeout, "entity", "request timeout")
}

// TestStopWaitsForInFlightRequest: Stop called while a request waits on its verb returns only after
// that request has answered, refusing every request that arrives meanwhile, and a later request gets
// 503.
func TestStopWaitsForInFlightRequest(t *testing.T) {
	client := fixtureClient(t)
	g := startGateway(t, client, `{}`)
	held := probe.NewCallback()
	t.Cleanup(held.Release)
	stub(t, client, verb(t, graph.ResponderGraphQuery, "entity").Subject, func(ctx context.Context, _ []byte) ([]byte, error) {
		held.Block(ctx)
		return []byte(`{"entity":` + storedEntity + `,"kv_revision":3}`), nil
	})

	inFlight := httptest.NewRecorder()
	served := make(chan struct{})
	go func() {
		defer close(served)
		body := `{"query":"{ entity(id: \"` + droneA + `\") { kvRevision } }"}`
		g.mux.ServeHTTP(inFlight, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/graph-gateway/graphql", strings.NewReader(body)))
	}()
	<-held.Entered()
	// lifecycle-suite, "Started owner reports its handles": the suite sees no request in flight.
	if held := (suiteOwner{c: g.c}).Observe().Unresolved; !slices.Equal(held, []string{"admitted request"}) {
		t.Errorf("a started gateway with a request in flight reports %v, want the admitted request", held)
	}

	stopped := make(chan error, 1)
	go func() { stopped <- g.c.Stop(t.Context()) }()
	// The shell stops reporting running when cleanup begins, which then closes admission.
	if _, err := probe.Await(t.Context(), func(context.Context) (bool, error) {
		return g.c.Health().Status == "stopped", nil
	}, func(begun bool) bool { return begun }); err != nil {
		t.Fatalf("await Stop's cleanup: %v", err)
	}
	if late := g.send(t, t.Context(), fieldQueries()["entity"], nil); late.Code != http.StatusServiceUnavailable {
		t.Errorf("a request while Stop waits: %d %q, want 503", late.Code, late.Body)
	}
	select {
	case err := <-stopped:
		t.Fatalf("Stop returned %v while a request waited on its verb", err)
	default:
	}

	held.Release()
	if err := <-stopped; err != nil {
		t.Fatalf("Stop: %v", err)
	}
	// Read after Stop returned: Stop's wait observed the request's return, which wrote the answer,
	// so this read is ordered after the write.
	requireData(t, decodeAnswer(t, inFlight), `{"entity":{"kvRevision":3}}`)
	<-served
	if later := g.send(t, t.Context(), fieldQueries()["entity"], nil); later.Code != http.StatusServiceUnavailable {
		t.Errorf("a request after Stop: %d %q, want 503", later.Code, later.Body)
	}
}
