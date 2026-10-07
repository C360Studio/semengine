package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/graphmutation"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/projection"
)

type graphEmitter interface {
	reconcile(context.Context, *graph.ReconcilePredicatesRequest) (*graph.ReconcilePredicatesResponse, error)
	create(context.Context, *graph.CreateEntityRequest) (*graph.CreateEntityResponse, error)
	delete(context.Context, *graph.DeleteEntityRequest) (*graph.DeleteEntityResponse, error)
}

// graphEmitterNATS sends the manager's writes through the canonical graph mutation client. With
// no client, every write is refused with ErrEmitFailed and unavailable, the reason there is none.
type graphEmitterNATS struct {
	client      *graphmutation.Client
	unavailable error
}

func newGraphEmitterNATS(client *natsclient.Client, timeout time.Duration) *graphEmitterNATS {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	// A nil *natsclient.Client is not a nil requester: graphmutation.NewClient would accept it,
	// and the first write would dereference it.
	if client == nil {
		return &graphEmitterNATS{unavailable: errors.New("no NATS client")}
	}
	mutationClient, err := graphmutation.NewClient(client, timeout)
	if err != nil {
		return &graphEmitterNATS{unavailable: err}
	}
	return &graphEmitterNATS{client: mutationClient}
}

func (g *graphEmitterNATS) reconcile(
	ctx context.Context,
	request *graph.ReconcilePredicatesRequest,
) (*graph.ReconcilePredicatesResponse, error) {
	if g.client == nil {
		return nil, fmt.Errorf("%w: graph mutation client is unavailable: %w", ErrEmitFailed, g.unavailable)
	}
	response, err := g.client.Reconcile(ctx, *request)
	if err != nil {
		return nil, classifyEmitError(graphmutation.ReconcilePredicates, err)
	}
	return response, nil
}

func (g *graphEmitterNATS) create(
	ctx context.Context,
	request *graph.CreateEntityRequest,
) (*graph.CreateEntityResponse, error) {
	if g.client == nil {
		return nil, fmt.Errorf("%w: graph mutation client is unavailable: %w", ErrEmitFailed, g.unavailable)
	}
	response, err := g.client.Create(ctx, *request)
	if err != nil {
		return nil, classifyEmitError(graphmutation.CreateEntity, err)
	}
	return response, nil
}

func (g *graphEmitterNATS) delete(
	ctx context.Context,
	request *graph.DeleteEntityRequest,
) (*graph.DeleteEntityResponse, error) {
	if g.client == nil {
		return nil, fmt.Errorf("%w: graph mutation client is unavailable: %w", ErrEmitFailed, g.unavailable)
	}
	response, err := g.client.Delete(ctx, *request)
	if err != nil {
		return nil, classifyEmitError(graphmutation.DeleteEntity, err)
	}
	return response, nil
}

func classifyEmitError(operation graphmutation.Operation, err error) error {
	var classified *errs.ClassifiedError
	if errors.As(err, &classified) {
		switch classified.Code {
		case graph.ErrorCodeRevisionMismatch:
			return err
		case graph.ErrorCodeEntityNotFound:
			return fmt.Errorf("%w: %s", ErrEntityNotFound, err.Error())
		case graph.ErrorCodeEntityExists:
			return fmt.Errorf("%w: %s", ErrAlreadyExists, err.Error())
		default:
			return fmt.Errorf("%w: %s: %w", ErrEmitFailed, operation, err)
		}
	}
	if natsclient.IsNoResponders(err) {
		return &projection.MutationError{
			Operation: lifecycleMutationOperation(operation), Kind: projection.MutationUnavailable,
			Class: errs.ErrorTransient, Commit: projection.CommitNotCommitted, Err: err,
		}
	}
	return &projection.MutationError{
		Operation: lifecycleMutationOperation(operation), Kind: projection.MutationCommitUnknown,
		Class: errs.Classify(err), Commit: projection.CommitUnknown, Err: err,
	}
}

func lifecycleMutationOperation(operation graphmutation.Operation) projection.MutationOperation {
	switch operation {
	case graphmutation.CreateEntity:
		return projection.MutationOperationCreate
	case graphmutation.ReconcilePredicates:
		return projection.MutationOperationReconcile
	case graphmutation.DeleteEntity:
		return projection.MutationOperationDelete
	default:
		return projection.MutationOperationAppend
	}
}

// triple is a statement the manager writes: under its own source, graph.SourceLifecycle, so a
// reconcile replaces the statements the manager wrote before and no other producer's. The
// manager originates the statement, so it stamps it from the clock (design D15).
func triple(subject, predicate string, object any) message.Triple {
	return message.Triple{
		Subject: subject, Predicate: predicate, Object: object,
		Source: graph.SourceLifecycle, Timestamp: time.Now(), Confidence: 1.0,
	}
}
