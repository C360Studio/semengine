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

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/natsclient"
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

// stubTextSearch stands in for graph-lexical's word search, answering no hits, and returns a check
// giving the requests it has received since the last: a question that reaches it was searched as text.
func stubTextSearch(t *testing.T, client *natsclient.Client) func() []string {
	t.Helper()
	stubWordIndex(t, client, lexicalReply(t, 1, graph.LexicalSearchData{}))
	return spyOnDownstream(t, client, verbRow(t, graph.ResponderGraphLexical, "search").Subject)
}

// requireTextSearched holds that question was searched for its words, one word search of the
// question as asked, and answered with text alone and hints exactly want.
func requireTextSearched(t *testing.T, question string, reply []byte, err error, asked func() []string, want ...graph.GlobalSearchHint) {
	t.Helper()
	if err != nil {
		t.Fatalf("%q: %v, want a text answer", question, err)
	}
	decodeTextAnswer(t, reply)
	_, data := decodeGlobal(t, reply)
	requireHintList(t, question, data.Hints, want)
	if got := asked(); len(got) != 1 {
		t.Errorf("%q: word searches = %q, want one", question, got)
	} else {
		requireSameJSON(t, []byte(got[0]), fmt.Sprintf(`{"query":%q,"limit":0}`, question))
	}
}

