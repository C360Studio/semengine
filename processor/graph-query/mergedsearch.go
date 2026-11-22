package graphquery

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/c360studio/semengine/graph"
	semtypes "github.com/c360studio/semengine/pkg/types"
)

// maxEmbeddingWait is the longest mergedSearch waits for the embedding search. A caller on
// natsclient's default request timeout of 5 seconds then has about a second left for the word search
// and the reply, so a slow embedder gives it the word answer rather than a timeout.
const maxEmbeddingWait = 4 * time.Second

// mergedSearch answers a graph.LexicalSearchRequest with the ranked entity IDs alone,
// graph.MergedSearchData, and reads no entity. With semanticSubject "" (the composition runs no
// graph-embedding) it is the word list. Otherwise it asks both searches together (askBoth), the
// embedding search waiting up to query_timeout_ms or maxEmbeddingWait, whichever is less, and answers
// the first limit of their merge, as searchGraph ranks them. A word hit whose ID is not a literal
// entity ID is the word index's broken reply, refused; an embedding hit's leaves the word list. An
// answer given as the word list alone because the embedding search failed is a declared degrade:
// once the reply is encoded it logs one Warn and adds one to the merge-failed counter.
//
// The reply is under the word search's revision, carrying the larger damaged count of the replies
// read. When the word index is not ready and the embedding search answers, the results are the
// embedding list, under revision 0, since no ENTITY_STATES revision was read; when the embedding
// search fails too, the word search's index_not_ready refusal is returned as it arrived.
func (c *Component) mergedSearch(lexicalSubject, semanticSubject string) queryHandler {
	return func(ctx context.Context, data []byte) ([]byte, error) {
		c.lifecycle.CountInput(len(data))
		request, err := decodeMergedSearchRequest(data)
		if err != nil {
			return nil, invalidRequest(err)
		}
		if semanticSubject == "" {
			return c.wordsAlone(ctx, lexicalSubject, request)
		}
		return c.mergedIDs(ctx, lexicalSubject, semanticSubject, request)
	}
}

// decodeMergedSearchRequest decodes strictly, refusing any key but query, limit, scope and
// min_revision by name, a blank query, a limit other than 0 or 1 to graph.MaxSimilarityLimit, and
// a scope member other than "" (every entity) that is not an entity-ID prefix.
func decodeMergedSearchRequest(data []byte) (graph.LexicalSearchRequest, error) {
	var request graph.LexicalSearchRequest
	if err := decodeStrict(data, &request); err != nil {
		return request, err
	}
	if strings.TrimSpace(request.Query) == "" {
		return request, errors.New("query is blank")
	}
	if err := checkSearchLimit(request.Limit); err != nil {
		return request, err
	}
	for i, prefix := range request.Scope {
		if prefix == "" {
			continue
		}
		if err := semtypes.ValidateEntityIDPrefix(prefix); err != nil {
			return request, fmt.Errorf("scope[%d]: %w", i, err)
		}
	}
	return request, nil
}

// wordsAlone is mergedSearch when the composition runs no graph-embedding: the word search asked
// with the request unchanged, answered as its list with the word scores and no merge.
func (c *Component) wordsAlone(ctx context.Context, lexicalSubject string, request graph.LexicalSearchRequest) ([]byte, error) {
	words, err := c.wordSearch(ctx, lexicalSubject, request)
	if err != nil {
		return nil, err
	}
	if err := c.literalWordIDs(lexicalSubject, words.Data.Results); err != nil {
		return nil, err
	}
	answer := graph.MergedSearchData{
		Results: mergedResults(wordList(words.Data.Results)), Excluded: words.Data.Excluded, BodiesNotIndexed: words.Data.BodiesNotIndexed,
	}
	return envelope(c, lexicalSubject, readOf(words), answer)
}

// mergedIDs is mergedSearch when the composition runs graph-embedding.
func (c *Component) mergedIDs(ctx context.Context, lexicalSubject, semanticSubject string, request graph.LexicalSearchRequest) ([]byte, error) {
	limit := cmp.Or(request.Limit, graph.DefaultSimilarityLimit)
	both, err := c.askBoth(ctx, lexicalSubject, semanticSubject, request, min(c.queryTimeout(), maxEmbeddingWait),
		func(words []graph.LexicalHit) error { return c.literalWordIDs(lexicalSubject, words) })
	if err != nil {
		return nil, err
	}
	checkVectors := func(vectors []graph.SimilarityHit) error { return c.literalVectorIDs(semanticSubject, vectors) }
	if both.notReady != nil {
		if err := checkVectors(both.semantic.hits); err != nil {
			return nil, both.notReady
		}
		list := mergeRanks(nil, both.semantic.hits)
		answer := graph.MergedSearchData{Results: mergedResults(list[:min(limit, len(list))]), Merge: &both.semantic.hint}
		return envelope(c, semanticSubject, indexRead{damaged: both.semantic.damaged}, answer)
	}
	list := both.merge(checkVectors)
	words := both.words
	answer := graph.MergedSearchData{
		Results: mergedResults(list[:min(limit, len(list))]), Excluded: words.Data.Excluded,
		BodiesNotIndexed: words.Data.BodiesNotIndexed, Merge: &both.semantic.hint,
	}
	read := indexRead{revision: words.IndexedRevision, damaged: max(words.DamagedEntities, both.semantic.damaged)}
	encoded, err := envelope(c, lexicalSubject, read, answer)
	if err != nil {
		return nil, err
	}
	if !both.semantic.hint.Honoured {
		c.declareMergeFailed("mergedSearch", both.semantic.hint)
	}
	return encoded, nil
}

// mergedResults is list as mergedSearch answers it: each entity's ID and score, in list's order.
func mergedResults(list []mergedHit) []graph.MergedSearchHit {
	results := make([]graph.MergedSearchHit, 0, len(list))
	for _, hit := range list {
		results = append(results, graph.MergedSearchHit{EntityID: hit.entityID, Score: hit.score})
	}
	return results
}

// literalWordIDs refuses, as the word index's broken reply, word hits holding an ID that is not a
// literal entity ID.
func (c *Component) literalWordIDs(lexicalSubject string, hits []graph.LexicalHit) error {
	for _, hit := range hits {
		if err := semtypes.ValidateEntityID(hit.EntityID); err != nil {
			return c.brokenReply(lexicalSubject, err)
		}
	}
	return nil
}

// literalVectorIDs refuses, as the embedding index's broken reply, embedding hits holding an ID that
// is not a literal entity ID.
func (c *Component) literalVectorIDs(semanticSubject string, hits []graph.SimilarityHit) error {
	for _, hit := range hits {
		if err := semtypes.ValidateEntityID(hit.EntityID); err != nil {
			return c.brokenReply(semanticSubject, err)
		}
	}
	return nil
}
