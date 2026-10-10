// Package lifecyclecleanup releases what a component's failed Start acquired, under a bounded
// context that the failure cannot cancel.
//
// A component whose Start can fail after acquiring something (a subscription, a goroutine, a
// connection) cleans up its own failed start with RollbackFailedStart. The component manager and
// the service manager still call Stop after a failed Start, as a second line. A component that
// calls the helper does five things:
//
//  1. Before its first acquisition, it records that cleanup is pending.
//  2. On a failure after that, it calls RollbackFailedStart synchronously, before Start returns,
//     with Start's own context and its cleanup function.
//  3. It joins the helper's result into Start's error, so the caller sees both the start failure
//     and any cleanup failure.
//  4. It clears the cleanup-pending record only when the helper returns nil. Otherwise it keeps
//     the record and the handles it could not release.
//  5. When Stop is called after a failed Start, it releases whatever is still held and returns an
//     error naming what it could not release.
//
// SemEngine's own components get these from internal/lifecycleguard. SemEngine can test the helper
// and its managers; only a component's own tests can show that the component follows the five.
package lifecyclecleanup

import (
	"context"
	"errors"
	"time"
)

// failedStartRollbackTimeout is the budget of each rollback, fresh per call.
const failedStartRollbackTimeout = 5 * time.Second

// RollbackFailedStart runs rollback synchronously under a context that keeps parent's values but
// not its cancellation or deadline, with a deadline five seconds after the call. It returns
// rollback's error joined with that context's error, so nil means the rollback returned nil within
// the budget. A nil parent or a nil rollback is refused with an error, and nothing runs.
func RollbackFailedStart(parent context.Context, rollback func(context.Context) error) error {
	return rollbackFailedStart(parent, failedStartRollbackTimeout, rollback)
}

func rollbackFailedStart(
	parent context.Context,
	budget time.Duration,
	rollback func(context.Context) error,
) error {
	if parent == nil {
		return errors.New("lifecyclecleanup: nil failed-Start parent")
	}
	if rollback == nil {
		return errors.New("lifecyclecleanup: nil rollback; no cleanup ran for the failed Start")
	}

	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), budget)
	defer cancel()

	rollbackErr := rollback(ctx)
	return errors.Join(rollbackErr, ctx.Err())
}
