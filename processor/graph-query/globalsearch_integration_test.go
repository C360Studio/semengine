//go:build integration

package graphquery

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/vocabulary"
)

const globalSearchSubject = "graph.query.globalSearch"

// askGlobal sends request to globalSearch on stack's connection.
func (s graphStack) askGlobal(t *testing.T, request string) ([]byte, error) {
	t.Helper()
	return s.client.RequestClassified(t.Context(), globalSearchSubject, []byte(request), requestTimeout)
}

// entityBytes is what the entity verb replies for id, read apart from graph-query.
func (s graphStack) entityBytes(t *testing.T, id string) []byte {
	t.Helper()
	raw, err := s.client.RequestClassified(t.Context(), verbRow(t, graph.ResponderGraphIngest, "entity").Subject,
		fmt.Appendf(nil, `{"id":%q}`, id), requestTimeout)
	if err != nil {
		t.Fatalf("direct entity read of %s: %v", id, err)
	}
	return raw
}

func aliased(alias string) [2]string { return [2]string{vocabulary.CoreIdentityAlias, alias} }

func linked(to string) [2]string { return [2]string{linkTo, to} }

// requireTextTierRefusal holds design D2's refusal: class invalid, its code and its message.
func requireTextTierRefusal(t *testing.T, question string, reply []byte, err error) {
	t.Helper()
	requireRefusal(t, reply, err, errs.ErrorInvalid, "text_search_unsupported")
	if err.Error() != textTierRefusal {
		t.Errorf("%q: message = %q, want %q", question, err.Error(), textTierRefusal)
	}
}

// TestGlobalSearchEntity: a one-token question is read through the shared resolver (design D3) and
// answered with the entity verb's reply bytes, under the alias reply's revision (design D4). A holder
// is read, and a valid ID nobody holds is read as itself. Real ingest and index, then stubs for the
// literal requests and bytes no real ingest sends.
func TestGlobalSearchEntity(t *testing.T) {
	t.Run("real ingest and index", func(t *testing.T) {
		stack := startStack(t)
		stack.create(t, drone1, aliased("held-1"))
		stack.create(t, drone2)
		later := stack.create(t, drone5)
		stack.awaitIndexed(t, later)

		for _, tc := range []struct{ question, id string }{{"held-1", drone1}, {drone2, drone2}} {
			raw, err := stack.askGlobal(t, fmt.Sprintf(`{"query":%q}`, tc.question))
			if err != nil {
				t.Fatalf("%q: %v", tc.question, err)
			}
			envelope := decodeEnvelope(t, raw)
			want := `{"strategy":"entity","entity":` + string(stack.entityBytes(t, tc.id)) + `}`
			if string(envelope.Data) != want {
				t.Errorf("%q: data = %s, want %s", tc.question, envelope.Data, want)
			}
			if envelope.IndexedRevision < later {
				t.Errorf("%q: indexed_revision = %d, below %d, which the alias verb has shown", tc.question, envelope.IndexedRevision, later)
			}
		}
	})

	t.Run("stubbed", func(t *testing.T) {
		client := fixtureClient(t)
		startComponent(t, client, `{}`)
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

		raw, err := client.RequestClassified(t.Context(), globalSearchSubject, []byte(`{"query":" held-1 ","min_revision":3}`), requestTimeout)
		if err != nil {
			t.Fatal(err)
		}
		// Each stub stores before it replies, so before the reply above.
		if got, want := aliasReceived.Load(), `{"alias":"held-1","min_revision":3}`; got == nil || string(*got) != want {
			t.Errorf("the alias verb received %s, want %q", printable(got), want)
		}
		if got, want := entityReceived.Load(), `{"id":"`+drone1+`"}`; got == nil || string(*got) != want {
			t.Errorf("the entity verb received %s, want %q", printable(got), want)
		}
		want := `{"data":{"strategy":"entity","entity":` + entityReply + `},"indexed_revision":77,"producer":"graph-query","timestamp":"`
		if !bytes.HasPrefix(raw, []byte(want)) {
			t.Errorf("reply = %q, want it to begin %q", raw, want)
		}
		decodeEnvelope(t, raw)
	})
}

