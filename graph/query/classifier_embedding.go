package query

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/c360studio/semengine/graph/embedding"
	"github.com/c360studio/semengine/pkg/errs"
)

// EmbeddingClassifier classifies queries by finding the most similar domain example: tier 1
// of the chain.
//
// It uses BM25 vectors only (graph/embedding's BM25Embedder, no external service), and its
// examples are fixed at construction. Embedding a query goes through Generate, which folds the
// query into the embedder's corpus statistics, so later scores depend on the queries seen so far
// (#96 keeps the move to the read-only GenerateQuery).
//
// Safe for concurrent FindBestMatch calls: the examples are never written after construction
// and the BM25 embedder locks its own statistics.
type EmbeddingClassifier struct {
	examples  []Example               // every loaded example, each with its vector
	embedder  *embedding.BM25Embedder // embeds the examples and each query
	threshold float64                 // minimum similarity for a match
}

// NewEmbeddingClassifier embeds every example of domains under ctx and returns a classifier
// whose matches need a similarity of at least threshold. Nil domains are skipped.
//
// It keeps the domains' examples and their Options maps; do not edit them after the call.
//
// It returns an error, and no classifier, when ctx is nil or has ended or when any example
// cannot be embedded; a classifier that could never match is never returned.
func NewEmbeddingClassifier(ctx context.Context, domains []*DomainExamples, threshold float64) (*EmbeddingClassifier, error) {
	if ctx == nil {
		return nil, errs.WrapInvalid(errors.New("nil context"), "EmbeddingClassifier", "NewEmbeddingClassifier", "missing context")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("embed examples: %w", err)
	}

	var examples []Example
	for _, domain := range domains {
		if domain == nil {
			continue
		}
		examples = append(examples, domain.Examples...)
	}

	bm25 := embedding.NewBM25Embedder(embedding.BM25Config{
		Dimensions: 384, // Standard embedding dimension
		K1:         1.5,
		B:          0.75,
	})

	if len(examples) > 0 {
		queries := make([]string, len(examples))
		for i, ex := range examples {
			queries[i] = ex.Query
		}
		vectors, err := bm25.Generate(ctx, queries)
		if err != nil {
			return nil, fmt.Errorf("embed examples: %w", err)
		}
		if len(vectors) != len(examples) {
			return nil, fmt.Errorf("embed examples: %d vectors for %d examples", len(vectors), len(examples))
		}
		for i := range examples {
			examples[i].Vector = vectors[i]
		}
	}

	return &EmbeddingClassifier{
		examples:  examples,
		embedder:  bm25,
		threshold: threshold,
	}, nil
}

// FindBestMatch returns the example most similar to query and its score.
//
// It returns (nil, 0) for a nil classifier, a blank query, a nil or ended context, or a query
// that cannot be embedded (which only an ended context causes); (nil, best) when the best score
// is below the threshold. Ties go to the first example in load order.
//
// The returned Example, its Options map and its Vector are copies: editing their fields,
// top-level entries or elements changes no later result. Values nested inside an Options entry
// are shared with the classifier.
func (c *EmbeddingClassifier) FindBestMatch(ctx context.Context, query string) (*Example, float64) {
	if c == nil || ctx == nil {
		return nil, 0.0
	}
	if strings.TrimSpace(query) == "" {
		return nil, 0.0
	}
	if ctx.Err() != nil {
		return nil, 0.0
	}

	vecs, err := c.embedder.Generate(ctx, []string{query})
	if err != nil || len(vecs) == 0 {
		return nil, 0.0
	}
	queryVec := vecs[0]

	best := -1
	var bestScore float64
	for i := range c.examples {
		// Check the context periodically for long example lists.
		if i%100 == 0 && ctx.Err() != nil {
			return nil, 0.0
		}
		score := embedding.CosineSimilarity(queryVec, c.examples[i].Vector)
		if score > bestScore {
			bestScore = score
			best = i
		}
	}

	if best < 0 || bestScore < c.threshold {
		return nil, bestScore
	}
	return copyExample(c.examples[best]), bestScore
}

// copyExample returns ex with its own Options map and Vector, so a caller's edits stay out of
// the classifier.
func copyExample(ex Example) *Example {
	ex.Options = maps.Clone(ex.Options)
	ex.Vector = slices.Clone(ex.Vector)
	return &ex
}

// Threshold returns the similarity threshold.
func (c *EmbeddingClassifier) Threshold() float64 {
	return c.threshold
}
