package projection

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	semtypes "github.com/c360studio/semengine/pkg/types"
)

// These tests are the typed client's half of projection-mutation's "Commit ambiguity is preserved"
// (task 4.1, #20; task 4.16). graph-ingest refuses an invalid request, a revision conflict, an
// entity not found or present, and a stored value it cannot read before it writes, each with its
// own code; any other classified reply, an invalid one included, can follow a write whose outcome
// the bucket did not confirm. The wire is a double, so each reply is the one the test names;
// processor/graph-ingest's TestIntegration_TypedClientPreservesCommitAmbiguity and
// TestIntegration_OversizedResponseAfterCommitIsCommitUnknown drive the same path over a broker.

// responseTooLarge is the code of natsclient's reply in place of a success response larger than
// the broker's payload limit (errorCodeResponseTooLarge, natsclient/request.go), spelled as the
// wire carries it. The reply is class invalid and follows the handler's commit.
const responseTooLarge = "response_too_large"

type writeOutcome struct {
	receipt MutationReceipt
	err     error
}

// eachWrite sends one Create, Reconcile, Append and Delete through a client whose wire answers
// every mutation with fail. Reconcile's exact read is answered with the entity at revision 7.
func eachWrite(t *testing.T, fail error) map[MutationOperation]writeOutcome {
	t.Helper()
	outcomes, _ := eachWriteWithContext(t.Context(), t, fail)
	return outcomes
}

// eachWriteWithContext is eachWrite with the caller's context, and it returns the requester so a
// test can see which requests reached the wire.
func eachWriteWithContext(
	ctx context.Context, t *testing.T, fail error,
) (map[MutationOperation]writeOutcome, *projectionRequester) {
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
	return outcomes, requester
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
		{"entity ID invalid", errs.ErrorInvalid, semtypes.ErrorCodeEntityIDInvalid, CommitNotCommitted, MutationInvalid},
		{"entity ID authority refused", errs.ErrorInvalid, semtypes.ErrorCodeEntityIDAuthorityInvalid, CommitNotCommitted, MutationInvalid},
		{"revision mismatch", errs.ErrorInvalid, graph.ErrorCodeRevisionMismatch, CommitNotCommitted, MutationRevisionConflict},
		{"entity not found", errs.ErrorInvalid, graph.ErrorCodeEntityNotFound, CommitNotCommitted, MutationNotFound},
		{"entity already exists", errs.ErrorInvalid, graph.ErrorCodeEntityExists, CommitNotCommitted, MutationConflict},
		{"stored value refused", errs.ErrorFatal, graph.ErrorCodeGraphStateResetRequired, CommitNotCommitted, MutationInternal},
		// Class invalid, and none is a code graph-ingest sends for a refusal before its write.
		{"response too large after a commit", errs.ErrorInvalid, responseTooLarge, CommitUnknown, MutationCommitUnknown},
		{"invalid with an unfamiliar code", errs.ErrorInvalid, "unfamiliar_refusal", CommitUnknown, MutationCommitUnknown},
		{"invalid without code", errs.ErrorInvalid, "", CommitUnknown, MutationCommitUnknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reply := errs.ClassifiedCode(test.class, test.code, errors.New("graph-ingest reply"))
			if test.code != "" {
				// A reply's detail travels with its code (errs.Classified inherits both or neither).
				reply.Detail = map[string]any{"entity_id": projectionTestEntity}
			}
			// A commit-unknown error is fatal whatever the reply's class (design D2); a refusal
			// before the write keeps the reply's class.
			wantClass := test.class
			if test.want == CommitUnknown {
				wantClass = errs.ErrorFatal
			}
			for operation, outcome := range eachWrite(t, reply) {
				var mutationErr *MutationError
				if !errors.As(outcome.err, &mutationErr) {
					t.Fatalf("%s: error = %v, want a *MutationError", operation, outcome.err)
				}
				if outcome.receipt != (MutationReceipt{}) || mutationErr.Commit != test.want || mutationErr.Kind != test.kind {
					t.Errorf("%s: receipt %#v, error commit %q, kind %q; want a zero receipt, %q, %q",
						operation, outcome.receipt, mutationErr.Commit, mutationErr.Kind, test.want, test.kind)
				}
				if mutationErr.Class != wantClass || mutationErr.Code != test.code ||
					!reflect.DeepEqual(mutationErr.Detail, reply.Detail) {
					t.Errorf("%s: class %q, code %q, detail %v; want %q and the reply's %q, %v",
						operation, mutationErr.Class, mutationErr.Code, mutationErr.Detail, wantClass, test.code, reply.Detail)
				}
				var classified *errs.ClassifiedError
				if !errors.As(outcome.err, &classified) || classified.Class != wantClass || classified.Code != test.code ||
					!reflect.DeepEqual(classified.Detail, reply.Detail) {
					t.Errorf("%s: errors.As *errs.ClassifiedError = %#v; want class %q with the reply's code %q and detail %v",
						operation, classified, wantClass, test.code, reply.Detail)
				}
				if !errors.Is(outcome.err, reply) {
					t.Errorf("%s: errors.Is does not reach the reply: %v", operation, outcome.err)
				}
				if test.want == CommitUnknown {
					assertCommitUnknownIsFatal(t, operation, outcome.err)
				}
			}
		})
	}
}

