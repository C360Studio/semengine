//go:build integration

package graphquery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
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
	"github.com/c360studio/semengine/pkg/lifecycle"
	graphindex "github.com/c360studio/semengine/processor/graph-index"
	graphingest "github.com/c360studio/semengine/processor/graph-ingest"
	"github.com/c360studio/semengine/types"
	"github.com/c360studio/semengine/vocabulary"
)

const (
	// A valid entity ID under the authority graph-ingest runs with below.
	drone1 = "c360.test.query.system.drone.001"
	drone2 = "c360.test.query.system.drone.002"
	drone3 = "c360.test.query.system.drone.003"
	drone4 = "c360.test.query.system.drone.004"
	drone5 = "c360.test.query.system.drone.005"
	// No entity has this ID, in the tests that use it.
	absentDrone = "c360.test.query.system.drone.999"
	linkTo      = "test.link.to"
)

var testEntityType = message.Type{Domain: "test", Category: "query", Version: "v1"}

// graphStack is real graph-ingest and graph-index behind graph-query, on one connection.
type graphStack struct {
	client    *natsclient.Client
	mutations *graphmutation.Client
}

// startStack starts all three on client; each stops at the end of the test, graph-query first.
func startStack(t *testing.T) graphStack {
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
	startComponent(t, client, `{}`)

	mutations, err := graphmutation.NewClient(client, requestTimeout)
	if err != nil {
		t.Fatal(err)
	}
	return graphStack{client: client, mutations: mutations}
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

// create writes an entity through graph-ingest and returns its kv_revision. Each statement is a
// predicate and an object.
func (s graphStack) create(t *testing.T, id string, statements ...[2]string) uint64 {
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

// awaitIndexed waits until a direct read of the alias verb reports revision reached. Every read of the
// index after it reflects that write, and carries indexed_revision at or above it.
func (s graphStack) awaitIndexed(t *testing.T, revision uint64) {
	t.Helper()
	request := fmt.Appendf(nil, `{"alias":"nobody-holds-this","min_revision":%d}`, revision)
	aliasVerb := verbRow(t, graph.ResponderGraphIndex, "alias").Subject
	_, err := probe.Await(t.Context(), func(ctx context.Context) (bool, error) {
		_, err := s.client.RequestClassified(ctx, aliasVerb, request, requestTimeout)
		return err == nil, err
	}, func(reached bool) bool { return reached })
	if err != nil {
		t.Fatalf("revision %d indexed: %v", revision, err)
	}
}

// wireEnvelope reads a composed verb's reply strictly: any key beside these four fails the decode.
type wireEnvelope struct {
	Data            json.RawMessage `json:"data"`
	IndexedRevision uint64          `json:"indexed_revision"`
	Producer        string          `json:"producer"`
	Timestamp       time.Time       `json:"timestamp"`
}

func decodeEnvelope(t *testing.T, raw []byte) wireEnvelope {
	t.Helper()
	var envelope wireEnvelope
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		t.Fatalf("decode envelope %q: %v", raw, err)
	}
	if envelope.Producer != "graph-query" {
		t.Errorf("producer = %q, want graph-query, in %q", envelope.Producer, raw)
	}
	if envelope.Timestamp.IsZero() {
		t.Errorf("no timestamp in %q", raw)
	}
	return envelope
}

// requireRefusal fails unless err is a refusal of this class and code. A literal code, as callers
// match the wire string.
func requireRefusal(t *testing.T, reply []byte, err error, class errs.ErrorClass, code string) *errs.ClassifiedError {
	t.Helper()
	if err == nil {
		t.Fatalf("succeeded with %q, want a %v %s refusal", reply, class, code)
	}
	got := refusalOf(t, err)
	if got.Class != class || got.Code != code {
		t.Fatalf("class, code = %v, %q (%v), want %v, %q", got.Class, got.Code, err, class, code)
	}
	return got
}

// spyOnEntityVerb watches every request the entity verb receives, on the connection every component
// here shares. The returned check fails if one arrived. It sends a marker on the same connection and
// reads what the spy saw first: NATS orders one connection's messages, so a request graph-query sent
// before it replied reaches the spy before the marker. That order, at the pinned versions, is shown by
// the "second request" row in notes/3.3.md; across versions it is not promised here. The marker is
// refused, which nothing reads.
func spyOnEntityVerb(t *testing.T, client *natsclient.Client) func() {
	t.Helper()
	subject := verbRow(t, graph.ResponderGraphIngest, "entity").Subject
	seen := make(chan []byte, 16)
	sub, err := client.Subscribe(t.Context(), subject, func(_ context.Context, msg *nats.Msg) { seen <- msg.Data })
	if err != nil {
		t.Fatalf("subscribe %s: %v", subject, err)
	}
	return func() {
		t.Helper()
		const marker = `{"id":"spy-marker"}`
		_, _ = client.RequestClassified(t.Context(), subject, []byte(marker), requestTimeout)
		if got := <-seen; string(got) != marker {
			t.Errorf("the entity verb received %q, want no request", got)
		}
		if err := sub.Unsubscribe(); err != nil {
			t.Errorf("unsubscribe %s: %v", subject, err)
		}
	}
}

// TestEntityByAlias: the alias verb is asked first and its refusals reach the caller; a holder is read,
// an unheld valid ID is read as itself, an unheld non-ID is not found, and the reply wraps the entity
// verb's JSON with the alias reply's revision (design D4, ADR-114). Real ingest and index where a
// real one can produce the case; stubs for the rows D4 names, and for reply bytes no real ingest
// sends.
func TestEntityByAlias(t *testing.T) {
	t.Run("real ingest and index", testEntityByAliasReal)
	t.Run("stubbed", testEntityByAliasStubbed)
	t.Run("entity JSON is not re-decoded", testEntityByAliasKeepsEntityJSON)
}

func testEntityByAliasReal(t *testing.T) {
	stack := startStack(t)
	aliased := func(alias string) [2]string { return [2]string{vocabulary.CoreIdentityAlias, alias} }

	holderRevision := stack.create(t, drone1, aliased("held-1"))
	plainRevision := stack.create(t, drone2)
	stack.create(t, drone3, aliased("shared-1"))
	stack.create(t, drone4, aliased("shared-1"))
	// A later write to another entity, so the index's revision is above every kv_revision read below.
	later := stack.create(t, drone5)
	stack.awaitIndexed(t, later)

	ask := func(t *testing.T, request string) ([]byte, error) {
		t.Helper()
		return stack.client.RequestClassified(t.Context(), verbRow(t, graph.ResponderGraphQuery, "entityByAlias").Subject,
			[]byte(request), requestTimeout)
	}
	entityBytes := func(t *testing.T, id string) []byte {
		t.Helper()
		raw, err := stack.client.RequestClassified(t.Context(), verbRow(t, graph.ResponderGraphIngest, "entity").Subject,
			fmt.Appendf(nil, `{"id":%q}`, id), requestTimeout)
		if err != nil {
			t.Fatalf("direct entity read of %s: %v", id, err)
		}
		return raw
	}
	// requireEntity holds a success: data is exactly what the entity verb replies for id, and the
	// envelope's revision is the alias reply's, which has passed that entity's own kv_revision.
	requireEntity := func(t *testing.T, raw []byte, id string, kvRevision uint64) {
		t.Helper()
		envelope := decodeEnvelope(t, raw)
		if want := entityBytes(t, id); !bytes.Equal(envelope.Data, want) {
			t.Errorf("data = %q, want the entity verb's %q", envelope.Data, want)
		}
		var entity graph.ExactEntity
		if err := json.Unmarshal(envelope.Data, &entity); err != nil {
			t.Fatalf("decode data: %v", err)
		}
		if entity.Entity == nil || entity.Entity.ID != id || entity.KVRevision != kvRevision {
			t.Errorf("data holds %+v at kv_revision %d, want %s at %d", entity.Entity, entity.KVRevision, id, kvRevision)
		}
		if envelope.IndexedRevision < later {
			t.Errorf("indexed_revision = %d, below %d, which the alias verb has shown: this is not the alias reply's", envelope.IndexedRevision, later)
		}
	}

	t.Run("a holder is read", func(t *testing.T) {
		raw, err := ask(t, `{"alias":"held-1"}`)
		if err != nil {
			t.Fatal(err)
		}
		requireEntity(t, raw, drone1, holderRevision)
	})
	t.Run("a valid ID nobody holds is read as itself", func(t *testing.T) {
		raw, err := ask(t, fmt.Sprintf(`{"alias":%q}`, drone2))
		if err != nil {
			t.Fatal(err)
		}
		requireEntity(t, raw, drone2, plainRevision)
	})
	t.Run("a valid ID with no entity is the entity verb's refusal", func(t *testing.T) {
		raw, err := ask(t, fmt.Sprintf(`{"alias":%q}`, absentDrone))
		requireRefusal(t, raw, err, errs.ErrorInvalid, "entity_not_found")
	})
	t.Run("an unknown non-ID is not found, with no second request", func(t *testing.T) {
		noEntityRequest := spyOnEntityVerb(t, stack.client)
		raw, err := ask(t, `{"alias":"sn-1"}`)
		requireRefusal(t, raw, err, errs.ErrorInvalid, "entity_not_found")
		noEntityRequest()
	})
	t.Run("a shared alias is refused, with no entity read", func(t *testing.T) {
		noEntityRequest := spyOnEntityVerb(t, stack.client)
		raw, err := ask(t, `{"alias":"shared-1"}`)
		requireRefusal(t, raw, err, errs.ErrorInvalid, "alias_ambiguous")
		if !strings.Contains(err.Error(), `"shared-1"`) {
			t.Errorf("message %q does not name the alias", err)
		}
		noEntityRequest()
	})
	t.Run("a revision far ahead is refused, with no entity read", func(t *testing.T) {
		noEntityRequest := spyOnEntityVerb(t, stack.client)
		raw, err := ask(t, `{"alias":"held-1","min_revision":1000000000}`)
		got := requireRefusal(t, raw, err, errs.ErrorTransient, "revision_not_reached")
		reached, ok := got.Detail["indexed_revision"].(float64)
		if !ok || uint64(reached) < later {
			t.Errorf("detail = %v, want indexed_revision at or above %d", got.Detail, later)
		}
		noEntityRequest()
	})
	t.Run("the alias verb refuses an empty alias", func(t *testing.T) {
		noEntityRequest := spyOnEntityVerb(t, stack.client)
		raw, err := ask(t, `{"alias":"  "}`)
		requireRefusal(t, raw, err, errs.ErrorInvalid, "invalid_request")
		noEntityRequest()
	})
	t.Run("a request that does not decode is refused", func(t *testing.T) {
		noEntityRequest := spyOnEntityVerb(t, stack.client)
		raw, err := ask(t, `not json`)
		requireRefusal(t, raw, err, errs.ErrorInvalid, "invalid_request")
		noEntityRequest()
	})
}

// stubAnswer is what a stubbed verb sends, a reply or a refusal.
type stubAnswer struct {
	reply   string
	refusal error
}

func (a stubAnswer) answer() ([]byte, error) {
	if a.refusal != nil {
		return nil, a.refusal
	}
	return []byte(a.reply), nil
}

// aliasReply is the alias verb's literal reply: revision 77, and held the ID, or null when empty.
func aliasReply(held string) stubAnswer {
	id := "null"
	if held != "" {
		id = `"` + held + `"`
	}
	return stubAnswer{reply: `{"data":{"canonical_id":` + id + `},"indexed_revision":77,"producer":"graph-index","timestamp":"2026-11-03T00:00:00Z"}`}
}

func testEntityByAliasStubbed(t *testing.T) {
	aliasVerb := verbRow(t, graph.ResponderGraphIndex, "alias")
	entityVerb := verbRow(t, graph.ResponderGraphIngest, "entity")
	notReady := errs.ClassifiedCodeDetail(errs.ErrorTransient, "index_not_ready", map[string]any{"bucket": "ALIAS_INDEX"}, errors.New("still replaying"))
	goneEntity := errs.ClassifiedCodeDetail(errs.ErrorInvalid, "entity_not_found", map[string]any{"entity_id": drone1}, errors.New("not found: "+drone1))
	internal := errs.ClassifiedCodeDetail(errs.ErrorTransient, "internal", map[string]any{"at": "index"}, errors.New("internal error: boom"))

	cases := []struct {
		name          string
		alias, entity *stubAnswer // nil: no responder
		class         errs.ErrorClass
		code          string
		detail        map[string]any // a relayed refusal's detail, as it decodes
		message       string         // a relayed refusal's exact message
		names         string         // what graph-query's own refusal names
		entityReads   int64
	}{
		{name: "index_not_ready", alias: &stubAnswer{refusal: notReady},
			class: errs.ErrorTransient, code: "index_not_ready", detail: map[string]any{"bucket": "ALIAS_INDEX"}, message: "still replaying"},
		{name: "internal", alias: &stubAnswer{refusal: internal},
			class: errs.ErrorTransient, code: "internal", detail: map[string]any{"at": "index"}, message: "internal error: boom"},
		{name: "an alias reply that does not decode", alias: &stubAnswer{reply: `not json`},
			class: errs.ErrorTransient, code: "internal", names: aliasVerb.Subject},
		{name: "an alias reply of the wrong shape", alias: &stubAnswer{reply: `{"data":"x","producer":"graph-index"}`},
			class: errs.ErrorTransient, code: "internal", names: aliasVerb.Subject},
		{name: "an alias reply of {}", alias: &stubAnswer{reply: `{}`},
			class: errs.ErrorTransient, code: "internal", names: aliasVerb.Subject},
		{name: "a holder in a reply naming no producer", alias: &stubAnswer{reply: `{"data":{"canonical_id":"` + drone1 + `"},"indexed_revision":77}`},
			class: errs.ErrorTransient, code: "internal", names: aliasVerb.Subject},
		{name: "no responder on the alias verb",
			class: errs.ErrorTransient, code: "internal", names: aliasVerb.Subject},
		{name: "no responder on the entity verb", alias: ptr(aliasReply(drone1)),
			class: errs.ErrorTransient, code: "internal", names: entityVerb.Subject},
		{name: "a holder whose entity is gone", alias: ptr(aliasReply(drone1)), entity: &stubAnswer{refusal: goneEntity},
			class: errs.ErrorInvalid, code: "entity_not_found", detail: map[string]any{"entity_id": drone1}, message: "not found: " + drone1, entityReads: 1},
		{name: "an entity reply that is not JSON", alias: ptr(aliasReply(drone1)), entity: &stubAnswer{reply: `not json`},
			class: errs.ErrorTransient, code: "internal", names: entityVerb.Subject, entityReads: 1},
		{name: "an empty entity reply", alias: ptr(aliasReply(drone1)), entity: &stubAnswer{reply: ``},
			class: errs.ErrorTransient, code: "internal", names: entityVerb.Subject, entityReads: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := fixtureClient(t)
			startComponent(t, client, `{}`)
			var entityReads atomic.Int64
			if tc.alias != nil {
				serve(t, client, aliasVerb.Subject, func(context.Context, []byte) ([]byte, error) { return tc.alias.answer() })
			}
			if tc.entity != nil {
				serve(t, client, entityVerb.Subject, func(context.Context, []byte) ([]byte, error) {
					entityReads.Add(1)
					return tc.entity.answer()
				})
			}
			raw, err := client.RequestClassified(t.Context(), "graph.query.entityByAlias", []byte(`{"alias":"held-1"}`), requestTimeout)
			got := requireRefusal(t, raw, err, tc.class, tc.code)
			if tc.names != "" && !strings.Contains(err.Error(), tc.names) {
				t.Errorf("refusal %q does not name %s", err, tc.names)
			}
			if tc.message != "" {
				if err.Error() != tc.message {
					t.Errorf("message = %q, want %q", err.Error(), tc.message)
				}
				if !reflect.DeepEqual(got.Detail, tc.detail) {
					t.Errorf("detail = %v, want %v", got.Detail, tc.detail)
				}
			}
			// The stub counts before it replies, so before the refusal above.
			if n := entityReads.Load(); n != tc.entityReads {
				t.Errorf("the entity verb received %d requests, want %d", n, tc.entityReads)
			}
		})
	}
}

