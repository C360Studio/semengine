package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/internal/harness/semantictest"
	"github.com/c360studio/semengine/message"
)

// fixedTime is a stable timestamp for triple fixtures.
var fixedTime = time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)

// withFixedTime supplies only the stable timestamp; every semantic position
// remains explicit at the fixture call site.
func withFixedTime(triple message.Triple) message.Triple {
	triple.Timestamp = fixedTime
	return triple
}

type failingGraphTripleReader struct {
	kvcatalog.CatalogReader
	err      error
	deadline time.Time
}

func (r *failingGraphTripleReader) Keys(ctx context.Context, _ ...jetstream.WatchOpt) ([]string, error) {
	r.deadline, _ = ctx.Deadline()
	return nil, r.err
}

func (r *failingGraphTripleReader) Get(ctx context.Context, _ string) (jetstream.KeyValueEntry, error) {
	r.deadline, _ = ctx.Deadline()
	return nil, r.err
}

func TestGraphTripleKeysPreservesCallerPolicy(t *testing.T) {
	sentinel := errors.New("list failed")
	reader := &failingGraphTripleReader{err: sentinel}
	started := time.Now()

	_, err := graphTripleKeys(t.Context(), reader)

	require.ErrorIs(t, err, sentinel)
	require.ErrorContains(t, err, "kv keys")
	require.WithinDuration(t, started.Add(5*time.Second), reader.deadline, time.Second)
}

func TestGraphTripleEntryPreservesCallerPolicy(t *testing.T) {
	sentinel := errors.New("read failed")
	reader := &failingGraphTripleReader{err: sentinel}
	started := time.Now()

	_, err := graphTripleEntry(t.Context(), reader, "entity-id")

	require.ErrorIs(t, err, sentinel)
	require.ErrorContains(t, err, "kv get entity-id")
	require.WithinDuration(t, started.Add(5*time.Second), reader.deadline, time.Second)
}

// ---- parseTripleQueryParams ----

func TestParseTripleQueryParams_Defaults(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/graph/triples", nil)
	p, errMsg := parseTripleQueryParams(r)

	require.Empty(t, errMsg)
	assert.Equal(t, "", p.subject)
	assert.Equal(t, "", p.predicate)
	assert.Equal(t, "", p.object)
	assert.Equal(t, graphTriplesDefaultLimit, p.limit)
}

func TestParseTripleQueryParams_AllParams(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/graph/triples?subject=acme.ops.test.service.entity.1&predicate=test.state.value&object=o&limit=50", nil)
	p, errMsg := parseTripleQueryParams(r)

	require.Empty(t, errMsg)
	assert.Equal(t, "acme.ops.test.service.entity.1", p.subject)
	assert.Equal(t, "test.state.value", p.predicate)
	assert.Equal(t, "o", p.object)
	assert.Equal(t, 50, p.limit)
}

func TestParseTripleQueryParams_LimitClamped(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/graph/triples?limit=9999", nil)
	p, errMsg := parseTripleQueryParams(r)

	require.Empty(t, errMsg)
	assert.Equal(t, graphTriplesMaxLimit, p.limit)
}

func TestParseTripleQueryParams_LimitZeroUsesDefault(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/graph/triples?limit=0", nil)
	p, errMsg := parseTripleQueryParams(r)

	require.Empty(t, errMsg)
	assert.Equal(t, graphTriplesDefaultLimit, p.limit)
}

func TestParseTripleQueryParams_LimitNegative(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/graph/triples?limit=-1", nil)
	_, errMsg := parseTripleQueryParams(r)

	assert.NotEmpty(t, errMsg, "negative limit should be rejected")
}

func TestParseTripleQueryParams_LimitNonInteger(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/graph/triples?limit=abc", nil)
	_, errMsg := parseTripleQueryParams(r)

	assert.NotEmpty(t, errMsg, "non-integer limit should be rejected")
}

func TestParseTripleQueryParams_LimitOne(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/graph/triples?limit=1", nil)
	p, errMsg := parseTripleQueryParams(r)

	require.Empty(t, errMsg)
	assert.Equal(t, 1, p.limit)
}

