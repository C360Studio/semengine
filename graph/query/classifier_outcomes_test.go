package query

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/c360studio/semengine/pkg/errs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingLLMClient is a fake LLMClient that counts its calls and answers with
// respond.
type countingLLMClient struct {
	calls   atomic.Int64
	respond func(ctx context.Context) (string, error)
}

func (c *countingLLMClient) ClassifyQuery(ctx context.Context, _ string) (string, error) {
	c.calls.Add(1)
	return c.respond(ctx)
}

// noHintQuery matches none of the keyword tier's patterns and no example in
// productExamples, so the chain reaches its later tiers.
const noHintQuery = "tell me about the pumps"

// defaultResult is design D4's default result, written out from the design.
func defaultResult() *ClassificationResult {
	return &ClassificationResult{Tier: 0, Intent: "", Options: map[string]any{}, Confidence: 1.0}
}

func endedContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	return ctx
}

// TestChainEndedContextReturnsNil: design D4's nil-context and ended-context
// rows. A chain never answers the default result for an ended context.
func TestChainEndedContextReturnsNil(t *testing.T) {
	t.Run("nil context", func(t *testing.T) {
		chain := NewClassifierChain(NewKeywordClassifier(), createTestEmbeddingClassifier(t))
		var nilCtx context.Context
		assert.Nil(t, chain.ClassifyQuery(nilCtx, noHintQuery))
	})

	t.Run("ended at entry", func(t *testing.T) {
		client := &countingLLMClient{respond: func(context.Context) (string, error) {
			return `{"strategy":"semantic"}`, nil
		}}
		chain := NewClassifierChain(NewKeywordClassifier(), nil, NewLLMClassifier(client, nil))
		assert.Nil(t, chain.ClassifyQuery(endedContext(t), noHintQuery))
		assert.Zero(t, client.calls.Load(), "no tier runs for an ended context")
	})

	t.Run("ended during the LLM tier", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		client := &countingLLMClient{respond: func(ctx context.Context) (string, error) {
			cancel()
			return "", ctx.Err()
		}}
		chain := NewClassifierChain(NewKeywordClassifier(), nil, NewLLMClassifier(client, nil))
		assert.Nil(t, chain.ClassifyQuery(ctx, noHintQuery))
		assert.Equal(t, int64(1), client.calls.Load())
	})
}

// TestChainLLMFailureGivesDefault: an LLM error under a live context yields the
// default result (design D4).
func TestChainLLMFailureGivesDefault(t *testing.T) {
	client := &countingLLMClient{respond: func(context.Context) (string, error) {
		return "", errors.New("provider unavailable")
	}}
	chain := NewClassifierChain(NewKeywordClassifier(), nil, NewLLMClassifier(client, nil))

	got := chain.ClassifyQuery(t.Context(), noHintQuery)

	assert.Equal(t, defaultResult(), got)
	assert.Equal(t, int64(1), client.calls.Load(), "the LLM tier was tried")
}

// TestChainKeywordHintStopsTheLadder: a keyword hint answers at tier 0 and no
// later tier runs.
func TestChainKeywordHintStopsTheLadder(t *testing.T) {
	client := &countingLLMClient{respond: func(context.Context) (string, error) {
		return `{"strategy":"semantic"}`, nil
	}}
	chain := NewClassifierChain(NewKeywordClassifier(), nil, NewLLMClassifier(client, nil))

	got := chain.ClassifyQuery(t.Context(), "events from yesterday")

	require.NotNil(t, got)
	assert.Equal(t, 0, got.Tier)
	assert.Equal(t, 1.0, got.Confidence)
	assert.Contains(t, got.Options, "time_range")
	assert.Zero(t, client.calls.Load(), "the LLM client was never called")
}

// TestLLMClassifierRefusesNilContext: a nil context is refused as invalid input
// before the client runs.
func TestLLMClassifierRefusesNilContext(t *testing.T) {
	client := &countingLLMClient{respond: func(context.Context) (string, error) {
		return `{"strategy":"semantic"}`, nil
	}}
	c := NewLLMClassifier(client, nil)

	var nilCtx context.Context
	result, err := c.ClassifyQuery(nilCtx, "query")

	assert.Nil(t, result)
	require.Error(t, err)
	assert.True(t, errs.IsInvalid(err), "want an invalid-input error, got %v", err)
	assert.Zero(t, client.calls.Load(), "the client was never called")
}

