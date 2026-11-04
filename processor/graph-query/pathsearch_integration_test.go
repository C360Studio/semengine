//go:build integration

package graphquery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/nats-io/nats.go"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

const (
	pathSearchSubject = "graph.query.pathSearch"
	drone6            = "c360.test.query.system.drone.006"
	linkNear          = "test.link.near"
)

func askPathSearch(t *testing.T, client *natsclient.Client, request string) ([]byte, error) {
	t.Helper()
	return client.RequestClassified(t.Context(), pathSearchSubject, []byte(request), requestTimeout)
}

// requireJSON compares data with the literal JSON a caller reads, whatever the key order.
func requireJSON(t *testing.T, data []byte, want string) {
	t.Helper()
	var got, expected any
	if err := errors.Join(json.Unmarshal(data, &got), json.Unmarshal([]byte(want), &expected)); err != nil {
		t.Fatalf("decode %s or %s: %v", data, want, err)
	}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("data\n got %s\nwant %s", data, want)
	}
}

// entityReply and hopReply are the literal bytes the real entity and index verbs send.
func entityReply(id string) string {
	return fmt.Sprintf(`{"entity":{"id":%q},"kv_revision":5}`, id)
}

func hopReply(revision uint64, rows string) string {
	return fmt.Sprintf(`{"data":{"relationships":[%s]},"indexed_revision":%d,"producer":"graph-index","timestamp":"2026-11-04T10:00:00Z"}`, rows, revision)
}

func outgoingRow(to string) string {
	return fmt.Sprintf(`{"to_entity_id":%q,"predicate":%q}`, to, linkTo)
}

func incomingRow(from string) string {
	return fmt.Sprintf(`{"from_entity_id":%q,"predicate":%q}`, from, linkTo)
}

// hopRecord holds the bytes of each request a stub received. Each is recorded before its reply, so
// before the caller's reply.
type hopRecord struct {
	mu       sync.Mutex
	requests []string
}

func (r *hopRecord) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.requests...)
}

// serveHop stands in for an index verb, answering each entity from the table and an entity it does not
// hold with no edges.
func serveHop(t *testing.T, client *natsclient.Client, name string, answers map[string]stubAnswer) *hopRecord {
	t.Helper()
	record := &hopRecord{}
	serve(t, client, verbRow(t, graph.ResponderGraphIndex, name).Subject, func(_ context.Context, data []byte) ([]byte, error) {
		record.mu.Lock()
		record.requests = append(record.requests, string(data))
		record.mu.Unlock()
		var request struct {
			EntityID string `json:"entity_id"`
		}
		if err := json.Unmarshal(data, &request); err != nil {
			return nil, err
		}
		answer, held := answers[request.EntityID]
		if !held {
			return []byte(hopReply(1, "")), nil
		}
		return answer.answer()
	})
	return record
}

func serveEntity(t *testing.T, client *natsclient.Client) *hopRecord {
	t.Helper()
	record := &hopRecord{}
	serve(t, client, verbRow(t, graph.ResponderGraphIngest, "entity").Subject, func(_ context.Context, data []byte) ([]byte, error) {
		record.mu.Lock()
		record.requests = append(record.requests, string(data))
		record.mu.Unlock()
		var request graph.EntityRequest
		if err := json.Unmarshal(data, &request); err != nil {
			return nil, err
		}
		return []byte(entityReply(request.ID)), nil
	})
	return record
}

