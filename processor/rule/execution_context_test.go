package rule

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gtypes "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/harness/semantictest"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/processor/rule/expression"
)

// Requirement: rule-substitution/"A template is read once", "A token ends where
// its name ends", "An unresolved token stays and is reported", "RuleID names
// the rule that built the state" and "The values of a predicate".

// substitutionWarning is one warning SubstituteVariables logged, as the test
// reads it back from the log's JSON.
type substitutionWarning struct {
	Msg        string   `json:"msg"`
	Unresolved []string `json:"unresolved"`
	RuleID     string   `json:"rule_id"`
	EntityID   string   `json:"entity_id"`
	Template   string   `json:"template"`
}

// warningLog holds what the process logger received while a test ran.
type warningLog struct {
	buf bytes.Buffer
}

// captureWarnings points the process logger at a buffer for the rest of the
// test. The logger is process-wide, so no test that calls this runs in
// parallel.
func captureWarnings(t *testing.T) *warningLog {
	t.Helper()
	log := &warningLog{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&log.buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return log
}

// take returns the warnings logged since the last take and empties the log.
func (l *warningLog) take(t failer) []substitutionWarning {
	t.Helper()
	var out []substitutionWarning
	for _, line := range strings.Split(strings.TrimSpace(l.buf.String()), "\n") {
		if line == "" {
			continue
		}
		var w substitutionWarning
		if err := json.Unmarshal([]byte(line), &w); err != nil {
			t.Fatalf("log line %q is not JSON: %v", line, err)
		}
		out = append(out, w)
	}
	l.buf.Reset()
	return out
}

// unresolved returns the tokens named by the one warning logged since the
// last take, or nil when none was logged. More than one warning fails.
func (l *warningLog) unresolved(t failer) []string {
	t.Helper()
	warnings := l.take(t)
	switch len(warnings) {
	case 0:
		return nil
	case 1:
		return warnings[0].Unresolved
	}
	t.Fatalf("one substitution logged %d warnings, want at most one: %+v", len(warnings), warnings)
	return nil
}

type failer interface {
	Helper()
	Fatalf(format string, args ...any)
}

func droneID(t *testing.T) string {
	t.Helper()
	return semantictest.EntityID(t, "c360", "platform1", "robotics", "mav1", "drone", "001")
}

func entityWith(id string, triples ...message.Triple) *gtypes.EntityState {
	for i := range triples {
		triples[i].Subject = id
	}
	return &gtypes.EntityState{ID: id, Triples: triples}
}

func TestSubstituteOnce(t *testing.T) {
	log := captureWarnings(t)
	id := droneID(t)

	t.Run("a triple value holding a message token", func(t *testing.T) {
		ec := &ExecutionContext{
			EntityID: id,
			Entity: entityWith(id, message.Triple{
				Predicate: semantictest.Predicate(t, "test", "fixture", "note"), Object: "see $message.secret",
			}),
			MessageData: map[string]any{"secret": "LEAK"},
		}

		assert.Equal(t, "note: see $message.secret", ec.SubstituteVariables(t.Context(), "note: $entity.triple.test.fixture.note"))
		assert.Nil(t, log.unresolved(t), "inserted text is never read, so nothing in it is reported")
	})

	t.Run("a message value holding an entity token", func(t *testing.T) {
		ec := &ExecutionContext{EntityID: id, MessageData: map[string]any{"reason": "ask $entity.id"}}

		assert.Equal(t, "why: ask $entity.id; who: "+id,
			ec.SubstituteVariables(t.Context(), "why: $message.reason; who: $entity.id"))
		assert.Nil(t, log.unresolved(t))
	})

	t.Run("a template with no dollar sign", func(t *testing.T) {
		ec := &ExecutionContext{EntityID: id}

		assert.Equal(t, "plain text. entity.id", ec.SubstituteVariables(t.Context(), "plain text. entity.id"))
		assert.Nil(t, log.unresolved(t))
	})
}

func TestTokenEnds(t *testing.T) {
	log := captureWarnings(t)
	id := droneID(t)

	cases := []struct {
		name       string
		ec         *ExecutionContext
		template   string
		want       string
		unresolved []string
	}{
		{
			name: "a longer absent predicate is not read as a shorter present one",
			ec: &ExecutionContext{EntityID: id, Entity: entityWith(id, message.Triple{
				Predicate: semantictest.Predicate(t, "agent", "lineage", "researcher-plan"), Object: "P",
			})},
			template:   "$entity.triple.agent.lineage.researcher-plan-entity",
			want:       "$entity.triple.agent.lineage.researcher-plan-entity",
			unresolved: []string{"$entity.triple.agent.lineage.researcher-plan-entity"},
		},
		{
			name:     "fixed names joined by dots",
			ec:       &ExecutionContext{EntityID: id},
			template: "$entity.org.$entity.platform.chain.$entity.instance",
			want:     "c360.platform1.chain.001",
		},
		{
			// The token grammar takes an underscore, which the predicate
			// grammar does not, so this predicate is written as is.
			name: "a sentence ends on a token",
			ec: &ExecutionContext{EntityID: id, Entity: entityWith(id, message.Triple{
				Predicate: "agent.run.entity_id", Object: "X",
			})},
			template: "ID is $entity.triple.agent.run.entity_id. Call",
			want:     "ID is X. Call",
		},
		{
			name: "names that extend or miss a fixed name",
			ec: &ExecutionContext{
				EntityID:  id,
				State:     &MatchState{RuleID: "rule-a", Iteration: 2, MaxIterations: 5},
				Lifecycle: knowing(id),
			},
			template: "$nowhere $entity.idx $entity.lifecycle.x $state.last_transition $schedule.last_fired_at",
			want:     "$nowhere $entity.idx $entity.lifecycle.x $state.last_transition $schedule.last_fired_at",
			unresolved: []string{
				"$nowhere", "$entity.idx", "$entity.lifecycle.x", "$state.last_transition", "$schedule.last_fired_at",
			},
		},
		{
			name:     "names outside the namespaces",
			ec:       &ExecutionContext{EntityID: id},
			template: "$caller.id and $related.id",
			want:     "$caller.id and $related.id",
		},
		{
			name:     "the value of an absent 3-part predicate",
			ec:       &ExecutionContext{EntityID: id, Entity: entityWith(id)},
			template: "[$entity.triple.gather.child.result.value]",
			want:     "[]",
		},
		{
			name: "a predicate whose last part is value",
			ec: &ExecutionContext{EntityID: id, Entity: entityWith(id, message.Triple{
				Predicate: semantictest.Predicate(t, "autoresearch", "best", "value"), Object: 0.82,
			})},
			template: "$entity.triple.autoresearch.best.value",
			want:     "0.82",
		},
		{
			name:     "a fixed name followed by a hyphen or a slash",
			ec:       &ExecutionContext{EntityID: id, State: &MatchState{Iteration: 2}},
			template: "$entity.instance-$state.iteration/$entity.type",
			want:     "001-2/drone",
		},
		{
			name:     "other dollar signs",
			ec:       &ExecutionContext{EntityID: id},
			template: "$ 5, $5, $$entity.instance, $USD, $entity",
			want:     "$ 5, $5, $001, $USD, $entity",
			// "$entity" without a dot is not a prefix, so it is not reported.
		},
		{
			name:       "an unresolved token stops before a final dot",
			ec:         &ExecutionContext{EntityID: id},
			template:   "see $state.iteration.",
			want:       "see $state.iteration.",
			unresolved: []string{"$state.iteration"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.ec.SubstituteVariables(t.Context(), tc.template))
			assert.Equal(t, tc.unresolved, log.unresolved(t))
		})
	}
}