// TestGlobalSearchOneWordIsAReference: a one-word question is resolved as an entity reference before
// any classification, so trigger words inside an ID or alias are not read as hints (design D1, spec "A
// one-word question is an entity reference"). Each word below is one the keyword tier reads as a time
// range (today), a path (zone-a, connector-7) or a hint no strategy honours (link, most, max, total).
// The IDs carry the reviewer's segments under the authority graph-ingest runs with here.
func TestGlobalSearchOneWordIsAReference(t *testing.T) {
	stack := startStack(t)
	const (
		todayID = "c360.test.today.sensor.temp.001"
		zoneID  = "c360.test.zone-a.sensor.temp.001"
		linkID  = "c360.test.link.sensor.temp.001"
		mostID  = "c360.test.most.sensor.temp.001"
	)
	for _, id := range []string{todayID, zoneID, linkID, mostID} {
		stack.create(t, id)
	}
	stack.create(t, drone1, aliased("connector-7"))
	stack.create(t, drone2, aliased("max-pump"))
	later := stack.create(t, drone3, aliased("total-flow"))
	stack.awaitIndexed(t, later)

	for _, tc := range []struct{ question, id string }{
		{todayID, todayID}, {zoneID, zoneID}, {linkID, linkID}, {mostID, mostID},
		{"connector-7", drone1}, {"max-pump", drone2}, {" total-flow\t", drone3},
	} {
		raw, err := stack.askGlobal(t, fmt.Sprintf(`{"query":%q}`, tc.question))
		if err != nil {
			t.Errorf("%q: %v", tc.question, err)
			continue
		}
		want := `{"strategy":"entity","entity":` + string(stack.entityBytes(t, tc.id)) + `}`
		if got := string(decodeEnvelope(t, raw).Data); got != want {
			t.Errorf("%q: data = %s, want %s", tc.question, got, want)
		}
	}
}

// TestGlobalSearchPath: a path question is answered by step 2's walk from the resolved start, in both
// directions, to depth 3 and 100 entities (design D4): the reply's path is pathSearch's data for the
// same request, and the literal walk below. Stubbed, the requests are literal, min_revision reaches the
// alias read and each hop, and the revision is the walk's, not the alias reply's.
func TestGlobalSearchPath(t *testing.T) {
	t.Run("real ingest and index", func(t *testing.T) {
		stack := startStack(t)
		stack.create(t, drone5)
		stack.create(t, drone3, linked(drone5))
		stack.create(t, drone2, linked(drone3))
		stack.create(t, drone4, linked(drone1))
		written := stack.create(t, drone1, aliased("held-1"), linked(drone2))
		stack.awaitIndexed(t, written)

		direct, err := stack.client.RequestClassified(t.Context(), "graph.query.pathSearch",
			fmt.Appendf(nil, `{"start_entity":%q,"direction":"both","max_depth":3,"max_nodes":100}`, drone1), requestTimeout)
		if err != nil {
			t.Fatalf("pathSearch: %v", err)
		}
		pathData := string(decodeEnvelope(t, direct).Data)
		edge := func(from, to string) string {
			return `{"predicate":"` + linkTo + `","from_entity_id":"` + from + `","to_entity_id":"` + to + `"}`
		}
		node := func(id string, depth int) string { return fmt.Sprintf(`{"entity_id":%q,"depth":%d}`, id, depth) }
		walk := `{"entities":[` + strings.Join([]string{node(drone1, 0), node(drone2, 1), node(drone4, 1), node(drone3, 2), node(drone5, 3)}, ",") +
			`],"edges":[` + strings.Join([]string{edge(drone1, drone2), edge(drone4, drone1), edge(drone2, drone3), edge(drone3, drone5)}, ",") +
			`],"truncated":false}`
		if pathData != walk {
			t.Fatalf("pathSearch data = %s, want %s", pathData, walk)
		}

		raw, err := stack.askGlobal(t, `{"query":"what is connected to held-1"}`)
		if err != nil {
			t.Fatal(err)
		}
		envelope := decodeEnvelope(t, raw)
		if want := `{"strategy":"path","path":` + pathData + `}`; string(envelope.Data) != want {
			t.Errorf("data = %s, want %s", envelope.Data, want)
		}
		if envelope.IndexedRevision < written {
			t.Errorf("indexed_revision = %d, below the write's %d", envelope.IndexedRevision, written)
		}
	})

	t.Run("stubbed", func(t *testing.T) {
		client := fixtureClient(t)
		startComponent(t, client, `{}`)
		received := make(map[string]*atomic.Pointer[[]byte])
		for _, name := range []string{"alias", "outgoing", "incoming"} {
			received[name] = new(atomic.Pointer[[]byte])
		}
		received["entity"] = new(atomic.Pointer[[]byte])
		serve(t, client, verbRow(t, graph.ResponderGraphIndex, "alias").Subject, func(_ context.Context, data []byte) ([]byte, error) {
			received["alias"].Store(&data)
			return aliasReply(drone1).answer()
		})
		serve(t, client, verbRow(t, graph.ResponderGraphIngest, "entity").Subject, func(_ context.Context, data []byte) ([]byte, error) {
			received["entity"].Store(&data)
			return []byte(`{"entity":{"id":"` + drone1 + `"},"kv_revision":5}`), nil
		})
		for name, revision := range map[string]int{"outgoing": 78, "incoming": 79} {
			serve(t, client, verbRow(t, graph.ResponderGraphIndex, name).Subject, func(_ context.Context, data []byte) ([]byte, error) {
				received[name].Store(&data)
				return fmt.Appendf(nil, `{"data":{"relationships":[]},"indexed_revision":%d,"producer":"graph-index"}`, revision), nil
			})
		}

		raw, err := client.RequestClassified(t.Context(), globalSearchSubject, []byte(`{"query":"what is connected to held-1","min_revision":7}`), requestTimeout)
		if err != nil {
			t.Fatal(err)
		}
		// Each stub stores before it replies, so before the reply above.
		for name, want := range map[string]string{
			"alias":    `{"alias":"held-1","min_revision":7}`,
			"entity":   `{"id":"` + drone1 + `"}`,
			"outgoing": `{"entity_id":"` + drone1 + `","min_revision":7}`,
			"incoming": `{"entity_id":"` + drone1 + `","min_revision":7}`,
		} {
			if got := received[name].Load(); got == nil || string(*got) != want {
				t.Errorf("the %s verb received %s, want %q", name, printable(got), want)
			}
		}
		want := `{"data":{"strategy":"path","path":{"entities":[{"entity_id":"` + drone1 + `","depth":0}],"edges":[],"truncated":false}},"indexed_revision":78,"producer":"graph-query","timestamp":"`
		if !bytes.HasPrefix(raw, []byte(want)) {
			t.Errorf("reply = %q, want it to begin %q", raw, want)
		}
	})
}