func ptr(a stubAnswer) *stubAnswer { return &a }

// testEntityByAliasKeepsEntityJSON: the alias verb receives the caller's exact bytes, the entity verb
// the literal request for the holder, and data is the entity verb's JSON as it came, with the alias
// reply's revision. A field ExactEntity does not know, a number's spelling and the key order would
// each change if data were decoded into a struct or a map.
func testEntityByAliasKeepsEntityJSON(t *testing.T) {
	client := fixtureClient(t)
	startComponent(t, client, `{}`)

	const request = `{ "alias" : "held-1",  "min_revision": 3, "unknown_field":[1.50] }`
	const entityReply = `{"kv_revision":5,"unknown_field":[1.50],"entity":{"id":"held"}}`
	var aliasReceived, entityReceived atomic.Pointer[[]byte]
	serve(t, client, verbRow(t, graph.ResponderGraphIndex, "alias").Subject, func(_ context.Context, data []byte) ([]byte, error) {
		aliasReceived.Store(&data)
		return aliasReply(drone1).answer()
	})
	serve(t, client, verbRow(t, graph.ResponderGraphIngest, "entity").Subject, func(_ context.Context, data []byte) ([]byte, error) {
		entityReceived.Store(&data)
		return []byte(entityReply), nil
	})

	raw, err := client.RequestClassified(t.Context(), "graph.query.entityByAlias", []byte(request), requestTimeout)
	if err != nil {
		t.Fatal(err)
	}
	// Each stub stores before it replies, so before the reply above.
	if got := aliasReceived.Load(); got == nil || string(*got) != request {
		t.Errorf("the alias verb received %s, want %q", printable(got), request)
	}
	if got, want := entityReceived.Load(), `{"id":"`+drone1+`"}`; got == nil || string(*got) != want {
		t.Errorf("the entity verb received %s, want %q", printable(got), want)
	}
	want := `{"data":` + entityReply + `,"indexed_revision":77,"producer":"graph-query","timestamp":"`
	if !bytes.HasPrefix(raw, []byte(want)) {
		t.Errorf("reply = %q, want it to begin %q", raw, want)
	}
	decodeEnvelope(t, raw)
}

