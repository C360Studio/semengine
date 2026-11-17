package main

import (
	"context"
	"errors"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/pkg/errs"
)

// readyBudget bounds each wait for an index to catch up with records it already has: the word
// indexes with the last write, the semantic verb's copy with the vectors seen generated. It is a
// guard against a wait that will never end, far above the seconds catching up takes.
const readyBudget = 5 * time.Minute

// awaitAnswer asks until done accepts an answer. An index's refusal that it has not caught up
// (index_not_ready, revision_not_reached) and an answer done does not accept are asked again, for
// at most budget, after which the error names the last answer and refusal. Any other error, such
// as the provider failing behind the semantic verb, ends the wait at once and is returned.
func awaitAnswer[T any](ctx context.Context, budget time.Duration, ask func(context.Context) (T, error), done func(T) bool) (T, error) {
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	var failed error
	got, err := probe.Await(ctx, func(ctx context.Context) (T, error) {
		got, err := ask(ctx)
		if err != nil && !catchingUp(err) {
			failed = err
			cancel()
		}
		return got, err
	}, done)
	if failed != nil {
		return got, failed
	}
	return got, err
}

func catchingUp(err error) bool {
	var refusal *errs.ClassifiedError
	if !errors.As(err, &refusal) {
		return false
	}
	return refusal.Code == graph.ErrorCodeIndexNotReady || refusal.Code == graph.ErrorCodeRevisionNotReached
}
