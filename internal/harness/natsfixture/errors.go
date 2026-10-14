package natsfixture

import (
	"errors"
	"fmt"
	"strings"
)

// ErrAlreadyUsed is returned by a second Start. A fixture is one container's lifetime: Restart
// stops and starts that same container, and a second Start never creates another.
var ErrAlreadyUsed = errors.New("natsfixture: fixture already started; use Restart to restart its container")

// Phase names one step of Start, in order, or a resource operation after it.
type Phase string

// Start runs these phases in this order; CreateStream and CreateMemoryStream report PhaseCreateStream.
// Restart runs PhaseStopContainer and PhaseStartContainer, then PhaseMappedPort, PhaseConnect and
// PhaseJetStream again.
const (
	PhaseImage          Phase = "image"
	PhaseStart          Phase = "start" // create and start the container, then wait for its ready log
	PhaseHost           Phase = "host"
	PhaseMappedPort     Phase = "mapped-port"
	PhaseConnect        Phase = "connect"
	PhaseJetStream      Phase = "jetstream-ready"
	PhaseCreateStream   Phase = "create-stream"
	PhaseStopContainer  Phase = "stop-container"
	PhaseStartContainer Phase = "start-container" // start the same container, then wait for a new ready log
)

// Error is a failed Start attempt or resource operation, carrying what a reader needs to tell a
// slow daemon from a dead container from a cancelled test, and whether cleanup completed.
type Error struct {
	Phase       Phase
	ContainerID string // empty when no container existed yet
	// ParentErr is the caller's context error at the moment of failure; nil means it was live.
	ParentErr error
	Cause     error
	// Cleanup is the outcome of rolling back what the attempt created; nil means Docker reported
	// the container removed.
	Cleanup error
}

func (e *Error) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "natsfixture: phase %s", e.Phase)
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

// Unwrap exposes the cause, the parent context's error, and the cleanup error to errors.Is and
// errors.As. The parent's error is included because a dependency interrupted by cancellation often
// reports something else (an aborted request, a closed socket), and a caller asking
// errors.Is(err, context.Canceled) must still learn that it cancelled the Start.
func (e *Error) Unwrap() []error {
	var errs []error
	if e.Cause != nil {
		errs = append(errs, e.Cause)
	}
	if e.ParentErr != nil {
		errs = append(errs, e.ParentErr)
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