func TestParseTripleQueryParams_LimitMaxExact(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/graph/triples?limit=1000", nil)
	p, errMsg := parseTripleQueryParams(r)

	require.Empty(t, errMsg)
	assert.Equal(t, 1000, p.limit)
}

// ---- tripleMatchesQuery ----

func TestTripleMatchesQuery_NoFilters(t *testing.T) {
	tr := withFixedTime(message.Triple{Subject: semantictest.EntityID(t, "acme", "ops", "test", "service", "entity", "1"), Predicate: semantictest.Predicate(t, "test", "state", "value"), Object: "val", Source: "src", Confidence: 1.0})
	p := tripleQueryParams{limit: 100}
	assert.True(t, tripleMatchesQuery(tr, p))
}

func TestTripleMatchesQuery_SubjectMatch(t *testing.T) {
	tr := withFixedTime(message.Triple{Subject: semantictest.EntityID(t, "acme", "ops", "test", "service", "entity", "1"), Predicate: semantictest.Predicate(t, "test", "state", "value"), Object: "val", Source: "src", Confidence: 1.0})
	p := tripleQueryParams{subject: "acme.ops.test.service.entity.1", limit: 100}
	assert.True(t, tripleMatchesQuery(tr, p))
}

func TestTripleMatchesQuery_SubjectNoMatch(t *testing.T) {
	tr := withFixedTime(message.Triple{Subject: semantictest.EntityID(t, "acme", "ops", "test", "service", "entity", "1"), Predicate: semantictest.Predicate(t, "test", "state", "value"), Object: "val", Source: "src", Confidence: 1.0})
	p := tripleQueryParams{subject: "acme.ops.test.service.entity.2", limit: 100}
	assert.False(t, tripleMatchesQuery(tr, p))
}

func TestTripleMatchesQuery_PredicateMatch(t *testing.T) {
	tr := withFixedTime(message.Triple{Subject: semantictest.EntityID(t, "acme", "ops", "test", "service", "entity", "1"), Predicate: semantictest.Predicate(t, "ops", "diagnosis", "finding"), Object: "found something", Source: "ops-emit-diagnosis", Confidence: 0.9})
	p := tripleQueryParams{predicate: "ops.diagnosis.finding", limit: 100}
	assert.True(t, tripleMatchesQuery(tr, p))
}

func TestTripleMatchesQuery_PredicateNoMatch(t *testing.T) {
	tr := withFixedTime(message.Triple{Subject: semantictest.EntityID(t, "acme", "ops", "test", "service", "entity", "1"), Predicate: semantictest.Predicate(t, "ops", "diagnosis", "finding"), Object: "found something", Source: "ops-emit-diagnosis", Confidence: 0.9})
	p := tripleQueryParams{predicate: "ops.diagnosis.recommendation", limit: 100}
	assert.False(t, tripleMatchesQuery(tr, p))
}

func TestTripleMatchesQuery_ObjectStringMatch(t *testing.T) {
	tr := withFixedTime(message.Triple{Subject: semantictest.EntityID(t, "acme", "ops", "test", "service", "entity", "1"), Predicate: semantictest.Predicate(t, "test", "state", "value"), Object: "hello world", Source: "src", Confidence: 1.0})
	p := tripleQueryParams{object: "hello world", limit: 100}
	assert.True(t, tripleMatchesQuery(tr, p))
}

func TestTripleMatchesQuery_ObjectStringNoMatch(t *testing.T) {
	tr := withFixedTime(message.Triple{Subject: semantictest.EntityID(t, "acme", "ops", "test", "service", "entity", "1"), Predicate: semantictest.Predicate(t, "test", "state", "value"), Object: "hello world", Source: "src", Confidence: 1.0})
	p := tripleQueryParams{object: "goodbye", limit: 100}
	assert.False(t, tripleMatchesQuery(tr, p))
}

