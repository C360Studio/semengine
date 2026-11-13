package graphembedding

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/embedding"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/graphview"
	semtypes "github.com/c360studio/semengine/pkg/types"
)

// queryHandlers maps each verb graph-embedding answers, by its name in graph.QueryVerbs, to its
// handler.
func (c *Component) queryHandlers() map[string]func(context.Context, []byte) ([]byte, error) {
	return map[string]func(context.Context, []byte) ([]byte, error){
		"search":  c.handleSearch,
		"similar": c.handleSimilar,
	}
}

// subscribeQueries subscribes to the subject of every verb the table in graph declares for
// graph-embedding, and to nothing else. A declared verb with no handler here fails
// Start. Each subscription is recorded as it is made, so a Start that fails part way drains the
// ones already made.
func (c *Component) subscribeQueries(ctx context.Context) error {
	handlers := c.queryHandlers()
	var subjects []string
	for _, verb := range graph.QueryVerbs() {
		if verb.Responder != graph.ResponderGraphEmbedding {
			continue
		}
		handler, ok := handlers[verb.Name]
		if !ok {
			return fmt.Errorf("query verb %q (%s) is declared for graph-embedding but has no handler", verb.Name, verb.Subject)
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

// handleSearch answers the entities whose vectors best match a text query. Every
// entry check runs before the query is embedded: the request decodes strictly, min_revision is 0,
// the limit is in range, the query is not blank, and each scope prefix is well formed. No fixed
// timeout: the request's context bounds the work.
func (c *Component) handleSearch(ctx context.Context, data []byte) ([]byte, error) {
	var req graph.SemanticRequest
	if err := decodeRequest(data, &req); err != nil {
		return nil, err
	}
	limit, err := checkCommon(req.MinRevision, req.Limit)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Query) == "" {
		return nil, invalidRequest(errors.New("query is blank"))
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

	if err := c.queryReady(ctx); err != nil {
		return nil, err
	}
	vectors, err := c.embedder.GenerateQuery(ctx, []string{req.Query})
	if err != nil {
		return nil, internalError(fmt.Errorf("embed the query: %w", err))
	}
	if len(vectors) != 1 {
		return nil, internalError(fmt.Errorf("embed the query: %d vectors for one text", len(vectors)))
	}
	var keep func(string) bool
	if len(req.Scope) > 0 {
		keep = func(id string) bool { return graph.MatchesAnyIDPrefix(id, req.Scope) }
	}
	results, unreadable, err := c.rank("", vectors[0], keep, limit)
	if err != nil {
		return nil, err
	}
	return reply(graph.SemanticData{
		Results:           results,
		EmbedderType:      c.config.EmbedderType,
		UnreadableRecords: unreadable,
		FailedRecords:     c.workerFailures(),
	})
}

// handleSimilar answers the entities whose vectors are most like a given entity's, never the
// entity itself. The source vector is read from EMBEDDING_INDEX, not the cache.
func (c *Component) handleSimilar(ctx context.Context, data []byte) ([]byte, error) {
	var req graph.SimilarRequest
	if err := decodeRequest(data, &req); err != nil {
		return nil, err
	}
	limit, err := checkCommon(req.MinRevision, req.Limit)
	if err != nil {
		return nil, err
	}
	if err := semtypes.ValidateEntityID(req.EntityID); err != nil {
		return nil, invalidRequest(fmt.Errorf("entity_id: %w", err))
	}

	if err := c.queryReady(ctx); err != nil {
		return nil, err
	}
	record, err := c.storage.GetEmbedding(ctx, req.EntityID)
	if err != nil {
		// An undecodable source record stays class invalid with no code, as GetEmbedding returns
		// it: graph-clustering counts it toward its abort rather than reading it as "no neighbours".
		if errs.IsInvalid(err) {
			return nil, err
		}
		return nil, internalError(fmt.Errorf("read the source embedding: %w", err))
	}
	if record == nil || record.Status != embedding.StatusGenerated || len(record.Vector) == 0 {
		return nil, errs.ClassifiedCode(errs.ErrorInvalid, graph.ErrorCodeEmbeddingUnavailable,
			fmt.Errorf("entity %s has no generated embedding", req.EntityID))
	}
	results, unreadable, err := c.rank(req.EntityID, record.Vector, nil, limit)
	if err != nil {
		return nil, err
	}
	return reply(graph.SimilarData{Results: results, UnreadableRecords: unreadable, FailedRecords: c.workerFailures()})
}

// queryReady refuses a request the verbs cannot answer truthfully now, after its entry checks and
// before any read: the follower's QueryReady (a latched reset fatal, a lost feed, an
// incomplete first build or a held hop-1 failure transient index_not_ready), then a lost vector
// cache watch, transient index_not_ready. similar reads its source from the bucket before the
// cache, so the lost watch is refused here rather than left to the cache.
func (c *Component) queryReady(ctx context.Context) error {
	if err := c.follower.Load().QueryReady(ctx); err != nil {
		return err
	}
	if c.cacheLost.Load() {
		return errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeIndexNotReady,
			errors.New("index not ready: the vector cache watch is lost; restart the process"))
	}
	return nil
}

// workerFailures is a reply's failed_records: the entities in the failed map whose latest outcome
// is a Worker failure. A hop-1 failure is the repair lane's and is not an embedding that failed.
func (c *Component) workerFailures() int {
	_, reasons := c.failed.counts()
	n := 0
	for reason, count := range reasons {
		if !repaired(reason) {
			n += int(count)
		}
	}
	return n
}

// rank returns the cache's best matches for query, up to limit, keeping only those scoring above
// 0: a zero vector, a stopword-only query's, scores every entity 0, and a 0 is no
// match. The cache sorts the full match set before the limit and scores only the entities keep
// accepts, so dropping the zeros after the limit loses no positive hit: they sort last. results
// is never nil, so it encodes as [].
func (c *Component) rank(exclude string, query []float32, keep func(string) bool, limit int) ([]graph.SimilarityHit, int, error) {
	scored, unreadable, err := c.cache.FindSimilar(exclude, query, keep, limit)
	if err != nil {
		if errors.Is(err, graphview.ErrNotReady) {
			return nil, 0, errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeIndexNotReady,
				fmt.Errorf("index not ready: the vector cache cannot answer: %w", err))
		}
		return nil, 0, internalError(err)
	}
	results := make([]graph.SimilarityHit, 0, len(scored))
	for _, hit := range scored {
		if hit.Similarity > 0 {
			results = append(results, graph.SimilarityHit{EntityID: hit.EntityID, Similarity: hit.Similarity})
		}
	}
	return results, unreadable, nil
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

// checkCommon refuses a min_revision above 0 and a limit outside 0 to graph.MaxSimilarityLimit,
// and returns the limit to use: 0 means graph.DefaultSimilarityLimit. A refused
// min_revision is class invalid, so no caller retries it.
func checkCommon(minRevision uint64, limit int) (int, error) {
	if minRevision > 0 {
		return 0, invalidRequest(fmt.Errorf(
			"min_revision %d: the embedding verbs claim no ENTITY_STATES revision; send 0", minRevision))
	}
	switch {
	case limit == 0:
		return graph.DefaultSimilarityLimit, nil
	case limit < 0 || limit > graph.MaxSimilarityLimit:
		return 0, invalidRequest(fmt.Errorf("limit %d is outside 0 to %d", limit, graph.MaxSimilarityLimit))
	}
	return limit, nil
}

// reply is a successful reply: data in the graph.QueryResponse envelope, producer
// graph-embedding, at indexed_revision 0, which claims nothing.
func reply[T any](data T) ([]byte, error) {
	response, err := graph.NewQueryResponse(graph.ResponderGraphEmbedding, 0, data)
	if err != nil {
		return nil, internalError(err)
	}
	return json.Marshal(response)
}

func invalidRequest(err error) error {
	return errs.ClassifiedCode(errs.ErrorInvalid, graph.ErrorCodeInvalidRequest, fmt.Errorf("invalid request: %w", err))
}

func internalError(err error) error {
	return errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeInternal, fmt.Errorf("internal error: %w", err))
}
