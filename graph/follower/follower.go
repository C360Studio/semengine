// Package follower follows ENTITY_STATES for a projection owner: it waits for the bucket, holds the
// one watch on it, replays the stored entities and then the live writes in delivery order, and
// keeps the owner's readiness: the watermark of applied revisions, the bootstrap latch, the reset
// latch, the lost-feed state, and the envelope it publishes to GRAPH_STATUS on every heartbeat
// (ADR-108 D4). The owner, a plug-in, takes each entry through Config.Deliver and reports each
// outcome back: Complete when a revision is applied, Config.Failed for its unresolved writes, Reset
// when its state can no longer be trusted. graph-index is the first owner.
//
// Limits, declared rather than promised:
//
//   - A lost feed is never repaired. When the watch cannot start, closes, or reads a stream other
//     than the one Open bound, the follower says so (Stuck, a degraded envelope, QueryReady refusing)
//     until the process restarts; nothing rebinds the watch.
//   - A restored stream can go unnoticed. The third trigger compares the backing stream's creation
//     time with the one Open read; a stream deleted and restored with that creation time inside one
//     stall of the watch reads as the same stream, and the revisions it skipped are not declared.
//   - How long a watch stalls after a reconnect, and when a deleted bucket closes it, is nats.go's
//     timing (about 20 s at nats.go v1.54.0, PR #414 comments 6425769680 and 6426443564), not
//     promised here.
package follower

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/graph/readiness"
	"github.com/c360studio/semengine/internal/resource"
	"github.com/c360studio/semengine/internal/revlag"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

// Entry is one delivered ENTITY_STATES write.
type Entry struct {
	Key      string
	Revision uint64
	Deleted  bool // a delete or purge marker
	Bytes    int  // the delivered value's length; 0 for a delete or purge marker
}

// Config is what Open needs. Client, Key, Deliver and Failed are required.
type Config struct {
	Client *natsclient.Client
	// Key is the producer's GRAPH_STATUS key (readiness.KeyGraphIndex). The gauges' Prometheus
	// subsystem is derived from it ("graph-index" becomes "graph_index"), so the name is spelled once.
	Key string
	// MetricsRegistry takes the readiness gauges. Nil registers nothing.
	MetricsRegistry *metric.MetricsRegistry
	// Logger defaults to slog.Default().
	Logger *slog.Logger
	// StartupAttempts and StartupInterval bound Open's wait for ENTITY_STATES; zero takes
	// internal/resource's defaults.
	StartupAttempts int
	StartupInterval time.Duration
	// Deliver takes one entry, in delivery order, on Run's goroutine. It is called after the
	// revision is in the watermark; the plug-in calls Complete once it is applied.
	Deliver func(ctx context.Context, e Entry)
	// Failed returns the plug-in's count of unresolved required writes.
	Failed func() int64
}

// degradedStuckAfter is how long the floor may stand still while behind before the envelope reads
// degraded (ADR-066 §4). It is a property of the envelope, not a Stuck state: a slow replay is
// catch-up (ADR-109 D4).
const degradedStuckAfter = 30 * time.Second

// resetRemedy is what an operator does about a latched reset, as graph-index said at the pin.
const resetRemedy = "export if needed, clear the graph and index buckets, and reingest canonical sources"

// Follower follows ENTITY_STATES for one plug-in. Build it with Open and run it with Run.
type Follower struct {
	key       string
	logger    *slog.Logger
	deliver   func(context.Context, Entry)
	failed    func() int64
	reader    kvcatalog.CatalogReader // the watch
	status    kvcatalog.CatalogReader // target and generation reads, a handle of its own
	publisher *readiness.Publisher
	gauges    *readiness.Gauges
	watermark *revlag.Watermark
	created   time.Time // the backing stream's creation time when Open bound it: its generation (#15)

	feedLost        atomic.Bool
	reset           atomic.Pointer[graph.StateResetReason]
	snapshotDone    atomic.Bool   // the watch's end-of-snapshot marker arrived
	bootstrapTarget atomic.Uint64 // stored before snapshotDone is raised
	bootstrapped    atomic.Bool   // the initial build is complete; never cleared

	// statusMu serialises reads through status: nats.go v1.54.0's KV Status writes the handle's
	// stream-info cache unsynchronised, and QueryReady callers and the heartbeat share the handle
	// (#417, TestFollowerConcurrentQueryReady).
	statusMu sync.Mutex

	mu              sync.Mutex // guards the stuck detector's progress record
	lastIndexedSeen uint64
	lastProgressAt  time.Time
}

