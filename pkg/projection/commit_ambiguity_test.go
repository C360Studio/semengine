package projection

import (
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/pkg/errs"
)

// These tests are the typed client's half of projection-mutation's "Commit ambiguity is preserved"
// (task 4.1, #20). graph-ingest refuses an invalid request, a revision conflict, an entity not
// found or present, and a stored value it cannot read before it writes; any other classified reply
// can follow a write whose outcome the bucket did not confirm. The wire is a double, so each
// reply is the one the test names; processor/graph-ingest's
// TestIntegration_TypedClientPreservesCommitAmbiguity drives the same path over a broker.

type writeOutcome struct {
	receipt MutationReceipt
	err     error
}

// eachWrite sends one Create, Reconcile, Append and Delete through a client whose wire answers
// every mutation with fail. Reconcile's exact read is answered with the entity at revision 7.
func eachWrite(t *testing.T, fail error) map[MutationOperation]writeOutcome {
	t.Helper()
	contract := projectionTestContract(t)
	requester := &projectionRequester{handle: func(subject string, _ []byte) ([]byte, error) {
		if subject == "graph.ingest.query.entity" {
			return projectionJSON(t, graph.ExactEntity{Entity: projectionTestState(), KVRevision: 7}), nil
		}
		return nil, fail
	}}
	client, err := newMutationClient(requester, []Contract{contract}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	metadata := MutationMetadata{RequestID: "commit-ambiguity", Source: "test-projector", Timestamp: projectionTestTime}
	outcomes := make(map[MutationOperation]writeOutcome, 4)
	receipt, err := client.Create(ctx, CreateMutation{
		Contract: contract.Name, Entity: projectionTestState(), Metadata: metadata,
		Triples: []message.Triple{{Subject: projectionTestEntity, Predicate: "test.identity.name", Object: "widget"}},
	})
	outcomes[MutationOperationCreate] = writeOutcome{receipt, err}
	receipt, err = client.Reconcile(ctx, ReconcileMutation{
		Contract: contract.Name, Group: "state", EntityID: projectionTestEntity, Metadata: metadata,
		Desired: []message.Triple{{Subject: projectionTestEntity, Predicate: "test.value.name", Object: "new"}},
	})
	outcomes[MutationOperationReconcile] = writeOutcome{receipt, err}
	receipt, err = client.Append(ctx, AppendMutation{
		Contract: contract.Name, Group: "events", EntityID: projectionTestEntity, Metadata: metadata,
		Triples: []message.Triple{{Subject: projectionTestEntity, Predicate: "test.event.seen", Object: "one"}},
	})
	outcomes[MutationOperationAppend] = writeOutcome{receipt, err}
	receipt, err = client.Delete(ctx, DeleteMutation{EntityID: projectionTestEntity, ExpectedRevision: 7, Metadata: metadata})
	outcomes[MutationOperationDelete] = writeOutcome{receipt, err}
	return outcomes
}

func TestClassifiedReplyIsNotCommittedOnlyWhenRefusedBeforeWrite(t *testing.T) {
	tests := []struct {
		name  string
		class errs.ErrorClass
		code  string
		want  CommitState
		kind  MutationErrorKind
	}{
		{"transient internal", errs.ErrorTransient, graph.ErrorCodeInternal, CommitUnknown, MutationCommitUnknown},
		{"transient without code", errs.ErrorTransient, "", CommitUnknown, MutationCommitUnknown},
		{"fatal without code", errs.ErrorFatal, "", CommitUnknown, MutationCommitUnknown},
		{"invalid request", errs.ErrorInvalid, graph.ErrorCodeInvalidRequest, CommitNotCommitted, MutationInvalid},
		{"structural invalid", errs.ErrorInvalid, graph.ErrorCodeStructuralInvalid, CommitNotCommitted, MutationInvalid},
		{"message type unregistered", errs.ErrorInvalid, graph.ErrorCodeMessageTypeUnregistered, CommitNotCommitted, MutationInvalid},
		{"revision mismatch", errs.ErrorInvalid, graph.ErrorCodeRevisionMismatch, CommitNotCommitted, MutationRevisionConflict},
		{"entity not found", errs.ErrorInvalid, graph.ErrorCodeEntityNotFound, CommitNotCommitted, MutationNotFound},
		{"entity already exists", errs.ErrorInvalid, graph.ErrorCodeEntityExists, CommitNotCommitted, MutationConflict},
		{"stored value refused", errs.ErrorFatal, graph.ErrorCodeGraphStateResetRequired, CommitNotCommitted, MutationInternal},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reply := errs.ClassifiedCode(test.class, test.code, errors.New("graph-ingest reply"))
			for operation, outcome := range eachWrite(t, reply) {
				var mutationErr *MutationError
				if !errors.As(outcome.err, &mutationErr) {
					t.Fatalf("%s: error = %v, want a *MutationError", operation, outcome.err)
				}
				if outcome.receipt.Commit != test.want || mutationErr.Commit != test.want || mutationErr.Kind != test.kind {
					t.Errorf("%s: receipt commit %q, error commit %q, kind %q; want %q, %q, %q",
						operation, outcome.receipt.Commit, mutationErr.Commit, mutationErr.Kind, test.want, test.want, test.kind)
				}
				if mutationErr.Class != test.class || mutationErr.Code != test.code {
					t.Errorf("%s: class %q, code %q; want the reply's %q, %q",
						operation, mutationErr.Class, mutationErr.Code, test.class, test.code)
				}
			}
		})
	}
}

