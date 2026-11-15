package query

import (
	"context"
	"errors"
	"maps"
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

// noHintQuery matches none of the keyword tier's patterns, so the chain reaches
// its LLM tier.
const noHintQuery = "tell me about the pumps"

// defaultResult is the chain's default result, written out by hand.
func defaultResult() *ClassificationResult {
	return &ClassificationResult{Tier: 0, Intent: "", Options: map[string]any{}, Confidence: 1.0}
}

func endedContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	return ctx
}

// TestChainEndedContextReturnsNil: a nil context and an ended context yield nil.
// A chain never answers the default result for an ended context.
func TestChainEndedContextReturnsNil(t *testing.T) {
	t.Run("nil context", func(t *testing.T) {
		client := &countingLLMClient{respond: func(context.Context) (string, error) {
			return `{"strategy":"semantic"}`, nil
		}}
		chain := NewClassifierChain(NewKeywordClassifier(), NewLLMClassifier(client, nil))
		var nilCtx context.Context
		got, err := chain.ClassifyQuery(nilCtx, noHintQuery)
		require.NoError(t, err)
		assert.Nil(t, got)
		assert.Zero(t, client.calls.Load(), "no tier runs for a nil context")
	})

	t.Run("ended at entry", func(t *testing.T) {
		client := &countingLLMClient{respond: func(context.Context) (string, error) {
			return `{"strategy":"semantic"}`, nil
		}}
		chain := NewClassifierChain(NewKeywordClassifier(), NewLLMClassifier(client, nil))
		got, err := chain.ClassifyQuery(endedContext(t), noHintQuery)
		require.NoError(t, err)
		assert.Nil(t, got)
		assert.Zero(t, client.calls.Load(), "no tier runs for an ended context")
	})

	t.Run("ended during the LLM tier", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		client := &countingLLMClient{respond: func(ctx context.Context) (string, error) {
			cancel()
			return "", ctx.Err()
		}}
		chain := NewClassifierChain(NewKeywordClassifier(), NewLLMClassifier(client, nil))
		got, err := chain.ClassifyQuery(ctx, noHintQuery)
		require.NoError(t, err)
		assert.Nil(t, got)
		assert.Equal(t, int64(1), client.calls.Load())
	})
}

// TestChainLLMFailureGivesDefault: an LLM error under a live context yields the
// default result.
func TestChainLLMFailureGivesDefault(t *testing.T) {
	client := &countingLLMClient{respond: func(context.Context) (string, error) {
		return "", errors.New("provider unavailable")
	}}
	chain := NewClassifierChain(NewKeywordClassifier(), NewLLMClassifier(client, nil))

	got, err := chain.ClassifyQuery(t.Context(), noHintQuery)
	require.NoError(t, err)

	assert.Equal(t, defaultResult(), got)
	assert.Equal(t, int64(1), client.calls.Load(), "the LLM tier was tried")
}