// Open waits for ENTITY_STATES up to Config.StartupAttempts times Config.StartupInterval, opens
// its readers, ensures GRAPH_STATUS and registers the readiness gauges. It does its work under
// ctx and starts nothing: the watch begins in Run. An absent bucket after the wait is a transient
// error naming it.
func Open(ctx context.Context, cfg Config) (*Follower, error) {
	switch {
	case ctx == nil:
		return nil, invalid("nil context")
	case cfg.Client == nil:
		return nil, invalid("nil client")
	case cfg.Key == "":
		return nil, invalid("empty status key")
	case cfg.Deliver == nil:
		return nil, invalid("nil Deliver")
	case cfg.Failed == nil:
		return nil, invalid("nil Failed")
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	wait := resource.NewWatcher(graph.BucketEntityStates, func(ctx context.Context) error {
		_, err := kvcatalog.OpenCatalogReader(ctx, cfg.Client, graph.BucketEntityStates)
		return err
	}, resource.Config{StartupAttempts: cfg.StartupAttempts, StartupInterval: cfg.StartupInterval, Logger: logger})
	if !wait.WaitForStartup(ctx) {
		return nil, errs.WrapTransient(fmt.Errorf("bucket %s not available after the startup wait", graph.BucketEntityStates),
			"follower", "Open", "wait for ENTITY_STATES")
	}
	reader, err := kvcatalog.OpenCatalogReader(ctx, cfg.Client, graph.BucketEntityStates)
	if err != nil {
		return nil, errs.Wrap(err, "follower", "Open", "open ENTITY_STATES")
	}
	status, err := kvcatalog.OpenCatalogReader(ctx, cfg.Client, graph.BucketEntityStates)
	if err != nil {
		return nil, errs.Wrap(err, "follower", "Open", "open ENTITY_STATES status reader")
	}
	_, created, err := readStream(ctx, status)
	if err != nil {
		return nil, errs.Wrap(err, "follower", "Open", "read the ENTITY_STATES stream")
	}
	statusBucket, err := kvcatalog.EnsureCatalogBucket(ctx, cfg.Client, graph.BucketGraphStatus)
	if err != nil {
		return nil, errs.Wrap(err, "follower", "Open", "ensure GRAPH_STATUS")
	}
	publisher, err := readiness.NewPublisher(statusBucket, cfg.Key)
	if err != nil {
		return nil, errs.Wrap(err, "follower", "Open", "build the status publisher")
	}
	gauges := readiness.NewRevisionGauges(strings.ReplaceAll(cfg.Key, "-", "_"))
	if err := gauges.Register(cfg.MetricsRegistry); err != nil {
		return nil, errs.Wrap(err, "follower", "Open", "register the readiness gauges")
	}
	return &Follower{
		key:       cfg.Key,
		logger:    logger,
		deliver:   cfg.Deliver,
		failed:    cfg.Failed,
		reader:    reader,
		status:    status,
		publisher: publisher,
		gauges:    gauges,
		watermark: revlag.New(),
		created:   created,
	}, nil
}

func invalid(what string) error {
	return errs.WrapInvalid(errors.New(what), "follower", "Open", "check the configuration")
}

// Run follows ENTITY_STATES on the caller's goroutine until ctx ends, then returns with the watch
// stopped, its delivery closed and its heartbeat goroutine joined. It returns nil, or the watch's
// Stop error when that error says more than that the watch was already gone. It waits for the
// delivery to close for at most watchDrainBudget (see stopWatch) and then returns
// errWatchDeliveryOpen. It publishes the
// envelope once at once and then every readiness.DefaultHeartbeat, on a goroutine of its own, so a
// Deliver that blocks does not delay a heartbeat. A nil context is refused.
func (f *Follower) Run(ctx context.Context) error {
	if ctx == nil {
		return errs.WrapInvalid(errors.New("nil context"), "follower", "Run", "check the context")
	}
	var heartbeat sync.WaitGroup
	heartbeat.Go(func() { f.heartbeat(ctx) })
	defer heartbeat.Wait()

	err := f.follow(ctx)
	<-ctx.Done()
	return err
}

// follow holds the watch until ctx ends or the feed is lost. Its error is stopWatch's.
func (f *Follower) follow(ctx context.Context) error {
	watcher, err := f.reader.WatchAll(ctx)
	if err != nil {
		if ctx.Err() == nil {
			f.loseFeed(fmt.Sprintf("the watch could not start: %v", err))
		}
		return nil
	}
	validating := true
	for {
		select {
		case <-ctx.Done():
			return stopWatch(ctx, watcher)
		case entry, ok := <-watcher.Updates():
			if !ok {
				// The channel closes after Stop or when nats.go gives the watch up, as it does when
				// the bucket is deleted. Only an ending context is Stop's.
				_ = watcher.Stop()
				if ctx.Err() == nil {
					f.loseFeed("the watch closed")
				}
				return nil
			}
			if entry == nil {
				if validating {
					validating = false
					// Every stored entity has been delivered, so the highest revision observed
					// bounds them: it is the revision the initial build must reach (ADR-084 D2).
					f.bootstrapTarget.Store(f.watermark.Observed())
					f.snapshotDone.Store(true)
				}
				continue
			}
			f.observe(ctx, entry, validating)
		}
	}
}

// watchDrainBudget bounds stopWatch's wait for the watch's delivery to close after Stop.
const watchDrainBudget = 5 * time.Second

// errWatchDeliveryOpen says the watch's update channel was still open when watchDrainBudget ran out.
var errWatchDeliveryOpen = errors.New("follower: the ENTITY_STATES watch's delivery did not close after Stop")

// stopWatch stops the watch and drains its updates until nats.go closes the channel, for at most
// watchDrainBudget after ctx, Run's, has ended. At nats.go v1.54.0 Stop only unsubscribes: a
// delivery already blocked on the full updates buffer ends only when the buffer is read, and the
// channel closes after it, so on that path returning is a join (TestFollowerRunReturnsAndLeavesNothing
// drives it with a backlogged watch). The bound is for the one path with nothing to join: WatchAll
// subscribes before it installs the handler that closes the channel (jetstream/kv.go:1356-1368), and
// nats.go reads that handler once, when its delivery goroutine exits (nats.go:3830-3834). A watch
// whose context ends, or whose connection closes, in between leaves the channel open for good with
// no delivery goroutine running (#454). TestStopWatchGivesUpOnADeliveryThatNeverCloses drives the
// bound on a scripted watcher; the window itself has not been reproduced against nats.go.
func stopWatch(ctx context.Context, watcher jetstream.KeyWatcher) error {
	stopErr := watcher.Stop()
	// Terminal cleanup after Run's context has ended, so it runs detached and bounded.
	drainCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), watchDrainBudget)
	defer cancel()
	updates := watcher.Updates()
	for open := true; open; {
		select {
		case _, open = <-updates:
		case <-drainCtx.Done():
			// The delivery never closed, so a Stop error cannot be read as "already gone".
			if stopErr != nil {
				return errors.Join(errWatchDeliveryOpen, fmt.Errorf("follower: stop the ENTITY_STATES watch: %w", stopErr))
			}
			return errWatchDeliveryOpen
		}
	}
	// The delivery has closed. Stop's error then says only that the watch was already gone: nats.go
	// also unsubscribes when the watch's context ends (js.go, subscribe), which is Run's, and can
	// delete the consumer before Stop does (seen as ErrConsumerNotFound in
	// TestFollowerRunReturnsAndLeavesNothing).
	if stopErr != nil && !errors.Is(stopErr, nats.ErrBadSubscription) &&
		!errors.Is(stopErr, nats.ErrConnectionClosed) && !errors.Is(stopErr, nats.ErrConsumerNotFound) {
		return fmt.Errorf("follower: stop the ENTITY_STATES watch: %w", stopErr)
	}
	return nil
}

