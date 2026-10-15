package projection

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/vocabulary"
)

// The typed client's half of design D11's tests for pkg/projection (task 5.2): each write sends its
// canonical request and reads its reply, a mutation error keeps the reply's code and detail (and its
// class when the reply proves nothing was written; a commit-unknown error is fatal), and a group or
// statement the contract does not allow is refused before any request. The wire is a double that
// answers each request as the test names.

// canonicalMetadata is the caller's provenance every write below sends.
var canonicalMetadata = MutationMetadata{
	RequestID: "request-001", TraceID: "trace-001", Source: "test-projector", Timestamp: projectionTestTime,
}

// canonicalStatement is a statement as a caller gives it: subject, predicate and object only, the
// rest left for the client to fill from canonicalMetadata.
func canonicalStatement(predicate string) message.Triple {
	return message.Triple{Subject: projectionTestEntity, Predicate: predicate, Object: "value"}
}

// filledStatement is canonicalStatement as it must reach graph-ingest: source, context and time
// taken from canonicalMetadata.
func filledStatement(predicate string) message.Triple {
	return message.Triple{
		Subject: projectionTestEntity, Predicate: predicate, Object: "value",
		Source: canonicalMetadata.Source, Context: canonicalMetadata.RequestID, Timestamp: projectionTestTime,
	}
}

