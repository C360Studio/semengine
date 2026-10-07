package dispatch

import "errors"

// Package error sentinels. Callers compare with errors.Is.
var (
	// ErrInvalidConfig is returned by NewKeyedPool for a nil context or
	// a KeyedConfig that is incomplete or out of range (Lanes <= 0,
	// QueueDepth <= 0, a missing KeyOf or Process). Catches wiring bugs
	// at construction time.
	ErrInvalidConfig = errors.New("dispatch: invalid config")

	// ErrStopped is returned by KeyedPool.SubmitBlocking once Shutdown
	// has begun.
	ErrStopped = errors.New("dispatch: pool stopped")
)