// observe records one delivered revision in the watermark, validates a stored entity during the
// replay, and hands the entry to the plug-in.
func (f *Follower) observe(ctx context.Context, entry jetstream.KeyValueEntry, validating bool) {
	// entry.Created() is the KV commit time, which makes staleness_ms the age of the view (ADR-083 D3).
	f.watermark.Observe(entry.Revision(), entry.Key(), entry.Created())
	e := Entry{Key: entry.Key(), Revision: entry.Revision(), Deleted: natsclient.IsKVTombstone(entry.Operation())}
	if !e.Deleted {
		e.Bytes = len(entry.Value())
		if validating {
			var state graph.EntityState
			if err := graph.UnmarshalEntityState(e.Key, entry.Value(), &state); err != nil {
				reason := graph.GraphStateReasonUnreadableEntity
				var stateErr *graph.StateContractError
				if errors.As(err, &stateErr) {
					reason = stateErr.Reason
				}
				f.logger.Error("stored entity refused during the startup replay",
					slog.String("key", e.Key), slog.Any("error", err))
				f.Reset(reason)
			}
		}
	}
	f.deliver(ctx, e)
}

// loseFeed declares the feed lost, once; it holds until the process restarts (PR #414 comment
// 6426254115, item 2).
func (f *Follower) loseFeed(cause string) {
	if f.feedLost.CompareAndSwap(false, true) {
		f.logger.Error("ENTITY_STATES feed lost; queries are refused until the process restarts",
			slog.String("cause", cause))
	}
}

