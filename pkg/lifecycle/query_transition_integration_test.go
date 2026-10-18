//go:build integration

// Integration tests for the Manager's query surface (List, Children) and its
// terminal transitions (Complete, Fail) on natsfixture: a production Manager
// (NewManager) over a broker of each test's own, its reads and writes answered
// by the KV-backed responders of manager_integration_test.go, and every outcome
// read back from ENTITY_STATES or through the Manager.

package lifecycle

import (
	"bytes"
	"log/slog"
	"sort"
	"testing"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

const (
	missionPrefix = "c360.platform1.gcs.lifecycle.mission."
	subtaskPrefix = "c360.platform1.gcs.lifecycle.subtask."
)

// appendTriples adds triples to a stored entity at its current revision, as an
// upstream emitter's write would: lifecycle itself writes no child link.
func appendTriples(t *testing.T, kv jetstream.KeyValue, entityID string, triples ...message.Triple) {
	t.Helper()
	entry, err := kv.Get(t.Context(), entityID)
	require.NoError(t, err)
	var state graph.EntityState
	require.NoError(t, graph.UnmarshalEntityState(entityID, entry.Value(), &state))
	state.Triples = append(state.Triples, triples...)
	body, err := graph.MarshalEntityState(&state)
	require.NoError(t, err)
	_, err = kv.Update(t.Context(), entityID, body, entry.Revision())
	require.NoError(t, err)
}

// putRawEntity stores an entity no workflow created: a valid stored value with
// no phase triple.
func putRawEntity(t *testing.T, kv jetstream.KeyValue, entityID string) {
	t.Helper()
	body, err := graph.MarshalEntityState(&graph.EntityState{ID: entityID, Triples: []message.Triple{
		{Subject: entityID, Predicate: "some.other.predicate", Object: "raw", Source: fixtureSource, Timestamp: fixtureTime},
	}})
	require.NoError(t, err)
	_, err = kv.Create(t.Context(), entityID, body)
	require.NoError(t, err)
}

// revisionOf reads an entity's revision straight from ENTITY_STATES.
func revisionOf(t *testing.T, kv jetstream.KeyValue, entityID string) uint64 {
	t.Helper()
	entry, err := kv.Get(t.Context(), entityID)
	require.NoError(t, err)
	return entry.Revision()
}

// storedObject reads one predicate's object straight from ENTITY_STATES.
func storedObject(t *testing.T, kv jetstream.KeyValue, entityID, predicate string) any {
	t.Helper()
	entry, err := kv.Get(t.Context(), entityID)
	require.NoError(t, err)
	var state graph.EntityState
	require.NoError(t, graph.UnmarshalEntityState(entityID, entry.Value(), &state))
	for _, triple := range state.Triples {
		if triple.Predicate == predicate {
			return triple.Object
		}
	}
	return nil
}

// TestIntegration_ListFiltersAndPagesOneWorkflow proves List returns the
// projected participants of one workflow only: keys outside its pattern and a
// key inside it with no phase triple are left out without an error. Phase,
// Active (the participant's own IsTerminal) and Match (one or more JSON field
// names, compared by value) each narrow the result and combine; Offset and
// Limit page what the filters kept. A Match key the schema does not have is
// refused with no result, and an unregistered workflow is
// ErrWorkflowNotRegistered.
func TestIntegration_ListFiltersAndPagesOneWorkflow(t *testing.T) {
	client := fixtureClient(t)
	ctx := t.Context()
	kv := startKVBackedResponders(t, client)
	mgr := NewManager(client, nil)
	require.NoError(t, mgr.Register(subtaskWorkflow()))
	require.NoError(t, mgr.Register(lifecycle{}.fixtureWorkflow()))

	created := map[string]*subtask{}
	for _, s := range []*subtask{
		{ID: subtaskPrefix + "s1", PhaseF: "queued", Owner: "ana"},
		{ID: subtaskPrefix + "s2", PhaseF: "running", Owner: "ana"},
		{ID: subtaskPrefix + "s3", PhaseF: "running", Owner: "bo"},
		{ID: subtaskPrefix + "s4", PhaseF: "done", Owner: "ana"},
		{ID: subtaskPrefix + "s5", PhaseF: "cancelled"},
	} {
		require.NoError(t, mgr.Create(ctx, s))
		created[s.ID] = s
	}
	require.NoError(t, mgr.Create(ctx, &fixtureMission{ID: missionPrefix + "m1", PhaseF: "planning"}))
	putRawEntity(t, kv, subtaskPrefix+"raw")

	// ids checks every listed participant is a subtask equal to the one created
	// under its ID, and returns the IDs sorted: List promises no order.
	ids := func(t *testing.T, got []Participant) []string {
		t.Helper()
		out := make([]string, 0, len(got))
		for _, p := range got {
			s, ok := p.(*subtask)
			require.Truef(t, ok, "listed %T, want *subtask", p)
			require.Equal(t, created[s.ID], s)
			out = append(out, s.ID)
		}
		sort.Strings(out)
		return out
	}
	s := func(names ...string) []string {
		out := make([]string, 0, len(names))
		for _, name := range names {
			out = append(out, subtaskPrefix+name)
		}
		return out
	}

	for _, tt := range []struct {
		name string
		opts ListOptions
		want []string
	}{
		{"no options", ListOptions{}, s("s1", "s2", "s3", "s4", "s5")},
		{"phase", ListOptions{Phase: "running"}, s("s2", "s3")},
		{"active", ListOptions{Active: true}, s("s1", "s2", "s3")},
		{"match", ListOptions{Match: map[string]any{"owner": "ana"}}, s("s1", "s2", "s4")},
		{"match on two fields", ListOptions{Match: map[string]any{"owner": "ana", "phase": "done"}}, s("s4")},
		{"phase, active and match", ListOptions{Phase: "running", Active: true, Match: map[string]any{"owner": "ana"}}, s("s2")},
		{"a phase nothing is in", ListOptions{Phase: "paused"}, s()},
		{"offset at the end", ListOptions{Offset: 5}, s()},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mgr.List(ctx, "subtask", tt.opts)
			require.NoError(t, err)
			require.Equal(t, tt.want, ids(t, got))
		})
	}

	t.Run("offset and limit page what the filters kept", func(t *testing.T) {
		for _, tt := range []struct {
			opts ListOptions
			from []string
			want int
		}{
			{ListOptions{Limit: 2}, s("s1", "s2", "s3", "s4", "s5"), 2},
			{ListOptions{Offset: 1}, s("s1", "s2", "s3", "s4", "s5"), 4},
			{ListOptions{Offset: 3, Limit: 5}, s("s1", "s2", "s3", "s4", "s5"), 2},
			{ListOptions{Phase: "running", Offset: 1}, s("s2", "s3"), 1},
			{ListOptions{Active: true, Limit: 2}, s("s1", "s2", "s3"), 2},
		} {
			listed, err := mgr.List(ctx, "subtask", tt.opts)
			require.NoError(t, err)
			got := ids(t, listed)
			require.Lenf(t, got, tt.want, "List(%+v)", tt.opts)
			require.Subsetf(t, tt.from, got, "List(%+v)", tt.opts)
		}
	})

	t.Run("another workflow lists its own entities", func(t *testing.T) {
		got, err := mgr.List(ctx, "fixture", ListOptions{})
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, missionPrefix+"m1", got[0].EntityID())
	})

	t.Run("a match key the schema does not have is refused", func(t *testing.T) {
		got, err := mgr.List(ctx, "subtask", ListOptions{Match: map[string]any{"assignee": "ana"}})
		require.Error(t, err)
		require.Nil(t, got)
	})

	t.Run("an unregistered workflow is refused", func(t *testing.T) {
		_, err := mgr.List(ctx, "unregistered", ListOptions{})
		require.ErrorIs(t, err, ErrWorkflowNotRegistered)
	})
}

