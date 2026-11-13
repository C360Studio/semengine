//go:build integration

package graphindex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/inference"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/internal/graphmutation"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/internal/harness/payloadfixture"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/lifecycle"
	graphingest "github.com/c360studio/semengine/processor/graph-ingest"
	"github.com/c360studio/semengine/types"
	"github.com/c360studio/semengine/vocabulary"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// requestTimeout bounds one query request.
const requestTimeout = 10 * time.Second

// subjectOf is the subject the verb table in graph declares for graph-index's verb name.
func subjectOf(t *testing.T, name string) string {
	t.Helper()
	for _, verb := range graph.QueryVerbs() {
		if verb.Responder == graph.ResponderGraphIndex && verb.Name == name {
			return verb.Subject
		}
	}
	t.Fatalf("the verb table declares no graph-index verb %q", name)
	return ""
}

// query sends body on the verb's subject through the classified request path a caller uses.
func query(t *testing.T, client *natsclient.Client, verb string, body any) ([]byte, error) {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return client.RequestClassified(t.Context(), subjectOf(t, verb), data, requestTimeout)
}

// envelope is a decoded successful reply.
type envelope[T any] struct {
	Data            T         `json:"data"`
	IndexedRevision uint64    `json:"indexed_revision"`
	Producer        string    `json:"producer"`
	Timestamp       time.Time `json:"timestamp"`
}

// answer sends body on verb, requires a successful reply from graph-index carrying at least
// revision minRevision, and returns its data.
func answer[T any](t *testing.T, client *natsclient.Client, verb string, body any, minRevision uint64) T {
	t.Helper()
	raw, err := query(t, client, verb, body)
	if err != nil {
		t.Fatalf("%s %v: %v", verb, body, err)
	}
	var reply envelope[T]
	if err := json.Unmarshal(raw, &reply); err != nil {
		t.Fatalf("%s reply %s: %v", verb, raw, err)
	}
	if reply.Producer != "graph-index" {
		t.Errorf("%s reply producer = %q, want graph-index", verb, reply.Producer)
	}
	if reply.IndexedRevision < minRevision {
		t.Errorf("%s reply indexed_revision = %d, want at least %d", verb, reply.IndexedRevision, minRevision)
	}
	if reply.Timestamp.IsZero() {
		t.Errorf("%s reply has no timestamp", verb)
	}
	return reply.Data
}

// requireNotReady requires err to be the transient index_not_ready refusal.
func requireNotReady(t *testing.T, verb string, err error) {
	t.Helper()
	var classified *errs.ClassifiedError
	if !errors.As(err, &classified) {
		t.Errorf("%s: err = %v, want a classified refusal", verb, err)
		return
	}
	if classified.Class != errs.ErrorTransient || classified.Code != graph.ErrorCodeIndexNotReady {
		t.Errorf("%s: class %v code %q (%v), want transient %q", verb, classified.Class, classified.Code, err,
			graph.ErrorCodeIndexNotReady)
	}
}

// everyVerb is one request on each of graph-index's eight verbs.
func everyVerb(subject, target string) map[string]any {
	return map[string]any{
		"outgoing":          map[string]string{"entity_id": subject},
		"incoming":          map[string]string{"entity_id": target},
		"alias":             map[string]string{"alias": "alpha-1"},
		"predicate":         map[string]string{"predicate": linkTo},
		"predicateList":     map[string]string{},
		"predicateStats":    map[string]string{"predicate": linkTo},
		"predicateCompound": map[string]any{"predicates": []string{linkTo, armed}, "operator": "AND"},
		"byName":            map[string]string{"name": "Alpha"},
	}
}

// withMinRevision is body with min_revision set, spelled here as it is on the wire so the test
// does not take the field's name from the code under test.
func withMinRevision(t *testing.T, body, minRevision any) map[string]any {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	fields := map[string]any{}
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	fields["min_revision"] = minRevision
	return fields
}

// answeredAt sends body on verb, requires a successful reply carrying exactly revision, and
// returns its data as the wire spelled it.
func answeredAt(t *testing.T, client *natsclient.Client, verb string, body any, revision uint64) string {
	t.Helper()
	raw, err := query(t, client, verb, body)
	if err != nil {
		t.Errorf("%s %v: %v, want an answer", verb, body, err)
		return ""
	}
	var reply envelope[json.RawMessage]
	if err := json.Unmarshal(raw, &reply); err != nil {
		t.Fatalf("%s reply %s: %v", verb, raw, err)
	}
	if reply.IndexedRevision != revision {
		t.Errorf("%s %v: indexed_revision = %d, want %d", verb, body, reply.IndexedRevision, revision)
	}
	return string(reply.Data)
}

// requireRevisionNotReached requires err to be the transient revision_not_reached refusal whose
// detail names reached and nothing else. The code and the detail key are the wire's literals. A
// number in a decoded detail is a JSON number, so float64.
func requireRevisionNotReached(t *testing.T, verb string, err error, reached uint64) {
	t.Helper()
	var classified *errs.ClassifiedError
	if !errors.As(err, &classified) {
		t.Errorf("%s: err = %v, want a classified refusal", verb, err)
		return
	}
	if classified.Class != errs.ErrorTransient || classified.Code != "revision_not_reached" {
		t.Errorf("%s: class %v code %q (%v), want transient revision_not_reached", verb, classified.Class,
			classified.Code, err)
	}
	want := map[string]any{"indexed_revision": float64(reached)}
	if !maps.Equal(classified.Detail, want) {
		t.Errorf("%s: detail = %v, want %v", verb, classified.Detail, want)
	}
}

