package fusionnats

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/fusion"
	semtypes "github.com/c360studio/semengine/pkg/types"
)

// The subjects of the rows the client uses, graph-query's and graph-lexical's
// search, spelled here as an oracle independent of the verb table the client
// reads them from.
const (
	subjectEntity        = "graph.query.entity"
	subjectBatch         = "graph.query.batch"
	subjectPrefixIDs     = "graph.query.prefixIDs"
	subjectByName        = "graph.query.byName"
	subjectRelationships = "graph.query.relationships"
	subjectWordSearch    = "graph.lexical.query.search"
)

// fakeRequester records the last request and returns a canned reply, or routes
// per subject through responder when set.
type fakeRequester struct {
	calls       int
	lastSubject string
	lastData    []byte
	lastTimeout time.Duration
	resp        []byte
	err         error
	responder   func(subject string, data []byte) ([]byte, error)
}

func (f *fakeRequester) RequestClassified(_ context.Context, subject string, data []byte, timeout time.Duration) ([]byte, error) {
	f.calls++
	f.lastSubject = subject
	f.lastData = append([]byte(nil), data...)
	f.lastTimeout = timeout
	if f.responder != nil {
		return f.responder(subject, data)
	}
	return f.resp, f.err
}

func newClient(t *testing.T, fake *fakeRequester) *Client {
	t.Helper()
	c, err := New(fake, time.Second)
	require.NoError(t, err)
	return c
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

// enveloped is data as an enveloped row's reply, built the way a responder
// builds one.
func enveloped[T any](t *testing.T, data T) []byte {
	t.Helper()
	reply, err := graph.NewQueryResponse("graph-index", 1, data)
	require.NoError(t, err)
	return mustJSON(t, reply)
}

// stamped is a triple that carries the statement metadata the stored-value rule
// requires of an entity's reply.
func stamped(subject, predicate string, object any) message.Triple {
	return message.Triple{
		Subject: subject, Predicate: predicate, Object: object,
		Source: "test", Timestamp: time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC),
	}
}

func TestNewRefusesNilRequester(t *testing.T) {
	c, err := New(nil, time.Second)
	require.Error(t, err)
	assert.Nil(t, c)
}

func TestNewRefusesNegativeTimeout(t *testing.T) {
	c, err := New(&fakeRequester{}, -time.Nanosecond)
	require.Error(t, err)
	assert.Nil(t, c)
}

// TestNewZeroTimeoutUsesDefault holds that a zero timeout reaches the requester
// as zero, which natsclient replaces with its DefaultRequestTimeout, rather than
// a default of the client's own.
func TestNewZeroTimeoutUsesDefault(t *testing.T) {
	fake := &fakeRequester{resp: mustJSON(t, graph.PrefixIDsResponse{IDs: []string{}})}
	c, err := New(fake, 0)
	require.NoError(t, err)
	_, err = c.Resolve(t.Context(), fusion.ResolveQuery{Mode: fusion.ResolveModePrefix, Limit: 1})
	require.NoError(t, err)
	assert.Equal(t, 1, fake.calls)
	assert.Equal(t, time.Duration(0), fake.lastTimeout)

	fake = &fakeRequester{resp: mustJSON(t, graph.PrefixIDsResponse{IDs: []string{}})}
	c, err = New(fake, 3*time.Second)
	require.NoError(t, err)
	_, err = c.Resolve(t.Context(), fusion.ResolveQuery{Mode: fusion.ResolveModePrefix, Limit: 1})
	require.NoError(t, err)
	assert.Equal(t, 3*time.Second, fake.lastTimeout, "a set timeout reaches the requester unchanged")
}

