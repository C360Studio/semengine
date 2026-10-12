package projection

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
)

// projectionAcceptingRequester answers the exact read, and a create, append or
// reconcile as applied, so the only refusal a test can see is the client's own.
func projectionAcceptingRequester(t *testing.T) *projectionRequester {
	t.Helper()
	return &projectionRequester{handle: func(subject string, _ []byte) ([]byte, error) {
		switch subject {
		case "graph.ingest.query.entity":
			return projectionJSON(t, graph.ExactEntity{Entity: projectionTestState(), KVRevision: 7}), nil
		case "graph.mutation.entity.create":
			return projectionJSON(t, graph.CreateEntityResponse{
				Outcome: graph.MutationApplied, Entity: projectionTestState(), KVRevision: 1,
			}), nil
		case "graph.mutation.triple.append":
			return projectionJSON(t, graph.AppendTriplesResponse{Results: []graph.AppendSubjectResult{{
				EntityID: projectionTestEntity, Outcome: graph.MutationApplied, KVRevision: 2,
			}}}), nil
		case "graph.mutation.entity.reconcile":
			return projectionJSON(t, graph.ReconcilePredicatesResponse{
				Outcome: graph.MutationApplied, Entity: projectionTestState(), KVRevision: 8,
			}), nil
		default:
			t.Errorf("unexpected subject %q", subject)
			return nil, errors.New("unexpected subject")
		}
	}}
}

// sentRequest decodes the one request sent on subject into request.
func sentRequest(t *testing.T, requester *projectionRequester, subject string, request any) {
	t.Helper()
	var payloads [][]byte
	for _, sent := range requester.requests {
		if sent.subject == subject {
			payloads = append(payloads, sent.payload)
		}
	}
	if len(payloads) != 1 {
		t.Fatalf("requests on %s = %d, want 1", subject, len(payloads))
	}
	if err := json.Unmarshal(payloads[0], request); err != nil {
		t.Fatal(err)
	}
}

// TestMutationClientRefusesMissingTimestamp: on Create, Append and Reconcile, a
// statement with no Timestamp, under metadata with none, is refused as invalid
// before any request is sent; a Timestamp given on the statement or in the
// metadata is the one sent.
//
// Requirement: projection-mutation/The typed client never reads the clock
func TestMutationClientRefusesMissingTimestamp(t *testing.T) {
	contract := projectionTestContract(t)
	given := projectionTestTime
	operations := []struct {
		operation MutationOperation
		predicate string
		send      func(*MutationClient, message.Triple, MutationMetadata) (MutationReceipt, error)
		sent      func(*testing.T, *projectionRequester) []message.Triple
	}{
		{
			operation: MutationOperationCreate,
			predicate: "test.identity.name",
			send: func(client *MutationClient, statement message.Triple, metadata MutationMetadata) (MutationReceipt, error) {
				return client.Create(context.Background(), CreateMutation{
					Contract: contract.Name, Entity: projectionTestState(),
					Triples: []message.Triple{statement}, Metadata: metadata,
				})
			},
			sent: func(t *testing.T, requester *projectionRequester) []message.Triple {
				var request graph.CreateEntityRequest
				sentRequest(t, requester, "graph.mutation.entity.create", &request)
				return request.Triples
			},
		},
		{
			operation: MutationOperationAppend,
			predicate: "test.event.seen",
			send: func(client *MutationClient, statement message.Triple, metadata MutationMetadata) (MutationReceipt, error) {
				return client.Append(context.Background(), AppendMutation{
					Contract: contract.Name, Group: "events", EntityID: projectionTestEntity,
					Triples: []message.Triple{statement}, Metadata: metadata,
				})
			},
			sent: func(t *testing.T, requester *projectionRequester) []message.Triple {
				var request graph.AppendTriplesRequest
				sentRequest(t, requester, "graph.mutation.triple.append", &request)
				return request.Triples
			},
		},
		{
			operation: MutationOperationReconcile,
			predicate: "test.value.name",
			send: func(client *MutationClient, statement message.Triple, metadata MutationMetadata) (MutationReceipt, error) {
				return client.Reconcile(context.Background(), ReconcileMutation{
					Contract: contract.Name, Group: "state", EntityID: projectionTestEntity,
					Desired: []message.Triple{statement}, Metadata: metadata,
				})
			},
			sent: func(t *testing.T, requester *projectionRequester) []message.Triple {
				var request graph.ReconcilePredicatesRequest
				sentRequest(t, requester, "graph.mutation.entity.reconcile", &request)
				return request.Desired
			},
		},
	}
	for _, op := range operations {
		t.Run(string(op.operation), func(t *testing.T) {
			cases := []struct {
				name              string
				statement, inMeta time.Time
			}{
				{name: "no timestamp anywhere"},
				{name: "on the statement", statement: given},
				{name: "in the metadata", inMeta: given},
				{name: "the same on both", statement: given, inMeta: given},
			}
			for _, test := range cases {
				t.Run(test.name, func(t *testing.T) {
					requester := projectionAcceptingRequester(t)
					client, err := newMutationClient(requester, []Contract{contract}, time.Second)
					if err != nil {
						t.Fatal(err)
					}
					statement := message.Triple{
						Subject: projectionTestEntity, Predicate: op.predicate, Object: "value",
						Timestamp: test.statement,
					}
					metadata := MutationMetadata{RequestID: "timestamp-001", Source: "test-projector", Timestamp: test.inMeta}
					receipt, err := op.send(client, statement, metadata)
					if test.statement.IsZero() && test.inMeta.IsZero() {
						assertProjectionRejectedBeforeRequest(t, receipt, err, op.operation, requester)
						return
					}
					if err != nil || receipt.Commit != CommitVerified {
						t.Fatalf("receipt = %#v, error = %v; want verified", receipt, err)
					}
					sent := op.sent(t, requester)
					if len(sent) != 1 || !sent[0].Timestamp.Equal(given) {
						t.Fatalf("sent statements = %#v, want one stamped %s", sent, given)
					}
				})
			}
		})
	}
}