// TestMinRevisionBoundary: a verb compares min_revision with the revision its reply would carry,
// after validation and readiness and before its first bucket read (spec "An unmet min_revision is
// refused"). The expected revisions are the ones the fixture's own writes returned.
func TestMinRevisionBoundary(t *testing.T) {
	subject, target := entityID(1), entityID(2)
	requests := everyVerb(subject, target)
	declared := 0
	for _, verb := range graph.QueryVerbs() {
		if verb.Responder != graph.ResponderGraphIndex {
			continue
		}
		declared++
		if _, ok := requests[verb.Name]; !ok {
			t.Fatalf("no request for declared verb %s", verb.Name)
		}
	}
	if declared != 8 || len(requests) != 8 {
		t.Fatalf("declared verbs %d, requests %d, want 8 of each", declared, len(requests))
	}
	// put writes the entity everyVerb's requests read, after another write, so the revision is
	// neither 0 nor 1, and waits until the follower has indexed it.
	put := func(t *testing.T, h *harness) uint64 {
		t.Helper()
		h.put(t, target, entityValue(t, target, stmt{armed, false}))
		rev := h.put(t, subject, entityValue(t, subject,
			stmt{linkTo, target}, stmt{armed, true}, stmt{vocabulary.DCTermsTitle, "Alpha"}, stmt{alias, "alpha-1"}))
		h.awaitIndexed(t, rev)
		return rev
	}

	t.Run("boundary", func(t *testing.T) {
		h := startIndex(t, fixtureClient(t), `{}`)
		rev := put(t, h)
		for verb, body := range requests {
			absent := answeredAt(t, h.client, verb, body, rev)
			if absent == "" || absent == "null" {
				t.Errorf("%s without min_revision carries no data: %q", verb, absent)
			}
			// Zero and JSON null are no minimum, as an absent field is; R is met.
			for _, met := range []any{0, nil, rev} {
				if got := answeredAt(t, h.client, verb, withMinRevision(t, body, met), rev); got != absent {
					t.Errorf("%s min_revision %v: data %s, want the answer without it, %s", verb, met, got, absent)
				}
			}
			for _, above := range []uint64{rev + 1, math.MaxUint64} {
				_, err := query(t, h.client, verb, withMinRevision(t, body, above))
				requireRevisionNotReached(t, verb, err, rev)
			}
		}
	})

	// A follower at revision 0: zero is still no minimum, 1 is above it, and the detail still
	// names the revision. The latch is awaited as a remote reader sees it, the published envelope,
	// because the empty index is otherwise not ready yet.
	t.Run("empty-index", func(t *testing.T) {
		h := startIndex(t, fixtureClient(t), `{}`)
		await(t, "an envelope reporting bootstrap complete",
			func(ctx context.Context) (graph.IndexStatusResponse, error) { return publishedEnvelope(ctx, h.client) },
			func(s graph.IndexStatusResponse) bool { return s.BootstrapComplete })
		for verb, body := range requests {
			answeredAt(t, h.client, verb, withMinRevision(t, body, 0), 0)
			_, err := query(t, h.client, verb, withMinRevision(t, body, 1))
			requireRevisionNotReached(t, verb, err, 0)
		}
	})

	// With every index bucket gone, a read cannot succeed: the same request with a met minimum
	// shows it, failing as internal. So a refusal that still names the revision came before any
	// read whose failure reaches the reply; a read that ignored its error would not show here.
	t.Run("refusal-before-reads", func(t *testing.T) {
		h := startIndex(t, fixtureClient(t), `{}`)
		rev := put(t, h)
		js, err := h.client.JetStream()
		if err != nil {
			t.Fatal(err)
		}
		for _, bucket := range []string{graph.BucketOutgoingIndex, graph.BucketIncomingIndex, graph.BucketAliasIndex,
			graph.BucketPredicateIndex, graph.BucketNameIndex} {
			if err := js.DeleteKeyValue(t.Context(), bucket); err != nil {
				t.Fatalf("delete %s: %v", bucket, err)
			}
		}
		for verb, body := range requests {
			_, err := query(t, h.client, verb, withMinRevision(t, body, rev+1))
			requireRevisionNotReached(t, verb, err, rev)

			_, err = query(t, h.client, verb, withMinRevision(t, body, rev))
			var classified *errs.ClassifiedError
			if !errors.As(err, &classified) || classified.Code != graph.ErrorCodeInternal {
				t.Errorf("%s with its bucket gone and min_revision met: err = %v, want code %s", verb, err,
					graph.ErrorCodeInternal)
			}
		}
	})

	// A follower with an unresolved write withholds readiness: being behind is not reported in
	// its place (spec "index_not_ready keeps its meaning"; ADR-109 Decision 4).
	t.Run("not-ready-first", func(t *testing.T) {
		h := startIndex(t, fixtureClient(t), `{}`)
		js, err := h.client.JetStream()
		if err != nil {
			t.Fatal(err)
		}
		if err := js.DeleteKeyValue(t.Context(), graph.BucketIncomingIndex); err != nil {
			t.Fatalf("delete %s: %v", graph.BucketIncomingIndex, err)
		}
		rev := put(t, h) // the lane marks the entity failed before it completes the revision
		if n := h.c.failedTotal(); n == 0 {
			t.Fatal("no failed entity: the fixture no longer withholds readiness")
		}
		for verb, body := range requests {
			_, err := query(t, h.client, verb, withMinRevision(t, body, rev+1))
			requireNotReady(t, verb, err)
		}
	})
}

// listedSubjects is the test adapter's list of graph-index's query request subscriptions: the
// subjects of the "query handlers registered" line it logs once it has subscribed to them.
func listedSubjects(t *testing.T, logs *logBuffer) []string {
	t.Helper()
	match := regexp.MustCompile(`msg="query handlers registered".* subjects="\[([^\]]*)\]"`).FindStringSubmatch(logs.String())
	if match == nil {
		t.Fatal(`graph-index logged no "query handlers registered" line`)
	}
	return strings.Fields(match[1])
}

// TestGraphIndexServesExactlyTheDeclaredVerbs: the subjects graph-index lists as its query
// subscriptions equal the verb table's subjects for graph-index, one subscription each, and each
// has a responder from another connection when Start returns (graph-transport-boundary, "The
// responder serves exactly its declared verbs").
func TestGraphIndexServesExactlyTheDeclaredVerbs(t *testing.T) {
	url := shared.Reset(t)
	h := startIndex(t, natsfixture.Open(t, url, openClient), `{}`)

	var declared []string
	for _, verb := range graph.QueryVerbs() {
		if verb.Responder == graph.ResponderGraphIndex {
			declared = append(declared, verb.Subject)
		}
	}
	if len(declared) != 8 {
		t.Fatalf("the verb table declares %d verbs for graph-index, want 8: %v", len(declared), declared)
	}
	listed := listedSubjects(t, h.logs)
	slices.Sort(declared)
	slices.Sort(listed)
	if !slices.Equal(listed, declared) {
		t.Fatalf("graph-index lists query subscriptions %v, the table declares %v", listed, declared)
	}
	if n := len(h.c.subscriptions); n != len(declared) {
		t.Errorf("graph-index holds %d request subscriptions, want %d", n, len(declared))
	}

	other := natsfixture.Open(t, url, openClient) // a connection graph-index does not use
	for _, subject := range listed {
		if _, err := other.GetConnection().Request(subject, []byte(`{}`), requestTimeout); errors.Is(err, nats.ErrNoResponders) {
			t.Errorf("listed subject %s has no responder", subject)
		}
	}
}

