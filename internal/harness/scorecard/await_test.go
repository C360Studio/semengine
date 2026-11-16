package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/pkg/errs"
)

var (
	notReady = errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeIndexNotReady, errors.New("index not ready"))
	behind   = errs.ClassifiedCode(errs.ErrorInvalid, graph.ErrorCodeRevisionNotReached, errors.New("revision not reached"))
	internal = errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeInternal, errors.New("embed the query: 503"))
)

// An index that refuses as not ready, or answers without what is awaited, is asked again until it
// answers with it.
func TestAwaitAnswerRetriesUntilReady(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		replies := []error{notReady, behind, nil, nil}
		asked := 0
		got, err := awaitAnswer(t.Context(), time.Minute, func(context.Context) (int, error) {
			err := replies[asked]
			asked++
			return asked, err
		}, func(n int) bool { return n == 4 })
		if err != nil || got != 4 || asked != 4 {
			t.Errorf("awaitAnswer = %d, %v after %d asks; want 4, nil after 4", got, err, asked)
		}
	})
}

// Any other error, such as a provider failure behind the semantic verb, ends the wait at once and
// is returned.
func TestAwaitAnswerStopsOnOtherErrors(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		for _, failure := range []error{internal, errors.New("connection closed")} {
			asked := 0
			_, err := awaitAnswer(t.Context(), time.Minute, func(context.Context) (int, error) {
				asked++
				return 0, failure
			}, func(int) bool { return true })
			if !errors.Is(err, failure) || asked != 1 {
				t.Errorf("awaitAnswer on %v = %v after %d asks; want that error after 1", failure, err, asked)
			}
		}
	})
}

// A wait that never sees what it awaits ends when its budget is spent, naming the last answer.
func TestAwaitAnswerEndsAtItsBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		_, err := awaitAnswer(t.Context(), time.Minute, func(context.Context) (int, error) {
			return 7, notReady
		}, func(int) bool { return false })
		if err == nil || !strings.Contains(err.Error(), "7") || !strings.Contains(err.Error(), "index not ready") {
			t.Errorf("awaitAnswer = %v, want an error naming the last answer 7 and its refusal", err)
		}
		if waited := time.Since(start); waited < time.Minute || waited > time.Minute+time.Second {
			t.Errorf("waited %s, want the one-minute budget", waited)
		}
	})
}
