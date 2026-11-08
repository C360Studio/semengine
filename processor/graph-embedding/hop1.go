package graphembedding

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/embedding"
	"github.com/c360studio/semengine/graph/follower"
	"github.com/c360studio/semengine/natsclient"
)

// Hop 1 turns an ENTITY_STATES revision into a pending EMBEDDING_INDEX record, which the
// Worker (hop 2) embeds. A revision completes in the follower when the Worker reports its
// terminal outcome (onTerminal), or at once when hop 1 ends it: no text, deleted, a generated
// record already standing (design D1; ADR-066 § 3). Every hop-1 mutation holds hop1Mu, so the
// follower's deliveries and the coalescer's batches never interleave on one entity (pin #629).
//
// shortcut: no failed map yet. Where the pin marks or clears a stranded entity (a failed
// delete, a failed pending write, a failed entity read), task 2.4 of
// setup-04a-05-first-consumer-graph-embedding adds the mark; until then such a failure only
// logs, and a failed pending write leaves its revision uncompleted.

// deliver is the follower's Deliver, on its Run goroutine. It counts the entry before any
// coalescing. A delete goes through at once and drops the key's pending coalesced update;
// an update is coalesced when coalesce_ms is above zero and reconciled at once otherwise.
// The entry carries no value, so hop 1 reads the entity's current one.
func (c *Component) deliver(ctx context.Context, e follower.Entry) {
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
// watch delivered (revlag.Watermark.Complete). Reading inside hop1Mu means a tombstone delivered meanwhile either ran first (the read
// finds the key absent) or runs after and wins (pin #629).
func (c *Component) reconcileEntity(ctx context.Context, key string) {
	c.hop1Mu.Lock()
	defer c.hop1Mu.Unlock()
	entry, err := c.entities.Get(ctx, key)
	if err != nil {
		c.readFailed(ctx, key, err)
		return
	}
	c.queueEntity(ctx, key, entry.Revision(), entry.Value())
}

// readFailed handles a failed read of key's current value, under hop1Mu. An absent key
// converges to no record and drains every revision of it (^uint64(0): the delivered
// revisions are not in hand). Any other failure drains too, so a transient read failure
// does not pin the floor (pin component.go:1628-1670).
func (c *Component) readFailed(ctx context.Context, key string, err error) {
	if natsclient.IsKVNotFoundError(err) {
		if delErr := c.storage.DeleteEmbedding(ctx, key); delErr != nil {
			c.logger.Warn("embedding record delete failed for an absent entity",
				slog.String("entity", key), slog.Any("error", delErr))
		}
		c.complete(key, ^uint64(0))
		return
	}
	c.logger.Warn("entity read failed; revision drained", slog.String("entity", key), slog.Any("error", err))
	c.complete(key, ^uint64(0))
}

// applyTombstone removes a deleted entity's record and completes the tombstone's revision,
// which drains every earlier revision of the key. A failed delete still completes: a
// revision left pending would pin the floor (ADR-066 § 3, pin #624).
func (c *Component) applyTombstone(ctx context.Context, key string, revision uint64) {
	c.hop1Mu.Lock()
	defer c.hop1Mu.Unlock()
	if err := c.storage.DeleteEmbedding(ctx, key); err != nil {
		c.logger.Warn("embedding record delete failed for a deleted entity",
			slog.String("entity", key), slog.Any("error", err))
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
		// until a later write of the entity is delivered (pin #613 F2).
		c.logger.Error("pending embedding write failed; revision left uncompleted",
			slog.String("entity", key), slog.Any("error", err))
		return
	}
	if !saved {
		// A generated record at this revision or a newer one already stands: nothing is
		// left to do for this revision (pin #722 B2).
		c.complete(key, revision)
	}
	// Saved: the Worker owns the terminal outcome, reported through onTerminal.
}

// removeTextless ends a revision with no text to embed: it removes any record the entity
// held, so search stops returning an earlier vector, and completes the revision. A failed
// delete still completes (ADR-066 § 3).
func (c *Component) removeTextless(ctx context.Context, key string, revision uint64) {
	if err := c.storage.DeleteEmbedding(ctx, key); err != nil {
		c.logger.Warn("embedding record delete failed for an entity with no text",
			slog.String("entity", key), slog.Any("error", err))
	}
	c.complete(key, revision)
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
// one of the entity, is complete. Every outcome completes (ADR-066 § 3).
//
// shortcut: called by tests only until task 2.3 of setup-04a-05-first-consumer-graph-embedding
// hands it to the Worker (WithOnTerminal); task 2.4 routes outcome and reason to the failed map.
func (c *Component) onTerminal(_ context.Context, entityID string, sourceRevision uint64, _ embedding.TerminalOutcome, _ string) {
	c.complete(entityID, sourceRevision)
}

// complete reports revision of key complete to the follower.
func (c *Component) complete(key string, revision uint64) {
	c.follower.Load().Complete(key, revision)
}
