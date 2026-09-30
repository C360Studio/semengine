package natsfixture

import (
	"errors"
	"fmt"
	"strings"
)

// ErrNotAdmitted is returned by Start when the test was not launched by the integration runner:
// Docker work on the shared daemon is admitted only through the host lock it holds.
var ErrNotAdmitted = errors.New("natsfixture: not admitted to Docker; run it through `task test:integration -- <package>`")

// ErrAlreadyUsed is returned by a second Start. A fixture is one container's lifetime; restart is
// not promised.
var ErrAlreadyUsed = errors.New("natsfixture: fixture already started; restart is not promised")

// Phase names one step of Start, in order, or a resource operation after it.
type Phase string

// Start runs these phases in this order; CreateStream and CreateKeyValue report their own.
const (
	PhaseImage        Phase = "image"
	PhaseStart        Phase = "start" // create and start the container, then wait for its ready log
	PhaseHost         Phase = "host"
	PhaseMappedPort   Phase = "mapped-port"
	PhaseConnect      Phase = "connect"
	PhaseJetStream    Phase = "jetstream-ready"
	PhaseCreateStream Phase = "create-stream"
	PhaseCreateKV     Phase = "create-kv"
	PhaseConsume      Phase = "consume"
)

// Error is a failed Start attempt or resource operation, carrying what a reader needs to tell a
// slow daemon from a dead container from a cancelled test, and whether cleanup completed.
type Error struct {
	Attempt     int
	Phase       Phase
	ContainerID string // empty when no container existed yet
	// ParentErr is the caller's context error at the moment of failure; nil means it was live.
	ParentErr error
	Cause     error
	// Cleanup is the outcome of rolling back what the attempt created; nil means every resource
	// was observed absent.
	Cleanup error
}

func (e *Error) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "natsfixture: attempt %d phase %s", e.Attempt, e.Phase)
	if e.ContainerID != "" {
		fmt.Fprintf(&b, " container %s", shortID(e.ContainerID))
	}
	if e.ParentErr == nil {
		b.WriteString(" (parent context live)")
	} else {
		fmt.Fprintf(&b, " (parent context %v)", e.ParentErr)
	}
	fmt.Fprintf(&b, ": %v", e.Cause)
	if e.Cleanup != nil {
		fmt.Fprintf(&b, " (cleanup: %v)", e.Cleanup)
	}
	return b.String()
}

// Unwrap exposes the cause and the cleanup error to errors.Is and errors.As.
func (e *Error) Unwrap() []error {
	var errs []error
	if e.Cause != nil {
		errs = append(errs, e.Cause)
	}
	if e.Cleanup != nil {
		errs = append(errs, e.Cleanup)
	}
	return errs
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
