package lifecycle

import (
	"context"
	"fmt"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/graphmutation"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/pkg/projection"
)

type graphEmitter interface {
	reconcile(context.Context, *graph.ReconcilePredicatesRequest) (*graph.EntityMutationResponse, error)
	create(context.Context, *graph.CreateEntityRequest) (*graph.EntityMutationResponse, error)
	delete(context.Context, *graph.DeleteEntityRequest) (*graph.DeleteEntityResponse, error)
}

// graphEmitterNATS sends the manager's writes through the canonical graph mutation client. A
// manager built without a NATS client holds none: every write reads first, and that read refuses
// with errNoNATSClient.
type graphEmitterNATS struct {
	client *graphmutation.Client
}

func (g *graphEmitterNATS) reconcile(
	ctx context.Context,
	request *graph.ReconcilePredicatesRequest,
) (*graph.EntityMutationResponse, error) {
	response, err := g.client.Reconcile(ctx, *request)
	if err != nil {
		return nil, emitError(projection.MutationOperationReconcile, err)
	}
	return response, nil
}

func (g *graphEmitterNATS) create(
	ctx context.Context,
	request *graph.CreateEntityRequest,
) (*graph.EntityMutationResponse, error) {
	response, err := g.client.Create(ctx, *request)
	if err != nil {
		return nil, emitError(projection.MutationOperationCreate, err)
	}
	return response, nil
}

func (g *graphEmitterNATS) delete(
	ctx context.Context,
	request *graph.DeleteEntityRequest,
) (*graph.DeleteEntityResponse, error) {
	response, err := g.client.Delete(ctx, *request)
	if err != nil {
		return nil, emitError(projection.MutationOperationDelete, err)
	}
	return response, nil
}

// emitError reports a write's failure through projection.ClassifyMutationError, the one mapping
// from a mutation request's error to its commit state (projection-mutation spec). A refusal that
// wrote nothing because the entity is missing or already exists is also the manager's sentinel.
func emitError(operation projection.MutationOperation, err error) error {
	mapped := projection.ClassifyMutationError(operation, err)
	if mapped.Commit == projection.CommitNotCommitted {
		switch mapped.Code {
		case graph.ErrorCodeEntityNotFound:
			return fmt.Errorf("%w: %w", ErrEntityNotFound, mapped)
		case graph.ErrorCodeEntityExists:
			return fmt.Errorf("%w: %w", ErrAlreadyExists, mapped)
		}
	}
	return mapped
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
