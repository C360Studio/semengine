//go:build integration

package graphembedding

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/embedding"
	"github.com/c360studio/semengine/natsclient"
)

// TestHealthIgnoresErrorCount: ErrorCount counts once per entity entering the failed set and once
// per latched reset, as graph-index counts (processor/graph-index/index.go:240-249), and never
// lowers Healthy (design D6; ADR-109 Decision 2). The pin's Health was not healthy while its error
// count was above zero (pin component.go:567). Here a failed entity read lowers Healthy, naming the
// unresolved write, and counts once however often the repair lane fails it again; once the repair
// lane converges the entity Health reads running and healthy with the failure still counted. A
// Worker failure counts once on entry, not again while the entity is failed, not when it is older
// than the entity's latest outcome, and once more when the entity fails again after a success.
func TestHealthIgnoresErrorCount(t *testing.T) {
	client := fixtureClient(t)
	h := startEmbedding(t, client, `{}`, nil)
	a, gone := entityID(1), entityID(2)
	h.awaitGenerated(t, a, h.put(t, a, entityValue(t, a, nil, "doc.meta.title", "first")))
	if health := h.c.Health(); !health.Healthy || health.ErrorCount != 0 {
		t.Fatalf("Health before any failure = %+v, want healthy with ErrorCount 0", health)
	}

	editStream(t, client, graph.BucketEntityStates, directGets(false))
	rev := h.put(t, a, entityValue(t, a, nil, "doc.meta.title", "second"))
	// Hop 1 marks and counts the read failure before it drains the revision, so the floor reaching
	// rev orders the Health read after both.
	h.awaitIndexed(t, "the unread revision drained", rev)
	failing := h.c.Health()
	if failing.Healthy || failing.Status != "degraded" || failing.ErrorCount != 1 ||
		!strings.Contains(failing.LastError, "1 index writes unresolved") {
		t.Fatalf("Health with a failed read = %+v, want degraded naming 1 unresolved write, ErrorCount 1",
			failing)
	}
	h.c.repair(t.Context()) // the read fails again: the entity is already failed
	if health := h.c.Health(); health.ErrorCount != 1 {
		t.Fatalf("ErrorCount after a repair pass that fails again = %d, want 1", health.ErrorCount)
	}

	editStream(t, client, graph.BucketEntityStates, directGets(true))
	h.c.repair(t.Context()) // the read answers: the pending record is written at rev
	h.assertFailed(t, "after the repair pass", nil)
	repaired := h.c.Health()
	if !repaired.Healthy || repaired.Status != "running" || repaired.ErrorCount != 1 {
		t.Fatalf("Health after the repair = %+v, want running and healthy with ErrorCount 1", repaired)
	}

	// The Worker's outcomes, through its production callback, each with the count it leaves.
	for _, step := range []struct {
		what     string
		revision uint64
		outcome  embedding.TerminalOutcome
		want     int
	}{
		{"a failure enters the failed set", 5, embedding.OutcomeFailed, 2},
		{"a newer failure of the failed entity", 6, embedding.OutcomeFailed, 2},
		{"a success clears it", 7, embedding.OutcomeGenerated, 2},
		{"a failure older than the success is refused", 6, embedding.OutcomeFailed, 2},
		{"a newer failure enters again", 8, embedding.OutcomeFailed, 3},
	} {
		h.c.onTerminal(t.Context(), gone, step.revision, step.outcome, "timeout")
		if health := h.c.Health(); health.ErrorCount != step.want {
			t.Fatalf("ErrorCount after %s = %d, want %d", step.what, health.ErrorCount, step.want)
		}
	}

	// A latched reset counts once: authoritative state the reader refuses. The count comes before
	// the latch, so Health naming the reset orders the read after it.
	h.put(t, entityID(3), []byte("not an entity state"))
	reset := await(t, "the reset latched", func(context.Context) (component.HealthStatus, error) {
		return h.c.Health(), nil
	}, func(health component.HealthStatus) bool {
		return strings.Contains(health.LastError, "graph state reset required")
	})
	if reset.ErrorCount != 4 {
		t.Errorf("ErrorCount after the latched reset = %d, want 4", reset.ErrorCount)
	}
}

// statusSubject is the pin's status verb (pin query.go:39), spelled here only to show nothing
// answers it: the verb is not carried (design D6; ADR-083), and a reader reads graph-embedding's
// GRAPH_STATUS key instead.
const statusSubject = "graph.embedding.query.status"

// requestBound is out of reach: no responder answers at once on nats-server's default
// configuration (natsclient.IsNoResponders), and go test -timeout is the deadline.
const requestBound = time.Hour

// TestNoStatusVerb: the verb table declares no status verb for graph-embedding, and a running
// graph-embedding answers nothing on the pin's status subject (design D6; ADR-083, ADR-109 D3).
func TestNoStatusVerb(t *testing.T) {
	for _, verb := range graph.QueryVerbs() {
		if verb.Responder == graph.ResponderGraphEmbedding && (verb.Name == "status" || verb.Subject == statusSubject) {
			t.Errorf("graph.QueryVerbs declares %+v; the status verb is not carried", verb)
		}
	}

	client := fixtureClient(t)
	h := startEmbedding(t, client, `{}`, nil)
	a := entityID(1)
	// Start has returned and the Worker has generated a record: the component is running.
	h.awaitGenerated(t, a, h.put(t, a, entityValue(t, a, nil, "doc.meta.title", "first")))
	reply, err := client.RequestClassified(t.Context(), statusSubject, []byte(`{}`), requestBound)
	if !natsclient.IsNoResponders(err) {
		t.Fatalf("request on %s = %q, %v; want no responders", statusSubject, reply, err)
	}
}