// TestGlobalSearchPassesAliasRefusals: a refusal from the alias verb, or from the entity verb other
// than its entity_not_found, reaches the caller with its class, code, detail and message, for both
// strategies, and nothing more is asked (design D3, ADR-114). The pin's resolver dropped every one and
// fell through to another strategy (pin:entity_resolver.go). The entity verb's fatal refusal carries
// the pin's TestHandleGlobalSearchDoesNotFallbackAfterAuthoritativePoison and
// TestEntityLookupStrategyDoesNotFallThroughAfterAuthoritativePoison (query_error_class_test.go:152-220
// at 8b99efe9): a refusal that is authoritative is never permission to answer some other way.
func TestGlobalSearchPassesAliasRefusals(t *testing.T) {
	questions := []string{`{"query":"shared-1"}`, `{"query":"what is connected to shared-1"}`}

	t.Run("a real shared alias", func(t *testing.T) {
		stack := startStack(t)
		stack.create(t, drone3, aliased("shared-1"))
		stack.awaitIndexed(t, stack.create(t, drone4, aliased("shared-1")))
		for _, request := range questions {
			noEntityRequest := spyOnEntityVerb(t, stack.client)
			raw, err := stack.askGlobal(t, request)
			requireRefusal(t, raw, err, errs.ErrorInvalid, "alias_ambiguous")
			if !strings.Contains(err.Error(), `"shared-1"`) {
				t.Errorf("%s: message %q does not name the alias", request, err)
			}
			noEntityRequest()
		}
	})

	cases := []struct {
		name          string
		alias, entity stubAnswer
		class         errs.ErrorClass
		code          string
		detail        map[string]any // as it decodes
		message       string
		entityReads   int64
	}{
		{name: "alias_ambiguous",
			alias: stubAnswer{refusal: errs.ClassifiedCodeDetail(errs.ErrorInvalid, "alias_ambiguous",
				map[string]any{"alias": "shared-1"}, errors.New(`alias "shared-1" is held by two entities`))},
			class: errs.ErrorInvalid, code: "alias_ambiguous", detail: map[string]any{"alias": "shared-1"},
			message: `alias "shared-1" is held by two entities`},
		{name: "index_not_ready",
			alias: stubAnswer{refusal: errs.ClassifiedCodeDetail(errs.ErrorTransient, "index_not_ready",
				map[string]any{"bucket": "ALIAS_INDEX"}, errors.New("still replaying"))},
			class: errs.ErrorTransient, code: "index_not_ready", detail: map[string]any{"bucket": "ALIAS_INDEX"},
			message: "still replaying"},
		{name: "internal",
			alias: stubAnswer{refusal: errs.ClassifiedCodeDetail(errs.ErrorTransient, "internal",
				map[string]any{"at": "index"}, errors.New("internal error: boom"))},
			class: errs.ErrorTransient, code: "internal", detail: map[string]any{"at": "index"},
			message: "internal error: boom"},
		{name: "revision_not_reached",
			alias: stubAnswer{refusal: errs.ClassifiedCodeDetail(errs.ErrorTransient, "revision_not_reached",
				map[string]any{"indexed_revision": 7}, errors.New("not yet"))},
			class: errs.ErrorTransient, code: "revision_not_reached", detail: map[string]any{"indexed_revision": float64(7)},
			message: "not yet"},
		{name: "the entity verb's fatal refusal", alias: aliasReply(drone1),
			entity: stubAnswer{refusal: errs.ClassifiedCodeDetail(errs.ErrorFatal, graph.ErrorCodeGraphStateResetRequired,
				map[string]any{"entity_id": drone1}, errors.New("stored state is unreadable"))},
			class: errs.ErrorFatal, code: graph.ErrorCodeGraphStateResetRequired, detail: map[string]any{"entity_id": drone1},
			message: "stored state is unreadable", entityReads: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := fixtureClient(t)
			c := startComponent(t, client, `{}`)
			var entityReads, hops atomic.Int64
			serve(t, client, verbRow(t, graph.ResponderGraphIndex, "alias").Subject, func(context.Context, []byte) ([]byte, error) {
				return tc.alias.answer()
			})
			serve(t, client, verbRow(t, graph.ResponderGraphIngest, "entity").Subject, func(context.Context, []byte) ([]byte, error) {
				entityReads.Add(1)
				return tc.entity.answer()
			})
			for _, name := range []string{"outgoing", "incoming"} {
				serve(t, client, verbRow(t, graph.ResponderGraphIndex, name).Subject, func(context.Context, []byte) ([]byte, error) {
					hops.Add(1)
					return []byte(`{"data":{"relationships":[]},"producer":"graph-index"}`), nil
				})
			}
			for i, request := range questions {
				raw, err := client.RequestClassified(t.Context(), globalSearchSubject, []byte(request), requestTimeout)
				got := requireRefusal(t, raw, err, tc.class, tc.code)
				if err.Error() != tc.message {
					t.Errorf("%s: message = %q, want %q", request, err.Error(), tc.message)
				}
				if !reflect.DeepEqual(got.Detail, tc.detail) {
					t.Errorf("%s: detail = %v, want %v", request, got.Detail, tc.detail)
				}
				// Each stub counts before it replies, so before the refusal above.
				if n, want := entityReads.Load(), tc.entityReads*int64(i+1); n != want {
					t.Errorf("%s: the entity verb has received %d requests, want %d", request, n, want)
				}
				if n := hops.Load(); n != 0 {
					t.Errorf("%s: the index's edge verbs received %d requests, want none", request, n)
				}
			}
			// A responder's refusal is an answer: no error in Health.
			if got := c.Health().ErrorCount; got != 0 {
				t.Errorf("ErrorCount = %d, want 0", got)
			}
		})
	}
}

