package graphingest

// poison_inventory.go — the per-entity poison inventory (poison-response-scoping
// D2/D3/D6). The inventory replaces the surface-global sticky latch: it is
// OBSERVABILITY-ONLY — no read or write path consults it for a decision.
// Refusal derives solely from decoding the bytes actually stored at each seam;
// the inventory exists so operators get an alertable, enumerable, self-healing
// signal (Health degraded + gauge + DebugStatus) instead of a whole-surface
// outage.
//
// Each record carries the KV revision whose decode failed. Clear paths (D3):
//
//	(a) DeleteEntity — the poisoned bytes are gone, up to the revision the
//	    delete removed;
//	(b) a successful commit to the key at a revision newer than the recorded
//	    one — the revision guard closes the record-after-clear interleaving
//	    (concurrent repair Put vs in-flight RMW classification);
//	(c) any successful validating read of the key — the read already ran the
//	    canonical decode, so an out-of-band repair recovers Health for free.
//
// Steady-state cost with an empty inventory is a single atomic load on every
// commit/read hot path (design D2, required).

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus"
)

// entityPoisonSampleCap bounds entity-ID lists in operator-facing messages
// (the Health message sample and aggregate-read failures). Full enumeration is
// available via DebugStatus. Proposed constant per the change's open questions.
const entityPoisonSampleCap = 10

// bootSweepErrorLogCap bounds individually-logged boot-sweep poison ERRORs;
// past it a single count-only WARN summarizes the remainder so a mass-poison
// event cannot flood the log. Proposed constant per the change's open questions.
const bootSweepErrorLogCap = 100

// entityPoisonRecord is one inventoried poisoned entity: the stamped typed
// error plus the KV revision whose decode failed.
type entityPoisonRecord struct {
	contractErr *graph.StateContractError
	revision    uint64
}

// newPoisonedEntitiesMetric builds the poison-inventory gauge.
// One gauge, never per-entity labels (cardinality discipline, design D6):
// enumeration belongs to DebugStatus, alerting belongs to this gauge plus the
// Health message.
func newPoisonedEntitiesMetric() prometheus.Gauge {
	poisonedEntitiesGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "semengine",
		Subsystem: "graph_ingest",
		Name:      "poisoned_entities",
		Help:      "Current per-entity poison inventory size (ENTITY_STATES values failing the canonical decode). Alert on this gauge + the Health message; enumerate via the component debug surface.",
	})
	return poisonedEntitiesGauge
}

// setEntityPoisonGauge publishes the inventory size to the gauge. Callers hold
// entityPoisonMu (len is read under the lock).
func (c *Component) setEntityPoisonGauge() {
	c.poisonedEntities.Set(float64(len(c.entityPoison)))
}

// recordEntityPoison inserts or refreshes an inventory record keyed by the
// stamped EntityID. Returns true when the entity was NEWLY inventoried (map
// presence is the once-per-entity log dedup). A re-record of an already
// inventoried entity keeps the newest failing revision and does not re-log.
// On a NEW record the entity's query-cache entry is invalidated (hygiene,
// mirroring the write-path invalidations — cached responses must not outlive
// detection; this is not an enforcement use of the inventory).
func (c *Component) recordEntityPoison(contractErr *graph.StateContractError, revision uint64) bool {
	if contractErr == nil || contractErr.EntityID == "" {
		return false // unstampable — enforcement still holds at every per-read decode
	}
	id := contractErr.EntityID

	c.entityPoisonMu.Lock()
	if c.entityPoison == nil {
		c.entityPoison = make(map[string]entityPoisonRecord)
	}
	existing, present := c.entityPoison[id]
	if present {
		if revision >= existing.revision {
			c.entityPoison[id] = entityPoisonRecord{contractErr: contractErr, revision: revision}
		}
		c.entityPoisonMu.Unlock()
		return false
	}
	c.entityPoison[id] = entityPoisonRecord{contractErr: contractErr, revision: revision}
	c.entityPoisonSize.Store(int64(len(c.entityPoison)))
	c.setEntityPoisonGauge()
	c.entityPoisonMu.Unlock()

	c.invalidateEntityCacheEntry(id)
	return true
}

