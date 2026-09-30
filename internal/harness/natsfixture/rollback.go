package natsfixture

import (
	"context"
	"errors"
	"time"
)

// rollbackBudget bounds each cleanup operation after a failed Start attempt. It is a failure
// ceiling: Docker removes a container in well under a second when healthy, and SemStreams measured
// deletion above five seconds only under concurrent package teardown.
const rollbackBudget = 15 * time.Second

// rollback runs one cleanup operation for a failed Start under a fresh bounded context that keeps
// the parent's values but not its cancellation, because the parent may be exactly what failed.
// Adapted from SemStreams internal/lifecyclecleanup/lifecyclecleanup.go at 5457b345 (ledger row
// L3): synchronous, bounded, and the bound's own expiry is reported alongside the operation's error.
func rollback(parent context.Context, op func(context.Context) error) error {
	if parent == nil {
		return errors.New("natsfixture: rollback with a nil parent context")
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), rollbackBudget)
	defer cancel()
	return errors.Join(op(ctx), ctx.Err())
}