func printable(data *[]byte) string {
	if data == nil {
		return "nothing"
	}
	return fmt.Sprintf("%q", *data)
}

// TestRelationshipsRowKeys: on a real index, one relationship read from each end is the same literal
// row, with exactly its three keys, and no rows is the literal [].
func TestRelationshipsRowKeys(t *testing.T) {
	stack := startStack(t)
	stack.awaitIndexed(t, stack.create(t, drone1, [2]string{linkTo, drone2}))
	row := `[{"predicate":"` + linkTo + `","from_entity_id":"` + drone1 + `","to_entity_id":"` + drone2 + `"}]`

	cases := []struct{ id, direction, want string }{
		{drone1, "outgoing", row},
		{drone2, "incoming", row},
		{drone2, "outgoing", `[]`},
		{drone1, "incoming", `[]`},
		{absentDrone, "outgoing", `[]`},
		{absentDrone, "incoming", `[]`},
	}
	for _, tc := range cases {
		request := fmt.Sprintf(`{"entity_id":%q,"direction":%q}`, tc.id, tc.direction)
		raw, err := stack.client.RequestClassified(t.Context(), "graph.query.relationships", []byte(request), requestTimeout)
		if err != nil {
			t.Fatalf("%s: %v", request, err)
		}
		if got := string(decodeEnvelope(t, raw).Data); got != tc.want {
			t.Errorf("%s: data = %s, want %s", request, got, tc.want)
		}
	}
}

