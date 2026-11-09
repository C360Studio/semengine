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
	"testing"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/vocabulary"
)

// indexedRevisionOf reads a revision_not_reached refusal: its detail names how far the index has got.
func indexedRevisionOf(t *testing.T, reply []byte, err error) uint64 {
	t.Helper()
	got := requireRefusal(t, reply, err, errs.ErrorTransient, "revision_not_reached")
	reached, ok := got.Detail["indexed_revision"].(float64)
	if !ok {
		t.Fatalf("refusal detail = %v, want indexed_revision", got.Detail)
	}
	return uint64(reached)
}

// TestReadYourWrites: a caller that wrote at revision W and asks relationships with min_revision W gets
// the write or revision_not_reached, never a success below W (spec, "Reading a write back"). The index
// may or may not lag on any one read and no test can choose which, so each write is read straight back,
// retrying only on that refusal; a build that never catches up hangs until go test -timeout. A build
// that drops the field passes that loop whenever the index has caught up, so the end of the test asks
// for one revision past the index's own, with every write finished, which only a request that still
// carries the field can refuse.
func TestReadYourWrites(t *testing.T) {
	stack := startStack(t)
	// Readiness is not what this test reads: below, the one refusal allowed is revision_not_reached.
	stack.awaitIndexed(t, 0)

	read := func(t *testing.T, entity, direction string, minRevision uint64) ([]byte, error) {
		t.Helper()
		request := fmt.Sprintf(`{"entity_id":%q,"direction":%q,"min_revision":%d}`, entity, direction, minRevision)
		return stack.client.RequestClassified(t.Context(), "graph.query.relationships", []byte(request), requestTimeout)
	}

	const rounds = 20
	var written, seen uint64
	refused := 0
	for round := range rounds {
		from := fmt.Sprintf("c360.test.query.system.drone.1%02d", round)
		to := fmt.Sprintf("c360.test.query.system.drone.2%02d", round)
		written = stack.create(t, from, [2]string{linkTo, to})
		want := []graph.RelationshipEdge{{Predicate: linkTo, FromEntityID: from, ToEntityID: to}}

		for _, ask := range []struct{ entity, direction string }{{from, "outgoing"}, {to, "incoming"}} {
			var raw []byte
			for {
				var err error
				raw, err = read(t, ask.entity, ask.direction, written)
				if err == nil {
					break
				}
				if reached := indexedRevisionOf(t, raw, err); reached >= written {
					t.Fatalf("%s %s: refused at indexed_revision %d, at or above the %d asked for", ask.direction, ask.entity, reached, written)
				}
				refused++
			}
			envelope := decodeEnvelope(t, raw)
			if envelope.IndexedRevision < written {
				t.Fatalf("%s %s: a success with indexed_revision %d, below the write's %d", ask.direction, ask.entity, envelope.IndexedRevision, written)
			}
			var edges []graph.RelationshipEdge
			if err := json.Unmarshal(envelope.Data, &edges); err != nil {
				t.Fatalf("decode %q: %v", envelope.Data, err)
			}
			if !reflect.DeepEqual(edges, want) {
				t.Errorf("%s %s at revision %d: edges = %+v, want the write's %+v", ask.direction, ask.entity, written, edges, want)
			}
			seen = envelope.IndexedRevision
		}
	}
	// Whether a real lag happened varies by run, so nothing asserts on it.
	t.Logf("%d reads refused while the index caught up", refused)

	// No write is in flight, so the index stays at the last write's revision: the reply's revision is it,
	// and the next revision has not been reached, however fast the index is.
	if seen != written {
		t.Fatalf("indexed_revision = %d after the last write's %d returned, and nothing else writes", seen, written)
	}
	last := fmt.Sprintf("c360.test.query.system.drone.1%02d", rounds-1)
	raw, err := read(t, last, "outgoing", written+1)
	if reached := indexedRevisionOf(t, raw, err); reached != written {
		t.Errorf("refusal names indexed_revision %d, want the last write's %d", reached, written)
	}
}

