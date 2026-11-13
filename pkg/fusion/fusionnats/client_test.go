package fusionnats

import (
	"context"
	"encoding/json"
	"errors"
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

// The subjects of the graph-query rows the client uses, spelled here as an
// oracle independent of the verb table the client reads them from.
const (
	subjectEntity        = "graph.query.entity"
	subjectBatch         = "graph.query.batch"
	subjectPrefix        = "graph.query.prefix"
	subjectByName        = "graph.query.byName"
	subjectRelationships = "graph.query.relationships"
	subjectSemantic      = "graph.query.semantic"
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
	fake := &fakeRequester{resp: mustJSON(t, graph.PrefixQueryResponse{})}
	c, err := New(fake, 0)
	require.NoError(t, err)
	_, err = c.Resolve(t.Context(), fusion.ResolveQuery{Mode: fusion.ResolveModePrefix, Limit: 1})
	require.NoError(t, err)
	assert.Equal(t, 1, fake.calls)
	assert.Equal(t, time.Duration(0), fake.lastTimeout)

	fake = &fakeRequester{resp: mustJSON(t, graph.PrefixQueryResponse{})}
	c, err = New(fake, 3*time.Second)
	require.NoError(t, err)
	_, err = c.Resolve(t.Context(), fusion.ResolveQuery{Mode: fusion.ResolveModePrefix, Limit: 1})
	require.NoError(t, err)
	assert.Equal(t, 3*time.Second, fake.lastTimeout, "a set timeout reaches the requester unchanged")
}

// TestSubjectsComeFromVerbTable drives every RetrievalClient method and checks
// each request went to its graph-query row's subject.
func TestSubjectsComeFromVerbTable(t *testing.T) {
	id := "acme.ops.robotics.gcs.drone.001"
	replies := map[string][]byte{
		subjectEntity:        mustJSON(t, graph.ExactEntity{Entity: &graph.EntityState{ID: id}, KVRevision: 1}),
		subjectBatch:         mustJSON(t, graph.EntityBatchResponse{}),
		subjectPrefix:        mustJSON(t, graph.PrefixQueryResponse{}),
		subjectByName:        enveloped(t, graph.NameData{}),
		subjectRelationships: enveloped(t, []graph.RelationshipEdge{}),
		subjectSemantic:      enveloped(t, graph.SemanticData{}),
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
		{"Resolve prefix", subjectPrefix, func() error {
			_, err := c.Resolve(ctx, fusion.ResolveQuery{Query: "acme.ops", Mode: fusion.ResolveModePrefix})
			return err
		}},
		{"Resolve nl", subjectSemantic, func() error {
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

func TestVerbLookupNamesMissingRow(t *testing.T) {
	var table []graph.QueryVerb
	for _, verb := range graph.QueryVerbs() {
		if verb.Responder == graph.ResponderGraphQuery && verb.Name == "batch" {
			continue
		}
		table = append(table, verb)
	}
	_, err := lookupVerbs(table)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"batch"`)

	_, err = lookupVerbs(graph.QueryVerbs())
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
		{"semantic", mustJSON(t, graph.SemanticData{Results: []graph.SimilarityHit{{EntityID: id, Similarity: 0.9}}}),
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

func TestResolve_Prefix(t *testing.T) {
	fake := &fakeRequester{resp: mustJSON(t, graph.PrefixQueryResponse{Entities: []graph.EntityState{
		{ID: "a.b.c.d.e.1"}, {ID: "a.b.c.d.e.2"},
	}})}
	c := newClient(t, fake)

	res, err := c.Resolve(t.Context(), fusion.ResolveQuery{Query: "a.b.c", Mode: fusion.ResolveModePrefix, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, subjectPrefix, fake.lastSubject)
	assert.Equal(t, []string{"a.b.c.d.e.1", "a.b.c.d.e.2"}, fusion.SeedIDs(res.Seeds))
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
	fake := &fakeRequester{resp: mustJSON(t, graph.PrefixQueryResponse{})}
	c := newClient(t, fake)
	_, err := c.Resolve(t.Context(), fusion.ResolveQuery{Mode: fusion.ResolveModePrefix, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, subjectPrefix, fake.lastSubject)
}

func TestResolve_Semantic(t *testing.T) {
	fake := &fakeRequester{resp: enveloped(t, graph.SemanticData{Results: []graph.SimilarityHit{
		{EntityID: "a.b.c.d.e.1", Similarity: 0.9},
		{EntityID: "a.b.c.d.e.2", Similarity: 0.8},
	}})}
	c := newClient(t, fake)

	res, err := c.Resolve(t.Context(), fusion.ResolveQuery{Query: "find me a widget", Mode: fusion.ResolveModeNL, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, subjectSemantic, fake.lastSubject)
	assert.Equal(t, []fusion.Seed{
		{ID: "a.b.c.d.e.1", Similarity: 0.9, HasSimilarity: true},
		{ID: "a.b.c.d.e.2", Similarity: 0.8, HasSimilarity: true},
	}, res.Seeds)
}

// TestResolve_Semantic_UnscopedByteParity holds ADR-071's rule that an unscoped
// request carries no scope key.
func TestResolve_Semantic_UnscopedByteParity(t *testing.T) {
	fake := &fakeRequester{resp: enveloped(t, graph.SemanticData{})}
	c := newClient(t, fake)
	_, err := c.Resolve(t.Context(), fusion.ResolveQuery{Query: "find a widget", Mode: fusion.ResolveModeNL, Limit: 10})
	require.NoError(t, err)
	assert.JSONEq(t, `{"query":"find a widget","limit":10}`, string(fake.lastData))
}

func TestResolve_Semantic_ScopeInBody(t *testing.T) {
	fake := &fakeRequester{resp: enveloped(t, graph.SemanticData{})}
	c := newClient(t, fake)
	scope := []string{"c360.semspec.source.doc"}
	_, err := c.Resolve(t.Context(), fusion.ResolveQuery{Query: "find a widget", Mode: fusion.ResolveModeNL, Scope: scope, Limit: 10})
	require.NoError(t, err)
	assert.JSONEq(t, `{"query":"find a widget","limit":10,"scope":["c360.semspec.source.doc"]}`, string(fake.lastData))
}

func TestResolve_SemanticInvalidScopeFailsBeforeNATS(t *testing.T) {
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

func TestResolve_SemanticEmptyScopeEntryRemainsMatchAll(t *testing.T) {
	fake := &fakeRequester{resp: enveloped(t, graph.SemanticData{})}
	c := newClient(t, fake)
	_, err := c.Resolve(t.Context(), fusion.ResolveQuery{
		Query: "find a widget", Mode: fusion.ResolveModeNL, Scope: []string{""}, Limit: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, subjectSemantic, fake.lastSubject)
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

// TestSemanticCountsReachResolution holds that an NL resolve passes on the
// search reply's counts of records it could not read, and that symbol and
// prefix resolves, whose replies carry no such counts, report 0.
func TestSemanticCountsReachResolution(t *testing.T) {
	fake := &fakeRequester{resp: enveloped(t, graph.SemanticData{
		Results:           []graph.SimilarityHit{{EntityID: "a.b.c.d.e.1", Similarity: 0.5}},
		UnreadableRecords: 2,
		FailedRecords:     1,
	})}
	res, err := newClient(t, fake).Resolve(t.Context(), fusion.ResolveQuery{Query: "widget", Mode: fusion.ResolveModeNL})
	require.NoError(t, err)
	assert.Equal(t, 2, res.UnreadableRecords)
	assert.Equal(t, 1, res.FailedRecords)

	symbol := &fakeRequester{resp: enveloped(t, graph.NameData{Matches: []graph.NameMatch{{EntityID: "a.b.c.d.e.1"}}})}
	res, err = newClient(t, symbol).Resolve(t.Context(), fusion.ResolveQuery{Query: "Widget", Mode: fusion.ResolveModeSymbol})
	require.NoError(t, err)
	assert.Equal(t, fusion.Resolution{Seeds: []fusion.Seed{{ID: "a.b.c.d.e.1"}}}, res)

	prefix := &fakeRequester{resp: mustJSON(t, graph.PrefixQueryResponse{Entities: []graph.EntityState{{ID: "a.b.c.d.e.1"}}})}
	res, err = newClient(t, prefix).Resolve(t.Context(), fusion.ResolveQuery{Query: "a.b", Mode: fusion.ResolveModePrefix})
	require.NoError(t, err)
	assert.Equal(t, fusion.Resolution{Seeds: []fusion.Seed{{ID: "a.b.c.d.e.1"}}}, res)
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
			name: "prefix malformed root",
			resp: mustJSON(t, graph.PrefixQueryResponse{Entities: []graph.EntityState{{ID: validID}, {ID: invalidEntityID}}}),
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
			name: "semantic malformed entity id",
			resp: enveloped(t, graph.SemanticData{Results: []graph.SimilarityHit{{EntityID: validID}, {EntityID: invalidEntityID}}}),
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
