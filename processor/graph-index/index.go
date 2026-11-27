package graphindex

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/follower"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/internal/dispatch"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/vocabulary"
)

// The index write path. The follower (graph/follower) delivers each ENTITY_STATES entry
// to deliver, after observing its revision in the watermark. deliver counts it and hands
// the key to the coalescer, when one is configured, or to the keyed pool. A lane applies
// the key's current authoritative state, read from ENTITY_STATES when the work reaches the
// head of the key's lane, and then completes the revision the work stands for in the
// follower's watermark. Entities whose required writes fail are kept in the failed set,
// which the follower reads through failedTotal to withhold readiness, and which the repair
// loop re-drives.

// indexLaneCapacity is the pool's total queue, split across the lanes.
const indexLaneCapacity = 1000

// indexWriteMaxAttempts bounds the in-place retry of an entity's required index writes.
// The writes are idempotent, so retrying the whole set recovers a transient KV blip
// without marking the entity failed.
const indexWriteMaxAttempts = 3

// indexRepairInterval is how often the repair loop re-drives entities whose required
// index writes or deletes failed.
const indexRepairInterval = 30 * time.Second

// indexWork names one key for a lane. revision is the delivered revision the work
// completes once applied; repair work carries 0 and completes nothing.
type indexWork struct {
	entityID string
	revision uint64
}

// openIndexBuckets acquires the five index buckets graph-index owns and its record
// bucket, as their owner, and the read handle on ENTITY_STATES the lanes read current state
// from, and reads the generation of ENTITY_STATES and of each index bucket. Config.Validate
// has already refused any output that is not one of the four declared index buckets;
// NAME_INDEX and GRAPH_INDEX_RECORDS are internal and not declared outputs.
func (c *Component) openIndexBuckets(ctx context.Context) error {
	buckets := []struct {
		name  string
		store **natsclient.KVStore
	}{
		{graph.BucketOutgoingIndex, &c.outgoingBucket},
		{graph.BucketIncomingIndex, &c.incomingBucket},
		{graph.BucketAliasIndex, &c.aliasBucket},
		{graph.BucketPredicateIndex, &c.predicateBucket},
		{graph.BucketNameIndex, &c.nameBucket},
	}
	generations := make(map[string]uint64, len(buckets)+1)
	for _, b := range buckets {
		kv, err := kvcatalog.EnsureCatalogBucket(ctx, c.natsClient, b.name)
		if err != nil {
			return errs.Wrap(err, "Component", "openIndexBuckets", "KV bucket: "+b.name)
		}
		if generations[b.name], err = bucketGeneration(ctx, kv); err != nil {
			return errs.Wrap(err, "Component", "openIndexBuckets", "generation of "+b.name)
		}
		*b.store = c.natsClient.NewKVStore(kv)
	}
	records, err := kvcatalog.EnsureCatalogBucket(ctx, c.natsClient, graph.BucketGraphIndexRecords)
	if err != nil {
		return errs.Wrap(err, "Component", "openIndexBuckets", "KV bucket: "+graph.BucketGraphIndexRecords)
	}
	c.recordBucket = c.natsClient.NewKVStore(records)
	entities, err := kvcatalog.OpenCatalogReader(ctx, c.natsClient, graph.BucketEntityStates)
	if err != nil {
		return errs.Wrap(err, "Component", "openIndexBuckets", "KV bucket: "+graph.BucketEntityStates)
	}
	if generations[graph.BucketEntityStates], err = bucketGeneration(ctx, entities); err != nil {
		return errs.Wrap(err, "Component", "openIndexBuckets", "generation of "+graph.BucketEntityStates)
	}
	c.entities, c.generations = entities, generations
	return nil
}