// TestPathSearchRefusesBadRequests: each request the verb does not accept is invalid_request naming
// the field and its bound, with every downstream verb answering yet asked nothing, and no error
// counted. The bounds themselves pass, so the table's edges are the requirement's.
func TestPathSearchRefusesBadRequests(t *testing.T) {
	client := fixtureClient(t)
	c := startComponent(t, client, `{}`)
	entity := serveEntity(t, client)
	outgoing := serveHop(t, client, "outgoing", nil)
	incoming := serveHop(t, client, "incoming", nil)

	valid := func(fields string) string {
		return fmt.Sprintf(`{"start_entity":%q,"direction":"outgoing","max_depth":2%s}`, drone1, fields)
	}
	for _, tc := range []struct{ name, request, names string }{
		{"not JSON", `not json`, "invalid character"},
		{"bytes after the request", valid("") + ` {}`, "after the request"},
		{"an array", `[]`, "cannot unmarshal array"},
		{"an unknown key", valid(`,"predicate":["test.link.to"]`), `unknown field "predicate"`},
		{"the pin's timeout", valid(`,"timeout":"5s"`), `unknown field "timeout"`},
		{"the pin's include_siblings", valid(`,"include_siblings":true`), `unknown field "include_siblings"`},
		{"the pin's max_paths", valid(`,"max_paths":3`), `unknown field "max_paths"`},
		{"no start", `{"direction":"outgoing","max_depth":2}`, "start_entity"},
		{"a start of five parts", `{"start_entity":"c360.test.query.system.drone","direction":"outgoing","max_depth":2}`, "start_entity"},
		{"a start with a wildcard", `{"start_entity":"c360.test.query.system.drone.*","direction":"outgoing","max_depth":2}`, "start_entity"},
		{"a predicate of two parts", valid(`,"predicates":["test.link"]`), "predicates[0]"},
		{"a second predicate of four parts", valid(`,"predicates":["test.link.to","test.link.to.more"]`), "predicates[1]"},
		{"an empty predicate", valid(`,"predicates":[""]`), "predicates[0]"},
		{"no direction", fmt.Sprintf(`{"start_entity":%q,"max_depth":2}`, drone1), `direction "" is not outgoing, incoming or both`},
		{"a mistyped direction", fmt.Sprintf(`{"start_entity":%q,"direction":"sideways","max_depth":2}`, drone1), `direction "sideways" is not outgoing, incoming or both`},
		{"another case", fmt.Sprintf(`{"start_entity":%q,"direction":"OUTGOING","max_depth":2}`, drone1), `direction "OUTGOING" is not outgoing, incoming or both`},
		{"no max_depth", fmt.Sprintf(`{"start_entity":%q,"direction":"outgoing"}`, drone1), "max_depth 0 is outside 1 to 10"},
		{"a negative max_depth", fmt.Sprintf(`{"start_entity":%q,"direction":"outgoing","max_depth":-5}`, drone1), "max_depth -5 is outside 1 to 10"},
		{"max_depth above the bound", fmt.Sprintf(`{"start_entity":%q,"direction":"outgoing","max_depth":11}`, drone1), "max_depth 11 is outside 1 to 10"},
		{"max_depth not a whole number", fmt.Sprintf(`{"start_entity":%q,"direction":"outgoing","max_depth":1.5}`, drone1), "max_depth"},
		{"a negative max_nodes", valid(`,"max_nodes":-1`), "max_nodes -1 is outside 0 to 500"},
		{"max_nodes above the bound", valid(`,"max_nodes":501`), "max_nodes 501 is outside 0 to 500"},
		{"the pin's two billion", valid(`,"max_nodes":2000000000`), "max_nodes 2000000000 is outside 0 to 500"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reply, err := askPathSearch(t, client, tc.request)
			requireRefusal(t, reply, err, errs.ErrorInvalid, graph.ErrorCodeInvalidRequest)
			if !strings.Contains(err.Error(), tc.names) {
				t.Errorf("refusal %q does not name %q", err, tc.names)
			}
		})
	}
	// Each refusal above arrived before this line, so a request made for it has been recorded.
	if got := append(append(entity.snapshot(), outgoing.snapshot()...), incoming.snapshot()...); len(got) != 0 {
		t.Errorf("downstream verbs received %q, want no request", got)
	}
	if got := c.Health().ErrorCount; got != 0 {
		t.Errorf("ErrorCount = %d, want 0: a caller's bad request is not graph-query's error", got)
	}

	// The same stubs answer what the bounds allow, so what was refused above was refused for its bound.
	for _, request := range []string{
		fmt.Sprintf(`{"start_entity":%q,"direction":"outgoing","max_depth":1}`, drone1),
		fmt.Sprintf(`{"start_entity":%q,"direction":"incoming","max_depth":10}`, drone1),
		fmt.Sprintf(`{"start_entity":%q,"direction":"both","max_depth":1}`, drone1),
		valid(`,"max_nodes":0`),
		valid(`,"max_nodes":1`),
		valid(`,"max_nodes":500`),
		valid(`,"predicates":[]`),
		valid(`,"predicates":["test.link.to"],"min_revision":1`),
	} {
		if reply, err := askPathSearch(t, client, request); err != nil {
			t.Errorf("%s: %v, %q", request, err, reply)
		}
	}
	if got := len(entity.snapshot()); got != 8 {
		t.Errorf("the entity verb received %d requests, want 8, one per accepted request", got)
	}
}

