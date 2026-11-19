//go:build integration

package graphembedding

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/pkg/errs"
)

// floorOrRefusal is what awaitPast observes: the follower's floor and its QueryReady answer.
type floorOrRefusal struct {
	indexed uint64
	refusal error
}

// awaitPast waits until the follower's floor reaches rev: rev, with every revision before it, is
// complete, so whatever hop 1 does before completing it is done. A refusal from QueryReady that
// is not transient ends the wait too, since no later revision will lift it; the assertions after
// the wait then read that refusal.
func (h *harness) awaitPast(t *testing.T, what string, rev uint64) {
	t.Helper()
	await(t, what, func(ctx context.Context) (floorOrRefusal, error) {
		return floorOrRefusal{indexed: h.f.Indexed(), refusal: h.f.QueryReady(ctx)}, nil
	}, func(o floorOrRefusal) bool {
		return o.indexed >= rev || (o.refusal != nil && !errs.IsTransient(o.refusal))
	})
}

// requireDamagedLeftOut requires the replies a running graph-embedding gives while b is damaged
// and a is embedded: search answers a alone counting b, a search scoped away from b counts
// nothing, similar to a answers counting b, and similar to b is refused with the recorded refusal.
// The caller has waited for the vector cache to hold a and not b.
func requireDamagedLeftOut(t *testing.T, h *harness, a, b string) {
	t.Helper()
	search := answerNow[graph.SemanticData](t, h, "search", `{"query":"gadget"}`)
	if got := ids(search.reply.Data.Results); !slices.Equal(got, []string{a}) || search.reply.DamagedEntities != 1 {
		t.Errorf("search gadget = %s; want only %s with damaged_entities 1", search.raw, a)
	}
	scoped := answerNow[graph.SemanticData](t, h, "search", `{"query":"gadget","scope":["`+scopeOf(a)+`"]}`)
	if got := ids(scoped.reply.Data.Results); !slices.Equal(got, []string{a}) || scoped.reply.DamagedEntities != 0 {
		t.Errorf("search gadget in %s = %s; want only %s with no damaged_entities", scopeOf(a), scoped.raw, a)
	}
	similar := answerNow[graph.SimilarData](t, h, "similar", `{"entity_id":"`+a+`"}`)
	if len(similar.reply.Data.Results) != 0 || similar.reply.DamagedEntities != 1 {
		t.Errorf("similar to %s = %s; want no hit with damaged_entities 1", a, similar.raw)
	}
	raw, err := h.ask(t, "similar", `{"entity_id":"`+b+`"}`)
	requireRefused(t, "similar to the damaged "+b, raw, err, errs.ErrorFatal, graph.ErrorCodeGraphStateResetRequired)
	if err != nil && !strings.Contains(err.Error(), string(graph.GraphStateReasonUnreadableEntity)) {
		t.Errorf("similar to the damaged %s refused %v; want the recorded refusal, %s", b, err, graph.GraphStateReasonUnreadableEntity)
	}
	if health := h.c.Health(); health.Healthy || !strings.Contains(health.LastError, b) {
		t.Errorf("Health with %s damaged = %+v; want not healthy, naming it", b, health)
	}
}

// scopeOf is the five-part prefix of a six-part entity ID.
func scopeOf(id string) string { return id[:strings.LastIndex(id, ".")] }

// TestDamagedEntityLeftOutOfSearch: B's stored value does not decode. graph-embedding keeps
// answering both verbs from the other vectors, counting B in damaged_entities (within a search's
// scope), and refuses similar to B with the refusal the follower recorded (graph-embedding spec,
// "A damaged entity is left out and counted", "Similar to a damaged entity is refused"). B's record
// is removed, so it is no hit, and a valid write of B makes it a hit again with no restart.
//
// Each reply is read after two waits: the follower's floor past B's revision, which hop 1 reaches
// only after reporting B and removing its record, then the vector cache answering without B. The
// replies claim no ENTITY_STATES revision, so the floor is the only way to order a reply after hop 1.
func TestDamagedEntityLeftOutOfSearch(t *testing.T) {
	damaged := []byte("{not json")
	a := entityID(1)
	b := fmt.Sprintf("acme.ops.test.other.doc.%03d", 2) // under another five-part prefix than a

	t.Run("stored before start, then repaired", func(t *testing.T) {
		client := fixtureClient(t)
		var damagedRev uint64
		h := startAfter(t, client, nil, nil, func(h *harness) {
			h.titled(t, "gadget", a)
			damagedRev = h.put(t, b, damaged)
		})
		h.awaitPast(t, "the replay passed the damaged revision", damagedRev)
		awaitAnswer(t, h, "search", `{"query":"gadget"}`, hitCount[graph.SemanticData](1))
		requireDamagedLeftOut(t, h, a, b)
		if got := h.c.Health().ErrorCount; got != 1 {
			t.Errorf("ErrorCount with one damaged entity = %d; want 1", got)
		}

		repaired := h.put(t, b, entityValue(t, b, nil, "doc.meta.title", "gadget"))
		// B's revision completes when the Worker has written its vector, which clears the record.
		h.awaitPast(t, "the repair completed", repaired)
		got := awaitAnswer(t, h, "search", `{"query":"gadget"}`, hitCount[graph.SemanticData](2))
		if got.reply.DamagedEntities != 0 {
			t.Errorf("search after the repair = %s; want no damaged_entities", got.raw)
		}
		if health := h.c.Health(); !health.Healthy {
			t.Errorf("Health after the repair = %+v; want healthy", health)
		}
	})

	t.Run("overwritten after start", func(t *testing.T) {
		h := startEmbedding(t, fixtureClient(t), `{}`, nil)
		h.titled(t, "gadget", a, b)
		awaitAnswer(t, h, "search", `{"query":"gadget"}`, hitCount[graph.SemanticData](2))

		h.awaitPast(t, "hop 1 passed the damaged revision", h.put(t, b, damaged))
		// The vector cache applies the record's delete after hop 1 made it.
		awaitAnswer(t, h, "search", `{"query":"gadget"}`, func(r graph.QueryResponse[graph.SemanticData]) bool {
			return slices.Equal(ids(r.Data.Results), []string{a})
		})
		requireDamagedLeftOut(t, h, a, b)
		if record, err := h.record(t.Context(), b); err != nil || record != nil {
			t.Errorf("%s's EMBEDDING_INDEX record = %+v, %v; want none", b, record, err)
		}
	})
}
