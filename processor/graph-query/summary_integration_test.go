//go:build integration

package graphquery

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/pkg/errs"
)

const summarySubject = "graph.query.summary"

// askSummary sends request and returns the reply, its data with next_cursor cut as cutCursor does, and
// the cursor.
func askSummary(t *testing.T, stack graphStack, request string) (wireEnvelope, []byte, string) {
	t.Helper()
	raw, err := stack.client.RequestClassified(t.Context(), summarySubject, []byte(request), requestTimeout)
	if err != nil {
		t.Fatalf("summary %s: %v", request, err)
	}
	envelope := decodeEnvelope(t, raw)
	data, cursor := cutCursor(t, envelope.Data)
	return envelope, data, cursor
}

// TestSummaryPredicates: a real index holding none, then some, and a caller who leaves them out. The
// keys of data are the literal's, so null and [] stay apart. create states core.identity.type on each
// entity, and graph-ingest stamps entity.indexing.profile on each. An answer that read the index carries
// the revision it read at; nothing else writes here, so that is the last write's (spec, "The three
// states of predicates").
func TestSummaryPredicates(t *testing.T) {
	stack := startStack(t)
	stack.awaitIndexed(t, 0) // The index refuses until its initial build is complete, even with nothing to build.

	_, data, _ := askSummary(t, stack, `{}`)
	requireSameJSON(t, data, `{"total_entities":0,"entities_scanned":0,"truncated":false,"entity_types":[],"predicates":[]}`)

	stack.create(t, drone1, [2]string{"dc.terms.title", "Alpha"})
	written := stack.create(t, drone2)
	stack.awaitIndexed(t, written)

	entityTypes := `"entity_types":[{"type":"query.system.drone","count":2,"examples":["c360.test.query.system.drone.001","c360.test.query.system.drone.002"]}]`

	envelope, data, _ := askSummary(t, stack, `{}`)
	requireSameJSON(t, data, `{"total_entities":2,"entities_scanned":2,"truncated":false,`+entityTypes+`,"predicates":[`+
		`{"predicate":"core.identity.type","entity_count":2},`+
		`{"predicate":"dc.terms.title","entity_count":1},`+
		`{"predicate":"entity.indexing.profile","entity_count":2}]}`)
	if envelope.IndexedRevision != written {
		t.Errorf("indexed_revision = %d, want the last write's %d", envelope.IndexedRevision, written)
	}

	_, data, _ = askSummary(t, stack, `{"exclude_predicates":true}`)
	requireSameJSON(t, data, `{"total_entities":2,"entities_scanned":2,"truncated":false,`+entityTypes+`,"predicates":null}`)
}

// TestAuthorityOnlyAnswerClaimsNoRevision: with predicates left out, no index is read, so a min_revision
// far above any revision is answered, and the answer claims revision 0 (spec, "A summary that read only
// the authority"; design D7). The same request with predicates is refused, so it is leaving them out that
// answered.
func TestAuthorityOnlyAnswerClaimsNoRevision(t *testing.T) {
	stack := startStack(t)
	stack.awaitIndexed(t, stack.create(t, drone1))

	farAhead := uint64(1) << 63
	envelope, data, _ := askSummary(t, stack, fmt.Sprintf(`{"exclude_predicates":true,"min_revision":%d}`, farAhead))
	requireSameJSON(t, data, `{"total_entities":1,"entities_scanned":1,"truncated":false,"entity_types":[`+
		`{"type":"query.system.drone","count":1,"examples":["c360.test.query.system.drone.001"]}],"predicates":null}`)
	if envelope.IndexedRevision != 0 {
		t.Errorf("indexed_revision = %d, want 0: this answer read no index", envelope.IndexedRevision)
	}
	raw, err := stack.client.RequestClassified(t.Context(), summarySubject, fmt.Appendf(nil, `{"min_revision":%d}`, farAhead), requestTimeout)
	requireRefusal(t, raw, err, errs.ErrorTransient, "revision_not_reached")
}

