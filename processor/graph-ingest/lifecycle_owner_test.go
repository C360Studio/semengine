package graphingest

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"

	"github.com/c360studio/semengine/internal/dispatch"
	"github.com/c360studio/semengine/pkg/errs"
)

type graphIngestLifecycleCoreSubscription struct {
	drainSeen chan struct{}
	drainOnce sync.Once
}

func (s *graphIngestLifecycleCoreSubscription) Drain(context.Context) error {
	s.drainOnce.Do(func() { close(s.drainSeen) })
	return nil
}

func TestLifecycleOwnerRunningStopPreservesEffectSettlementOrder(t *testing.T) {
	consumer := &graphIngestLifecycleConsumeContext{
		closed: make(chan struct{}), drainSeen: make(chan struct{}),
	}
	coreSub := &graphIngestLifecycleCoreSubscription{drainSeen: make(chan struct{})}
	processEntered := make(chan struct{})
	releaseProcess := make(chan struct{})
	processDone := make(chan struct{})
	poolCtx, poolCancel := context.WithCancel(t.Context())
	pool, err := dispatch.NewKeyedPool(poolCtx, dispatch.KeyedConfig[ingestWork]{
		Lanes:      1,
		QueueDepth: 1,
		KeyOf:      func(ingestWork) string { return "entity" },
		Process: func(ctx context.Context, _ int, _ ingestWork) error {
			close(processEntered)
			select {
			case <-releaseProcess:
				close(processDone)
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}, dispatch.KeyedDeps{Logger: slog.Default()})
	if err != nil {
		t.Fatalf("construct keyed pool: %v", err)
	}
	if err := pool.SubmitBlocking(t.Context(), ingestWork{}); err != nil { // Submit is gone (design D6)
		t.Fatalf("submit work: %v", err)
	}
	<-processEntered

	runCtx, runCancel := context.WithCancel(t.Context())
	submitCtx, submitCancel := context.WithCancel(runCtx)
	owner := withTestRegistry(t, &Component{
		logger:             slog.Default(),
		running:            true,
		cancel:             runCancel,
		ingestPool:         pool,
		ingestPoolCancel:   poolCancel,
		ingestSubmitCancel: submitCancel,
		consumers: []graphIngestConsumerBinding{{
			handle: consumer,
		}},
		subscriptions:  []graphIngestCoreSubscription{coreSub},
		boundConsumers: []boundConsumer{{stream: "ENTITY", name: "graph-ingest-entity"}},
	})
	startGuardAcquiringNothing(t, owner) // the pin set lifecycleUsed: true (design D22)

	stopResult := make(chan error, 1)
	go func() { stopResult <- owner.Stop(t.Context()) }()
	<-consumer.drainSeen
	if err := submitCtx.Err(); err != nil {
		t.Fatalf("submission authority canceled before native Closed: %v", err)
	}
	if err := poolCtx.Err(); err != nil {
		t.Fatalf("pool authority canceled before native Closed: %v", err)
	}
	if got := owner.boundConsumerSnapshot(); len(got) != 1 {
		t.Fatalf("readiness observation removed before native Closed: %v", got)
	}
	select {
	case <-coreSub.drainSeen:
		t.Fatal("core subscription drained before JetStream Closed and keyed settlement")
	default:
	}

	close(consumer.closed)
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
	if consumer.drains.Load() != 1 {
		t.Fatalf("native Drain calls = %d, want 1", consumer.drains.Load())
	}
	if !errors.Is(runCtx.Err(), context.Canceled) {
		t.Fatalf("runtime context after Stop = %v, want canceled", runCtx.Err())
	}
	if got := owner.boundConsumerSnapshot(); len(got) != 0 {
		t.Fatalf("terminal readiness observation retained: %v", got)
	}
}

func TestLifecycleOwnerFailedCleanupRetainsExactHandlesForLaterStop(t *testing.T) {
	consumer := &graphIngestLifecycleConsumeContext{
		closed: make(chan struct{}), drainSeen: make(chan struct{}),
	}
	runCtx, runCancel := context.WithCancel(t.Context())
	owner := withTestRegistry(t, &Component{
		logger:      slog.Default(),
		initialized: true,                 // passed Initialize, so Start reaches the guard (design D13)
		natsClient:  newTestNATSClient(t), // never connected
		cancel:      runCancel,
		consumers: []graphIngestConsumerBinding{{
			handle: consumer,
		}},
	})
	leaveGuardCleanupPending(t, owner) // the pin set lifecycleUsed and cleanupPending (design D22)

	expired, expire := context.WithCancel(t.Context())
	expire()
	if err := owner.cleanup(expired); !errors.Is(err, context.Canceled) {
		t.Fatalf("expired cleanup error = %v, want context.Canceled", err)
	}
	<-consumer.drainSeen
	if consumer.drains.Load() != 1 || len(owner.consumers) != 1 || !owner.consumers[0].drainIssued {
		t.Fatalf("failed cleanup lost exact handle: drains=%d consumers=%d", consumer.drains.Load(), len(owner.consumers))
	}
	if owner.cancel == nil {
		t.Fatal("failed cleanup discarded runtime cancellation authority")
	}
	if !errors.Is(runCtx.Err(), context.Canceled) {
		t.Fatalf("failed cleanup runtime context = %v, want canceled", runCtx.Err())
	}
	if err := owner.Start(t.Context()); !errors.Is(err, errs.ErrAlreadyStarted) {
		t.Fatalf("Start while cleanup pending error = %v, want ErrAlreadyStarted", err)
	}

	close(consumer.closed)
	if err := owner.Stop(t.Context()); err != nil {
		t.Fatalf("later Stop: %v", err)
	}
	if consumer.drains.Load() != 1 {
		t.Fatalf("native Drain replayed: calls=%d", consumer.drains.Load())
	}
	if err := owner.Stop(t.Context()); err != nil {
		t.Fatalf("repeated terminal Stop: %v", err)
	}
}

// TestLifecycleOwnerRunningDeadlineKeepsCleanupWithoutReplay is the pin's
// TestLifecycleOwnerRunningDeadlineIsTerminalWithoutReplay, inverted (design D22): a
// running owner whose Stop fails during the drain keeps its handles, and the next Stop
// runs cleanup again and returns its result. At the pin the failed Stop cleared the
// handles and the next Stop returned nil with the consumer still open; under design
// D13 only a cleanup that succeeded clears them. The drain is still issued once.
func TestLifecycleOwnerRunningDeadlineKeepsCleanupWithoutReplay(t *testing.T) {
	consumer := &graphIngestLifecycleConsumeContext{
		closed: make(chan struct{}), drainSeen: make(chan struct{}),
	}
	owner := withTestRegistry(t, &Component{
		logger:      slog.Default(),
		initialized: true,                 // passed Initialize, so Start reaches the guard (design D13)
		natsClient:  newTestNATSClient(t), // never connected
		running:     true,
		cancel:      func() {},
		consumers: []graphIngestConsumerBinding{{
			handle: consumer,
		}},
	})
	startGuardAcquiringNothing(t, owner) // the pin set lifecycleUsed: true (design D22)
	stopCtx, expire := context.WithCancel(t.Context())
	stopResult := make(chan error, 1)
	go func() { stopResult <- owner.Stop(stopCtx) }()
	<-consumer.drainSeen
	expire()
	if err := <-stopResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("deadline Stop error = %v, want context.Canceled", err)
	}
	if len(owner.consumers) != 1 || !owner.consumers[0].drainIssued {
		t.Fatalf("deadline Stop dropped the handle it failed to close: consumers=%d", len(owner.consumers))
	}

	// The next Stop runs cleanup again; with the consumer closed it succeeds and
	// clears the handles.
	close(consumer.closed)
	if err := owner.Stop(t.Context()); err != nil {
		t.Fatalf("next Stop after the consumer closed: %v", err)
	}
	if len(owner.consumers) != 0 {
		t.Fatalf("next Stop did not run cleanup: consumers=%d", len(owner.consumers))
	}
	if consumer.drains.Load() != 1 {
		t.Fatalf("native Drain replayed: calls=%d", consumer.drains.Load())
	}
	if err := owner.Stop(t.Context()); err != nil {
		t.Fatalf("repeated Stop after a completed one: %v", err)
	}
	if err := owner.Start(t.Context()); !errors.Is(err, errs.ErrAlreadyStarted) {
		t.Fatalf("restart after Stop error = %v, want ErrAlreadyStarted", err)
	}
}

// TestLifecycleOwnerStopBeforeStartIsTerminal reads, at the pin, lifecycleUsed after the
// refused Start and after the refused Stop(nil). The guard keeps its state to itself
// (design D22), so each read is a probe, assertGuardUnused, which uses the guard up: each
// probe runs on a fresh component that repeats the steps before it. The components passed
// Initialize, so Start reaches the guard (design D13).
func TestLifecycleOwnerStopBeforeStartIsTerminal(t *testing.T) {
	newOwner := func() *Component {
		return withTestRegistry(t, &Component{initialized: true, natsClient: newTestNATSClient(t)})
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	startCanceled := func(owner *Component) {
		t.Helper()
		if err := owner.Start(canceled); err == nil {
			t.Fatal("pre-canceled Start succeeded")
		}
	}
	stopNil := func(owner *Component) {
		t.Helper()
		if err := owner.Stop(nil); err == nil {
			t.Fatal("Stop(nil) succeeded")
		}
	}

	owner := newOwner()
	startCanceled(owner)
	assertGuardUnused(t, owner) // the pre-canceled Start consumed no lifecycle authority

	owner = newOwner()
	startCanceled(owner)
	stopNil(owner)
	assertGuardUnused(t, owner) // Stop(nil) consumed no lifecycle authority

	owner = newOwner()
	startCanceled(owner)
	stopNil(owner)
	if err := owner.Stop(t.Context()); err != nil {
		t.Fatalf("Stop before Start: %v", err)
	}
	if err := owner.Start(t.Context()); !errors.Is(err, errs.ErrAlreadyStarted) {
		t.Fatalf("Start after terminal Stop error = %v, want ErrAlreadyStarted", err)
	}
	if err := owner.Stop(t.Context()); err != nil {
		t.Fatalf("repeated completed Stop: %v", err)
	}
}
