package graphingest

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/pkg/errs"
	semtypes "github.com/c360studio/semengine/pkg/types"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleQueryPrefix_InvalidPrefixHasNoBucketIO(t *testing.T) {
	comp, mock := createTestComponentWithMockKVBucket(t, withAuthority("acme", "ops"))
	var lists atomic.Int64
	mock.listFilteredFunc = func(context.Context, ...string) (jetstream.KeyLister, error) {
		lists.Add(1)
		return nil, nil
	}
	request, err := json.Marshal(graph.PrefixQueryRequest{Prefix: "acme.*"})
	require.NoError(t, err)
	_, err = comp.handleQueryPrefixWithMaxPayload(context.Background(), request, 1<<20)
	require.Error(t, err)
	assert.True(t, errs.IsInvalid(err))
	var classified *errs.ClassifiedError
	require.ErrorAs(t, err, &classified)
	assert.Equal(t, semtypes.ErrorCodeEntityIDPrefixInvalid, classified.Code)
	assert.Zero(t, lists.Load())
}

// entity-id-audit:classify intentional-malformed "acme.*" line=27 column=64 surface=go-field:PrefixQueryRequest.Prefix entity_id_prefix_invalid:first_byte wildcard query-prefix rejection fixture

// storePrefixEntity is a helper that stores an entity with a single triple.
func storePrefixEntity(t *testing.T, comp *Component, id string) {
	t.Helper()
	ctx := context.Background()
	entity := &graph.EntityState{
		ID:          id,
		MessageType: testEntityType(),
		Triples: withTestMetadata(
			message.Triple{
				Subject:   id,
				Predicate: "test.entity.attribute",
				Object:    "val",
				Timestamp: time.Now(),
			},
		),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, comp.CreateEntity(ctx, entity))
}

// callPrefixHandler is a helper that marshals the request and calls
// handleQueryPrefixNATS, returning a decoded PrefixQueryResponse.
func callPrefixHandler(t *testing.T, comp *Component, req graph.PrefixQueryRequest) graph.PrefixQueryResponse {
	return callPrefixHandlerWithMaxPayload(t, comp, req, 1<<20)
}

func callPrefixHandlerWithMaxPayload(
	t *testing.T,
	comp *Component,
	req graph.PrefixQueryRequest,
	maxPayload int64,
) graph.PrefixQueryResponse {
	t.Helper()
	reqData, err := json.Marshal(req)
	require.NoError(t, err)

	respData, err := comp.handleQueryPrefixWithMaxPayload(context.Background(), reqData, maxPayload)
	require.NoError(t, err)

	var resp graph.PrefixQueryResponse
	require.NoError(t, json.Unmarshal(respData, &resp))
	return resp
}

// TestHandleQueryPrefix_SortDeterminism verifies that the same set of keys is
// always returned in sorted lexicographic order regardless of insertion order.
func TestHandleQueryPrefix_SortDeterminism(t *testing.T) {
	comp := createTestComponentWithMockKV(t, withAuthority("acme", "ops"))

	// Store entities in reverse lexicographic order to confirm sorting happens.
	ids := []string{
		"acme.ops.robotics.gcs.drone.zzz",
		"acme.ops.robotics.gcs.drone.aaa",
		"acme.ops.robotics.gcs.drone.mmm",
	}
	for _, id := range ids {
		storePrefixEntity(t, comp, id)
	}

	resp := callPrefixHandler(t, comp, graph.PrefixQueryRequest{Prefix: "acme"})
	require.Len(t, resp.Entities, 3)

	// Verify lexicographic order.
	assert.Equal(t, "acme.ops.robotics.gcs.drone.aaa", resp.Entities[0].ID)
	assert.Equal(t, "acme.ops.robotics.gcs.drone.mmm", resp.Entities[1].ID)
	assert.Equal(t, "acme.ops.robotics.gcs.drone.zzz", resp.Entities[2].ID)
}