// TestQueryDuringStartupReplayIsIndexNotReady: entities stored before Start make a replay that
// one lane applies far more slowly than eight requests are sent. Each of the eight verbs answers
// class transient, code index_not_ready, during the replay; the follower's floor, read after the
// last reply, is still below the last stored revision, which shows every reply came during the
// replay. Once the floor reaches it, the same eight requests succeed (spec "Startup-replay queries
// get a retryable refusal").
func TestQueryDuringStartupReplayIsIndexNotReady(t *testing.T) {
	client := fixtureClient(t)
	states := ensureEntityStates(t, client)
	const stored = 500
	subject, target := entityID(1), entityID(900)
	put := func(id string, value []byte) uint64 {
		rev, err := states.Put(t.Context(), id, value)
		if err != nil {
			t.Fatalf("put %s: %v", id, err)
		}
		return rev
	}
	put(subject, entityValue(t, subject,
		stmt{linkTo, target}, stmt{armed, true}, stmt{vocabulary.DCTermsTitle, "Alpha"}, stmt{alias, "alpha-1"}))
	var last uint64
	for n := 2; n <= stored; n++ {
		id := entityID(n)
		last = put(id, entityValue(t, id, stmt{linkTo, target}, stmt{armed, true}, stmt{vocabulary.DCTermsTitle, "Alpha"}))
	}

	h := startIndex(t, client, `{"workers": 1}`)
	requests := everyVerb(subject, target)
	for verb, body := range requests {
		_, err := query(t, client, verb, body)
		requireNotReady(t, verb, err)
	}
	if indexed := h.f.Indexed(); indexed >= last {
		t.Fatalf("indexed revision %d reached the last stored revision %d before the last reply: "+
			"the fixture no longer models a startup replay", indexed, last)
	}

	h.awaitIndexed(t, last)
	for verb, body := range requests {
		answer[json.RawMessage](t, client, verb, body, last)
	}
}

// TestQueryResponseCarriesIndexedRevisionAndProducer: after a write at R has been indexed, every
// verb's reply is the graph.QueryResponse envelope with producer graph-index, a timestamp, and an
// indexed_revision at least R and at most the floor read after the reply: the follower's
// watermark, not a revision of its own.
func TestQueryResponseCarriesIndexedRevisionAndProducer(t *testing.T) {
	h := startIndex(t, fixtureClient(t), `{}`)
	subject, target := entityID(1), entityID(2)
	rev := h.put(t, subject, entityValue(t, subject,
		stmt{linkTo, target}, stmt{armed, true}, stmt{vocabulary.DCTermsTitle, "Alpha"}, stmt{alias, "alpha-1"}))
	h.awaitIndexed(t, rev)
	for verb, body := range everyVerb(subject, target) {
		raw, err := query(t, h.client, verb, body)
		if err != nil {
			t.Fatalf("%s: %v", verb, err)
		}
		floor := h.f.Indexed()
		var reply envelope[json.RawMessage]
		if err := json.Unmarshal(raw, &reply); err != nil {
			t.Fatalf("%s reply %s: %v", verb, raw, err)
		}
		if reply.Producer != "graph-index" || reply.Timestamp.IsZero() {
			t.Errorf("%s reply producer %q timestamp %v, want graph-index and a time", verb, reply.Producer, reply.Timestamp)
		}
		if reply.IndexedRevision < rev || reply.IndexedRevision > floor {
			t.Errorf("%s reply indexed_revision = %d, want between the write %d and the floor %d", verb,
				reply.IndexedRevision, rev, floor)
		}
		if len(reply.Data) == 0 || string(reply.Data) == "null" {
			t.Errorf("%s reply carries no data: %s", verb, raw)
		}
	}
}

// replyMatchesEntry decodes raw as a caller holding verb's table entry does: the envelope when the
// entry is Enveloped, then the entry's Reply, each refusing an unknown field. Re-encoding the decoded
// body must give the same JSON, so a Reply with fields the wire does not carry fails too.
func replyMatchesEntry(verb graph.QueryVerb, raw []byte) error {
	decodeStrict := func(data []byte, into any) error {
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		return decoder.Decode(into)
	}
	body := raw
	if verb.Enveloped {
		var reply graph.QueryResponse[json.RawMessage]
		if err := decodeStrict(raw, &reply); err != nil {
			return fmt.Errorf("as an envelope: %w", err)
		}
		body = reply.Data
	}
	decoded := reflect.New(verb.Reply)
	if err := decodeStrict(body, decoded.Interface()); err != nil {
		return fmt.Errorf("as %s: %w", verb.Reply, err)
	}
	again, err := json.Marshal(decoded.Interface())
	if err != nil {
		return err
	}
	var sent, resent any
	if err := errors.Join(json.Unmarshal(body, &sent), json.Unmarshal(again, &resent)); err != nil {
		return err
	}
	if !reflect.DeepEqual(sent, resent) {
		return fmt.Errorf("as %s re-encodes to %s, the reply body is %s", verb.Reply, again, body)
	}
	return nil
}

