package graphgateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/pkg/errs"
)

// rootVerbs maps each root field to the graph.query.* verb that answers it (design D2).
var rootVerbs = map[string]string{
	"entity":              "entity",
	"entitiesByPrefix":    "prefix",
	"entitiesByPredicate": "predicate",
	"pathSearch":          "pathSearch",
}

// argumentError is an argument the schema accepts and the gateway refuses, answered HTTP 400.
type argumentError struct{ message string }

func (e *argumentError) Error() string { return e.message }

// noAnswer is a request that got no reply: no responders, query_timeout_ms passing, the HTTP
// request's context ending, or a lost connection. It names the subject. Its class is transient and
// its code internal, as graph-query's own (processor/graph-query/query.go).
type noAnswer struct {
	subject string
	err     error
}

func (e *noAnswer) Error() string { return fmt.Sprintf("request to %s: %v", e.subject, e.err) }

func (e *noAnswer) Unwrap() error { return e.err }

// rootResolvers builds a resolver for each root field from its verb's row in graph.QueryVerbs, so
// the subjects are the table's and a reply is decoded by the row's Enveloped flag.
func (c *Component) rootResolvers() (map[string]resolver, error) {
	verbs := map[string]graph.QueryVerb{}
	for field, name := range rootVerbs {
		for _, verb := range graph.QueryVerbs() {
			if verb.Responder == graph.ResponderGraphQuery && verb.Name == name {
				verbs[field] = verb
			}
		}
		if _, ok := verbs[field]; !ok {
			return nil, fmt.Errorf("root field %s: graph.QueryVerbs declares no graph-query verb %q", field, name)
		}
	}
	return map[string]resolver{
		"entity":              c.entity(verbs["entity"]),
		"entitiesByPrefix":    c.entitiesByPrefix(verbs["entitiesByPrefix"]),
		"entitiesByPredicate": c.entitiesByPredicate(verbs["entitiesByPredicate"]),
		"pathSearch":          c.pathSearch(verbs["pathSearch"]),
	}, nil
}

// ask sends body to verb's subject under ctx and query_timeout_ms and returns the reply body, the
// envelope's data when the row is enveloped. A refusal the responder sent returns as it arrived,
// keeping class, code and detail; any other failure is a noAnswer.
func (c *Component) ask(ctx context.Context, verb graph.QueryVerb, request any) ([]byte, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal %s request: %w", verb.Name, err)
	}
	reply, err := c.natsClient.RequestClassified(ctx, verb.Subject, body, c.timeout())
	if err != nil {
		var refusal *errs.ClassifiedError
		if errors.As(err, &refusal) {
			return nil, err
		}
		return nil, errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeInternal, &noAnswer{verb.Subject, err})
	}
	if !verb.Enveloped {
		return reply, nil
	}
	var envelope graph.QueryResponse[json.RawMessage]
	if err := json.Unmarshal(reply, &envelope); err != nil {
		return nil, unfit(verb, err)
	}
	return envelope.Data, nil
}

// unfit is a reply that does not decode as verb's row declares, or fails a check of design D2: the
// root field answers null with this error at its path, HTTP 200.
func unfit(verb graph.QueryVerb, err error) error {
	return fmt.Errorf("reply of %s refused: %w", verb.Subject, err)
}

// entity answers {entity {id triples}, kvRevision}, refusing a missing entity or a zero kv_revision
// and decoding the stored value with graph.UnmarshalEntityState, as graph.ExactEntityReader does
// (graph/exact_entity.go, ADR-091 §4).
func (c *Component) entity(verb graph.QueryVerb) resolver {
	return func(ctx context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		reply, err := c.ask(ctx, verb, graph.EntityRequest{ID: id})
		if err != nil {
			return nil, err
		}
		var exact struct {
			Entity     json.RawMessage `json:"entity"`
			KVRevision uint64          `json:"kv_revision"`
		}
		if err := json.Unmarshal(reply, &exact); err != nil {
			return nil, unfit(verb, err)
		}
		if len(exact.Entity) == 0 || string(exact.Entity) == "null" {
			return nil, unfit(verb, errors.New("no entity"))
		}
		if exact.KVRevision == 0 {
			return nil, unfit(verb, errors.New("zero kv_revision"))
		}
		var state graph.EntityState
		if err := graph.UnmarshalEntityState(id, exact.Entity, &state); err != nil {
			return nil, unfit(verb, err)
		}
		return map[string]any{"entity": entityValue(state), "kvRevision": exact.KVRevision}, nil
	}
}

