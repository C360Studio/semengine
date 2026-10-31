package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// aggregationCase is one question and the exact result the keyword-only chain
// must give for it. The expected hints are written by hand from issue #94's
// table and design D1, never derived from the classifier.
type aggregationCase struct {
	name  string
	query string
	want  map[string]any // the result's Options; empty means the default result
}

func agg(op, field string) map[string]any {
	m := map[string]any{"aggregation_type": op}
	if field != "" {
		m["aggregation_field"] = field
	}
	return m
}

func runAggregationCases(t *testing.T, cases []aggregationCase) {
	t.Helper()
	chain := NewClassifierChain(NewKeywordClassifier(), nil)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := chain.ClassifyQuery(t.Context(), tc.query)
			require.NoError(t, err)
			require.NotNil(t, got)
			if len(tc.want) == 0 {
				assert.Equal(t, defaultResult(), got, "%q activates nothing", tc.query)
				return
			}
			assert.Equal(t, 0, got.Tier)
			assert.Equal(t, 1.0, got.Confidence)
			assert.Equal(t, tc.want, got.Options, "%q", tc.query)
		})
	}
}

// TestAggregationFieldBinding: the field is the first word after the trigger,
// skipping an optional "of" and "the"; fields are the caller's words (#94, D1).
func TestAggregationFieldBinding(t *testing.T) {
	runAggregationCases(t, []aggregationCase{
		{"R17 mean of a field", "Compute the arithmetic mean of pressure", agg(AggregationAvg, "pressure")},
		{"R21 total of a field", "Return the combined total of humidity readings", agg(AggregationSum, "humidity")},
		{"of and the skipped", "Compute the mean of the pressure", agg(AggregationAvg, "pressure")},
		{"no of: the next word", "average temperature of sensors", agg(AggregationAvg, "temperature")},
		{"caller field after of", "Give the maximum of flow_rate", agg(AggregationMax, "flow_rate")},
		{"caller field after of the", "Compute the sum of the kilowatt_hours", agg(AggregationSum, "kilowatt_hours")},
		{"min of a caller field", "minimum of turbidity", agg(AggregationMin, "turbidity")},
		// A caller's field is kept whole: any script, inner hyphens.
		{"non-ASCII field", "average température of sensors", agg(AggregationAvg, "température")},
		{"hyphenated field", "average flow-rate", agg(AggregationAvg, "flow-rate")},
		{"non-ASCII extremum field", "Give the lowest température reading", agg(AggregationMin, "température")},
		// Nothing follows "of": the operation stands and no field is invented.
		{"nothing after of", "What is the average of?", agg(AggregationAvg, "")},
	})
}

// TestOrdinaryAggregationPhrases: "the number of" counts, numeric extremum
// phrases give min or max with their field, and descriptive mentions activate
// nothing (#94, D1).
func TestOrdinaryAggregationPhrases(t *testing.T) {
	runAggregationCases(t, []aggregationCase{
		{"R14 the number of", "Return the number of sensors", agg(AggregationCount, "")},
		{"R23 smallest value", "Give the smallest temperature value", agg(AggregationMin, "temperature")},
		{"R24 lowest reading", "Return the lowest pressure reading", agg(AggregationMin, "pressure")},
		{"R26 largest value", "Give the largest humidity value", agg(AggregationMax, "humidity")},
		{"R27 highest reading", "Return the highest temperature reading", agg(AggregationMax, "temperature")},
		{"greatest, plural noun", "Give the greatest flow values", agg(AggregationMax, "flow")},
		{"lowest measurements", "Return the lowest voltage measurements", agg(AggregationMin, "voltage")},
		{"highest level", "Report the highest tank level", agg(AggregationMax, "tank")},

		// Precedence min > max holds for ordinary extrema in either order, each with its own field.
		{"highest then lowest", "Give the highest pressure reading and lowest temperature reading", agg(AggregationMin, "temperature")},
		{"lowest then highest", "Give the lowest temperature reading and highest pressure reading", agg(AggregationMin, "temperature")},
		{"highest; lowest", "Give the highest pressure reading; give the lowest temperature reading", agg(AggregationMin, "temperature")},
		{"maximum then minimum", "Give the maximum pressure reading and minimum temperature reading", agg(AggregationMin, "temperature")},
		{"highest then lowest after which", "Which pump has the lowest flow value; give the highest pressure reading", agg(AggregationMax, "pressure")},

		// Precedence: "total number of" keeps count over sum.
		{"total number of is count", "What is the total number of pumps", agg(AggregationCount, "")},
		{"total number of before a value noun", "Return the total number of humidity readings", agg(AggregationCount, "")},

		// "which", "who" or "with" in another clause does not stop the extremum.
		{"which in an earlier clause", "Which pump failed; give the highest pressure reading", agg(AggregationMax, "pressure")},
		{"with before but", "List pumps with alarms but give the lowest pressure reading", agg(AggregationMin, "pressure")},

		// Descriptive mentions: no operation.
		{"a number of", "Show sensors with a number of faults", nil},
		{"which before highest", "Which pump has the highest pressure reading", nil},
		{"with before lowest", "Show the sensor with the lowest battery level", nil},
		{"entity ranking", "Which sensor has the highest temperature reading", nil},
		{"who before largest", "Who logged the largest pressure value", nil},
		{"bare highest", "Give the highest temperature", nil},
		// "of" is a skipped word, never the extremum's field.
		{"extremum of a value noun", "Give the lowest of readings", nil},
		// A mark inside a number ends no clause, so "which" or "with" still stops the extremum.
		{"which before a decimal", "Which pump above 2.5 bar has the highest pressure reading", nil},
		{"which before a thousands comma", "Which pump above 1,000 kPa has the highest pressure reading", nil},
		{"with before a decimal", "Show the sensor with a 0.5 offset and the lowest battery level", nil},

		// Entity ranking stays ranking, not a numeric extremum.
		{"top N", "top 5 sensors by temperature", map[string]any{"ranking_intent": true, "limit": 5}},
	})
}