func TestTripleMatchesQuery_SubjectAndPredicateBothMatch(t *testing.T) {
	tr := withFixedTime(message.Triple{Subject: semantictest.EntityID(t, "acme", "ops", "test", "service", "entity", "1"), Predicate: semantictest.Predicate(t, "test", "state", "value"), Object: "val", Source: "src", Confidence: 1.0})
	p := tripleQueryParams{subject: "acme.ops.test.service.entity.1", predicate: "test.state.value", limit: 100}
	assert.True(t, tripleMatchesQuery(tr, p))
}

func TestTripleMatchesQuery_SubjectMatchPredicateNoMatch(t *testing.T) {
	tr := withFixedTime(message.Triple{Subject: semantictest.EntityID(t, "acme", "ops", "test", "service", "entity", "1"), Predicate: semantictest.Predicate(t, "test", "state", "value"), Object: "val", Source: "src", Confidence: 1.0})
	p := tripleQueryParams{subject: "acme.ops.test.service.entity.1", predicate: "test.state.other", limit: 100}
	assert.False(t, tripleMatchesQuery(tr, p))
}

// ---- serveGraphTriples HTTP handler ----

// stubQuerier builds a querier function that returns a fixed result or error.
func stubQuerier(results []message.Triple, err error) func(context.Context, tripleQueryParams) ([]message.Triple, error) {
	return func(_ context.Context, _ tripleQueryParams) ([]message.Triple, error) {
		return results, err
	}
}

// captureQuerier builds a querier that captures the params it was called with
// and returns the provided results.
func captureQuerier(results []message.Triple) (func(context.Context, tripleQueryParams) ([]message.Triple, error), *tripleQueryParams) {
	var captured tripleQueryParams
	fn := func(_ context.Context, p tripleQueryParams) ([]message.Triple, error) {
		captured = p
		return results, nil
	}
	return fn, &captured
}

func TestServeGraphTriples_MethodNotAllowed(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		t.Run(method, func(t *testing.T) {
			r := httptest.NewRequest(method, "/graph/triples", nil)
			w := httptest.NewRecorder()

			serveGraphTriples(w, r, stubQuerier(nil, nil), nil)

			assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
		})
	}
}

func TestServeGraphTriples_MalformedLimit(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/graph/triples?limit=bad", nil)
	w := httptest.NewRecorder()

	serveGraphTriples(w, r, stubQuerier(nil, nil), nil)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestServeGraphTriples_NegativeLimit(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/graph/triples?limit=-5", nil)
	w := httptest.NewRecorder()

	serveGraphTriples(w, r, stubQuerier(nil, nil), nil)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestServeGraphTriples_EmptyResultIsArray(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/graph/triples", nil)
	w := httptest.NewRecorder()

	serveGraphTriples(w, r, stubQuerier([]message.Triple{}, nil), nil)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	// Must decode as a JSON array, never null.
	var decoded []message.Triple
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &decoded))
	assert.NotNil(t, decoded, "empty result must be [] not null")
	assert.Len(t, decoded, 0)
}

func TestServeGraphTriples_NonEmptyResult(t *testing.T) {
	ts := fixedTime
	triples := []message.Triple{
		{
			Subject:    "acme.ops.demo.sys.entity.001",
			Predicate:  semantictest.Predicate(t, "ops", "diagnosis", "finding"),
			Object:     "high error rate",
			Source:     "ops-emit-diagnosis",
			Confidence: 0.9,
			Timestamp:  ts,
		},
		{
			Subject:    "acme.ops.demo.sys.entity.002",
			Predicate:  semantictest.Predicate(t, "ops", "diagnosis", "recommendation"),
			Object:     "restart component",
			Source:     "ops-emit-diagnosis",
			Confidence: 0.8,
			Timestamp:  ts,
		},
	}

	r := httptest.NewRequest(http.MethodGet, "/graph/triples?predicate=ops.diagnosis.finding", nil)
	w := httptest.NewRecorder()

	serveGraphTriples(w, r, stubQuerier(triples, nil), nil)

	require.Equal(t, http.StatusOK, w.Code)

	var decoded []message.Triple
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &decoded))
	require.Len(t, decoded, 2)
	assert.Equal(t, "ops.diagnosis.finding", decoded[0].Predicate)
	assert.Equal(t, 0.9, decoded[0].Confidence)
	assert.Equal(t, "ops-emit-diagnosis", decoded[0].Source)
	assert.Equal(t, ts.UTC(), decoded[0].Timestamp.UTC())
}