// entitiesByPrefix answers {entities, total, next_cursor} after graph.ValidateDecodedEntityStates. An
// empty prefix matches every entity, as graph-ingest's prefix verb reads it.
func (c *Component) entitiesByPrefix(verb graph.QueryVerb) resolver {
	return func(ctx context.Context, args map[string]any) (any, error) {
		request := graph.PrefixQueryRequest{}
		request.Prefix, _ = args["prefix"].(string)
		request.Cursor, _ = args["cursor"].(string)
		var err error
		if request.Limit, err = intArg(args, "limit"); err != nil {
			return nil, err
		}
		reply, err := c.ask(ctx, verb, request)
		if err != nil {
			return nil, err
		}
		var page graph.PrefixQueryResponse
		if err := json.Unmarshal(reply, &page); err != nil {
			return nil, unfit(verb, err)
		}
		if err := graph.ValidateDecodedEntityStates(page.Entities); err != nil {
			return nil, unfit(verb, err)
		}
		entities := make([]any, len(page.Entities))
		for i, state := range page.Entities {
			entities[i] = entityValue(state)
		}
		var next any
		if page.NextCursor != "" {
			next = page.NextCursor
		}
		return map[string]any{"entities": entities, "total": page.Total, "next_cursor": next}, nil
	}
}

// entitiesByPredicate answers {entities}: the IDs of the entities holding predicate, with value
// when it is given.
func (c *Component) entitiesByPredicate(verb graph.QueryVerb) resolver {
	return func(ctx context.Context, args map[string]any) (any, error) {
		request := graph.PredicateRequest{}
		request.Predicate, _ = args["predicate"].(string)
		if value, ok := args["value"].(string); ok {
			request.Value = &value
		}
		var err error
		if request.Limit, err = intArg(args, "limit"); err != nil {
			return nil, err
		}
		reply, err := c.ask(ctx, verb, request)
		if err != nil {
			return nil, err
		}
		var matches graph.PredicateData
		if err := json.Unmarshal(reply, &matches); err != nil {
			return nil, unfit(verb, err)
		}
		entities := make([]any, len(matches.Entities))
		for i, id := range matches.Entities {
			entities[i] = id
		}
		return map[string]any{"entities": entities}, nil
	}
}

// pathSearch answers {entities {id depth}, paths, truncated}. direction is sent lower-cased, and a
// null item in predicates is refused before anything is sent.
func (c *Component) pathSearch(verb graph.QueryVerb) resolver {
	return func(ctx context.Context, args map[string]any) (any, error) {
		request := graph.PathSearchRequest{}
		request.StartEntity, _ = args["startEntity"].(string)
		direction, _ := args["direction"].(string)
		request.Direction = strings.ToLower(direction)
		var err error
		if request.MaxDepth, err = intArg(args, "maxDepth"); err != nil {
			return nil, err
		}
		if request.MaxNodes, err = intArg(args, "maxNodes"); err != nil {
			return nil, err
		}
		if request.Predicates, err = predicatesArg(args); err != nil {
			return nil, err
		}
		reply, err := c.ask(ctx, verb, request)
		if err != nil {
			return nil, err
		}
		var found graph.PathSearchData
		if err := json.Unmarshal(reply, &found); err != nil {
			return nil, unfit(verb, err)
		}
		paths, err := pathsOf(found)
		if err != nil {
			return nil, unfit(verb, err)
		}
		entities := make([]any, len(found.Entities))
		for i, entity := range found.Entities {
			entities[i] = map[string]any{"id": entity.EntityID, "depth": entity.Depth}
		}
		return map[string]any{"entities": entities, "paths": paths, "truncated": found.Truncated}, nil
	}
}