func TestTripleLength(t *testing.T) {
	log := captureWarnings(t)
	id := droneID(t)
	completed := semantictest.Predicate(t, "gather", "child", "completed")
	subtopics := semantictest.Predicate(t, "coordinator", "decision", "subtopics")

	ec := &ExecutionContext{EntityID: id, Entity: entityWith(id,
		message.Triple{Predicate: completed, Object: "a"},
		message.Triple{Predicate: completed, Object: "b"},
		message.Triple{Predicate: completed, Object: "c"},
		message.Triple{Predicate: subtopics, Object: []any{"x", "y"}},
	)}

	assert.Equal(t, "3", ec.SubstituteVariables(t.Context(), "$entity.triple.gather.child.completed.length"),
		"three scalar triples are three values")
	assert.Equal(t, `["a","b","c"]`, ec.SubstituteVariables(t.Context(), "$entity.triple.gather.child.completed.triples"))
	assert.Equal(t, "2", ec.SubstituteVariables(t.Context(), "$entity.triple.coordinator.decision.subtopics.length"),
		"one list-valued triple counts its elements")
	assert.Equal(t, "0", ec.SubstituteVariables(t.Context(), "$entity.triple.gather.child.missing.length"))
	assert.Nil(t, log.unresolved(t))
}

func TestSubstituteVariablesWarning(t *testing.T) {
	log := captureWarnings(t)
	id := droneID(t)
	ec := &ExecutionContext{EntityID: id, State: &MatchState{RuleID: "rule-a"}}

	got := ec.SubstituteVariables(t.Context(), "$message.a and $message.b")

	assert.Equal(t, "$message.a and $message.b", got)
	warnings := log.take(t)
	require.Len(t, warnings, 1, "every unresolved token is named in one warning")
	assert.Equal(t, []string{"$message.a", "$message.b"}, warnings[0].Unresolved)
	assert.Equal(t, "rule-a", warnings[0].RuleID)
	assert.Equal(t, id, warnings[0].EntityID)
	assert.Equal(t, "$message.a and $message.b", warnings[0].Template)
}

