package lifecycle

import (
	"errors"
	"reflect"
	"testing"
)

// subtask is a second workflow's participant: its own phase predicate, one
// operator-writable field, and IsTerminal read from its phase, as an app
// writes it. The integration tests use it as a child workflow.
type subtask struct {
	ID     string `json:"entity_id" lifecycle:"id"`
	PhaseF string `json:"phase" lifecycle:"phase,predicate=subtask.lifecycle.phase"`
	Owner  string `json:"owner,omitempty" lifecycle:"operator_writable,predicate=subtask.assignment.owner"`
}

func (s *subtask) EntityID() string       { return s.ID }
func (s *subtask) Workflow() string       { return "subtask" }
func (s *subtask) Phase() string          { return s.PhaseF }
func (s *subtask) ParentEntityID() string { return "" }
func (s *subtask) IsTerminal() bool {
	switch s.PhaseF {
	case "done", "failed", "cancelled":
		return true
	}
	return false
}

// subtaskWorkflow declares subtask. From queued no terminal is one step away;
// from running two are (done and failed); from paused one is (cancelled).
func subtaskWorkflow() Workflow {
	return Workflow{
		Name:            "subtask",
		EntityIDPattern: "*.*.gcs.lifecycle.subtask.*",
		Transitions: Transitions{
			"queued":    {"running"},
			"running":   {"done", "failed", "paused"},
			"paused":    {"running", "cancelled"},
			"done":      {},
			"failed":    {},
			"cancelled": {},
		},
		PhasePredicate:             "subtask.lifecycle.phase",
		Schema:                     reflect.TypeOf(subtask{}),
		OperatorWritablePredicates: []string{"subtask.assignment.owner"},
		AuditPredicates:            AuditSpec{Note: "subtask.transition.note"},
	}
}

// TestRegisterRefusesAnInvalidWorkflowDeclaration proves Register refuses each
// malformed declaration a workflow definition's validate names, and registers
// nothing for it: each case changes one field of a declaration that uses every
// optional field and that the same test shows accepted. A missing required
// field, a predicate the vocabulary does not declare, a child or reference with
// an empty or duplicate predicate, or a malformed target pattern is
// ErrInvalidWorkflow; an inconsistent transitions table is
// ErrInvalidTransitionsTable. A second Register of an accepted name is
// ErrWorkflowAlreadyRegistered.
func TestRegisterRefusesAnInvalidWorkflowDeclaration(t *testing.T) {
	t.Parallel()
	full := func() Workflow {
		w := subtaskWorkflow()
		w.ChildWorkflows = []ChildSpec{{Workflow: "fixture", LinkPredicate: "mission.child.submission"}}
		w.ReferencePredicates = []ReferenceSpec{{Predicate: "mission.assignment.drone", TargetPattern: "*.*.gcs.fleet.drone.*"}}
		return w
	}

	tests := []struct {
		name   string
		change func(*Workflow)
		want   error
	}{
		{"no name", func(w *Workflow) { w.Name = "" }, ErrInvalidWorkflow},
		{"no entity ID pattern", func(w *Workflow) { w.EntityIDPattern = "" }, ErrInvalidWorkflow},
		{"an entity ID pattern of five segments", func(w *Workflow) { w.EntityIDPattern = "*.gcs.lifecycle.subtask.*" }, ErrInvalidWorkflow},
		{"no phase predicate", func(w *Workflow) { w.PhasePredicate = "" }, ErrInvalidWorkflow},
		{"an undeclared phase predicate", func(w *Workflow) { w.PhasePredicate = "subtask.lifecycle.stage" }, ErrInvalidWorkflow},
		{"no schema", func(w *Workflow) { w.Schema = nil }, ErrInvalidWorkflow},
		{"a transition to an undeclared phase", func(w *Workflow) {
			w.Transitions = Transitions{"queued": {"archived"}}
		}, ErrInvalidTransitionsTable},
		{"an undeclared audit predicate", func(w *Workflow) { w.AuditPredicates.Source = "subtask.transition.source" }, ErrInvalidWorkflow},
		{"an undeclared operator-writable predicate", func(w *Workflow) {
			w.OperatorWritablePredicates = []string{"subtask.assignment.owner", "subtask.assignment.robot"}
		}, ErrInvalidWorkflow},
		{"a child with no workflow", func(w *Workflow) { w.ChildWorkflows[0].Workflow = "" }, ErrInvalidWorkflow},
		{"a child with no link predicate", func(w *Workflow) { w.ChildWorkflows[0].LinkPredicate = "" }, ErrInvalidWorkflow},
		{"a child with an undeclared link predicate", func(w *Workflow) {
			w.ChildWorkflows[0].LinkPredicate = "mission.child.unknown"
		}, ErrInvalidWorkflow},
		{"two children on one link predicate", func(w *Workflow) {
			w.ChildWorkflows = append(w.ChildWorkflows, ChildSpec{Workflow: "subtask", LinkPredicate: "mission.child.submission"})
		}, ErrInvalidWorkflow},
		{"a reference with no predicate", func(w *Workflow) { w.ReferencePredicates[0].Predicate = "" }, ErrInvalidWorkflow},
		{"a reference with an undeclared predicate", func(w *Workflow) {
			w.ReferencePredicates[0].Predicate = "mission.assignment.robot"
		}, ErrInvalidWorkflow},
		{"a reference with a target pattern of three segments", func(w *Workflow) {
			w.ReferencePredicates[0].TargetPattern = "*.drone.*"
		}, ErrInvalidWorkflow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mgr := NewManager(nil, nil)
			w := full()
			tt.change(&w)
			err := mgr.Register(w)
			if !errors.Is(err, tt.want) {
				t.Fatalf("Register = %v, want %v", err, tt.want)
			}
			if defs := mgr.ListWorkflows(); len(defs) != 0 {
				t.Fatalf("a refused declaration was registered: %+v", defs)
			}
		})
	}

	t.Run("the declaration every case changes is accepted once", func(t *testing.T) {
		t.Parallel()
		mgr := NewManager(nil, nil)
		if err := mgr.Register(full()); err != nil {
			t.Fatalf("Register: %v", err)
		}
		if err := mgr.Register(full()); !errors.Is(err, ErrWorkflowAlreadyRegistered) {
			t.Fatalf("second Register = %v, want %v", err, ErrWorkflowAlreadyRegistered)
		}
		if defs := mgr.ListWorkflows(); len(defs) != 1 || defs[0].Workflow != "subtask" {
			t.Fatalf("ListWorkflows = %+v, want the one subtask workflow", defs)
		}
	})
}

