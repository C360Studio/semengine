package graphquery

import (
	"cmp"
	"context"
	"encoding/json"
	"slices"

	"github.com/c360studio/semengine/graph"
)

// mergeK is the constant in each list's term of the merged score, 1 / (mergeK + place): the value the
// tier 1 scorecard was measured with. A larger one weighs a list's lower places closer to its first.
const mergeK = 60

// detailMerged is the merge hint's Detail when searchGraph asked both searches for a merged list.
const detailMerged = "merged"

// mergedHit is one entity of a list searchGraph reads back: its ID, the score its match carries, and
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

// semanticSearch asks the embedding search for question, graph.MaxSimilarityLimit deep, and no
// min_revision, which the embedding verbs refuse above 0.
func (c *Component) semanticSearch(ctx context.Context, subject, question string) semanticOutcome {
	body, err := json.Marshal(graph.SemanticRequest{Query: question, Limit: graph.MaxSimilarityLimit})
	if err != nil {
		return semanticOutcome{err: internalError(err)}
	}
	raw, err := c.natsClient.RequestClassified(ctx, subject, body, c.queryTimeout())
	return semanticOutcome{raw: raw, err: err}
}

// semanticRead is what searchGraph read of the embedding search: the hint naming the step, honoured
// with the reply's data as its bytes came and its hits, or not honoured naming the refusal, the
// timeout (an internal refusal, after query_timeout_ms) or the broken reply.
type semanticRead struct {
	hint graph.GlobalSearchHint
	data json.RawMessage
	hits []graph.SimilarityHit
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
	hint.Honoured = true
	return semanticRead{hint: hint, data: reply.Data, hits: data.Results}
}