// TestHandleQueryPrefix_CursorPagination verifies that paginating with a cursor
// returns sorted, disjoint pages that together cover the full result set.
func TestHandleQueryPrefix_CursorPagination(t *testing.T) {
	comp := createTestComponentWithMockKV(t, withAuthority("org", "ops"))

	// Store 5 entities.
	for i := 1; i <= 5; i++ {
		storePrefixEntity(t, comp, fmt.Sprintf("org.ops.dom.sys.type.entity-%03d", i))
	}

	// Page 1: limit=2.
	page1 := callPrefixHandler(t, comp, graph.PrefixQueryRequest{
		Prefix: "org",
		Limit:  2,
	})
	require.Len(t, page1.Entities, 2)
	assert.NotEmpty(t, page1.NextCursor, "page1 must carry a cursor because there are more results")
	assert.Equal(t, "org.ops.dom.sys.type.entity-001", page1.Entities[0].ID)
	assert.Equal(t, "org.ops.dom.sys.type.entity-002", page1.Entities[1].ID)

	// Page 2: limit=2, cursor from page 1.
	page2 := callPrefixHandler(t, comp, graph.PrefixQueryRequest{
		Prefix: "org",
		Limit:  2,
		Cursor: page1.NextCursor,
	})
	require.Len(t, page2.Entities, 2)
	assert.NotEmpty(t, page2.NextCursor, "page2 must carry a cursor because there are more results")
	assert.Equal(t, "org.ops.dom.sys.type.entity-003", page2.Entities[0].ID)
	assert.Equal(t, "org.ops.dom.sys.type.entity-004", page2.Entities[1].ID)

	// Page 3: cursor from page 2 — should return the last entity with no cursor.
	page3 := callPrefixHandler(t, comp, graph.PrefixQueryRequest{
		Prefix: "org",
		Limit:  2,
		Cursor: page2.NextCursor,
	})
	require.Len(t, page3.Entities, 1)
	assert.Empty(t, page3.NextCursor, "page3 is the last page: no cursor expected")
	assert.Equal(t, "org.ops.dom.sys.type.entity-005", page3.Entities[0].ID)

	// Disjointness: collect all IDs across pages and verify no duplicates.
	seen := map[string]bool{}
	for _, p := range []graph.PrefixQueryResponse{page1, page2, page3} {
		for _, e := range p.Entities {
			assert.False(t, seen[e.ID], "duplicate entity ID across pages: %s", e.ID)
			seen[e.ID] = true
		}
	}
	assert.Len(t, seen, 5, "all 5 entities must appear across the pages")
}

// TestHandleQueryPrefix_LimitClampZero verifies that limit<=0 is clamped to
// DefaultPrefixQueryLimit rather than returning zero or an error.
func TestHandleQueryPrefix_LimitClampZero(t *testing.T) {
	comp := createTestComponentWithMockKV(t, withAuthority("acme", "ops"))
	storePrefixEntity(t, comp, "acme.ops.dom.sys.type.entity-001")
	storePrefixEntity(t, comp, "acme.ops.dom.sys.type.entity-002")

	// limit=0 should default to DefaultPrefixQueryLimit (both entities returned).
	resp := callPrefixHandler(t, comp, graph.PrefixQueryRequest{Prefix: "acme", Limit: 0})
	assert.Len(t, resp.Entities, 2, "limit=0 must default to DefaultPrefixQueryLimit")

	// limit=-1 should also default.
	resp2 := callPrefixHandler(t, comp, graph.PrefixQueryRequest{Prefix: "acme", Limit: -1})
	assert.Len(t, resp2.Entities, 2, "limit=-1 must default to DefaultPrefixQueryLimit")
}

// TestHandleQueryPrefix_LimitClampAboveMax verifies that limit > MaxPrefixQueryLimit
// is silently clamped — the handler returns at most MaxPrefixQueryLimit entities.
func TestHandleQueryPrefix_LimitClampAboveMax(t *testing.T) {
	comp := createTestComponentWithMockKV(t, withAuthority("org", "ops"))
	// Store 3 entities — well below the cap but enough to verify clamping doesn't error.
	for i := 1; i <= 3; i++ {
		storePrefixEntity(t, comp, fmt.Sprintf("org.ops.dom.sys.type.entity-%03d", i))
	}

	// Request limit of 999999 (above MaxPrefixQueryLimit=1000).
	resp := callPrefixHandler(t, comp, graph.PrefixQueryRequest{Prefix: "org", Limit: 999999})
	// All 3 entities must be returned (clamping doesn't truncate the actual result
	// when total entities < MaxPrefixQueryLimit).
	assert.Len(t, resp.Entities, 3, "clamped limit must not reduce below actual result count")
	assert.Empty(t, resp.NextCursor, "no cursor when all entities fit in one page")
}