// decodeGlobal decodes a globalSearch success strictly, the envelope and then its data, and fails
// unless the data holds text: every answer does.
func decodeGlobal(t *testing.T, raw []byte) (wireEnvelope, graph.GlobalSearchData) {
	t.Helper()
	envelope := decodeEnvelope(t, raw)
	var data graph.GlobalSearchData
	decoder := json.NewDecoder(bytes.NewReader(envelope.Data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&data); err != nil {
		t.Fatalf("decode data %s: %v", envelope.Data, err)
	}
	if data.Text == nil || data.Hints == nil {
		t.Fatalf("data = %s, want text and hints", envelope.Data)
	}
	return envelope, data
}

// requireHintList holds that a reply lists exactly want, in order.
func requireHintList(t *testing.T, question string, got, want []graph.GlobalSearchHint) {
	t.Helper()
	if len(want) == 0 {
		want = []graph.GlobalSearchHint{}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%q: hints = %+v, want %+v", question, got, want)
	}
}

// notFoundHint is a reference's miss as a hint: not honoured, entity_not_found, as entityByAlias
// would refuse it.
func notFoundHint(kind, reference, named string) graph.GlobalSearchHint {
	return graph.GlobalSearchHint{Hint: kind, Detail: reference, Reason: "not found: " + named, Code: graph.ErrorCodeEntityNotFound}
}

// startSearchStack is real graph-ingest, graph-index and graph-lexical behind graph-query.
func startSearchStack(t *testing.T) (graphStack, *Component) {
	t.Helper()
	stack := startIngestAndIndex(t)
	startWordIndex(t, stack.client)
	return stack, startComponent(t, stack.client, `{}`)
}

// awaitSearchable waits until both indexes have applied revision.
func (s graphStack) awaitSearchable(t *testing.T, revision uint64) {
	t.Helper()
	s.awaitIndexed(t, revision)
	awaitWordIndexed(t, s.client, revision)
}

// emptyWordSearch is the word index's reply with no hits, at revision 9, and emptyText the text a
// global search answers from it.
const emptyText = `{"level":0,"groups":[],"matches":[],"matched":0,"missing":0,"bodies_not_indexed":0,` +
	`"truncated":false,"unreadable_groups":0,"answer":"Found 0 matching entities in 0 groups."}`

func emptyWordSearch(t *testing.T, client *natsclient.Client) {
	t.Helper()
	stubWordIndex(t, client, lexicalReply(t, 9, graph.LexicalSearchData{}))
}

// TestGlobalSearchEntity: a one-token question is read through the shared resolver and, when it
// names an entity, the entity verb's reply bytes are added beside the text, its hint honoured. A
// holder is read, and a valid ID nobody holds is read as itself. Real ingest, index and word index,
// then stubs for the literal requests and bytes no real ingest sends; the reply's revision is the
// word index's.
func TestGlobalSearchEntity(t *testing.T) {
	t.Run("real ingest and index", func(t *testing.T) {
		stack, _ := startSearchStack(t)
		stack.create(t, drone1, aliased("held-1"))
		stack.create(t, drone2)
		stack.awaitSearchable(t, stack.create(t, drone5))

		for _, tc := range []struct{ question, id string }{{"held-1", drone1}, {drone2, drone2}} {
			raw, err := stack.askGlobal(t, fmt.Sprintf(`{"query":%q}`, tc.question))
			if err != nil {
				t.Fatalf("%q: %v", tc.question, err)
			}
			_, data := decodeGlobal(t, raw)
			if want := stack.entityBytes(t, tc.id); string(data.Entity) != string(want) || data.Path != nil {
				t.Errorf("%q: entity = %s, path %v; want %s and no path", tc.question, data.Entity, data.Path, want)
			}
			requireHintList(t, tc.question, data.Hints, []graph.GlobalSearchHint{{Hint: "entity", Detail: tc.question, Honoured: true}})
		}
	})

	t.Run("stubbed", func(t *testing.T) {
		client := fixtureClient(t)
		emptyWordSearch(t, client)
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
		want := `{"data":{"text":` + emptyText + `,"entity":` + entityReply + `,"hints":[{"hint":"entity","detail":"held-1","honoured":true}]},` +
			`"indexed_revision":9,"producer":"graph-query","timestamp":"`
		if !bytes.HasPrefix(raw, []byte(want)) {
			t.Errorf("reply = %q, want it to begin %q", raw, want)
		}
		decodeGlobal(t, raw)
	})
}

// TestGlobalSearchOneWordIsAReference: a one-word question is resolved as an entity reference and
// not classified, so trigger words inside an ID or alias are not read as hints. Each word below is
// one the keyword classifier reads as a time range (today), a path (zone-a, connector-7) or another
// hint (link, most, max, total). Four IDs carry these words as segments, under the authority
// graph-ingest runs with here, and three aliases carry the rest.
func TestGlobalSearchOneWordIsAReference(t *testing.T) {
	stack, _ := startSearchStack(t)
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
	stack.awaitSearchable(t, stack.create(t, drone3, aliased("total-flow")))

	for _, tc := range []struct{ question, id string }{
		{todayID, todayID}, {zoneID, zoneID}, {linkID, linkID}, {mostID, mostID},
		{"connector-7", drone1}, {"max-pump", drone2}, {" total-flow\t", drone3},
	} {
		raw, err := stack.askGlobal(t, fmt.Sprintf(`{"query":%q}`, tc.question))
		if err != nil {
			t.Errorf("%q: %v", tc.question, err)
			continue
		}
		_, data := decodeGlobal(t, raw)
		if want := stack.entityBytes(t, tc.id); string(data.Entity) != string(want) {
			t.Errorf("%q: entity = %s, want %s", tc.question, data.Entity, want)
		}
		requireHintList(t, tc.question, data.Hints,
			[]graph.GlobalSearchHint{{Hint: "entity", Detail: strings.TrimSpace(tc.question), Honoured: true}})
	}
}

// TestVerifiedStartAddsAWalk: a path question whose start resolves gets the path search's walk from
// it beside the text, in both directions, to depth 3 and 100 entities, its hint honoured: the
// reply's path is pathSearch's data for the same request, and the literal walk below (spec, "A start
// that resolves"). Stubbed, the requests are literal, min_revision reaches the alias read and each
// hop, and the reply's revision is the word index's, not the walk's.
func TestVerifiedStartAddsAWalk(t *testing.T) {
	t.Run("real ingest and index", func(t *testing.T) {
		stack, _ := startSearchStack(t)
		stack.create(t, drone5)
		stack.create(t, drone3, linked(drone5))
		stack.create(t, drone2, linked(drone3))
		stack.create(t, drone4, linked(drone1))
		stack.awaitSearchable(t, stack.create(t, drone1, aliased("pump-7"), linked(drone2)))

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

		const question = "what is connected to pump-7"
		raw, err := stack.askGlobal(t, fmt.Sprintf(`{"query":%q}`, question))
		if err != nil {
			t.Fatal(err)
		}
		_, data := decodeGlobal(t, raw)
		got, err := json.Marshal(data.Path)
		if err != nil || string(got) != pathData || data.Entity != nil {
			t.Errorf("path = %s, entity %s; want %s and no entity", got, data.Entity, pathData)
		}
		requireHintList(t, question, data.Hints, []graph.GlobalSearchHint{{Hint: "path", Detail: "pump-7", Honoured: true}})
	})

	t.Run("stubbed", func(t *testing.T) {
		client := fixtureClient(t)
		emptyWordSearch(t, client)
		startComponent(t, client, `{}`)
		received := map[string]*atomic.Pointer[[]byte]{}
		for _, name := range []string{"alias", "entity", "outgoing", "incoming"} {
			received[name] = new(atomic.Pointer[[]byte])
		}
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
		want := `{"data":{"text":` + emptyText + `,"path":{"entities":[{"entity_id":"` + drone1 + `","depth":0}],"edges":[],"truncated":false},` +
			`"hints":[{"hint":"path","detail":"held-1","honoured":true}]},"indexed_revision":9,"producer":"graph-query","timestamp":"`
		if !bytes.HasPrefix(raw, []byte(want)) {
			t.Errorf("reply = %q, want it to begin %q", raw, want)
		}
	})
}

// TestUnverifiedStartIsReported: a path question whose start names nothing is answered with its
// text, and the path hint is not honoured with code entity_not_found: no entity read and no walk
// is made, and no error is counted (spec, "A start that does not resolve").
func TestUnverifiedStartIsReported(t *testing.T) {
	stack, c := startSearchStack(t)
	stack.awaitSearchable(t, stack.create(t, drone1, aliased("held-1")))
	reads := spyOnWalk(t, stack.client)
	const question = "what is connected to pump-99"
	raw, err := stack.askGlobal(t, fmt.Sprintf(`{"query":%q}`, question))
	if err != nil {
		t.Fatalf("%q: %v", question, err)
	}
	_, data := decodeGlobal(t, raw)
	if data.Path != nil || data.Entity != nil {
		t.Errorf("%q: path %v, entity %s; want neither", question, data.Path, data.Entity)
	}
	requireHintList(t, question, data.Hints, []graph.GlobalSearchHint{notFoundHint("path", "pump-99", "pump-99")})
	if got := reads.count(t); len(got) != 0 {
		t.Errorf("%q: entity and edge requests %v, want none", question, got)
	}
	if got := c.Health().ErrorCount; got != 0 {
		t.Errorf("ErrorCount = %d, want 0", got)
	}
}

// TestGlobalSearchPassesAliasRefusals: a refusal from the alias verb, or from the entity verb other
// than its entity_not_found, is the hint's code and reason, for an entity hint and a path hint
// alike, and the question is still answered with its text: a hint's refusal never refuses the reply
// (ADR-114 keeps the refusal's code; it is reported, not acted on). The entity verb's fatal refusal
// is among them, and no walk is made past it.
func TestGlobalSearchPassesAliasRefusals(t *testing.T) {
	questions := []struct{ request, hint, reference string }{
		{`{"query":"shared-1"}`, "entity", "shared-1"},
		{`{"query":"what is connected to shared-1"}`, "path", "shared-1"},
	}

	t.Run("a real shared alias", func(t *testing.T) {
		stack, _ := startSearchStack(t)
		stack.create(t, drone3, aliased("shared-1"))
		stack.awaitSearchable(t, stack.create(t, drone4, aliased("shared-1")))
		for _, q := range questions {
			noEntityRequest := spyOnEntityVerb(t, stack.client)
			raw, err := stack.askGlobal(t, q.request)
			if err != nil {
				t.Fatalf("%s: %v, want an answer", q.request, err)
			}
			_, data := decodeGlobal(t, raw)
			if len(data.Hints) != 1 || data.Hints[0].Code != "alias_ambiguous" || data.Hints[0].Honoured ||
				!strings.Contains(data.Hints[0].Reason, `"shared-1"`) || data.Entity != nil || data.Path != nil {
				t.Errorf("%s: data %+v, want one alias_ambiguous hint naming the alias, nothing more", q.request, data)
			}
			noEntityRequest()
		}
	})

	cases := []struct {
		name          string
		alias, entity stubAnswer
		code          string
		message       string
		entityReads   int64
	}{
		{name: "alias_ambiguous",
			alias: stubAnswer{refusal: errs.ClassifiedCodeDetail(errs.ErrorInvalid, "alias_ambiguous",
				map[string]any{"alias": "shared-1"}, errors.New(`alias "shared-1" is held by two entities`))},
			code: "alias_ambiguous", message: `alias "shared-1" is held by two entities`},
		{name: "index_not_ready",
			alias: stubAnswer{refusal: errs.ClassifiedCodeDetail(errs.ErrorTransient, "index_not_ready",
				map[string]any{"bucket": "ALIAS_INDEX"}, errors.New("still replaying"))},
			code: "index_not_ready", message: "still replaying"},
		{name: "internal",
			alias: stubAnswer{refusal: errs.ClassifiedCodeDetail(errs.ErrorTransient, "internal",
				map[string]any{"at": "index"}, errors.New("internal error: boom"))},
			code: "internal", message: "internal error: boom"},
		{name: "revision_not_reached",
			alias: stubAnswer{refusal: errs.ClassifiedCodeDetail(errs.ErrorTransient, "revision_not_reached",
				map[string]any{"indexed_revision": 7}, errors.New("not yet"))},
			code: "revision_not_reached", message: "not yet"},
		{name: "the entity verb's fatal refusal", alias: aliasReply(drone1),
			entity: stubAnswer{refusal: errs.ClassifiedCodeDetail(errs.ErrorFatal, graph.ErrorCodeGraphStateResetRequired,
				map[string]any{"entity_id": drone1}, errors.New("stored state is unreadable"))},
			code: graph.ErrorCodeGraphStateResetRequired, message: "stored state is unreadable", entityReads: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := fixtureClient(t)
			emptyWordSearch(t, client)
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
			for i, q := range questions {
				raw, err := client.RequestClassified(t.Context(), globalSearchSubject, []byte(q.request), requestTimeout)
				if err != nil {
					t.Fatalf("%s: %v, want an answer", q.request, err)
				}
				_, data := decodeGlobal(t, raw)
				if data.Entity != nil || data.Path != nil {
					t.Errorf("%s: entity %s, path %v; want neither", q.request, data.Entity, data.Path)
				}
				requireHintList(t, q.request, data.Hints, []graph.GlobalSearchHint{{Hint: q.hint, Detail: q.reference, Reason: tc.message, Code: tc.code}})
				// Each stub counts before it replies, so before the reply above.
				if n, want := entityReads.Load(), tc.entityReads*int64(i+1); n != want {
					t.Errorf("%s: the entity verb has received %d requests, want %d", q.request, n, want)
				}
				if n := hops.Load(); n != 0 {
					t.Errorf("%s: the index's edge verbs received %d requests, want none", q.request, n)
				}
			}
			// A responder's refusal is an answer: no error in Health.
			if got := c.Health().ErrorCount; got != 0 {
				t.Errorf("ErrorCount = %d, want 0", got)
			}
		})
	}
}