// bucketGeneration reads a bucket's generation: the creation time on the server of its backing
// stream, in nanoseconds since the Unix epoch. A bucket deleted and created again gets a new one
// (TestFollowerDeclaresLostFeed's "deleted and recreated" case drives that on the pinned server);
// one restored with its creation time does not. A stream that reports no creation time is
// refused, since two of its generations could not be told apart.
func bucketGeneration(ctx context.Context, bucket interface {
	Status(context.Context) (jetstream.KeyValueStatus, error)
}) (uint64, error) {
	s, err := bucket.Status(ctx)
	if err != nil {
		return 0, err
	}
	status, ok := s.(*jetstream.KeyValueBucketStatus)
	if !ok || status.StreamInfo() == nil {
		return 0, fmt.Errorf("kv status: unexpected %T without stream info", s)
	}
	created := status.StreamInfo().Created
	if created.IsZero() {
		return 0, errors.New("the backing stream reports no creation time")
	}
	return uint64(created.UnixNano()), nil
}

// startIndexPool builds the keyed pool, and the coalescer when coalesce_ms is above zero,
// after openIndexBuckets. f is the follower whose watermark and damage record the lanes
// report to. poolCtx is handed to every lane's apply; it must stay live until the pool's
// Shutdown has returned, so a write a lane has started finishes whole. The coalescer's run is
// the caller's to start and join.
func (c *Component) startIndexPool(poolCtx context.Context, f *follower.Follower) error {
	c.follower.Store(f)
	c.aliasPredicates = vocabulary.DiscoverAliasPredicates()
	c.namePredicates = vocabulary.DiscoverLabelPredicates()
	pool, err := dispatch.NewKeyedPool(poolCtx, dispatch.KeyedConfig[indexWork]{
		Lanes:      c.config.Workers,
		QueueDepth: max(indexLaneCapacity/c.config.Workers, 1),
		Name:       metricSubsystem,
		Process: func(ctx context.Context, _ int, work indexWork) {
			c.processWork(ctx, work)
		},
		OnPanic: func(work indexWork, _ any) {
			// The repair loop retries the entity. The revision completes, as a failed
			// write's does: readiness is withheld by the failed set, and a revision
			// left pending would pin the watermark after the repair.
			c.markEntityFailed(work.entityID)
			if work.revision > 0 {
				c.follower.Load().Complete(work.entityID, work.revision)
			}
		},
		MetricsRegistry: c.metricsRegistry,
		Logger:          c.logger,
	})
	if err != nil {
		return errs.Wrap(err, "Component", "startIndexPool", "keyed pool construction")
	}
	c.pool = pool
	if c.config.CoalesceMs > 0 {
		c.coalescer = newRevisionCoalescer(time.Duration(c.config.CoalesceMs)*time.Millisecond, c.submitBatch)
	}
	return nil
}

// deliver is the follower's Deliver: it counts the delivery by kind, before any coalescing,
// and routes its key. DataFlow's input is counted where the entity is read, since the
// follower's watch carries no values. A key the shared KV key validator refuses is settled
// here and never routed (settleUnreadable). A delete drops the key's pending coalesced
// revision and goes to the lane at once; the delete's revision is greater, so completing it
// completes the dropped one (revlag).
func (c *Component) deliver(ctx context.Context, e follower.Entry) {
	event := "update"
	if e.Deleted {
		event = "delete"
	}
	c.metrics.watchEvents.WithLabelValues(event).Inc()
	if natsclient.ValidateKVLiteralKey(e.Key) != nil {
		c.settleUnreadable(e)
		return
	}
	work := indexWork{entityID: e.Key, revision: e.Revision}
	if e.Deleted {
		if c.coalescer != nil {
			c.coalescer.Remove(e.Key)
		}
		c.submit(ctx, work)
		return
	}
	if c.coalescer != nil {
		c.coalescer.Add(e.Key, e.Revision)
		return
	}
	c.submit(ctx, work)
}

// settleUnreadable handles a key the shared KV key validator refuses, one longer than it allows
// or with more tokens. Such a key cannot be read back, and no row was ever built under it, so its
// delivered events decide, in the order they arrive: a write records it damaged at the write's
// revision, and a delete or purge, whose delivery has already cleared the record, records
// nothing. Each revision completes here. Were such a write queued in the coalescer or a lane, it
// could be applied after the key's delete and record a key that no longer exists, with no later
// event to clear it.
func (c *Component) settleUnreadable(e follower.Entry) {
	if !e.Deleted {
		c.reportDamaged(e.Key, e.Revision, noncanonicalRefusal(e.Key, validateEntityLiteralKey(e.Key)))
	}
	c.follower.Load().Complete(e.Key, e.Revision)
}

