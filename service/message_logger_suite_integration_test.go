//go:build integration

package service

import (
	"context"
	"log/slog"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/internal/harness/lifecycletest"
	"github.com/c360studio/semengine/pkg/errs"
)

// suiteMessageLoggerConfig monitors two explicit subjects, listed out of order, so a started
// logger holds two subscriptions and needs no component registry declarations.
const suiteMessageLoggerConfig = `{"monitor_subjects":["suite.logger.b","suite.logger.a"]}`

// messageLoggerOwner adapts the message logger to the lifecycle suite (lifecycle-suite, "Observe
// adapter contract"; design D2, D7 U-a). Observe lists "base-status=<status>" while the base
// service is not stopped, "subscription:<subject>" for each subscription the logger holds, in
// sorted order, "subscribe-retry" while the retry of failed subscribes runs, and
// "watch-streams=<n>" while n watch streams are open. It reads the base service's status through
// its accessor and the rest from the logger's unexported state under the logger's own lock, never a
// lifecycle guard's field, and adds nothing to the MessageLogger (#186, #190).
type messageLoggerOwner struct {
	ml *MessageLogger
}

func (o *messageLoggerOwner) Start(ctx context.Context) error { return o.ml.Start(ctx) }
func (o *messageLoggerOwner) Stop(ctx context.Context) error  { return o.ml.Stop(ctx) }

func (o *messageLoggerOwner) Observe() lifecycletest.Observation {
	var held []string
	if status := o.ml.Status(); status != StatusStopped {
		held = append(held, "base-status="+status.String())
	}
	o.ml.lifecycleMu.Lock()
	subjects := make([]string, 0, len(o.ml.subscriptions))
	for subject := range o.ml.subscriptions {
		subjects = append(subjects, subject)
	}
	retryDone := o.ml.retryDone
	streams := len(o.ml.watchStreams)
	o.ml.lifecycleMu.Unlock()
	slices.Sort(subjects)
	for _, subject := range subjects {
		held = append(held, "subscription:"+subject)
	}
	if retryDone != nil && !isClosed(retryDone) {
		held = append(held, "subscribe-retry")
	}
	if streams > 0 {
		held = append(held, "watch-streams="+strconv.Itoa(streams))
	}
	return lifecycletest.Observation{Unresolved: held}
}

// TestMessageLoggerPassesTheLifecycleSuite runs the lifecycle suite on the message logger (ADR-113;
// design D2; the pin's message_logger_lifecycle_test.go:70-100). Every logger is built through
// NewMessageLoggerService over one fixture client, shared because each fixtureClient call resets
// the shared server. The must-fail owner is a logger stopped before Start, whose Start is refused
// (base_lifecycle_test.go's precedent). The logger is one-shot: no restart is promised.
//
// spec: service-lifecycle / The message logger's Start bounds Start and Stop is the only stop
func TestMessageLoggerPassesTheLifecycleSuite(t *testing.T) {
	client := fixtureClient(t)
	newOwner := func() *messageLoggerOwner {
		return &messageLoggerOwner{ml: newTestMessageLogger(t, client, suiteMessageLoggerConfig, nil)}
	}
	factory := func() lifecycletest.Owner { return newOwner() }
	mustFail := func() lifecycletest.Owner {
		owner := newOwner()
		require.NoError(t, owner.Stop(stopContext(t)))
		return owner
	}
	lifecycletest.Run(t, factory, mustFail, lifecycletest.Promise{})

	// lifecycle-suite, "Started owner reports its handles": the suite judges only what is left
	// after a Stop, so an adapter that never reported a subscription would pass it.
	t.Run("StartedOwnerReportsItsHandles", func(t *testing.T) {
		owner := newOwner()
		require.NoError(t, owner.Start(t.Context()))
		require.Equal(t, []string{
			"base-status=running", "subscription:suite.logger.a", "subscription:suite.logger.b",
		}, owner.Observe().Unresolved)
		require.NoError(t, owner.Stop(stopContext(t)))
		require.Empty(t, owner.Observe().Unresolved, "a Stop that returned nil left a handle")
	})

	// design D2, "Refusals": a Start after any Stop is refused with errs.ErrAlreadyStarted and
	// holds nothing. The suite's FailedStartHoldsNothing accepts any refusal.
	t.Run("StartAfterStopIsRefused", func(t *testing.T) {
		owner := newOwner()
		require.NoError(t, owner.Stop(stopContext(t)))
		require.ErrorIs(t, owner.Start(t.Context()), errs.ErrAlreadyStarted)
		require.Empty(t, owner.Observe().Unresolved)
	})
}

// TestMessageLoggerStartFailsWhenItsContextEndsAfterSubscribing is design D2's final check: with
// one subject, the Start context is cancelled while the logger writes that subject's Debug
// "Subscribed to subject" record, which it writes after its last subscribe has succeeded and been
// recorded. Start then returns an error matching context.Canceled, writes no "MessageLogger
// started" record, and holds nothing; a later Stop returns nil (ADR-113: if ctx ends before Start
// returns, Start returns its error and holds nothing). A logger without the check after its loop
// starts with the subscription held.
//
// spec: service-lifecycle / The message logger's Start bounds Start and Stop is the only stop
func TestMessageLoggerStartFailsWhenItsContextEndsAfterSubscribing(t *testing.T) {
	client := fixtureClient(t)
	handler := newHoldingHandler(t, "Subscribed to subject")
	owner := &messageLoggerOwner{ml: newTestMessageLogger(t, client,
		`{"monitor_subjects":["suite.logger.final-check"]}`, slog.New(handler))}
	stopLoggerOnCleanup(t, owner.ml)
	started := handler.arrived("MessageLogger started")

	startCtx, cancelStart := context.WithCancel(t.Context())
	defer cancelStart()
	startResult := make(chan error, 1)
	go func() { startResult <- owner.Start(startCtx) }()
	awaitSignal(t, handler.heldRecord(), "the subject's Subscribed record held")
	cancelStart()
	handler.releaseHeld()

	require.ErrorIs(t, <-startResult, context.Canceled, "a Start whose context ended after its last subscribe fails")
	select {
	case <-started:
		t.Fatal(`a failed Start wrote "MessageLogger started"`)
	default:
	}
	require.Empty(t, owner.Observe().Unresolved, "a failed Start holds nothing")
	require.NoError(t, owner.Stop(stopContext(t)))
}