// TestGlobalSearchUnknownWordIsSearched: a one-word reference that names no entity, no alias holder
// and no valid ID, is searched for its words with its entity hint not honoured, asks nothing of the
// entity verb and counts no error (spec, "An unknown reference").
func TestGlobalSearchUnknownWordIsSearched(t *testing.T) {
	stack := startIngestAndIndex(t)
	asked := stubTextSearch(t, stack.client)
	c := startComponent(t, stack.client, `{}`)
	stack.awaitIndexed(t, stack.create(t, drone1, aliased("held-1")))
	check := spyOnEntityVerb(t, stack.client)
	raw, err := stack.askGlobal(t, `{"query":"forklift"}`)
	requireTextSearched(t, "forklift", raw, err, asked, notFoundHint("entity", "forklift", "forklift"))
	check()
	if got := c.Health().ErrorCount; got != 0 {
		t.Errorf("ErrorCount = %d, want 0", got)
	}
}

// TestGlobalSearchUnknownIDIsSearched: a one-word reference that is a valid ID with no record, or
// an alias whose holder is gone, is searched for its words, its entity hint not honoured with the
// entity verb's entity_not_found, and counts no error (spec, "An unknown reference").
func TestGlobalSearchUnknownIDIsSearched(t *testing.T) {
	t.Run("real ingest and index", func(t *testing.T) {
		stack := startIngestAndIndex(t)
		asked := stubTextSearch(t, stack.client)
		c := startComponent(t, stack.client, `{}`)
		stack.awaitIndexed(t, stack.create(t, drone1, aliased("held-1")))
		raw, err := stack.askGlobal(t, fmt.Sprintf(`{"query":%q}`, absentDrone))
		requireTextSearched(t, absentDrone, raw, err, asked, notFoundHint("entity", absentDrone, absentDrone))
		if got := c.Health().ErrorCount; got != 0 {
			t.Errorf("ErrorCount = %d, want 0", got)
		}
	})
	t.Run("a holder whose entity is gone", func(t *testing.T) {
		client := fixtureClient(t)
		asked := stubTextSearch(t, client)
		c := startComponent(t, client, `{}`)
		serve(t, client, verbRow(t, graph.ResponderGraphIndex, "alias").Subject, func(context.Context, []byte) ([]byte, error) {
			return aliasReply(drone1).answer()
		})
		serve(t, client, verbRow(t, graph.ResponderGraphIngest, "entity").Subject, func(context.Context, []byte) ([]byte, error) {
			return nil, errs.ClassifiedCode(errs.ErrorInvalid, "entity_not_found", errors.New("not found: "+drone1))
		})
		raw, err := client.RequestClassified(t.Context(), globalSearchSubject, []byte(`{"query":"held-1"}`), requestTimeout)
		requireTextSearched(t, "held-1", raw, err, asked, notFoundHint("entity", "held-1", drone1))
		if got := c.Health().ErrorCount; got != 0 {
			t.Errorf("ErrorCount = %d, want 0", got)
		}
	})
}