// TestRepliesMatchTheirTableEntries: every row the verb table declares for graph-index, not a list
// kept here, gets a real reply carrying data, and that reply has the row's shape and type
// (graph-transport-boundary).
func TestRepliesMatchTheirTableEntries(t *testing.T) {
	h := startIndex(t, fixtureClient(t), `{}`)
	subject, target := entityID(1), entityID(2)
	rev := h.put(t, subject, entityValue(t, subject,
		stmt{linkTo, target}, stmt{armed, true}, stmt{vocabulary.DCTermsTitle, "Alpha"}, stmt{alias, "alpha-1"}))
	h.awaitIndexed(t, rev)

	requests := everyVerb(subject, target)
	walked := 0
	for _, verb := range graph.QueryVerbs() {
		if verb.Responder != graph.ResponderGraphIndex {
			continue
		}
		walked++
		request, ok := requests[verb.Name]
		if !ok {
			t.Errorf("the table declares %s and this test has no request for it", verb.Name)
			continue
		}
		raw, err := query(t, h.client, verb.Name, request)
		if err != nil {
			t.Errorf("%s: %v", verb.Name, err)
			continue
		}
		if err := replyMatchesEntry(verb, raw); err != nil {
			t.Errorf("%s reply does not match its table entry: %v; reply %s", verb.Name, err, raw)
		}
	}
	if walked == 0 {
		t.Fatal("the table declares no graph-index verb")
	}
}

// TestMaximumLengthIDsAndPredicatesThroughPublicReads: a 256-byte entity ID and a 194-byte predicate
// are indexed, read through every public verb that sees them, and replaced to nothing, so the
// longest key of each index layout is proved on a real server, not by arithmetic (spec "Indexes
// implement replacement"; developer contract, "Prove the longest supported key").
func TestMaximumLengthIDsAndPredicatesThroughPublicReads(t *testing.T) {
	id := func(prefix, last string) string { return prefix + ".b.c.d.e." + strings.Repeat("z", 245) + last }
	src, other, target := id("a", "a"), id("a", "b"), id("b", "a")
	pred := strings.Repeat("a", 64) + "." + strings.Repeat("b", 64) + "." + strings.Repeat("c", 64)
	label := strings.Repeat("a", 64) + "." + strings.Repeat("b", 64) + "." + strings.Repeat("d", 64)
	aliasPred := strings.Repeat("f", 64) + "." + strings.Repeat("b", 64) + "." + strings.Repeat("e", 64)
	if len(src) != 256 || len(target) != 256 || len(pred) != 194 || len(aliasPred) != 194 {
		t.Fatalf("fixture is not maximum length: %d %d %d %d", len(src), len(target), len(pred), len(aliasPred))
	}
	defer vocabulary.SnapshotRegistry()()
	vocabulary.Register(label, vocabulary.WithAlias(vocabulary.AliasTypeLabel, 0))
	vocabulary.Register(aliasPred, vocabulary.WithAlias(vocabulary.AliasTypeIdentity, 0))
	h := startIndex(t, fixtureClient(t), `{}`)
	h.put(t, other, entityValue(t, other, stmt{pred, target}))
	rev := h.put(t, src, entityValue(t, src, stmt{pred, target}, stmt{label, "Longest"}, stmt{aliasPred, "longest-1"}))
	h.awaitIndexed(t, rev)
	// The longest alias key: 64 hex + 256-byte ID + 388 hex of the predicate, two dots.
	if key := nameHash("longest-1") + "." + src + "." + hexOf(aliasPred); len(key) != 710 ||
		!slices.Equal(h.keys(t, graph.BucketAliasIndex), []string{key}) {
		t.Errorf("alias keys = %q, want the one %d-byte key %q", h.keys(t, graph.BucketAliasIndex), len(key), key)
	}
	if got := resolveAlias(t, h, "longest-1", rev); got != src {
		t.Fatalf("alias = %q, want %s", got, src)
	}

	out := answer[graph.OutgoingRelationshipsData](t, h.client, "outgoing", map[string]any{"entity_id": src}, rev)
	if len(out.Relationships) != 1 || out.Relationships[0].ToEntityID != target {
		t.Fatalf("outgoing = %+v", out)
	}
	in := answer[graph.IncomingRelationshipsData](t, h.client, "incoming", map[string]any{"entity_id": target}, rev)
	if len(in.Relationships) != 2 || in.Relationships[0].FromEntityID != src || in.Relationships[1].FromEntityID != other {
		t.Fatalf("incoming = %+v", in)
	}
	pm := answer[graph.PredicateData](t, h.client, "predicate", map[string]any{"predicate": pred}, rev)
	if !slices.Equal(pm.Entities, []string{src, other}) {
		t.Fatalf("predicate = %+v", pm)
	}
	names := answer[graph.NameData](t, h.client, "byName", map[string]any{"name": "Longest"}, rev)
	if len(names.Matches) != 1 || names.Matches[0].EntityID != src {
		t.Fatalf("name = %+v", names)
	}
	for _, scope := range []string{strings.Repeat("a", 64), strings.Repeat("a", 64) + "." + strings.Repeat("b", 64)} {
		list := answer[graph.PredicateListData](t, h.client, "predicateList", map[string]any{"namespace": scope}, rev)
		if len(list.Predicates) != 2 || list.Predicates[0].EntityCount != 2 || list.Predicates[1].EntityCount != 1 {
			t.Fatalf("namespace %s = %+v", scope, list)
		}
	}

	rev = h.put(t, src, entityValue(t, src))
	h.awaitIndexed(t, rev)
	in = answer[graph.IncomingRelationshipsData](t, h.client, "incoming", map[string]any{"entity_id": target}, rev)
	if len(in.Relationships) != 1 || in.Relationships[0].FromEntityID != other {
		t.Fatalf("incoming after replacement = %+v", in)
	}
	pm = answer[graph.PredicateData](t, h.client, "predicate", map[string]any{"predicate": pred}, rev)
	if !slices.Equal(pm.Entities, []string{other}) {
		t.Fatalf("predicate after replacement = %+v", pm)
	}
	names = answer[graph.NameData](t, h.client, "byName", map[string]any{"name": "Longest"}, rev)
	if len(names.Matches) != 0 {
		t.Fatalf("name after replacement = %+v", names)
	}

	rev = h.put(t, src, entityValue(t, src, stmt{aliasPred, "longest-1"}))
	h.awaitIndexed(t, rev)
	if got := resolveAlias(t, h, "longest-1", rev); got != src {
		t.Fatalf("alias before delete = %q, want %s", got, src)
	}
	if err := h.states.Delete(t.Context(), src); err != nil {
		t.Fatalf("delete %s: %v", src, err)
	}
	// A later write's revision is above the delete's: the floor reaching it orders the read
	// after the delete's apply.
	rev = h.put(t, other, entityValue(t, other))
	h.awaitIndexed(t, rev)
	if got := resolveAlias(t, h, "longest-1", rev); got != "" {
		t.Fatalf("alias after delete = %q, want no ID", got)
	}
}

