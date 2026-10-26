package component

import (
	"context"

	"github.com/c360studio/semengine/types"
)

// State represents the current lifecycle state of a component
type State int

const (
	// StateCreated indicates component was created but not initialized
	StateCreated State = iota
	// StateInitialized indicates component was initialized but not started
	StateInitialized
	// StateStarted indicates component is running
	StateStarted
	// StateStopped indicates component was stopped
	StateStopped
	// StateFailed indicates component failed during lifecycle operation
	StateFailed
)

// String returns a string representation of the component state
func (cs State) String() string {
	switch cs {
	case StateCreated:
		return "created"
	case StateInitialized:
		return "initialized"
	case StateStarted:
		return "started"
	case StateStopped:
		return "stopped"
	case StateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// LifecycleComponent is a component its host initializes, starts and stops.
// A caller can rely on these outcomes (ADR-108, Decision 3):
//
//   - Start and Stop refuse a nil context, and Start refuses a context that has
//     already ended.
//   - Stop before Start is safe.
//   - Start(ctx) does its startup work under ctx and returns once the
//     component is ready; if ctx ends first, Start returns that error and
//     holds nothing. Ending ctx after Start returned stops nothing: the
//     component runs its continuing work under a context of its own, which
//     Stop ends (ADR-113).
//   - Stop(ctx) is the only stop. It drains, joins and cleans up within ctx's
//     bound and returns nil only once everything is released; if ctx ends
//     first, it returns that context's error and a later Stop tries again.
//   - A repeated Stop is a no-op once an earlier Stop's cleanup succeeded;
//     after a failed cleanup, what is left stays pending for a later Stop.
//   - A second Start is refused. An instance never restarts; a host that needs
//     the component again creates a new one.
//   - A failed Start holds nothing, or, when its own cleanup also fails, it
//     returns both errors and a later Stop releases what it left.
//   - Configuration is validated once, at construction, and Initialize is
//     idempotent and does no I/O.
//
// Concurrent lifecycle calls are not part of the contract. Reads are: Health,
// DataFlow and DebugStatus may be called at any time, during Start or Stop and
// after Stop, and return without waiting for either; after Stop they report
// what the component knows then. Callers do so without waiting for Stop: the
// component manager's health and status routes, its health publisher (which
// ends only after the components have stopped) and the service manager's
// /readyz. The rule is tested for graph-ingest
// (TestGraphIngestHealthAndDataFlowAnswerDuringAndAfterStop) and is review
// only for components outside this module.
//
// component.Lifecycle implements these rules for a component that holds one:
// nil and ended contexts refused, Stop before Start safe, one Start and no
// restart, a repeated Stop a no-op, a failed Start rolled back and a failed
// cleanup kept for the next Stop, Start refused before Initialize, Initialize
// idempotent with no I/O, and Health and DataFlow answering at any time without
// waiting. The component keeps the rest: its start creates the run context for
// its continuing work and its cleanup ends it, drains and joins that work within
// Stop's context and tolerates a partial start; it validates its configuration
// at construction; and its DebugStatus, if it has one, must not wait for Start
// or Stop.
type LifecycleComponent interface {
	Discoverable
	Initialize() error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// ManagedComponent tracks a component and its lifecycle state
// This is used by ComponentManager to properly manage component lifecycle
type ManagedComponent struct {
	// Component is the actual component instance
	Component Discoverable

	// State tracks the current lifecycle state
	State State

	// Config is the boot configuration this instance was created from. The
	// component set and each component's configuration are fixed at
	// construction, so it never changes for the life of the instance.
	Config types.ComponentConfig

	// StartOrder tracks the order components were started for reverse shutdown
	StartOrder int

	// LastError tracks the last error that occurred during lifecycle operations
	LastError error
}

// AsLifecycleComponent safely casts a component to LifecycleComponent
func AsLifecycleComponent(comp Discoverable) (LifecycleComponent, bool) {
	lc, ok := comp.(LifecycleComponent)
	return lc, ok
}
