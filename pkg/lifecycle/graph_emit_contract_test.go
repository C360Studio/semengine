package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/graphmutation"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/projection"
	"github.com/nats-io/nats.go"
)

type mutationFaultRequester struct {
	calls    int
	subject  string
	response []byte
	err      error
}

func (r *mutationFaultRequester) RequestClassified(_ context.Context, subject string, _ []byte, _ time.Duration) ([]byte, error) {
	r.calls++
	r.subject = subject
	return r.response, r.err
}

func TestMutationAmbiguousReplyReturnsCommitUnknownAfterOneAttempt(t *testing.T) {
	entity := &graph.EntityState{ID: "acme.ops.test.system.widget.001"}
	operations := []struct {
		name      string
		subject   string
		operation projection.MutationOperation
		call      func(*graphEmitterNATS) error
	}{
		{
			name: "create", subject: "graph.mutation.entity.create", operation: projection.MutationOperationCreate,
			call: func(emitter *graphEmitterNATS) error {
				_, err := emitter.create(context.Background(), &graph.CreateEntityRequest{Entity: entity})
				return err
			},
		},
		{
			name: "reconcile", subject: "graph.mutation.entity.reconcile", operation: projection.MutationOperationReconcile,
			call: func(emitter *graphEmitterNATS) error {
				_, err := emitter.reconcile(context.Background(), &graph.ReconcilePredicatesRequest{
					EntityID: entity.ID, ExpectedRevision: 9, Predicates: []string{"test.state.value"},
				})
				return err
			},
		},
		{
			name: "delete", subject: "graph.mutation.entity.delete", operation: projection.MutationOperationDelete,
			call: func(emitter *graphEmitterNATS) error {
				_, err := emitter.delete(context.Background(), &graph.DeleteEntityRequest{
					EntityID: entity.ID, ExpectedRevision: 9,
				})
				return err
			},
		},
	}
	faults := []struct {
		name     string
		response []byte
		err      error
	}{
		{name: "deadline", err: context.DeadlineExceeded},
		{name: "malformed response", response: []byte(`{"outcome":`)},
	}
	for _, operation := range operations {
		for _, fault := range faults {
			t.Run(operation.name+"/"+fault.name, func(t *testing.T) {
				requester := &mutationFaultRequester{response: fault.response, err: fault.err}
				client, newErr := graphmutation.NewClient(requester, time.Second)
				if newErr != nil {
					t.Fatalf("NewClient: %v", newErr)
				}
				emitter := &graphEmitterNATS{client: client}
				err := operation.call(emitter)
				var mutationErr *projection.MutationError
				if !errors.As(err, &mutationErr) ||
					mutationErr.Operation != operation.operation ||
					mutationErr.Kind != projection.MutationCommitUnknown ||
					mutationErr.Commit != projection.CommitUnknown {
					t.Fatalf("error = %#v, want %s commit_unknown", mutationErr, operation.name)
				}
				if requester.calls != 1 {
					t.Fatalf("calls = %d, want one", requester.calls)
				}
				if requester.subject != operation.subject {
					t.Fatalf("subject = %q, want %q", requester.subject, operation.subject)
				}
			})
		}
	}
}

// classifiedRefusalRequester answers every request with the graph's classified refusal: class
// invalid, the given code, and a detail naming the entity. natsclient.ClassifyReply decodes the
// reply, as RequestClassified does in production.
type classifiedRefusalRequester struct {
	code     string
	entityID string
}

func (r classifiedRefusalRequester) RequestClassified(context.Context, string, []byte, time.Duration) ([]byte, error) {
	msg := &nats.Msg{
		Header: nats.Header{},
		Data:   []byte(`{"message":"refused by the graph","detail":{"entity_id":"` + r.entityID + `"}}`),
	}
	msg.Header.Set(natsclient.HeaderStatus, natsclient.HeaderStatusError)
	msg.Header.Set(natsclient.HeaderErrorClass, natsclient.ErrorClassInvalid)
	msg.Header.Set(natsclient.HeaderErrorCode, r.code)
	return natsclient.ClassifyReply(msg)
}

// TestEmitMappingsKeepTheClassifiedCause: where the manager turns the graph's refusal into its own
// sentinel (entity not found, entity already exists), the error still carries the refusal, with
// the class, code and detail the graph sent, so errs.Classify reads it as invalid rather than as
// a transient error worth retrying. Each case reaches one mapping: the emitter's (delete, create),
// the exact read's (Get), and Create's remap of a lost create race.
func TestEmitMappingsKeepTheClassifiedCause(t *testing.T) {
	t.Parallel()
	const entityID = "acme.ops.gcs.lifecycle.mission.refused"
	notFound := classifiedRefusalRequester{code: graph.ErrorCodeEntityNotFound, entityID: entityID}
	exists := classifiedRefusalRequester{code: graph.ErrorCodeEntityExists, entityID: entityID}
	emitterFor := func(t *testing.T, requester classifiedRefusalRequester) *graphEmitterNATS {
		t.Helper()
		client, err := graphmutation.NewClient(requester, time.Second)
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}
		return &graphEmitterNATS{client: client}
	}
	cases := []struct {
		name     string
		sentinel error
		code     string
		call     func(*testing.T) error
	}{
		{"delete of a missing entity", ErrEntityNotFound, graph.ErrorCodeEntityNotFound, func(t *testing.T) error {
			_, err := emitterFor(t, notFound).delete(context.Background(),
				&graph.DeleteEntityRequest{EntityID: entityID, ExpectedRevision: 3})
			return err
		}},
		{"create of an existing entity", ErrAlreadyExists, graph.ErrorCodeEntityExists, func(t *testing.T) error {
			_, err := emitterFor(t, exists).create(context.Background(),
				&graph.CreateEntityRequest{Entity: &graph.EntityState{ID: entityID}})
			return err
		}},
		{"Get of a missing entity", ErrEntityNotFound, graph.ErrorCodeEntityNotFound, func(t *testing.T) error {
			mgr, _, _ := newTestManager(t)
			mgr.exactReader = productionExactReader(t, notFound)
			_, err := mgr.Get(context.Background(), "fixture", entityID)
			return err
		}},
		{"Create that loses the create race", ErrAlreadyExists, graph.ErrorCodeEntityExists, func(t *testing.T) error {
			mgr, _, _ := newTestManager(t)
			mgr.exactReader = productionExactReader(t, notFound)
			mgr.emitter = emitterFor(t, exists)
			return mgr.Create(context.Background(), &fixtureMission{ID: entityID, PhaseF: "planning"})
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			err := c.call(t)
			if !errors.Is(err, c.sentinel) {
				t.Errorf("error %v does not match %v", err, c.sentinel)
			}
			var classified *errs.ClassifiedError
			if !errors.As(err, &classified) {
				t.Fatalf("error %v lost the graph's classified refusal", err)
			}
			if classified.Class != errs.ErrorInvalid || classified.Code != c.code ||
				classified.Detail["entity_id"] != entityID {
				t.Errorf("refusal = class %v, code %q, detail %v; want invalid, %q, entity_id %s",
					classified.Class, classified.Code, classified.Detail, c.code, entityID)
			}
			if class := errs.Classify(err); class != errs.ErrorInvalid {
				t.Errorf("errs.Classify = %v, want invalid", class)
			}
		})
	}
}