// TestSummaryTypeBuckets: carried from the pin's HappyPath, TieBreak and CanonicalOrder cases in
// summary_test.go, through real graph-ingest. Type is system.domain.type (ADR-102), highest count first
// with ties by type, and examples are capped per type.
func TestSummaryTypeBuckets(t *testing.T) {
	stack := startStack(t)
	for _, id := range []string{
		"c360.test.zeta.system.type.e1",
		"c360.test.web.agent.observation.h3",
		"c360.test.web.agent.observation.h1",
		"c360.test.agentic-loop.agent.execution.l2",
		"c360.test.web.agent.observation.h2",
		"c360.test.agentic-loop.agent.execution.l1",
		"c360.test.alpha.system.type.e1",
	} {
		stack.create(t, id)
	}

	for _, tc := range []struct{ name, request, want string }{
		{"the default, two examples", `{"exclude_predicates":true}`, `{"total_entities":7,"entities_scanned":7,"truncated":false,"entity_types":[` +
			`{"type":"web.agent.observation","count":3,"examples":["c360.test.web.agent.observation.h1","c360.test.web.agent.observation.h2"]},` +
			`{"type":"agentic-loop.agent.execution","count":2,"examples":["c360.test.agentic-loop.agent.execution.l1","c360.test.agentic-loop.agent.execution.l2"]},` +
			`{"type":"alpha.system.type","count":1,"examples":["c360.test.alpha.system.type.e1"]},` +
			`{"type":"zeta.system.type","count":1,"examples":["c360.test.zeta.system.type.e1"]}],"predicates":null}`},
		{"one example", `{"exclude_predicates":true,"examples_per_type":1}`, `{"total_entities":7,"entities_scanned":7,"truncated":false,"entity_types":[` +
			`{"type":"web.agent.observation","count":3,"examples":["c360.test.web.agent.observation.h1"]},` +
			`{"type":"agentic-loop.agent.execution","count":2,"examples":["c360.test.agentic-loop.agent.execution.l1"]},` +
			`{"type":"alpha.system.type","count":1,"examples":["c360.test.alpha.system.type.e1"]},` +
			`{"type":"zeta.system.type","count":1,"examples":["c360.test.zeta.system.type.e1"]}],"predicates":null}`},
		{"three examples", `{"prefix":"c360.test.web","exclude_predicates":true,"examples_per_type":3}`, `{"total_entities":3,"entities_scanned":3,"truncated":false,"entity_types":[` +
			`{"type":"web.agent.observation","count":3,"examples":["c360.test.web.agent.observation.h1","c360.test.web.agent.observation.h2","c360.test.web.agent.observation.h3"]}],"predicates":null}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, data, _ := askSummary(t, stack, tc.request)
			requireSameJSON(t, data, tc.want)
		})
	}
}

// TestSummaryOrdersTypes: all counts tied, so type alone decides, written in reverse. Twenty, as with
// hierarchyStats' children: with few ties a missing tie-break passes some runs (notes/4.3.md). Go promises
// no map order, so this detection is a measure, not a proof.
func TestSummaryOrdersTypes(t *testing.T) {
	stack := startStack(t)
	systems := strings.Fields("aa ab ac ad ae af ag ah ai aj ak al am an ao ap aq ar as at")
	for _, system := range slices.Backward(systems) {
		stack.create(t, "c360.test."+system+".system.drone.001")
	}
	rows := make([]string, 0, len(systems))
	for _, system := range systems {
		rows = append(rows, `{"type":"`+system+`.system.drone","count":1,"examples":["c360.test.`+system+`.system.drone.001"]}`)
	}
	_, data, _ := askSummary(t, stack, `{"exclude_predicates":true}`)
	requireSameJSON(t, data, `{"total_entities":20,"entities_scanned":20,"truncated":false,"entity_types":[`+strings.Join(rows, ",")+`],"predicates":null}`)
}

// TestSummaryPrefixReadsOneGroup: two groups, and the answer holds the one asked for and its true total.
// A prefix holding nothing is an answer of zero (spec, "One group").
func TestSummaryPrefixReadsOneGroup(t *testing.T) {
	stack := startStack(t)
	stack.create(t, drone1)
	stack.create(t, drone2)
	stack.create(t, "c360.test.other.system.sensor.001")

	for _, tc := range []struct{ name, prefix, want string }{
		{"the first group", "c360.test.query", `{"total_entities":2,"entities_scanned":2,"truncated":false,"entity_types":[` +
			`{"type":"query.system.drone","count":2,"examples":["c360.test.query.system.drone.001","c360.test.query.system.drone.002"]}],"predicates":null}`},
		{"the second group", "c360.test.other", `{"total_entities":1,"entities_scanned":1,"truncated":false,"entity_types":[` +
			`{"type":"other.system.sensor","count":1,"examples":["c360.test.other.system.sensor.001"]}],"predicates":null}`},
		{"a group holding nothing", "c360.test.empty", `{"total_entities":0,"entities_scanned":0,"truncated":false,"entity_types":[],"predicates":null}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			envelope, data, _ := askSummary(t, stack, `{"prefix":"`+tc.prefix+`","exclude_predicates":true}`)
			requireSameJSON(t, data, tc.want)
			if envelope.IndexedRevision != 0 {
				t.Errorf("indexed_revision = %d, want 0", envelope.IndexedRevision)
			}
		})
	}
}