// seedPathGraph writes, on real ingest and index:
//
//	drone1 -to-> drone2 -to-> drone3 -to-> drone1, a cycle
//	drone1 -near-> drone4, and drone4 -to-> absentDrone, which has no record
//	drone5 -to-> drone1
//
// and returns once the index holds them all.
func seedPathGraph(t *testing.T, stack graphStack) uint64 {
	t.Helper()
	stack.create(t, drone1, [2]string{linkTo, drone2}, [2]string{linkNear, drone4})
	stack.create(t, drone2, [2]string{linkTo, drone3})
	stack.create(t, drone3, [2]string{linkTo, drone1})
	stack.create(t, drone4, [2]string{linkTo, absentDrone})
	written := stack.create(t, drone5, [2]string{linkTo, drone1})
	stack.awaitIndexed(t, written)
	return written
}

// TestPathSearchOnARealIndex: each direction, the cycle, the filter, a depth cut and a dangling edge,
// as literal JSON from real ingest and index (design D3; spec, "An incoming edge", "A start that does
// not exist"). The carried behaviours of the pin's TestComponent_PathSearch_* live here.
func TestPathSearchOnARealIndex(t *testing.T) {
	stack := startStack(t)
	written := seedPathGraph(t, stack)

	to := func(from, target string) string {
		return fmt.Sprintf(`{"predicate":%q,"from_entity_id":%q,"to_entity_id":%q}`, linkTo, from, target)
	}
	near := func(from, target string) string {
		return fmt.Sprintf(`{"predicate":%q,"from_entity_id":%q,"to_entity_id":%q}`, linkNear, from, target)
	}
	at := func(id string, depth int) string { return fmt.Sprintf(`{"entity_id":%q,"depth":%d}`, id, depth) }
	data := func(entities, edges []string) string {
		return fmt.Sprintf(`{"entities":[%s],"edges":[%s],"truncated":false}`, strings.Join(entities, ","), strings.Join(edges, ","))
	}

	for _, tc := range []struct{ name, request, want string }{
		{
			"outgoing, one hop",
			fmt.Sprintf(`{"start_entity":%q,"direction":"outgoing","max_depth":1}`, drone1),
			data([]string{at(drone1, 0), at(drone2, 1), at(drone4, 1)}, []string{to(drone1, drone2), near(drone1, drone4)}),
		},
		{
			"outgoing, round the cycle and onto an entity with no record",
			fmt.Sprintf(`{"start_entity":%q,"direction":"outgoing","max_depth":10}`, drone1),
			data(
				[]string{at(drone1, 0), at(drone2, 1), at(drone4, 1), at(drone3, 2), at(absentDrone, 2)},
				[]string{to(drone1, drone2), near(drone1, drone4), to(drone2, drone3), to(drone4, absentDrone)},
			),
		},
		{
			"incoming: edges as the graph holds them",
			fmt.Sprintf(`{"start_entity":%q,"direction":"incoming","max_depth":1}`, drone1),
			data([]string{at(drone1, 0), at(drone3, 1), at(drone5, 1)}, []string{to(drone3, drone1), to(drone5, drone1)}),
		},
		{
			"both",
			fmt.Sprintf(`{"start_entity":%q,"direction":"both","max_depth":1}`, drone2),
			data([]string{at(drone2, 0), at(drone1, 1), at(drone3, 1)}, []string{to(drone1, drone2), to(drone2, drone3)}),
		},
		{
			"a filter on one predicate",
			fmt.Sprintf(`{"start_entity":%q,"direction":"outgoing","max_depth":10,"predicates":[%q]}`, drone1, linkNear),
			data([]string{at(drone1, 0), at(drone4, 1)}, []string{near(drone1, drone4)}),
		},
		{
			"a filter nothing matches",
			fmt.Sprintf(`{"start_entity":%q,"direction":"outgoing","max_depth":10,"predicates":["test.link.other"]}`, drone1),
			data([]string{at(drone1, 0)}, nil),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := askPathSearch(t, stack.client, tc.request)
			if err != nil {
				t.Fatalf("%s: %v", tc.request, err)
			}
			envelope := decodeEnvelope(t, raw)
			requireJSON(t, envelope.Data, tc.want)
			if envelope.IndexedRevision < written {
				t.Errorf("indexed_revision = %d, below the last write's %d", envelope.IndexedRevision, written)
			}
		})
	}

	t.Run("a start with no record", func(t *testing.T) {
		raw, err := askPathSearch(t, stack.client, fmt.Sprintf(`{"start_entity":%q,"direction":"outgoing","max_depth":1}`, absentDrone))
		requireRefusal(t, raw, err, errs.ErrorInvalid, graph.ErrorCodeEntityNotFound)
		if !strings.Contains(err.Error(), absentDrone) {
			t.Errorf("refusal %q does not name %s", err, absentDrone)
		}
	})
}