// TestSubjectsComeFromVerbTable drives every RetrievalClient method and checks
// each request went to its row's subject: graph-lexical's search for an NL
// resolve, graph-query's rows for the rest.
func TestSubjectsComeFromVerbTable(t *testing.T) {
	id := "acme.ops.robotics.gcs.drone.001"
	replies := map[string][]byte{
		subjectEntity:        mustJSON(t, graph.ExactEntity{Entity: &graph.EntityState{ID: id}, KVRevision: 1}),
		subjectBatch:         mustJSON(t, graph.EntityBatchResponse{}),
		subjectPrefixIDs:     mustJSON(t, graph.PrefixIDsResponse{IDs: []string{}}),
		subjectByName:        enveloped(t, graph.NameData{}),
		subjectRelationships: enveloped(t, []graph.RelationshipEdge{}),
		subjectWordSearch:    enveloped(t, graph.LexicalSearchData{}),
	}
	fake := &fakeRequester{responder: func(subject string, _ []byte) ([]byte, error) {
		reply, ok := replies[subject]
		if !ok {
			return nil, errors.New("no reply for " + subject)
		}
		return reply, nil
	}}
	c := newClient(t, fake)
	ctx := t.Context()
	calls := []struct {
		name string
		want string
		call func() error
	}{
		{"Resolve symbol", subjectByName, func() error {
			_, err := c.Resolve(ctx, fusion.ResolveQuery{Query: "Scout", Mode: fusion.ResolveModeSymbol})
			return err
		}},
		{"Resolve prefix", subjectPrefixIDs, func() error {
			_, err := c.Resolve(ctx, fusion.ResolveQuery{Query: "acme.ops", Mode: fusion.ResolveModePrefix})
			return err
		}},
		{"Resolve nl", subjectWordSearch, func() error {
			_, err := c.Resolve(ctx, fusion.ResolveQuery{Query: "scout", Mode: fusion.ResolveModeNL})
			return err
		}},
		{"Entity", subjectEntity, func() error { _, err := c.Entity(ctx, id); return err }},
		{"Entities", subjectBatch, func() error { _, err := c.Entities(ctx, []string{id}); return err }},
		{"Neighbors", subjectRelationships, func() error {
			_, err := c.Neighbors(ctx, id, nil, fusion.Outgoing)
			return err
		}},
		{"Names", subjectByName, func() error { _, err := c.Names(ctx, "Scout", 5); return err }},
	}
	for _, tc := range calls {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, tc.call())
			assert.Equal(t, tc.want, fake.lastSubject)
		})
	}
}

// TestVerbLookupNamesMissingRow holds that a table without a row the client
// uses is refused, naming the verb. graph-embedding also has a search row, so
// the lexical case shows the row is matched by its responder too.
func TestVerbLookupNamesMissingRow(t *testing.T) {
	without := func(responder, name string) []graph.QueryVerb {
		var table []graph.QueryVerb
		for _, verb := range graph.QueryVerbs() {
			if verb.Responder == responder && verb.Name == name {
				continue
			}
			table = append(table, verb)
		}
		return table
	}
	for _, missing := range []struct{ responder, name string }{
		{graph.ResponderGraphQuery, "batch"},
		{graph.ResponderGraphLexical, "search"},
	} {
		_, err := lookupVerbs(without(missing.responder, missing.name))
		require.Error(t, err, "a table without %s's %s", missing.responder, missing.name)
		assert.Contains(t, err.Error(), missing.responder)
		assert.Contains(t, err.Error(), `"`+missing.name+`"`)
	}

	_, err := lookupVerbs(graph.QueryVerbs())
	require.NoError(t, err, "the full table holds every row the client uses")
}

// TestEnvelopedRowNeedsProducer holds that an enveloped row's reply is decoded as
// an envelope, never by its content: a bare body, and "{}", which decodes as an
// envelope naming no producer, are both refused.
func TestEnvelopedRowNeedsProducer(t *testing.T) {
	id := "acme.ops.robotics.gcs.drone.001"
	neighbor := "acme.ops.robotics.gcs.sensor.002"
	rows := []struct {
		name string
		bare []byte
		call func(*Client) error
	}{
		{"byName", mustJSON(t, graph.NameData{Matches: []graph.NameMatch{{EntityID: id, MatchedName: "Scout"}}}),
			func(c *Client) error { _, err := c.Names(t.Context(), "Sco", 5); return err }},
		{"relationships", mustJSON(t, []graph.RelationshipEdge{{FromEntityID: id, ToEntityID: neighbor, Predicate: "robotics.sensor.reads"}}),
			func(c *Client) error { _, err := c.Neighbors(t.Context(), id, nil, fusion.Outgoing); return err }},
		{"search", mustJSON(t, graph.LexicalSearchData{Results: []graph.LexicalHit{{EntityID: id, Score: 0.9}}}),
			func(c *Client) error {
				_, err := c.Resolve(t.Context(), fusion.ResolveQuery{Query: "scout", Mode: fusion.ResolveModeNL})
				return err
			}},
	}
	for _, row := range rows {
		for name, reply := range map[string][]byte{"bare body": row.bare, "empty object": []byte(`{}`)} {
			t.Run(row.name+" "+name, func(t *testing.T) {
				err := row.call(newClient(t, &fakeRequester{resp: reply}))
				require.Error(t, err)
			})
		}
		t.Run(row.name+" enveloped", func(t *testing.T) {
			var data any
			require.NoError(t, json.Unmarshal(row.bare, &data))
			require.NoError(t, row.call(newClient(t, &fakeRequester{resp: enveloped(t, data)})))
		})
	}
}