// heartbeat publishes the envelope at once and on every tick until ctx ends.
func (f *Follower) heartbeat(ctx context.Context) {
	f.publish(ctx)
	ticker := time.NewTicker(readiness.DefaultHeartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			f.publish(ctx)
		}
	}
}

// publish computes the envelope once and gives it to both the gauges and GRAPH_STATUS, so the two
// never disagree. A failed write is counted and logged; the next heartbeat is the recovery.
func (f *Follower) publish(ctx context.Context) {
	status := f.compute(ctx)
	f.gauges.Set(status)
	if err := f.publisher.Publish(ctx, status); err != nil {
		if ctx.Err() != nil {
			return // Run is ending
		}
		f.gauges.RecordPublishFailure()
		f.logger.Warn("readiness status publish failed",
			slog.String("bucket", graph.BucketGraphStatus), slog.String("key", f.key), slog.Any("error", err))
	}
}

// Complete records that revision of key is applied, with every earlier revision of it.
func (f *Follower) Complete(key string, revision uint64) {
	f.watermark.Complete(key, revision)
}

// Reset latches reason for the life of the process. The first reason holds; later calls change
// nothing (ADR-074, ADR-084 D4).
func (f *Follower) Reset(reason graph.StateResetReason) {
	f.reset.CompareAndSwap(nil, &reason)
}

// Indexed returns the watermark's floor: every delivered revision at or below it is complete.
func (f *Follower) Indexed() uint64 {
	indexed, _ := f.watermark.IndexedAt()
	return indexed
}

// Stuck returns the states that hold now, each a short sentence: a latched reset, unresolved
// writes, and a lost feed. It is empty when none holds.
func (f *Follower) Stuck() []string {
	var stuck []string
	if r := f.reset.Load(); r != nil {
		stuck = append(stuck, fmt.Sprintf("graph state reset required: %s", *r))
	}
	if n := f.failed(); n > 0 {
		stuck = append(stuck, fmt.Sprintf("%d index writes unresolved", n))
	}
	if f.feedLost.Load() {
		stuck = append(stuck, "ENTITY_STATES feed lost; restart the process")
	}
	return stuck
}

// QueryReady says whether a query may be answered now (ADR-109 D1): nil, or a classified refusal.
// A latched reset is fatal graph_state_reset_required; a lost feed, unresolved writes and an
// incomplete initial build are transient index_not_ready. Once the initial build is complete it answers nil at once.
func (f *Follower) QueryReady(ctx context.Context) error {
	if ctx == nil {
		return errs.WrapInvalid(errors.New("nil context"), "follower", "QueryReady", "check the context")
	}
	if r := f.reset.Load(); r != nil {
		return errs.ClassifiedCode(errs.ErrorFatal, graph.ErrorCodeGraphStateResetRequired,
			&graph.StateContractError{Reason: *r, Err: errors.New(resetRemedy)})
	}
	if f.feedLost.Load() {
		return notReady("the ENTITY_STATES feed is lost")
	}
	if f.failed() > 0 {
		return notReady("unresolved index write failures")
	}
	if f.bootstrapped.Load() {
		return nil
	}
	status := f.compute(ctx)
	reason := readiness.EvaluateReadinessGate(readiness.Reading{Status: status, Fresh: true})
	if reason == readiness.DeferNone {
		return nil
	}
	f.logger.Debug("query deferred", slog.String("reason", string(reason)), slog.String("state", string(status.State)),
		slog.Uint64("indexed_revision", status.IndexedRevision), slog.Uint64("target_revision", status.TargetRevision))
	return notReady("initial build has not completed")
}