// TestIntegration_ChildrenLoadsLinkedChildrenAcrossWorkflows proves Children
// follows each ChildSpec's link predicate on the parent and loads every linked
// child through its own workflow, across workflows, sorted by workflow then
// entity ID. A predicate that is not a declared link and a link whose object is
// not a string are ignored; a link to an entity that is gone is left out, and a
// link into a workflow that is not registered fails the call with
// ErrWorkflowNotRegistered. ChildOptions.Workflow narrows to one child
// workflow, and Offset and Limit page the children kept. An unknown parent is
// ErrEntityNotFound, and an entity outside every registered pattern is refused.
func TestIntegration_ChildrenLoadsLinkedChildrenAcrossWorkflows(t *testing.T) {
	client := fixtureClient(t)
	ctx := t.Context()
	kv := startKVBackedResponders(t, client)
	mgr := NewManager(client, nil)
	parent := lifecycle{}.fixtureWorkflow()
	parent.ChildWorkflows = []ChildSpec{
		{Workflow: "subtask", LinkPredicate: "mission.child.subtask"},
		{Workflow: "fixture", LinkPredicate: "mission.child.submission"},
		{Workflow: "unregistered", LinkPredicate: "mission.child.orphan"},
	}
	require.NoError(t, mgr.Register(parent))
	require.NoError(t, mgr.Register(subtaskWorkflow()))

	parentID := missionPrefix + "parent"
	require.NoError(t, mgr.Create(ctx, &fixtureMission{ID: parentID, PhaseF: "planning"}))
	require.NoError(t, mgr.Create(ctx, &fixtureMission{ID: missionPrefix + "sub", PhaseF: "flying"}))
	for _, s := range []*subtask{
		{ID: subtaskPrefix + "b", PhaseF: "running", Owner: "bo"},
		{ID: subtaskPrefix + "a", PhaseF: "queued", Owner: "ana"},
		{ID: subtaskPrefix + "unlinked", PhaseF: "queued"},
	} {
		require.NoError(t, mgr.Create(ctx, s))
	}
	link := func(predicate string, object any) message.Triple {
		return message.Triple{
			Subject: parentID, Predicate: predicate, Object: object, Source: fixtureSource, Timestamp: fixtureTime,
		}
	}
	appendTriples(t, kv, parentID,
		link("mission.child.subtask", subtaskPrefix+"b"),
		link("mission.child.subtask", subtaskPrefix+"gone"),
		link("mission.child.subtask", subtaskPrefix+"a"),
		link("mission.child.subtask", 42),
		link("mission.child.submission", missionPrefix+"sub"),
		link("some.other.predicate", subtaskPrefix+"unlinked"),
	)

	type child struct{ workflow, id, phase string }
	view := func(t *testing.T, got []ChildResult) []child {
		t.Helper()
		out := make([]child, 0, len(got))
		for _, r := range got {
			out = append(out, child{r.Workflow, r.State.EntityID(), r.State.Phase()})
		}
		return out
	}
	subA := child{"subtask", subtaskPrefix + "a", "queued"}
	subB := child{"subtask", subtaskPrefix + "b", "running"}
	sub := child{"fixture", missionPrefix + "sub", "flying"}

	got, err := mgr.Children(ctx, parentID, ChildOptions{})
	require.NoError(t, err)
	require.Equal(t, []child{sub, subA, subB}, view(t, got))
	require.Equal(t, &subtask{ID: subtaskPrefix + "a", PhaseF: "queued", Owner: "ana"}, got[1].State)

	for _, tt := range []struct {
		name string
		opts ChildOptions
		want []child
	}{
		{"one child workflow", ChildOptions{Workflow: "subtask"}, []child{subA, subB}},
		{"the parent's own workflow", ChildOptions{Workflow: "fixture"}, []child{sub}},
		{"a page of one", ChildOptions{Workflow: "subtask", Offset: 1, Limit: 1}, []child{subB}},
		{"a page past the gone link", ChildOptions{Workflow: "subtask", Limit: 2}, []child{subA, subB}},
		{"a page past the links", ChildOptions{Workflow: "subtask", Offset: 10}, []child{}},
		{"a workflow with no ChildSpec", ChildOptions{Workflow: "other"}, []child{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mgr.Children(ctx, parentID, tt.opts)
			require.NoError(t, err)
			require.Equal(t, tt.want, view(t, got))
		})
	}

	t.Run("a child workflow declares no children", func(t *testing.T) {
		got, err := mgr.Children(ctx, subtaskPrefix+"a", ChildOptions{})
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("an unknown parent is not found", func(t *testing.T) {
		_, err := mgr.Children(ctx, missionPrefix+"absent", ChildOptions{})
		require.ErrorIs(t, err, ErrEntityNotFound)
	})

	t.Run("an entity outside every registered pattern is refused", func(t *testing.T) {
		const outside = "c360.platform1.gcs.lifecycle.other.x"
		putRawEntity(t, kv, outside)
		got, err := mgr.Children(ctx, outside, ChildOptions{})
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrEntityNotFound)
		require.Nil(t, got)
	})

	t.Run("a link into an unregistered workflow fails the call", func(t *testing.T) {
		appendTriples(t, kv, parentID, link("mission.child.orphan", subtaskPrefix+"orphan"))
		got, err := mgr.Children(ctx, parentID, ChildOptions{})
		require.ErrorIs(t, err, ErrWorkflowNotRegistered)
		require.Nil(t, got)
	})
}

// lastEvent returns the most recent transition History records for an entity.
func lastEvent(t *testing.T, mgr *Manager, workflow, entityID string) TransitionEvent {
	t.Helper()
	events, err := mgr.History(t.Context(), workflow, entityID)
	require.NoError(t, err)
	require.NotEmpty(t, events)
	return events[len(events)-1]
}

// TestIntegration_CompleteMovesToTheReachableTerminal proves Complete moves an
// entity to a terminal phase one transition away, recorded as a framework
// transition: the only one when one is reachable, and the first in the current
// phase's declared out-edges when several are, with a Warn naming the choice. With no terminal reachable it is
// ErrInvalidTransition; an entity already terminal is ErrTerminalPhase; an
// absent entity is ErrEntityNotFound; one with no phase triple is
// ErrEntityNotLifecycleManaged; an unregistered workflow is
// ErrWorkflowNotRegistered. A refusal leaves the stored entity at its revision.
func TestIntegration_CompleteMovesToTheReachableTerminal(t *testing.T) {
	client := fixtureClient(t)
	ctx := t.Context()
	kv := startKVBackedResponders(t, client)
	var logs bytes.Buffer
	mgr := NewManager(client, slog.New(slog.NewJSONHandler(&logs, nil)))
	require.NoError(t, mgr.Register(subtaskWorkflow()))
	for _, s := range []*subtask{
		{ID: subtaskPrefix + "running", PhaseF: "running"},
		{ID: subtaskPrefix + "paused", PhaseF: "paused"},
		{ID: subtaskPrefix + "queued", PhaseF: "queued"},
		{ID: subtaskPrefix + "done", PhaseF: "done"},
	} {
		require.NoError(t, mgr.Create(ctx, s))
	}
	putRawEntity(t, kv, subtaskPrefix+"raw")

	t.Run("two terminals reachable: the first declared, with a Warn", func(t *testing.T) {
		id := subtaskPrefix + "running"
		require.NoError(t, mgr.Complete(ctx, "subtask", id))
		require.Equal(t, "done", storedObject(t, kv, id, "subtask.lifecycle.phase"))
		event := lastEvent(t, mgr, "subtask", id)
		require.Equal(t, "running", event.From)
		require.Equal(t, "done", event.To)
		require.Equal(t, TransitionSourceFramework, event.Triggered)
		require.Contains(t, logs.String(), `"msg":"lifecycle: terminal selection is ambiguous`)
		require.Contains(t, logs.String(), `"entity_id":"`+id+`","from":"running","picked":"done","alternatives":["failed"]`)
	})

	t.Run("one terminal reachable", func(t *testing.T) {
		id := subtaskPrefix + "paused"
		require.NoError(t, mgr.Complete(ctx, "subtask", id))
		require.Equal(t, "cancelled", storedObject(t, kv, id, "subtask.lifecycle.phase"))
		event := lastEvent(t, mgr, "subtask", id)
		require.Equal(t, "paused", event.From)
		require.Equal(t, "cancelled", event.To)
		require.Equal(t, TransitionSourceFramework, event.Triggered)
		require.NotContains(t, logs.String(), `"entity_id":"`+id+`","from":"paused"`)
	})

	for _, tt := range []struct {
		name, workflow, id string
		want               error
	}{
		{"no terminal reachable", "subtask", subtaskPrefix + "queued", ErrInvalidTransition},
		{"already terminal", "subtask", subtaskPrefix + "done", ErrTerminalPhase},
		{"no phase triple", "subtask", subtaskPrefix + "raw", ErrEntityNotLifecycleManaged},
	} {
		t.Run(tt.name, func(t *testing.T) {
			before := revisionOf(t, kv, tt.id)
			require.ErrorIs(t, mgr.Complete(ctx, tt.workflow, tt.id), tt.want)
			require.Equal(t, before, revisionOf(t, kv, tt.id))
		})
	}
	t.Run("an absent entity", func(t *testing.T) {
		require.ErrorIs(t, mgr.Complete(ctx, "subtask", subtaskPrefix+"absent"), ErrEntityNotFound)
	})
	t.Run("an unregistered workflow", func(t *testing.T) {
		require.ErrorIs(t, mgr.Complete(ctx, "unregistered", subtaskPrefix+"queued"), ErrWorkflowNotRegistered)
	})
}

// TestIntegration_FailMovesToFailedWithTheReason proves Fail moves an entity
// to the workflow's "failed" phase as a framework transition whose note is the
// reason, in History and in the workflow's audit note predicate. An empty
// reason, a phase with no edge to "failed", and a workflow that declares no
// "failed" phase are refused and leave the stored entity at its revision; an
// unregistered workflow is ErrWorkflowNotRegistered.
func TestIntegration_FailMovesToFailedWithTheReason(t *testing.T) {
	client := fixtureClient(t)
	ctx := t.Context()
	kv := startKVBackedResponders(t, client)
	mgr := NewManager(client, nil)
	require.NoError(t, mgr.Register(subtaskWorkflow()))
	noFailed := lifecycle{}.fixtureWorkflow()
	delete(noFailed.Transitions, "failed")
	require.NoError(t, mgr.Register(noFailed))
	for _, s := range []*subtask{
		{ID: subtaskPrefix + "running", PhaseF: "running"},
		{ID: subtaskPrefix + "queued", PhaseF: "queued"},
	} {
		require.NoError(t, mgr.Create(ctx, s))
	}
	missionID := missionPrefix + "m1"
	require.NoError(t, mgr.Create(ctx, &fixtureMission{ID: missionID, PhaseF: "flying"}))

	t.Run("an empty reason is refused", func(t *testing.T) {
		id := subtaskPrefix + "running"
		before := revisionOf(t, kv, id)
		require.Error(t, mgr.Fail(ctx, "subtask", id, ""))
		require.Equal(t, before, revisionOf(t, kv, id))
	})

	t.Run("fails with the reason recorded", func(t *testing.T) {
		id := subtaskPrefix + "running"
		require.NoError(t, mgr.Fail(ctx, "subtask", id, "sensor offline"))
		require.Equal(t, "failed", storedObject(t, kv, id, "subtask.lifecycle.phase"))
		require.Equal(t, "sensor offline", storedObject(t, kv, id, "subtask.transition.note"))
		event := lastEvent(t, mgr, "subtask", id)
		require.Equal(t, "running", event.From)
		require.Equal(t, "failed", event.To)
		require.Equal(t, TransitionSourceFramework, event.Triggered)
		require.Equal(t, "sensor offline", event.Note)
	})

	for _, tt := range []struct {
		name, workflow, id string
	}{
		{"no edge to failed", "subtask", subtaskPrefix + "queued"},
		{"no failed phase declared", "fixture", missionID},
	} {
		t.Run(tt.name, func(t *testing.T) {
			before := revisionOf(t, kv, tt.id)
			require.ErrorIs(t, mgr.Fail(ctx, tt.workflow, tt.id, "sensor offline"), ErrInvalidTransition)
			require.Equal(t, before, revisionOf(t, kv, tt.id))
		})
	}
	t.Run("an unregistered workflow", func(t *testing.T) {
		require.ErrorIs(t, mgr.Fail(ctx, "unregistered", subtaskPrefix+"queued", "x"), ErrWorkflowNotRegistered)
	})
}