// submitBatch is the coalescer's callback: each key goes to its lane as reconciliation
// work carrying the greatest revision delivered for it.
func (c *Component) submitBatch(ctx context.Context, entities []coalescedEntity) {
	for _, entity := range entities {
		if ctx.Err() != nil {
			return
		}
		c.submit(ctx, indexWork{entityID: entity.entityID, revision: entity.revision})
	}
}

// submit hands work to its key's lane, waiting while the lane is full. It fails only when
// ctx has ended or the pool is shutting down; the revision then stays incomplete.
func (c *Component) submit(ctx context.Context, work indexWork) {
	if err := c.pool.SubmitBlocking(ctx, work.entityID, work); err != nil {
		level := slog.LevelWarn
		if ctx.Err() != nil {
			level = slog.LevelDebug // shutting down
		}
		c.logger.Log(ctx, level, "index work not submitted",
			slog.String("entity", work.entityID), slog.Any("error", err))
	}
}

// processWork is the one mutation entry for deliveries, coalesced batches and repair; the
// pool keeps each key's work in submit order. Each item applies current authoritative
// state, not the event it came from, so a repair that read R2 before the follower
// submitted R1 cannot be undone by R1. From the moment Stop begins it does nothing: the work
// is neither applied nor completed, and the next start's replay re-indexes the entity.
func (c *Component) processWork(ctx context.Context, work indexWork) {
	if c.stopping.Load() {
		return
	}
	if c.reconcileEntity(ctx, work) && work.revision > 0 {
		c.follower.Load().Complete(work.entityID, work.revision)
	}
}

// failedTotal is the follower's Failed: the count of entities whose required writes are
// unresolved.
func (c *Component) failedTotal() int64 {
	return c.failedCount.Load()
}

// reconcileEntity applies entityID's current authoritative state and reports whether the
// revision that led here may complete. work's revision is the delivered one, 0 for a repair pass.
func (c *Component) reconcileEntity(ctx context.Context, work indexWork) bool {
	entityID := work.entityID
	if err := validateEntityLiteralKey(entityID); err != nil {
		c.reconcileNoncanonical(ctx, entityID, err)
		return true
	}
	entry, err := c.entities.Get(ctx, entityID)
	if err != nil {
		if natsclient.IsKVNotFoundError(err) {
			if delErr := c.deleteFromIndexes(ctx, entityID); delErr != nil {
				c.logger.Debug("reconcile: delete still failing",
					slog.String("entity", entityID), slog.Any("error", delErr))
			}
			return true
		}
		c.markEntityFailed(entityID)
		c.logger.Warn("reconcile: authoritative entity read failed; readiness withheld",
			slog.String("entity", entityID), slog.Any("error", err))
		return true
	}
	// One input is one entity read, of the bytes read.
	c.lifecycle.CountInput(len(entry.Value()))
	if err := c.indexEntity(ctx, entityID, entry.Revision(), entry.Value()); err != nil {
		// A refused plan or a failed write still completes: readiness is gated on the failed
		// set, and a revision left pending would pin the watermark for ever (ADR-066 §1).
		c.logger.Warn("entity index failed; readiness withheld until re-index",
			slog.String("entity", entityID), slog.Any("error", err))
	}
	return true
}

