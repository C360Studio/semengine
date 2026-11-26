package rule

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"

	"github.com/c360studio/semengine/pkg/lifecycle"
	"github.com/c360studio/semengine/processor/rule/expression"
)

// $entity.lifecycle.* reads the lifecycle state of the context's entity from
// its lifecycle manager:
//
//   - $entity.lifecycle.phase: the participant's phase.
//   - $entity.lifecycle.terminal: "true" or "false".
//   - $entity.lifecycle.workflow: the workflow's name.
//   - $entity.lifecycle.workflow_def: the workflow's definition as JSON.
//
// The four are fixed names, so $entity.lifecycle.workflow_def is never read as
// $entity.lifecycle.workflow. With no manager, no entity ID, an entity the
// manager does not know, or a definition it cannot give, the token stays and
// is reported.

const lifecyclePrefix = "$entity.lifecycle."

// LifecycleManager is what substitution asks a lifecycle manager:
// *lifecycle.Manager implements it. An interface, so a context can be built
// without a running manager.
type LifecycleManager interface {
	// LookupByEntityID returns the participant entityID is, under the first
	// workflow whose entity pattern it matches.
	LookupByEntityID(ctx context.Context, entityID string) (lifecycle.Participant, error)
	// GetWorkflowDefinition returns the definition of a registered workflow.
	GetWorkflowDefinition(workflow string) (lifecycle.WorkflowDef, error)
}

var _ LifecycleManager = (*lifecycle.Manager)(nil)

// lifecycleLookupResult is the outcome of the one lookup of a context's entity.
type lifecycleLookupResult struct {
	participant lifecycle.Participant
	err         error
}

// resolveLifecycle reads the $entity.lifecycle.* token at the start of s.
func (ec *ExecutionContext) resolveLifecycle(ctx context.Context, s string) (string, int, bool) {
	names := []string{"phase", "terminal", "workflow", "workflow_def"}
	var name string
	for _, candidate := range names {
		if fixedName(s, lifecyclePrefix+candidate) {
			name = candidate
			break
		}
	}
	if name == "" || ec.Lifecycle == nil || ec.EntityID == "" {
		return "", 0, false
	}
	p, err := ec.lifecycleParticipant(ctx)
	if err != nil {
		return "", 0, false
	}
	n := len(lifecyclePrefix) + len(name)
	switch name {
	case "phase":
		return p.Phase(), n, true
	case "terminal":
		return strconv.FormatBool(p.IsTerminal()), n, true
	case "workflow":
		return p.Workflow(), n, true
	}
	def, ok := ec.workflowDefinitionJSON(ctx, p.Workflow())
	return def, n, ok
}

// lifecycleParticipant looks the context's entity up once; later calls on the
// same context return the kept participant or error.
func (ec *ExecutionContext) lifecycleParticipant(ctx context.Context) (lifecycle.Participant, error) {
	if ec.lifecycleLookup == nil {
		p, err := ec.Lifecycle.LookupByEntityID(ctx, ec.EntityID)
		ec.lifecycleLookup = &lifecycleLookupResult{participant: p, err: err}
	}
	return ec.lifecycleLookup.participant, ec.lifecycleLookup.err
}

// workflowDefinitionJSON returns the named workflow's definition as JSON. A
// failure is logged with its error, and the token then stays and is reported.
func (ec *ExecutionContext) workflowDefinitionJSON(ctx context.Context, workflow string) (string, bool) {
	def, err := ec.Lifecycle.GetWorkflowDefinition(workflow)
	if err != nil {
		slog.Default().WarnContext(ctx, "lifecycle workflow definition lookup failed",
			slog.String("workflow", workflow), slog.String("error", err.Error()))
		return "", false
	}
	encoded, err := json.Marshal(def)
	if err != nil {
		slog.Default().WarnContext(ctx, "lifecycle workflow definition does not encode as JSON",
			slog.String("workflow", workflow), slog.String("error", err.Error()))
		return "", false
	}
	return string(encoded), true
}

// populateLifecycleStateFields writes the three $entity.lifecycle.* condition
// fields (phase, terminal, workflow) for entityID into fields, from manager.
// A nil manager, an empty entity ID or nil fields writes nothing and returns
// nil: the caller supplied no lifecycle state. A failed lookup writes nothing
// and returns its error, since "a manager was supplied" and "lifecycle state
// resolved" are different facts: a caller that answers without the state must
// know it is missing.
func populateLifecycleStateFields(ctx context.Context, manager LifecycleManager, entityID string, fields expression.StateFields) error {
	if manager == nil || entityID == "" || fields == nil {
		return nil
	}
	p, err := manager.LookupByEntityID(ctx, entityID)
	if err != nil {
		return err
	}
	fields[lifecyclePrefix+"phase"] = p.Phase()
	fields[lifecyclePrefix+"terminal"] = p.IsTerminal()
	fields[lifecyclePrefix+"workflow"] = p.Workflow()
	return nil
}
