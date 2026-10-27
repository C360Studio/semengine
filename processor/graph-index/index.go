package graphindex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

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

// indexLaneCapacity is the pool's total queue, split across the lanes (design D7).
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

// openIndexBuckets acquires the five index buckets graph-index owns, as their owner,
// and the read handle on ENTITY_STATES the lanes read current state from. Config.Validate
// has already refused any output that is not one of the four declared index buckets;
// NAME_INDEX is internal and not a declared output.
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
	for _, b := range buckets {
		kv, err := kvcatalog.EnsureCatalogBucket(ctx, c.natsClient, b.name)
		if err != nil {
			return errs.Wrap(err, "Component", "openIndexBuckets", "KV bucket: "+b.name)
		}
		*b.store = c.natsClient.NewKVStore(kv)
	}
	entities, err := kvcatalog.OpenCatalogReader(ctx, c.natsClient, graph.BucketEntityStates)
	if err != nil {
		return errs.Wrap(err, "Component", "openIndexBuckets", "KV bucket: "+graph.BucketEntityStates)
	}
	c.entities = entities
	return nil
}

// startIndexPool builds the keyed pool, and the coalescer when coalesce_ms is above zero,
// after openIndexBuckets. f is the follower whose watermark and reset latch the lanes
// report to. poolCtx is handed to every lane's apply; it must stay live until the pool's
// Shutdown has returned, so accepted work is written. The coalescer's run is the caller's
// to start and join.
func (c *Component) startIndexPool(poolCtx context.Context, f *follower.Follower) error {
	c.follower = f
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
			// left pending would pin the watermark after the repair (design D7).
			c.markEntityFailed(work.entityID)
			if work.revision > 0 {
				c.follower.Complete(work.entityID, work.revision)
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

// deliver is the follower's Deliver: it counts the entry, before any coalescing, and
// routes its key. A delete drops the key's pending coalesced revision and goes to the
// lane at once; the delete's revision is greater, so completing it completes the dropped
// one (revlag).
func (c *Component) deliver(ctx context.Context, e follower.Entry) {
	c.lifecycle.CountInput(e.Bytes)
	work := indexWork{entityID: e.Key, revision: e.Revision}
	if e.Deleted {
		c.metrics.watchEvents.WithLabelValues("delete").Inc()
		if c.coalescer != nil {
			c.coalescer.Remove(e.Key)
		}
		c.submit(ctx, work)
		return
	}
	c.metrics.watchEvents.WithLabelValues("update").Inc()
	if c.coalescer != nil {
		c.coalescer.Add(e.Key, e.Revision)
		return
	}
	c.submit(ctx, work)
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
// submitted R1 cannot be undone by R1.
func (c *Component) processWork(ctx context.Context, work indexWork) {
	if c.reconcileEntity(ctx, work.entityID) && work.revision > 0 {
		c.follower.Complete(work.entityID, work.revision)
	}
}

// failedTotal is the follower's Failed: the count of entities whose required writes are
// unresolved.
func (c *Component) failedTotal() int64 {
	return c.failedCount.Load()
}

// reconcileEntity applies entityID's current authoritative state and reports whether the
// revision that led here may complete.
func (c *Component) reconcileEntity(ctx context.Context, entityID string) bool {
	if err := validateEntityLiteralKey(entityID); err != nil {
		c.follower.Reset(graph.GraphStateReasonNoncanonicalEntityID)
		c.markEntityFailed(entityID)
		c.logger.Error("authoritative entity key is noncanonical; graph reset required",
			slog.String("entity", entityID), slog.Any("error", err))
		// No owner filter is safe for this key, so it can never be applied: keep the
		// revision pending, so the watermark never says it was processed.
		return false
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
	if err := c.indexEntity(ctx, entityID, entry.Value()); err != nil {
		// A failed write still completes: readiness is gated on the failed set, and a
		// revision left pending would pin the watermark for ever (ADR-066 §1).
		c.logger.Warn("entity index write failed; readiness withheld until re-index",
			slog.String("entity", entityID), slog.Any("error", err))
		return !errors.Is(err, errAuthoritativeIdentityMismatch)
	}
	return true
}

// retryIndexWrites runs writeAll up to indexWriteMaxAttempts times, returning nil on the
// first success and the last error otherwise; an ended ctx stops it at once.
func retryIndexWrites(ctx context.Context, writeAll func() error) error {
	var writeErr error
	for attempt := range indexWriteMaxAttempts {
		if writeErr = writeAll(); writeErr == nil {
			return nil
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
// failedCount moves only on the first mark, so it is the set's size.
func (c *Component) markEntityFailed(entityID string) {
	if _, loaded := c.failedEntities.LoadOrStore(entityID, struct{}{}); !loaded {
		c.failedCount.Add(1)
		c.metrics.writeFailures.Inc()
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
	aliases          []string
	nameWrites       []nameIndexWrite
}

// buildEntityIndexPlan collects and validates the rows for state without store I/O. An
// alias that is not a KV-safe key is skipped and logged; every required row is validated.
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
			if alias, ok := triple.Object.(string); ok && alias != "" {
				if err := natsclient.ValidateKVLiteralKey(alias); err != nil {
					c.logger.Warn("alias index: KV-unsafe alias skipped",
						slog.String("entity_id", entityID), slog.String("predicate", triple.Predicate),
						slog.Any("error", err))
					continue
				}
				plan.aliases = append(plan.aliases, alias)
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
	for _, entry := range plan.outgoing {
		if _, err := vocabulary.ParsePredicate(entry.Predicate); err != nil {
			return entityIndexPlan{}, err
		}
	}
	return plan, nil
}

// isAliasPredicate reports whether predicate's values go to ALIAS_INDEX.
func (c *Component) isAliasPredicate(predicate string) bool {
	_, isVocabAlias := c.aliasPredicates[predicate]
	return isVocabAlias || predicate == "core.identity.alias"
}

// applyEntityIndexPlan writes plan to every index, returning every failure joined.
func (c *Component) applyEntityIndexPlan(ctx context.Context, plan entityIndexPlan) error {
	var errList []error
	if err := c.reconcileIncomingIndex(ctx, plan.entityID, plan.incomingByTarget); err != nil {
		errList = append(errList, fmt.Errorf("incoming: %w", err))
	}
	if err := c.writeOutgoingIndex(ctx, plan.entityID, plan.outgoing); err != nil {
		errList = append(errList, fmt.Errorf("outgoing: %w", err))
	}
	if err := c.reconcilePredicateIndex(ctx, plan.entityID, plan.predicatesUsed); err != nil {
		errList = append(errList, fmt.Errorf("predicate: %w", err))
	}
	// ALIAS_INDEX has no owner axis and is outside replacement: a dropped alias keeps
	// resolving until #415 lands (design D12, owner ruling PR #414 comment 6426254115).
	for _, alias := range plan.aliases {
		if err := c.updateAliasIndex(ctx, alias, plan.entityID); err != nil {
			errList = append(errList, fmt.Errorf("alias[%s]: %w", alias, err))
		}
	}
	if err := c.reconcileNameIndex(ctx, plan.entityID, plan.nameWrites); err != nil {
		errList = append(errList, fmt.Errorf("name: %w", err))
	}
	return errors.Join(errList...)
}

// indexEntity indexes entityID from its stored value data. It returns an error for
// incompatible authoritative state, which latches reset, and when required writes still
// fail after retry, which marks the entity failed and withholds readiness until it
// re-indexes.
func (c *Component) indexEntity(ctx context.Context, entityID string, data []byte) error {
	var state graph.EntityState
	if err := graph.UnmarshalEntityState(entityID, data, &state); err != nil {
		reason := graph.GraphStateReasonUnreadableEntity
		var stateErr *graph.StateContractError
		if errors.As(err, &stateErr) {
			reason = stateErr.Reason
		}
		c.follower.Reset(reason)
		if reason == graph.GraphStateReasonNoncanonicalEntityID {
			c.markEntityFailed(entityID)
			return fmt.Errorf("%w: %w", errAuthoritativeIdentityMismatch, err)
		}
		return fmt.Errorf("ENTITY_STATES %q is incompatible; graph reset and canonical reingest required: %w",
			entityID, err)
	}
	plan, err := c.buildEntityIndexPlan(state, entityID)
	if err != nil {
		return err
	}

	// Re-index instrumentation, observe only: the projection is compared with the last
	// one indexed, and stored only after the writes succeed, so a failed write never
	// becomes the baseline (#108 measures it, design D13).
	projection := computeIndexProjection(state, c.namePredicates, c.isAliasPredicate)
	prev, loaded := c.lastProjections.Load(entityID)
	c.metrics.recordReindex(loaded && prev.(string) == projection)

	if err := retryIndexWrites(ctx, func() error { return c.applyEntityIndexPlan(ctx, plan) }); err != nil {
		c.markEntityFailed(entityID)
		return errs.WrapTransient(err, "Component", "indexEntity",
			fmt.Sprintf("index writes failed for %s after %d attempts", entityID, indexWriteMaxAttempts))
	}
	c.clearEntityFailed(entityID)
	c.lastProjections.Store(entityID, projection)
	for _, indexType := range []string{"name", "predicate", "incoming", "outgoing"} {
		c.metrics.indexUpdates.WithLabelValues(indexType).Inc()
	}
	c.metrics.eventsProcessed.Inc()
	c.logger.Debug("indexed entity", slog.String("entity", entityID), slog.Int("triples", len(state.Triples)))
	return nil
}

// computeIndexProjection is a canonical, sorted string of everything the indexes read
// from state: relationship (predicate, target) pairs, the distinct predicates, (name
// predicate, name) pairs and (alias predicate, alias) pairs. Two states with the same
// projection produce the same index writes.
func computeIndexProjection(state graph.EntityState, namePredicates map[string]int, isAlias func(string) bool) string {
	parts := make([]string, 0, len(state.Triples)*3)
	predicateSeen := make(map[string]bool, len(state.Triples))
	for _, t := range state.Triples {
		if t.IsRelationship() {
			if targetID, ok := t.Object.(string); ok {
				parts = append(parts, "rel:"+t.Predicate+":"+targetID)
			}
		}
		if !predicateSeen[t.Predicate] {
			predicateSeen[t.Predicate] = true
			parts = append(parts, "pred:"+t.Predicate)
		}
		if _, isName := namePredicates[t.Predicate]; isName {
			if name, ok := t.Object.(string); ok && name != "" {
				parts = append(parts, "name:"+t.Predicate+":"+name)
			}
		}
		if isAlias(t.Predicate) {
			if alias, ok := t.Object.(string); ok && alias != "" {
				parts = append(parts, "alias:"+t.Predicate+":"+alias)
			}
		}
	}
	slices.Sort(parts)
	return strings.Join(parts, "|")
}

// writeOutgoingIndex stores entityID's complete outgoing list under its own key; a Put of
// the whole list replaces the previous one.
func (c *Component) writeOutgoingIndex(ctx context.Context, entityID string, entries []graph.OutgoingEntry) error {
	if err := validateEntityLiteralKey(entityID); err != nil {
		return err
	}
	for _, entry := range entries {
		if err := validateEntityLiteralKey(entry.ToEntityID); err != nil {
			return err
		}
	}
	data, err := json.Marshal(entries)
	if err != nil {
		c.lifecycle.CountError()
		return errs.Wrap(err, "Component", "writeOutgoingIndex", "entry serialization")
	}
	_, putErr := c.outgoingBucket.Put(ctx, entityID, data)
	c.metrics.kvOperations.WithLabelValues("put", "outgoing").Inc()
	if putErr != nil {
		c.lifecycle.CountError()
		return errs.Wrap(putErr, "Component", "writeOutgoingIndex", "KV store")
	}
	return nil
}

// updateAliasIndex maps alias to entityID in ALIAS_INDEX.
func (c *Component) updateAliasIndex(ctx context.Context, alias, entityID string) error {
	if err := validateEntityLiteralKey(entityID); err != nil {
		return err
	}
	if err := natsclient.ValidateKVLiteralKey(alias); err != nil {
		return err
	}
	_, err := c.aliasBucket.Put(ctx, alias, []byte(entityID))
	c.metrics.kvOperations.WithLabelValues("put", "alias").Inc()
	if err != nil {
		c.lifecycle.CountError()
		return errs.Wrap(err, "Component", "updateAliasIndex", "KV store")
	}
	c.metrics.indexUpdates.WithLabelValues("alias").Inc()
	return nil
}

// deleteFromIndexes removes a deleted entity's rows. PREDICATE, NAME and OUTGOING rows are
// owned by the entity. INCOMING rows are owned by their source, though their key begins
// with the target, so the delete lists by the source filter and keeps edges from live
// sources that still point at the deleted entity. ALIAS has no owner axis and is left
// (design D12, #415).
func (c *Component) deleteFromIndexes(ctx context.Context, entityID string) error {
	// Validate the owner and its filters before the first list or delete.
	if err := validateEntityLiteralKey(entityID); err != nil {
		return err
	}
	type deleteSet struct {
		name   string
		bucket *natsclient.KVStore
		filter string
		keys   []string
	}
	deleteSets := []deleteSet{
		{name: "incoming", bucket: c.incomingBucket, filter: incomingIndexSourceFilter(entityID)},
		{name: "predicate", bucket: c.predicateBucket, filter: predicateIndexEntityFilter(entityID)},
		{name: "name", bucket: c.nameBucket, filter: nameIndexEntityFilter(entityID)},
	}
	for _, set := range deleteSets {
		if err := natsclient.ValidateKVWildcardFilter(set.filter); err != nil {
			return err
		}
	}

	// List and validate every owned key before the first delete, so a failed list or a
	// poisoned key never leaves one family retracted and another not; a retry starts from
	// the same complete plan.
	preflightFailures := 0
	for i := range deleteSets {
		set := &deleteSets[i]
		keys, listErr := set.bucket.KeysByFilter(ctx, set.filter)
		c.metrics.kvOperations.WithLabelValues("list", set.name).Inc()
		c.metrics.recordReconcileOperation(set.name, "list", listErr)
		if listErr != nil {
			c.lifecycle.CountError()
			preflightFailures++
			c.logger.Warn("failed to list owned index rows for delete",
				slog.String("index", set.name), slog.String("entity_id", entityID), slog.Any("error", listErr))
			continue
		}
		slices.Sort(keys)
		set.keys = slices.Compact(keys)
		for _, key := range set.keys {
			if keyErr := natsclient.ValidateKVLiteralKey(key); keyErr != nil {
				preflightFailures++
				c.logger.Warn("invalid owned index key returned for delete",
					slog.String("index", set.name), slog.String("key", key), slog.Any("error", keyErr))
			}
		}
	}
	if preflightFailures > 0 {
		c.markEntityFailed(entityID)
		return errs.WrapTransient(errIndexWritePartial, "Component", "deleteFromIndexes",
			fmt.Sprintf("%d delete-plan operations failed for %s (no rows were deleted)", preflightFailures, entityID))
	}

	failures := 0
	outgoingErr := c.outgoingBucket.Delete(ctx, entityID)
	c.metrics.kvOperations.WithLabelValues("delete", "outgoing").Inc()
	if outgoingErr != nil && !natsclient.IsKVNotFoundError(outgoingErr) {
		c.lifecycle.CountError()
		failures++
		c.logger.Warn("failed to delete from outgoing index",
			slog.String("entity_id", entityID), slog.Any("error", outgoingErr))
	}
	for _, set := range deleteSets {
		for _, key := range set.keys {
			delErr := set.bucket.Delete(ctx, key)
			if natsclient.IsKVNotFoundError(delErr) {
				delErr = nil
			}
			c.metrics.kvOperations.WithLabelValues("delete", set.name).Inc()
			c.metrics.recordReconcileOperation(set.name, "delete", delErr)
			if delErr != nil {
				c.lifecycle.CountError()
				failures++
				c.logger.Warn("failed to delete owned index row",
					slog.String("index", set.name), slog.String("key", key), slog.Any("error", delErr))
			}
		}
	}
	// A failed list or delete may leave stale rows: the index is known incomplete, so the
	// entity stays failed and the repair loop runs the delete again.
	if failures > 0 {
		c.markEntityFailed(entityID)
		return errs.WrapTransient(errIndexWritePartial, "Component", "deleteFromIndexes",
			fmt.Sprintf("%d delete/list operations failed for %s (stale index rows may remain)", failures, entityID))
	}
	// A clean delete evicts the projection baseline, so a re-add is not read as unchanged.
	c.lastProjections.Delete(entityID)
	c.clearEntityFailed(entityID)
	for _, indexType := range []string{"name", "predicate", "incoming", "outgoing"} {
		c.metrics.indexUpdates.WithLabelValues(indexType).Inc()
	}
	c.metrics.eventsProcessed.Inc()
	c.logger.Debug("entity deleted from indexes", slog.String("entity_id", entityID))
	return nil
}