// logEntityPoisonRecorded emits the once-per-entity structured ERROR for a new
// inventory record. Kept separate from recordEntityPoison so the boot sweep
// can apply its own log cap.
func (c *Component) logEntityPoisonRecorded(contractErr *graph.StateContractError, revision uint64) {
	c.logger.Error("poisoned entity inventoried; per-entity refusal until repaired (delete + recreate)",
		slog.String("code", graph.ErrorCodeGraphStateResetRequired),
		slog.String("entity_id", contractErr.EntityID),
		slog.String("reason", string(contractErr.Reason)),
		slog.Uint64("revision", revision))
}

// inventoryEntityPoison is the standard record path for detection sites that
// hold the failing entry's revision (query lanes, mutation read seams, and the
// write seam's callbacks, at the revision they read: design D23). It
// records, logs once on a new record, and then verifies the record against the
// key's CURRENT bytes — closing the last record/clear interleaving: a repair
// (delete or commit) that completed entirely between this detection's read and
// its record found no entry to clear, so without the verify the fresh record
// would strand a spurious degraded-Health entry until the key's next touch.
func (c *Component) inventoryEntityPoison(ctx context.Context, contractErr *graph.StateContractError, revision uint64) {
	if c.recordEntityPoison(contractErr, revision) {
		c.logEntityPoisonRecorded(contractErr, revision)
		c.verifyEntityPoisonRecord(ctx, contractErr.EntityID)
	}
}

// verifyEntityPoisonRecord re-reads a just-recorded key and drops the record
// if the CURRENT bytes prove repair: absent key (repaired by delete) or bytes
// that pass readEntity's rule, decodeStoredEntity (repaired by commit or
// out-of-band overwrite, at that revision — the revision guard keeps a
// re-poison newer than the proof). An absent key proves repair only up to the
// revision recorded before the read: a newer poisoned revision recorded while
// the read was in flight stays (checkpoint 3 B1). A transient KV read error
// keeps the record; it self-heals on the key's next touch (D3c). One extra KV
// Get, only on the already-failing detection path.
func (c *Component) verifyEntityPoisonRecord(ctx context.Context, entityID string) {
	if c.entityBucket == nil {
		return
	}
	c.entityPoisonMu.Lock()
	recorded, ok := c.entityPoison[entityID]
	c.entityPoisonMu.Unlock()
	if !ok {
		return // already cleared; a later record verifies itself
	}
	entry, err := c.entityBucket.Get(ctx, entityID)
	if err != nil {
		if natsclient.IsKVNotFoundError(err) {
			c.dropEntityPoisonRecord(entityID, recorded.revision) // bytes are gone — repaired by delete
		}
		return // transient read error: keep the record; next touch resolves it
	}
	if _, err := decodeStoredEntity(entityID, entry.Value); err == nil {
		c.dropEntityPoisonRecord(entityID, entry.Revision)
	}
}

// dropEntityPoisonRecord removes an inventory entry. proofRev is the revision
// up to which the key is PROVEN repaired: valid at it (a committed write or a
// validating read), or gone with it (a delete, or an absent key read after the
// record at it). The entry is kept when its recorded poisoned revision is
// newer than the proof — a re-poison after the proof must not be erased. A
// revision's bytes are immutable, so proofRev == recorded means the recorded
// revision is repaired and the entry is stale. Every clear goes through this
// guard (checkpoint 3 B1).
func (c *Component) dropEntityPoisonRecord(entityID string, proofRev uint64) {
	c.entityPoisonMu.Lock()
	defer c.entityPoisonMu.Unlock()
	rec, ok := c.entityPoison[entityID]
	if !ok {
		return
	}
	if rec.revision > proofRev {
		return
	}
	delete(c.entityPoison, entityID)
	c.entityPoisonSize.Store(int64(len(c.entityPoison)))
	c.setEntityPoisonGauge()
}