// TestSummaryFollowsACursor: real graph-ingest, a group larger than one page. The first answer declares
// a sample of the lexically smallest IDs beside the true total; the cursor reads the rest; the rows sum to
// the number seeded (spec, "A group larger than a page").
func TestSummaryFollowsACursor(t *testing.T) {
	stack := startStack(t)
	seedPagedGroup(t, stack)

	first := `{"prefix":"` + countedGroup + `","exclude_predicates":true`
	_, data, cursor := askSummary(t, stack, first+`}`)
	requireSameJSON(t, data, `{"total_entities":5,"entities_scanned":4,"truncated":true,"next_cursor":true,"entity_types":[`+
		`{"type":"query.system.beta","count":2,"examples":["c360.test.query.system.beta.001","c360.test.query.system.beta.002"]},`+
		`{"type":"query.system.alpha","count":1,"examples":["c360.test.query.system.alpha.001"]},`+
		`{"type":"query.system.gamma","count":1,"examples":["c360.test.query.system.gamma.001"]}],"predicates":null}`)
	if cursor == "" {
		t.Fatal("a partial page names no cursor")
	}

	_, data, cursor = askSummary(t, stack, first+`,"cursor":"`+cursor+`"}`)
	requireSameJSON(t, data, `{"total_entities":5,"entities_scanned":1,"truncated":false,"entity_types":[`+
		`{"type":"query.system.zeta","count":1,"examples":["c360.test.query.system.zeta.001"]}],"predicates":null}`)
	if cursor != "" {
		t.Errorf("the last page left next_cursor %q", cursor)
	}

	// The cursor is opaque here, so only real graph-ingest can refuse one, and its refusal arrives.
	raw, err := stack.client.RequestClassified(t.Context(), summarySubject, []byte(first+`,"cursor":"!not-a-cursor!"}`), requestTimeout)
	if refusal := requireRefusal(t, raw, err, errs.ErrorInvalid, "invalid_request"); !strings.Contains(refusal.Error(), "invalid cursor") {
		t.Errorf("refusal = %v, want graph-ingest's naming the cursor", refusal)
	}
}