func TestResolve_Symbol(t *testing.T) {
	fake := &fakeRequester{resp: enveloped(t, graph.NameData{Matches: []graph.NameMatch{
		{EntityID: "a.b.c.d.e.1", MatchedName: "Widget"},
		{EntityID: "a.b.c.d.e.2", MatchedName: "Widget"},
	}})}
	c := newClient(t, fake)

	res, err := c.Resolve(t.Context(), fusion.ResolveQuery{Query: "Widget", Mode: fusion.ResolveModeSymbol, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, subjectByName, fake.lastSubject)
	var req map[string]any
	require.NoError(t, json.Unmarshal(fake.lastData, &req))
	assert.Equal(t, "Widget", req["name"], "the query travels under name")
	assert.Equal(t, float64(10), req["limit"])
	assert.Equal(t, []string{"a.b.c.d.e.1", "a.b.c.d.e.2"}, fusion.SeedIDs(res.Seeds))
}

// TestResolve_Prefix holds that a prefix resolve sends one listing request, the query as prefix
// with the resolve limit, to graph-query's prefixIDs row, and returns one seed per listed ID in
// the order listed, with no score. The fake lists out of key order so an order of the client's
// own would show.
func TestResolve_Prefix(t *testing.T) {
	fake := &fakeRequester{resp: mustJSON(t, graph.PrefixIDsResponse{
		IDs: []string{"a.b.c.d.e.2", "a.b.c.d.e.1"}, Total: 2,
	})}
	c := newClient(t, fake)

	res, err := c.Resolve(t.Context(), fusion.ResolveQuery{Query: "a.b.c", Mode: fusion.ResolveModePrefix, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 1, fake.calls)
	assert.Equal(t, subjectPrefixIDs, fake.lastSubject)
	assert.JSONEq(t, `{"prefix":"a.b.c","limit":10}`, string(fake.lastData))
	assert.Equal(t, fusion.Resolution{Seeds: []fusion.Seed{{ID: "a.b.c.d.e.2"}, {ID: "a.b.c.d.e.1"}}}, res)
}

func TestResolve_InvalidPrefixFailsBeforeNATS(t *testing.T) {
	fake := &fakeRequester{}
	c := newClient(t, fake)
	_, err := c.Resolve(t.Context(), fusion.ResolveQuery{Query: "acme.*", Mode: fusion.ResolveModePrefix, Limit: 10})
	require.Error(t, err)
	var classified *errs.ClassifiedError
	require.ErrorAs(t, err, &classified)
	assert.Equal(t, semtypes.ErrorCodeEntityIDPrefixInvalid, classified.Code)
	assert.Zero(t, fake.calls, "an invalid prefix fails before a NATS request")
}

func TestResolve_EmptyPrefixRemainsMatchAll(t *testing.T) {
	fake := &fakeRequester{resp: mustJSON(t, graph.PrefixIDsResponse{IDs: []string{}})}
	c := newClient(t, fake)
	_, err := c.Resolve(t.Context(), fusion.ResolveQuery{Mode: fusion.ResolveModePrefix, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, subjectPrefixIDs, fake.lastSubject)
}

// TestNLResolveAsksTheWordIndex holds that an NL resolve sends one word search
// request holding the query, limit and scope to graph-lexical's search row, and
// returns the hits as seeds in the reply's order, each with its score. The hits
// are in neither ID nor score order, so a resolve that sorted them would fail.
func TestNLResolveAsksTheWordIndex(t *testing.T) {
	fake := &fakeRequester{resp: enveloped(t, graph.LexicalSearchData{Results: []graph.LexicalHit{
		{EntityID: "a.b.c.d.e.2", Score: 1.5},
		{EntityID: "a.b.c.d.e.3", Score: 4.25},
		{EntityID: "a.b.c.d.e.1", Score: 0.75},
	}, Matched: 3})}
	c := newClient(t, fake)

	res, err := c.Resolve(t.Context(), fusion.ResolveQuery{
		Query: "retry -race", Mode: fusion.ResolveModeNL, Scope: []string{"a.b.c"}, Limit: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, fake.calls)
	assert.Equal(t, subjectWordSearch, fake.lastSubject)
	assert.JSONEq(t, `{"query":"retry -race","limit":10,"scope":["a.b.c"]}`, string(fake.lastData))
	assert.Equal(t, []fusion.Seed{
		{ID: "a.b.c.d.e.2", Similarity: 1.5, HasSimilarity: true},
		{ID: "a.b.c.d.e.3", Similarity: 4.25, HasSimilarity: true},
		{ID: "a.b.c.d.e.1", Similarity: 0.75, HasSimilarity: true},
	}, res.Seeds)
}

// TestResolve_NL_UnscopedByteParity holds ADR-071's rule that an unscoped
// request carries no scope key.
func TestResolve_NL_UnscopedByteParity(t *testing.T) {
	fake := &fakeRequester{resp: enveloped(t, graph.LexicalSearchData{})}
	c := newClient(t, fake)
	_, err := c.Resolve(t.Context(), fusion.ResolveQuery{Query: "find a widget", Mode: fusion.ResolveModeNL, Limit: 10})
	require.NoError(t, err)
	assert.JSONEq(t, `{"query":"find a widget","limit":10}`, string(fake.lastData))
}

func TestResolve_NLInvalidScopeFailsBeforeNATS(t *testing.T) {
	fake := &fakeRequester{}
	c := newClient(t, fake)
	_, err := c.Resolve(t.Context(), fusion.ResolveQuery{
		Query: "find a widget", Mode: fusion.ResolveModeNL, Scope: []string{"acme.*"}, Limit: 10,
	})
	require.Error(t, err)
	var classified *errs.ClassifiedError
	require.ErrorAs(t, err, &classified)
	assert.Equal(t, semtypes.ErrorCodeEntityIDPrefixInvalid, classified.Code)
	assert.Zero(t, fake.calls, "an invalid scope fails before a NATS request")
}

func TestResolve_NLEmptyScopeEntryRemainsMatchAll(t *testing.T) {
	fake := &fakeRequester{resp: enveloped(t, graph.LexicalSearchData{})}
	c := newClient(t, fake)
	_, err := c.Resolve(t.Context(), fusion.ResolveQuery{
		Query: "find a widget", Mode: fusion.ResolveModeNL, Scope: []string{""}, Limit: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, subjectWordSearch, fake.lastSubject)
	assert.JSONEq(t, `{"query":"find a widget","limit":10,"scope":[""]}`, string(fake.lastData))
}

// TestWordIndexCountsReachResolution holds that an NL resolve copies the word
// index reply's bodies_not_indexed and excluded into the Resolution, and that a
// symbol or prefix resolve leaves both empty even when its reply carries keys of
// those names.
func TestWordIndexCountsReachResolution(t *testing.T) {
	t.Run("nl", func(t *testing.T) {
		fake := &fakeRequester{resp: enveloped(t, graph.LexicalSearchData{
			Results:  []graph.LexicalHit{{EntityID: "a.b.c.d.e.1", Score: 1}},
			Matched:  1,
			Excluded: []string{"race"}, BodiesNotIndexed: 2,
		})}
		res, err := newClient(t, fake).Resolve(t.Context(), fusion.ResolveQuery{Query: "retry -race", Mode: fusion.ResolveModeNL, Limit: 10})
		require.NoError(t, err)
		assert.Equal(t, 2, res.BodiesNotIndexed)
		assert.Equal(t, []string{"race"}, res.Excluded)
	})

	for name, tc := range map[string]struct {
		mode  fusion.ResolveMode
		query string
		reply string
	}{
		"symbol": {fusion.ResolveModeSymbol, "Widget",
			`{"producer":"graph-index","data":{"matches":[{"entity_id":"a.b.c.d.e.1","matched_name":"Widget"}],` +
				`"bodies_not_indexed":2,"excluded":["race"]}}`},
		"prefix": {fusion.ResolveModePrefix, "a.b.c",
			`{"ids":["a.b.c.d.e.1"],"total":1,"bodies_not_indexed":2,"excluded":["race"]}`},
	} {
		t.Run(name, func(t *testing.T) {
			fake := &fakeRequester{resp: []byte(tc.reply)}
			res, err := newClient(t, fake).Resolve(t.Context(), fusion.ResolveQuery{Query: tc.query, Mode: tc.mode, Limit: 10})
			require.NoError(t, err)
			require.Equal(t, []string{"a.b.c.d.e.1"}, fusion.SeedIDs(res.Seeds), "the reply decoded")
			assert.Zero(t, res.BodiesNotIndexed)
			assert.Nil(t, res.Excluded)
		})
	}
}

// nlLens resolves every query as natural language and walks no edges, so a
// Fuse call reads only the word search and the seeds.
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

func newNLEngine(t *testing.T, fake *fakeRequester) *fusion.Engine {
	t.Helper()
	eng, err := fusion.NewEngine(newClient(t, fake), fusion.NewBodyResolver(fusion.MapStoreResolver{}), slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	return eng
}

// TestNLWordIndexRefusalReturned holds that the word index's refusal of a
// question reaches the Fuse caller as it was sent: class, code, detail and
// message, with no answer beside it.
func TestNLWordIndexRefusalReturned(t *testing.T) {
	detail := map[string]any{"term": "name:retry"}
	sent := errs.ClassifiedCodeDetail(errs.ErrorInvalid, graph.ErrorCodeInvalidRequest, detail,
		errors.New(`invalid request: "name:" is reserved; quote the term to search for it`))
	fake := &fakeRequester{err: sent}

	resp, err := newNLEngine(t, fake).Fuse(t.Context(), fusion.Request{Query: "name:retry"}, nlLens{})
	require.Error(t, err)
	assert.Equal(t, subjectWordSearch, fake.lastSubject)
	var refusal *errs.ClassifiedError
	require.ErrorAs(t, err, &refusal)
	assert.Equal(t, errs.ErrorInvalid, refusal.Class)
	assert.Equal(t, graph.ErrorCodeInvalidRequest, refusal.Code)
	assert.Equal(t, detail, refusal.Detail)
	assert.Contains(t, err.Error(), "quote the term")
	assert.Equal(t, fusion.Response{}, resp, "a refused question has no answer")
}

// TestNLIndexNotReadyDefers holds that a word index not yet ready defers the
// answer, as for every other read, rather than failing it or answering a miss.
func TestNLIndexNotReadyDefers(t *testing.T) {
	fake := &fakeRequester{err: errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeIndexNotReady,
		errors.New("index not ready"))}

	resp, err := newNLEngine(t, fake).Fuse(t.Context(), fusion.Request{Query: "widget"}, nlLens{})
	require.NoError(t, err)
	assert.Equal(t, subjectWordSearch, fake.lastSubject)
	assert.True(t, resp.Deferred)
	assert.Equal(t, graph.ErrorCodeIndexNotReady, resp.DeferReason)
	assert.Empty(t, resp.Nodes)
	assert.Empty(t, resp.Misses)
}

func TestResolve_Symbol_CarriesNoScope(t *testing.T) {
	fake := &fakeRequester{resp: enveloped(t, graph.NameData{})}
	c := newClient(t, fake)
	_, err := c.Resolve(t.Context(), fusion.ResolveQuery{Query: "Widget", Mode: fusion.ResolveModeSymbol, Scope: []string{"a.b.c"}, Limit: 10})
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, json.Unmarshal(fake.lastData, &body))
	assert.NotContains(t, body, "scope", "scope is for an NL resolve only")
}

func TestResolve_UnknownMode(t *testing.T) {
	fake := &fakeRequester{}
	c := newClient(t, fake)
	_, err := c.Resolve(t.Context(), fusion.ResolveQuery{Query: "x", Mode: fusion.ResolveMode("bogus"), Limit: 10})
	require.Error(t, err)
	assert.Zero(t, fake.calls)
}

func TestEntity_Found(t *testing.T) {
	id := "a.b.c.d.e.1"
	es := graph.EntityState{ID: id, Triples: []message.Triple{stamped(id, "dc.terms.title", "Widget")}}
	fake := &fakeRequester{resp: mustJSON(t, graph.ExactEntity{Entity: &es, KVRevision: 1})}
	c := newClient(t, fake)

	ent, err := c.Entity(t.Context(), id)
	require.NoError(t, err)
	assert.Equal(t, subjectEntity, fake.lastSubject)
	assert.JSONEq(t, `{"id":"a.b.c.d.e.1"}`, string(fake.lastData))
	require.NotNil(t, ent)
	assert.Equal(t, id, ent.ID)
	assert.Equal(t, "Widget", ent.First("dc.terms.title"))
}

func TestEntity_NotFoundIsAbsence(t *testing.T) {
	fake := &fakeRequester{err: errs.ClassifiedCode(errs.ErrorInvalid, graph.ErrorCodeEntityNotFound, errors.New("not found: x"))}
	ent, err := newClient(t, fake).Entity(t.Context(), "missing")
	require.NoError(t, err, "entity_not_found is (nil, nil)")
	assert.Nil(t, ent)
}

func TestEntity_BackendErrorPropagates(t *testing.T) {
	backend := errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeInternal, errors.New("boom"))
	_, err := newClient(t, &fakeRequester{err: backend}).Entity(t.Context(), "x")
	require.ErrorIs(t, err, backend, "a backend error is returned, never read as absence")
}

func TestEntities_BatchDecodes(t *testing.T) {
	fake := &fakeRequester{resp: mustJSON(t, graph.EntityBatchResponse{Entities: []graph.EntityState{
		{ID: "a.b.c.d.e.1"}, {ID: "a.b.c.d.e.2"},
	}})}
	c := newClient(t, fake)

	ents, err := c.Entities(t.Context(), []string{"a.b.c.d.e.1", "a.b.c.d.e.2"})
	require.NoError(t, err)
	assert.Equal(t, subjectBatch, fake.lastSubject)
	assert.JSONEq(t, `{"ids":["a.b.c.d.e.1","a.b.c.d.e.2"]}`, string(fake.lastData))
	assert.Len(t, ents.Entities, 2)
	assert.Empty(t, ents.Unhydrated, "a complete batch reports nothing unhydrated")
}

// TestEntities_BatchIgnoresUnknownKeys holds that a bare row is decoded as its
// type with unknown keys ignored, so an additive field keeps working.
func TestEntities_BatchIgnoresUnknownKeys(t *testing.T) {
	fake := &fakeRequester{resp: []byte(`{"entities":[{"id":"a.b.c.d.e.1"}],"added_later":true}`)}
	ents, err := newClient(t, fake).Entities(t.Context(), []string{"a.b.c.d.e.1"})
	require.NoError(t, err)
	require.Len(t, ents.Entities, 1)
	assert.Equal(t, "a.b.c.d.e.1", ents.Entities[0].ID)
}

func TestEntities_EmptyShortCircuits(t *testing.T) {
	fake := &fakeRequester{err: errors.New("should not be called")}
	ents, err := newClient(t, fake).Entities(t.Context(), nil)
	require.NoError(t, err)
	assert.Empty(t, ents.Entities)
	assert.Empty(t, ents.Unhydrated)
	assert.Zero(t, fake.calls, "zero IDs send no request")
}

func TestAuthoritativeRepliesRejectPoisonBeforeProjection(t *testing.T) {
	validID := "acme.ops.test.system.widget.001"
	invalidEntityID := "bad"
	tests := []struct {
		name string
		resp []byte
		call func(*Client) (any, error)
	}{
		{
			name: "prefix malformed listed id",
			resp: mustJSON(t, graph.PrefixIDsResponse{IDs: []string{validID, invalidEntityID}, Total: 2}),
			call: func(c *Client) (any, error) {
				return c.Resolve(t.Context(), fusion.ResolveQuery{Query: "acme.ops", Mode: fusion.ResolveModePrefix, Limit: 10})
			},
		},
		{
			name: "entity malformed subject",
			resp: mustJSON(t, graph.ExactEntity{
				Entity:     &graph.EntityState{ID: validID, Triples: []message.Triple{stamped(invalidEntityID, "test.state.value", "x")}},
				KVRevision: 1,
			}),
			call: func(c *Client) (any, error) { return c.Entity(t.Context(), validID) },
		},
		{
			name: "batch malformed reference",
			resp: mustJSON(t, graph.EntityBatchResponse{Entities: []graph.EntityState{
				{ID: validID},
				{ID: validID, Triples: []message.Triple{{
					Subject: validID, Predicate: "test.state.target", Object: invalidEntityID,
					Datatype: message.EntityReferenceDatatype,
				}}},
			}}),
			call: func(c *Client) (any, error) { return c.Entities(t.Context(), []string{validID}) },
		},
		{
			name: "word search malformed entity id",
			resp: enveloped(t, graph.LexicalSearchData{Results: []graph.LexicalHit{{EntityID: validID}, {EntityID: invalidEntityID}}}),
			call: func(c *Client) (any, error) {
				return c.Resolve(t.Context(), fusion.ResolveQuery{Query: "widget", Mode: fusion.ResolveModeNL, Limit: 10})
			},
		},
		{
			name: "relationship malformed endpoint",
			resp: enveloped(t, []graph.RelationshipEdge{{FromEntityID: validID, ToEntityID: invalidEntityID, Predicate: "test.state.target"}}),
			call: func(c *Client) (any, error) { return c.Neighbors(t.Context(), validID, nil, fusion.Outgoing) },
		},
		{
			name: "name match malformed entity id",
			resp: enveloped(t, graph.NameData{Matches: []graph.NameMatch{{EntityID: invalidEntityID, MatchedName: "Widget"}}}),
			call: func(c *Client) (any, error) { return c.Names(t.Context(), "Widget", 10) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.call(newClient(t, &fakeRequester{resp: tt.resp}))
			require.Error(t, err)
			assert.True(t, graph.IsStateContractError(err))
			assertNoProjection(t, got)
			var classified *errs.ClassifiedError
			require.ErrorAs(t, err, &classified)
			assert.Equal(t, errs.ErrorFatal, classified.Class)
			assert.Equal(t, graph.ErrorCodeGraphStateResetRequired, classified.Code)
		})
	}
}

// assertNoProjection checks that a refused reply yielded nothing usable. A
// Hydration or Resolution is a struct whose zero value is the empty result, so
// those are checked for empty lists rather than nil.
func assertNoProjection(t *testing.T, got any) {
	t.Helper()
	switch v := got.(type) {
	case fusion.Hydration:
		assert.Empty(t, v.Entities, "no partial projection may escape")
		assert.Empty(t, v.Unhydrated, "a refused reply asserts nothing about what it could not hydrate")
	case fusion.Resolution:
		assert.Equal(t, fusion.Resolution{}, v, "no partial projection may escape")
	default:
		assert.Nil(t, got, "no partial projection may escape")
	}
}

func TestNeighbors_OutgoingFiltersPredicates(t *testing.T) {
	fake := &fakeRequester{resp: enveloped(t, []graph.RelationshipEdge{
		{FromEntityID: "a.b.c.d.e.seed", ToEntityID: "a.b.c.d.e.callee1", Predicate: "code.calls.fn"},
		{FromEntityID: "a.b.c.d.e.seed", ToEntityID: "a.b.c.d.e.other", Predicate: "code.imports.pkg"},
		{FromEntityID: "a.b.c.d.e.seed", ToEntityID: "a.b.c.d.e.callee2", Predicate: "code.calls.fn"},
	})}
	c := newClient(t, fake)

	edges, err := c.Neighbors(t.Context(), "a.b.c.d.e.seed", []string{"code.calls.fn"}, fusion.Outgoing)
	require.NoError(t, err)
	assert.Equal(t, subjectRelationships, fake.lastSubject)
	assert.JSONEq(t, `{"entity_id":"a.b.c.d.e.seed","direction":"outgoing"}`, string(fake.lastData))
	assert.Equal(t, []fusion.Edge{
		{Predicate: "code.calls.fn", Target: "a.b.c.d.e.callee1"},
		{Predicate: "code.calls.fn", Target: "a.b.c.d.e.callee2"},
	}, edges, "code.imports.pkg is filtered out; the outgoing target is to_entity_id")
}

func TestNeighbors_IncomingTargetsFromEnd(t *testing.T) {
	fake := &fakeRequester{resp: enveloped(t, []graph.RelationshipEdge{
		{FromEntityID: "a.b.c.d.e.caller1", ToEntityID: "a.b.c.d.e.seed", Predicate: "code.calls.fn"},
	})}
	c := newClient(t, fake)

	edges, err := c.Neighbors(t.Context(), "a.b.c.d.e.seed", []string{"code.calls.fn"}, fusion.Incoming)
	require.NoError(t, err)
	assert.JSONEq(t, `{"entity_id":"a.b.c.d.e.seed","direction":"incoming"}`, string(fake.lastData))
	assert.Equal(t, []fusion.Edge{{Predicate: "code.calls.fn", Target: "a.b.c.d.e.caller1"}}, edges,
		"the incoming target is from_entity_id")
}

func TestNeighbors_NoPredicatesMeansNoFilter(t *testing.T) {
	fake := &fakeRequester{resp: enveloped(t, []graph.RelationshipEdge{
		{FromEntityID: "a.b.c.d.e.seed", ToEntityID: "a.b.c.d.e.x", Predicate: "a.b.c"},
		{FromEntityID: "a.b.c.d.e.seed", ToEntityID: "a.b.c.d.e.y", Predicate: "d.e.f"},
	})}
	edges, err := newClient(t, fake).Neighbors(t.Context(), "a.b.c.d.e.seed", nil, fusion.Outgoing)
	require.NoError(t, err)
	assert.Len(t, edges, 2)
}

// TestNeighborsReadsPredicate holds that an edge's predicate is read from the
// wire key predicate, never edge_type: a reply with only edge_type has edges with
// no predicate, which a filter keeps none of.
func TestNeighborsReadsPredicate(t *testing.T) {
	reply := enveloped(t, []map[string]string{{
		"from_entity_id": "a.b.c.d.e.seed", "to_entity_id": "a.b.c.d.e.x", "predicate": "code.calls.fn",
	}})
	edges, err := newClient(t, &fakeRequester{resp: reply}).Neighbors(t.Context(), "a.b.c.d.e.seed", nil, fusion.Outgoing)
	require.NoError(t, err)
	assert.Equal(t, []fusion.Edge{{Predicate: "code.calls.fn", Target: "a.b.c.d.e.x"}}, edges)

	pinWire := enveloped(t, []map[string]string{{
		"from_entity_id": "a.b.c.d.e.seed", "to_entity_id": "a.b.c.d.e.x", "edge_type": "code.calls.fn",
	}})
	edges, err = newClient(t, &fakeRequester{resp: pinWire}).Neighbors(t.Context(), "a.b.c.d.e.seed", []string{"code.calls.fn"}, fusion.Outgoing)
	require.NoError(t, err)
	assert.Empty(t, edges, "edge_type is not the tree's key")
}

func TestNames_DedupesAndCaps(t *testing.T) {
	fake := &fakeRequester{resp: enveloped(t, graph.NameData{Matches: []graph.NameMatch{
		{EntityID: "a.b.c.d.e.1", MatchedName: "Widget"},
		{EntityID: "a.b.c.d.e.2", MatchedName: "Widget"},
		{EntityID: "a.b.c.d.e.3", MatchedName: "Gadget"},
		{EntityID: "a.b.c.d.e.4", MatchedName: "Gizmo"},
	}})}
	c := newClient(t, fake)

	names, err := c.Names(t.Context(), "Wid", 2)
	require.NoError(t, err)
	assert.Equal(t, subjectByName, fake.lastSubject)
	assert.Equal(t, []string{"Widget", "Gadget"}, names, "deduplicated in rank order, capped at 2")
}

// TestEntitiesRefusesNilContext holds that a nil context is refused before the
// empty-batch shortcut, with no request; a valid empty batch still sends none
// (TestEntities_EmptyShortCircuits).
func TestEntitiesRefusesNilContext(t *testing.T) {
	fake := &fakeRequester{err: errors.New("must not be called")}
	// A nil context is the input under test.
	_, err := newClient(t, fake).Entities(nil, nil)
	require.Error(t, err)
	assert.Zero(t, fake.calls)
}