// TestPathSearchTruncatedIsExact: truncated is true exactly when an unseen entity was found and left
// out, on real ingest and index (spec, "The limit meets the graph exactly"; the pin said true at the
// limit, P9).
func TestPathSearchTruncatedIsExact(t *testing.T) {
	stack := startStack(t)
	// drone1 -> drone2, drone3; drone2 -> drone3; drone3 -> drone4. drone5 has no edge.
	stack.create(t, drone1, [2]string{linkTo, drone2}, [2]string{linkTo, drone3})
	stack.create(t, drone2, [2]string{linkTo, drone3})
	stack.create(t, drone3, [2]string{linkTo, drone4})
	stack.create(t, drone4)
	written := stack.create(t, drone5)
	stack.awaitIndexed(t, written)

	for _, tc := range []struct {
		name      string
		start     string
		depth     int
		nodes     int
		ids       []string
		truncated bool
	}{
		{"the limit meets the graph exactly", drone1, 2, 4, []string{drone1, drone2, drone3, drone4}, false},
		{"the limit one lower", drone1, 2, 3, []string{drone1, drone2, drone3}, true},
		{"the limit is reached, and the next read finds only what it holds", drone1, 1, 3, []string{drone1, drone2, drone3}, false},
		{"the limit is the start, with a neighbour", drone1, 1, 1, []string{drone1}, true},
		{"the limit is the start, with none", drone5, 1, 1, []string{drone5}, false},
		{"the limit is reached, and what lies past max_depth does not count", drone2, 1, 2, []string{drone2, drone3}, false},
		{"the limit is reached, and a later read finds one", drone3, 2, 1, []string{drone3}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := askPathSearch(t, stack.client,
				fmt.Sprintf(`{"start_entity":%q,"direction":"outgoing","max_depth":%d,"max_nodes":%d}`, tc.start, tc.depth, tc.nodes))
			if err != nil {
				t.Fatal(err)
			}
			var data struct {
				Entities []struct {
					EntityID string `json:"entity_id"`
				} `json:"entities"`
				Truncated bool `json:"truncated"`
			}
			if err := json.Unmarshal(decodeEnvelope(t, raw).Data, &data); err != nil {
				t.Fatal(err)
			}
			ids := make([]string, 0, len(data.Entities))
			for _, row := range data.Entities {
				ids = append(ids, row.EntityID)
			}
			if !reflect.DeepEqual(ids, tc.ids) || data.Truncated != tc.truncated {
				t.Errorf("entities %v, truncated %t; want %v, %t", ids, data.Truncated, tc.ids, tc.truncated)
			}
		})
	}
}

