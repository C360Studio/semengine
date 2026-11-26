package rule

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	gtypes "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/harness/semantictest"
	"github.com/c360studio/semengine/message"
)

// Requirement: rule-substitution/"One token keeps its type".

// TestSubstituteVariablesTyped checks what the typed path returns for each
// template shape: a whole-template bare triple, message or state token keeps
// its value's Go type, and everything else returns false so the caller
// substitutes as text. The value and its type are both checked, since two
// values of different types can print the same.
func TestSubstituteVariablesTyped(t *testing.T) {
	t.Parallel()
	entityID := semantictest.EntityID(t, "c360", "platform", "robotics", "mav1", "drone", "001")

	ec := &ExecutionContext{
		EntityID: entityID,
		Entity: &gtypes.EntityState{
			ID: entityID,
			Triples: []message.Triple{
				{Subject: entityID, Predicate: semantictest.Predicate(t, "test", "metric", "value"), Object: float64(0.83)},
				{Subject: entityID, Predicate: semantictest.Predicate(t, "test", "metric", "active"), Object: true},
				{Subject: entityID, Predicate: semantictest.Predicate(t, "test", "metric", "count"), Object: int(7)},
				{Subject: entityID, Predicate: semantictest.Predicate(t, "test", "metric", "count"), Object: "second"},
				{Subject: entityID, Predicate: semantictest.Predicate(t, "test", "fixture", "score"), Object: float64(0.82)},
			},
		},
		MessageData: map[string]any{
			"score": 0.42, "flag": true, "nested": map[string]any{"deep": "value"}, "nullable": nil,
		},
		State: &MatchState{Iteration: 3, MaxIterations: 5},
	}

	tests := []struct {
		name       string
		template   string
		wantOK     bool
		wantValue  any
		wantTypeOf any // a value of the expected Go type; nil when wantOK is false
	}{
		{"a number stays a number", "$entity.triple.test.fixture.score", true, float64(0.82), float64(0)},
		{"entity triple float64", "$entity.triple.test.metric.value", true, float64(0.83), float64(0)},
		{"entity triple bool", "$entity.triple.test.metric.active", true, true, false},
		{"the first triple of a predicate wins by value and type", "$entity.triple.test.metric.count", true, int(7), int(0)},
		{"message scalar float", "$message.score", true, 0.42, float64(0)},
		{"message scalar bool", "$message.flag", true, true, false},
		{"message nested path", "$message.nested.deep", true, "value", ""},
		{"message map value", "$message.nested", true, map[string]any{"deep": "value"}, map[string]any{}},
		// A present field holding JSON null keeps its nil: the typed path does
		// not turn it into text.
		{"message null value", "$message.nullable", true, nil, nil},
		{"state iteration int", "$state.iteration", true, int(3), int(0)},
		{"state max_iterations int", "$state.max_iterations", true, int(5), int(0)},
		{"text around a token", "score=$entity.triple.test.fixture.score", false, nil, nil},
		{"absent entity triple", "$entity.triple.test.fixture.missing", false, nil, nil},
		{"absent message field", "$message.missing", false, nil, nil},
		{"mixed template", "prefix-$entity.triple.test.metric.value", false, nil, nil},
		{"literal only", "escalated", false, nil, nil},
		{"bare dollar", "$", false, nil, nil},
		{"empty", "", false, nil, nil},
		{"trailing space", "$entity.triple.test.metric.value ", false, nil, nil},
		{"trailing dot", "$message.score.", false, nil, nil},
		{"two tokens", "$state.iteration$state.iteration", false, nil, nil},
		{"length suffix", "$entity.triple.test.metric.value.length", false, nil, nil},
		{"triples suffix", "$entity.triple.test.metric.value.triples", false, nil, nil},
		{"value suffix after a 3-part predicate", "$entity.triple.test.metric.active.value", false, nil, nil},
		{"state name extended", "$state.iterations", false, nil, nil},
		{"entity id is text", "$entity.id", false, nil, nil},
		{"now is text", "$now", false, nil, nil},
		{"entity part is text", "$entity.instance", false, nil, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			val, ok := ec.SubstituteVariablesTyped(tt.template)
			assert.Equal(t, tt.wantOK, ok)
			if !tt.wantOK {
				assert.Nil(t, val, "the value is nil when the result is false")
				return
			}
			assert.Equal(t, tt.wantValue, val)
			assert.Equal(t, reflect.TypeOf(tt.wantTypeOf), reflect.TypeOf(val), "the Go type is kept")
		})
	}
}

func TestSubstituteVariablesTypedWithoutSources(t *testing.T) {
	t.Parallel()
	ec := &ExecutionContext{}

	for _, template := range []string{"$entity.triple.test.metric.value", "$message.score", "$state.iteration"} {
		val, ok := ec.SubstituteVariablesTyped(template)
		assert.False(t, ok, "%s with no entity, message or state", template)
		assert.Nil(t, val)
	}
}