// reconcileNoncanonical handles a key that is not a literal entity ID. No row was ever built under
// such a key, so there is nothing to remove and no owner filter is built from it. The key is
// damaged while a value is stored under it, and current state decides that, as for every other
// key: once the key is deleted or purged, its delete, and an older write handled after it, record
// nothing, so the delete clears the record with no restart. Every key here passes the shared KV
// key validator, which gates the read: deliver settles the keys it refuses (settleUnreadable).
func (c *Component) reconcileNoncanonical(ctx context.Context, entityID string, invalid error) {
	entry, err := c.entities.Get(ctx, entityID)
	switch {
	case natsclient.IsKVNotFoundError(err):
		c.clearEntityFailed(entityID)
		return
	case err != nil:
		c.markEntityFailed(entityID)
		c.logger.Warn("reconcile: authoritative entity read failed; readiness withheld",
			slog.String("entity", entityID), slog.Any("error", err))
		return
	}
	c.lifecycle.CountInput(len(entry.Value()))
	c.clearEntityFailed(entityID)
	c.reportDamaged(entityID, entry.Revision(), noncanonicalRefusal(entityID, invalid))
}

// noncanonicalRefusal is the refusal recorded for entityID, a key that is not a literal entity ID;
// invalid is why. Like the decoder's refusal, it names the key in its detail.
func noncanonicalRefusal(entityID string, invalid error) error {
	return errs.ClassifiedCodeDetail(errs.ErrorFatal, graph.ErrorCodeGraphStateResetRequired,
		map[string]any{"entity_ids": []string{entityID}},
		&graph.StateContractError{Reason: graph.GraphStateReasonNoncanonicalEntityID, EntityID: entityID, Err: invalid})
}

// reportDamaged records entityID damaged at revision with refusal, counting one error the first
// time the entity is recorded; the follower's record names it in Health and replies count it.
func (c *Component) reportDamaged(entityID string, revision uint64, refusal error) {
	if c.follower.Load().ReportDamaged(entityID, revision, refusal) {
		c.lifecycle.CountError()
		c.logger.Error("authoritative entity is damaged; left out of the indexes until a valid write or a delete",
			slog.String("entity", entityID), slog.Uint64("revision", revision), slog.Any("error", refusal))
	}
}

// retryIndexWrites runs writeAll up to indexWriteMaxAttempts times, returning nil on the
// first success and the last error otherwise; an ended ctx or a record conflict
// (errRecordConflict) stops it at once.
func retryIndexWrites(ctx context.Context, writeAll func() error) error {
	var writeErr error
	for attempt := range indexWriteMaxAttempts {
		if writeErr = writeAll(); writeErr == nil {
			return nil
		}
		// Another writer put the record since it was read: retrying in place would undo what it
		// wrote, so the entity is failed and the repair pass starts again from the stored record.
		if errors.Is(writeErr, errRecordConflict) {
			return writeErr
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if attempt < indexWriteMaxAttempts-1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt+1) * 25 * time.Millisecond):
			}
		}
	}
	return writeErr
}

// markEntityFailed records that entityID's required index writes did not all succeed.
// failedCount moves only on the first mark, so it is the set's size. ErrorCount moves with
// it, once per entity entering the set: retries within one apply, and repair
// passes that fail again, count nothing more.
func (c *Component) markEntityFailed(entityID string) {
	if _, loaded := c.failedEntities.LoadOrStore(entityID, struct{}{}); !loaded {
		c.failedCount.Add(1)
		c.metrics.writeFailures.Inc()
		c.lifecycle.CountError()
	}
}

// clearEntityFailed removes entityID from the failed set after a clean re-index or delete.
func (c *Component) clearEntityFailed(entityID string) {
	if _, loaded := c.failedEntities.LoadAndDelete(entityID); loaded {
		c.failedCount.Add(-1)
	}
}

// repairLoop re-drives the failed set every indexRepairInterval until ctx ends, so a
// transient KV outage heals without another event for the entity or a restart.
func (c *Component) repairLoop(ctx context.Context) {
	ticker := time.NewTicker(indexRepairInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.repairFailedEntities(ctx)
		}
	}
}

// repairFailedEntities submits every failed entity to its lane, behind any work already
// queued for it. The lane reads the authoritative value only when the work reaches it, so
// repair never writes a stale snapshot over a newer delivery.
func (c *Component) repairFailedEntities(ctx context.Context) {
	if c.failedCount.Load() == 0 {
		return
	}
	c.failedEntities.Range(func(k, _ any) bool {
		if ctx.Err() != nil {
			return false
		}
		c.submit(ctx, indexWork{entityID: k.(string)})
		return true
	})
}