// TestGlobalSearchUnknownReferenceFallsThrough: a reference that names no entity, an unknown alias
// that is no ID or a valid ID with no record, is not an answer to the question: it is refused as the
// text tier's, never entity_not_found, and counts no error (design D3). A non-ID nobody holds asks
// nothing of the entity verb.
func TestGlobalSearchUnknownReferenceFallsThrough(t *testing.T) {
	t.Run("real ingest and index", func(t *testing.T) {
		stack := startIngestAndIndex(t)
		c := startComponent(t, stack.client, `{}`)
		stack.awaitIndexed(t, stack.create(t, drone1, aliased("held-1")))
		for _, tc := range []struct {
			question      string
			readsAnEntity bool
		}{
			{"forklift", false},
			{"what is connected to nobody-1", false},
			{absentDrone, true},
		} {
			check := func() {}
			if !tc.readsAnEntity {
				check = spyOnEntityVerb(t, stack.client)
			}
			raw, err := stack.askGlobal(t, fmt.Sprintf(`{"query":%q}`, tc.question))
			requireTextTierRefusal(t, tc.question, raw, err)
			check()
		}
		if got := c.Health().ErrorCount; got != 0 {
			t.Errorf("ErrorCount = %d, want 0", got)
		}
	})

	t.Run("a holder whose entity is gone", func(t *testing.T) {
		client := fixtureClient(t)
		c := startComponent(t, client, `{}`)
		var hops atomic.Int64
		serve(t, client, verbRow(t, graph.ResponderGraphIndex, "alias").Subject, func(context.Context, []byte) ([]byte, error) {
			return aliasReply(drone1).answer()
		})
		serve(t, client, verbRow(t, graph.ResponderGraphIngest, "entity").Subject, func(context.Context, []byte) ([]byte, error) {
			return nil, errs.ClassifiedCode(errs.ErrorInvalid, "entity_not_found", errors.New("not found: "+drone1))
		})
		for _, name := range []string{"outgoing", "incoming"} {
			serve(t, client, verbRow(t, graph.ResponderGraphIndex, name).Subject, func(context.Context, []byte) ([]byte, error) {
				hops.Add(1)
				return []byte(`{"data":{"relationships":[]},"producer":"graph-index"}`), nil
			})
		}
		for _, question := range []string{"held-1", "what is connected to held-1"} {
			raw, err := client.RequestClassified(t.Context(), globalSearchSubject, fmt.Appendf(nil, `{"query":%q}`, question), requestTimeout)
			requireTextTierRefusal(t, question, raw, err)
		}
		// Each stub counts before it replies, so before the refusals above.
		if n := hops.Load(); n != 0 {
			t.Errorf("the index's edge verbs received %d requests, want none", n)
		}
		if got := c.Health().ErrorCount; got != 0 {
			t.Errorf("ErrorCount = %d, want 0", got)
		}
	})
}

