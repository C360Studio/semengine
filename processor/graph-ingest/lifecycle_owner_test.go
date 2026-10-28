//go:build integration

package graphingest

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/internal/dispatch"
	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/pkg/errs"
)

// The tests in this file, and the Stop tests in discoverable_reads_test.go,
// entity_cache_stop_test.go and test_owner_integration_test.go, put a running component's
// handles in place by hand, except its JetStream input: cleanup waits for that through
// natsclient's WaitConsumerHandlers, which refuses a handle its client did not return
// (#441), so the input is a real consumer on the package's fixture server, bound as Start
// binds it. A consume callback held on the poison warning stands for a consumer still open.

// newFixtureInputComponent builds graph-ingest with its default configuration on a client
// of its own on the package's fixture server, with a logger that holds the goroutine
// logging the poison warning until the returned pause is released. Every other record is
// dropped.
func newFixtureInputComponent(t *testing.T) (*Component, *pauseOnRecord) {
	t.Helper()
	client := newFixtureClient(t, entityStream)
	pause := newPauseOnRecord("graph-ingest: poison message; terminating")
	// Registered after the client's own cleanup, so it runs first: closing the client
	// joins a callback still held.
	t.Cleanup(pause.Release)
	deps := testDependencies(t, client)
	deps.Logger = slog.New(pause)
	configJSON, err := json.Marshal(DefaultConfig())
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	created, err := CreateGraphIngest(configJSON, deps)
	if err != nil {
		t.Fatalf("CreateGraphIngest: %v", err)
	}
	return created.(*Component), pause
}

// bindHeldInput binds c's input as Start does (setupSubscriptions, submitting to
// c.ingestPool under submitCtx), publishes one message the callback cannot decode, and
// returns the input's handle once that callback is held on the poison warning. Only
// pause.Release lets it return; a poison message is never submitted.
func bindHeldInput(ctx, submitCtx context.Context, t *testing.T, c *Component, pause *pauseOnRecord) jetstream.ConsumeContext {
	t.Helper()
	if err := c.setupSubscriptions(ctx, submitCtx); err != nil {
		t.Fatalf("bind the input: %v", err)
	}
	if len(c.consumers) != 1 {
		t.Fatalf("bound %d inputs, want 1", len(c.consumers))
	}
	if err := c.natsClient.PublishToStream(ctx, "entity.poison", []byte("not a message")); err != nil {
		t.Fatalf("publish the poison message: %v", err)
	}
	pause.awaitEntered(ctx, t, 30*time.Second)
	return c.consumers[0].handle
}

// handlersReturned reports, without waiting, whether delivery on handle has ended and
// every callback of it has returned: WaitConsumerHandlers under a context that has already
// ended returns nil only then.
func handlersReturned(t *testing.T, c *Component, handle jetstream.ConsumeContext) bool {
	ended, end := context.WithCancel(t.Context())
	end()
	return c.natsClient.WaitConsumerHandlers(ended, handle) == nil
}

// awaitReleaseBegun waits until c's Health reports "stopped": the shell clears running
// as it begins release, so a Stop running on another goroutine has begun its cleanup.
// Each Health read must return within readBound; one that does not fails the test.
func awaitReleaseBegun(ctx context.Context, t *testing.T, c *Component) {
	t.Helper()
	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	const blocked = "blocked"
	status, err := probe.Await(waitCtx, func(context.Context) (string, error) {
		health, ok := readWithin(t, "Health while Stop begins", c.Health)
		if !ok {
			return blocked, nil
		}
		return health.Status, nil
	}, func(status string) bool { return status == "stopped" || status == blocked })
	if err != nil {
		t.Fatalf("Stop never began its release: %v", err)
	}
	if status == blocked {
		t.FailNow()
	}
}

type graphIngestLifecycleCoreSubscription struct {
	drainSeen chan struct{}
	drainOnce sync.Once
}

func (s *graphIngestLifecycleCoreSubscription) Drain(context.Context) error {
	s.drainOnce.Do(func() { close(s.drainSeen) })
	return nil
}