// TestMinRevisionRefusalThroughGraphQuery: each enveloped verb asked for a revision far above the
// index's passes on the index's refusal with class, code and detail, localSearch refuses on its
// group record's revision and the text tier of globalSearch and searchGraph on its level's highest, while the same
// request without the field, or at the revision reached, succeeds, so it is the field that was refused. The index and the record have reached the only write
// and nothing else writes, so the detail is that write's revision.
func TestMinRevisionRefusalThroughGraphQuery(t *testing.T) {
	stack := startCommunityStack(t)
	written := stack.write(t, drone1,
		[2]string{vocabulary.DCTermsTitle, "Alpha"},
		[2]string{vocabulary.CoreIdentityAlias, "held-1"},
		[2]string{linkTo, drone2})
	tick(t, stack.maintainer)
	stack.awaitIndexed(t, written)
	awaitCommunityView(t, stack.client, drone1, written)

	for _, tc := range []struct{ name, verb, body string }{
		{"byName", "byName", `"name":"Alpha"`},
		{"relationships outgoing", "relationships", fmt.Sprintf(`"entity_id":%q,"direction":"outgoing"`, drone1)},
		{"relationships incoming", "relationships", fmt.Sprintf(`"entity_id":%q,"direction":"incoming"`, drone2)},
		{"entityByAlias", "entityByAlias", `"alias":"held-1"`},
		{"pathSearch outgoing", "pathSearch", fmt.Sprintf(`"start_entity":%q,"direction":"outgoing","max_depth":1`, drone1)},
		{"pathSearch incoming", "pathSearch", fmt.Sprintf(`"start_entity":%q,"direction":"incoming","max_depth":1`, drone1)},
		{"summary", "summary", `"examples_per_type":1`},
		// The record's entity_revision is the write's: the one write to its group.
		{"localSearch", "localSearch", fmt.Sprintf(`"entity_id":%q,"query":"drone"`, drone1)},
		// The alias verb refuses for both strategies, before the entity read or the walk.
		{"globalSearch entity", "globalSearch", `"query":"held-1"`},
		{"globalSearch path", "globalSearch", `"query":"what is connected to held-1"`},
		// The text tier's mark is the highest entity_revision at level 0: drone1's group's, the write's.
		{"globalSearch text", "globalSearch", `"query":"alpha drone"`},
		{"searchGraph entity", "searchGraph", `"query":"held-1"`},
		{"searchGraph path", "searchGraph", `"query":"what is connected to held-1"`},
		{"searchGraph text", "searchGraph", `"query":"alpha drone"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			subject := verbRow(t, graph.ResponderGraphQuery, tc.verb).Subject
			ask := func(request string) ([]byte, error) {
				return stack.client.RequestClassified(t.Context(), subject, []byte(request), requestTimeout)
			}
			for _, request := range []string{
				"{" + tc.body + "}",
				fmt.Sprintf(`{%s,"min_revision":%d}`, tc.body, written),
			} {
				raw, err := ask(request)
				if err != nil {
					t.Fatalf("%s: %v", request, err)
				}
				if !bytes.Contains(raw, []byte(drone1)) {
					t.Fatalf("%s: reply %q does not name the entity written", request, raw)
				}
			}
			raw, err := ask(fmt.Sprintf(`{%s,"min_revision":%d}`, tc.body, farAhead))
			if reached := indexedRevisionOf(t, raw, err); reached != written {
				t.Errorf("refusal names indexed_revision %d, want the write's %d", reached, written)
			}
		})
	}
}

// replyMatchesEntry decodes raw as a caller holding verb's table entry does: the envelope when the
// entry is Enveloped, then the entry's Reply, each refusing an unknown field. Re-encoding the decoded
// body must give the same JSON, so a Reply with fields the wire does not carry fails too. The third
// copy, with graph-index's and graph-ingest's.
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

// TestGraphQueryRepliesDecodeByTheTable: every row the verb table declares for graph-query, not a list
// kept here, gets a reply naming the entity written, and that reply has the row's shape and type
// (design D1). The reply is real, from real ingest and index, except on the four forwards whose
// responders changes 5 and 7 bring: there a stub sends the literal JSON the row declares.
// relationships is read both ways, its two reply paths.
func TestGraphQueryRepliesDecodeByTheTable(t *testing.T) {
	stack := startCommunityStack(t)
	stack.write(t, drone1,
		[2]string{vocabulary.DCTermsTitle, "Alpha"},
		[2]string{vocabulary.CoreIdentityAlias, "held-1"},
		[2]string{linkTo, drone2})
	written := stack.write(t, drone3)
	tick(t, stack.maintainer)
	stack.awaitIndexed(t, written)
	awaitCommunityView(t, stack.client, drone1, written)

	for _, stub := range []struct{ responder, name, reply string }{
		{graph.ResponderGraphEmbedding, "search", `{"data":{"results":[{"entity_id":"c360.test.query.system.drone.001","similarity":0.75}],"embedder_type":"bm25","unreadable_records":1,"failed_records":2},"indexed_revision":9,"producer":"graph-embedding","timestamp":"2026-11-04T10:00:00Z"}`},
		{graph.ResponderGraphEmbedding, "similar", `{"data":{"results":[{"entity_id":"c360.test.query.system.drone.001","similarity":0.5}],"unreadable_records":1,"failed_records":2},"indexed_revision":9,"producer":"graph-embedding","timestamp":"2026-11-04T10:00:00Z"}`},
		{graph.ResponderGraphIndexSpatial, "bounds", `{"data":{"results":[{"entity_id":"c360.test.query.system.drone.001","lat":48.25,"lon":11.5,"alt":310.5}],"truncated":true},"indexed_revision":9,"producer":"graph-index-spatial","timestamp":"2026-11-04T10:00:00Z"}`},
		{graph.ResponderGraphIndexTemporal, "range", `{"data":{"results":[{"entity_id":"c360.test.query.system.drone.001"}],"truncated":false},"indexed_revision":9,"producer":"graph-index-temporal","timestamp":"2026-11-04T10:00:00Z"}`},
	} {
		serve(t, stack.client, verbRow(t, stub.responder, stub.name).Subject, func(context.Context, []byte) ([]byte, error) {
			return []byte(stub.reply), nil
		})
	}

	requests := map[string][]string{
		"semantic":      {`{"query":"alpha","scope":["team-a"]}`},
		"similar":       {fmt.Sprintf(`{"entity_id":%q}`, drone2)},
		"spatial":       {`{"north":49,"south":48,"east":12,"west":11}`},
		"temporal":      {`{"start_time":"2026-11-04T00:00:00Z","end_time":"2026-11-05T00:00:00Z"}`},
		"entity":        {fmt.Sprintf(`{"id":%q}`, drone1)},
		"batch":         {fmt.Sprintf(`{"ids":[%q,%q]}`, drone1, absentDrone)},
		"prefix":        {`{"prefix":"c360","limit":1}`},
		"byName":        {`{"name":"Alpha"}`},
		"entityByAlias": {`{"alias":"held-1"}`},
		"relationships": {
			fmt.Sprintf(`{"entity_id":%q,"direction":"outgoing"}`, drone1),
			fmt.Sprintf(`{"entity_id":%q,"direction":"incoming"}`, drone2),
		},
		"pathSearch":     {fmt.Sprintf(`{"start_entity":%q,"direction":"outgoing","max_depth":2,"predicates":[%q]}`, drone1, linkTo)},
		"hierarchyStats": {`{"prefix":"c360.test.query.system.drone"}`},
		"summary":        {`{}`, `{"prefix":"c360.test.query.system.drone","exclude_predicates":true}`},
		"localSearch":    {fmt.Sprintf(`{"entity_id":%q,"query":"alpha"}`, drone1)},
		"globalSearch":   {`{"query":"held-1"}`, `{"query":"what is connected to held-1"}`},
		"searchGraph":    {`{"query":"held-1"}`, `{"query":"what is connected to held-1"}`},
	}
	walked := 0
	for _, verb := range graph.QueryVerbs() {
		if verb.Responder != graph.ResponderGraphQuery {
			continue
		}
		walked++
		asks, ok := requests[verb.Name]
		if !ok {
			t.Errorf("the table declares %s and this test has no request for it", verb.Name)
			continue
		}
		for _, request := range asks {
			raw, err := stack.client.RequestClassified(t.Context(), verb.Subject, []byte(request), requestTimeout)
			if err != nil {
				t.Errorf("%s %s: %v", verb.Name, request, err)
				continue
			}
			// A reply with nothing in it would match any entry.
			if !strings.Contains(string(raw), drone1) {
				t.Errorf("%s %s: reply %s does not name the entity written", verb.Name, request, raw)
			}
			if err := replyMatchesEntry(verb, raw); err != nil {
				t.Errorf("%s %s: reply does not match its table entry: %v; reply %s", verb.Name, request, err, raw)
			}
		}
	}
	if walked == 0 {
		t.Fatal("the table declares no graph-query verb")
	}
}
