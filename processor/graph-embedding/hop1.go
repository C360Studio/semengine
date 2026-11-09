package graphembedding

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/embedding"
	"github.com/c360studio/semengine/graph/follower"
	"github.com/c360studio/semengine/natsclient"
)

// Hop 1 turns an ENTITY_STATES revision into a pending EMBEDDING_INDEX record, which the
// Worker (hop 2) embeds. A revision completes in the follower when the Worker reports its
// terminal outcome (onTerminal), or at once when hop 1 ends it: no text, deleted, a generated
// record already standing (design D1; ADR-066 § 3). Every hop-1 mutation holds hop1Mu, so the
// follower's deliveries, the coalescer's batches and the repair lane never interleave on one
// entity (pin #629).
//
// Hop 1's own failures (a failed record delete, pending write or entity read) go to the failed
// map before the revision completes, so readiness never reads ready over them, and the repair
// lane retries them; each convergence of an entity clears them (design D3).

// deliver is the follower's Deliver, on its Run goroutine. It counts the entry before any
// coalescing. A delete goes through at once and drops the key's pending coalesced update;
// an update is coalesced when coalesce_ms is above zero and reconciled at once otherwise.
// The entry carries no value, so hop 1 reads the entity's current one.
func (c *Component) deliver(ctx context.Context, e follower.Entry) {
	c.delivered.Store(e.Revision)
	c.lifecycle.CountInput(e.Bytes)
	if e.Deleted {
		if c.coalescer != nil {
			c.coalescer.Remove(e.Key)
		}
		c.applyTombstone(ctx, e.Key, e.Revision)
		return
	}
	if c.coalescer != nil {
		c.coalescer.Add(e.Key)
		return
	}
	c.reconcileEntity(ctx, e.Key)
}

// reconcileBatch is the coalescer's callback, on the coalescer's goroutine under the run
// context: each key converges on the ENTITY_STATES value read now.
func (c *Component) reconcileBatch(ctx context.Context, keys []string) {
	for _, key := range keys {
		if ctx.Err() != nil {
			return
		}
		c.reconcileEntity(ctx, key)
	}
}

// reconcileEntity converges key's pending record on the value ENTITY_STATES holds now, at
// the revision read; completing that revision drains every lower revision of the key the
// watch delivered (revlag.Watermark.Complete). Reading inside hop1Mu means a tombstone delivered
// meanwhile either ran first (the read finds the key absent) or runs after and wins (pin #629).
// A read that finds no value ends only the revisions delivered before it began, so the highest
// delivered revision is taken before the read (#555).
func (c *Component) reconcileEntity(ctx context.Context, key string) {
	c.hop1Mu.Lock()
	defer c.hop1Mu.Unlock()
	delivered := c.delivered.Load()
	entry, err := c.entities.Get(ctx, key)
	if err != nil {
		c.readFailed(ctx, key, delivered, err)
		return
	}
	c.queueEntity(ctx, key, entry.Revision(), entry.Value())
}

// readFailed handles a failed read of key's current value, under hop1Mu. delivered is the
// highest revision the follower had delivered when the read began: the read ends key's revisions
// at or below it and no later one. An absent key converges to no record and completes delivered;
// a failed delete of its record is stranded in the failed map. Any other failure is stranded too
// and completes delivered, so a transient read failure does not pin the floor (pin
// component.go:1628-1670).
//
// The pin completed at ^uint64(0) instead (#555). On the coalesced lane the follower keeps
// delivering while the coalescer's goroutine reads, so that also drained a re-creation of key
// observed during the read, and readiness read caught up while its record waited for hop 2. A
// revision delivered after the read began reaches a batch of its own (or is a tombstone, which
// completes itself), so it stays outstanding until its own outcome. On the inline lane the
// follower is blocked in Deliver for the read, so delivered is the revision being reconciled.
func (c *Component) readFailed(ctx context.Context, key string, delivered uint64, err error) {
	if natsclient.IsKVNotFoundError(err) {
		if delErr := c.storage.DeleteEmbedding(ctx, key); delErr != nil {
			c.logger.Warn("embedding record delete failed for an absent entity; the repair lane retries it",
				slog.String("entity", key), slog.Any("error", delErr))
			c.markFailed(key, strandedRevision, embedding.ReasonDeleteFailed)
		} else {
			c.failed.absent(key)
		}
		c.complete(key, delivered)
		return
	}
	c.logger.Warn("entity read failed; delivered revisions drained, the repair lane retries the read",
		slog.String("entity", key), slog.Any("error", err))
	c.markFailed(key, strandedRevision, embedding.ReasonEntityReadFailed)
	c.complete(key, delivered)
}

