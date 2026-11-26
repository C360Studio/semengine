package rule

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/internal/harness/semantictest"
	"github.com/c360studio/semengine/pkg/lifecycle"
	"github.com/c360studio/semengine/processor/rule/expression"
)

// Requirement: rule-substitution/"A token ends where its name ends" (the four
// $entity.lifecycle.* names), "An unresolved token stays and is reported" and
// "Lifecycle names read the context's lifecycle manager".

// fakeParticipant is a lifecycle participant the fake manager returns.
type fakeParticipant struct {
	entityID string
	workflow string
	phase    string
	terminal bool
}

func (p *fakeParticipant) EntityID() string       { return p.entityID }
func (p *fakeParticipant) Workflow() string       { return p.workflow }
func (p *fakeParticipant) Phase() string          { return p.phase }
func (p *fakeParticipant) IsTerminal() bool       { return p.terminal }
func (p *fakeParticipant) ParentEntityID() string { return "" }

// fakeLifecycleManager answers the two calls substitution makes from maps the
// test fills, and counts lookups and records the context each one got. Each
// test builds its own.
type fakeLifecycleManager struct {
	participants map[string]*fakeParticipant // entity ID -> participant
	workflowDefs map[string]lifecycle.WorkflowDef
	lookups      int
	lookupCtx    context.Context
}

func newFakeManager() *fakeLifecycleManager {
	return &fakeLifecycleManager{
		participants: make(map[string]*fakeParticipant),
		workflowDefs: make(map[string]lifecycle.WorkflowDef),
	}
}

func (m *fakeLifecycleManager) seed(workflow string, p *fakeParticipant) {
	p.workflow = workflow
	m.participants[p.entityID] = p
}

func (m *fakeLifecycleManager) LookupByEntityID(ctx context.Context, entityID string) (lifecycle.Participant, error) {
	m.lookups++
	m.lookupCtx = ctx
	if p, ok := m.participants[entityID]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("%w: %s", lifecycle.ErrEntityNotFound, entityID)
}

func (m *fakeLifecycleManager) GetWorkflowDefinition(workflow string) (lifecycle.WorkflowDef, error) {
	def, ok := m.workflowDefs[workflow]
	if !ok {
		return lifecycle.WorkflowDef{}, fmt.Errorf("workflow not registered: %s", workflow)
	}
	return def, nil
}

func lifecycleEntityID(t *testing.T, instance string) string {
	t.Helper()
	return semantictest.EntityID(t, "test", "rule", "lifecycle", "mission", "entity", instance)
}

func TestLifecycleTokens(t *testing.T) {
	log := captureWarnings(t)
	cases := []struct {
		name        string
		participant fakeParticipant
		template    string
		want        string
	}{
		{"phase", fakeParticipant{phase: "flying"}, "phase=$entity.lifecycle.phase", "phase=flying"},
		{"terminal true", fakeParticipant{phase: "completed", terminal: true}, "t=$entity.lifecycle.terminal", "t=true"},
		{"terminal false", fakeParticipant{phase: "flying"}, "t=$entity.lifecycle.terminal", "t=false"},
		{"workflow", fakeParticipant{phase: "planning"}, "w=$entity.lifecycle.workflow", "w=mission"},
		{"before a full stop", fakeParticipant{phase: "flying"}, "now $entity.lifecycle.phase.", "now flying."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := lifecycleEntityID(t, "001")
			mgr := newFakeManager()
			p := tc.participant
			p.entityID = id
			mgr.seed("mission", &p)
			ec := &ExecutionContext{EntityID: id, Lifecycle: mgr}

			assert.Equal(t, tc.want, ec.SubstituteVariables(t.Context(), tc.template))
			assert.Nil(t, log.unresolved(t))
		})
	}
}

func TestLifecycleLookupGetsTheCallersContext(t *testing.T) {
	t.Parallel()
	id := lifecycleEntityID(t, "001")
	mgr := newFakeManager()
	mgr.seed("mission", &fakeParticipant{entityID: id, phase: "flying"})
	ec := &ExecutionContext{EntityID: id, Lifecycle: mgr}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	ec.SubstituteVariables(ctx, "$entity.lifecycle.phase")

	require.Equal(t, 1, mgr.lookups)
	assert.True(t, mgr.lookupCtx == ctx, "the lookup runs under the context substitution was given")
}

func TestSubstituteConditionValuesLifecycle(t *testing.T) {
	t.Parallel()
	id := lifecycleEntityID(t, "001")
	mgr := newFakeManager()
	mgr.seed("mission", &fakeParticipant{entityID: id, phase: "flying"})
	ec := &ExecutionContext{EntityID: id, Lifecycle: mgr}
	conditions := []expression.ConditionExpression{{
		Field: "test.fixture.phase", Operator: expression.OpEqual, Value: "$entity.lifecycle.phase",
	}}

	got := substituteConditionValues(t.Context(), conditions, ec)

	assert.Equal(t, "flying", got[0].Value)
}