// TestRelationshipsEnvelopeKeepsIndexRevision: the envelope carries the index reply's revision, in
// each direction. A later write to another entity moves a real index past the entity's own write. A
// stub with a revision no write produced shows it is the reply's own, and a min_revision and an
// unknown field reach the index in the request's bytes.
func TestRelationshipsEnvelopeKeepsIndexRevision(t *testing.T) {
	t.Run("real index", func(t *testing.T) {
		stack := startStack(t)
		stack.create(t, drone1, [2]string{linkTo, drone2})
		later := stack.create(t, drone3)
		stack.awaitIndexed(t, later)
		for _, request := range []string{
			fmt.Sprintf(`{"entity_id":%q,"direction":"outgoing","min_revision":%d}`, drone1, later),
			fmt.Sprintf(`{"entity_id":%q,"direction":"incoming","min_revision":%d}`, drone2, later),
		} {
			raw, err := stack.client.RequestClassified(t.Context(), "graph.query.relationships", []byte(request), requestTimeout)
			if err != nil {
				t.Fatalf("%s: %v", request, err)
			}
			if got := decodeEnvelope(t, raw).IndexedRevision; got < later {
				t.Errorf("%s: indexed_revision = %d, below %d, which the index has shown", request, got, later)
			}
		}
	})
	t.Run("stubbed index", func(t *testing.T) {
		client := fixtureClient(t)
		startComponent(t, client, `{}`)
		received := map[string]*atomic.Pointer[[]byte]{"outgoing": {}, "incoming": {}}
		serve(t, client, verbRow(t, graph.ResponderGraphIndex, "outgoing").Subject, func(_ context.Context, data []byte) ([]byte, error) {
			received["outgoing"].Store(&data)
			return []byte(`{"data":{"relationships":[{"to_entity_id":"` + drone2 + `","predicate":"` + linkTo + `"}]},"indexed_revision":77,"producer":"graph-index","timestamp":"2026-11-03T00:00:00Z"}`), nil
		})
		serve(t, client, verbRow(t, graph.ResponderGraphIndex, "incoming").Subject, func(_ context.Context, data []byte) ([]byte, error) {
			received["incoming"].Store(&data)
			return []byte(`{"data":{"relationships":[{"from_entity_id":"` + drone2 + `","predicate":"` + linkTo + `"}]},"indexed_revision":78,"producer":"graph-index","timestamp":"2026-11-03T00:00:00Z"}`), nil
		})
		for _, tc := range []struct {
			direction, row string
			revision       string
		}{
			{"outgoing", `[{"predicate":"` + linkTo + `","from_entity_id":"` + drone1 + `","to_entity_id":"` + drone2 + `"}]`, "77"},
			{"incoming", `[{"predicate":"` + linkTo + `","from_entity_id":"` + drone2 + `","to_entity_id":"` + drone1 + `"}]`, "78"},
		} {
			request := fmt.Sprintf(`{ "entity_id" : %q,  "direction": %q, "min_revision": 7, "unknown_field":[1.50] }`, drone1, tc.direction)
			raw, err := client.RequestClassified(t.Context(), "graph.query.relationships", []byte(request), requestTimeout)
			if err != nil {
				t.Fatalf("%s: %v", tc.direction, err)
			}
			// The stub stores before it replies, so before the reply above.
			if got := received[tc.direction].Load(); got == nil || string(*got) != request {
				t.Errorf("%s: the index received %s, want %q", tc.direction, printable(got), request)
			}
			want := `{"data":` + tc.row + `,"indexed_revision":` + tc.revision + `,"producer":"graph-query","timestamp":"`
			if !bytes.HasPrefix(raw, []byte(want)) {
				t.Errorf("%s: reply = %q, want it to begin %q", tc.direction, raw, want)
			}
		}
	})
}

