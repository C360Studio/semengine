package graphquery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/pkg/errs"
	semtypes "github.com/c360studio/semengine/pkg/types"
)

// queryHandler answers one verb's request; natsclient.Client.SubscribeForRequests takes it.
type queryHandler = func(ctx context.Context, data []byte) ([]byte, error)

// queryHandlers maps each verb graph-query answers, by its name in graph.QueryVerbs, to its handler.
// A forward answers from the row it names here by responder and name, since semantic forwards to
// search (design D1), and entityByAlias and relationships from the rows they name, so a row that is
// gone leaves its verb with no handler, and Start fails naming it.
func (c *Component) queryHandlers() map[string]queryHandler {
	forwards := map[string]struct{ responder, name string }{
		"entity":   {graph.ResponderGraphIngest, "entity"},
		"batch":    {graph.ResponderGraphIngest, "batch"},
		"prefix":   {graph.ResponderGraphIngest, "prefix"},
		"byName":   {graph.ResponderGraphIndex, "byName"},
		"semantic": {graph.ResponderGraphEmbedding, "search"},
		"similar":  {graph.ResponderGraphEmbedding, "similar"},
		"spatial":  {graph.ResponderGraphIndexSpatial, "bounds"},
		"temporal": {graph.ResponderGraphIndexTemporal, "range"},
	}
	handlers := make(map[string]queryHandler, len(forwards))
	for verb, downstream := range forwards {
		if subject := subjectOf(downstream.responder, downstream.name); subject != "" {
			handlers[verb] = c.forward(subject)
		}
	}
	alias, entity := subjectOf(graph.ResponderGraphIndex, "alias"), subjectOf(graph.ResponderGraphIngest, "entity")
	if alias != "" && entity != "" {
		handlers["entityByAlias"] = c.entityByAlias(alias, entity)
	}
	outgoing, incoming := subjectOf(graph.ResponderGraphIndex, "outgoing"), subjectOf(graph.ResponderGraphIndex, "incoming")
	if outgoing != "" && incoming != "" {
		handlers["relationships"] = c.relationships(outgoing, incoming)
	}
	return handlers
}