// TestPathSearchRevisionIsTheSmallest: the reply's indexed_revision is the smallest any hop answered at,
// wherever in the walk it falls, and every hop carries the request's min_revision unchanged (stubs: a
// real index cannot answer a walk's hops at chosen revisions).
func TestPathSearchRevisionIsTheSmallest(t *testing.T) {
	chain := func(first, second, third uint64) map[string]stubAnswer {
		return map[string]stubAnswer{
			drone1: {reply: hopReply(first, outgoingRow(drone2))},
			drone2: {reply: hopReply(second, outgoingRow(drone3))},
			drone3: {reply: hopReply(third, "")},
		}
	}
	for _, tc := range []struct {
		name string
		hops map[string]stubAnswer
		want uint64
	}{
		{"first", chain(3, 5, 7), 3},
		{"middle", chain(9, 5, 7), 5},
		{"last", chain(9, 8, 2), 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := fixtureClient(t)
			startComponent(t, client, `{}`)
			entity := serveEntity(t, client)
			outgoing := serveHop(t, client, "outgoing", tc.hops)
			raw, err := askPathSearch(t, client, fmt.Sprintf(`{"start_entity":%q,"direction":"outgoing","max_depth":10,"min_revision":2}`, drone1))
			if err != nil {
				t.Fatal(err)
			}
			if got := decodeEnvelope(t, raw).IndexedRevision; got != tc.want {
				t.Errorf("indexed_revision = %d, want %d", got, tc.want)
			}
			want := []string{
				fmt.Sprintf(`{"entity_id":%q,"min_revision":2}`, drone1),
				fmt.Sprintf(`{"entity_id":%q,"min_revision":2}`, drone2),
				fmt.Sprintf(`{"entity_id":%q,"min_revision":2}`, drone3),
			}
			if got := outgoing.snapshot(); !reflect.DeepEqual(got, want) {
				t.Errorf("outgoing received %q, want %q", got, want)
			}
			if got, want := entity.snapshot(), []string{fmt.Sprintf(`{"id":%q}`, drone1)}; !reflect.DeepEqual(got, want) {
				t.Errorf("entity received %q, want %q", got, want)
			}
		})
	}

	t.Run("both legs of a hop", func(t *testing.T) {
		client := fixtureClient(t)
		startComponent(t, client, `{}`)
		serveEntity(t, client)
		outgoing := serveHop(t, client, "outgoing", map[string]stubAnswer{drone1: {reply: hopReply(9, "")}})
		incoming := serveHop(t, client, "incoming", map[string]stubAnswer{drone1: {reply: hopReply(4, "")}})
		raw, err := askPathSearch(t, client, fmt.Sprintf(`{"start_entity":%q,"direction":"both","max_depth":1,"min_revision":2}`, drone1))
		if err != nil {
			t.Fatal(err)
		}
		if got := decodeEnvelope(t, raw).IndexedRevision; got != 4 {
			t.Errorf("indexed_revision = %d, want 4", got)
		}
		want := []string{fmt.Sprintf(`{"entity_id":%q,"min_revision":2}`, drone1)}
		if got := outgoing.snapshot(); !reflect.DeepEqual(got, want) {
			t.Errorf("outgoing received %q, want %q", got, want)
		}
		if got := incoming.snapshot(); !reflect.DeepEqual(got, want) {
			t.Errorf("incoming received %q, want %q", got, want)
		}
	})
}

