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
)

// Seeds for the poison tests: literal six-part IDs.
const (
	seedA = "acme.ops.test.system.widget.a"
	seedB = "acme.ops.test.system.widget.b"
	seedC = "acme.ops.test.system.widget.c"
)

// resetRefusal is the refusal a responder sends for a poisoned stored entity,
// as ClassifyReply rebuilds it from the reply's headers. Its text names no ID,
// so a client that read the text would find nothing to act on.
func resetRefusal() error {
	return errs.ClassifiedCode(errs.ErrorFatal, graph.ErrorCodeGraphStateResetRequired, errors.New("stored entity refused"))
}

// poisonWire fakes graph-query's wire for the per-seed read. The batch returns
// batchErr, or batchReply when batchErr is nil. An entity request is answered
// by entityErr or entityReply for its ID, an empty ID is refused
// invalid_request as graph-ingest refuses it, and every other ID gets a good
// entity. It records the ID of each entity request in send order.
type poisonWire struct {
	t           *testing.T
	batchErr    error
	batchReply  []byte
	prefixReply []byte
	entityErr   map[string]error
	entityReply map[string][]byte
	onEntity    func(id string)
	entityIDs   []string
}

func (w *poisonWire) RequestClassified(_ context.Context, subject string, data []byte, _ time.Duration) ([]byte, error) {
	switch subject {
	case subjectPrefix:
		return w.prefixReply, nil
	case subjectBatch:
		return w.batchReply, w.batchErr
	case subjectEntity:
		var req graph.EntityRequest
		require.NoError(w.t, json.Unmarshal(data, &req))
		w.entityIDs = append(w.entityIDs, req.ID)
		if w.onEntity != nil {
			w.onEntity(req.ID)
		}
		if req.ID == "" {
			return nil, errs.ClassifiedCode(errs.ErrorInvalid, graph.ErrorCodeInvalidRequest, errors.New("id is required"))
		}
		if err, ok := w.entityErr[req.ID]; ok {
			return nil, err
		}
		if raw, ok := w.entityReply[req.ID]; ok {
			return raw, nil
		}
		return goodEntity(w.t, req.ID), nil
	default:
		w.t.Fatalf("unexpected subject %q", subject)
		return nil, nil
	}
}

// goodEntity is an entity reply the stored-value rule accepts.
func goodEntity(t *testing.T, id string) []byte {
	t.Helper()
	es := graph.EntityState{ID: id, Triples: []message.Triple{stamped(id, "dc.terms.title", id)}}
	return mustJSON(t, graph.ExactEntity{Entity: &es, KVRevision: 1})
}

// refusedEntity is an entity reply whose triple names a malformed subject, which
// the client's own UnmarshalEntityState refuses.
func refusedEntity(t *testing.T, id string) []byte {
	t.Helper()
	es := graph.EntityState{ID: id, Triples: []message.Triple{stamped("bad", "dc.terms.title", "x")}}
	return mustJSON(t, graph.ExactEntity{Entity: &es, KVRevision: 1})
}

func newPoisonClient(t *testing.T, w *poisonWire) *Client {
	t.Helper()
	w.t = t
	c, err := New(w, time.Second)
	require.NoError(t, err)
	return c
}

func entityIDs(h fusion.Hydration) []string {
	ids := make([]string, 0, len(h.Entities))
	for _, e := range h.Entities {
		ids = append(ids, e.ID)
	}
	return ids
}

// TestPoisonedSeedUnhydrated holds that a batch refused for poison is read one
// ID at a time, the poisoned seed (second of three) listed unhydrated error and
// the other two hydrated in request order.
func TestPoisonedSeedUnhydrated(t *testing.T) {
	w := &poisonWire{batchErr: resetRefusal(), entityErr: map[string]error{seedB: resetRefusal()}}
	got, err := newPoisonClient(t, w).Entities(t.Context(), []string{seedA, seedB, seedC})
	require.NoError(t, err)

	assert.Equal(t, []string{seedA, seedB, seedC}, w.entityIDs, "one entity request per seed, in request order")
	assert.Equal(t, []string{seedA, seedC}, entityIDs(got), "the good seeds hydrate in request order")
	assert.Equal(t, []fusion.Unhydrated{{Handle: seedB, Reason: fusion.UnhydratedError}}, got.Unhydrated)
}