func TestRuleID(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "", (&ExecutionContext{}).RuleID(), "a context with no state")
	assert.Equal(t, "rule-a", (&ExecutionContext{State: &MatchState{RuleID: "rule-a"}}).RuleID())
}

func TestStateTokens(t *testing.T) {
	log := captureWarnings(t)
	ec := &ExecutionContext{State: &MatchState{Iteration: 4, MaxIterations: 10}}

	assert.Equal(t, "4 of 10", ec.SubstituteVariables(t.Context(), "$state.iteration of $state.max_iterations"))
	assert.Nil(t, log.unresolved(t))

	noState := &ExecutionContext{}
	assert.Equal(t, "$state.iteration", noState.SubstituteVariables(t.Context(), "$state.iteration"))
	assert.Equal(t, []string{"$state.iteration"}, log.unresolved(t), "a context with no state leaves the token")
}

func TestNowToken(t *testing.T) {
	log := captureWarnings(t)
	ec := &ExecutionContext{}

	got := ec.SubstituteVariables(t.Context(), "at $now.")

	require.True(t, strings.HasPrefix(got, "at ") && strings.HasSuffix(got, "."), "got %q", got)
	stamp := strings.TrimSuffix(strings.TrimPrefix(got, "at "), ".")
	assert.Regexp(t, `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`, stamp, "RFC 3339 in UTC")
	assert.Nil(t, log.unresolved(t))
}

func TestSubstituteConditionValues(t *testing.T) {
	log := captureWarnings(t)
	id := droneID(t)
	ec := &ExecutionContext{EntityID: id, Entity: entityWith(id,
		message.Triple{Predicate: semantictest.Predicate(t, "coordinator", "decision", "subtopics"), Object: []any{"x", "y"}},
	)}
	conds := []expression.ConditionExpression{
		{Field: "gather.child.completed", Operator: expression.OpLengthEq, Value: "$entity.triple.coordinator.decision.subtopics.length"},
		{Field: "a.b.c", Operator: expression.OpEqual, Value: 3.0},
		{Field: "a.b.d", Operator: expression.OpEqual, Value: "plain"},
	}

	got := substituteConditionValues(t.Context(), conds, ec)

	assert.Equal(t, "2", got[0].Value, "a string value is substituted")
	assert.Equal(t, 3.0, got[1].Value, "a value that is not a string is kept")
	assert.Equal(t, "plain", got[2].Value)
	assert.Equal(t, "$entity.triple.coordinator.decision.subtopics.length", conds[0].Value,
		"the caller's conditions are not changed")
	assert.Nil(t, log.unresolved(t))
	assert.Nil(t, substituteConditionValues(t.Context(), nil, ec))
}

