package graphlexical

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/lexical"
	"github.com/c360studio/semengine/pkg/errs"
	semtypes "github.com/c360studio/semengine/pkg/types"
)

// queryHandlers maps each verb graph-lexical answers, by its name in graph.QueryVerbs, to its
// handler.
func (c *Component) queryHandlers() map[string]func(context.Context, []byte) ([]byte, error) {
	return map[string]func(context.Context, []byte) ([]byte, error){
		"search": c.handleSearch,
	}
}

// subscribeQueries subscribes to the subject of every verb the table in graph declares for
// graph-lexical, and to nothing else. A declared verb with no handler here fails Start. Each
// subscription is recorded as it is made, so a Start that fails part way drains the ones already
// made.
func (c *Component) subscribeQueries(ctx context.Context) error {
	handlers := c.queryHandlers()
	var subjects []string
	for _, verb := range graph.QueryVerbs() {
		if verb.Responder != graph.ResponderGraphLexical {
			continue
		}
		handler, ok := handlers[verb.Name]
		if !ok {
			return fmt.Errorf("query verb %q (%s) is declared for graph-lexical but has no handler", verb.Name, verb.Subject)
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

// handleSearch answers the entities whose words best match a query. The request is checked
// first: it decodes strictly, the limit is in range, each scope prefix is well formed and the
// query parses. Then the follower's readiness (QueryReady), then min_revision against the
// follower's floor, read before the search: every revision at or below it is in the index the
// search reads, so the reply's indexed_revision is a lower bound on what it reflects.
func (c *Component) handleSearch(ctx context.Context, data []byte) ([]byte, error) {
	var req graph.LexicalSearchRequest
	if err := decodeRequest(data, &req); err != nil {
		return nil, err
	}
	limit, err := searchLimit(req.Limit)
	if err != nil {
		return nil, err
	}
	// "" is an explicit match-all (graph.MatchesAnyIDPrefix); any other element is a prefix.
	for i, prefix := range req.Scope {
		if prefix == "" {
			continue
		}
		if err := semtypes.ValidateEntityIDPrefix(prefix); err != nil {
			return nil, invalidRequest(fmt.Errorf("scope[%d]: %w", i, err))
		}
	}
	query, err := lexical.ParseQuery(req.Query)
	if err != nil {
		return nil, invalidRequest(err)
	}

	f := c.follower.Load()
	if err := f.QueryReady(ctx); err != nil {
		return nil, err
	}
	revision := f.Indexed()
	if req.MinRevision > revision {
		return nil, errs.ClassifiedCodeDetail(errs.ErrorTransient, graph.ErrorCodeRevisionNotReached,
			map[string]any{"indexed_revision": revision},
			fmt.Errorf("min_revision %d is above the indexed revision %d", req.MinRevision, revision))
	}

	result, err := c.apply.index.Search(query, lexical.Options{Scope: req.Scope, Limit: limit})
	if err != nil {
		return nil, internalError(err)
	}
	hits := make([]graph.LexicalHit, 0, len(result.Hits))
	for _, hit := range result.Hits {
		hits = append(hits, graph.LexicalHit{EntityID: hit.EntityID, Score: hit.Score})
	}
	response, err := graph.NewQueryResponse(graph.ResponderGraphLexical, revision, graph.LexicalSearchData{
		Results:          hits,
		Matched:          result.Matched,
		BodiesNotIndexed: c.apply.notIndexed(),
	})
	if err != nil {
		return nil, internalError(err)
	}
	return json.Marshal(response)
}

// searchLimit returns the limit to use: 0 means graph.DefaultSimilarityLimit, 1 to
// graph.MaxSimilarityLimit is used as given, and any other value is refused.
func searchLimit(limit int) (int, error) {
	switch {
	case limit == 0:
		return graph.DefaultSimilarityLimit, nil
	case limit < 0 || limit > graph.MaxSimilarityLimit:
		return 0, invalidRequest(fmt.Errorf("limit %d is outside 0 to %d", limit, graph.MaxSimilarityLimit))
	}
	return limit, nil
}

// decodeRequest decodes data into req strictly: a key req does not define is refused naming it,
// and so is any data after the JSON object.
func decodeRequest(data []byte, req any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(req); err != nil {
		return invalidRequest(err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return invalidRequest(errors.New("data after the request object"))
	}
	return nil
}

func invalidRequest(err error) error {
	return errs.ClassifiedCode(errs.ErrorInvalid, graph.ErrorCodeInvalidRequest, fmt.Errorf("invalid request: %w", err))
}

func internalError(err error) error {
	return errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeInternal, fmt.Errorf("internal error: %w", err))
}
