package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

// TestManagerWithoutClientRefusesNATSWork: a manager built with no NATS client and nothing
// injected registers workflows and answers definition queries, and every call that needs NATS
// fails with the one no-client error, without a panic (design D2, owner question 1).
func TestManagerWithoutClientRefusesNATSWork(t *testing.T) {
	t.Parallel()
	mgr := NewManager(nil, nil)
	if err := mgr.Register(lifecycle{}.fixtureWorkflow()); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, err := mgr.GetWorkflowDefinition("fixture"); err != nil {
		t.Errorf("GetWorkflowDefinition: %v", err)
	}
	if defs := mgr.ListWorkflows(); len(defs) != 1 || defs[0].Workflow != "fixture" {
		t.Errorf("ListWorkflows = %+v, want the fixture workflow", defs)
	}
	if err := mgr.AssertRuleWritable("fixture", "note"); err != nil {
		t.Errorf("AssertRuleWritable: %v", err)
	}
	if _, err := mgr.FieldType("fixture", "note"); err != nil {
		t.Errorf("FieldType: %v", err)
	}

	ctx := context.Background()
	const id = "c360.platform1.gcs.lifecycle.mission.no-client"
	body, err := json.Marshal(fixtureMission{ID: id, PhaseF: "planning"})
	if err != nil {
		t.Fatalf("marshal initial state: %v", err)
	}
	calls := []struct {
		name string
		call func() error
	}{
		{"Get", func() error { _, err := mgr.Get(ctx, "fixture", id); return err }},
		{"GetWithRevision", func() error { _, _, err := mgr.GetWithRevision(ctx, "fixture", id); return err }},
		{"GetRaw", func() error { _, err := mgr.GetRaw(ctx, id); return err }},
		{"Create", func() error { return mgr.Create(ctx, &fixtureMission{ID: id, PhaseF: "planning"}) }},
		{"CreateFromOperator", func() error { _, err := mgr.CreateFromOperator(ctx, "fixture", body); return err }},
		{"Transition", func() error {
			return mgr.Transition(ctx, "fixture", id, "flying", TransitionSourceOperator, "")
		}},
		{"TransitionWith", func() error {
			return mgr.TransitionWith(ctx, "fixture", id, "flying", TransitionSourceOperator, "",
				func(Participant) error { return nil })
		}},
		{"UpdateFromOperator", func() error {
			return mgr.UpdateFromOperator(ctx, "fixture", id, map[string]any{"note": "n"})
		}},
		{"Complete", func() error { return mgr.Complete(ctx, "fixture", id) }},
		{"Fail", func() error { return mgr.Fail(ctx, "fixture", id, "lost link") }},
		{"Despawn", func() error { return mgr.Despawn(ctx, "fixture", id) }},
		{"DespawnWith", func() error {
			return mgr.DespawnWith(ctx, "fixture", id, TransitionSourceOperator, "")
		}},
		{"List", func() error { _, err := mgr.List(ctx, "fixture", ListOptions{}); return err }},
		{"Watch", func() error { return mgr.Watch(ctx, "fixture", func(Participant) error { return nil }) }},
		{"WatchEvents", func() error { return mgr.WatchEvents(ctx, "fixture", func(Event) error { return nil }) }},
		{"History", func() error { _, err := mgr.History(ctx, "fixture", id); return err }},
		{"Children", func() error { _, err := mgr.Children(ctx, id, ChildOptions{}); return err }},
		{"References", func() error { _, err := mgr.References(ctx, id); return err }},
		{"LookupByEntityID", func() error { _, err := mgr.LookupByEntityID(ctx, id); return err }},
	}
	for _, c := range calls {
		err := callWithoutPanic(c.call)
		if !errors.Is(err, errNoNATSClient) {
			t.Errorf("%s: error = %v, want %v", c.name, err, errNoNATSClient)
		}
	}
}

// callWithoutPanic runs call and reports a panic as an error, so one panicking call does not hide
// the others.
func callWithoutPanic(call func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return call()
}
