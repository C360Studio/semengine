package lifecycle

import (
	"context"
	"encoding/json"
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

// emitWriteOperation is one of the graph emitter's writes, with the subject and mutation operation
// it reports.
type emitWriteOperation struct {
	name      string
	subject   string
	operation projection.MutationOperation
	call      func(*graphEmitterNATS) error
}

func emitWriteOperations() []emitWriteOperation {
	entity := &graph.EntityState{ID: "acme.ops.test.system.widget.001"}
	return []emitWriteOperation{
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
}

func TestMutationAmbiguousReplyReturnsCommitUnknownAfterOneAttempt(t *testing.T) {
	operations := emitWriteOperations()
	var syntaxErr *json.SyntaxError
	faults := []struct {
		name     string
		response []byte
		err      error
		// reachesCause reports whether the returned error still reaches the fault's own cause.
		reachesCause func(error) bool
	}{
		{name: "deadline", err: context.DeadlineExceeded,
			reachesCause: func(err error) bool { return errors.Is(err, context.DeadlineExceeded) }},
		{name: "malformed response", response: []byte(`{"outcome":`),
			reachesCause: func(err error) bool { return errors.As(err, &syntaxErr) }},
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
				// A write that may have committed is never retryable (design D2). No reply came
				// back, so there is no reply code to carry: Code stays empty.
				if mutationErr.Class != errs.ErrorFatal || mutationErr.Code != "" {
					t.Errorf("class %q, code %q; want fatal with no code", mutationErr.Class, mutationErr.Code)
				}
				if errs.IsTransient(err) || !errs.IsFatal(err) || errs.Classify(err) != errs.ErrorFatal {
					t.Errorf("IsTransient %t, IsFatal %t, Classify %v; want false, true, fatal (err=%v)",
						errs.IsTransient(err), errs.IsFatal(err), errs.Classify(err), err)
				}
				var classified *errs.ClassifiedError
				found := errors.As(err, &classified)
				if !found || classified.Class != errs.ErrorFatal || classified.Code != "" {
					t.Errorf("errors.As *errs.ClassifiedError %t = %#v; want fatal with no code", found, classified)
				}
				var transport *graphmutation.TransportError
				if !errors.As(err, &transport) || transport.Outcome != graphmutation.TransportCommitUnknown {
					t.Errorf("errors.As *graphmutation.TransportError = %#v; want outcome commit_unknown", transport)
				}
				if !fault.reachesCause(err) {
					t.Errorf("the cause is not reachable: %v", err)
				}
			})
		}
	}
}

// TestNoRespondersIsTransientAndNotCommitted pins the lifecycle half of the projection-mutation
// requirement "A request no responder took keeps the transient class": no responder took the
// request, so nothing was written and the Class field is transient. Only the field is promised;
// the raw nats.ErrNoResponders it wraps is unclassified.
func TestNoRespondersIsTransientAndNotCommitted(t *testing.T) {
	for _, operation := range emitWriteOperations() {
		t.Run(operation.name, func(t *testing.T) {
			client, err := graphmutation.NewClient(&mutationFaultRequester{err: nats.ErrNoResponders}, time.Second)
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			err = operation.call(&graphEmitterNATS{client: client})
			var mutationErr *projection.MutationError
			if !errors.As(err, &mutationErr) ||
				mutationErr.Operation != operation.operation ||
				mutationErr.Kind != projection.MutationUnavailable ||
				mutationErr.Commit != projection.CommitNotCommitted {
				t.Fatalf("error = %#v, want %s unavailable and not committed", mutationErr, operation.name)
			}
			if mutationErr.Class != errs.ErrorTransient {
				t.Errorf("class %q, want transient: no responder took the request", mutationErr.Class)
			}
			if !errors.Is(err, nats.ErrNoResponders) {
				t.Errorf("the cause is not reachable: %v", err)
			}
		})
	}
}

// classifiedRefusalRequester answers every request with the graph's classified error reply: the
// given class (invalid when empty) and code, and a detail naming the entity.
// natsclient.ClassifyReply decodes the reply, as RequestClassified does in production.
type classifiedRefusalRequester struct {
	class    string
	code     string
	entityID string
}

