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

// Seeds for the damaged-seed tests: literal six-part IDs.
const (
	seedA = "acme.ops.test.system.widget.a"
	seedB = "acme.ops.test.system.widget.b"
	seedC = "acme.ops.test.system.widget.c"
)

// resetRefusal is the refusal a responder sends for a stored entity it will not
// serve, as ClassifyReply rebuilds it from the reply's headers.
func resetRefusal() error {
	return errs.ClassifiedCode(errs.ErrorFatal, graph.ErrorCodeGraphStateResetRequired, errors.New("stored entity refused"))
}

// damageWire fakes graph-query's wire. The batch returns batchErr, or
// batchReply when batchErr is nil, and the prefix ID listing returns
// prefixReply. It records the ID of every entity request, which Entities must
// never send.
type damageWire struct {
	t           *testing.T
	batchErr    error
	batchReply  []byte
	prefixReply []byte
	entityIDs   []string
}

func (w *damageWire) RequestClassified(_ context.Context, subject string, data []byte, _ time.Duration) ([]byte, error) {
	switch subject {
	case subjectPrefixIDs:
		return w.prefixReply, nil
	case subjectBatch:
		return w.batchReply, w.batchErr
	case subjectEntity:
		var req graph.EntityRequest
		require.NoError(w.t, json.Unmarshal(data, &req))
		w.entityIDs = append(w.entityIDs, req.ID)
		return nil, errors.New("Entities sends no entity request")
	default:
		w.t.Fatalf("unexpected subject %q", subject)
		return nil, nil
	}
}

func newDamageClient(t *testing.T, w *damageWire) *Client {
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

// batchNaming is a batch reply returning entities in the order given and naming
// each of damaged `damaged`.
func batchNaming(t *testing.T, entities []string, damaged ...string) []byte {
	t.Helper()
	reply := graph.EntityBatchResponse{Entities: []graph.EntityState{}}
	for _, id := range entities {
		reply.Entities = append(reply.Entities, graph.EntityState{ID: id, Triples: []message.Triple{stamped(id, "dc.terms.title", id)}})
	}
	for _, id := range damaged {
		reply.Missing = append(reply.Missing, graph.MissingEntity{ID: id, Reason: graph.MissingDamaged})
	}
	return mustJSON(t, reply)
}

// TestDamagedSeedUnhydrated holds that a seed the batch names damaged (the
// second of three) is listed unhydrated error, the other two hydrate in request
// order though the reply returns them reversed, and the batch is the one
// request.
func TestDamagedSeedUnhydrated(t *testing.T) {
	w := &damageWire{batchReply: batchNaming(t, []string{seedC, seedA}, seedB)}
	got, err := newDamageClient(t, w).Entities(t.Context(), []string{seedA, seedB, seedC})
	require.NoError(t, err)

	assert.Equal(t, []string{seedA, seedC}, entityIDs(got), "the good seeds hydrate in request order")
	assert.Equal(t, []fusion.Unhydrated{{Handle: seedB, Reason: fusion.UnhydratedError}}, got.Unhydrated)
	assert.Empty(t, w.entityIDs, "the batch is the one request")
}

// TestDamagedDuplicateListedOnce holds that a damaged seed requested twice is
// listed once.
func TestDamagedDuplicateListedOnce(t *testing.T) {
	w := &damageWire{batchReply: batchNaming(t, []string{seedA}, seedB, seedB)}
	got, err := newDamageClient(t, w).Entities(t.Context(), []string{seedB, seedA, seedB})
	require.NoError(t, err)

	assert.Equal(t, []string{seedA}, entityIDs(got))
	assert.Equal(t, []fusion.Unhydrated{{Handle: seedB, Reason: fusion.UnhydratedError}}, got.Unhydrated)
	assert.Empty(t, w.entityIDs)
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

// TestAllSeedsDamagedDefer holds that when the batch names every seed damaged
// the engine over fusionnats answers deferred all_seeds_unhydrated, each seed
// listed error, rather than failing the call.
func TestAllSeedsDamagedDefer(t *testing.T) {
	w := &damageWire{
		prefixReply: mustJSON(t, graph.PrefixIDsResponse{IDs: []string{seedA, seedB}, Total: 2}),
		batchReply:  batchNaming(t, nil, seedA, seedB),
	}
	eng, err := fusion.NewEngine(newDamageClient(t, w), fusion.NewBodyResolver(fusion.MapStoreResolver{}), slog.New(slog.DiscardHandler))
	require.NoError(t, err)

	resp, err := eng.Fuse(t.Context(), fusion.Request{Query: "acme.ops"}, prefixLens{})
	require.NoError(t, err, "a damaged seed is listed, never a failed answer")
	assert.Empty(t, resp.Nodes)
	assert.Empty(t, resp.Misses)
	assert.True(t, resp.Deferred)
	assert.Equal(t, "all_seeds_unhydrated", resp.DeferReason)
	assert.Equal(t, []fusion.Unhydrated{
		{Handle: seedA, Reason: fusion.UnhydratedError},
		{Handle: seedB, Reason: fusion.UnhydratedError},
	}, resp.Unhydrated)
	assert.Empty(t, w.entityIDs)
}

// TestOwnValidationNotRetried holds that the client's own refusal of the batch
// reply, which carries the same reset code as the responder's, is returned and
// sends no entity request.
func TestOwnValidationNotRetried(t *testing.T) {
	w := &damageWire{batchReply: mustJSON(t, graph.EntityBatchResponse{Entities: []graph.EntityState{{
		ID: seedA, Triples: []message.Triple{stamped("bad", "dc.terms.title", "x")},
	}}})}
	got, err := newDamageClient(t, w).Entities(t.Context(), []string{seedA, seedB})
	require.Error(t, err)
	assert.True(t, graph.IsStateContractError(err), "the client's refusal carries the reset code too")
	assertNoProjection(t, got)
	assert.Empty(t, w.entityIDs, "a refused batch reply sends no entity request")
}

// TestBatchRefusalNotReadAlone holds that a refused batch, whatever its class
// or code, is returned and sends no entity request.
func TestBatchRefusalNotReadAlone(t *testing.T) {
	for name, refusal := range map[string]error{
		"reset":     resetRefusal(),
		"transient": errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeInternal, errors.New("boom")),
		"invalid":   errs.ClassifiedCode(errs.ErrorInvalid, graph.ErrorCodeInvalidRequest, errors.New("bad request")),
	} {
		t.Run(name, func(t *testing.T) {
			w := &damageWire{batchErr: refusal}
			got, err := newDamageClient(t, w).Entities(t.Context(), []string{seedA, seedB})
			require.ErrorIs(t, err, refusal)
			assertNoProjection(t, got)
			assert.Empty(t, w.entityIDs, "a refused batch sends no entity request")
		})
	}
}