// TestQueryReadsImplementReplacement drives one entity's statements from A to B to none and,
// after each step, reads it through every public read but alias: outgoing, incoming, predicate
// (exact and by value), predicateList, predicateStats, predicateCompound and byName. Each answers
// exactly the current state, and another entity's rows are untouched (spec "Indexes implement
// replacement").
func TestQueryReadsImplementReplacement(t *testing.T) {
	h := startIndex(t, fixtureClient(t), `{"workers": 4}`)
	src, other, targetA, targetB := entityID(1), entityID(2), entityID(10), entityID(11)
	title := vocabulary.DCTermsTitle
	h.put(t, other, entityValue(t, other, stmt{linkTo, targetA}, stmt{armed, false}))

	type state struct {
		outgoing      []graph.OutgoingEntry
		incomingA     []graph.IncomingEntry
		incomingB     []graph.IncomingEntry
		linked        []string // predicate linkTo
		armedTrue     []string // predicate armed, value "true"
		predicates    []graph.PredicateSummary
		disarmedCount int
		compoundAnd   []string // linkTo AND armed
		compoundOr    []string // armed OR disarmed
		alpha, beta   []string // byName
	}
	check := func(step string, rev uint64, want state) {
		t.Helper()
		h.awaitIndexed(t, rev)
		out := answer[graph.OutgoingRelationshipsData](t, h.client, "outgoing", map[string]string{"entity_id": src}, rev)
		if !slices.Equal(out.Relationships, want.outgoing) {
			t.Errorf("%s: outgoing = %v, want %v", step, out.Relationships, want.outgoing)
		}
		for _, c := range []struct {
			target string
			want   []graph.IncomingEntry
		}{{targetA, want.incomingA}, {targetB, want.incomingB}} {
			in := answer[graph.IncomingRelationshipsData](t, h.client, "incoming", map[string]string{"entity_id": c.target}, rev)
			if !slices.Equal(in.Relationships, c.want) {
				t.Errorf("%s: incoming %s = %v, want %v", step, c.target, in.Relationships, c.want)
			}
		}
		linked := answer[graph.PredicateData](t, h.client, "predicate", map[string]string{"predicate": linkTo}, rev)
		if !slices.Equal(linked.Entities, want.linked) {
			t.Errorf("%s: predicate %s = %v, want %v", step, linkTo, linked.Entities, want.linked)
		}
		armedTrue := answer[graph.PredicateData](t, h.client, "predicate",
			map[string]string{"predicate": armed, "value": "true"}, rev)
		if armedTrue.Entities == nil || !slices.Equal(armedTrue.Entities, want.armedTrue) {
			t.Errorf("%s: predicate %s=true = %#v, want %#v", step, armed, armedTrue.Entities, want.armedTrue)
		}
		list := answer[graph.PredicateListData](t, h.client, "predicateList", map[string]string{}, rev)
		if !slices.Equal(list.Predicates, want.predicates) || list.Total != len(want.predicates) {
			t.Errorf("%s: predicateList = %v total %d, want %v", step, list.Predicates, list.Total, want.predicates)
		}
		stats := answer[graph.PredicateStatsData](t, h.client, "predicateStats", map[string]string{"predicate": disarmed}, rev)
		if stats.EntityCount != want.disarmedCount || len(stats.SampleEntities) != want.disarmedCount {
			t.Errorf("%s: predicateStats %s = %+v, want %d entities", step, disarmed, stats, want.disarmedCount)
		}
		and := answer[graph.CompoundPredicateData](t, h.client, "predicateCompound",
			map[string]any{"predicates": []string{linkTo, armed}, "operator": "AND"}, rev)
		if !slices.Equal(and.Entities, want.compoundAnd) || and.Matched != len(want.compoundAnd) {
			t.Errorf("%s: %s AND %s = %+v, want %v", step, linkTo, armed, and, want.compoundAnd)
		}
		or := answer[graph.CompoundPredicateData](t, h.client, "predicateCompound",
			map[string]any{"predicates": []string{armed, disarmed}, "operator": "OR"}, rev)
		if !slices.Equal(or.Entities, want.compoundOr) || or.Matched != len(want.compoundOr) {
			t.Errorf("%s: %s OR %s = %+v, want %v", step, armed, disarmed, or, want.compoundOr)
		}
		for name, wantIDs := range map[string][]string{"alpha": want.alpha, "BETA": want.beta} {
			matches := answer[graph.NameData](t, h.client, "byName", map[string]string{"name": name}, rev).Matches
			ids := make([]string, 0, len(matches))
			for _, m := range matches {
				ids = append(ids, m.EntityID)
			}
			if !slices.Equal(ids, wantIDs) {
				t.Errorf("%s: byName %q = %v, want %v", step, name, ids, wantIDs)
			}
		}
	}
	none := []string{}
	fromOther := []graph.IncomingEntry{{FromEntityID: other, Predicate: linkTo}}

	revA := h.put(t, src, entityValue(t, src, stmt{linkTo, targetA}, stmt{armed, true}, stmt{title, "Alpha"}))
	check("A", revA, state{
		outgoing:  []graph.OutgoingEntry{{ToEntityID: targetA, Predicate: linkTo}},
		incomingA: []graph.IncomingEntry{{FromEntityID: src, Predicate: linkTo}, {FromEntityID: other, Predicate: linkTo}},
		incomingB: []graph.IncomingEntry{},
		linked:    []string{src, other}, armedTrue: []string{src},
		predicates:  []graph.PredicateSummary{{Predicate: title, EntityCount: 1}, {Predicate: linkTo, EntityCount: 2}, {Predicate: armed, EntityCount: 2}},
		compoundAnd: []string{src, other}, compoundOr: []string{src, other},
		alpha: []string{src}, beta: none,
	})

	revB := h.put(t, src, entityValue(t, src, stmt{linkTo, targetB}, stmt{disarmed, true}, stmt{title, "Beta"}))
	check("B", revB, state{
		outgoing:  []graph.OutgoingEntry{{ToEntityID: targetB, Predicate: linkTo}},
		incomingA: fromOther,
		incomingB: []graph.IncomingEntry{{FromEntityID: src, Predicate: linkTo}},
		linked:    []string{src, other}, armedTrue: none,
		predicates:    []graph.PredicateSummary{{Predicate: title, EntityCount: 1}, {Predicate: linkTo, EntityCount: 2}, {Predicate: armed, EntityCount: 1}, {Predicate: disarmed, EntityCount: 1}},
		disarmedCount: 1,
		compoundAnd:   []string{other}, compoundOr: []string{src, other},
		alpha: none, beta: []string{src},
	})

	revNone := h.put(t, src, entityValue(t, src))
	check("none", revNone, state{
		outgoing:  []graph.OutgoingEntry{},
		incomingA: fromOther,
		incomingB: []graph.IncomingEntry{},
		linked:    []string{other}, armedTrue: none,
		predicates:  []graph.PredicateSummary{{Predicate: linkTo, EntityCount: 1}, {Predicate: armed, EntityCount: 1}},
		compoundAnd: []string{other}, compoundOr: []string{other},
		alpha: none, beta: none,
	})
}