func TestTransportOutcomesKeepTheirCommitState(t *testing.T) {
	tests := []struct {
		name string
		fail error
		want CommitState
		kind MutationErrorKind
	}{
		{"no responders", nats.ErrNoResponders, CommitNotCommitted, MutationUnavailable},
		{"timeout after possible delivery", nats.ErrTimeout, CommitUnknown, MutationCommitUnknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for operation, outcome := range eachWrite(t, test.fail) {
				var mutationErr *MutationError
				if !errors.As(outcome.err, &mutationErr) || outcome.receipt.Commit != test.want ||
					mutationErr.Commit != test.want || mutationErr.Kind != test.kind {
					t.Errorf("%s: receipt = %#v, error = %#v; want commit %q, kind %q",
						operation, outcome.receipt, mutationErr, test.want, test.kind)
				}
			}
		})
	}
}

// An append reports each subject's failure in its response body, not as a classified reply; the
// same rule holds for it.
func TestAppendSubjectFailureAfterPossibleWriteIsCommitUnknown(t *testing.T) {
	contract := projectionTestContract(t)
	requester := &projectionRequester{handle: func(string, []byte) ([]byte, error) {
		return projectionJSON(t, graph.AppendTriplesResponse{Results: []graph.AppendSubjectResult{{
			EntityID: projectionTestEntity,
			Outcome:  graph.MutationFailed,
			Error:    &graph.MutationFailure{Class: errs.ErrorTransient.String(), Code: graph.ErrorCodeInternal},
		}}}), nil
	}}
	client, err := newMutationClient(requester, []Contract{contract}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := client.Append(t.Context(), AppendMutation{
		Contract: contract.Name, Group: "events", EntityID: projectionTestEntity,
		Triples:  []message.Triple{{Subject: projectionTestEntity, Predicate: "test.event.seen", Object: "one"}},
		Metadata: MutationMetadata{RequestID: "append-unconfirmed", Source: "test-projector", Timestamp: projectionTestTime},
	})
	var mutationErr *MutationError
	if !errors.As(err, &mutationErr) || receipt.Commit != CommitUnknown || mutationErr.Commit != CommitUnknown ||
		mutationErr.Kind != MutationCommitUnknown || mutationErr.Code != graph.ErrorCodeInternal {
		t.Fatalf("receipt = %#v, error = %#v", receipt, mutationErr)
	}
}

// A read writes nothing, so a classified reply to Reconcile's exact read stays not committed.
func TestReconcileReadFailureIsNotCommitted(t *testing.T) {
	contract := projectionTestContract(t)
	requester := &projectionRequester{handle: func(subject string, _ []byte) ([]byte, error) {
		if subject != "graph.ingest.query.entity" {
			t.Fatalf("subject = %q; a failed read sends no mutation", subject)
		}
		return nil, errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeInternal, errors.New("kv get failed"))
	}}
	client, err := newMutationClient(requester, []Contract{contract}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := client.Reconcile(t.Context(), ReconcileMutation{
		Contract: contract.Name, Group: "state", EntityID: projectionTestEntity,
		Desired:  []message.Triple{{Subject: projectionTestEntity, Predicate: "test.value.name", Object: "new"}},
		Metadata: MutationMetadata{Source: "test-projector", Timestamp: projectionTestTime},
	})
	var mutationErr *MutationError
	if !errors.As(err, &mutationErr) || receipt.Commit != CommitNotCommitted || mutationErr.Commit != CommitNotCommitted ||
		mutationErr.Operation != MutationOperationReadAuthoritative {
		t.Fatalf("receipt = %#v, error = %#v", receipt, mutationErr)
	}
}
