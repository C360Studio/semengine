package graphquery

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/pkg/errs"
)

// queryHandler answers one verb's request; natsclient.Client.SubscribeForRequests takes it.
type queryHandler = func(ctx context.Context, data []byte) ([]byte, error)

// queryHandlers maps each verb graph-query answers, by its name in graph.QueryVerbs, to its handler.
// A forward answers from the row of the same name whose responder is given here (design D1), so a
// row that is gone leaves its verb with no handler, and Start fails naming it.
func (c *Component) queryHandlers() map[string]queryHandler {
	forwardedFrom := map[string]string{
		"entity": graph.ResponderGraphIngest,
		"batch":  graph.ResponderGraphIngest,
		"prefix": graph.ResponderGraphIngest,
		"byName": graph.ResponderGraphIndex,
	}
	handlers := make(map[string]queryHandler, len(forwardedFrom))
	for _, verb := range graph.QueryVerbs() {
		if forwardedFrom[verb.Name] == verb.Responder {
			handlers[verb.Name] = c.forward(verb.Subject)
		}
	}
	return handlers
}

// handlerFor returns verb's handler, or an error naming a declared verb that has none, so a table
// row cannot land unserved.
func handlerFor(verb graph.QueryVerb, handlers map[string]queryHandler) (queryHandler, error) {
	handler, ok := handlers[verb.Name]
	if !ok {
		return nil, fmt.Errorf("query verb %q (%s) is declared for graph-query but has no handler", verb.Name, verb.Subject)
	}
	return handler, nil
}

// subscribeQueries subscribes to the subject of every verb the table in graph declares for
// graph-query, and to nothing else. A declared verb with no handler fails Start. Each subscription
// is recorded as it is made, so a Start that fails part way drains the ones already made.
func (c *Component) subscribeQueries(ctx context.Context) error {
	handlers := c.queryHandlers()
	var subjects []string
	for _, verb := range graph.QueryVerbs() {
		if verb.Responder != graph.ResponderGraphQuery {
			continue
		}
		handler, err := handlerFor(verb, handlers)
		if err != nil {
			return err
		}
		sub, err := c.natsClient.SubscribeForRequests(ctx, verb.Subject, handler)
		if err != nil {
			return errs.Wrap(err, "Component", "Start", "subscribe "+verb.Name+" query")
		}
		c.subscriptions = append(c.subscriptions, sub)
		subjects = append(subjects, verb.Subject)
	}
	c.logger.Info("query handlers registered", slog.Any("subjects", subjects))
	return nil
}

// forward answers a verb from subject, under the handler's context and query_timeout_ms, without
// reading the request or the reply: their bytes pass unchanged, so a field graph-query does not know
// reaches the responder and min_revision reaches the index (design D2). It keeps neither once it has
// returned.
//
// A refusal the responder sent returns as it arrived, keeping class, code and detail. Any other
// failure means no answer arrived: no responders, whose class is not promised (ADR-112),
// query_timeout_ms passing, the handler's context ending (natsclient's handler budget or the client
// closing), or a lost connection. Each is graph-query's own transient internal refusal, naming the
// subject.
//
// Every request counts as one input. Only a request with no answer counts as an error: a responder's
// refusal is an answer, and the forward did its work, as a refused graph-ingest mutation counts its
// input and no error (graph-ingest's meteredCanonicalMutation).
func (c *Component) forward(subject string) queryHandler {
	timeout := time.Duration(c.config.QueryTimeoutMs) * time.Millisecond
	return func(ctx context.Context, data []byte) ([]byte, error) {
		c.lifecycle.CountInput(len(data))
		reply, err := c.natsClient.RequestClassified(ctx, subject, data, timeout)
		if err == nil {
			return reply, nil
		}
		var refusal *errs.ClassifiedError
		if errors.As(err, &refusal) {
			return nil, err
		}
		c.lifecycle.CountError()
		return nil, errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeInternal, fmt.Errorf("request to %s: %w", subject, err))
	}
}
