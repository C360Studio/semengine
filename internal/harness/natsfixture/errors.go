package natsfixture

import (
	"errors"
	"fmt"
	"strings"
)

// ErrAlreadyUsed is returned by a second Start. A fixture is one container's lifetime: Restart
// stops and starts that same container, and a second Start never creates another.
var ErrAlreadyUsed = errors.New("natsfixture: fixture already started; use Restart to restart its container")

// phaseError is a failed phase of Start or Restart, or a failed resource operation. Its text
// carries what a reader needs to tell a slow daemon from a dead container from a cancelled test,
// and whether cleanup completed.
//
// Start's phases, in order: image, start (create and start the container, then wait for its ready
// log), host, mapped-port, connect, jetstream-ready. Restart's: stop-container, start-container
// (start the same container, then wait for a new ready log), mapped-port, connect,
// jetstream-ready. CreateStream and CreateMemoryStream report create-stream.
type phaseError struct {
	phase       string
	containerID string // empty when no container existed yet
	// parentErr is the caller's context error at the moment of failure; nil means it was live.
	parentErr error
	cause     error
	// cleanup is the outcome of rolling back what a failed Start created; nil means Docker
	// reported the container removed.
	cleanup error
}

func (e *phaseError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "natsfixture: phase %s", e.phase)
	if e.containerID != "" {
		fmt.Fprintf(&b, " container %s", shortID(e.containerID))
	}
	if e.parentErr == nil {
		b.WriteString(" (parent context live)")
	} else {
		fmt.Fprintf(&b, " (parent context %v)", e.parentErr)
	}
	fmt.Fprintf(&b, ": %v", e.cause)
	if e.cleanup != nil {
		fmt.Fprintf(&b, " (cleanup: %v)", e.cleanup)
	}
	return b.String()
}

// Unwrap exposes the cause, the parent context's error, and the cleanup error to errors.Is. The
// parent's error is included because a call interrupted by cancellation often reports something
// else (an aborted request, a closed socket), and a caller asking errors.Is(err, context.Canceled)
// must still learn that it cancelled the Start.
func (e *phaseError) Unwrap() []error {
	var errs []error
	if e.cause != nil {
		errs = append(errs, e.cause)
	}
	if e.parentErr != nil {
		errs = append(errs, e.parentErr)
	}
	if e.cleanup != nil {
		errs = append(errs, e.cleanup)
	}
	return errs
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