// subjectOf returns the subject of responder's verb name in graph.QueryVerbs, or "" if it has none.
func subjectOf(responder, name string) string {
	for _, verb := range graph.QueryVerbs() {
		if verb.Responder == responder && verb.Name == name {
			return verb.Subject
		}
	}
	return ""
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

// forward answers a verb from subject without reading the request or the reply: their bytes pass
// unchanged, so a field graph-query does not know reaches the responder and min_revision reaches the
// index (design D2). It keeps neither once it has returned.
//
// Every request counts as one input. Only a request with no answer counts as an error, which ask
// does: a responder's refusal is an answer, and the forward did its work, as a refused graph-ingest
// mutation counts its input and no error (graph-ingest's meteredCanonicalMutation).
func (c *Component) forward(subject string) queryHandler {
	return func(ctx context.Context, data []byte) ([]byte, error) {
		c.lifecycle.CountInput(len(data))
		return c.ask(ctx, subject, data)
	}
}

// ask makes one downstream request, under the handler's context and query_timeout_ms.
//
// A refusal the responder sent returns as it arrived, keeping class, code and detail. Any other
// failure means no answer arrived: no responders, whose class is not promised (ADR-112),
// query_timeout_ms passing, the handler's context ending (natsclient's handler budget or the client
// closing), or a lost connection. Each is graph-query's own transient internal refusal, naming the
// subject, and counts as an error.
//
// The timeout and the handler's context end the wait for the reply. Neither ends a wait for the
// connection's write lock, so a broker that has stopped reading can hold the call past both (ADR-112,
// "Time": P13, P14, P17).
func (c *Component) ask(ctx context.Context, subject string, data []byte) ([]byte, error) {
	timeout := time.Duration(c.config.QueryTimeoutMs) * time.Millisecond
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

// entityByAlias asks the alias verb, sending the request's bytes unchanged, then reads the holder, or
// the input as an entity ID when nobody holds it. Any refusal from either reaches the caller as it
// arrived (ADR-114). The alias verb's one miss that is not a refusal is a success naming no holder
// (design D4). Input that is no entity ID is then not found: the entity verb would refuse it as
// malformed, which it is not, so no second request goes out.
//
// data is the entity verb's JSON as it came, not re-decoded into a struct. A reply that is not JSON
// cannot be enveloped, and is refused.
//
// A request is one input however many requests it makes, and at most one error: each way out below
// refuses once. A responder's refusal and a caller's bad request count none.
func (c *Component) entityByAlias(aliasSubject, entitySubject string) queryHandler {
	return func(ctx context.Context, data []byte) ([]byte, error) {
		c.lifecycle.CountInput(len(data))
		var request struct {
			Alias string `json:"alias"`
		}
		if err := json.Unmarshal(data, &request); err != nil {
			return nil, invalidRequest(err)
		}
		raw, err := c.ask(ctx, aliasSubject, data)
		if err != nil {
			return nil, err
		}
		aliasReply, err := decodeIndexReply[graph.AliasData](c, aliasSubject, raw)
		if err != nil {
			return nil, err
		}
		id := request.Alias
		if aliasReply.Data.CanonicalID != nil {
			id = *aliasReply.Data.CanonicalID
		} else if semtypes.ValidateEntityID(id) != nil {
			return nil, errs.ClassifiedCode(errs.ErrorInvalid, graph.ErrorCodeEntityNotFound, fmt.Errorf("not found: %s", id))
		}
		entityRequest, err := json.Marshal(graph.EntityRequest{ID: id})
		if err != nil {
			return nil, internalError(err)
		}
		entity, err := c.ask(ctx, entitySubject, entityRequest)
		if err != nil {
			return nil, err
		}
		return envelope(c, entitySubject, aliasReply.IndexedRevision, json.RawMessage(entity))
	}
}

// relationships answers an entity's links in the one direction asked for, sending the request's bytes
// unchanged to that direction's verb, so min_revision and any field graph-query does not know reach
// the index. Any other direction, or none, is refused before a request goes out: the pin read each as
// outgoing, which silently gave half the links. Counted as entityByAlias is.
func (c *Component) relationships(outgoingSubject, incomingSubject string) queryHandler {
	return func(ctx context.Context, data []byte) ([]byte, error) {
		c.lifecycle.CountInput(len(data))
		var request struct {
			EntityID  string `json:"entity_id"`
			Direction string `json:"direction"`
		}
		if err := json.Unmarshal(data, &request); err != nil {
			return nil, invalidRequest(err)
		}
		switch request.Direction {
		case "outgoing":
			return c.outgoingEdges(ctx, outgoingSubject, request.EntityID, data)
		case "incoming":
			return c.incomingEdges(ctx, incomingSubject, request.EntityID, data)
		default:
			return nil, invalidRequest(fmt.Errorf("direction %q is not outgoing or incoming", request.Direction))
		}
	}
}

func (c *Component) outgoingEdges(ctx context.Context, subject, entityID string, data []byte) ([]byte, error) {
	raw, err := c.ask(ctx, subject, data)
	if err != nil {
		return nil, err
	}
	reply, err := decodeIndexReply[graph.OutgoingRelationshipsData](c, subject, raw)
	if err != nil {
		return nil, err
	}
	edges := make([]graph.RelationshipEdge, 0, len(reply.Data.Relationships))
	for _, entry := range reply.Data.Relationships {
		edges = append(edges, graph.RelationshipEdge{Predicate: entry.Predicate, FromEntityID: entityID, ToEntityID: entry.ToEntityID})
	}
	return envelope(c, subject, reply.IndexedRevision, edges)
}

func (c *Component) incomingEdges(ctx context.Context, subject, entityID string, data []byte) ([]byte, error) {
	raw, err := c.ask(ctx, subject, data)
	if err != nil {
		return nil, err
	}
	reply, err := decodeIndexReply[graph.IncomingRelationshipsData](c, subject, raw)
	if err != nil {
		return nil, err
	}
	edges := make([]graph.RelationshipEdge, 0, len(reply.Data.Relationships))
	for _, entry := range reply.Data.Relationships {
		edges = append(edges, graph.RelationshipEdge{Predicate: entry.Predicate, FromEntityID: entry.FromEntityID, ToEntityID: entityID})
	}
	return envelope(c, subject, reply.IndexedRevision, edges)
}

// decodeIndexReply reads an index verb's success. One that does not decode, or names no producer,
// which graph.NewQueryResponse never builds, is refused: "{}" decodes, and would otherwise read as no
// rows or no holder.
func decodeIndexReply[T any](c *Component, subject string, raw []byte) (graph.QueryResponse[T], error) {
	var reply graph.QueryResponse[T]
	if err := json.Unmarshal(raw, &reply); err != nil {
		return reply, c.brokenReply(subject, err)
	}
	if reply.Producer == "" {
		return reply, c.brokenReply(subject, errors.New("names no producer"))
	}
	return reply, nil
}

// envelope builds graph-query's reply to a verb, from what subject answered. A json.RawMessage that is
// not JSON fails the marshal, which is how a broken entity reply is refused. The marshal compacts and
// HTML-escapes a json.RawMessage, which bytes another Go json.Marshal wrote do not show.
func envelope[T any](c *Component, subject string, revision uint64, data T) ([]byte, error) {
	response, err := graph.NewQueryResponse(graph.ResponderGraphQuery, revision, data)
	if err != nil {
		return nil, c.brokenReply(subject, err)
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		return nil, c.brokenReply(subject, err)
	}
	return encoded, nil
}

// brokenReply is graph-query's refusal of a reply it cannot use. An answer arrived but no usable one,
// so it counts as an error, as ask counts no answer at all: otherwise a broken responder would be
// visible only to its callers.
func (c *Component) brokenReply(subject string, err error) error {
	c.lifecycle.CountError()
	return internalError(fmt.Errorf("reply from %s: %w", subject, err))
}

func invalidRequest(err error) error {
	return errs.ClassifiedCode(errs.ErrorInvalid, graph.ErrorCodeInvalidRequest, fmt.Errorf("invalid request: %w", err))
}

func internalError(err error) error {
	return errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeInternal, fmt.Errorf("internal error: %w", err))
}
