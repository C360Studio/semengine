//go:build integration

package lifecycle

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/metric"
)

// TestTransitionReplacesItsPhaseStatement drives a create and two transitions, under different
// TransitionSources, through a real graph-ingest, and reads the entity back from ENTITY_STATES:
// it holds one phase statement, the last phase, and that statement carries the manager's own
// source, semengine-lifecycle, not the TransitionSource of either transition (design D15). The
// TransitionSource is the audit field's value, not the statement's source.
func TestTransitionReplacesItsPhaseStatement(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	client := ingestClient(t)
	stop := startGraphIngest(ctx, t, client, metric.NewMetricsRegistry())
	defer stop()

	mgr := NewManager(client, nil)
	workflow := gateWorkflow()
	require.NoError(t, mgr.Register(workflow))
	const id = "c360.platform1.lifecycle.gcs.mission.phase-001"
	require.NoError(t, mgr.Create(ctx, &gateMission{ID: id, PhaseF: "planning"}))
	require.NoError(t, mgr.Transition(ctx, workflow.Name, id, "flying", TransitionSourceRule, "rule fired"))
	require.NoError(t, mgr.Transition(ctx, workflow.Name, id, "completed", TransitionSourceOperator, "operator closed"))

	js, err := client.JetStream()
	require.NoError(t, err)
	kv, err := js.KeyValue(ctx, graph.BucketEntityStates)
	require.NoError(t, err)
	entry, err := kv.Get(ctx, id)
	require.NoError(t, err)
	var stored graph.EntityState
	require.NoError(t, json.Unmarshal(entry.Value(), &stored))

	var phases []message.Triple
	for _, statement := range stored.Triples {
		if statement.Predicate == workflow.PhasePredicate {
			phases = append(phases, statement)
		}
	}
	require.Len(t, phases, 1, "the entity must hold one %s statement after two transitions: %+v",
		workflow.PhasePredicate, phases)
	require.Equal(t, "completed", phases[0].Object)
	require.Equal(t, "semengine-lifecycle", phases[0].Source,
		"the phase statement carries the manager's source, not the transition's")
}