// pathsOf aligns the paths with the entities, as the pin did (pin processor/graph-query/pathrag.go:139-140):
// paths[k] is the run of edges from the start to entities[k], and paths[0] is empty. edges[k-1]
// first reached entities[k] (graph/query_path_types.go), from the end of it that is not entities[k],
// which an earlier entity must be; the path is that entity's with the edge appended. A reply breaking
// that shape is refused, not answered in part.
func pathsOf(found graph.PathSearchData) ([]any, error) {
	if len(found.Entities) == 0 {
		return nil, errors.New("no start entity")
	}
	if len(found.Edges) != len(found.Entities)-1 {
		return nil, fmt.Errorf("%d edges for %d entities, want one fewer", len(found.Edges), len(found.Entities))
	}
	at := map[string]int{found.Entities[0].EntityID: 0}
	paths := []any{[]any{}}
	for k := 1; k < len(found.Entities); k++ {
		id, edge := found.Entities[k].EntityID, found.Edges[k-1]
		from := edge.FromEntityID
		if from == id {
			from = edge.ToEntityID
		} else if edge.ToEntityID != id {
			return nil, fmt.Errorf("edge %d does not reach entity %d (%s)", k-1, k, id)
		}
		parent, ok := at[from]
		if !ok {
			return nil, fmt.Errorf("edge %d reaches entity %d (%s) from %s, which no earlier entity is", k-1, k, id, from)
		}
		if _, seen := at[id]; seen {
			return nil, fmt.Errorf("entity %d (%s) is listed twice", k, id)
		}
		at[id] = k
		step := map[string]any{"from": edge.FromEntityID, "predicate": edge.Predicate, "to": edge.ToEntityID}
		paths = append(paths, append(append([]any{}, paths[parent].([]any)...), step))
	}
	return paths, nil
}

// entityValue is an entity keyed by the schema's names. A triple's timestamp is RFC 3339 text.
func entityValue(state graph.EntityState) map[string]any {
	triples := make([]any, len(state.Triples))
	for i, triple := range state.Triples {
		triples[i] = map[string]any{
			"subject":    triple.Subject,
			"predicate":  triple.Predicate,
			"object":     triple.Object,
			"source":     triple.Source,
			"timestamp":  triple.Timestamp.Format(time.RFC3339Nano),
			"confidence": triple.Confidence,
		}
	}
	return map[string]any{"id": state.ID, "triples": triples}
}

// intArg reads an optional Int argument as coerced by the validator: an int64 from a literal, a
// json.Number from a variable. Absent or null is 0, which every verb reads as its default.
func intArg(args map[string]any, name string) (int, error) {
	var n int64
	switch v := args[name].(type) {
	case nil:
		return 0, nil
	case int64:
		n = v
	case json.Number:
		parsed, err := v.Int64()
		if err != nil {
			return 0, &argumentError{fmt.Sprintf("argument %s: %v", name, err)}
		}
		n = parsed
	default:
		return 0, &argumentError{fmt.Sprintf("argument %s: %T is not an Int", name, v)}
	}
	if n < math.MinInt32 || n > math.MaxInt32 {
		return 0, &argumentError{fmt.Sprintf("argument %s: %d is outside Int's 32 bits", name, n)}
	}
	return int(n), nil
}

// predicatesArg reads pathSearch's predicates, a [String] as at the pin, refusing a null item: it
// would name no predicate, and dropping it would widen the answer.
func predicatesArg(args map[string]any) ([]string, error) {
	var items []any
	switch v := args["predicates"].(type) {
	case nil:
	case []any:
		items = v
	case string:
		// GraphQL input coercion: one value where a list is declared is a list of that one.
		items = []any{v}
	default:
		return nil, &argumentError{fmt.Sprintf("argument predicates: %T is not a [String]", v)}
	}
	var predicates []string
	for i, item := range items {
		predicate, ok := item.(string)
		if !ok {
			return nil, &argumentError{fmt.Sprintf("argument predicates: item %d is null", i)}
		}
		predicates = append(predicates, predicate)
	}
	return predicates, nil
}
