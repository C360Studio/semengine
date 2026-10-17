package projection

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/pkg/errs"
)

// reconcileServer is a test-owned model of graph-ingest's reconcile at a caller's revision, taken
// from the projection-mutation requirement "Conditional reconcile at a caller-observed revision":
// it holds one entity at a revision, answers an exact read with that revision, applies a reconcile
// only at that revision, and otherwise refuses with a revision conflict naming the expected and
// the current revision. It records the expected revision of every reconcile it receives.
type reconcileServer struct {
	t        *testing.T
	revision uint64
	expected []uint64
}

func (s *reconcileServer) handle(subject string, payload []byte) ([]byte, error) {
	switch subject {
	case "graph.ingest.query.entity":
		return projectionJSON(s.t, graph.ExactEntity{Entity: projectionTestState(), KVRevision: s.revision}), nil
	case "graph.mutation.entity.reconcile":
		var request graph.ReconcilePredicatesRequest
		if err := json.Unmarshal(payload, &request); err != nil {
			s.t.Fatal(err)
		}
		s.expected = append(s.expected, request.ExpectedRevision)
		if request.ExpectedRevision != s.revision {
			return nil, errs.ClassifiedCodeDetail(errs.ErrorInvalid, graph.ErrorCodeRevisionMismatch,
				map[string]any{"expected_revision": request.ExpectedRevision, "current_revision": s.revision},
				errors.New("revision mismatch"))
		}
		s.revision++
		return projectionJSON(s.t, graph.EntityMutationResponse{
			Outcome: graph.MutationApplied, Entity: projectionTestState(), KVRevision: s.revision,
		}), nil
	default:
		s.t.Fatalf("unexpected subject %q", subject)
		return nil, nil
	}
}

func newReconcileServerClient(t *testing.T, revision uint64) (*MutationClient, *reconcileServer, Contract) {
	t.Helper()
	contract := projectionTestContract(t)
	server := &reconcileServer{t: t, revision: revision}
	client, err := newMutationClient(&projectionRequester{handle: server.handle}, []Contract{contract}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return client, server, contract
}

func stateReconcile(contract Contract, expectedRevision uint64) ReconcileMutation {
	return ReconcileMutation{
		Contract: contract.Name, Group: "state", EntityID: projectionTestEntity, ExpectedRevision: expectedRevision,
		Desired:  []message.Triple{{Subject: projectionTestEntity, Predicate: "test.value.name", Object: "new"}},
		Metadata: MutationMetadata{Source: "test-projector", Timestamp: projectionTestTime},
	}
}

// TestReconcileAtAStaleRevisionIsARevisionConflict is the scenario "Concurrent update between read
// and reconcile": the caller read revision R, another writer committed R+1, and the caller
// reconciles at R. The request carries R, the caller's revision, not the R+1 the client's own read
// returns; the result is a revision conflict naming R and R+1, not committed, and nothing applied.
func TestReconcileAtAStaleRevisionIsARevisionConflict(t *testing.T) {
	const read = uint64(7)
	client, server, contract := newReconcileServerClient(t, read)
	exact, err := client.ReadAuthoritative(context.Background(), projectionTestEntity)
	if err != nil || exact.KVRevision != read {
		t.Fatalf("caller's read = %#v, %v; want revision %d", exact, err, read)
	}
	server.revision = read + 1 // another writer commits R+1

	receipt, err := client.Reconcile(context.Background(), stateReconcile(contract, exact.KVRevision))

	var mutationErr *MutationError
	if !errors.As(err, &mutationErr) || mutationErr.Kind != MutationRevisionConflict ||
		mutationErr.Code != graph.ErrorCodeRevisionMismatch || mutationErr.Commit != CommitNotCommitted ||
		receipt != (MutationReceipt{}) {
		t.Fatalf("receipt = %#v, error = %v; want a not-committed revision conflict", receipt, err)
	}
	if got, want := mutationErr.Detail["expected_revision"], read; got != want {
		t.Fatalf("expected_revision = %v, want %d", got, want)
	}
	if got, want := mutationErr.Detail["current_revision"], read+1; got != want {
		t.Fatalf("current_revision = %v, want %d", got, want)
	}
	if len(server.expected) != 1 || server.expected[0] != read || server.revision != read+1 {
		t.Fatalf("reconciles sent at %v, server at %d; want one at %d and nothing applied past %d",
			server.expected, server.revision, read, read+1)
	}
}

// TestReconcileAtTheCurrentRevisionIsVerifiedAtTheNewRevision is the scenario "Unchanged
// revision": the caller reconciles at the current revision R, the request carries R, and the
// mutation is applied and verified at the new revision.
func TestReconcileAtTheCurrentRevisionIsVerifiedAtTheNewRevision(t *testing.T) {
	const read = uint64(7)
	client, server, contract := newReconcileServerClient(t, read)
	exact, err := client.ReadAuthoritative(context.Background(), projectionTestEntity)
	if err != nil || exact.KVRevision != read {
		t.Fatalf("caller's read = %#v, %v; want revision %d", exact, err, read)
	}

	receipt, err := client.Reconcile(context.Background(), stateReconcile(contract, exact.KVRevision))
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if receipt.KVRevision != read+1 || receipt.Entity == nil {
		t.Fatalf("receipt = %#v; want the entity at revision %d", receipt, read+1)
	}
	if len(server.expected) != 1 || server.expected[0] != read {
		t.Fatalf("reconciles sent at %v; want one at %d", server.expected, read)
	}
}