// TestMutationClientReconcileRequiresSource: Reconcile requires Metadata.Source,
// as Create and Append do, and sends it as the request's source (design D15,
// "Conditional replace"); without it nothing is sent, whatever the desired
// statements carry.
//
// Requirement: projection-mutation/Conditional reconcile at a caller-observed revision
func TestMutationClientReconcileRequiresSource(t *testing.T) {
	contract := projectionTestContract(t)
	statement := message.Triple{Subject: projectionTestEntity, Predicate: "test.value.name", Object: "new"}
	ownSource := statement
	ownSource.Source = "test-projector"
	reconcile := func(
		t *testing.T, requester *projectionRequester, desired []message.Triple, source string,
	) (MutationReceipt, error) {
		t.Helper()
		client, err := newMutationClient(requester, []Contract{contract}, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		return client.Reconcile(context.Background(), ReconcileMutation{
			Contract: contract.Name, Group: "state", EntityID: projectionTestEntity, Desired: desired,
			Metadata: MutationMetadata{RequestID: "reconcile-001", Source: source, Timestamp: projectionTestTime},
		})
	}

	refused := []struct {
		name    string
		desired []message.Triple
	}{
		{name: "clearing the group"},
		{name: "a statement without a source", desired: []message.Triple{statement}},
		{name: "a statement naming its own source", desired: []message.Triple{ownSource}},
	}
	for _, test := range refused {
		t.Run(test.name, func(t *testing.T) {
			requester := projectionAcceptingRequester(t)
			receipt, err := reconcile(t, requester, test.desired, "")
			assertProjectionRejectedBeforeRequest(t, receipt, err, MutationOperationReconcile, requester)
		})
	}

	t.Run("the metadata's source is the request's source", func(t *testing.T) {
		requester := projectionAcceptingRequester(t)
		receipt, err := reconcile(t, requester, []message.Triple{statement}, "test-projector")
		if err != nil || receipt.Commit != CommitVerified {
			t.Fatalf("receipt = %#v, error = %v; want verified", receipt, err)
		}
		var sent graph.ReconcilePredicatesRequest
		sentRequest(t, requester, "graph.mutation.entity.reconcile", &sent)
		if sent.Source != "test-projector" {
			t.Errorf("request source = %q, want %q", sent.Source, "test-projector")
		}
		if len(sent.Desired) != 1 || sent.Desired[0].Source != "test-projector" {
			t.Errorf("desired = %#v, want one statement from %q", sent.Desired, "test-projector")
		}
	})
}
