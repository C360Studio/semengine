package graphquery

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/pkg/errs"
)

// queryHandler answers one verb's request; natsclient.Client.SubscribeForRequests takes it.
type queryHandler = func(ctx context.Context, data []byte) ([]byte, error)

// queryHandlers maps each verb's name to its handler. graph.QueryVerbs declares no graph-query verb
// yet, so there is none.
func (c *Component) queryHandlers() map[string]queryHandler {
	return map[string]queryHandler{}
}

// subscribeQueries is Start's work: the table decides what graph-query serves.
func (c *Component) subscribeQueries(ctx context.Context) error {
	return c.subscribeVerbs(ctx, graph.QueryVerbs(), c.queryHandlers())
}

// subscribeVerbs subscribes a handler to every verb declared for graph-query, and fails on a
// declared verb with no handler, so a table row cannot land unserved. What it subscribed before a
// failure stays on c.subscriptions for the rollback to drain.
func (c *Component) subscribeVerbs(ctx context.Context, verbs []graph.QueryVerb, handlers map[string]queryHandler) error {
	var subjects []string
	for _, verb := range verbs {
		if verb.Responder != graph.ResponderGraphQuery {
			continue
		}
		handler, ok := handlers[verb.Name]
		if !ok {
			return fmt.Errorf("query verb %q (%s) is declared for graph-query but has no handler", verb.Name, verb.Subject)
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