func TestServeGraphTriples_BackendError(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/graph/triples", nil)
	w := httptest.NewRecorder()

	serveGraphTriples(w, r, stubQuerier(nil, errors.New("nats connection refused")), nil)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	// Body should be a terse message, not a stack trace.
	body := w.Body.String()
	assert.NotEmpty(t, body)
	assert.NotContains(t, body, "goroutine")
}

func TestServeGraphTriples_ParamsForwardedToQuerier(t *testing.T) {
	querier, captured := captureQuerier([]message.Triple{})

	r := httptest.NewRequest(http.MethodGet, "/graph/triples?subject=acme.ops.test.service.entity.1&predicate=test.state.value&object=obj&limit=42", nil)
	w := httptest.NewRecorder()

	serveGraphTriples(w, r, querier, nil)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "acme.ops.test.service.entity.1", captured.subject)
	assert.Equal(t, "test.state.value", captured.predicate)
	assert.Equal(t, "obj", captured.object)
	assert.Equal(t, 42, captured.limit)
}

func TestServeGraphTriples_LimitClamped(t *testing.T) {
	querier, captured := captureQuerier([]message.Triple{})

	r := httptest.NewRequest(http.MethodGet, "/graph/triples?limit=50000", nil)
	w := httptest.NewRecorder()

	serveGraphTriples(w, r, querier, nil)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, graphTriplesMaxLimit, captured.limit, "limit must be clamped to max")
}

func TestServeGraphTriples_ContentTypeJSON(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/graph/triples", nil)
	w := httptest.NewRecorder()

	serveGraphTriples(w, r, stubQuerier([]message.Triple{}, nil), nil)

	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
}

// ---- message.Triple wire-shape round-trip ----

// TestServeGraphTriples_ContextAndDatatypeRoundTrip verifies that Context and
// Datatype — fields that TripleResponse formerly dropped — now survive the full
// HTTP encode/decode cycle.
func TestServeGraphTriples_ContextAndDatatypeRoundTrip(t *testing.T) {
	ts := fixedTime
	triple := message.Triple{
		Subject:    "acme.ops.demo.sys.entity.001",
		Predicate:  semantictest.Predicate(t, "ops", "diagnosis", "finding"),
		Object:     "high error rate",
		Source:     "ops-emit-diagnosis",
		Confidence: 0.9,
		Timestamp:  ts,
		Context:    "batch-abc-123",
		Datatype:   "xsd:string",
	}

	r := httptest.NewRequest(http.MethodGet, "/graph/triples", nil)
	w := httptest.NewRecorder()

	serveGraphTriples(w, r, stubQuerier([]message.Triple{triple}, nil), nil)

	require.Equal(t, http.StatusOK, w.Code)

	var decoded []message.Triple
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &decoded))
	require.Len(t, decoded, 1)
	assert.Equal(t, "batch-abc-123", decoded[0].Context, "Context field must round-trip")
	assert.Equal(t, "xsd:string", decoded[0].Datatype, "Datatype field must round-trip")
}

// TestServeGraphTriples_SourcePreservedWhenEmpty verifies that an empty Source is
// preserved in JSON (message.Triple uses source without omitempty), not silently
// dropped. The wire JSON must contain "source":"" for consumers relying on
// the authoritative shape.
func TestServeGraphTriples_SourcePreservedWhenEmpty(t *testing.T) {
	triple := message.Triple{
		Subject:    "acme.ops.demo.sys.entity.001",
		Predicate:  semantictest.Predicate(t, "ops", "test", "predicate"),
		Object:     "value",
		Source:     "", // explicitly empty
		Confidence: 1.0,
		Timestamp:  fixedTime,
	}

	r := httptest.NewRequest(http.MethodGet, "/graph/triples", nil)
	w := httptest.NewRecorder()

	serveGraphTriples(w, r, stubQuerier([]message.Triple{triple}, nil), nil)

	require.Equal(t, http.StatusOK, w.Code)

	// Inspect raw JSON map to confirm "source" key is present with empty value.
	var raw []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	require.Len(t, raw, 1)
	val, hasSource := raw[0]["source"]
	assert.True(t, hasSource, "source key must be present even when empty (no omitempty on message.Triple.Source)")
	assert.Equal(t, "", val, "source value must be empty string, not omitted")
}