// entityIndexPlan is every row one entity's state asks for, validated before any write.
type entityIndexPlan struct {
	entityID         string
	outgoing         []graph.OutgoingEntry
	predicatesUsed   map[string]bool
	incomingByTarget map[string][]graph.IncomingEntry
	aliasWrites      []aliasIndexWrite
	nameWrites       []nameIndexWrite
}

// buildEntityIndexPlan collects and validates the rows for state without store I/O; every
// required row is validated.
func (c *Component) buildEntityIndexPlan(state graph.EntityState, entityID string) (entityIndexPlan, error) {
	plan := entityIndexPlan{
		entityID:         entityID,
		outgoing:         make([]graph.OutgoingEntry, 0),
		predicatesUsed:   make(map[string]bool),
		incomingByTarget: make(map[string][]graph.IncomingEntry),
	}
	for _, triple := range state.Triples {
		plan.predicatesUsed[triple.Predicate] = true
		if triple.IsRelationship() {
			targetID, _ := triple.Object.(string)
			plan.outgoing = append(plan.outgoing, graph.OutgoingEntry{ToEntityID: targetID, Predicate: triple.Predicate})
			plan.incomingByTarget[targetID] = append(plan.incomingByTarget[targetID],
				graph.IncomingEntry{FromEntityID: entityID, Predicate: triple.Predicate})
		}
		if c.isAliasPredicate(triple.Predicate) {
			// Any bytes: the key holds the alias's hash (ADR-114 §5). A value empty after
			// trimming is not an alias.
			if alias, ok := triple.Object.(string); ok && strings.TrimSpace(alias) != "" {
				plan.aliasWrites = append(plan.aliasWrites, aliasIndexWrite{alias: alias, predicate: triple.Predicate})
			}
		}
		if priority, isName := c.namePredicates[triple.Predicate]; isName {
			if name, ok := triple.Object.(string); ok && name != "" {
				plan.nameWrites = append(plan.nameWrites, nameIndexWrite{
					name: name, predicate: triple.Predicate, priority: priority,
				})
			}
		}
	}

	for _, filter := range []string{
		incomingIndexSourceFilter(entityID), predicateIndexEntityFilter(entityID), nameIndexEntityFilter(entityID),
		aliasIndexEntityFilter(entityID),
	} {
		if err := natsclient.ValidateKVWildcardFilter(filter); err != nil {
			return entityIndexPlan{}, err
		}
	}
	for targetID, entries := range plan.incomingByTarget {
		if err := validateEntityLiteralKey(targetID); err != nil {
			return entityIndexPlan{}, err
		}
		for _, entry := range entries {
			if err := natsclient.ValidateKVLiteralKey(incomingIndexKey(targetID, entityID, entry.Predicate)); err != nil {
				return entityIndexPlan{}, err
			}
		}
	}
	for predicate := range plan.predicatesUsed {
		if _, err := vocabulary.ParsePredicate(predicate); err != nil {
			return entityIndexPlan{}, err
		}
		if err := natsclient.ValidateKVLiteralKey(predicateIndexKey(predicate, entityID)); err != nil {
			return entityIndexPlan{}, err
		}
	}
	for _, write := range plan.nameWrites {
		if !validateNameKeyInputs(entityID, write.predicate, c.logger) {
			return entityIndexPlan{}, errs.WrapInvalid(errs.ErrInvalidData, "Component", "buildEntityIndexPlan",
				"invalid name membership")
		}
		if err := natsclient.ValidateKVLiteralKey(nameCompositeKey(nameIndexKey(write.name), entityID, write.predicate)); err != nil {
			return entityIndexPlan{}, err
		}
	}
	for _, write := range plan.aliasWrites {
		if err := natsclient.ValidateKVLiteralKey(aliasIndexKey(write.alias, entityID, write.predicate)); err != nil {
			return entityIndexPlan{}, err
		}
	}
	for _, entry := range plan.outgoing {
		if _, err := vocabulary.ParsePredicate(entry.Predicate); err != nil {
			return entityIndexPlan{}, err
		}
	}
	return plan, nil
}

