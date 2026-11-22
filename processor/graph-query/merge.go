package graphquery

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"time"

	"github.com/c360studio/semengine/graph"
)

// mergeK is the constant in each list's term of the merged score, 1 / (mergeK + place): the value the
// tier 1 scorecard was measured with. A larger one weighs a list's lower places closer to its first.
const mergeK = 60

// detailMerged is the merge hint's Detail when a search asked both searches for a merged list.
const detailMerged = "merged"

// mergedHit is one entity of a list a search answers: its ID, the score its match carries, and
// its 1-based place in the word list and in the embedding list, 0 when it is not in that list.
type mergedHit struct {
	entityID             string
	score                float64
	wordRank, vectorRank int
}

// mergeRanks merges a word list and an embedding list, each best first, into one: each entity once,
// scored the sum of 1 / (mergeK + place) over the lists holding it, an entity's first place in a list
// being its place there; ordered by score, highest first, then entity ID ascending. An empty
// embedding list leaves the word order, and an empty word list the embedding order.
func mergeRanks(words []graph.LexicalHit, vectors []graph.SimilarityHit) []mergedHit {
	merged := make([]mergedHit, 0, len(words)+len(vectors))
	at := make(map[string]int, len(words)+len(vectors))
	place := func(id string) int {
		i, ok := at[id]
		if !ok {
			i = len(merged)
			at[id] = i
			merged = append(merged, mergedHit{entityID: id})
		}
		return i
	}
	for n, hit := range words {
		i := place(hit.EntityID)
		if merged[i].wordRank == 0 {
			merged[i].wordRank = n + 1
		}
	}
	for n, hit := range vectors {
		i := place(hit.EntityID)
		if merged[i].vectorRank == 0 {
			merged[i].vectorRank = n + 1
		}
	}
	for i := range merged {
		hit := &merged[i]
		if hit.wordRank > 0 {
			hit.score += 1 / float64(mergeK+hit.wordRank)
		}
		if hit.vectorRank > 0 {
			hit.score += 1 / float64(mergeK+hit.vectorRank)
		}
	}
	slices.SortFunc(merged, func(a, b mergedHit) int {
		return cmp.Or(cmp.Compare(b.score, a.score), cmp.Compare(a.entityID, b.entityID))
	})
	return merged
}

// wordList is the word list as searchGraph reads it back when nothing is merged: the word order, the
// word index's scores, and each hit's place in it as its word rank.
func wordList(words []graph.LexicalHit) []mergedHit {
	list := make([]mergedHit, 0, len(words))
	for n, hit := range words {
		list = append(list, mergedHit{entityID: hit.EntityID, score: hit.Score, wordRank: n + 1})
	}
	return list
}

// unranked is the word list as global search reads it back: the word order and scores, with no ranks.
func unranked(words []graph.LexicalHit) []mergedHit {
	list := make([]mergedHit, 0, len(words))
	for _, hit := range words {
		list = append(list, mergedHit{entityID: hit.EntityID, score: hit.Score})
	}
	return list
}

// goSearch runs search in its own goroutine under a child of ctx, so the caller can send another
// request before waiting on this one. await waits for search's result and returns it, every call the
// same. stop cancels search's context and waits for search to return, discarding its result, so no
// goroutine outlives the caller: a request in flight ends with its context, never waiting for its
// reply. The caller defers stop, which is harmless after await.
func goSearch[S any](ctx context.Context, search func(context.Context) S) (await func() S, stop func()) {
	ctx, cancel := context.WithCancel(ctx)
	result := make(chan S, 1)
	go func() { result <- search(ctx) }()
	var got S
	received := false
	await = func() S {
		if !received {
			got, received = <-result, true
		}
		return got
	}
	stop = func() {
		cancel()
		await()
	}
	return await, stop
}

// semanticOutcome is the embedding search's reply, or the request's error as the client returned it,
// not yet classified or counted: a request stopped because the word search was refused is no
// failure of graph-query's.
type semanticOutcome struct {
	raw []byte
	err error
}

// semanticSearch asks the embedding search for question within scope, graph.MaxSimilarityLimit
// deep, waiting up to timeout for the reply, and no min_revision, which the embedding verbs refuse
// above 0.
func (c *Component) semanticSearch(ctx context.Context, subject, question string, scope []string, timeout time.Duration) semanticOutcome {
	body, err := json.Marshal(graph.SemanticRequest{Query: question, Limit: graph.MaxSimilarityLimit, Scope: scope})
	if err != nil {
		return semanticOutcome{err: internalError(err)}
	}
	raw, err := c.natsClient.RequestClassified(ctx, subject, body, timeout)
	return semanticOutcome{raw: raw, err: err}
}

// bothSearches is what the word search and the embedding search answered when asked together: the
// word search's reply, or notReady, its index_not_ready refusal, when the answer is the embedding
// list alone; and what was read of the embedding search.
type bothSearches struct {
	words    graph.QueryResponse[graph.LexicalSearchData]
	notReady error
	semantic semanticRead
}