// TestPoisonFallbackNotFound holds that a seed the per-ID read does not find is
// listed unhydrated not_found.
func TestPoisonFallbackNotFound(t *testing.T) {
	notFound := errs.ClassifiedCode(errs.ErrorInvalid, graph.ErrorCodeEntityNotFound, errors.New("not found"))
	w := &poisonWire{batchErr: resetRefusal(), entityErr: map[string]error{seedB: resetRefusal(), seedC: notFound}}
	got, err := newPoisonClient(t, w).Entities(t.Context(), []string{seedA, seedB, seedC})
	require.NoError(t, err)

	assert.Equal(t, []string{seedA}, entityIDs(got))
	assert.Equal(t, []fusion.Unhydrated{
		{Handle: seedB, Reason: fusion.UnhydratedError},
		{Handle: seedC, Reason: fusion.UnhydratedNotFound},
	}, got.Unhydrated)
}

// prefixLens resolves every query as an ID prefix and walks no edges, so a
// Fuse call reads only the prefix and the seeds.
type prefixLens struct{}

func (prefixLens) Name() string                          { return "prefix" }
func (prefixLens) ResolveMode(string) fusion.ResolveMode { return fusion.ResolveModePrefix }
func (prefixLens) Edges() []fusion.EdgeSpec              { return nil }
func (prefixLens) Label(e *fusion.Entity) string         { return e.ID }
func (prefixLens) Kind(*fusion.Entity) string            { return "entity" }
func (prefixLens) Location(*fusion.Entity) fusion.Locator {
	return fusion.Locator{}
}
func (prefixLens) Hydrate(context.Context, *fusion.Entity) (*message.StorageReference, error) {
	return nil, nil
}

// TestAllSeedsPoisonedDefer holds that when every seed is poisoned the engine
// over fusionnats answers deferred all_seeds_unhydrated, each seed listed
// error, rather than failing the call.
func TestAllSeedsPoisonedDefer(t *testing.T) {
	w := &poisonWire{
		prefixReply: mustJSON(t, graph.PrefixQueryResponse{Entities: []graph.EntityState{{ID: seedA}, {ID: seedB}}}),
		batchErr:    resetRefusal(),
		entityErr:   map[string]error{seedA: resetRefusal(), seedB: resetRefusal()},
	}
	eng, err := fusion.NewEngine(newPoisonClient(t, w), fusion.NewBodyResolver(fusion.MapStoreResolver{}), slog.New(slog.DiscardHandler))
	require.NoError(t, err)

	resp, err := eng.Fuse(t.Context(), fusion.Request{Query: "acme.ops"}, prefixLens{})
	require.NoError(t, err, "a poisoned seed is listed, never a failed answer")
	assert.Empty(t, resp.Nodes)
	assert.Empty(t, resp.Misses)
	assert.True(t, resp.Deferred)
	assert.Equal(t, "all_seeds_unhydrated", resp.DeferReason)
	assert.Equal(t, []fusion.Unhydrated{
		{Handle: seedA, Reason: fusion.UnhydratedError},
		{Handle: seedB, Reason: fusion.UnhydratedError},
	}, resp.Unhydrated)
}

// TestPoisonedDuplicateReadOnce holds that a poisoned ID requested twice is read
// once and listed once.
func TestPoisonedDuplicateReadOnce(t *testing.T) {
	w := &poisonWire{batchErr: resetRefusal(), entityErr: map[string]error{seedB: resetRefusal()}}
	got, err := newPoisonClient(t, w).Entities(t.Context(), []string{seedB, seedA, seedB})
	require.NoError(t, err)

	assert.Equal(t, []string{seedB, seedA}, w.entityIDs, "each distinct ID is read once")
	assert.Equal(t, []string{seedA}, entityIDs(got))
	assert.Equal(t, []fusion.Unhydrated{{Handle: seedB, Reason: fusion.UnhydratedError}}, got.Unhydrated)
}