// applyTombstone removes a deleted entity's record and completes the tombstone's revision,
// which drains every earlier revision of the key. A failed delete is marked at the
// tombstone's revision and still completes: a revision left pending would pin the floor
// (ADR-066 § 3, pin #624).
func (c *Component) applyTombstone(ctx context.Context, key string, revision uint64) {
	c.hop1Mu.Lock()
	defer c.hop1Mu.Unlock()
	c.removeRecord(ctx, key, revision, "embedding record delete failed for a deleted entity; the repair lane retries it")
}

// removeRecord deletes key's record for revision and completes revision, under hop1Mu. A
// failed delete is marked at revision for the repair lane, and logged as warning.
func (c *Component) removeRecord(ctx context.Context, key string, revision uint64, warning string) {
	if err := c.storage.DeleteEmbedding(ctx, key); err != nil {
		c.logger.Warn(warning, slog.String("entity", key), slog.Any("error", err))
		c.markFailed(key, revision, embedding.ReasonDeleteFailed)
	} else {
		c.failed.ended(key, revision)
	}
	c.complete(key, revision)
}

// queueEntity writes key's pending record for revision, under hop1Mu: the inline lane when
// the entity's body is not offloaded to a registered store, the offloaded lane when it is.
func (c *Component) queueEntity(ctx context.Context, key string, revision uint64, data []byte) {
	var state graph.EntityState
	if err := graph.UnmarshalEntityState(key, data, &state); err != nil {
		// Authoritative state this reader cannot trust is a latched reset, not a terminal
		// skip: the revision stays pending (pin component.go:1795-1806).
		reason := graph.GraphStateReasonUnreadableEntity
		var stateErr *graph.StateContractError
		if errors.As(err, &stateErr) {
			reason = stateErr.Reason
		}
		c.logger.Error("entity state refused; graph reset required",
			slog.String("entity", key), slog.Any("error", err))
		c.lifecycle.CountError() // the reset, as graph-index counts one (design D6)
		c.follower.Load().Reset(reason)
		return
	}

	record := &embedding.Record{EntityID: key, SourceRevision: revision}
	switch {
	case state.StorageRef != nil && c.storeRegistered(state.StorageRef.StorageInstance):
		// The offloaded lane (ADR-063): hop 2 fetches the body and embeds this record's
		// identity text ahead of it; SourceText stays empty.
		record.StorageRef = &embedding.StorageRef{
			StorageInstance: state.StorageRef.StorageInstance,
			Key:             state.StorageRef.Key,
		}
		record.IdentityText = c.extractText(&state)
	default:
		if state.StorageRef != nil {
			c.reportOffloadedBodyExcluded(key, state.StorageRef.StorageInstance)
		}
		record.SourceText = c.extractText(&state)
		if record.SourceText == "" {
			c.removeTextless(ctx, key, revision)
			return
		}
	}

	saved, err := c.storage.SavePendingGuarded(ctx, record)
	if err != nil {
		// No durable record was written, so the revision is not done: it stays pending
		// until the repair lane or a later write of the entity writes one (pin #613 F2).
		c.logger.Error("pending embedding write failed; revision left uncompleted, the repair lane retries it",
			slog.String("entity", key), slog.Any("error", err))
		c.markFailed(key, revision, embedding.ReasonPendingWriteFailed)
		return
	}
	if !saved {
		// A generated record at this revision or a newer one already stands: nothing is
		// left to do for this revision (pin #722 B2).
		c.failed.ended(key, revision)
		c.complete(key, revision)
		return
	}
	// Saved: hop 1 has converged, and the Worker owns the terminal outcome, reported through
	// onTerminal.
	c.failed.queued(key, revision)
}

// removeTextless ends a revision with no text to embed: it removes any record the entity
// held, so search stops returning an earlier vector, and completes the revision. A failed
// delete is marked and still completes (ADR-066 § 3).
func (c *Component) removeTextless(ctx context.Context, key string, revision uint64) {
	c.removeRecord(ctx, key, revision, "embedding record delete failed for an entity with no text; the repair lane retries it")
}

// storeRegistered reports whether instance is a store the component may read offloaded
// bodies from: one the store registry holds by that exact name. Another registered store is
// not equivalent, since StorageInstance names the body's owner (pin #875).
func (c *Component) storeRegistered(instance string) bool {
	if c.storeRegistry == nil {
		return false
	}
	_, ok := c.storeRegistry.Streamable(instance)
	return ok
}