// resolveAlias reads alias through the public alias verb and returns the ID it answers, or ""
// when it answers no ID.
func resolveAlias(t *testing.T, h *harness, alias string, minRevision uint64) string {
	t.Helper()
	data := answer[graph.AliasData](t, h.client, "alias", map[string]string{"alias": alias}, minRevision)
	if data.CanonicalID == nil {
		return ""
	}
	return *data.CanonicalID
}

// TestAliasReadImplementsReplacement drives one entity's alias from alpha-1 to beta-1 to none,
// then gives it beta-1 again and deletes the entity, reading both aliases through the public
// alias verb after each step: a dropped or changed alias answers no ID, and a deleted entity's
// alias answers no ID. Another entity's alias answers it throughout (spec "Indexes implement
// replacement", ADR-114).
func TestAliasReadImplementsReplacement(t *testing.T) {
	h := startIndex(t, fixtureClient(t), `{"workers": 4}`)
	src, other := entityID(1), entityID(2)
	h.put(t, other, entityValue(t, other, stmt{alias, "other-1"}))

	check := func(step string, rev uint64, wantAlpha, wantBeta string) {
		t.Helper()
		h.awaitIndexed(t, rev)
		for a, want := range map[string]string{"alpha-1": wantAlpha, "beta-1": wantBeta, "other-1": other} {
			if got := resolveAlias(t, h, a, rev); got != want {
				t.Errorf("%s: alias %q = %q, want %q", step, a, got, want)
			}
		}
	}

	check("A", h.put(t, src, entityValue(t, src, stmt{alias, "alpha-1"})), src, "")
	check("changed", h.put(t, src, entityValue(t, src, stmt{alias, "beta-1"})), "", src)
	check("dropped", h.put(t, src, entityValue(t, src)), "", "")
	check("re-added", h.put(t, src, entityValue(t, src, stmt{alias, "beta-1"})), "", src)
	if err := h.states.Delete(t.Context(), src); err != nil {
		t.Fatalf("delete %s: %v", src, err)
	}
	// A later write's revision is above the delete's: the floor reaching it orders the reads
	// after the delete's apply.
	check("deleted", h.put(t, entityID(50), entityValue(t, entityID(50))), "", "")
}

// TestSharedAliasIsRefusedAsAmbiguous: an alias two entities carry is refused, class invalid,
// code alias_ambiguous, the message naming the alias; once one holder drops it, or is deleted,
// the alias answers the other, so the delete's `*.<entity6>.*` filter leaves the other's row.
// One entity carrying an alias under two predicates is one holder and answers (spec "A shared
// alias is refused, then resolves", "One holder of a shared alias is deleted"; ADR-114 §3).
func TestSharedAliasIsRefusedAsAmbiguous(t *testing.T) {
	const serial = "test.identity.serial"
	defer vocabulary.SnapshotRegistry()()
	vocabulary.Register(serial, vocabulary.WithAlias(vocabulary.AliasTypeIdentity, 0))
	h := startIndex(t, fixtureClient(t), `{"workers": 4}`)
	e1, e2, e3 := entityID(1), entityID(2), entityID(3)

	refused := func(step string, rev uint64) {
		t.Helper()
		h.awaitIndexed(t, rev)
		_, err := query(t, h.client, "alias", map[string]string{"alias": "shared-1"})
		var classified *errs.ClassifiedError
		if !errors.As(err, &classified) {
			t.Errorf("%s: err = %v, want a classified refusal", step, err)
			return
		}
		// The literal, not the constant: callers match the wire string.
		if classified.Class != errs.ErrorInvalid || classified.Code != "alias_ambiguous" {
			t.Errorf("%s: class %v code %q (%v), want invalid alias_ambiguous", step, classified.Class,
				classified.Code, err)
		}
		if !strings.Contains(err.Error(), `"shared-1"`) {
			t.Errorf("%s: message %q does not name the alias", step, err)
		}
	}
	resolves := func(step string, rev uint64, alias, want string) {
		t.Helper()
		h.awaitIndexed(t, rev)
		if got := resolveAlias(t, h, alias, rev); got != want {
			t.Errorf("%s: alias %q = %q, want %q", step, alias, got, want)
		}
	}

	h.put(t, e1, entityValue(t, e1, stmt{alias, "shared-1"}))
	refused("shared", h.put(t, e2, entityValue(t, e2, stmt{alias, "shared-1"})))
	resolves("e2 dropped it", h.put(t, e2, entityValue(t, e2)), "shared-1", e1)
	refused("shared again", h.put(t, e2, entityValue(t, e2, stmt{alias, "shared-1"})))
	if err := h.states.Delete(t.Context(), e2); err != nil {
		t.Fatalf("delete %s: %v", e2, err)
	}
	// A later write's revision is above the delete's: the floor reaching it orders the read
	// after the delete's apply.
	resolves("e2 deleted", h.put(t, entityID(50), entityValue(t, entityID(50))), "shared-1", e1)
	resolves("two predicates", h.put(t, e3, entityValue(t, e3, stmt{alias, "dual-1"}, stmt{serial, "dual-1"})),
		"dual-1", e3)
}