// TestPoisonFallbackReturnsOtherError holds that a per-ID read failing for any
// reason but poison or absence fails Entities with that error.
func TestPoisonFallbackReturnsOtherError(t *testing.T) {
	transient := errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeInternal, errors.New("boom"))
	w := &poisonWire{batchErr: resetRefusal(), entityErr: map[string]error{seedB: transient}}
	got, err := newPoisonClient(t, w).Entities(t.Context(), []string{seedA, seedB, seedC})
	require.ErrorIs(t, err, transient)
	assertNoProjection(t, got)
	assert.Equal(t, []string{seedA, seedB}, w.entityIDs, "the read stops at the fault")
}

// TestPoisonFallbackEndedContext holds that the per-ID read stops and returns
// the context's error once the caller gives up, even when the requester still
// answers.
func TestPoisonFallbackEndedContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	// The caller gives up while the first entity request is in flight; its
	// reply still arrives.
	w := &poisonWire{batchErr: resetRefusal(), onEntity: func(string) { cancel() }}
	got, err := newPoisonClient(t, w).Entities(ctx, []string{seedA, seedB, seedC})
	require.ErrorIs(t, err, context.Canceled)
	assertNoProjection(t, got)
	assert.Equal(t, []string{seedA}, w.entityIDs, "no request is sent after the context ended")
}

// TestPoisonFallbackEmptyID holds that an empty requested ID is listed
// unhydrated error without a request, as the batch lists it.
func TestPoisonFallbackEmptyID(t *testing.T) {
	w := &poisonWire{batchErr: resetRefusal()}
	got, err := newPoisonClient(t, w).Entities(t.Context(), []string{seedA, "", seedC})
	require.NoError(t, err)

	assert.Equal(t, []string{seedA, seedC}, w.entityIDs, "no entity request carries the empty ID")
	assert.Equal(t, []string{seedA, seedC}, entityIDs(got))
	assert.Equal(t, []fusion.Unhydrated{{Handle: "", Reason: fusion.UnhydratedError}}, got.Unhydrated)
}

// TestOwnValidationNotRetried holds that the client's own refusal of a reply,
// which carries the same reset code as the responder's, is returned: a refused
// batch reply starts no per-ID read, and a refused per-ID reply is not listed.
func TestOwnValidationNotRetried(t *testing.T) {
	t.Run("batch reply", func(t *testing.T) {
		w := &poisonWire{batchReply: mustJSON(t, graph.EntityBatchResponse{Entities: []graph.EntityState{{
			ID: seedA, Triples: []message.Triple{stamped("bad", "dc.terms.title", "x")},
		}}})}
		got, err := newPoisonClient(t, w).Entities(t.Context(), []string{seedA, seedB})
		require.Error(t, err)
		assert.True(t, graph.IsStateContractError(err), "the client's refusal carries the reset code too")
		assertNoProjection(t, got)
		assert.Empty(t, w.entityIDs, "a refused batch reply sends no entity request")
	})
	t.Run("per-ID reply", func(t *testing.T) {
		w := &poisonWire{batchErr: resetRefusal(), entityReply: map[string][]byte{seedB: refusedEntity(t, seedB)}}
		got, err := newPoisonClient(t, w).Entities(t.Context(), []string{seedA, seedB, seedC})
		require.Error(t, err)
		assert.True(t, graph.IsStateContractError(err), "the client's refusal carries the reset code too")
		assertNoProjection(t, got)
		assert.Equal(t, []string{seedA, seedB}, w.entityIDs)
	})
}

// TestBatchFaultNotReadAlone holds that a batch error other than the poison
// refusal is returned and starts no per-ID read.
func TestBatchFaultNotReadAlone(t *testing.T) {
	for name, fault := range map[string]error{
		"transient": errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeInternal, errors.New("boom")),
		"invalid":   errs.ClassifiedCode(errs.ErrorInvalid, graph.ErrorCodeInvalidRequest, errors.New("bad request")),
	} {
		t.Run(name, func(t *testing.T) {
			w := &poisonWire{batchErr: fault}
			got, err := newPoisonClient(t, w).Entities(t.Context(), []string{seedA, seedB})
			require.ErrorIs(t, err, fault)
			assertNoProjection(t, got)
			assert.Empty(t, w.entityIDs, "only the poison refusal starts the per-ID read")
		})
	}
}
