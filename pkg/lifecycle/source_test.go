package lifecycle

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
)

// TestManagerWritesUnderTheLifecycleSource: every statement the manager sends names
// graph.SourceLifecycle, whichever TransitionSource caused the write, and every reconcile sends
// it as the request's source (design D15). The writes cover the create request and the three
// reconcile builders: a birth, two transitions caused by different TransitionSources, the attach
// to an entity that exists without a phase, and an operator patch. The end-to-end form, that the
// entity then holds one phase statement, is TestTransitionReplacesItsPhaseStatement, which runs
// through graph-ingest (task 3.12).
func TestManagerWritesUnderTheLifecycleSource(t *testing.T) {
	t.Parallel()
	mgr, emitter, bucket := newTestManager(t)
	ctx := context.Background()

	const born = "c360.platform1.gcs.lifecycle.mission.source-born"
	if err := mgr.Create(ctx, &fixtureMission{ID: born, PhaseF: "planning", OwnerOrgID: "acme"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := mgr.Transition(ctx, "fixture", born, "flying", TransitionSourceRule, "launched"); err != nil {
		t.Fatalf("Transition by a rule: %v", err)
	}
	if err := mgr.Transition(ctx, "fixture", born, "aborted", TransitionSourceOperator, "recalled"); err != nil {
		t.Fatalf("Transition by an operator: %v", err)
	}

	// An entity another producer wrote, with no phase yet: Create attaches through a reconcile.
	const attached = "c360.platform1.gcs.lifecycle.mission.source-attached"
	bucket.put(attached, &graph.EntityState{ID: attached, Triples: []message.Triple{{
		Subject: attached, Predicate: "mission.identity.owner-org-id", Object: "acme", Source: "another-producer",
	}}})
	if err := mgr.Create(ctx, &fixtureMission{ID: attached, PhaseF: "planning"}); err != nil {
		t.Fatalf("Create on an existing entity: %v", err)
	}
	if err := mgr.UpdateFromOperator(ctx, "fixture", attached, map[string]any{"owner_org_id": "globex"}); err != nil {
		t.Fatalf("UpdateFromOperator: %v", err)
	}

	emitter.mu.Lock()
	creates := append([]*graph.CreateEntityRequest(nil), emitter.creates...)
	reconciles := append([]*graph.ReconcilePredicatesRequest(nil), emitter.requests...)
	emitter.mu.Unlock()

	if len(creates) != 1 || len(reconciles) != 4 {
		t.Fatalf("requests sent: %d creates and %d reconciles, want 1 and 4", len(creates), len(reconciles))
	}
	requireLifecycleSource(t, "create "+creates[0].Entity.ID, creates[0].Triples)
	for i, request := range reconciles {
		name := "reconcile " + request.EntityID
		if request.Source != graph.SourceLifecycle {
			t.Errorf("%s (request %d): request source = %q, want %q", name, i, request.Source, graph.SourceLifecycle)
		}
		requireLifecycleSource(t, name, request.Desired)
	}
}

func requireLifecycleSource(t *testing.T, request string, statements []message.Triple) {
	t.Helper()
	if len(statements) == 0 {
		t.Errorf("%s: no statements, want at least the one it writes", request)
	}
	var others []string
	for _, statement := range statements {
		if statement.Source != graph.SourceLifecycle {
			others = append(others, fmt.Sprintf("%s=%q", statement.Predicate, statement.Source))
		}
	}
	if len(others) > 0 {
		t.Errorf("%s: %d of %d statements have a source other than %q: %s",
			request, len(others), len(statements), graph.SourceLifecycle, strings.Join(others, ", "))
	}
}