func TestLifecycleWorkflowDefinition(t *testing.T) {
	log := captureWarnings(t)
	id := lifecycleEntityID(t, "001")
	def := lifecycle.WorkflowDef{
		Workflow:        "mission",
		EntityIDPattern: "*.test.gcs.lifecycle.mission.*",
		PhasePredicate:  "test.mission.phase",
	}

	t.Run("workflow and workflow_def side by side", func(t *testing.T) {
		mgr := newFakeManager()
		mgr.seed("mission", &fakeParticipant{entityID: id, phase: "planning"})
		mgr.workflowDefs["mission"] = def
		ec := &ExecutionContext{EntityID: id, Lifecycle: mgr}

		got := ec.SubstituteVariables(t.Context(), "w=$entity.lifecycle.workflow|def=$entity.lifecycle.workflow_def")

		encoded, ok := strings.CutPrefix(got, "w=mission|def=")
		require.True(t, ok, "got %q", got)
		var decoded lifecycle.WorkflowDef
		require.NoError(t, json.Unmarshal([]byte(encoded), &decoded), "workflow_def is the definition as JSON")
		assert.Equal(t, def, decoded)
		assert.Nil(t, log.unresolved(t))
	})

	t.Run("a failed definition lookup leaves the token", func(t *testing.T) {
		mgr := newFakeManager()
		mgr.seed("mission", &fakeParticipant{entityID: id, phase: "planning"})
		ec := &ExecutionContext{EntityID: id, Lifecycle: mgr}

		assert.Equal(t, "def=$entity.lifecycle.workflow_def",
			ec.SubstituteVariables(t.Context(), "def=$entity.lifecycle.workflow_def"))
		assertDefinitionFailureLogged(t, log)
	})

	t.Run("a failed definition lookup does not change the workflow token beside it", func(t *testing.T) {
		mgr := newFakeManager()
		mgr.seed("mission", &fakeParticipant{entityID: id, phase: "planning"})
		ec := &ExecutionContext{EntityID: id, Lifecycle: mgr}

		assert.Equal(t, "w=mission|def=$entity.lifecycle.workflow_def",
			ec.SubstituteVariables(t.Context(), "w=$entity.lifecycle.workflow|def=$entity.lifecycle.workflow_def"))
		assertDefinitionFailureLogged(t, log)
	})
}

// assertDefinitionFailureLogged checks the two warnings a failed definition
// lookup logs: the failure with the workflow, then the token left unresolved.
func assertDefinitionFailureLogged(t *testing.T, log *warningLog) {
	t.Helper()
	warnings := log.take(t)
	require.Len(t, warnings, 2, "%+v", warnings)
	assert.Equal(t, "lifecycle workflow definition lookup failed", warnings[0].Msg)
	assert.Equal(t, []string{"$entity.lifecycle.workflow_def"}, warnings[1].Unresolved)
}

func TestLifecycleTokensUnresolved(t *testing.T) {
	log := captureWarnings(t)
	id := lifecycleEntityID(t, "001")

	t.Run("no lifecycle manager", func(t *testing.T) {
		ec := &ExecutionContext{EntityID: id}
		assert.Equal(t, "phase=$entity.lifecycle.phase", ec.SubstituteVariables(t.Context(), "phase=$entity.lifecycle.phase"))
		assert.Equal(t, []string{"$entity.lifecycle.phase"}, log.unresolved(t))
	})

	t.Run("an entity the manager does not know", func(t *testing.T) {
		ec := &ExecutionContext{EntityID: lifecycleEntityID(t, "missing"), Lifecycle: newFakeManager()}
		assert.Equal(t, "phase=$entity.lifecycle.phase", ec.SubstituteVariables(t.Context(), "phase=$entity.lifecycle.phase"))
		assert.Equal(t, []string{"$entity.lifecycle.phase"}, log.unresolved(t))
	})

	t.Run("no entity ID", func(t *testing.T) {
		mgr := newFakeManager()
		ec := &ExecutionContext{Lifecycle: mgr}
		assert.Equal(t, "$entity.lifecycle.phase", ec.SubstituteVariables(t.Context(), "$entity.lifecycle.phase"))
		assert.Equal(t, []string{"$entity.lifecycle.phase"}, log.unresolved(t))
		assert.Zero(t, mgr.lookups, "no lookup is made without an entity ID")
	})

	t.Run("a name that extends a lifecycle name", func(t *testing.T) {
		mgr := newFakeManager()
		mgr.seed("mission", &fakeParticipant{entityID: id, phase: "flying"})
		ec := &ExecutionContext{EntityID: id, Lifecycle: mgr}
		assert.Equal(t, "$entity.lifecycle.phases $entity.lifecycle.workflowx",
			ec.SubstituteVariables(t.Context(), "$entity.lifecycle.phases $entity.lifecycle.workflowx"))
		assert.Equal(t, []string{"$entity.lifecycle.phases", "$entity.lifecycle.workflowx"}, log.unresolved(t))
	})
}