// TestSummaryRequestsOneOfEach: beside the real responders, a summary is one predicateList request with
// min_revision and nothing else, and one prefix request at the maximum limit with the cursor unchanged.
// Predicates left out, predicateList gets none (spec, "Counting the requests").
func TestSummaryRequestsOneOfEach(t *testing.T) {
	stack := startStack(t)
	stack.create(t, drone1)
	stack.create(t, drone2)
	written := stack.create(t, drone3)
	stack.awaitIndexed(t, written)

	raw, err := stack.client.RequestClassified(t.Context(), verbRow(t, graph.ResponderGraphIngest, "prefix").Subject, []byte(`{"prefix":"c360","limit":1}`), requestTimeout)
	if err != nil {
		t.Fatal(err)
	}
	_, cursor := cutCursor(t, raw)

	prefixRequests := spyOnPrefixVerb(t, stack.client)
	predicateRequests := spyOnDownstream(t, stack.client, verbRow(t, graph.ResponderGraphIndex, "predicateList").Subject)
	for _, tc := range []struct {
		name, request, wantPage string
		wantPredicates          []string
	}{
		{"with predicates", `{}`, `{"prefix":"","limit":1000}`, []string{`{}`}},
		{"with min_revision", fmt.Sprintf(`{"min_revision":%d}`, written), `{"prefix":"","limit":1000}`, []string{fmt.Sprintf(`{"min_revision":%d}`, written)}},
		{"left out", fmt.Sprintf(`{"prefix":"c360","exclude_predicates":true,"cursor":%q,"min_revision":7}`, cursor),
			fmt.Sprintf(`{"prefix":"c360","limit":1000,"cursor":%q}`, cursor), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := stack.client.RequestClassified(t.Context(), summarySubject, []byte(tc.request), requestTimeout); err != nil {
				t.Fatal(err)
			}
			pages := prefixRequests()
			if len(pages) != 1 {
				t.Fatalf("prefix requests = %q, want one", pages)
			}
			requireSameJSON(t, []byte(pages[0]), tc.wantPage)
			predicates := predicateRequests()
			if len(predicates) != len(tc.wantPredicates) {
				t.Fatalf("predicateList requests = %q, want %q", predicates, tc.wantPredicates)
			}
			for i := range predicates {
				requireSameJSON(t, []byte(predicates[i]), tc.wantPredicates[i])
			}
		})
	}
}

// TestSummaryRefusesWhenPredicatesCannot: the pin's PredicateFailure_StillReturnsEntityFacet reversed. A
// real index's refusal reaches the caller with class, code and detail, and an index that is gone is
// graph-query's own refusal naming the verb. Neither costs a page, so predicateList ran first (spec, "An
// index that cannot answer").
func TestSummaryRefusesWhenPredicatesCannot(t *testing.T) {
	t.Run("the index refuses", func(t *testing.T) {
		stack := startStack(t)
		written := stack.create(t, drone1)
		stack.awaitIndexed(t, written)
		prefixRequests := spyOnPrefixVerb(t, stack.client)

		raw, err := stack.client.RequestClassified(t.Context(), summarySubject, fmt.Appendf(nil, `{"min_revision":%d}`, uint64(1)<<40), requestTimeout)
		if reached := indexedRevisionOf(t, raw, err); reached != written {
			t.Errorf("refusal names indexed_revision %d, want the write's %d", reached, written)
		}
		if got := prefixRequests(); len(got) != 0 {
			t.Errorf("prefix requests = %q, want none", got)
		}
	})
	t.Run("the index is gone", func(t *testing.T) {
		client := fixtureClient(t)
		c := startComponent(t, client, `{}`)
		serve(t, client, verbRow(t, graph.ResponderGraphIngest, "prefix").Subject, func(context.Context, []byte) ([]byte, error) {
			return []byte(`{"entities":[{"id":"` + drone1 + `"}],"total":1}`), nil
		})
		prefixRequests := spyOnPrefixVerb(t, client)

		raw, err := client.RequestClassified(t.Context(), summarySubject, []byte(`{}`), requestTimeout)
		refusal := requireRefusal(t, raw, err, errs.ErrorTransient, "internal")
		if subject := verbRow(t, graph.ResponderGraphIndex, "predicateList").Subject; !strings.Contains(refusal.Error(), subject) {
			t.Errorf("refusal = %v, want it to name %s", refusal, subject)
		}
		if got := prefixRequests(); len(got) != 0 {
			t.Errorf("prefix requests = %q, want none", got)
		}
		if health := c.Health(); health.ErrorCount != 1 {
			t.Errorf("ErrorCount = %d, want 1", health.ErrorCount)
		}
	})
}