// TestRelationshipsRefusesUnknownDirection: a direction that is not exactly outgoing or incoming is
// invalid_request and no index verb receives a request; the pin read all of these as outgoing.
func TestRelationshipsRefusesUnknownDirection(t *testing.T) {
	client := fixtureClient(t)
	startComponent(t, client, `{}`)
	var indexRequests atomic.Int64
	for _, name := range []string{"outgoing", "incoming"} {
		serve(t, client, verbRow(t, graph.ResponderGraphIndex, name).Subject, func(context.Context, []byte) ([]byte, error) {
			indexRequests.Add(1)
			return []byte(`{"data":{"relationships":[]},"producer":"graph-index"}`), nil
		})
	}
	for _, request := range []string{
		`{"entity_id":"` + drone1 + `","direction":"both"}`,
		`{"entity_id":"` + drone1 + `","direction":""}`,
		`{"entity_id":"` + drone1 + `"}`,
		`{"entity_id":"` + drone1 + `","direction":"OUTGOING"}`,
		`{"entity_id":"` + drone1 + `","direction":"outgoing "}`,
		`{"entity_id":"` + drone1 + `","direction":null}`,
		`{"entity_id":"` + drone1 + `","direction":1}`,
		`not json`,
	} {
		raw, err := client.RequestClassified(t.Context(), "graph.query.relationships", []byte(request), requestTimeout)
		requireRefusal(t, raw, err, errs.ErrorInvalid, "invalid_request")
		// A stub counts before it replies, so before graph-query's refusal.
		if n := indexRequests.Load(); n != 0 {
			t.Fatalf("%s: the index received %d requests, want none", request, n)
		}
	}
}