// ---- D6 (Q-A): validated filters, a sorted scan, and read errors ----

// TestGraphTriplesRefusesMalformedSubjectAndPredicate: a subject that is not a literal entity ID
// and a predicate that is not three parts are refused with 400 naming the parameter, before any
// read (design D6, Q-A).
//
// spec: operator-surface / /graph/triples answers a sorted, validated read of ENTITY_STATES
func TestGraphTriplesRefusesMalformedSubjectAndPredicate(t *testing.T) {
	for _, tt := range []struct{ query, param string }{
		{"subject=a.b", "subject"},
		{"predicate=x", "predicate"},
		{"predicate=a.b.c.d", "predicate"},
	} {
		t.Run(tt.query, func(t *testing.T) {
			called := false
			querier := func(context.Context, tripleQueryParams) ([]message.Triple, error) {
				called = true
				return []message.Triple{}, nil
			}
			r := httptest.NewRequest(http.MethodGet, "/graph/triples?"+tt.query, nil)
			w := httptest.NewRecorder()

			serveGraphTriples(w, r, querier, nil)

			require.Equal(t, http.StatusBadRequest, w.Code)
			require.Contains(t, w.Body.String(), tt.param, "the 400 does not name the parameter")
			require.False(t, called, "a refused request reached the query")
		})
	}
}

// stubEntityReader is a CatalogReader over an in-memory ENTITY_STATES: Keys lists keys in the
// order given, which need not be sorted, and Get answers the error set for a key, else its stored
// bytes, else jetstream.ErrKeyNotFound. Any other method panics through the nil embedded reader.
type stubEntityReader struct {
	kvcatalog.CatalogReader
	keys   []string
	values map[string][]byte
	errs   map[string]error
}

func (r *stubEntityReader) Keys(context.Context, ...jetstream.WatchOpt) ([]string, error) {
	return slices.Clone(r.keys), nil
}

func (r *stubEntityReader) Get(_ context.Context, key string) (jetstream.KeyValueEntry, error) {
	if err := r.errs[key]; err != nil {
		return nil, err
	}
	value, ok := r.values[key]
	if !ok {
		return nil, jetstream.ErrKeyNotFound
	}
	return stubEntry{key: key, value: value}, nil
}

// stubEntry is one stored ENTITY_STATES value.
type stubEntry struct {
	key   string
	value []byte
}

func (e stubEntry) Bucket() string                  { return graph.BucketEntityStates }
func (e stubEntry) Key() string                     { return e.key }
func (e stubEntry) Value() []byte                   { return e.value }
func (e stubEntry) Revision() uint64                { return 1 }
func (e stubEntry) Created() time.Time              { return fixedTime }
func (e stubEntry) Delta() uint64                   { return 0 }
func (e stubEntry) Operation() jetstream.KeyValueOp { return jetstream.KeyValuePut }

// discardLogger is the logger the scan tests hand scanGraphTriples.
var discardLogger = slog.New(slog.DiscardHandler)

// storedEntity encodes an entity through the bucket's one writer-side codec.
func storedEntity(t testing.TB, id string, triples []message.Triple) []byte {
	t.Helper()
	data, err := graph.MarshalEntityState(&graph.EntityState{ID: id, Triples: triples})
	require.NoError(t, err)
	return data
}

// entityTriple is one triple on entity id, with the metadata the stored-value rule requires.
func entityTriple(id, predicate string, object any) message.Triple {
	return withFixedTime(message.Triple{Subject: id, Predicate: predicate, Object: object, Source: "src", Confidence: 1})
}

// tripleJSON renders triples as the endpoint does, so triples read back through the codec compare
// with the ones a test wrote.
func tripleJSON(t testing.TB, triples []message.Triple) string {
	t.Helper()
	data, err := json.Marshal(triples)
	require.NoError(t, err)
	return string(data)
}