// assertCommitUnknownIsFatal checks how errs reads a commit-unknown error: never retryable
// (design D2).
func assertCommitUnknownIsFatal(t *testing.T, operation MutationOperation, err error) {
	t.Helper()
	if errs.IsTransient(err) || !errs.IsFatal(err) || errs.Classify(err) != errs.ErrorFatal {
		t.Errorf("%s: IsTransient %t, IsFatal %t, Classify %v; want false, true, fatal (err=%v)",
			operation, errs.IsTransient(err), errs.IsFatal(err), errs.Classify(err), err)
	}
}

func TestTransportOutcomesKeepTheirCommitState(t *testing.T) {
	ended, cancel := context.WithCancel(t.Context())
	cancel()
	tests := []struct {
		name string
		// ctx is nil for the test's own context.
		ctx  context.Context
		fail error
		// is is the error errors.Is must reach: the wire's fail, or the context's cause.
		is   error
		want CommitState
		kind MutationErrorKind
	}{
		{"no responders", nil, nats.ErrNoResponders, nats.ErrNoResponders, CommitNotCommitted, MutationUnavailable},
		{"timeout after possible delivery", nil, nats.ErrTimeout, nats.ErrTimeout, CommitUnknown, MutationCommitUnknown},
		// Design D1: nothing was sent, so nothing was written and a retry is safe; errors.Is still
		// tells it apart from a missing responder. The wire's error must never be reached.
		{"context ended before send", ended, errors.New("the wire must not be reached"), context.Canceled,
			CommitNotCommitted, MutationUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := test.ctx
			if ctx == nil {
				ctx = t.Context()
			}
			outcomes, requester := eachWriteWithContext(ctx, t, test.fail)
			for operation, outcome := range outcomes {
				if test.ctx != nil && operation == MutationOperationReconcile {
					// Reconcile reads before it writes; its read is the other test's (inventory § 7, D-1).
					continue
				}
				var mutationErr *MutationError
				if !errors.As(outcome.err, &mutationErr) || outcome.receipt != (MutationReceipt{}) ||
					mutationErr.Commit != test.want || mutationErr.Kind != test.kind {
					t.Errorf("%s: receipt = %#v, error = %#v; want a zero receipt, commit %q, kind %q",
						operation, outcome.receipt, mutationErr, test.want, test.kind)
					continue
				}
				if !errors.Is(outcome.err, test.is) {
					t.Errorf("%s: errors.Is does not reach %v: %v", operation, test.is, outcome.err)
				}
				if test.want != CommitUnknown {
					// Design D3: no responder took the request, or it was never sent, so nothing was
					// written and a retry is safe. Only the Class field is promised; the raw
					// nats.ErrNoResponders underneath stays unclassified.
					if mutationErr.Class != errs.ErrorTransient {
						t.Errorf("%s: class %q, want transient: the request was not delivered", operation, mutationErr.Class)
					}
					continue
				}
				// No reply came back, so there is no reply code: the fatal class carries none.
				var classified *errs.ClassifiedError
				found := errors.As(outcome.err, &classified)
				if mutationErr.Class != errs.ErrorFatal || mutationErr.Code != "" ||
					!found || classified.Class != errs.ErrorFatal || classified.Code != "" {
					t.Errorf("%s: class %q, code %q, errors.As *errs.ClassifiedError %t = %#v; want fatal with no code",
						operation, mutationErr.Class, mutationErr.Code, found, classified)
				}
				assertCommitUnknownIsFatal(t, operation, outcome.err)
			}
			if test.ctx == nil {
				return
			}
			for _, request := range requester.requests {
				if request.subject != "graph.ingest.query.entity" {
					t.Errorf("a request reached the wire on %q; a context that ended sends no mutation", request.subject)
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
	if !errors.As(err, &mutationErr) || receipt != (MutationReceipt{}) || mutationErr.Commit != CommitUnknown ||
		mutationErr.Kind != MutationCommitUnknown || mutationErr.Code != graph.ErrorCodeInternal {
		t.Fatalf("receipt = %#v, error = %#v", receipt, mutationErr)
	}
	var classified *errs.ClassifiedError
	found := errors.As(err, &classified)
	if mutationErr.Class != errs.ErrorFatal || !found ||
		classified.Class != errs.ErrorFatal || classified.Code != graph.ErrorCodeInternal {
		t.Errorf("class %q, errors.As *errs.ClassifiedError %t = %#v; want fatal with code %q",
			mutationErr.Class, found, classified, graph.ErrorCodeInternal)
	}
	assertCommitUnknownIsFatal(t, MutationOperationAppend, err)
}

// A create reply that names another entity proves nothing about the request's entity, which may
// have been written: commit-unknown and fatal (design D4; the wire client refuses the reply).
func TestCreateReplyNamingAnotherEntityIsCommitUnknown(t *testing.T) {
	contract := projectionTestContract(t)
	other := projectionTestState()
	other.ID = "acme.ops.test.system.widget.002"
	requester := &projectionRequester{handle: func(string, []byte) ([]byte, error) {
		return projectionJSON(t, graph.CreateEntityResponse{
			Outcome: graph.MutationApplied, Entity: other, KVRevision: 1,
		}), nil
	}}
	client, err := newMutationClient(requester, []Contract{contract}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := client.Create(t.Context(), CreateMutation{
		Contract: contract.Name, Entity: projectionTestState(), Metadata: canonicalMetadata,
		Triples: []message.Triple{canonicalStatement("test.identity.name")},
	})
	var mutationErr *MutationError
	if !errors.As(err, &mutationErr) || receipt != (MutationReceipt{}) || mutationErr.Commit != CommitUnknown ||
		mutationErr.Kind != MutationCommitUnknown {
		t.Fatalf("receipt = %#v, error = %#v; want commit-unknown", receipt, err)
	}
	assertCommitUnknownIsFatal(t, MutationOperationCreate, err)
}

// A read writes nothing, so every failure of Reconcile's exact read is not committed: a
// classified reply, and an unclassified error such as a timeout or a client that is not
// connected (#267, design D1).
func TestReconcileReadFailureIsNotCommitted(t *testing.T) {
	tests := []struct {
		name string
		fail error
	}{
		{"classified reply", errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeInternal, errors.New("kv get failed"))},
		{"timeout", nats.ErrTimeout},
		{"not connected", natsclient.ErrNotConnected},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			contract := projectionTestContract(t)
			requester := &projectionRequester{handle: func(subject string, _ []byte) ([]byte, error) {
				if subject != "graph.ingest.query.entity" {
					t.Errorf("subject = %q; a failed read sends no mutation", subject)
				}
				return nil, test.fail
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
			if !errors.As(err, &mutationErr) || receipt != (MutationReceipt{}) || mutationErr.Commit != CommitNotCommitted ||
				mutationErr.Operation != MutationOperationReadAuthoritative {
				t.Fatalf("receipt = %#v, error = %#v; want not committed from operation %q",
					receipt, mutationErr, MutationOperationReadAuthoritative)
			}
			if mutationErr.Kind == MutationCommitUnknown {
				t.Errorf("kind %q; a read that writes nothing is never commit-unknown", mutationErr.Kind)
			}
			if !errors.Is(err, test.fail) {
				t.Errorf("errors.Is does not reach %v: %v", test.fail, err)
			}
			if len(requester.requests) != 1 {
				t.Errorf("%d requests reached the wire; want the one read", len(requester.requests))
			}
		})
	}
}