// TestRelationshipsRefusesBrokenIndexReply: in each direction, an index reply that does not decode or
// names no producer is transient internal, naming the subject, never an empty list.
func TestRelationshipsRefusesBrokenIndexReply(t *testing.T) {
	for _, reply := range []string{
		`{}`,
		`null`,
		`not json`,
		`{"data":{"relationships":"x"},"producer":"graph-index"}`,
		`{"data":{"relationships":[]},"indexed_revision":77}`,
	} {
		t.Run(reply, func(t *testing.T) {
			client := fixtureClient(t)
			startComponent(t, client, `{}`)
			for _, direction := range []string{"outgoing", "incoming"} {
				subject := verbRow(t, graph.ResponderGraphIndex, direction).Subject
				serve(t, client, subject, func(context.Context, []byte) ([]byte, error) { return []byte(reply), nil })
				request := fmt.Sprintf(`{"entity_id":%q,"direction":%q}`, drone1, direction)
				raw, err := client.RequestClassified(t.Context(), "graph.query.relationships", []byte(request), requestTimeout)
				requireRefusal(t, raw, err, errs.ErrorTransient, "internal")
				if !strings.Contains(err.Error(), subject) {
					t.Errorf("%s: refusal %q does not name %s", direction, err, subject)
				}
			}
		})
	}
}