// TestChainKeywordHintStopsTheLadder: a keyword hint answers at tier 0 and the
// LLM tier does not run.
func TestChainKeywordHintStopsTheLadder(t *testing.T) {
	client := &countingLLMClient{respond: func(context.Context) (string, error) {
		return `{"strategy":"semantic"}`, nil
	}}
	chain := NewClassifierChain(NewKeywordClassifier(), NewLLMClassifier(client, nil))

	got, err := chain.ClassifyQuery(t.Context(), "events from yesterday")
	require.NoError(t, err)

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

// TestChainAnswersFromFirstTier composes the chain the way a product does: an
// example file on disk read with LoadDomainExamples, a KeywordClassifier, and
// an LLM tier over the product's own LLMClient. A keyword hint answers at tier
// 0 without the LLM; a question with no keyword hint gets the LLM's answer at
// tier 3; with no LLM tier composed, that question gets the default result.
// Each expected result is written out by hand from the fake LLM's JSON.
func TestChainAnswersFromFirstTier(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pumps.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"domain":"plant","version":"1.0","examples":[
		{"query":"Which pumps are running?","intent":"entity_lookup","options":{"types":["pump"]}}]}`), 0o600))
	domains, err := LoadDomainExamples(path)
	require.NoError(t, err)

	client := &countingLLMClient{respond: func(context.Context) (string, error) {
		return `{"types":["pump"]}`, nil
	}}
	chain := NewClassifierChain(NewKeywordClassifier(), NewLLMClassifier(client, []*DomainExamples{domains}))

	t.Run("keyword hint", func(t *testing.T) {
		got, err := chain.ClassifyQuery(t.Context(), "find similar items")
		require.NoError(t, err)
		assert.Equal(t, &ClassificationResult{Tier: 0, Options: map[string]any{"use_embeddings": true},
			Confidence: 1.0}, got)
		assert.Zero(t, client.calls.Load(), "a keyword hint stops the ladder")
	})

	t.Run("LLM result", func(t *testing.T) {
		got, err := chain.ClassifyQuery(t.Context(), noHintQuery)
		require.NoError(t, err)
		assert.Equal(t, &ClassificationResult{Tier: 3, Intent: "entity_lookup",
			Options: map[string]any{"query": noHintQuery, "types": []string{"pump"}}, Confidence: 0.9}, got)
		assert.Equal(t, int64(1), client.calls.Load())
	})

	t.Run("default without an LLM tier", func(t *testing.T) {
		got, err := NewClassifierChain(NewKeywordClassifier(), nil).ClassifyQuery(t.Context(), noHintQuery)
		require.NoError(t, err)
		assert.Equal(t, defaultResult(), got)
	})
}

// TestChainOptionsAreCopies: a caller that deletes and adds top-level keys in a
// returned Options map changes no later result, from each tier and from the
// default. The expected result is the first answer, copied before the edit.
func TestChainOptionsAreCopies(t *testing.T) {
	client := &countingLLMClient{respond: func(context.Context) (string, error) {
		return `{"types":["pump"]}`, nil
	}}
	withLLM := NewClassifierChain(NewKeywordClassifier(), NewLLMClassifier(client, nil))
	keywordOnly := NewClassifierChain(NewKeywordClassifier(), nil)
	for _, tt := range []struct {
		name, query string
		chain       *ClassifierChain
		tier        int
	}{
		{"keyword tier", "find similar items", withLLM, 0},
		{"LLM tier", noHintQuery, withLLM, 3},
		{"default result", noHintQuery, keywordOnly, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			first, err := tt.chain.ClassifyQuery(t.Context(), tt.query)
			require.NoError(t, err)
			require.NotNil(t, first)
			require.Equal(t, tt.tier, first.Tier)
			want := *first
			want.Options = maps.Clone(first.Options)

			for k := range first.Options {
				delete(first.Options, k)
			}
			first.Options["injected"] = true

			next, err := tt.chain.ClassifyQuery(t.Context(), tt.query)
			require.NoError(t, err)
			assert.Equal(t, &want, next)
		})
	}
}

// TestZoneQueryAddsNoPredicate: a zone or area phrase keeps its path intent and
// start node and adds no predicate filter.
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

			got, err := chain.ClassifyQuery(t.Context(), tt.query)
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, map[string]any{"path_intent": true, "path_start_node": tt.startNode}, got.Options)
		})
	}
}

// TestChainRefusesAnEntityBesideAZone: a question naming an entity reference and a zone or area
// phrase names two places, and the keyword tier answers a path from only one of them, so the chain
// refuses it naming both, with or without path words. A zone alone and a start
// alone classify as before, and a reference that holds a zone word ("sensor-zone-1", "zone-b"
// itself) is one place.
func TestChainRefusesAnEntityBesideAZone(t *testing.T) {
	chain := NewClassifierChain(NewKeywordClassifier(), nil)
	for _, tt := range []struct{ query, start, zone string }{
		{"what is connected to sensor-001 in zone-b", "sensor-001", "zone-b"},
		{"what is connected to sensor-001 near zone-b", "sensor-001", "zone-b"},
		{"what in zone-b is connected to sensor-001", "sensor-001", "zone-b"},
		{"show the path from pump-42 in area north", "pump-42", "area-north"},
		{"show sensor-001 in zone-b", "sensor-001", "zone-b"},
		{"status of sensor-001 in zone-b", "sensor-001", "zone-b"},
		// Zone names written with a space are places too.
		{"what is connected to zone a in zone b", "zone-a", "zone-b"},
		{"what is in zone a near area b", "area-b", "zone-a"},
		// A full ID with no hyphenated segment is a place too.
		{"what is connected to c360.test.x.y.z.001 in zone b", "c360.test.x.y.z.001", "zone-b"},
		{"show c360.test.x.y.z.001 in zone-b", "c360.test.x.y.z.001", "zone-b"},
		{"in zone b, what is connected to c360.test.x.y.z.001?", "c360.test.x.y.z.001", "zone-b"},
	} {
		t.Run("refused/"+tt.query, func(t *testing.T) {
			got, err := chain.ClassifyQuery(t.Context(), tt.query)
			assert.Nil(t, got, "no result")
			require.Error(t, err)
			assert.True(t, errs.IsInvalid(err), "want an invalid-input error, got %v", err)
			assert.ErrorContains(t, err, "the question names two places, "+tt.start+" and "+tt.zone+"; ask about one")
		})
	}
	for _, tt := range []struct{ query, start string }{
		{"what is in zone-b", "zone-b"},
		{"pumps near area north", "area-north"},
		{"what is connected to sensor-001", "sensor-001"},
		{"what is connected to zone-b", "zone-b"},
		{"what is connected to sensor-zone-1", "sensor-zone-1"},
		{"what is connected to area-north-pump", "area-north-pump"},
		{"what is connected to pump-zone-1", "pump-zone-1"},
		{"what is connected to zone a", "zone-a"},
		{"what is in zone b", "zone-b"},
		// One place spelled twice is one place, whatever the separator or case.
		{"what is in zone a, zone-a", "zone-a"},
		{"what is in Zone A near zone a", "Zone-A"},
		// A full ID holding the zone phrase is that one place.
		{"what is connected to c360.test.x.y.zone-b.001", "zone-b"},
	} {
		t.Run("one place/"+tt.query, func(t *testing.T) {
			got, err := chain.ClassifyQuery(t.Context(), tt.query)
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, map[string]any{"path_intent": true, "path_start_node": tt.start}, got.Options)
		})
	}
	// A zone beside other hints and no entity reference is one place; the hints stand.
	for _, q := range []string{"what changed in zone-b today", "how many sensors in zone-b"} {
		t.Run("one place with hints/"+q, func(t *testing.T) {
			got, err := chain.ClassifyQuery(t.Context(), q)
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, "zone-b", got.Options["path_start_node"])
		})
	}
}