// TestZoneQuestionNeverAnswersEmpty: #479 item 3. The keyword tier reads "the area of the plant" as a
// path from area-of (notes/probes/step4-tree-probe.log); no entity holds that alias, so the question
// is refused as the text tier's. A started pin component fell through to its community search: it built
// the cache before the responders (pin:component.go:556), so the empty pathrag answer
// (pin:graphrag.go:475-489) ran only without a cache, outside a successful Start.
func TestZoneQuestionNeverAnswersEmpty(t *testing.T) {
	stack := startStack(t)
	stack.awaitIndexed(t, stack.create(t, drone1, aliased("held-1")))
	raw, err := stack.askGlobal(t, `{"query":"the area of the plant"}`)
	requireTextTierRefusal(t, "the area of the plant", raw, err)
}

// TestGlobalSearchRefusesADroppedPlace: a path question that also names a zone ("what is connected to
// sensor-001 in zone-b") names two places, and the keyword tier reads one start, so the question is
// refused invalid_request naming both, never answered as the walk from one (PR #530 review, #535).
// Two zones written with spaces ("zone a in zone b") are refused the same way, each clause alone
// answered first (PR #530 re-review), and so is a full ID with no hyphenated segment beside a zone
// (owner ruling 2026-11-07). The zone alone is still answered, from its own holder. Real ingest,
// index and query.
func TestGlobalSearchRefusesADroppedPlace(t *testing.T) {
	stack := startStack(t)
	stack.create(t, drone2)
	stack.create(t, drone3, aliased("zone-b"))
	stack.create(t, drone4, aliased("zone-a"), linked(drone2))
	stack.awaitIndexed(t, stack.create(t, drone1, aliased("sensor-001"), linked(drone2)))

	zone, err := stack.askGlobal(t, `{"query":"what is in zone-b"}`)
	if err != nil {
		t.Fatalf("the zone alone: %v", err)
	}
	if data := decodeEnvelope(t, zone).Data; !bytes.Contains(data, []byte(drone3)) || bytes.Contains(data, []byte(drone1)) {
		t.Fatalf("the zone alone answered %s, want the walk from %s", data, drone3)
	}
	// Each spaced clause alone is answered, so a refusal of the pair below is not an unread name.
	for _, tc := range []struct{ question, holder, other string }{
		{"what is connected to zone a", drone4, drone3},
		{"what is in zone b", drone3, drone4},
	} {
		raw, err := stack.askGlobal(t, fmt.Sprintf(`{"query":%q}`, tc.question))
		if err != nil {
			t.Fatalf("%q alone: %v", tc.question, err)
		}
		if data := decodeEnvelope(t, raw).Data; !bytes.Contains(data, []byte(tc.holder)) || bytes.Contains(data, []byte(tc.other)) {
			t.Fatalf("%q alone answered %s, want the walk from %s", tc.question, data, tc.holder)
		}
	}

	for _, phrase := range []string{"in zone-b", "near zone-b"} {
		t.Run(phrase, func(t *testing.T) {
			raw, err := stack.askGlobal(t, fmt.Sprintf(`{"query":%q}`, "what is connected to sensor-001 "+phrase))
			requireRefusal(t, raw, err, errs.ErrorInvalid, graph.ErrorCodeInvalidRequest)
			const want = "the question names two places, sensor-001 and zone-b; ask about one"
			if msg := err.Error(); !strings.HasPrefix(msg, "invalid request: ") || !strings.HasSuffix(msg, want) {
				t.Errorf("message = %q, want invalid request ending %q", msg, want)
			}
		})
	}
	// PR #530 re-review: two zones written with spaces, each answered alone above.
	t.Run("zone a in zone b", func(t *testing.T) {
		raw, err := stack.askGlobal(t, `{"query":"what is connected to zone a in zone b"}`)
		requireRefusal(t, raw, err, errs.ErrorInvalid, graph.ErrorCodeInvalidRequest)
		const want = "the question names two places, zone-a and zone-b; ask about one"
		if msg := err.Error(); !strings.HasPrefix(msg, "invalid request: ") || !strings.HasSuffix(msg, want) {
			t.Errorf("message = %q, want invalid request ending %q", msg, want)
		}
	})
	// Owner ruling 2026-11-07: a full ID with no hyphenated segment beside a zone is a second place.
	// The ID alone is answered as its entity and "what is in zone b" alone above, so the refusal is
	// not an unread name.
	t.Run("full ID in zone b", func(t *testing.T) {
		raw, err := stack.askGlobal(t, fmt.Sprintf(`{"query":%q}`, drone1))
		if err != nil {
			t.Fatalf("%s alone: %v", drone1, err)
		}
		want := `{"strategy":"entity","entity":` + string(stack.entityBytes(t, drone1)) + `}`
		if got := string(decodeEnvelope(t, raw).Data); got != want {
			t.Fatalf("%s alone: data = %s, want %s", drone1, got, want)
		}
		raw, err = stack.askGlobal(t, fmt.Sprintf(`{"query":%q}`, "what is connected to "+drone1+" in zone b"))
		requireRefusal(t, raw, err, errs.ErrorInvalid, graph.ErrorCodeInvalidRequest)
		wantMsg := "the question names two places, " + drone1 + " and zone-b; ask about one"
		if msg := err.Error(); !strings.HasPrefix(msg, "invalid request: ") || !strings.HasSuffix(msg, wantMsg) {
			t.Errorf("message = %q, want invalid request ending %q", msg, wantMsg)
		}
	})
}