// TestGlobalSearchUnknownPathStart: a path start whose holder's entity is gone is searched for its
// words, its path hint carrying the entity verb's refusal as it was sent, with no hop asked and no
// error counted (spec, "An unknown named start").
func TestGlobalSearchUnknownPathStart(t *testing.T) {
	client := fixtureClient(t)
	asked := stubTextSearch(t, client)
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
	const question = "what is connected to held-1"
	raw, err := client.RequestClassified(t.Context(), globalSearchSubject, fmt.Appendf(nil, `{"query":%q}`, question), requestTimeout)
	requireTextSearched(t, question, raw, err, asked, notFoundHint("path", "held-1", drone1))
	// Each stub counts before it replies, so before the reply above.
	if n := hops.Load(); n != 0 {
		t.Errorf("the index's edge verbs received %d requests, want none", n)
	}
	if got := c.Health().ErrorCount; got != 0 {
		t.Errorf("ErrorCount = %d, want 0", got)
	}
}

// TestZoneQuestionIsSearched: the keyword classifier reads "the area of the plant" as a path from
// area-of; no entity holds that alias, so the question is searched for its words, the path hint not
// honoured with entity_not_found, and no entity read is made (spec, "An unknown named start").
func TestZoneQuestionIsSearched(t *testing.T) {
	stack, _ := startSearchStack(t)
	stack.awaitSearchable(t, stack.create(t, drone1, aliased("held-1")))
	noEntityRequest := spyOnEntityVerb(t, stack.client)
	const question = "the area of the plant"
	raw, err := stack.askGlobal(t, fmt.Sprintf(`{"query":%q}`, question))
	if err != nil {
		t.Fatalf("%q: %v", question, err)
	}
	_, data := decodeGlobal(t, raw)
	requireHintList(t, question, data.Hints, []graph.GlobalSearchHint{notFoundHint("path", "area-of", "area-of")})
	noEntityRequest()
}

