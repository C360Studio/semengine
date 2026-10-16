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
//   - Stop is controlled while the Start context is still live: it drains,
//     joins and cleans up within its own context's bound. Ending the Start
//     context first makes Stop an abort: it makes what progress it can within
//     that bound and may return the cleanup or deadline error; if the bound
//     ends first, no complete join is promised.
//   - A repeated Stop is a no-op once an earlier Stop's cleanup succeeded;
//     after a failed cleanup, what is left stays pending for a later Stop.
//   - A second Start is refused. An instance never restarts; a host that needs
//     the component again creates a new one.
//   - A failed Start holds nothing, or, when its own cleanup also fails, it
//     returns both errors and a later Stop releases what it left.
//   - Configuration is validated once, at construction, and Initialize is
//     idempotent and does no I/O.
//
// Concurrent lifecycle calls are not part of the contract. graph-ingest breaks
// the last rule today: it validates its configuration in its factory and again
// in Initialize, and the second check leaves with simplification wave PR 10 or
// the processor shell (#109).
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

	// Config is the effective component configuration this instance was
	// created from. The ComponentManager retains it so a per-component runtime
	// config update can restart the component only when the effective config
	// actually changed — a no-op update is skipped instead of stop/start-cycling
	// a healthy running component (gh#520).
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