// TestMarshalFittingPrefixPageIncludesCursorInExactCandidate verifies that the
// encoded page, including its continuation token, is the value fitted against
// the observed carrier limit.
func TestMarshalFittingPrefixPageIncludesCursorInExactCandidate(t *testing.T) {
	entities := []graph.EntityState{
		{ID: "a.b.c.d.e.one"},
		{ID: "a.b.c.d.e.two"},
		{ID: "a.b.c.d.e.three"},
	}

	want := graph.PrefixQueryResponse{
		Entities:   entities[:1],
		NextCursor: graph.EncodeCursor(entities[0].ID),
	}
	wantBytes, err := json.Marshal(want)
	require.NoError(t, err)

	encoded, err := marshalFittingPrefixPage(entities, "", int64(len(wantBytes)))
	require.NoError(t, err)
	assert.JSONEq(t, string(wantBytes), string(encoded))
	assert.Len(t, encoded, len(wantBytes), "fitting must use the exact complete page encoding")
}

func TestMarshalFittingPrefixPageRejectsFirstIndivisibleEntity(t *testing.T) {
	entities := []graph.EntityState{{ID: "a.b.c.d.e.one"}, {ID: "a.b.c.d.e.two"}}
	firstCandidate, err := json.Marshal(graph.PrefixQueryResponse{
		Entities:   entities[:1],
		NextCursor: graph.EncodeCursor(entities[0].ID),
	})
	require.NoError(t, err)
	maxPayload := int64(len(firstCandidate) - 1)

	encoded, fitErr := marshalFittingPrefixPage(entities, "", maxPayload)
	require.Error(t, fitErr)
	assert.Nil(t, encoded)
	assert.True(t, errs.IsInvalid(fitErr))
	var classified *errs.ClassifiedError
	require.ErrorAs(t, fitErr, &classified)
	assert.Equal(t, "response_too_large", classified.Code)
	assert.Equal(t, int64(len(firstCandidate)), classified.Detail["response_bytes"])
	assert.Equal(t, maxPayload, classified.Detail["max_payload"])
}

// TestHandleQueryPrefix_InvalidCursor verifies that a malformed cursor returns
// a classified error rather than panicking or silently returning wrong data.
func TestHandleQueryPrefix_InvalidCursor(t *testing.T) {
	comp := createTestComponentWithMockKV(t, withAuthority("acme", "ops"))
	storePrefixEntity(t, comp, "acme.ops.dom.sys.type.entity-001")

	reqData, err := json.Marshal(graph.PrefixQueryRequest{
		Prefix: "acme",
		Limit:  10,
		Cursor: "!!!invalid-base64!!!",
	})
	require.NoError(t, err)

	_, err = comp.handleQueryPrefixWithMaxPayload(context.Background(), reqData, 1<<20)
	require.Error(t, err, "invalid cursor must return an error")
}

// TestHandleQueryPrefix_SinglePageNoNextCursor verifies that an exhausted page
// omits NextCursor.
func TestHandleQueryPrefix_SinglePageNoNextCursor(t *testing.T) {
	comp := createTestComponentWithMockKV(t, withAuthority("acme", "ops"))
	storePrefixEntity(t, comp, "acme.ops.dom.sys.type.entity-001")
	storePrefixEntity(t, comp, "acme.ops.dom.sys.type.entity-002")

	reqData, err := json.Marshal(graph.PrefixQueryRequest{Prefix: "acme", Limit: 100})
	require.NoError(t, err)

	respData, err := comp.handleQueryPrefixWithMaxPayload(context.Background(), reqData, 1<<20)
	require.NoError(t, err)

	// Decode as raw map to check field absence.
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(respData, &raw))
	_, hasCursor := raw["next_cursor"]
	assert.False(t, hasCursor, "next_cursor must be absent when all results fit in one page")

	// Also verify entities key is present.
	_, hasEntities := raw["entities"]
	assert.True(t, hasEntities, "entities key must always be present")
}

// TestHandleQueryPrefix_FirstPageRequest verifies that omitting cursor requests
// the first page.
func TestHandleQueryPrefix_FirstPageRequest(t *testing.T) {
	comp := createTestComponentWithMockKV(t, withAuthority("c360", "platform"))
	storePrefixEntity(t, comp, "c360.platform.robotics.mav1.drone.001")

	req := map[string]any{"prefix": "c360", "limit": 10}
	reqData, err := json.Marshal(req)
	require.NoError(t, err)

	respData, err := comp.handleQueryPrefixWithMaxPayload(context.Background(), reqData, 1<<20)
	require.NoError(t, err)

	// Must decode into PrefixQueryResponse successfully.
	var resp graph.PrefixQueryResponse
	require.NoError(t, json.Unmarshal(respData, &resp))
	require.Len(t, resp.Entities, 1)
	assert.Equal(t, "c360.platform.robotics.mav1.drone.001", resp.Entities[0].ID)
}