// expectedHint is a hint a plain question is expected to list, not honoured: its kind, and its
// detail where the test spells it ("" when the hint names nothing, "*" for a time range, whose
// clock the test does not hold). A path start names nothing in this graph, so its code is
// entity_not_found.
type expectedHint struct{ hint, detail string }

// TestEveryQuestionIsSearched feeds the router probe's 27 plain questions and the ones the keyword
// classifier cannot read whole ("a path from A to B", a second place) to a graph naming none of
// their words as entities: none is refused, each reply holds text, and each hint is listed not
// honoured with a reason, a start's with code entity_not_found and a classifier refusal's with
// invalid_request (spec, "Plain questions", "An aggregation word", "A question this build cannot
// answer"). The expected hints are the probe's routes and refusals, spelled out here. Every request
// is one in DataFlow and none an error in Health.
func TestEveryQuestionIsSearched(t *testing.T) {
	stack, c := startSearchStack(t)
	stack.awaitSearchable(t, stack.create(t, drone1, [2]string{vocabulary.DCTermsTitle, "pump station"}))
	cases := []struct {
		question string
		hints    []expectedHint
	}{
		{"what is the difference between real-time and batch ingest", []expectedHint{{"path", "real-time"}}},
		{"how are entities related to their sources", []expectedHint{{"path", ""}}},
		{"which sensors are linked to the gateway", []expectedHint{{"path", ""}}},
		{"storage area is flooded", []expectedHint{{"path", "area-is"}}},
		{"what is the area of responsibility for the ops team", []expectedHint{{"path", "area-of"}}},
		{"time zone settings for the scheduler", []expectedHint{{"path", "zone-settings"}}},
		{"what does the dashboard look like", []expectedHint{{"similarity", ""}}},
		{"I would like the pump manual", []expectedHint{{"similarity", ""}}},
		{"compare pump-7 and pump-9", []expectedHint{{"similarity", ""}}},
		{"what is the max temperature of pump-7", []expectedHint{{"aggregation", "max of temperature"}}},
		{"total cost of the outage", []expectedHint{{"aggregation", "sum of cost"}}},
		{"how many sensors are offline", []expectedHint{{"aggregation", "count"}}},
		{"count of open work orders", []expectedHint{{"aggregation", "count"}}},
		{"mean time between failures", []expectedHint{{"path", ""}, {"aggregation", "avg of time"}}},
		{"what failed pumps were reported last week", []expectedHint{{"time_range", "*"}}},
		{"what happened today", []expectedHint{{"time_range", "*"}}},
		{"pump failures today", []expectedHint{{"time_range", "*"}}},
		{"the most recent firmware update", []expectedHint{{"ranking", ""}}},
		{"top 5 alerts", []expectedHint{{"ranking", ""}}},
		{"minimum viable configuration", []expectedHint{{"aggregation", "min of viable"}}},
		{"sum-check errors on the bus", []expectedHint{{"aggregation", "sum"}}},
		{"relational database schema", []expectedHint{{"path", ""}}},
		{"pathology report for patient", []expectedHint{{"path", ""}}},
		{"connection pool exhausted", []expectedHint{{"path", ""}}},
		{"pump failure vibration", nil},
		{"status of pump-7", nil},
		{"find documents similar to the maintenance guide", []expectedHint{{"similarity", ""}}},
		// The keyword classifier reads one start of "from A to B"; the word search still reads B.
		{"a path from pump-7 to pump-9", []expectedHint{{"path", "pump-7"}}},
		{"what is connected to sensor-001 near zone-b", []expectedHint{{"classification", "*"}}},
		{"show sensor-001 in zone-b", []expectedHint{{"classification", "*"}}},
	}
	for _, tc := range cases {
		raw, err := stack.askGlobal(t, fmt.Sprintf(`{"query":%q}`, tc.question))
		if err != nil {
			t.Errorf("%q refused: %v", tc.question, err)
			continue
		}
		_, data := decodeGlobal(t, raw)
		if data.Entity != nil || data.Path != nil {
			t.Errorf("%q: entity %s, path %v; want text alone", tc.question, data.Entity, data.Path)
		}
		if len(data.Hints) != len(tc.hints) {
			t.Errorf("%q: hints = %+v, want %v", tc.question, data.Hints, tc.hints)
			continue
		}
		for i, hint := range data.Hints {
			want := tc.hints[i]
			code := ""
			switch {
			case want.hint == "classification":
				code = graph.ErrorCodeInvalidRequest
			case want.hint == "path" && want.detail != "":
				code = graph.ErrorCodeEntityNotFound
			}
			if hint.Hint != want.hint || hint.Honoured || hint.Reason == "" || hint.Code != code ||
				(want.detail != "*" && hint.Detail != want.detail) || (want.detail == "*" && hint.Detail == "") {
				t.Errorf("%q: hint %d = %+v, want %s %q not honoured, with a reason and code %q", tc.question, i, hint, want.hint, want.detail, code)
			}
		}
		if hint := firstHint(data.Hints, "aggregation"); hint != nil && hint.Reason != "global search computes no aggregation" {
			t.Errorf("%q: aggregation reason = %q, want it to say global search computes no aggregation", tc.question, hint.Reason)
		}
	}
	// Each handler counts before it replies.
	if flow := c.DataFlow(); flow.Messages != int64(len(cases)) {
		t.Errorf("DataFlow counted %d requests, want %d", flow.Messages, len(cases))
	}
	if got := c.Health().ErrorCount; got != 0 {
		t.Errorf("ErrorCount = %d, want 0: a hint not honoured is no error", got)
	}
}