// TestComposedVerbsCountAsForwardsDo: entityByAlias and relationships count one input per request they
// receive, with its bytes, and one error per request graph-query refuses because no usable answer
// arrived: nothing answered, or the reply was broken. A responder's refusal and a caller's bad request
// count none. Two downstream requests are still one request received. Each step states the running
// error total as a literal.
func TestComposedVerbsCountAsForwardsDo(t *testing.T) {
	client := fixtureClient(t)
	c := startComponent(t, client, `{}`)

	var wantMessages, wantBytes int64
	// Each handler counts before it returns, so before its reply.
	step := func(name, verb, request string, wantErrors int) {
		t.Helper()
		// A step's reply or refusal is not its subject: the tests above read them.
		_, _ = client.RequestClassified(t.Context(), verb, []byte(request), requestTimeout)
		wantMessages++
		wantBytes += int64(len(request))
		flow, health := c.DataFlow(), c.Health()
		if flow.Messages != wantMessages || flow.Bytes != wantBytes {
			t.Errorf("%s: DataFlow messages, bytes = %d, %d, want %d, %d", name, flow.Messages, flow.Bytes, wantMessages, wantBytes)
		}
		if health.ErrorCount != wantErrors {
			t.Errorf("%s: ErrorCount = %d, want %d", name, health.ErrorCount, wantErrors)
		}
	}
	// stub serves subject with whatever the returned pointer holds when a request arrives.
	stub := func(responder, verb string) *atomic.Pointer[stubAnswer] {
		answer := new(atomic.Pointer[stubAnswer])
		serve(t, client, verbRow(t, responder, verb).Subject, func(context.Context, []byte) ([]byte, error) {
			return answer.Load().answer()
		})
		return answer
	}
	const (
		entityByAlias = "graph.query.entityByAlias"
		relationships = "graph.query.relationships"
		aliasRequest  = `{"alias":"held-1"}`
	)
	outgoingRequest := `{"entity_id":"` + drone1 + `","direction":"outgoing"}`
	incomingRequest := `{"entity_id":"` + drone1 + `","direction":"incoming"}`
	replying := func(reply string) *stubAnswer { return &stubAnswer{reply: reply} }
	refusing := &stubAnswer{refusal: errs.ClassifiedCode(errs.ErrorTransient, "index_not_ready", errors.New("replaying"))}

	step("alias: no responders", entityByAlias, aliasRequest, 1)
	step("relationships: no responders", relationships, outgoingRequest, 2)
	step("relationships: a direction it refuses", relationships, `{"entity_id":"`+drone1+`","direction":"both"}`, 2)
	step("alias: a request it refuses", entityByAlias, `not json`, 2)

	alias := stub(graph.ResponderGraphIndex, "alias")
	alias.Store(ptr(aliasReply(drone1)))
	step("alias answered: no responder on the entity verb", entityByAlias, aliasRequest, 3)

	entity := stub(graph.ResponderGraphIngest, "entity")
	entity.Store(replying(`{"entity":{"id":"` + drone1 + `"},"kv_revision":5}`))
	step("alias answered: both requests, one input", entityByAlias, aliasRequest, 3)

	entity.Store(replying(`not json`))
	step("entity: a reply that is not JSON", entityByAlias, aliasRequest, 4)
	entity.Store(refusing)
	step("entity: refused by the responder", entityByAlias, aliasRequest, 4)

	alias.Store(replying(`not json`))
	step("alias: a reply that does not decode", entityByAlias, aliasRequest, 5)
	alias.Store(replying(`{}`))
	step("alias: a reply naming no producer", entityByAlias, aliasRequest, 6)
	alias.Store(refusing)
	step("alias: refused by the responder", entityByAlias, aliasRequest, 6)

	// A responder that leaves after two hops: the server drops the subscription as it delivers the
	// second message, so the third hop has no responder (spec, "A walk that loses its responder"). Raw
	// NATS, since natsclient puts no count on a subscription. The walk waits for each reply, so the
	// handler's calls do not overlap.
	const pathSearch = "graph.query.pathSearch"
	walkRequest := `{"start_entity":"` + drone1 + `","direction":"outgoing","max_depth":3}`
	entity.Store(replying(`{"entity":{"id":"` + drone1 + `"},"kv_revision":5}`))
	nextHops := []string{drone2, drone3}
	leaving, err := client.GetConnection().Subscribe(verbRow(t, graph.ResponderGraphIndex, "outgoing").Subject, func(msg *nats.Msg) {
		reply := `{"data":{"relationships":[{"to_entity_id":"` + nextHops[0] + `","predicate":"` + linkTo + `"}]},"producer":"graph-index"}`
		nextHops = nextHops[1:]
		if err := msg.Respond([]byte(reply)); err != nil {
			t.Errorf("respond: %v", err)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := leaving.AutoUnsubscribe(2); err != nil {
		t.Fatal(err)
	}
	step("walk: a later hop finds no responder", pathSearch, walkRequest, 7)

	outgoing := stub(graph.ResponderGraphIndex, "outgoing")
	incoming := stub(graph.ResponderGraphIndex, "incoming")
	outgoing.Store(replying(`not json`))
	step("outgoing: a reply that does not decode", relationships, outgoingRequest, 8)
	outgoing.Store(replying(`{}`))
	step("outgoing: a reply naming no producer", relationships, outgoingRequest, 9)
	incoming.Store(replying(`not json`))
	step("incoming: a reply that does not decode", relationships, incomingRequest, 10)
	incoming.Store(replying(`{}`))
	step("incoming: a reply naming no producer", relationships, incomingRequest, 11)
	outgoing.Store(refusing)
	step("outgoing: refused by the responder", relationships, outgoingRequest, 11)
	incoming.Store(replying(`{"data":{"relationships":[]},"producer":"graph-index"}`))
	step("incoming: answered", relationships, incomingRequest, 11)

	step("walk: a request it refuses", pathSearch, `{"start_entity":"`+drone1+`","direction":"sideways","max_depth":3}`, 11)
	entity.Store(refusing)
	step("walk: the start refused by the responder", pathSearch, walkRequest, 11)
	entity.Store(replying(`not json`))
	step("walk: a start reply that is not JSON", pathSearch, walkRequest, 12)
	entity.Store(replying(`{"kv_revision":5}`))
	step("walk: a start reply with no entity", pathSearch, walkRequest, 13)

	entity.Store(replying(`{"entity":{"id":"` + drone1 + `"},"kv_revision":5}`))
	outgoing.Store(refusing)
	step("walk: a hop refused by the responder", pathSearch, walkRequest, 13)
	outgoing.Store(replying(`not json`))
	step("walk: a hop reply that does not decode", pathSearch, walkRequest, 14)
	outgoing.Store(replying(`{}`))
	step("walk: a hop reply naming no producer", pathSearch, walkRequest, 15)
	outgoing.Store(replying(`{"data":{"relationships":[]},"producer":"graph-index"}`))
	step("walk: answered", pathSearch, walkRequest, 15)

	const hierarchyStats = "graph.query.hierarchyStats"
	countRequest := `{"prefix":"c360.test"}`
	step("counts: no responders", hierarchyStats, countRequest, 16)
	step("counts: a request it refuses", hierarchyStats, `{"prefix":"c360..test"}`, 16)
	prefix := stub(graph.ResponderGraphIngest, "prefix")
	prefix.Store(replying(`{}`))
	step("counts: a broken page", hierarchyStats, countRequest, 17)
	prefix.Store(refusing)
	step("counts: refused by the responder", hierarchyStats, countRequest, 17)
	prefix.Store(replying(`{"entities":null,"total":0}`))
	step("counts: answered", hierarchyStats, countRequest, 17)
}