// TestSummaryRefusesABrokenPredicateReply: a stub sends what no real graph-index does (#111 item 2). Each
// is one transient internal refusal and one error, not an empty list, and reads no page. A good reply on
// the same stub answers, so the refusals are the replies' doing.
func TestSummaryRefusesABrokenPredicateReply(t *testing.T) {
	client := fixtureClient(t)
	c := startComponent(t, client, `{}`)
	var reply atomic.Pointer[string]
	serve(t, client, verbRow(t, graph.ResponderGraphIndex, "predicateList").Subject, func(context.Context, []byte) ([]byte, error) {
		return []byte(*reply.Load()), nil
	})
	serve(t, client, verbRow(t, graph.ResponderGraphIngest, "prefix").Subject, func(context.Context, []byte) ([]byte, error) {
		return []byte(`{"entities":null,"total":0}`), nil
	})
	prefixRequests := spyOnPrefixVerb(t, client)

	errorsSoFar := 0
	for _, tc := range []struct{ name, reply, says string }{
		{"not JSON", `not json`, "invalid character"},
		{"an empty object", `{}`, "names no producer"},
		{"no producer", `{"data":{"predicates":[]}}`, "names no producer"},
		{"no predicates key", `{"data":{},"producer":"graph-index"}`, "holds no predicates"},
		{"predicates null", `{"data":{"predicates":null},"producer":"graph-index"}`, "holds no predicates"},
		{"predicates not a list", `{"data":{"predicates":"none"},"producer":"graph-index"}`, "cannot unmarshal"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reply.Store(&tc.reply)
			got, err := client.RequestClassified(t.Context(), summarySubject, []byte(`{}`), requestTimeout)
			refusal := requireRefusal(t, got, err, errs.ErrorTransient, "internal")
			if !strings.Contains(refusal.Error(), tc.says) {
				t.Errorf("refusal = %v, want it to say %q", refusal, tc.says)
			}
			errorsSoFar++
			if health := c.Health(); health.ErrorCount != errorsSoFar {
				t.Errorf("ErrorCount = %d, want %d", health.ErrorCount, errorsSoFar)
			}
			if pages := prefixRequests(); len(pages) != 0 {
				t.Errorf("prefix requests = %q, want none", pages)
			}
		})
	}

	good := `{"data":{"predicates":[]},"indexed_revision":9,"producer":"graph-index"}`
	reply.Store(&good)
	got, err := client.RequestClassified(t.Context(), summarySubject, []byte(`{}`), requestTimeout)
	if err != nil {
		t.Fatalf("a good reply: %v", err)
	}
	envelope := decodeEnvelope(t, got)
	requireSameJSON(t, envelope.Data, `{"total_entities":0,"entities_scanned":0,"truncated":false,"entity_types":[],"predicates":[]}`)
	if envelope.IndexedRevision != 9 {
		t.Errorf("indexed_revision = %d, want the predicate reply's 9", envelope.IndexedRevision)
	}
}

// TestSummaryRefusesABrokenPage: the page read is hierarchyStats' (TestCountsRefuseABrokenPage), and a
// summary refuses what it refuses. The pin refused a bad ID too; the class and code are new.
func TestSummaryRefusesABrokenPage(t *testing.T) {
	client := fixtureClient(t)
	c := startComponent(t, client, `{}`)
	var reply atomic.Pointer[string]
	serve(t, client, verbRow(t, graph.ResponderGraphIngest, "prefix").Subject, func(context.Context, []byte) ([]byte, error) {
		return []byte(*reply.Load()), nil
	})

	errorsSoFar := 0
	for _, tc := range brokenPages {
		t.Run(tc.name, func(t *testing.T) {
			reply.Store(&tc.reply)
			got, err := client.RequestClassified(t.Context(), summarySubject, []byte(`{"prefix":"c360.test","exclude_predicates":true}`), requestTimeout)
			refusal := requireRefusal(t, got, err, errs.ErrorTransient, "internal")
			if !strings.Contains(refusal.Error(), tc.says) {
				t.Errorf("refusal = %v, want it to say %q", refusal, tc.says)
			}
			errorsSoFar++
			if health := c.Health(); health.ErrorCount != errorsSoFar {
				t.Errorf("ErrorCount = %d, want %d", health.ErrorCount, errorsSoFar)
			}
		})
	}
}