// clearEntityPoisonOnDelete is clear path (a): a successful delete at
// deletedRev removed the poisoned bytes up to that revision. A record newer
// than deletedRev is a re-poison after the delete and stays. Steady-state cost
// is the single atomic load.
func (c *Component) clearEntityPoisonOnDelete(entityID string, deletedRev uint64) {
	if c.entityPoisonSize.Load() == 0 {
		return
	}
	c.dropEntityPoisonRecord(entityID, deletedRev)
}

// clearEntityPoisonOnCommit is clear path (b): a write-gate-validated commit
// to the key proves repair when its revision is newer than the recorded one.
// committedRev is the revision that commit produced, as the write returned it.
// The healthy hot path exits on the first atomic load (design D2's required
// fast path).
func (c *Component) clearEntityPoisonOnCommit(_ context.Context, entityID string, committedRev uint64) {
	if c.entityPoisonSize.Load() == 0 {
		return // steady-state: one atomic load, mutex untouched
	}
	c.dropEntityPoisonRecord(entityID, committedRev)
}

// clearEntityPoisonOnValidRead is clear path (c): any read that successfully
// validated the key's current bytes proves any recorded poison at or below
// that revision is stale. The out-of-band repair (bytes replaced outside the
// mutation API) recovers Health through this path without a restart.
func (c *Component) clearEntityPoisonOnValidRead(entityID string, readRev uint64) {
	if c.entityPoisonSize.Load() == 0 {
		return // steady-state: one atomic load, mutex untouched
	}
	c.dropEntityPoisonRecord(entityID, readRev)
}

