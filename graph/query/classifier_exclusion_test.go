package query

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestKeywordExclusions: a negated, quoted or undecided operation gives no
// hint, and other hints stand (#95 R28-R32, design D2). The expected results
// are written by hand from #95's table and D2's rule.
func TestKeywordExclusions(t *testing.T) {
	similarity := map[string]any{"use_embeddings": true}
	runAggregationCases(t, []aggregationCase{
		// #95's four cases: no operation hint.
		{"R28 negated count", "Do not count sensors; list their names", nil},
		{"R29 negated similarity", "Show pump-42 records, not devices similar to it", nil},
		{"R30 quoted average", `Find documents containing the phrase "average temperature"`, nil},
		{"R31 count or average", "Maybe count devices or average pressure; I have not decided", nil},

		// Each of the seven operations is excluded on its own, by every negating word.
		{"count after never", "never count the sensors", nil},
		{"avg after don't", "don't average pressure", nil},
		{"avg after curly don't", "Don’t average pressure", nil},
		{"sum after without", "pumps without the sum of flow", nil},
		{"min after no", "no minimum pressure", nil},
		{"min extremum after not", "not the lowest pressure reading", nil},
		{"max after do not", "do not give the maximum pressure", nil},
		{"max extremum after not", "not the highest pressure reading", nil},
		{"ranking top N after not", "not the top 5 pumps", nil},
		{"ranking most after not", "not the most active pumps", nil},
		{"negated total number of is no sum", "Do not give the total number of sensors", nil},
		{"no space after a mark is one clause", "Do not.count sensors", nil},

		// A negation reaches only its own clause; other operations stand.
		{"negation then affirmative", "Do not count sensors; average pressure", agg(AggregationAvg, "pressure")},
		{"negation after but", "Do not count sensors but average pressure", agg(AggregationAvg, "pressure")},
		{"excluded count gives way to ranking", "Do not count sensors, give the top 3 pumps",
			map[string]any{"ranking_intent": true, "limit": 3}},
		{"negated count after an affirmative average", "Show the average pressure, not the count",
			agg(AggregationAvg, "pressure")},
		{"negation after the trigger", "Count sensors that are not active", agg(AggregationCount, "")},
		{"not inside a word", "Annotate and count sensors", agg(AggregationCount, "")},

		// Quotes: a trigger inside a balanced pair gives no hint; a quoted name
		// in a valid request leaves the request's own hint.
		{"quoted name in a count", `Count sensors named "like-new"`, agg(AggregationCount, "")},
		{"curly quoted ranking", "Show sensors similar to “top 5 pumps”", similarity},
		{"unbalanced quote", `Count sensors named "pump`, agg(AggregationCount, "")},

		// "or" excludes only two different operations in one clause.
		{"one operation or", "Count sensors or pumps", agg(AggregationCount, "")},
		{"same operation twice", "count sensors or count pumps", agg(AggregationCount, "")},
		{"two operations in another clause", "Count sensors; average or total pressure", agg(AggregationCount, "")},
		{"min or max", "the minimum or maximum pressure", nil},
		{"or the before the second", "count sensors or the average pressure", nil},
		{"or an before the second", "count sensors or an average pressure", nil},
		{"or a before the second", "Maybe average pressure or a count of sensors; I have not decided", nil},
		{"or an, count first", "Maybe count sensors or an average pressure; I have not decided", nil},
		// "or" joining nouns or other words leaves the operations decided.
		{"or joins nouns", "How many sensors or pumps have an average temperature above 30?", agg(AggregationCount, "")},
		{"or joins adjectives", "Is the maximum pressure higher or lower than the average?",
			agg(AggregationAvg, "")},
		{"or joins verbs", "Count sensors that failed or are likely to", agg(AggregationCount, "")},
		{"or a joins nouns", "Count sensors with a pump or a valve", agg(AggregationCount, "")},
		// "likely" is not the similarity word "like".
		{"likely is no similarity", "Show sensors likely to fail", nil},

		// R32: a partial path intent is kept and no node is invented.
		{"R32 partial path intent", "Show connections from that device", map[string]any{"path_intent": true}},
	})
}

// TestKeywordExclusionKeepsOtherHints: an excluded operation beside a
// temporal hint leaves the temporal hint at tier 0.
func TestKeywordExclusionKeepsOtherHints(t *testing.T) {
	got, err := NewClassifierChain(NewKeywordClassifier(), nil).ClassifyQuery(t.Context(),
		"Do not count sensors from yesterday")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 0, got.Tier)
	assert.Contains(t, got.Options, "time_range")
	assert.NotContains(t, got.Options, "aggregation_type")
}

// exclusionDomains are the pin's iot example shapes (configs/domains/iot.json
// at 8b99efe9): an operation in the caller's own words, so no framework hint
// name says "count". The similarity example is design D2's "Find devices
// similar to TEMP-001".
func exclusionDomains() []*DomainExamples {
	return []*DomainExamples{{
		Domain: "iot",
		Examples: []Example{
			{Query: "Show sensor SENS-001 status", Intent: "entity_lookup",
				Options: map[string]any{"entity_pattern": "sensor"}},
			{Query: "What sensors were active yesterday?", Intent: "temporal_filter",
				Options: map[string]any{"status_filter": "active"}},
			{Query: "How many sensors are reporting?", Intent: "aggregation",
				Options: map[string]any{"operation": "count", "status_filter": "reporting"}},
			{Query: "Count devices in error state", Intent: "aggregation",
				Options: map[string]any{"operation": "count", "status_filter": "error"}},
			{Query: "Find devices similar to TEMP-001", Intent: "similarity",
				Options: map[string]any{"reference_entity": true}},
		},
	}}
}