// TestAliasMatchFoldsCase: an alias request is trimmed and case-folded as NAME's is, so ` sn-1 `
// finds the entity carrying `SN-1`; an alias empty after trimming is refused as invalid_request,
// where NAME answers a whitespace-only name (spec "Case and space fold"; ADR-114 §3, §5).
func TestAliasMatchFoldsCase(t *testing.T) {
	h := startIndex(t, fixtureClient(t), `{}`)
	e := entityID(1)
	rev := h.put(t, e, entityValue(t, e, stmt{alias, "SN-1"}))
	h.awaitIndexed(t, rev)
	for _, request := range []string{" sn-1 ", "sn-1", "SN-1"} {
		if got := resolveAlias(t, h, request, rev); got != e {
			t.Errorf("alias %q = %q, want %q", request, got, e)
		}
	}

	_, err := query(t, h.client, "alias", map[string]string{"alias": "   "})
	var classified *errs.ClassifiedError
	if !errors.As(err, &classified) {
		t.Fatalf("whitespace-only alias: err = %v, want a classified refusal", err)
	}
	// The literal, not the constant: callers match the wire string.
	if classified.Class != errs.ErrorInvalid || classified.Code != "invalid_request" {
		t.Errorf("whitespace-only alias: class %v code %q (%v), want invalid invalid_request",
			classified.Class, classified.Code, err)
	}
}

// TestQueriesAreRefusedWhileTheFeedIsLost: ENTITY_STATES deleted under a running graph-index
// loses the feed; once Health names it, each of the eight verbs answers class transient, code
// index_not_ready (spec "Queries are refused while the feed is lost"). Before the delete the
// same requests succeed.
func TestQueriesAreRefusedWhileTheFeedIsLost(t *testing.T) {
	h := startIndex(t, fixtureClient(t), `{}`)
	subject, target := entityID(1), entityID(2)
	rev := h.put(t, subject, entityValue(t, subject, stmt{linkTo, target}, stmt{alias, "alpha-1"}))
	h.awaitIndexed(t, rev)
	requests := everyVerb(subject, target)
	for verb, body := range requests {
		answer[json.RawMessage](t, h.client, verb, body, rev)
	}

	js, err := h.client.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	if err := js.DeleteKeyValue(t.Context(), graph.BucketEntityStates); err != nil {
		t.Fatalf("delete %s: %v", graph.BucketEntityStates, err)
	}
	// A deleted bucket closes the watch within two of nats.go's activity checks (measured 18 s at
	// nats.go v1.54.0). This wait sees the close again at the go.mod version, bounded only by
	// waitBound.
	await(t, "Health naming the lost feed",
		func(context.Context) (component.HealthStatus, error) { return h.c.Health(), nil },
		func(s component.HealthStatus) bool { return strings.Contains(s.LastError, "ENTITY_STATES feed lost") })
	for verb, body := range requests {
		_, err := query(t, h.client, verb, body)
		requireNotReady(t, verb, err)
	}
}