func notReady(why string) error {
	return errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeIndexNotReady, errors.New("index not ready: "+why))
}

// compute builds the envelope: ComputeIndexStatus over the floor and the stream's last sequence,
// then the overrides only this producer can see, then the bootstrap latch.
func (f *Follower) compute(ctx context.Context) graph.IndexStatusResponse {
	if r := f.reset.Load(); r != nil {
		return graph.IndexStatusResponse{
			State:             graph.IndexStateResetRequired,
			Code:              graph.ErrorCodeGraphStateResetRequired,
			Reason:            string(*r),
			BootstrapComplete: f.bootstrapped.Load(),
		}
	}
	indexed, indexedAt := f.watermark.IndexedAt()
	f.statusMu.Lock()
	target, created, err := readStream(ctx, f.status)
	f.statusMu.Unlock()
	if err == nil && !created.Equal(f.created) {
		// The bucket was deleted and created again: the watch, still open, reads nothing more
		// (PR #414 comment 6426443564).
		f.loseFeed("ENTITY_STATES was created again after the follower bound it")
	}
	if err != nil {
		// Without the target, caught up cannot be confirmed: degraded, not building (ADR-066 §4).
		f.logger.Warn("readiness: ENTITY_STATES status read failed", slog.Any("error", err))
		return graph.IndexStatusResponse{
			State:             graph.IndexStateDegraded,
			IndexedRevision:   indexed,
			BootstrapComplete: f.bootstrapped.Load(),
		}
	}
	failed := f.failed()
	status := readiness.ComputeIndexStatus(readiness.IndexStatusInputs{
		Indexed:     indexed,
		Target:      target,
		Stuck:       f.stalled(indexed, target),
		IndexedAt:   indexedAt,
		FailedCount: uint64(max(failed, 0)),
	})
	// An authoritatively empty graph is ready once the snapshot is delivered, though ready needs a
	// target above zero (pin gh#474).
	if !status.Ready && target == 0 && f.snapshotDone.Load() {
		status.Ready, status.State = true, graph.IndexStateReady
	}
	// Unresolved writes, or a feed that no longer delivers, make the index known-incomplete at any
	// lag (ADR-082).
	if failed > 0 || f.feedLost.Load() {
		status.Ready, status.State = false, graph.IndexStateDegraded
	}
	// The latch: the floor reached the target fixed at the snapshot's end, or the envelope is ready.
	if status.Ready || (f.snapshotDone.Load() && indexed >= f.bootstrapTarget.Load()) {
		f.bootstrapped.Store(true)
	}
	status.BootstrapComplete = f.bootstrapped.Load()
	return status
}

// readStream reads the backing stream once: its last sequence, the target, and its creation time,
// the stream's generation (#15).
func readStream(ctx context.Context, reader kvcatalog.CatalogReader) (lastSeq uint64, created time.Time, err error) {
	s, err := reader.Status(ctx)
	if err != nil {
		return 0, time.Time{}, err
	}
	bucket, ok := s.(*jetstream.KeyValueBucketStatus)
	if !ok || bucket.StreamInfo() == nil {
		return 0, time.Time{}, fmt.Errorf("kv status: unexpected %T without stream info", s)
	}
	info := bucket.StreamInfo()
	return info.State.LastSeq, info.Created, nil
}

// stalled is the stuck-watermark detector: the floor has not advanced for degradedStuckAfter while
// behind the target.
func (f *Follower) stalled(indexed, target uint64) bool {
	now := time.Now()
	f.mu.Lock()
	defer f.mu.Unlock()
	if indexed > f.lastIndexedSeen || f.lastProgressAt.IsZero() {
		f.lastIndexedSeen, f.lastProgressAt = indexed, now
	}
	return !(target > 0 && indexed >= target) && now.Sub(f.lastProgressAt) > degradedStuckAfter
}