// reportOffloadedBodyExcluded declares a skipped body (pin gh#414): the entity's body is
// offloaded to a store the registry does not hold, so only its inline text is embedded.
// Continuing is safe because the inline text is still the entity's; the counter counts each
// such entity and the warning is logged once, so the log is one actionable line.
func (c *Component) reportOffloadedBodyExcluded(key, instance string) {
	c.metrics.contentUnresolved.Inc()
	c.offloadExcludedWarn.Do(func() {
		c.logger.Warn("offloaded body excluded from embeddings: no store is registered for its "+
			"storage instance; start or restore that storage component (inline text is still embedded)",
			slog.String("entity", key), slog.String("storage_instance", instance))
	})
	c.logger.Debug("offloaded body excluded; embedding inline text only",
		slog.String("entity", key), slog.String("storage_instance", instance))
}

// extractText joins the entity's string values whose predicate ends with one of the
// configured text suffixes, relationships excepted (pin component.go:2062-2090). For an
// offloaded entity it is the identity text: the body is not a triple.
func (c *Component) extractText(state *graph.EntityState) string {
	var parts []string
	for _, triple := range state.Triples {
		if triple.IsRelationship() {
			continue
		}
		predicate := strings.ToLower(triple.Predicate)
		for _, suffix := range c.config.TextSuffixes {
			if strings.HasSuffix(predicate, suffix) {
				if text, ok := triple.Object.(string); ok && text != "" {
					parts = append(parts, text)
				}
				break
			}
		}
	}
	return strings.Join(parts, " ")
}

// onTerminal is the Worker's terminal callback (embedding.TerminalCallback): the pending
// record for sourceRevision reached a terminal outcome, so that revision, with every lower
// one of the entity, is complete. Every outcome completes (ADR-066 § 3); a failure is in the
// failed map first, so readiness never reads ready over it. A failure counts in ErrorCount only
// when the entity enters the failed set, as markFailed's does.
func (c *Component) onTerminal(_ context.Context, entityID string, sourceRevision uint64, outcome embedding.TerminalOutcome, reason string) {
	if outcome == embedding.OutcomeFailed {
		if c.failed.failed(entityID, sourceRevision, reason) {
			c.lifecycle.CountError()
		}
	} else {
		c.failed.succeeded(entityID, sourceRevision)
	}
	c.complete(entityID, sourceRevision)
}

// markFailed records hop 1's own failure of key at revision in the failed map. ErrorCount counts
// once per entity entering the failed set, as graph-index counts (index.go markEntityFailed): a
// failure of an entity already failed, such as a repair pass failing again, and a failure refused
// as older than the entity's latest outcome count nothing. ErrorCount never lowers Healthy (design
// D6; ADR-109 Decision 2): the failed map's count, through the follower's stuck states, is what
// Health reads.
func (c *Component) markFailed(key string, revision uint64, reason string) {
	if c.failed.mark(key, revision, reason) {
		c.lifecycle.CountError()
	}
}

// complete reports revision of key complete to the follower.
func (c *Component) complete(key string, revision uint64) {
	c.follower.Load().Complete(key, revision)
}

// repairInterval is how often the repair lane retries hop 1's failures (pin component.go:1248,
// as graph-index's indexRepairInterval).
const repairInterval = 30 * time.Second

// repairLoop is the repair lane (design D3; pin component.go:1266-1278): every repairInterval
// it retries hop 1's failures, on a goroutine of its own so its reads and writes never delay
// the follower's heartbeat, until ctx, the run context, ends. Retry is flat and unbounded: the
// lane re-drives only failures of the component's own reads and writes, never the Worker's, so
// no entity fails there for good, and while one is failed readiness reads degraded.
func (c *Component) repairLoop(ctx context.Context) {
	ticker := time.NewTicker(repairInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.repair(ctx)
		}
	}
}

// repair is one pass of the repair lane: each entity whose failure is hop 1's own converges on
// the ENTITY_STATES value read now, through the same reconcileEntity as a delivery; a failure
// on the way marks it again for the next pass. The targets are read before hop1Mu is taken
// (lock order hop1Mu, then the map's).
func (c *Component) repair(ctx context.Context) {
	for _, key := range c.failed.repairTargets() {
		if ctx.Err() != nil {
			return
		}
		c.reconcileEntity(ctx, key)
	}
}
