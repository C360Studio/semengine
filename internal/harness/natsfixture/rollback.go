package natsfixture

import (
	"context"
	"errors"
	"time"
)

// rollbackBudget bounds each cleanup operation after a failed Start attempt. It is a failure
// ceiling: Docker removes a container in well under a second when healthy, and deletion above five
// seconds was measured only under concurrent package teardown; Docker promises neither.
const rollbackBudget = 15 * time.Second

// rollback runs one cleanup operation for a failed Start under a fresh bounded context that keeps
// the parent's values but not its cancellation, because the parent may be exactly what failed. It
// is synchronous and bounded, and the bound's own expiry is reported alongside the operation's
// error.
func rollback(parent context.Context, op func(context.Context) error) error {
	if parent == nil {
		return errors.New("natsfixture: rollback with a nil parent context")
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), rollbackBudget)
	defer cancel()
	return errors.Join(op(ctx), ctx.Err())
}