// TestLifecycleOwnerRunningStopPreservesEffectSettlementOrder: Stop keeps submission,
// pool and readiness authority until every consume callback has returned, and drains the
// core subscriptions only after the keyed pool completed the work it admitted. The input's
// callback is held; each cancel cleanup calls checks, when it runs, that the callback has
// returned (handlersReturned).
func TestLifecycleOwnerRunningStopPreservesEffectSettlementOrder(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	owner, pause := newFixtureInputComponent(t)
	processEntered := make(chan struct{})
	releaseProcess := make(chan struct{})
	processDone := make(chan struct{})
	poolCtx, poolCancel := context.WithCancel(t.Context())
	pool, err := dispatch.NewKeyedPool(poolCtx, dispatch.KeyedConfig[ingestWork]{
		Lanes:      1,
		QueueDepth: 1,
		Process: func(ctx context.Context, _ int, _ ingestWork) {
			close(processEntered)
			select {
			case <-releaseProcess:
				close(processDone)
			case <-ctx.Done():
			}
		},
		Logger: slog.Default(),
	})
	if err != nil {
		t.Fatalf("construct keyed pool: %v", err)
	}
	if err := pool.SubmitBlocking(t.Context(), "entity", ingestWork{}); err != nil { // Submit is gone (design D6)
		t.Fatalf("submit work: %v", err)
	}
	<-processEntered

	runCtx, runCancel := context.WithCancel(t.Context())
	submitCtx, submitCancel := context.WithCancel(runCtx)
	owner.cancel = runCancel
	owner.ingestPool = pool
	handle := bindHeldInput(ctx, submitCtx, t, owner, pause)
	owner.ingestSubmitCancel = func() {
		if !handlersReturned(t, owner, handle) {
			t.Error("submission authority canceled while a consume callback still runs")
		}
		if got := owner.boundConsumerSnapshot(); len(got) != 1 {
			t.Errorf("readiness observation removed before the consume callbacks returned: %v", got)
		}
		submitCancel()
	}
	owner.ingestPoolCancel = func() {
		if !handlersReturned(t, owner, handle) {
			t.Error("pool authority canceled while a consume callback still runs")
		}
		poolCancel()
	}
	coreSub := &graphIngestLifecycleCoreSubscription{drainSeen: make(chan struct{})}
	owner.subscriptions = []graphIngestCoreSubscription{coreSub}
	startGuardAcquiringNothing(t, owner) // the pin set lifecycleUsed: true (design D22)

	stopResult := make(chan error, 1)
	go func() { stopResult <- owner.Stop(ctx) }()
	awaitReleaseBegun(ctx, t, owner)
	select {
	case <-coreSub.drainSeen:
		t.Fatal("core subscription drained while a consume callback still runs")
	default:
	}

	pause.Release()
	<-submitCtx.Done()
	select {
	case <-coreSub.drainSeen:
		t.Fatal("core subscription drained before keyed pool completed admitted work")
	default:
	}
	close(releaseProcess)
	<-processDone
	<-coreSub.drainSeen
	if err := <-stopResult; err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !handlersReturned(t, owner, handle) {
		t.Fatal("Stop returned nil before the input's delivery ended")
	}
	if !errors.Is(runCtx.Err(), context.Canceled) {
		t.Fatalf("runtime context after Stop = %v, want canceled", runCtx.Err())
	}
	if got := owner.boundConsumerSnapshot(); len(got) != 0 {
		t.Fatalf("terminal readiness observation retained: %v", got)
	}
}