// askBoth sends the word search for request and the embedding search for its query and scope, each
// graph.MaxSimilarityLimit deep whatever the request's limit, so a shorter limit is a prefix of a
// longer one. Both are sent before either is awaited, so the embedding wait is not added to the
// word wait. min_revision goes to the word search alone, since the embedding verbs refuse one above
// 0. The embedding search's reply is awaited up to embeddingWait.
//
// checkWords runs on the word hits before the embedding reply is awaited, so a broken word reply is
// refused without that wait. A word search refused index_not_ready answers from the embedding search
// alone, notReady set, when its reply was read, and otherwise returns that refusal, the first one.
// Any other word refusal, or checkWords' refusal, is returned as it arrived; the embedding request is
// then cancelled and the call waits for it to end, never for its reply, so nothing it started
// outlives it, and its end counts no error.
func (c *Component) askBoth(ctx context.Context, lexicalSubject, semanticSubject string, request graph.LexicalSearchRequest, embeddingWait time.Duration, checkWords func([]graph.LexicalHit) error) (bothSearches, error) {
	await, stop := goSearch(ctx, func(ctx context.Context) semanticOutcome {
		return c.semanticSearch(ctx, semanticSubject, request.Query, request.Scope, embeddingWait)
	})
	defer stop()
	request.Limit = graph.MaxSimilarityLimit
	words, err := c.wordSearch(ctx, lexicalSubject, request)
	if err != nil {
		if !isIndexNotReady(err) {
			return bothSearches{}, err
		}
		semantic := c.readSemantic(semanticSubject, await(), graph.ErrorCodeIndexNotReady)
		if !semantic.hint.Honoured {
			return bothSearches{}, err
		}
		return bothSearches{notReady: err, semantic: semantic}, nil
	}
	if err := checkWords(words.Data.Results); err != nil {
		return bothSearches{}, err
	}
	return bothSearches{words: words, semantic: c.readSemantic(semanticSubject, await(), detailMerged)}, nil
}

// merge is the list both searches answer, best first, before any limit: the merge of the word list
// and the embedding list (mergeRanks) when the embedding list was read and checkVectors accepts its
// hits; otherwise the word list in its own order, with the merge hint not honoured, naming why. A
// refusal from checkVectors is the embedding index's broken reply.
func (b *bothSearches) merge(checkVectors func([]graph.SimilarityHit) error) []mergedHit {
	words := b.words.Data.Results
	if !b.semantic.hint.Honoured {
		return wordList(words)
	}
	if err := checkVectors(b.semantic.hits); err != nil {
		b.semantic = semanticRead{hint: notHonouredBy(b.semantic.hint, err)}
		return wordList(words)
	}
	return mergeRanks(words, b.semantic.hits)
}

// declareMergeFailed declares an answer of verb given as the word list alone because the embedding
// search failed, as hint names: one Warn naming the verb, the failure and why answering is safe, and
// one count on the merge-failed counter the search verbs share.
func (c *Component) declareMergeFailed(verb string, hint graph.GlobalSearchHint) {
	c.metrics.searchMergeFailed.Inc()
	c.logger.Warn(verb+" answered the word list alone: the embedding search failed",
		slog.String("verb", verb), slog.String("code", hint.Code), slog.String("error", hint.Reason),
		slog.String("reason", "the reply names the failed merge, and the word list is the answer "+verb+" gives with no embedding search"))
}

// semanticRead is what a search read of the embedding search: the hint naming the step, honoured
// with the reply's data as its bytes came, its hits and its damaged count, or not honoured naming the
// refusal, the timeout (an internal refusal, once the wait has passed) or the broken reply.
type semanticRead struct {
	hint    graph.GlobalSearchHint
	data    json.RawMessage
	hits    []graph.SimilarityHit
	damaged int
}

// readSemantic reads outcome, the embedding search's reply on subject, for a hint with detail. It
// never refuses: the caller decides what a failed embedding search answers. A failure that is
// graph-query's own (no answer, a reply it cannot use) counts its error here.
func (c *Component) readSemantic(subject string, outcome semanticOutcome, detail string) semanticRead {
	hint := graph.GlobalSearchHint{Hint: hintSemantic, Detail: detail}
	failed := func(err error) semanticRead { return semanticRead{hint: notHonouredBy(hint, err)} }
	if outcome.err != nil {
		return failed(c.requestFailed(subject, outcome.err))
	}
	reply, err := decodeIndexReply[json.RawMessage](c, subject, outcome.raw)
	if err != nil {
		return failed(err)
	}
	var data graph.SemanticData
	if err := json.Unmarshal(reply.Data, &data); err != nil {
		return failed(c.brokenReply(subject, err))
	}
	// Data of {}, null or {"results":null} decodes without error but lists nothing: a reply with no
	// results list is broken, not an empty one, which is {"results":[]}.
	if data.Results == nil {
		return failed(c.brokenReply(subject, errors.New("holds no results")))
	}
	hint.Honoured = true
	return semanticRead{hint: hint, data: reply.Data, hits: data.Results, damaged: reply.DamagedEntities}
}