// TestNewEmbeddingClassifierRefusesEndedContext: construction embeds under the
// caller's context and refuses a nil or ended one with no classifier.
func TestNewEmbeddingClassifierRefusesEndedContext(t *testing.T) {
	domains := []*DomainExamples{{
		Domain:   "test",
		Examples: []Example{{Query: "test query", Intent: "test"}},
	}}

	t.Run("ended", func(t *testing.T) {
		c, err := NewEmbeddingClassifier(endedContext(t), domains, 0.5)
		assert.Nil(t, c)
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("nil", func(t *testing.T) {
		var nilCtx context.Context
		c, err := NewEmbeddingClassifier(nilCtx, domains, 0.5)
		assert.Nil(t, c)
		require.Error(t, err)
		assert.True(t, errs.IsInvalid(err), "want an invalid-input error, got %v", err)
	})

	t.Run("ended with no examples", func(t *testing.T) {
		c, err := NewEmbeddingClassifier(endedContext(t), nil, 0.5)
		assert.Nil(t, c)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

// TestFindBestMatchEndedContext: no match and a score of 0 for a nil or ended
// context.
func TestFindBestMatchEndedContext(t *testing.T) {
	c := createTestEmbeddingClassifier(t)
	const query = "Show delayed deliveries"

	var nilCtx context.Context
	match, score := c.FindBestMatch(nilCtx, query)
	assert.Nil(t, match)
	assert.Equal(t, 0.0, score)

	match, score = c.FindBestMatch(endedContext(t), query)
	assert.Nil(t, match)
	assert.Equal(t, 0.0, score)
}

// copyTestDomains returns fresh example domains on each call, sharing no map
// or slice with an earlier call, so two classifiers built from them start
// identical and independent.
func copyTestDomains() []*DomainExamples {
	return []*DomainExamples{{
		Domain: "logistics",
		Examples: []Example{
			{Query: "Where is shipment ABC-123?", Intent: "entity_lookup",
				Options: map[string]any{"entity_pattern": "shipment"}},
			{Query: "Show delayed deliveries", Intent: "temporal_filter",
				Options: map[string]any{"status_filter": "delayed"}},
			{Query: "Find packages near location", Intent: "spatial_filter",
				Options: map[string]any{"spatial": true}},
		},
	}}
}

// copyTestQueries each match an example at tier 1 and no keyword pattern.
var copyTestQueries = []string{
	"Show delayed deliveries",
	"Where is shipment ABC-123?",
	"Find packages near location",
	"Show delayed deliveries",
	"Where is shipment ABC-123?",
	"Find packages near location",
}

// TestResultEditsDoNotReachClassifier: a caller that edits what it was handed
// changes no later result (design D4; S9). Each subtest drives an edited
// classifier and an untouched control built from identical examples through
// the same query sequence. Both learn from the queries through Generate (#96),
// so the corpus drift is the same on both and any difference is the edit's.
func TestResultEditsDoNotReachClassifier(t *testing.T) {
	t.Run("FindBestMatch", func(t *testing.T) {
		edited := mustNewEmbeddingClassifier(t, copyTestDomains(), 0.3)
		control := mustNewEmbeddingClassifier(t, copyTestDomains(), 0.3)

		for step, query := range copyTestQueries {
			got, gotScore := edited.FindBestMatch(t.Context(), query)
			want, wantScore := control.FindBestMatch(t.Context(), query)
			require.NotNil(t, want, "step %d: the control must match at tier 1", step)
			require.NotNil(t, got, "step %d: edited classifier lost its match", step)
			assert.Equal(t, want.Query, got.Query, "step %d: query", step)
			assert.Equal(t, want.Intent, got.Intent, "step %d: intent", step)
			assert.Equal(t, want.Options, got.Options, "step %d: options", step)
			assert.Equal(t, want.Vector, got.Vector, "step %d: vector", step)
			assert.Equal(t, wantScore, gotScore, "step %d: score", step)

			// The caller edits every part of what it was handed.
			got.Query = "edited"
			got.Intent = "edited"
			for k := range got.Options {
				delete(got.Options, k)
			}
			got.Options["injected"] = true
			for i := range got.Vector {
				got.Vector[i] = 0
			}
		}
	})

	t.Run("chain", func(t *testing.T) {
		edited := NewClassifierChain(NewKeywordClassifier(), mustNewEmbeddingClassifier(t, copyTestDomains(), 0.3))
		control := NewClassifierChain(NewKeywordClassifier(), mustNewEmbeddingClassifier(t, copyTestDomains(), 0.3))

		for step, query := range copyTestQueries {
			got := edited.ClassifyQuery(t.Context(), query)
			want := control.ClassifyQuery(t.Context(), query)
			require.NotNil(t, want)
			require.Equal(t, 1, want.Tier, "step %d: the control must answer at tier 1", step)
			assert.Equal(t, want, got, "step %d: result", step)

			for k := range got.Options {
				delete(got.Options, k)
			}
			got.Options["injected"] = true
		}
	})
}

// TestZoneQueryAddsNoPredicate: a zone or area phrase keeps its path intent and
// start node and adds no predicate filter (design D5).
func TestZoneQueryAddsNoPredicate(t *testing.T) {
	tests := []struct {
		query     string
		startNode string
	}{
		{"sensors in zone A", "zone-A"},
		{"show items in zone-A", "zone-A"},
		{"pumps near area north", "area-north"},
	}
	k := NewKeywordClassifier()
	chain := NewClassifierChain(k, nil)
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			opts := k.ClassifyQuery(t.Context(), tt.query)
			require.NotNil(t, opts)
			assert.True(t, opts.PathIntent)
			assert.Equal(t, tt.startNode, opts.PathStartNode)
			assert.Empty(t, opts.PathPredicates)

			got := chain.ClassifyQuery(t.Context(), tt.query)
			require.NotNil(t, got)
			assert.Equal(t, map[string]any{"path_intent": true, "path_start_node": tt.startNode}, got.Options)
		})
	}
}

// TestEmbeddingClassifierAsAProduct calls the example tier the way a product
// does: two example files on disk, LoadDomainExamples, NewEmbeddingClassifier,
// and a chain beside a KeywordClassifier the product holds as a Classifier.
func TestEmbeddingClassifierAsAProduct(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"logistics.json": `{"domain":"logistics","version":"1.0","examples":[
			{"query":"Show delayed deliveries","intent":"temporal_filter","options":{"status_filter":"delayed","limit":5}},
			{"query":"Where is shipment ABC-123?","intent":"entity_lookup"}]}`,
		"iot.json": `{"domain":"iot","version":"1.0","examples":[
			{"query":"Show temperature readings for every sensor","intent":"sensor_query","options":{"metric":"temperature"}}]}`,
	}
	var domains []*DomainExamples
	for _, name := range []string{"logistics.json", "iot.json"} {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte(files[name]), 0o600))
		d, err := LoadDomainExamples(path)
		require.NoError(t, err)
		domains = append(domains, d)
	}

	const threshold = 0.3
	examples, err := NewEmbeddingClassifier(t.Context(), domains, threshold)
	require.NoError(t, err)

	var keyword Classifier = NewKeywordClassifier()
	const question = "temperature readings for every sensor"
	opts := keyword.ClassifyQuery(t.Context(), question)
	require.NotNil(t, opts)
	assert.Equal(t, question, opts.Query)

	kc, ok := keyword.(*KeywordClassifier)
	require.True(t, ok)
	require.Equal(t, defaultResult(), NewClassifierChain(kc, nil).ClassifyQuery(t.Context(), question),
		"the question must carry no keyword hint")
	chain := NewClassifierChain(kc, examples)

	got := chain.ClassifyQuery(t.Context(), question)

	require.NotNil(t, got)
	assert.Equal(t, 1, got.Tier)
	assert.Equal(t, "sensor_query", got.Intent)
	// The oracle is the iot file's options as encoding/json decodes them.
	assert.Equal(t, map[string]any{"metric": "temperature"}, got.Options)
	assert.GreaterOrEqual(t, got.Confidence, threshold)
	assert.LessOrEqual(t, got.Confidence, 1.0+1e-9)
}