func matchesWildcardPattern(filter, key string) bool {
	matched, err := semtypes.MatchEntityIDPattern(filter, key)
	return err == nil && matched
}

// TestQueryPrefix_PatternMatchesAcrossSystems verifies that a 6-position
// wildcard pattern matches entities across multiple system segments and returns
// them in deterministic lexicographic ID order.
func TestQueryPrefix_PatternMatchesAcrossSystems(t *testing.T) {
	comp := createTestComponentWithMockKV(t, withAuthority("o", "p"))

	for _, id := range []string{
		"o.p.sys2.intel.platform.uav-02",
		"o.p.sys1.intel.platform.uav-01",
		"o.p.sys1.intel.sensor.radar-01",
		"o.p.sys3.other.platform.uav-03",
		"o.p.sys0.intel.platform.uav-00",
	} {
		storePrefixEntity(t, comp, id)
	}

	resp := callPrefixHandler(t, comp, graph.PrefixQueryRequest{
		Pattern: "o.p.*.intel.platform.*",
	})
	require.Len(t, resp.Entities, 3)
	assert.Equal(t, "o.p.sys0.intel.platform.uav-00", resp.Entities[0].ID)
	assert.Equal(t, "o.p.sys1.intel.platform.uav-01", resp.Entities[1].ID)
	assert.Equal(t, "o.p.sys2.intel.platform.uav-02", resp.Entities[2].ID)
	assert.Empty(t, resp.NextCursor)
}

// TestQueryPrefix_PatternAndPrefixMutuallyExclusive verifies that setting both
// Prefix and Pattern on the same request is rejected before any KV I/O with a
// classified invalid_request error.
func TestQueryPrefix_PatternAndPrefixMutuallyExclusive(t *testing.T) {
	comp, mock := createTestComponentWithMockKVBucket(t, withAuthority("acme", "ops"))
	var lists atomic.Int64
	mock.listFilteredFunc = func(context.Context, ...string) (jetstream.KeyLister, error) {
		lists.Add(1)
		return nil, nil
	}

	reqData, err := json.Marshal(graph.PrefixQueryRequest{
		Prefix:  "acme.ops",
		Pattern: "acme.ops.*.intel.platform.*",
	})
	require.NoError(t, err)

	_, err = comp.handleQueryPrefixWithMaxPayload(context.Background(), reqData, 1<<20)
	require.Error(t, err)
	assert.True(t, errs.IsInvalid(err))
	var classified *errs.ClassifiedError
	require.ErrorAs(t, err, &classified)
	assert.Equal(t, graph.ErrorCodeInvalidRequest, classified.Code)
	assert.Zero(t, lists.Load(), "mutually exclusive request must not touch KV bucket")
}

// TestQueryPrefix_PatternInvalid verifies that 5-part, 7-part, ">", and
// malformed-segment patterns are rejected by ValidateEntityIDPattern before any
// KV I/O.
func TestQueryPrefix_PatternInvalid(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		reason  string
	}{
		{name: "five parts", pattern: "o.p.*.intel.platform", reason: semtypes.EntityIDReasonArity},
		{name: "seven parts", pattern: "o.p.s.intel.platform.inst.extra", reason: semtypes.EntityIDReasonArity},
		{name: "tail wildcard >", pattern: "o.p.s.intel.platform.>", reason: semtypes.EntityIDReasonFirstByte},
		{name: "partial wildcard segment", pattern: "o.p.s.intel.plat*.inst", reason: semtypes.EntityIDReasonAlphabet},
		{name: "empty segment", pattern: "o.p..intel.platform.*", reason: semtypes.EntityIDReasonEmptySegment},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			comp, mock := createTestComponentWithMockKVBucket(t, withAuthority("o", "p"))
			var lists atomic.Int64
			mock.listFilteredFunc = func(context.Context, ...string) (jetstream.KeyLister, error) {
				lists.Add(1)
				return nil, nil
			}

			reqData, err := json.Marshal(graph.PrefixQueryRequest{Pattern: tc.pattern})
			require.NoError(t, err)

			_, err = comp.handleQueryPrefixWithMaxPayload(context.Background(), reqData, 1<<20)
			require.Error(t, err)
			assert.True(t, errs.IsInvalid(err))
			var classified *errs.ClassifiedError
			require.ErrorAs(t, err, &classified)
			assert.Equal(t, semtypes.ErrorCodeEntityIDPatternInvalid, classified.Code)
			assert.Equal(t, tc.reason, classified.Detail[semtypes.EntityIDDetailReason])
			assert.Zero(t, lists.Load(), "invalid pattern must not touch KV bucket")
		})
	}
}