// TestGlobalSearchTextTierRefusal: a question only the text search could answer is refused class
// invalid, code text_search_unsupported, whether or not COMMUNITY_INDEX holds records, and counts one
// input and no error (design D2; spec, "A question this build cannot answer").
func TestGlobalSearchTextTierRefusal(t *testing.T) {
	questions := []string{"forklift", "how many pumps are there", "find sensors similar to pump-42"}
	check := func(t *testing.T, stack graphStack, c *Component) {
		t.Helper()
		before, errorsBefore := c.DataFlow().Messages, c.Health().ErrorCount
		for _, question := range questions {
			raw, err := stack.askGlobal(t, fmt.Sprintf(`{"query":%q}`, question))
			requireTextTierRefusal(t, question, raw, err)
		}
		// Each handler counts before it returns, so before its reply.
		if got := c.DataFlow().Messages - before; got != int64(len(questions)) {
			t.Errorf("DataFlow counted %d requests, want %d", got, len(questions))
		}
		if got := c.Health().ErrorCount; got != errorsBefore {
			t.Errorf("ErrorCount = %d, want %d", got, errorsBefore)
		}
	}

	t.Run("no community records", func(t *testing.T) {
		stack := startIngestAndIndex(t)
		c := startComponent(t, stack.client, `{}`)
		stack.awaitIndexed(t, stack.create(t, drone1, aliased("held-1")))
		check(t, stack, c)
	})
	t.Run("community records", func(t *testing.T) {
		stack := startCommunityStack(t)
		written := stack.write(t, drone1, aliased("held-1"))
		tick(t, stack.maintainer)
		stack.awaitIndexed(t, written)
		awaitCommunityView(t, stack.client, drone1, written)
		check(t, stack.graphStack, stack.component)
	})
}