// knowing returns a lifecycle manager that knows id as a mission in flight.
func knowing(id string) *fakeLifecycleManager {
	mgr := newFakeManager()
	mgr.seed("mission", &fakeParticipant{entityID: id, phase: "flying"})
	return mgr
}

// Requirement: rule-substitution/"A handler's own names are filled in the same
// read".
func TestSubstituteWithNames(t *testing.T) {
	log := captureWarnings(t)
	id := droneID(t)

	t.Run("a loop item holding a token comes out as written", func(t *testing.T) {
		ec := &ExecutionContext{EntityID: id}

		got, err := ec.SubstituteVariablesWith(t.Context(), "$entity.instance: $subtopic",
			map[string]string{"subtopic": "see $entity.id"})

		require.NoError(t, err)
		assert.Equal(t, "001: see $entity.id", got)
		assert.Nil(t, log.unresolved(t))
	})

	t.Run("a name ends where a fixed name ends", func(t *testing.T) {
		ec := &ExecutionContext{EntityID: id}

		got, err := ec.SubstituteVariablesWith(t.Context(), "$subtopic. $subtopic-x $subtopics $subtopic_1 $subtopic",
			map[string]string{"subtopic": "S", "subtopic_1": "T"})

		require.NoError(t, err)
		assert.Equal(t, "S. S-x $subtopics T S", got)
		assert.Nil(t, log.unresolved(t), "a dollar sign that names no framework token is not reported")
	})

	t.Run("a framework value naming a handler name is not read again", func(t *testing.T) {
		ec := &ExecutionContext{EntityID: id, MessageData: map[string]any{"m": "$subtopic"}}

		got, err := ec.SubstituteVariablesWith(t.Context(), "$message.m/$subtopic", map[string]string{"subtopic": "S"})

		require.NoError(t, err)
		assert.Equal(t, "$subtopic/S", got)
	})

	t.Run("framework tokens beside names are still reported", func(t *testing.T) {
		ec := &ExecutionContext{EntityID: id}

		got, err := ec.SubstituteVariablesWith(t.Context(), "$item $message.missing", map[string]string{"item": "I"})

		require.NoError(t, err)
		assert.Equal(t, "I $message.missing", got)
		assert.Equal(t, []string{"$message.missing"}, log.unresolved(t))
	})

	t.Run("names that only start like a framework name", func(t *testing.T) {
		ec := &ExecutionContext{EntityID: id}

		got, err := ec.SubstituteVariablesWith(t.Context(), "$entityx $no $statement",
			map[string]string{"entityx": "E", "no": "N", "statement": "S"})

		require.NoError(t, err)
		assert.Equal(t, "E N S", got)
	})

	t.Run("no names", func(t *testing.T) {
		ec := &ExecutionContext{EntityID: id}

		got, err := ec.SubstituteVariablesWith(t.Context(), "$entity.instance $item", nil)

		require.NoError(t, err)
		assert.Equal(t, "001 $item", got)
	})

	for _, name := range []string{"now", "nowx", "nowhere", "entity", "message", "state", "schedule", "", "a-b", "a.b", "a b", "é"} {
		t.Run("the name "+strconv.Quote(name)+" is refused", func(t *testing.T) {
			mgr := knowing(id)
			ec := &ExecutionContext{EntityID: id, Lifecycle: mgr}

			got, err := ec.SubstituteVariablesWith(t.Context(), "$entity.lifecycle.phase $message.missing $item",
				map[string]string{"item": "I", name: "X"})

			require.Error(t, err)
			assert.True(t, errs.IsInvalid(err), "classified invalid: %v", err)
			assert.Contains(t, err.Error(), strconv.Quote(name), "the error names the name")
			assert.Empty(t, got)
			assert.Zero(t, mgr.lookups, "refused before any scan: no lookup")
			assert.Nil(t, log.unresolved(t), "refused before any scan: nothing reported")
		})
	}

	t.Run("a nil context is refused", func(t *testing.T) {
		ec := &ExecutionContext{EntityID: id}
		var nilCtx context.Context

		got, err := ec.SubstituteVariablesWith(nilCtx, "$entity.instance", nil)

		require.Error(t, err)
		assert.True(t, errs.IsInvalid(err), "classified invalid: %v", err)
		assert.Empty(t, got)
	})
}