// TestCreateSendsItsCanonicalRequestAndReadsTheReply: Create sends one request on the create
// subject carrying the entity, its statements with source, context and time filled from the
// metadata, the contract's indexing profile, and the metadata's trace and request IDs; the receipt
// carries the reply's entity and revision, verified.
func TestCreateSendsItsCanonicalRequestAndReadsTheReply(t *testing.T) {
	contract := projectionTestContract(t)
	contract.IndexingProfile = vocabulary.IndexingProfileContent
	requester := projectionAcceptingRequester(t)
	client, err := newMutationClient(requester, []Contract{contract}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := client.Create(t.Context(), CreateMutation{
		Contract: contract.Name, Entity: projectionTestState(),
		Triples: []message.Triple{canonicalStatement("test.identity.name")}, Metadata: canonicalMetadata,
	})
	if err != nil {
		t.Fatal(err)
	}
	var sent graph.CreateEntityRequest
	sentRequest(t, requester, "graph.mutation.entity.create", &sent)
	if len(requester.requests) != 1 {
		t.Fatalf("requests = %d, want the create alone", len(requester.requests))
	}
	if sent.Entity == nil || sent.Entity.ID != projectionTestEntity || !sent.Entity.MessageType.Equal(contract.MessageType) {
		t.Fatalf("sent entity = %#v, want %s of type %s", sent.Entity, projectionTestEntity, contract.MessageType.Key())
	}
	want := []message.Triple{filledStatement("test.identity.name")}
	if !reflect.DeepEqual(sent.Triples, want) || sent.IndexingProfile != vocabulary.IndexingProfileContent ||
		sent.TraceID != canonicalMetadata.TraceID || sent.RequestID != canonicalMetadata.RequestID {
		t.Fatalf("sent = %#v, want statements %#v, profile %q, trace %q, request %q", sent, want,
			vocabulary.IndexingProfileContent, canonicalMetadata.TraceID, canonicalMetadata.RequestID)
	}
	// projectionAcceptingRequester answers a create at revision 1.
	if receipt.Commit != CommitVerified || receipt.KVRevision != 1 || receipt.Entity == nil ||
		receipt.Entity.ID != projectionTestEntity {
		t.Fatalf("receipt = %#v, want the reply's entity verified at revision 1", receipt)
	}
}

// TestAppendSendsItsCanonicalRequestAndReadsTheReply: Append sends one request on the append
// subject carrying its statements, filled from the metadata, and the metadata's trace and request
// IDs. An applied or unchanged reply is verified at the reply's revision; an entity-not-found
// reply is a not-committed not-found error with that code, since nothing was written.
func TestAppendSendsItsCanonicalRequestAndReadsTheReply(t *testing.T) {
	contract := projectionTestContract(t)
	tests := []struct {
		outcome graph.MutationOutcome
		found   bool
	}{
		{graph.MutationApplied, true},
		{graph.MutationUnchanged, true},
		{graph.MutationEntityNotFound, false},
	}
	for _, test := range tests {
		t.Run(string(test.outcome), func(t *testing.T) {
			result := graph.AppendSubjectResult{EntityID: projectionTestEntity, Outcome: test.outcome}
			if test.found {
				result.KVRevision = 5
			}
			requester := &projectionRequester{handle: func(string, []byte) ([]byte, error) {
				return projectionJSON(t, graph.AppendTriplesResponse{Results: []graph.AppendSubjectResult{result}}), nil
			}}
			client, err := newMutationClient(requester, []Contract{contract}, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			receipt, err := client.Append(t.Context(), AppendMutation{
				Contract: contract.Name, Group: "events", EntityID: projectionTestEntity,
				Triples: []message.Triple{canonicalStatement("test.event.seen")}, Metadata: canonicalMetadata,
			})
			var sent graph.AppendTriplesRequest
			sentRequest(t, requester, "graph.mutation.triple.append", &sent)
			want := graph.AppendTriplesRequest{
				Triples: []message.Triple{filledStatement("test.event.seen")},
				TraceID: canonicalMetadata.TraceID, RequestID: canonicalMetadata.RequestID,
			}
			if len(requester.requests) != 1 || !reflect.DeepEqual(sent, want) {
				t.Fatalf("requests = %d, sent %#v; want one, %#v", len(requester.requests), sent, want)
			}
			if test.found {
				if err != nil || receipt.Commit != CommitVerified || receipt.KVRevision != 5 {
					t.Fatalf("receipt = %#v, error = %v; want verified at revision 5", receipt, err)
				}
				return
			}
			var mutationErr *MutationError
			if !errors.As(err, &mutationErr) || mutationErr.Kind != MutationNotFound ||
				mutationErr.Code != graph.ErrorCodeEntityNotFound || mutationErr.Class != errs.ErrorInvalid ||
				mutationErr.Commit != CommitNotCommitted || receipt.Commit != CommitNotCommitted {
				t.Fatalf("receipt = %#v, error = %#v; want a not-committed entity-not-found", receipt, err)
			}
		})
	}
}

// TestDeleteSendsItsCanonicalRequestAndReadsTheReply: Delete sends one request on the delete
// subject carrying the entity, the caller's revision fence and the metadata's trace and request
// IDs, with no read before it; the receipt is verified at the fence the reply confirms.
func TestDeleteSendsItsCanonicalRequestAndReadsTheReply(t *testing.T) {
	contract := projectionTestContract(t)
	const fence = uint64(7)
	requester := &projectionRequester{handle: func(string, []byte) ([]byte, error) {
		return projectionJSON(t, graph.DeleteEntityResponse{
			EntityID: projectionTestEntity, Outcome: graph.MutationApplied, ExpectedRevision: fence,
		}), nil
	}}
	client, err := newMutationClient(requester, []Contract{contract}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := client.Delete(t.Context(), DeleteMutation{
		EntityID: projectionTestEntity, ExpectedRevision: fence, Metadata: canonicalMetadata,
	})
	if err != nil {
		t.Fatal(err)
	}
	var sent graph.DeleteEntityRequest
	sentRequest(t, requester, "graph.mutation.entity.delete", &sent)
	want := graph.DeleteEntityRequest{
		EntityID: projectionTestEntity, ExpectedRevision: fence,
		TraceID: canonicalMetadata.TraceID, RequestID: canonicalMetadata.RequestID,
	}
	if len(requester.requests) != 1 || sent != want {
		t.Fatalf("requests = %d, sent %#v; want one, %#v", len(requester.requests), sent, want)
	}
	if receipt.Commit != CommitVerified || receipt.KVRevision != fence {
		t.Fatalf("receipt = %#v, want verified at revision %d", receipt, fence)
	}
}

// TestMutationErrorKeepsTheReplysClassCodeAndDetail: on every write, the error for a classified
// reply carries the reply's class, code and detail, and still unwraps to the reply itself. The
// detail is the error's own copy: a later change to the reply's detail does not reach it.
func TestMutationErrorKeepsTheReplysClassCodeAndDetail(t *testing.T) {
	detail := map[string]any{"expected_revision": uint64(7), "current_revision": uint64(8)}
	want := map[string]any{"expected_revision": uint64(7), "current_revision": uint64(8)}
	reply := errs.ClassifiedCodeDetail(errs.ErrorInvalid, graph.ErrorCodeRevisionMismatch, detail,
		errors.New("revision mismatch"))
	outcomes := eachWrite(t, reply)
	detail["current_revision"] = uint64(9)
	detail["added_later"] = true
	for operation, outcome := range outcomes {
		var mutationErr *MutationError
		if !errors.As(outcome.err, &mutationErr) {
			t.Fatalf("%s: error = %v, want a *MutationError", operation, outcome.err)
		}
		if mutationErr.Class != errs.ErrorInvalid || mutationErr.Code != graph.ErrorCodeRevisionMismatch {
			t.Errorf("%s: class %q, code %q; want the reply's %q, %q", operation, mutationErr.Class,
				mutationErr.Code, errs.ErrorInvalid, graph.ErrorCodeRevisionMismatch)
		}
		if !reflect.DeepEqual(mutationErr.Detail, want) {
			t.Errorf("%s: detail %v, want the reply's detail as it was, %v", operation, mutationErr.Detail, want)
		}
		var classified *errs.ClassifiedError
		if !errors.As(outcome.err, &classified) || classified != reply {
			t.Errorf("%s: error %v does not unwrap to the reply", operation, outcome.err)
		}
	}
}

// groupWrite is one write's inputs, as a refusal case changes them.
type groupWrite struct {
	contract, group, entityID string
	statement                 message.Triple
	metadata                  MutationMetadata
}

// writeChange changes one thing in a write the contract and metadata allow.
type writeChange struct {
	name   string
	change func(*groupWrite)
}

// TestGroupAndStatementCanonicalizationRefusedBeforeRequest: a write naming a contract, group,
// entity, subject, predicate, source or context its contract and metadata do not allow is refused
// as invalid, not committed, before any request. Each case changes one thing in a write the test
// first shows is accepted, against a wire that accepts everything, so a refusal is the client's
// and has that one cause.
func TestGroupAndStatementCanonicalizationRefusedBeforeRequest(t *testing.T) {
	contract := projectionTestContract(t)
	const outside = "acme.ops.test.system.gadget.001" // the contract's pattern is *.*.test.system.widget.*
	operations := []struct {
		operation MutationOperation
		group     string
		predicate string
		// otherGroup is a declared group of the other mode; otherPredicate is outside what the
		// operation may write: for Create, undeclared by the contract; otherwise, in another group.
		otherGroup, otherPredicate string
		send                       func(*MutationClient, groupWrite) (MutationReceipt, error)
	}{
		{
			operation: MutationOperationCreate, predicate: "test.identity.name", otherPredicate: "test.undeclared.name",
			send: func(client *MutationClient, w groupWrite) (MutationReceipt, error) {
				entity := projectionTestState()
				entity.ID = w.entityID
				return client.Create(t.Context(), CreateMutation{
					Contract: w.contract, Entity: entity, Triples: []message.Triple{w.statement}, Metadata: w.metadata,
				})
			},
		},
		{
			operation: MutationOperationReconcile, group: "state", predicate: "test.value.name",
			otherGroup: "events", otherPredicate: "test.event.seen",
			send: func(client *MutationClient, w groupWrite) (MutationReceipt, error) {
				return client.Reconcile(t.Context(), ReconcileMutation{
					Contract: w.contract, Group: w.group, EntityID: w.entityID,
					Desired: []message.Triple{w.statement}, Metadata: w.metadata,
				})
			},
		},
		{
			operation: MutationOperationAppend, group: "events", predicate: "test.event.seen",
			otherGroup: "state", otherPredicate: "test.value.name",
			send: func(client *MutationClient, w groupWrite) (MutationReceipt, error) {
				return client.Append(t.Context(), AppendMutation{
					Contract: w.contract, Group: w.group, EntityID: w.entityID,
					Triples: []message.Triple{w.statement}, Metadata: w.metadata,
				})
			},
		},
	}
	for _, op := range operations {
		cases := []writeChange{
			{"as given", func(*groupWrite) {}},
			{"unknown contract", func(w *groupWrite) { w.contract = "unknown" }},
			{"entity outside the contract's pattern", func(w *groupWrite) { w.entityID, w.statement.Subject = outside, outside }},
			{"statement on another subject", func(w *groupWrite) { w.statement.Subject = "acme.ops.test.system.widget.002" }},
			{"predicate the operation may not write", func(w *groupWrite) { w.statement.Predicate = op.otherPredicate }},
			{"statement source differs from the metadata", func(w *groupWrite) { w.statement.Source = "another-projector" }},
			{"statement context differs from the request ID", func(w *groupWrite) { w.statement.Context = "another-request" }},
		}
		if op.group != "" {
			cases = append(cases,
				writeChange{"undeclared group", func(w *groupWrite) { w.group = "undeclared" }},
				writeChange{"group of the other mode", func(w *groupWrite) { w.group = op.otherGroup }},
			)
		}
		for _, test := range cases {
			t.Run(string(op.operation)+"/"+test.name, func(t *testing.T) {
				write := groupWrite{
					contract: contract.Name, group: op.group, entityID: projectionTestEntity,
					statement: canonicalStatement(op.predicate), metadata: canonicalMetadata,
				}
				test.change(&write)
				requester := projectionAcceptingRequester(t)
				client, err := newMutationClient(requester, []Contract{contract}, time.Second)
				if err != nil {
					t.Fatal(err)
				}
				receipt, err := op.send(client, write)
				if test.name == "as given" {
					if err != nil || receipt.Commit != CommitVerified {
						t.Fatalf("receipt = %#v, error = %v; want the unchanged write verified", receipt, err)
					}
					return
				}
				assertProjectionRejectedBeforeRequest(t, receipt, err, op.operation, requester)
			})
		}
	}
}

// TestCreateRefusesAnEntityItCannotBuildFrom: Create needs an entity, and takes its statements
// only from the mutation's own list, so a missing entity or one that carries statements is refused
// as invalid, not committed, before any request.
func TestCreateRefusesAnEntityItCannotBuildFrom(t *testing.T) {
	contract := projectionTestContract(t)
	carrying := projectionTestState()
	carrying.Triples = []message.Triple{canonicalStatement("test.identity.name")}
	for name, entity := range map[string]*graph.EntityState{"no entity": nil, "entity carrying statements": carrying} {
		t.Run(name, func(t *testing.T) {
			requester := projectionAcceptingRequester(t)
			client, err := newMutationClient(requester, []Contract{contract}, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			receipt, err := client.Create(t.Context(), CreateMutation{
				Contract: contract.Name, Entity: entity,
				Triples: []message.Triple{canonicalStatement("test.identity.name")}, Metadata: canonicalMetadata,
			})
			assertProjectionRejectedBeforeRequest(t, receipt, err, MutationOperationCreate, requester)
		})
	}
}