// TestPathSearchRefusedMidWalk: at a later hop, a responder's refusal reaches the caller with its class,
// code and detail, its message naming that hop's entity, and a reply graph-query cannot use is transient
// internal naming it too, never the entities found so far (spec, "A hop refused mid-walk", "A hub over
// the reply limit"). Carries the pin's RejectsStructurallyInvalidRelationshipEnvelopes bodies: "{}",
// "null" and an envelope with no relationships name no producer.
func TestPathSearchRefusedMidWalk(t *testing.T) {
	notReady := errs.ClassifiedCodeDetail(errs.ErrorTransient, "index_not_ready", map[string]any{"replayed": float64(3)}, errors.New("replaying"))

	first := map[string]stubAnswer{drone1: {reply: hopReply(9, outgoingRow(drone2))}}
	second := func(answer stubAnswer) map[string]stubAnswer {
		return map[string]stubAnswer{drone1: first[drone1], drone2: answer}
	}
	for _, tc := range []struct {
		name       string
		direction  string
		outgoing   map[string]stubAnswer
		incoming   map[string]stubAnswer
		class      errs.ErrorClass
		code       string
		detail     map[string]any
		messageHas string
	}{
		{name: "a refusal on the later hop", direction: "outgoing", outgoing: second(stubAnswer{refusal: notReady}),
			class: errs.ErrorTransient, code: "index_not_ready", detail: map[string]any{"replayed": float64(3)}, messageHas: "replaying"},
		{name: "a refusal on the incoming leg of a later hop", direction: "both", outgoing: first,
			incoming: map[string]stubAnswer{drone2: {refusal: notReady}},
			class:    errs.ErrorTransient, code: "index_not_ready", detail: map[string]any{"replayed": float64(3)}, messageHas: "replaying"},
		{name: "a reply that is not JSON", direction: "outgoing", outgoing: second(stubAnswer{reply: `not json`}),
			class: errs.ErrorTransient, code: graph.ErrorCodeInternal, messageHas: "graph.index.query.outgoing"},
		{name: "an empty object", direction: "outgoing", outgoing: second(stubAnswer{reply: `{}`}),
			class: errs.ErrorTransient, code: graph.ErrorCodeInternal, messageHas: "names no producer"},
		{name: "null", direction: "outgoing", outgoing: second(stubAnswer{reply: `null`}),
			class: errs.ErrorTransient, code: graph.ErrorCodeInternal, messageHas: "names no producer"},
		{name: "an envelope with no relationships or producer", direction: "outgoing", outgoing: second(stubAnswer{reply: `{"data":{}}`}),
			class: errs.ErrorTransient, code: graph.ErrorCodeInternal, messageHas: "names no producer"},
		{name: "a broken incoming leg", direction: "both", outgoing: first, incoming: map[string]stubAnswer{drone2: {reply: `{"data":{}}`}},
			class: errs.ErrorTransient, code: graph.ErrorCodeInternal, messageHas: "graph.index.query.incoming"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := fixtureClient(t)
			startComponent(t, client, `{}`)
			serveEntity(t, client)
			serveHop(t, client, "outgoing", tc.outgoing)
			serveHop(t, client, "incoming", tc.incoming)
			raw, err := askPathSearch(t, client, fmt.Sprintf(`{"start_entity":%q,"direction":%q,"max_depth":3}`, drone1, tc.direction))
			got := requireRefusal(t, raw, err, tc.class, tc.code)
			if tc.detail != nil && !reflect.DeepEqual(got.Detail, tc.detail) {
				t.Errorf("detail = %v, want %v", got.Detail, tc.detail)
			}
			if !strings.Contains(err.Error(), drone2) {
				t.Errorf("refusal %q does not name the hop's entity %s", err, drone2)
			}
			if !strings.Contains(err.Error(), tc.messageHas) {
				t.Errorf("refusal %q lost %q", err, tc.messageHas)
			}
		})
	}

	// natsclient itself refuses the hop's reply: nothing here builds the refusal.
	t.Run("a hub over the reply limit", func(t *testing.T) {
		client := fixtureClient(t)
		maxPayload := client.GetConnection().MaxPayload()
		startComponent(t, client, `{}`)
		serveEntity(t, client)
		hub := strings.Repeat(outgoingRow(drone3)+",", int(maxPayload)/len(outgoingRow(drone3))+1)
		serveHop(t, client, "outgoing", second(stubAnswer{reply: hopReply(9, strings.TrimSuffix(hub, ","))}))
		raw, err := askPathSearch(t, client, fmt.Sprintf(`{"start_entity":%q,"direction":"outgoing","max_depth":3}`, drone1))
		got := requireRefusal(t, raw, err, errs.ErrorInvalid, "response_too_large")
		if got.Detail["max_payload"] != float64(maxPayload) {
			t.Errorf("detail = %v, want max_payload %d", got.Detail, maxPayload)
		}
		if !strings.Contains(err.Error(), drone2) {
			t.Errorf("refusal %q does not name the hub %s", err, drone2)
		}
	})

	t.Run("no responder on the incoming leg", func(t *testing.T) {
		client := fixtureClient(t)
		startComponent(t, client, `{}`)
		serveEntity(t, client)
		serveHop(t, client, "outgoing", first)
		raw, err := askPathSearch(t, client, fmt.Sprintf(`{"start_entity":%q,"direction":"both","max_depth":3}`, drone1))
		requireRefusal(t, raw, err, errs.ErrorTransient, graph.ErrorCodeInternal)
		for _, named := range []string{drone1, "graph.index.query.incoming"} {
			if !strings.Contains(err.Error(), named) {
				t.Errorf("refusal %q does not name %s", err, named)
			}
		}
	})

	for _, tc := range []struct{ name, reply, has string }{
		{"a start reply that is not JSON", `not json`, "graph.ingest.query.entity"},
		{"a start reply with no entity", `{"kv_revision":5}`, "holds no entity"},
		{"a start reply whose entity is null", `{"entity":null,"kv_revision":5}`, "holds no entity"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := fixtureClient(t)
			c := startComponent(t, client, `{}`)
			serve(t, client, verbRow(t, graph.ResponderGraphIngest, "entity").Subject, func(context.Context, []byte) ([]byte, error) {
				return []byte(tc.reply), nil
			})
			raw, err := askPathSearch(t, client, fmt.Sprintf(`{"start_entity":%q,"direction":"outgoing","max_depth":1}`, drone1))
			requireRefusal(t, raw, err, errs.ErrorTransient, graph.ErrorCodeInternal)
			if !strings.Contains(err.Error(), tc.has) {
				t.Errorf("refusal %q does not say %q", err, tc.has)
			}
			if got := c.Health().ErrorCount; got != 1 {
				t.Errorf("ErrorCount = %d, want 1", got)
			}
		})
	}
}