// TestSummaryRefusesPrefixWithPredicates: predicateList takes no entity prefix, so the two together
// would set one group's types beside the whole graph's predicates. Refused naming both, with no request
// to either real responder (spec, "A prefix with predicates").
func TestSummaryRefusesPrefixWithPredicates(t *testing.T) {
	stack := startStack(t)
	stack.awaitIndexed(t, stack.create(t, drone1))
	prefixRequests := spyOnPrefixVerb(t, stack.client)
	predicateRequests := spyOnDownstream(t, stack.client, verbRow(t, graph.ResponderGraphIndex, "predicateList").Subject)

	for _, request := range []string{
		`{"prefix":"c360.test"}`,
		`{"prefix":"c360.test","exclude_predicates":false}`,
	} {
		t.Run(request, func(t *testing.T) {
			got, err := stack.client.RequestClassified(t.Context(), summarySubject, []byte(request), requestTimeout)
			refusal := requireRefusal(t, got, err, errs.ErrorInvalid, "invalid_request")
			for _, name := range []string{"prefix", "exclude_predicates"} {
				if !strings.Contains(refusal.Error(), name) {
					t.Errorf("refusal = %v, want it to name %s", refusal, name)
				}
			}
			if pages, predicates := prefixRequests(), predicateRequests(); len(pages) != 0 || len(predicates) != 0 {
				t.Errorf("prefix requests = %q, predicateList requests = %q, want none", pages, predicates)
			}
		})
	}
}

// TestSummaryRefusesABadRequest: with nothing behind it, a request the verb refuses is its own refusal
// and not the no-responders one, so no read went out. The pin's keys are refused by name, and a negative
// count, which the pin read as the default, is refused too (spec, "A summary has its predicates or says
// why not").
func TestSummaryRefusesABadRequest(t *testing.T) {
	client := fixtureClient(t)
	c := startComponent(t, client, `{}`)
	for _, tc := range []struct{ name, request, code, says string }{
		{"not JSON", `not json`, "invalid_request", "invalid request"},
		{"no body", ``, "invalid_request", "invalid request"},
		{"bytes after the request", `{} {}`, "invalid_request", "bytes after"},
		{"the pin's include_predicates", `{"include_predicates":false}`, "invalid_request", "include_predicates"},
		{"the pin's entity_sample_limit", `{"entity_sample_limit":10}`, "invalid_request", "entity_sample_limit"},
		{"a negative examples_per_type", `{"examples_per_type":-1}`, "invalid_request", "examples_per_type"},
		{"a prefix with an empty segment", `{"prefix":"c360..test","exclude_predicates":true}`, "entity_id_prefix_invalid", ""},
		{"a prefix of seven parts", `{"prefix":"a.b.c.d.e.f.g","exclude_predicates":true}`, "entity_id_prefix_invalid", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := client.RequestClassified(t.Context(), summarySubject, []byte(tc.request), requestTimeout)
			refusal := requireRefusal(t, got, err, errs.ErrorInvalid, tc.code)
			if !strings.Contains(refusal.Error(), tc.says) {
				t.Errorf("refusal = %v, want it to say %q", refusal, tc.says)
			}
		})
	}
	if health := c.Health(); health.ErrorCount != 0 {
		t.Errorf("ErrorCount = %d, want 0: a caller's bad request is not graph-query's error", health.ErrorCount)
	}
}