// TestAssertRuleWritableAllowsOnlyOperatorWritableFields proves the rule
// layer's write check is default-deny: on the fixture workflow only the two
// fields tagged operator_writable may be set; the ID, the phase, a reference,
// a readonly field, a field with no lifecycle tag and a name the schema does
// not have are each ErrFieldNotOperatorWritable, and an unregistered workflow
// is ErrWorkflowNotRegistered. FieldType, the rule executor's typed read,
// answers the field's Go type for a writable field and refuses what the check
// refuses.
func TestAssertRuleWritableAllowsOnlyOperatorWritableFields(t *testing.T) {
	t.Parallel()
	mgr := NewManager(nil, nil)
	if err := mgr.Register(lifecycle{}.fixtureWorkflow()); err != nil {
		t.Fatalf("Register: %v", err)
	}

	for _, field := range []string{"owner_org_id", "note"} {
		if err := mgr.AssertRuleWritable("fixture", field); err != nil {
			t.Errorf("AssertRuleWritable(%q) = %v, want nil", field, err)
		}
		got, err := mgr.FieldType("fixture", field)
		if err != nil || got != reflect.TypeOf("") {
			t.Errorf("FieldType(%q) = %v, %v; want string", field, got, err)
		}
	}
	for _, field := range []string{"entity_id", "phase", "drone_id", "last_at", "untagged", "no_such_field"} {
		if err := mgr.AssertRuleWritable("fixture", field); !errors.Is(err, ErrFieldNotOperatorWritable) {
			t.Errorf("AssertRuleWritable(%q) = %v, want %v", field, err, ErrFieldNotOperatorWritable)
		}
		if got, err := mgr.FieldType("fixture", field); !errors.Is(err, ErrFieldNotOperatorWritable) || got != nil {
			t.Errorf("FieldType(%q) = %v, %v; want nil, %v", field, got, err, ErrFieldNotOperatorWritable)
		}
	}
	if err := mgr.AssertRuleWritable("unregistered", "note"); !errors.Is(err, ErrWorkflowNotRegistered) {
		t.Errorf("AssertRuleWritable on an unregistered workflow = %v, want %v", err, ErrWorkflowNotRegistered)
	}
}

// TestListWorkflowsDescribesEveryRegisteredWorkflowInNameOrder proves
// ListWorkflows returns one definition per registered workflow, sorted by name
// whatever the registration order, each carrying the declared name,
// transitions, pattern and phase predicate and the operator-writable fields and
// predicates read from the schema's tags, sorted. GetWorkflowDefinition answers
// the same definition for one name and ErrWorkflowNotRegistered for an unknown
// one; a Manager with nothing registered lists nothing.
func TestListWorkflowsDescribesEveryRegisteredWorkflowInNameOrder(t *testing.T) {
	t.Parallel()
	mgr := NewManager(nil, nil)
	if defs := mgr.ListWorkflows(); len(defs) != 0 {
		t.Fatalf("ListWorkflows before Register = %+v, want none", defs)
	}
	for _, w := range []Workflow{subtaskWorkflow(), lifecycle{}.fixtureWorkflow()} {
		if err := mgr.Register(w); err != nil {
			t.Fatalf("Register %q: %v", w.Name, err)
		}
	}

	want := []WorkflowDef{
		{
			Workflow:                   "fixture",
			Transitions:                lifecycle{}.fixtureWorkflow().Transitions,
			EntityIDPattern:            "*.*.gcs.lifecycle.mission.*",
			PhasePredicate:             "mission.lifecycle.phase",
			OperatorWritableFields:     []string{"note", "owner_org_id"},
			OperatorWritablePredicates: []string{"mission.annotation.note", "mission.identity.owner-org-id"},
		},
		{
			Workflow:                   "subtask",
			Transitions:                subtaskWorkflow().Transitions,
			EntityIDPattern:            "*.*.gcs.lifecycle.subtask.*",
			PhasePredicate:             "subtask.lifecycle.phase",
			OperatorWritableFields:     []string{"owner"},
			OperatorWritablePredicates: []string{"subtask.assignment.owner"},
		},
	}
	if got := mgr.ListWorkflows(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ListWorkflows =\n%+v\nwant\n%+v", got, want)
	}
	got, err := mgr.GetWorkflowDefinition("subtask")
	if err != nil || !reflect.DeepEqual(got, want[1]) {
		t.Fatalf("GetWorkflowDefinition(subtask) = %+v, %v; want %+v", got, err, want[1])
	}
	if _, err := mgr.GetWorkflowDefinition("unregistered"); !errors.Is(err, ErrWorkflowNotRegistered) {
		t.Fatalf("GetWorkflowDefinition(unregistered) = %v, want %v", err, ErrWorkflowNotRegistered)
	}
}