// entityPoisonHealthMessage renders the bounded operator summary for Health:
// count + first-N sorted entity IDs with their bounded reasons. Full
// enumeration lives in DebugStatus.
func (c *Component) entityPoisonHealthMessage() string {
	c.entityPoisonMu.Lock()
	total := len(c.entityPoison)
	ids := make([]string, 0, total)
	for id := range c.entityPoison {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	sample := ids
	if len(sample) > entityPoisonSampleCap {
		sample = sample[:entityPoisonSampleCap]
	}
	parts := make([]string, 0, len(sample))
	for _, id := range sample {
		parts = append(parts, fmt.Sprintf("%s (%s)", id, c.entityPoison[id].contractErr.Reason))
	}
	c.entityPoisonMu.Unlock()

	noun := "entities"
	if total == 1 {
		noun = "entity"
	}
	msg := fmt.Sprintf("%s: %d poisoned %s (per-entity refusal; repair = delete + recreate): %s",
		graph.ErrorCodeGraphStateResetRequired, total, noun, strings.Join(parts, ", "))
	if total > len(sample) {
		msg += fmt.Sprintf(" (+%d more; enumerate via debug status)", total-len(sample))
	}
	return msg
}

// entityPoisonDebugEntry is one enumerated inventory record for the component
// debug surface — JSON-serializable per DebugStatusProvider.
type entityPoisonDebugEntry struct {
	EntityID string `json:"entity_id"`
	Reason   string `json:"reason"`
	Revision uint64 `json:"revision"`
	Error    string `json:"error"`
}

// entityPoisonDebugStatus is the DebugStatus payload shape.
type entityPoisonDebugStatus struct {
	PoisonedEntityCount int                      `json:"poisoned_entity_count"`
	PoisonedEntities    []entityPoisonDebugEntry `json:"poisoned_entities"`
}

// DebugStatus implements component.DebugStatusProvider: the FULL poison
// inventory, sorted by entity ID, so a mass-poison event is enumerable in-band
// (design D6 — the Health message and gauge are bounded; this is not).
func (c *Component) DebugStatus() any {
	c.entityPoisonMu.Lock()
	defer c.entityPoisonMu.Unlock()

	entries := make([]entityPoisonDebugEntry, 0, len(c.entityPoison))
	for id, rec := range c.entityPoison {
		entries = append(entries, entityPoisonDebugEntry{
			EntityID: id,
			Reason:   string(rec.contractErr.Reason),
			Revision: rec.revision,
			Error:    rec.contractErr.Error(),
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].EntityID < entries[j].EntityID })
	return entityPoisonDebugStatus{
		PoisonedEntityCount: len(entries),
		PoisonedEntities:    entries,
	}
}

// startEntityStateGuard synchronously validates the resident ENTITY_STATES
// snapshot into the per-entity poison inventory, then STOPS the watcher —
// graph-ingest holds no steady-state self-watch on the bucket it writes
// (steady-state detection rides the read points that already exist). A sweep
// that cannot reach the end of the snapshot returns an error, which fails Start.
//
// Snapshot completeness leans on ENTITY_STATES History=1: with a per-subject
// limit of 1 the pre-marker replay carries exactly the latest revision per
// key, so gap-resets cannot inflate the received set and the nil
// end-of-snapshot marker means the full resident state was seen. Raising the
// bucket's history depth invalidates this marker math.
func (c *Component) startEntityStateGuard(ctx context.Context, bucket *natsclient.KVStore) error {
	watcher, err := bucket.Watch(ctx, ">")
	if err != nil {
		return errs.WrapTransient(err, "Component", "startEntityStateGuard", "open ENTITY_STATES snapshot watcher")
	}

	// Drain bookkeeping is LAST-REVISION-WINS per key: a poisoned revision
	// superseded by a valid (or tombstoned) pre-marker revision of the same
	// key ends with no inventory entry.
	sweep := make(map[string]entityPoisonRecord)
	updates := watcher.Updates()
	for {
		select {
		case <-ctx.Done():
			c.stopEntityStateGuardWatcher(watcher, updates)
			return errs.WrapTransient(ctx.Err(), "Component", "startEntityStateGuard", "ENTITY_STATES snapshot sweep")
		case entry, ok := <-updates:
			if !ok {
				// The channel closed before the end-of-snapshot marker, so the
				// snapshot was not seen in full; no poison is recorded from it.
				_ = watcher.Stop()
				cause := ctx.Err()
				if cause == nil {
					cause = errors.New("watch closed before the end of the snapshot")
				}
				return errs.WrapTransient(cause, "Component", "startEntityStateGuard", "ENTITY_STATES snapshot sweep")
			}
			if entry == nil {
				// End-of-snapshot marker: the full resident snapshot was seen.
				// Stop deliberately, then record the surviving sweep entries.
				c.stopEntityStateGuardWatcher(watcher, updates)
				c.recordBootSweepPoison(ctx, sweep)
				return nil
			}
			sweepEntityStateGuardEntry(sweep, entry)
		}
	}
}

// stopEntityStateGuardWatcher performs the deliberate snapshot-watcher stop
// (design D1 mandatory shape): Stop(), then KEEP READING the updates channel
// until nats.go closes it, discarding entries. The nats.go update callback
// blocks on a full channel while holding the watcher mutex, so an unread
// channel would wedge the connection's async-callback dispatcher (via
// SetClosedHandler). The post-Stop closure here is deliberate and is not a sweep
// failure.
func (c *Component) stopEntityStateGuardWatcher(watcher jetstream.KeyWatcher, updates <-chan jetstream.KeyValueEntry) {
	_ = watcher.Stop()
	discarded := 0
	for range updates {
		discarded++ // deliberate drain-to-close; entries are discarded unvalidated
	}
	if discarded > 0 {
		c.logger.Debug("snapshot sweep watcher drained to close",
			slog.Int("discarded_entries", discarded))
	}
}

// sweepEntityStateGuardEntry folds one pre-marker snapshot delivery into the
// sweep map, last-revision-wins per key: a valid or tombstoned delivery erases
// an earlier poisoned one for the same key. A delivery is judged by
// decodeStoredEntity, the rule readEntity applies, so a value naming another
// entity than its key is inventoried at boot, not at its first read.
func sweepEntityStateGuardEntry(sweep map[string]entityPoisonRecord, entry jetstream.KeyValueEntry) {
	if natsclient.IsKVTombstone(entry.Operation()) {
		delete(sweep, entry.Key()) // key deleted: nothing resident to inventory
		return
	}
	_, err := decodeStoredEntity(entry.Key(), entry.Value())
	if err == nil {
		delete(sweep, entry.Key()) // valid revision supersedes an earlier poisoned one
		return
	}
	var contractErr *graph.StateContractError
	errors.As(err, &contractErr) // every refusal of the stored-value rule is a state error
	sweep[entry.Key()] = entityPoisonRecord{contractErr: contractErr, revision: entry.Revision()}
}

// recordBootSweepPoison moves the surviving sweep entries into the poison
// inventory. The first bootSweepErrorLogCap new entries log individually as
// structured ERRORs; the remainder is summarized by one count-only WARN so a
// mass-poison boot cannot flood the log.
func (c *Component) recordBootSweepPoison(ctx context.Context, sweep map[string]entityPoisonRecord) {
	if len(sweep) == 0 {
		return
	}
	keys := make([]string, 0, len(sweep))
	for key := range sweep {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	logged := 0
	for _, key := range keys {
		rec := sweep[key]
		if c.recordEntityPoison(rec.contractErr, rec.revision) {
			if logged < bootSweepErrorLogCap {
				c.logEntityPoisonRecorded(rec.contractErr, rec.revision)
				logged++
			}
			// Close the cross-process record/repair window: a repair that
			// completed after this key's snapshot delivery found no entry to
			// clear; verifying against current bytes drops the stale record.
			c.verifyEntityPoisonRecord(ctx, key)
		}
	}
	if len(keys) > logged {
		c.logger.Warn("boot snapshot sweep: additional poisoned entities inventoried past the log cap",
			slog.Int("total", len(keys)),
			slog.Int("logged_individually", logged))
	}
}

// classifyStoredStateRMWError re-attributes a read-modify-write failure to
// resident stored-state poison (SemStreams gh#562). The owner's own RMW reads check only
// the trusted decode and the key (decodeStoredForWrite, design D23), so a
// noncanonical STORED statement, or one with no Source or no Timestamp
// (checkpoint 3 H3), does not fail on the read — it surfaces either as a
// trusted-decode failure (unreadable JSON) or as a rejection by the merge's
// metadata check or MarshalEntityState write gate (encodeMerge) that would
// otherwise blame the merged CANDIDATE the caller submitted. On this slow path
// (the write is already failing)
// re-validate the stored bytes under decodeStoredEntity, the rule readEntity
// and the boot sweep apply: when the poison predates the merge, record it,
// stamped with the RMW target's entity ID, in the per-entity poison inventory
// at revision, the revision the RMW read current at, and return the
// graph-state-reset-required classification instead of a candidate-invalid
// error. A canonical stored state returns cycleErr unchanged — the candidate
// really was at fault. Every caller's bytes have already failed the decode or
// passed the key check, so only the decode or the statement-metadata rule can
// refuse them here.
//
// entityID is the RMW target (the CAS key) — the closures are the only place
// the identity is reliably in scope, so stamping happens here rather than at
// any outer helper (design D4).
func (c *Component) classifyStoredStateRMWError(ctx context.Context, entityID string, current []byte, revision uint64, cycleErr error) error {
	_, err := decodeStoredEntity(entityID, current)
	if err == nil {
		return cycleErr
	}
	var contractErr *graph.StateContractError
	if errors.As(err, &contractErr) {
		c.inventoryEntityPoison(ctx, contractErr, revision)
	}
	return err
}
