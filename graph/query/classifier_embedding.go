package query

import (
	"bytes"
	"context"
	"encoding/json"
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
// examples are fixed at construction: construction embeds them as documents with Generate, which
// builds the corpus statistics, and FindBestMatch embeds each query with the read-only
// GenerateQuery, so a query's score does not depend on the queries classified before it (#96).
//
// Safe for concurrent FindBestMatch calls: neither the examples nor the corpus statistics are
// written after construction.
type EmbeddingClassifier struct {
	examples  []Example               // every loaded example, each with its vector
	embedder  *embedding.BM25Embedder // embeds the examples and each query
	threshold float64                 // minimum similarity for a match
}

// NewEmbeddingClassifier embeds every example of domains under ctx and returns a classifier
// whose matches need a similarity of at least threshold. Nil domains are skipped.
//
// Each option whose name is a SearchOptions JSON name is decoded to the Go type tiers 0 and 3
// give it (*TimeRange, *SpatialBounds, []string, int, bool or string; strategy as a string); other
// names are kept unchanged. The classifier keeps its own copy of each example and its Options
// map; values under other names are shared with the caller.
//
// It returns an error, and no classifier, when ctx is nil or has ended, when any example cannot
// be embedded, or when a known option is null, does not decode, or is impossible: a strategy that is not a
// SearchStrategy value, a time range with a zero start or end or ending before it starts, geo
// bounds with North or South outside [-90, 90] or South above North, or a negative limit. The
// error names the domain, the example and the option. A classifier that could never match is
// never returned.
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
		for _, ex := range domain.Examples {
			options, err := typedOptions(ex.Options)
			if err != nil {
				return nil, errs.WrapInvalid(fmt.Errorf("domain %q, example %q: %w", domain.Domain, ex.Query, err),
					"EmbeddingClassifier", "NewEmbeddingClassifier", "decode example options")
			}
			ex.Options = options
			examples = append(examples, ex)
		}
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

	vecs, err := c.embedder.GenerateQuery(ctx, []string{query})
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

// typedOptions returns a copy of options in which each SearchOptions JSON name holds the Go type
// tiers 0 and 3 give it, or an error naming the first option that does not decode or is
// impossible. It returns nil for nil options.
func typedOptions(options map[string]any) (map[string]any, error) {
	if options == nil {
		return nil, nil
	}
	typed := make(map[string]any, len(options))
	for name, value := range options {
		v, err := typedOption(name, value)
		if err != nil {
			return nil, fmt.Errorf("option %q: %w", name, err)
		}
		typed[name] = v
	}
	return typed, nil
}

// typedOption decodes one option through SearchOptions' own JSON decoding and returns the field's
// value. A name that is not a SearchOptions JSON name is the caller's own and comes back unchanged.
//
// TestEveryHintNameIsTyped fails when a SearchOptions field has no case here.
func typedOption(name string, value any) (any, error) {
	var o SearchOptions
	encoded, err := json.Marshal(value)
	if err == nil {
		var raw []byte
		raw, err = json.Marshal(map[string]json.RawMessage{name: encoded})
		if err == nil {
			err = json.Unmarshal(raw, &o)
		}
	}

	var typed any
	switch name {
	case "query":
		typed = o.Query
	case "geo_bounds":
		typed = o.GeoBounds
	case "time_range":
		typed = o.TimeRange
	case "predicates":
		typed = o.Predicates
	case "types":
		typed = o.Types
	case "key_terms":
		typed = o.KeyTerms
	case "use_embeddings":
		typed = o.UseEmbeddings
	case "strategy":
		typed = string(o.Strategy)
	case "limit":
		typed = o.Limit
	case "path_intent":
		typed = o.PathIntent
	case "path_start_node":
		typed = o.PathStartNode
	case "path_predicates":
		typed = o.PathPredicates
	case "aggregation_type":
		typed = o.AggregationType
	case "aggregation_field":
		typed = o.AggregationField
	case "ranking_intent":
		typed = o.RankingIntent
	default:
		return value, nil
	}
	if err != nil {
		return nil, err
	}
	// JSON null under a known name would decode to the field's zero value ("limit": null gives 0):
	// refused under every known name, as one rule. The encoded form is checked, so a typed nil
	// pointer or slice and a json.RawMessage holding null are refused too.
	if bytes.Equal(encoded, []byte("null")) {
		return nil, errors.New("null value")
	}
	if err := checkOption(name, &o); err != nil {
		return nil, err
	}
	return typed, nil
}

// checkOption refuses the impossible values tier 3 and the keyword tier never give: the checks of
// design D4 (#478).
func checkOption(name string, o *SearchOptions) error {
	switch name {
	case "strategy":
		if !isValidStrategy(o.Strategy) {
			return fmt.Errorf("strategy %q is not a SearchStrategy value", o.Strategy)
		}
	case "time_range":
		tr := o.TimeRange
		if tr == nil || tr.Start.IsZero() || tr.End.IsZero() {
			return errors.New("time range needs a start and an end")
		}
		if tr.End.Before(tr.Start) {
			return errors.New("time range ends before it starts")
		}
	case "geo_bounds":
		b := o.GeoBounds
		if b == nil {
			return errors.New("geo bounds missing")
		}
		if b.North < -90 || b.North > 90 || b.South < -90 || b.South > 90 {
			return errors.New("geo bounds North and South must lie in [-90, 90]")
		}
		if b.South > b.North {
			return errors.New("geo bounds South is above North")
		}
	case "limit":
		if o.Limit < 0 {
			return fmt.Errorf("limit %d is negative", o.Limit)
		}
	}
	return nil
}