// TestGraphTriplesSkipsOnlyADeletedKey: a key whose read reports not found (deleted between the
// listing and the read) is skipped; a read that fails any other way, here a timeout, and an entity
// the decoder refuses fail the whole read, which the endpoint answers with 500, never with the
// triples read so far (design D6, Q-A).
//
// spec: operator-surface / /graph/triples answers a sorted, validated read of ENTITY_STATES
func TestGraphTriplesSkipsOnlyADeletedKey(t *testing.T) {
	deleted := semantictest.EntityID(t, "acme", "ops", "test", "service", "entity", "1")
	failing := semantictest.EntityID(t, "acme", "ops", "test", "service", "entity", "2")
	kept := semantictest.EntityID(t, "acme", "ops", "test", "service", "entity", "3")
	predicate := semantictest.Predicate(t, "test", "state", "value")
	keptTriples := []message.Triple{entityTriple(kept, predicate, "v")}

	t.Run("a deleted key is skipped", func(t *testing.T) {
		reader := &stubEntityReader{
			keys:   []string{kept, deleted},
			values: map[string][]byte{kept: storedEntity(t, kept, keptTriples)},
			errs:   map[string]error{deleted: jetstream.ErrKeyNotFound},
		}
		got, err := scanGraphTriples(t.Context(), reader, tripleQueryParams{limit: graphTriplesDefaultLimit}, discardLogger)
		require.NoError(t, err)
		require.JSONEq(t, tripleJSON(t, keptTriples), tripleJSON(t, got))
	})

	for _, tt := range []struct {
		name   string
		reader *stubEntityReader
		want   error
	}{
		{
			name: "a read that times out",
			reader: &stubEntityReader{
				keys:   []string{kept, failing, deleted},
				values: map[string][]byte{kept: storedEntity(t, kept, keptTriples)},
				errs:   map[string]error{deleted: jetstream.ErrKeyNotFound, failing: context.DeadlineExceeded},
			},
			want: context.DeadlineExceeded,
		},
		{
			name: "an entity the decoder refuses",
			reader: &stubEntityReader{
				keys: []string{kept, failing},
				values: map[string][]byte{
					kept:    storedEntity(t, kept, keptTriples),
					failing: []byte(`{"id":"not-an-entity-id","triples":[]}`),
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := scanGraphTriples(t.Context(), tt.reader, tripleQueryParams{limit: graphTriplesDefaultLimit}, discardLogger)
			require.Error(t, err)
			require.Nil(t, got, "a failed read returned triples")
			if tt.want != nil {
				require.ErrorIs(t, err, tt.want)
			}

			querier := func(ctx context.Context, p tripleQueryParams) ([]message.Triple, error) {
				return scanGraphTriples(ctx, tt.reader, p, discardLogger)
			}
			w := httptest.NewRecorder()
			serveGraphTriples(w, httptest.NewRequest(http.MethodGet, "/graph/triples", nil), querier, discardLogger)
			require.Equal(t, http.StatusInternalServerError, w.Code)
		})
	}
}

// TestScanGraphTriplesMatchesSortedOracle: the answer is the first limit triples, in key and then
// position order, of every triple matching the filters across all entities, however the reader
// lists its keys (design D6 and § Generated checks: a stated law). The oracle sorts the entity IDs
// itself and matches each filter on its own: subject and predicate by equality, object by its %v
// text.
//
// The named example fixes the boundary the law exists for: keys listed in reverse and a limit
// below the matches, so a scan in listing order returns the wrong triple on every run. The
// generated part draws zero to six entities of zero to three triples each from small pools, so
// filters often match several entities; lists them in a drawn permutation; draws each filter as
// absent or one value from its pool, a subject included; and draws the limit from one, one below
// the match count, the match count and one above, so a cut inside, at and past the matches is
// reached by construction. Both assertions run on every draw.
//
// spec: operator-surface / /graph/triples answers a sorted, validated read of ENTITY_STATES
func TestScanGraphTriplesMatchesSortedOracle(t *testing.T) {
	ids := make([]string, 6)
	for i := range ids {
		ids[i] = semantictest.EntityID(t, "acme", "ops", "test", "service", "entity", strconv.Itoa(i))
	}
	predicates := []string{
		semantictest.Predicate(t, "test", "state", "a"),
		semantictest.Predicate(t, "test", "state", "b"),
	}
	objects := []any{"x", "y", 2.0}

	t.Run("reverse listing below the limit", func(t *testing.T) {
		values := map[string][]byte{}
		var sorted []message.Triple
		for _, id := range ids[:3] {
			triples := []message.Triple{entityTriple(id, predicates[0], "x"), entityTriple(id, predicates[1], "y")}
			values[id] = storedEntity(t, id, triples)
			sorted = append(sorted, triples...)
		}
		reader := &stubEntityReader{keys: []string{ids[2], ids[1], ids[0]}, values: values}
		for _, limit := range []int{1, 3} {
			got, err := scanGraphTriples(t.Context(), reader, tripleQueryParams{limit: limit}, discardLogger)
			require.NoError(t, err)
			require.JSONEq(t, tripleJSON(t, sorted[:limit]), tripleJSON(t, got), "limit %d", limit)
		}
	})

	rapid.Check(t, func(rt *rapid.T) {
		listing := rapid.Permutation(ids).Draw(rt, "listing")
		listing = listing[:rapid.IntRange(0, len(ids)).Draw(rt, "entities")]
		values := map[string][]byte{}
		stored := map[string][]message.Triple{}
		for _, id := range listing {
			triples := make([]message.Triple, rapid.IntRange(0, 3).Draw(rt, "triple count"))
			for i := range triples {
				triples[i] = entityTriple(id,
					rapid.SampledFrom(predicates).Draw(rt, "predicate"),
					rapid.SampledFrom(objects).Draw(rt, "object"))
			}
			data, err := graph.MarshalEntityState(&graph.EntityState{ID: id, Triples: triples})
			if err != nil {
				rt.Fatalf("encode %s: %v", id, err)
			}
			values[id], stored[id] = data, triples
		}
		params := tripleQueryParams{
			subject:   rapid.SampledFrom(append([]string{""}, ids...)).Draw(rt, "subject filter"),
			predicate: rapid.SampledFrom(append([]string{""}, predicates...)).Draw(rt, "predicate filter"),
			object:    rapid.SampledFrom([]string{"", "x", "y", "2"}).Draw(rt, "object filter"),
		}

		var matches []message.Triple
		for _, id := range slices.Sorted(maps.Keys(stored)) {
			for _, tr := range stored[id] {
				if (params.subject == "" || tr.Subject == params.subject) &&
					(params.predicate == "" || tr.Predicate == params.predicate) &&
					(params.object == "" || fmt.Sprintf("%v", tr.Object) == params.object) {
					matches = append(matches, tr)
				}
			}
		}
		limits := []int{1}
		for _, l := range []int{len(matches) - 1, len(matches), len(matches) + 1} {
			if l > 1 && !slices.Contains(limits, l) {
				limits = append(limits, l)
			}
		}
		params.limit = rapid.SampledFrom(limits).Draw(rt, "limit")
		want := matches[:min(params.limit, len(matches))]

		got, err := scanGraphTriples(t.Context(), &stubEntityReader{keys: listing, values: values}, params, discardLogger)
		if err != nil {
			rt.Fatalf("scan: %v", err)
		}
		if got == nil {
			rt.Fatalf("scan returned nil, want an empty slice")
		}
		wantJSON, err := json.Marshal(want)
		if err != nil {
			rt.Fatalf("encode want: %v", err)
		}
		gotJSON, err := json.Marshal(got)
		if err != nil {
			rt.Fatalf("encode got: %v", err)
		}
		if want == nil {
			wantJSON = []byte("[]")
		}
		if string(gotJSON) != string(wantJSON) {
			rt.Fatalf("scan with %+v over listing %v:\n got: %s\nwant: %s", params, listing, gotJSON, wantJSON)
		}
	})
}