// TestQueryPrefix_PatternCursorPaging verifies that paginating a pattern query
// both by Limit and by max_payload byte-trimming returns sorted, disjoint pages
// covering the exact matching set.
func TestQueryPrefix_PatternCursorPaging(t *testing.T) {
	comp := createTestComponentWithMockKV(t, withAuthority("o", "p"))

	expected := []string{
		"o.p.sys1.intel.platform.uav-01",
		"o.p.sys1.intel.platform.uav-02",
		"o.p.sys2.intel.platform.uav-03",
		"o.p.sys3.intel.platform.uav-04",
		"o.p.sys4.intel.platform.uav-05",
	}
	// Seed matching entities out of order plus non-matching entities.
	for _, id := range []string{
		expected[3],
		"o.p.sys1.intel.sensor.s-01",
		expected[0],
		expected[4],
		"o.p.sys2.other.platform.x-01",
		expected[2],
		expected[1],
	} {
		storePrefixEntity(t, comp, id)
	}

	t.Run("count limit pagination", func(t *testing.T) {
		var got []string
		var cursor string
		for page := 0; page < 10; page++ {
			resp := callPrefixHandler(t, comp, graph.PrefixQueryRequest{
				Pattern: "o.p.*.intel.platform.*",
				Limit:   2,
				Cursor:  cursor,
			})
			require.NotEmpty(t, resp.Entities)
			for _, e := range resp.Entities {
				got = append(got, e.ID)
			}
			cursor = resp.NextCursor
			if cursor == "" {
				break
			}
		}
		assert.Equal(t, expected, got)
	})

	t.Run("byte-trimmed pagination", func(t *testing.T) {
		var got []string
		var cursor string
		sawTrim := false
		for page := 0; page < 10; page++ {
			resp := callPrefixHandlerWithMaxPayload(t, comp, graph.PrefixQueryRequest{
				Pattern: "o.p.*.intel.platform.*",
				Limit:   len(expected),
				Cursor:  cursor,
			}, 1200)
			require.NotEmpty(t, resp.Entities)
			if len(resp.Entities) < len(expected) {
				sawTrim = true
			}
			for _, e := range resp.Entities {
				got = append(got, e.ID)
			}
			cursor = resp.NextCursor
			if cursor == "" {
				break
			}
		}
		require.True(t, sawTrim, "byte budget must trim at least one page")
		assert.Equal(t, expected, got)
	})
}

// TestQueryPrefix_PatternNoMatches verifies that a valid pattern matching no
// keys returns an empty page without attempting an exact-entity Get fallback.
func TestQueryPrefix_PatternNoMatches(t *testing.T) {
	comp, mock := createTestComponentWithMockKVBucket(t, withAuthority("o", "p"))
	storePrefixEntity(t, comp, "o.p.sys1.intel.sensor.radar-01")

	var gets atomic.Int64
	mock.getFunc = func(context.Context, string) (jetstream.KeyValueEntry, error) {
		gets.Add(1)
		return nil, jetstream.ErrKeyNotFound
	}

	// Concrete 6-part pattern with no wildcards or with wildcards must not trigger Get fallback.
	for _, pattern := range []string{
		"o.p.*.intel.platform.*",
		"o.p.sys1.intel.platform.uav-01",
	} {
		reqData, err := json.Marshal(graph.PrefixQueryRequest{
			Pattern: pattern,
		})
		require.NoError(t, err)

		respData, err := comp.handleQueryPrefixWithMaxPayload(context.Background(), reqData, 1<<20)
		require.NoError(t, err)
		assert.JSONEq(t, `{"entities":null}`, string(respData))

		var resp graph.PrefixQueryResponse
		require.NoError(t, json.Unmarshal(respData, &resp))
		assert.Empty(t, resp.Entities)
		assert.Empty(t, resp.NextCursor)
		assert.Zero(t, gets.Load(), "pattern query must not fall back to exact-entity Get (%s)", pattern)
	}
}