// TestIntegration_KVRevisionAndIndexedRevisionShareOneSpace: the kv_revision graph-ingest's
// mutation reply reports and the indexed_revision of graph-index's replies are one revision space,
// so a caller holding a write's kv_revision can wait for a reply carrying at least it and read the
// write there. The test reads the replies callers read.
func TestIntegration_KVRevisionAndIndexedRevisionShareOneSpace(t *testing.T) {
	client := fixtureClient(t)
	js, err := client.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := natsfixture.CreateStream(t.Context(), js, jetstream.FileStorage, "ENTITY", "entity.>"); err != nil {
		t.Fatalf("create ENTITY stream: %v", err)
	}
	startGraphIngest(t, client)
	h := startIndex(t, client, `{}`)
	mutations, err := graphmutation.NewClient(client, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	id := "c360.test.revspace.system.drone.001"
	now := time.Now()
	created, err := mutations.Create(t.Context(), graph.CreateEntityRequest{
		Entity: &graph.EntityState{ID: id, MessageType: revisionType, UpdatedAt: now},
		Triples: []message.Triple{{
			Subject: id, Predicate: "core.identity.type", Object: "drone", Source: "test", Timestamp: now, Confidence: 1,
		}},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.KVRevision == 0 {
		t.Fatal("the create reported no kv_revision")
	}
	readsWrite(t, h, "core.identity.type", id, created.KVRevision)

	appended, err := mutations.Append(t.Context(), graph.AppendTriplesRequest{Triples: []message.Triple{{
		Subject: id, Predicate: "core.identity.label", Object: "second", Source: "test", Timestamp: time.Now(), Confidence: 1,
	}}})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if len(appended.Results) != 1 || appended.Results[0].Outcome != graph.MutationApplied {
		t.Fatalf("append results = %+v, want one applied", appended.Results)
	}
	second := appended.Results[0].KVRevision
	if second <= created.KVRevision {
		t.Fatalf("the append's kv_revision %d is not above the create's %d", second, created.KVRevision)
	}
	readsWrite(t, h, "core.identity.label", id, second)
}

// readsWrite waits for a predicate reply carrying at least revision, and requires it to hold id:
// the write at revision is reflected in a reply whose indexed_revision covers it.
func readsWrite(t *testing.T, h *harness, predicate, id string, revision uint64) {
	t.Helper()
	body := map[string]string{"predicate": predicate}
	reply := await(t, fmt.Sprintf("a %s reply carrying revision %d", predicate, revision),
		func(context.Context) (envelope[graph.PredicateData], error) {
			var reply envelope[graph.PredicateData]
			raw, err := query(t, h.client, "predicate", body)
			if err != nil {
				return reply, err
			}
			return reply, json.Unmarshal(raw, &reply)
		},
		func(reply envelope[graph.PredicateData]) bool { return reply.IndexedRevision >= revision })
	if !slices.Contains(reply.Data.Entities, id) {
		t.Errorf("a %s reply at indexed_revision %d (write at %d) = %v, want it to hold %s", predicate,
			reply.IndexedRevision, revision, reply.Data.Entities, id)
	}
}

var revisionType = message.Type{Domain: "test", Category: "revision", Version: "v1"}

// startGraphIngest starts graph-ingest under the c360.test authority on client, the writer whose
// mutation replies report kv_revision; the test's cleanup stops it.
func startGraphIngest(t *testing.T, client *natsclient.Client) {
	t.Helper()
	registry := payloadfixture.NewWithSubset(t, message.RegisterPayloads, inference.RegisterPayloads, lifecycle.RegisterPayloads)
	payloadfixture.RegisterTestType(t, registry, revisionType)
	raw, err := json.Marshal(graphingest.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	built, err := graphingest.CreateGraphIngest(raw, component.Dependencies{
		NATSClient: client, PayloadRegistry: registry, MetricsRegistry: metric.NewMetricsRegistry(),
		Platform: types.PlatformMeta{Org: "c360", Platform: "test"},
	})
	if err != nil {
		t.Fatalf("CreateGraphIngest: %v", err)
	}
	ingest := built.(component.LifecycleComponent)
	if err := ingest.Initialize(); err != nil {
		t.Fatal(err)
	}
	if err := ingest.Start(t.Context()); err != nil {
		t.Fatalf("start graph-ingest: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), waitBound)
		defer cancel()
		if err := ingest.Stop(ctx); err != nil {
			t.Errorf("stop graph-ingest: %v", err)
		}
	})
}

// TestMalformedIndexRowsAreSkippedLoggedAndCounted: a stored index row that is not its index's
// layout is left out of every query that lists it, and the skip is declared: a warning names the
// index, and query_rows_skipped_total counts it by index, while the query still answers the
// well-formed rows. No graph-index writer stores such a row, so the test writes each
// one into its bucket directly.
func TestMalformedIndexRowsAreSkippedLoggedAndCounted(t *testing.T) {
	h := startIndex(t, fixtureClient(t), `{}`)
	src, target, stranger := entityID(1), entityID(2), entityID(3)
	title := vocabulary.DCTermsTitle
	rev := h.put(t, src, entityValue(t, src, stmt{linkTo, target}, stmt{title, "Alpha"}, stmt{alias, "alpha-1"}))
	h.awaitIndexed(t, rev)

	// Spelled from the layouts (target.source6.hex(predicate), predicate3.entity6,
	// sha256(folded name).entity6.hex(predicate), the same for an alias), not built by the package.
	for bucket, rows := range map[string]map[string]string{
		graph.BucketIncomingIndex: {target + ".not-a-row": "{}"},
		// Nine tokens, so the exact predicate filter lists it, but "_bad" is no entity ID segment.
		graph.BucketPredicateIndex: {linkTo + ".acme.ops.test.system.widget._bad": "{}"},
		graph.BucketNameIndex: {
			nameHash("Alpha") + ".not-a-row":                        "{}",
			nameHash("Alpha") + "." + stranger + "." + hexOf(title): "not json",
		},
		graph.BucketAliasIndex: {nameHash("alpha-1") + ".not-a-row": "{}"},
	} {
		kv, err := kvcatalog.EnsureCatalogBucket(t.Context(), h.client, bucket)
		if err != nil {
			t.Fatalf("open %s: %v", bucket, err)
		}
		for k, v := range rows {
			if _, err := kv.Put(t.Context(), k, []byte(v)); err != nil {
				t.Fatalf("put %s/%s: %v", bucket, k, err)
			}
		}
	}

	// Each handler logs and counts its skips before it replies, so every reply below orders the
	// log and counter reads after them.
	in := answer[graph.IncomingRelationshipsData](t, h.client, "incoming", map[string]string{"entity_id": target}, rev)
	if want := []graph.IncomingEntry{{FromEntityID: src, Predicate: linkTo}}; !slices.Equal(in.Relationships, want) {
		t.Errorf("incoming = %v, want %v", in.Relationships, want)
	}
	linked := answer[graph.PredicateData](t, h.client, "predicate", map[string]string{"predicate": linkTo}, rev)
	if !slices.Equal(linked.Entities, []string{src}) {
		t.Errorf("predicate %s = %v, want [%s]", linkTo, linked.Entities, src)
	}
	list := answer[graph.PredicateListData](t, h.client, "predicateList", map[string]string{}, rev)
	if want := []graph.PredicateSummary{{Predicate: alias, EntityCount: 1}, {Predicate: title, EntityCount: 1}, {Predicate: linkTo, EntityCount: 1}}; !slices.Equal(list.Predicates, want) {
		t.Errorf("predicateList = %v, want %v", list.Predicates, want)
	}
	names := answer[graph.NameData](t, h.client, "byName", map[string]string{"name": "Alpha"}, rev)
	if len(names.Matches) != 1 || names.Matches[0].EntityID != src {
		t.Errorf("byName Alpha = %+v, want %s only", names.Matches, src)
	}
	if got := resolveAlias(t, h, "alpha-1", rev); got != src {
		t.Errorf("alias alpha-1 = %q, want %s", got, src)
	}

	logs := h.logs.String()
	for _, index := range []string{"incoming", "predicate", "name", "alias"} {
		warned := regexp.MustCompile(`level=WARN msg="index query: malformed row skipped" .*index_type=` + index + ` `)
		if !warned.MatchString(logs) {
			t.Errorf("no warning naming the %s index's skipped row; logs:\n%s", index, logs)
		}
	}
	// incoming: one key; predicate: the bad key, listed by the predicate query and again by
	// predicateList; name: the bad key and the bad value; alias: one key.
	want := map[string]float64{"incoming": 1, "predicate": 2, "name": 2, "alias": 1}
	got := map[string]float64{}
	families, err := h.c.metricsRegistry.PrometheusRegistry().Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	for _, family := range families {
		if family.GetName() != "semengine_graph_index_query_rows_skipped_total" {
			continue
		}
		for _, m := range family.GetMetric() {
			for _, label := range m.GetLabel() {
				if label.GetName() == "index_type" {
					got[label.GetValue()] += m.GetCounter().GetValue()
				}
			}
		}
	}
	if !maps.Equal(got, want) {
		t.Errorf("semengine_graph_index_query_rows_skipped_total by index_type = %v, want %v", got, want)
	}
}