// pathRequestSpy counts every request a walk makes, beside the real responders, on the connection
// every component here shares. count sends a marker on that connection and reads the spy up to it:
// NATS orders one connection's messages, so whatever graph-query sent before it replied reaches the spy
// first, as spyOnEntityVerb relies on (its comment says how far that is shown). The marker is refused,
// which nothing reads.
type pathRequestSpy struct {
	client *natsclient.Client
	seen   map[string]chan string
}

func spyOnWalk(t *testing.T, client *natsclient.Client) *pathRequestSpy {
	t.Helper()
	spy := &pathRequestSpy{client: client, seen: map[string]chan string{}}
	for _, verb := range []graph.QueryVerb{
		verbRow(t, graph.ResponderGraphIngest, "entity"),
		verbRow(t, graph.ResponderGraphIndex, "outgoing"),
		verbRow(t, graph.ResponderGraphIndex, "incoming"),
	} {
		seen := make(chan string, 256)
		spy.seen[verb.Name] = seen
		if _, err := client.Subscribe(t.Context(), verb.Subject, func(_ context.Context, msg *nats.Msg) { seen <- string(msg.Data) }); err != nil {
			t.Fatalf("subscribe %s: %v", verb.Subject, err)
		}
	}
	return spy
}

func (s *pathRequestSpy) count(t *testing.T) map[string]int {
	t.Helper()
	const marker = `{"id":"spy-marker","entity_id":"spy-marker"}`
	counts := map[string]int{}
	for name, seen := range s.seen {
		responder := graph.ResponderGraphIndex
		if name == "entity" {
			responder = graph.ResponderGraphIngest
		}
		_, _ = s.client.RequestClassified(t.Context(), verbRow(t, responder, name).Subject, []byte(marker), requestTimeout)
		for got := <-seen; got != marker; got = <-seen {
			counts[name]++
		}
	}
	return counts
}

