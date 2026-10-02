package probe

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// pollInterval is how often Await re-observes. It is fixed rather than a parameter: it bounds how
// quickly a met condition is noticed, never whether it is, and callers bound the whole wait with
// their context.
const pollInterval = 20 * time.Millisecond

// Await polls observe under the caller's context until done accepts an observation, and returns
// that observation. When the context ends first it returns the last observation and an error that
// wraps the context's error and that observation's own error, if it had one, and names the last
// value, so a failed wait says what was seen rather than only that time ran out. An error from an
// earlier observation is not reported once a later one returns without error. An observation error
// does not end the wait: the thing observed may not exist yet.
func Await[T any](ctx context.Context, observe func(context.Context) (T, error), done func(T) bool) (T, error) {
	var last T
	if ctx == nil {
		return last, errors.New("probe: Await with a nil context")
	}
	started := time.Now()
	var lastErr error
	for attempts := 1; ; attempts++ {
		value, err := observe(ctx)
		last, lastErr = value, err
		if err == nil && done(value) {
			return value, nil
		}
		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			if lastErr != nil {
				return last, fmt.Errorf("probe: condition not met after %d observation(s) in %s: last observation %+v: last error: %w: %w",
					attempts, time.Since(started).Round(time.Millisecond), last, lastErr, ctx.Err())
			}
			return last, fmt.Errorf("probe: condition not met after %d observation(s) in %s: last observation %+v (no error): %w",
				attempts, time.Since(started).Round(time.Millisecond), last, ctx.Err())
		case <-timer.C:
		}
	}
}