// TestChainKeepsExclusions: a later tier cannot restore an operation the
// keyword tier excluded (#95, design D2 option B).
func TestChainKeepsExclusions(t *testing.T) {
	const threshold = 0.7 // the measurement design D2 cites
	excludedQuestions := []struct{ query, unwanted string }{
		{"Do not count devices in error state", "Count devices in error state"},
		{"Not devices similar to TEMP-001", "Find devices similar to TEMP-001"},
	}

	// (1) The example tier alone would answer each question with the unwanted
	// example; without this the chain part proves nothing.
	t.Run("example tier alone matches the unwanted example", func(t *testing.T) {
		examples, err := NewEmbeddingClassifier(t.Context(), exclusionDomains(), threshold)
		require.NoError(t, err)
		for _, q := range excludedQuestions {
			match, score := examples.FindBestMatch(t.Context(), q.query)
			require.NotNil(t, match, "%q", q.query)
			assert.Equal(t, q.unwanted, match.Query, "%q", q.query)
			assert.GreaterOrEqual(t, score, threshold, "%q", q.query)
		}
	})

	// (2) The full chain gives the default result and never asks the LLM.
	t.Run("chain gives the default result", func(t *testing.T) {
		for _, q := range excludedQuestions {
			examples, err := NewEmbeddingClassifier(t.Context(), exclusionDomains(), threshold)
			require.NoError(t, err)
			client := &countingLLMClient{respond: func(context.Context) (string, error) {
				return `{"aggregation_type":"count"}`, nil
			}}
			chain := NewClassifierChain(NewKeywordClassifier(), examples, NewLLMClassifier(client, exclusionDomains()))

			got, err := chain.ClassifyQuery(t.Context(), q.query)
			require.NoError(t, err)

			assert.Equal(t, defaultResult(), got, "%q", q.query)
			assert.Zero(t, client.calls.Load(), "%q: the LLM client was never called", q.query)
		}
	})

	// (3) The affirmative questions answer at tier 0.
	t.Run("affirmative controls answer at tier 0", func(t *testing.T) {
		examples, err := NewEmbeddingClassifier(t.Context(), exclusionDomains(), threshold)
		require.NoError(t, err)
		chain := NewClassifierChain(NewKeywordClassifier(), examples)
		for query, want := range map[string]map[string]any{
			"Count devices in error state": agg(AggregationCount, ""),
			"Devices similar to TEMP-001":  {"use_embeddings": true},
		} {
			got, err := chain.ClassifyQuery(t.Context(), query)
			require.NoError(t, err)
			require.NotNil(t, got, "%q", query)
			assert.Equal(t, 0, got.Tier, "%q", query)
			assert.Equal(t, want, got.Options, "%q", query)
		}
	})
}

// quoteMarks are the characters that open or close a quoted span.
var quoteMarks = strings.NewReplacer(`"`, "", "“", "", "”", "")

// FuzzKeywordClassify checks the two laws of the spec requirement "The keyword
// tier accepts any text" (design D2): no input panics, and a question whose
// only triggers lie inside double quotes gives no operation hint.
//
// The second law's oracle does not read the trigger patterns: the question is
// fixed text with no trigger, plus the fuzzed text with its quote marks removed
// and wrapped in straight or curly quotes, so every trigger it holds is quoted.
func FuzzKeywordClassify(f *testing.F) {
	for _, seed := range []string{
		"",
		"Do not count sensors; list their names",
		"Show pump-42 records, not devices similar to it",
		`Find documents containing the phrase "average temperature"`,
		"Maybe count devices or average pressure; I have not decided",
		"Do not count sensors; average pressure",
		"how many sensors, the number of pumps, total number of devices",
		"the lowest pressure reading or the highest flow value",
		"mean, sum, minimum, maximum, top 5, bottom 3, most, least",
		"similar to pump-42 but not like it",
		"“top 5” \"count\" “unclosed",
		"within 5km of 37.7,-122.4 last 3 hours",
		"2.5, 1,000? no; never: without!",
		"\xe2”\x80\x9d count sensors",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, s string) {
		classifier := NewKeywordClassifier()
		require.NotNil(t, classifier.ClassifyQuery(t.Context(), s))

		// Invalid bytes go first, so the replacer cannot join split bytes into a new mark.
		inner := quoteMarks.Replace(strings.ToValidUTF8(s, ""))
		for _, q := range []string{
			`Find documents containing "` + inner + `"`,
			"Find documents containing “" + inner + "”",
		} {
			opts := classifier.ClassifyQuery(t.Context(), q)
			require.NotNil(t, opts)
			assert.Empty(t, opts.AggregationType, "%q", q)
			assert.False(t, opts.UseEmbeddings, "%q", q)
			assert.False(t, opts.RankingIntent, "%q", q)
		}
	})
}