// TestLifecycleOwnerFailedCleanupRetainsExactHandlesForLaterStop proves graph-ingest's
// branch of lifecycle-suite, "Failed start whose own cleanup fails" (design D7.3), which
// the suite's failed-start check does not judge: a Start that fails after acquiring
// resources, and whose own cleanup then fails too, returns both failures, keeps what it
// could not release on record, and a later Stop that can complete the cleanup returns nil
// and leaves nothing. What is held is read through the suite's adapter, suiteOwner.
//
// The guard is reached through its Start (design D22): a start function that binds the
// input and fails, and a rollback that is the component's own release under a context
// that has already ended, as a rollback budget that expires before the input's callbacks
// return would leave it. The guard runs the one cleanup Start was given, so that cleanup
// fails on its first call, the rollback, and runs release with the later Stop's context
// after that. As storage/objectstore's TestFailedCleanupRetainsInputsForLaterStop does,
// the input is a real consumer bound as Start binds it.
func TestLifecycleOwnerFailedCleanupRetainsExactHandlesForLaterStop(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	component, _ := newFixtureInputComponent(t)
	if err := component.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	runCtx, runCancel := context.WithCancel(t.Context())
	owner := newSuiteOwner(component)

	errStart := errors.New("start fails after acquiring")
	expired, expire := context.WithCancel(t.Context())
	expire()
	rolledBack := false // the guard runs the cleanup one call at a time
	err := component.lifecycle.Start(ctx,
		func(ctx context.Context) error {
			component.cancel = runCancel
			// Nothing is published to the input, so its callback never runs and needs no pool.
			if err := component.setupSubscriptions(ctx, runCtx); err != nil {
				return err
			}
			return errStart
		},
		func(ctx context.Context) error {
			if !rolledBack { // the rollback: release under the ended context fails
				rolledBack = true
				return component.release(expired)
			}
			return component.release(ctx)
		})
	if !errors.Is(err, errStart) || !errors.Is(err, context.Canceled) {
		t.Fatalf("failed Start whose cleanup fails = %v, want the start failure and context.Canceled", err)
	}
	want := []string{"runtime cancel", "consumers", "readiness bound consumers"}
	if got := owner.Observe().Unresolved; !slices.Equal(got, want) {
		t.Fatalf("held after the failed cleanup = %v, want %v", got, want)
	}
	if !component.consumers[0].drainIssued {
		t.Fatal("the failed cleanup issued no Drain on the input")
	}
	if !errors.Is(runCtx.Err(), context.Canceled) {
		t.Fatalf("failed cleanup runtime context = %v, want canceled", runCtx.Err())
	}
	if err := owner.Start(t.Context()); !errors.Is(err, errs.ErrAlreadyStarted) {
		t.Fatalf("Start while cleanup pending error = %v, want ErrAlreadyStarted", err)
	}

	stopCtx, stopCancel := context.WithTimeout(ctx, 30*time.Second)
	defer stopCancel()
	if err := owner.Stop(stopCtx); err != nil {
		t.Fatalf("later Stop: %v", err)
	}
	if got := owner.Observe().Unresolved; len(got) != 0 {
		t.Fatalf("held after the later Stop = %v, want nothing", got)
	}
	if err := owner.Stop(stopCtx); err != nil {
		t.Fatalf("repeated terminal Stop: %v", err)
	}
}

// TestLifecycleOwnerRunningDeadlineKeepsCleanupWithoutReplay is the pin's
// TestLifecycleOwnerRunningDeadlineIsTerminalWithoutReplay, inverted (design D22): a
// running owner whose Stop context ends while it waits for the input's callback keeps its
// handles, and the next Stop runs cleanup again and returns its result. At the pin the
// failed Stop cleared the handles and the next Stop returned nil with the consumer still
// open; under design D13 only a cleanup that succeeded clears them. The failed Stop
// records its Drain (drainIssued), so the next one does not issue it again; a second
// Drain on a real handle changes nothing this test can see (notes/2.2.md of
// fix-441-graph-ingest-stop-join).
func TestLifecycleOwnerRunningDeadlineKeepsCleanupWithoutReplay(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	owner, pause := newFixtureInputComponent(t)
	if err := owner.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	owner.cancel = func() {}
	bindHeldInput(ctx, ctx, t, owner, pause)
	startGuardAcquiringNothing(t, owner) // the pin set lifecycleUsed: true (design D22)
	stopCtx, expire := context.WithCancel(ctx)
	stopResult := make(chan error, 1)
	go func() { stopResult <- owner.Stop(stopCtx) }()
	awaitReleaseBegun(ctx, t, owner)
	expire()
	if err := <-stopResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("deadline Stop error = %v, want context.Canceled", err)
	}
	if len(owner.consumers) != 1 || !owner.consumers[0].drainIssued {
		t.Fatalf("deadline Stop dropped the handle it failed to close: consumers=%d", len(owner.consumers))
	}

	// The next Stop runs cleanup again; with the callback returned it succeeds and
	// clears the handles.
	pause.Release()
	nextCtx, nextCancel := context.WithTimeout(ctx, 30*time.Second)
	defer nextCancel()
	if err := owner.Stop(nextCtx); err != nil {
		t.Fatalf("next Stop after the callback returned: %v", err)
	}
	if len(owner.consumers) != 0 {
		t.Fatalf("next Stop did not run cleanup: consumers=%d", len(owner.consumers))
	}
	if err := owner.Stop(nextCtx); err != nil {
		t.Fatalf("repeated Stop after a completed one: %v", err)
	}
	if err := owner.Start(t.Context()); !errors.Is(err, errs.ErrAlreadyStarted) {
		t.Fatalf("restart after Stop error = %v, want ErrAlreadyStarted", err)
	}
}