func TestLifecycleLookupOncePerContext(t *testing.T) {
	t.Parallel()
	id := lifecycleEntityID(t, "001")

	t.Run("no lifecycle token, no lookup", func(t *testing.T) {
		t.Parallel()
		mgr := newFakeManager()
		mgr.seed("mission", &fakeParticipant{entityID: id, phase: "planning"})
		ec := &ExecutionContext{EntityID: id, Lifecycle: mgr}

		ec.SubstituteVariables(t.Context(), "plain text, no tokens")
		ec.SubstituteVariables(t.Context(), "$entity.id only")
		assert.Zero(t, mgr.lookups)
	})

	t.Run("a found participant is looked up once", func(t *testing.T) {
		t.Parallel()
		mgr := newFakeManager()
		mgr.seed("mission", &fakeParticipant{entityID: id, phase: "flying"})
		ec := &ExecutionContext{EntityID: id, Lifecycle: mgr}

		assert.Equal(t, "flying mission", ec.SubstituteVariables(t.Context(), "$entity.lifecycle.phase $entity.lifecycle.workflow"))
		assert.Equal(t, "false", ec.SubstituteVariables(t.Context(), "$entity.lifecycle.terminal"))
		assert.Equal(t, 1, mgr.lookups)
	})

	t.Run("a failed lookup is kept and not repeated", func(t *testing.T) {
		t.Parallel()
		mgr := newFakeManager()
		ec := &ExecutionContext{EntityID: id, Lifecycle: mgr}

		ec.SubstituteVariables(t.Context(), "a=$entity.lifecycle.phase")
		ec.SubstituteVariables(t.Context(), "b=$entity.lifecycle.workflow")
		assert.Equal(t, 1, mgr.lookups)
	})
}

func TestPopulateLifecycleStateFields(t *testing.T) {
	t.Parallel()

	t.Run("fills the three condition keys", func(t *testing.T) {
		t.Parallel()
		id := lifecycleEntityID(t, "001")
		mgr := newFakeManager()
		mgr.seed("mission", &fakeParticipant{entityID: id, phase: "flying"})
		fields := expression.StateFields{}

		require.NoError(t, populateLifecycleStateFields(t.Context(), mgr, id, fields))

		assert.Equal(t, expression.StateFields{
			"$entity.lifecycle.phase":    "flying",
			"$entity.lifecycle.workflow": "mission",
			"$entity.lifecycle.terminal": false,
		}, fields)
	})

	t.Run("no manager fills nothing", func(t *testing.T) {
		t.Parallel()
		fields := expression.StateFields{}
		require.NoError(t, populateLifecycleStateFields(t.Context(), nil, lifecycleEntityID(t, "002"), fields))
		assert.Empty(t, fields)
	})

	t.Run("an unknown entity returns the lookup error and fills nothing", func(t *testing.T) {
		t.Parallel()
		fields := expression.StateFields{}
		err := populateLifecycleStateFields(t.Context(), newFakeManager(), lifecycleEntityID(t, "missing"), fields)
		assert.True(t, errors.Is(err, lifecycle.ErrEntityNotFound), "got %v", err)
		assert.Empty(t, fields)
	})
}

func TestEvaluatorReadsLifecycleStateFields(t *testing.T) {
	t.Parallel()
	expr := expression.LogicalExpression{
		Logic: expression.LogicAnd,
		Conditions: []expression.ConditionExpression{
			{Field: "$entity.lifecycle.phase", Operator: expression.OpEqual, Value: "flying"},
		},
	}
	for _, tc := range []struct {
		phase string
		want  bool
	}{{"flying", true}, {"planning", false}} {
		t.Run(tc.phase, func(t *testing.T) {
			t.Parallel()
			id := lifecycleEntityID(t, "001")
			mgr := newFakeManager()
			mgr.seed("mission", &fakeParticipant{entityID: id, phase: tc.phase})
			fields := expression.StateFields{}
			require.NoError(t, populateLifecycleStateFields(t.Context(), mgr, id, fields))

			got, err := expression.NewExpressionEvaluator().EvaluateWithStateAndMessage(nil, fields, nil, expr)

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