func (r classifiedRefusalRequester) RequestClassified(context.Context, string, []byte, time.Duration) ([]byte, error) {
	msg := &nats.Msg{
		Header: nats.Header{},
		Data:   []byte(`{"message":"refused by the graph","detail":{"entity_id":"` + r.entityID + `"}}`),
	}
	msg.Header.Set(natsclient.HeaderStatus, natsclient.HeaderStatusError)
	class := r.class
	if class == "" {
		class = natsclient.ErrorClassInvalid
	}
	msg.Header.Set(natsclient.HeaderErrorClass, class)
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

// TestInternalReplyIsCommitUnknownAndNotRetryable: graph-ingest's transient internal reply can
// follow a write the bucket did not confirm, so for every lifecycle write it is commit-unknown and
// fatal, never retryable, and is not reported as ErrEmitFailed (projection-mutation spec, "A
// lifecycle write answered after a possible write"; #257).
func TestInternalReplyIsCommitUnknownAndNotRetryable(t *testing.T) {
	t.Parallel()
	for _, operation := range emitWriteOperations() {
		t.Run(operation.name, func(t *testing.T) {
			t.Parallel()
			requester := classifiedRefusalRequester{
				class: natsclient.ErrorClassTransient, code: graph.ErrorCodeInternal,
				entityID: "acme.ops.test.system.widget.001",
			}
			client, err := graphmutation.NewClient(requester, time.Second)
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			err = operation.call(&graphEmitterNATS{client: client})
			var mutationErr *projection.MutationError
			if !errors.As(err, &mutationErr) {
				t.Fatalf("error = %v, want a *projection.MutationError", err)
			}
			if mutationErr.Operation != operation.operation ||
				mutationErr.Kind != projection.MutationCommitUnknown ||
				mutationErr.Commit != projection.CommitUnknown ||
				mutationErr.Class != errs.ErrorFatal ||
				mutationErr.Code != graph.ErrorCodeInternal {
				t.Errorf("error = %+v; want %s commit_unknown, class fatal, code internal",
					mutationErr, operation.operation)
			}
			if errs.IsTransient(err) {
				t.Errorf("errs.IsTransient = true; a write that may have committed is never retryable (err=%v)", err)
			}
			if errors.Is(err, ErrEmitFailed) {
				t.Errorf("error matches ErrEmitFailed: %v", err)
			}
		})
	}
}

// TestCancelledContextSendsNothingAndIsNotCommitted: a write whose context ended before the
// request was sent reached no broker, so it is not committed, unavailable and transient, and the
// cancellation stays reachable (projection-mutation spec, "A lifecycle write whose context ended
// before sending").
func TestCancelledContextSendsNothingAndIsNotCommitted(t *testing.T) {
	t.Parallel()
	entity := &graph.EntityState{ID: "acme.ops.test.system.widget.001"}
	writes := []struct {
		name      string
		operation projection.MutationOperation
		call      func(context.Context, *graphEmitterNATS) error
	}{
		{"create", projection.MutationOperationCreate, func(ctx context.Context, emitter *graphEmitterNATS) error {
			_, err := emitter.create(ctx, &graph.CreateEntityRequest{Entity: entity})
			return err
		}},
		{"reconcile", projection.MutationOperationReconcile, func(ctx context.Context, emitter *graphEmitterNATS) error {
			_, err := emitter.reconcile(ctx, &graph.ReconcilePredicatesRequest{
				EntityID: entity.ID, ExpectedRevision: 9, Predicates: []string{"test.state.value"},
			})
			return err
		}},
		{"delete", projection.MutationOperationDelete, func(ctx context.Context, emitter *graphEmitterNATS) error {
			_, err := emitter.delete(ctx, &graph.DeleteEntityRequest{EntityID: entity.ID, ExpectedRevision: 9})
			return err
		}},
	}
	for _, w := range writes {
		t.Run(w.name, func(t *testing.T) {
			t.Parallel()
			requester := &mutationFaultRequester{}
			client, err := graphmutation.NewClient(requester, time.Second)
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			err = w.call(ctx, &graphEmitterNATS{client: client})
			if requester.calls != 0 {
				t.Errorf("requester calls = %d, want none: the context ended before sending", requester.calls)
			}
			var mutationErr *projection.MutationError
			if !errors.As(err, &mutationErr) {
				t.Fatalf("error = %v, want a *projection.MutationError", err)
			}
			if mutationErr.Operation != w.operation ||
				mutationErr.Kind != projection.MutationUnavailable ||
				mutationErr.Commit != projection.CommitNotCommitted ||
				mutationErr.Class != errs.ErrorTransient {
				t.Errorf("error = %+v; want %s unavailable, not committed, class transient", mutationErr, w.operation)
			}
			if !errors.Is(err, context.Canceled) {
				t.Errorf("errors.Is(err, context.Canceled) = false: %v", err)
			}
		})
	}
}