// TestPathSearchRequestsAreBounded: counting beside the real responders, a walk makes at most 1 +
// max_nodes requests, or 1 + 2 x max_nodes for both, and each count here is the exact one for its
// graph: the bound is met when nothing cuts the walk short, and truncation stops it (spec, "Counting
// the requests"; design D4).
func TestPathSearchRequestsAreBounded(t *testing.T) {
	stack := startStack(t)
	// A chain, drone1 to drone5, and drone6 pointing at its first three.
	stack.create(t, drone1, [2]string{linkTo, drone2})
	stack.create(t, drone2, [2]string{linkTo, drone3})
	stack.create(t, drone3, [2]string{linkTo, drone4})
	stack.create(t, drone4, [2]string{linkTo, drone5})
	stack.create(t, drone5)
	written := stack.create(t, drone6, [2]string{linkTo, drone1}, [2]string{linkTo, drone2}, [2]string{linkTo, drone3})
	stack.awaitIndexed(t, written)
	spy := spyOnWalk(t, stack.client)

	for _, tc := range []struct {
		name, start, direction string
		depth, nodes           int
		want                   map[string]int
	}{
		{"outgoing, the whole chain", drone1, "outgoing", 10, 5, map[string]int{"entity": 1, "outgoing": 5}},
		{"outgoing, max_nodes 0 reads what is there", drone1, "outgoing", 10, 0, map[string]int{"entity": 1, "outgoing": 5}},
		{"incoming, the bound met", drone5, "incoming", 10, 6, map[string]int{"entity": 1, "incoming": 6}},
		{"both, the bound met", drone1, "both", 10, 6, map[string]int{"entity": 1, "outgoing": 6, "incoming": 6}},
		{"max_depth ends the reads", drone1, "outgoing", 1, 5, map[string]int{"entity": 1, "outgoing": 1}},
		{"the first unseen entity past the limit ends the reads", drone6, "outgoing", 10, 2, map[string]int{"entity": 1, "outgoing": 1}},
		{"reads go on while the limit holds and nothing is left out", drone1, "outgoing", 10, 4, map[string]int{"entity": 1, "outgoing": 4}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := askPathSearch(t, stack.client,
				fmt.Sprintf(`{"start_entity":%q,"direction":%q,"max_depth":%d,"max_nodes":%d}`, tc.start, tc.direction, tc.depth, tc.nodes)); err != nil {
				t.Fatal(err)
			}
			got := spy.count(t)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("requests = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestSlowPathSearchLeavesOtherVerbsAnswering: while a walk waits on a hop the test holds, the entity
// verb answers; the walk then completes (spec, "A walk that is held"). query_timeout_ms is set high, so
// the release ends the wait, not that bound; natsclient's 30 s handler budget still applies, and a run
// that slow fails here loudly.
func TestSlowPathSearchLeavesOtherVerbsAnswering(t *testing.T) {
	client := fixtureClient(t)
	startComponent(t, client, `{"query_timeout_ms": 600000}`)
	serveEntity(t, client)

	entered, release := make(chan struct{}), make(chan struct{})
	releaseOnce := sync.OnceFunc(func() { close(release) })
	t.Cleanup(releaseOnce) // before the Stop and the fixture's Close
	serve(t, client, verbRow(t, graph.ResponderGraphIndex, "outgoing").Subject, func(context.Context, []byte) ([]byte, error) {
		close(entered)
		<-release
		return []byte(hopReply(9, "")), nil
	})

	type outcome struct {
		reply []byte
		err   error
	}
	walked := make(chan outcome, 1)
	go func() {
		reply, err := client.RequestClassified(t.Context(), pathSearchSubject,
			fmt.Appendf(nil, `{"start_entity":%q,"direction":"outgoing","max_depth":1}`, drone1), requestTimeout)
		walked <- outcome{reply, err}
	}()
	<-entered

	reply, err := client.RequestClassified(t.Context(), "graph.query.entity", fmt.Appendf(nil, `{"id":%q}`, drone2), requestTimeout)
	if err != nil {
		t.Fatalf("entity during a held walk: %v", err)
	}
	requireJSON(t, reply, entityReply(drone2))

	releaseOnce()
	done := <-walked
	if done.err != nil {
		t.Fatalf("the walk, released: %v", done.err)
	}
	requireJSON(t, decodeEnvelope(t, done.reply).Data,
		fmt.Sprintf(`{"entities":[{"entity_id":%q,"depth":0}],"edges":[],"truncated":false}`, drone1))
}