// firstHint is the first hint of kind, or nil.
func firstHint(hints []graph.GlobalSearchHint, kind string) *graph.GlobalSearchHint {
	for i := range hints {
		if hints[i].Hint == kind {
			return &hints[i]
		}
	}
	return nil
}

// TestWordIndexRefusalComesFirst: a question the word index's rules refuse, `the of and` (nothing
// left to search) or the one token `pump*` (a reserved form), is refused with the word index's
// invalid_request as graph-lexical sends it, and no alias, entity or path request is made: the
// one token is not looked up as an entity (spec, "A question the word index refuses").
func TestWordIndexRefusalComesFirst(t *testing.T) {
	stack, c := startSearchStack(t)
	stack.awaitSearchable(t, stack.create(t, drone1, aliased("pump"), [2]string{vocabulary.DCTermsTitle, "pump station"}))
	aliasRequests := spyOnDownstream(t, stack.client, verbRow(t, graph.ResponderGraphIndex, "alias").Subject)
	reads := spyOnWalk(t, stack.client)
	for _, question := range []string{"the of and", "pump*"} {
		_, direct := wordSearchDirect(t, stack.client, graph.LexicalSearchRequest{Query: question})
		if codeOf(direct) != graph.ErrorCodeInvalidRequest {
			t.Fatalf("%q: graph-lexical answered %v, want its invalid_request", question, direct)
		}
		raw, err := stack.askGlobal(t, fmt.Sprintf(`{"query":%q}`, question))
		requireRefusal(t, raw, err, errs.ErrorInvalid, graph.ErrorCodeInvalidRequest)
		if err.Error() != direct.Error() {
			t.Errorf("%q: refusal %q, want the word index's %q", question, err, direct)
		}
	}
	if got := aliasRequests(); len(got) != 0 {
		t.Errorf("alias requests = %q, want none", got)
	}
	if got := reads.count(t); len(got) != 0 {
		t.Errorf("entity and edge requests = %v, want none", got)
	}
	if got := c.Health().ErrorCount; got != 0 {
		t.Errorf("ErrorCount = %d, want 0", got)
	}
}