// isAliasPredicate reports whether predicate's values go to ALIAS_INDEX.
func (c *Component) isAliasPredicate(predicate string) bool {
	_, ok := c.aliasPredicates[predicate]
	return ok
}

// indexEntity indexes entityID from its stored value data, read at revision. A value
// graph.UnmarshalEntityState refuses is damaged: it is reported first, before the record is read, so
// a record never hides damage, and then every row the entity's earlier value built is removed, so a
// reply that lacks those rows already counts it; a failed removal is a failed write, marked by the
// delete. A value whose planned rows its clean record already holds writes nothing. It returns an
// error for a refused index plan and for required writes that still fail after retry; both mark the
// entity failed and withhold readiness until it re-indexes.
func (c *Component) indexEntity(ctx context.Context, entityID string, revision uint64, data []byte) error {
	var state graph.EntityState
	if err := graph.UnmarshalEntityState(entityID, data, &state); err != nil {
		c.reportDamaged(entityID, revision, err)
		if delErr := c.deleteFromIndexes(ctx, entityID); delErr != nil {
			return fmt.Errorf("remove the rows of damaged entity %q: %w", entityID, delErr)
		}
		return nil
	}
	plan, err := c.buildEntityIndexPlan(state, entityID)
	if err != nil {
		// Held like a failed write, so readiness is withheld rather than the old rows
		// answering. A state the contract accepts always plans, so no
		// stored value is known to reach this.
		c.markEntityFailed(entityID)
		return err
	}
	planned, err := c.plannedRows(plan)
	if err != nil {
		c.markEntityFailed(entityID)
		return err
	}
	unchanged, err := c.writeRows(ctx, entityID, planned)
	if err != nil {
		c.markEntityFailed(entityID)
		return errs.WrapTransient(err, "Component", "indexEntity",
			fmt.Sprintf("index writes failed for %s", entityID))
	}
	c.clearEntityFailed(entityID)
	c.metrics.recordReindex(unchanged)
	c.metrics.eventsProcessed.Inc()
	if unchanged {
		c.logger.Debug("entity rows unchanged", slog.String("entity", entityID))
		return nil
	}
	for _, indexType := range []string{"name", "alias", "predicate", "incoming", "outgoing"} {
		c.metrics.indexUpdates.WithLabelValues(indexType).Inc()
	}
	c.logger.Debug("indexed entity", slog.String("entity", entityID), slog.Int("triples", len(state.Triples)))
	return nil
}

// deleteFromIndexes removes the rows of an entity that is deleted, or whose value is damaged, and
// leaves it a clean empty record, which a replay of the delete finds equal and skips. A failed
// write leaves stale rows the record still names: the entity stays failed and the repair pass runs
// the delete again.
func (c *Component) deleteFromIndexes(ctx context.Context, entityID string) error {
	if err := validateEntityLiteralKey(entityID); err != nil {
		return err
	}
	unchanged, err := c.writeRows(ctx, entityID, nil)
	if err != nil {
		c.markEntityFailed(entityID)
		c.logger.Warn("entity delete from the indexes failed; readiness withheld until it is retried",
			slog.String("entity_id", entityID), slog.Any("error", err))
		return errs.WrapTransient(err, "Component", "deleteFromIndexes",
			fmt.Sprintf("index deletes failed for %s", entityID))
	}
	c.clearEntityFailed(entityID)
	c.metrics.eventsProcessed.Inc()
	if unchanged {
		return nil
	}
	for _, indexType := range []string{"name", "alias", "predicate", "incoming", "outgoing"} {
		c.metrics.indexUpdates.WithLabelValues(indexType).Inc()
	}
	c.logger.Debug("entity deleted from indexes", slog.String("entity_id", entityID))
	return nil
}
