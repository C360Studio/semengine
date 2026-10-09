package graphingest

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/dispatch"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"
)

// ingestWork is one unit submitted to the keyed ingest pool: a decoded entity
// plus the JetStream message (for ack/nak) and the (stream, sequence) the
// redelivery guard keys on. Partitioned by entityID so same-entity updates
// serialize on one lane in arrival order (ADR-072, gh#480).
type ingestWork struct {
	entity   *graph.EntityState
	msg      jetstream.Msg
	entityID string
	stream   string
	seq      uint64
	// deliveredAt is the JetStream timestamp of this message, carried from the
	// consume closure so the ack path can age the readiness view without a second
	// Metadata() call on a per-message hot path.
	deliveredAt time.Time
	// subject is the arrival port's filter subject — the lane label the
	// authority rejection is metered under, so an operator reading
	// mutation_rejections can see WHICH port a refused write came in on.
	subject string
	// importLane is the arrival port's own "import": true declaration (ADR-102
	// d5). It is read from the port, never from the message: a peer cannot
	// declare itself trusted.
	importLane bool
}

// laneGuard is one lane's in-memory applied-sequence cache — tier 1 of the
// redelivery guard. It is accessed only by that lane's single goroutine, so it
// needs no locking. Bounded: past max, one arbitrary entry is evicted — a cache
// miss just falls through to the durable tier, so which entry leaves the cache
// never affects correctness (ADR-072 B3).
type laneGuard struct {
	seq      map[string]uint64
	capacity int
	// refused holds the keys whose durable record this lane refused as undecodable and logged,
	// so a redelivery loop logs once per key (design D21). A key leaves when its record is next
	// read absent or decodable, which is the repair. It is bounded like seq: past capacity one
	// arbitrary key leaves, and if that key's record is still undecodable it is logged again.
	refused map[string]struct{}
}

func newLaneGuard(capacity int) *laneGuard {
	return &laneGuard{seq: make(map[string]uint64), capacity: capacity, refused: make(map[string]struct{})}
}

// markRefused records key as refused and reports whether it was not recorded already.
func (g *laneGuard) markRefused(key string) bool {
	if _, ok := g.refused[key]; ok {
		return false
	}
	if len(g.refused) >= g.capacity {
		for k := range g.refused {
			delete(g.refused, k)
			break
		}
	}
	g.refused[key] = struct{}{}
	return true
}

func (g *laneGuard) forgetRefused(key string) {
	delete(g.refused, key)
}

func (g *laneGuard) get(key string) (uint64, bool) {
	v, ok := g.seq[key]
	return v, ok
}

func (g *laneGuard) set(key string, v uint64) {
	if _, exists := g.seq[key]; !exists && len(g.seq) >= g.capacity {
		// Evict one arbitrary entry (map iteration order). The durable tier backs
		// correctness, so which entry leaves the cache is immaterial.
		for k := range g.seq {
			delete(g.seq, k)
			break
		}
	}
	g.seq[key] = v
}

// guardKey composes the durable/in-memory guard key. entityID is dotted and
// stream is uppercase alphanumeric; "/" separates them and is a valid NATS KV
// key character, so the composite is a valid durable-bucket key.
func guardKey(entityID, stream string) string {
	return entityID + "/" + stream
}

// buildIngestPool constructs the keyed-concurrent ingest pool + the in-memory
// guard shards. Called from Start BEFORE subscriptions (ADR-072 M3). The exact
// Start-derived pool context is supplied lexically and never retained.
func (c *Component) buildIngestPool(poolCtx context.Context) error {
	lanes := c.config.IngestLanes
	if lanes < 1 {
		lanes = 1
	}

	c.ingestGuardMem = make([]*laneGuard, lanes)
	for i := range c.ingestGuardMem {
		c.ingestGuardMem[i] = newLaneGuard(ingestGuardMemMaxPerLane)
	}

	pool, err := dispatch.NewKeyedPool(poolCtx, dispatch.KeyedConfig[ingestWork]{
		Lanes:      lanes,
		QueueDepth: ingestLaneQueueDepth,
		Name:       "graph_ingest",
		KeyOf:      func(w ingestWork) string { return w.entityID },
		Process:    c.processIngest,
		OnPanic: func(w ingestWork, _ any) {
			// The lane recovered a panic in processIngest; Nak so the server
			// redelivers rather than silently losing the message.
			if err := w.msg.Nak(); err != nil {
				c.logger.Error("Failed to Nak after ingest panic", slog.Any("error", err))
			}
		},
	}, dispatch.KeyedDeps{MetricsRegistry: c.metricsRegistry, Logger: c.logger})
	if err != nil {
		return errs.Wrap(err, "Component", "buildIngestPool", "keyed pool construction")
	}
	c.ingestPool = pool
	return nil
}

// processIngest is the keyed pool's per-item handler, run on lane `lane`'s
// single goroutine. It applies the redelivery guard, ingests the entity, then
// stamps the guard (durable first, then in-memory) and acks — the ADR-072
// two-tier guard, updated AFTER side effects and BEFORE ack.
func (c *Component) processIngest(ctx context.Context, lane int, work ingestWork) error {
	// Validate and prepare the complete Graphable candidate before its identity
	// participates in the idempotency guard. Structural failures are terminal
	// for this immutable stream message and must not create a guard key/Get or a
	// redelivery loop.
	if validationErr := c.prepareFactProjection(work.entity, work.importLane); validationErr != nil {
		// An authority rejection is its own class: metered under its own reason
		// and logged without the identity, never counted as a structural
		// contract rejection. Terminal either way — a foreign write can never
		// become admissible by redelivery.
		if reason, isAuthority := authorityMetricReason(validationErr); isAuthority {
			c.recordAuthorityRejection(work.subject, reason, validationErr)
		} else {
			c.recordStructuralRejection("graphable", validationErr)
		}
		if termErr := work.msg.Term(); termErr != nil {
			c.logger.Error("Failed to terminate structurally invalid ingest", slog.Any("error", termErr))
		}
		return validationErr
	}

	// Redelivery guard (ADR-072 B1/B2/B3): drop a stale re-delivery whose stream
	// sequence is not newer than the last already applied to this entity from the
	// same stream.
	stale, err := c.ingestGuardStale(ctx, lane, work)
	if err != nil {
		// Transient durable-guard read failure, or a stored record that cannot be
		// decoded — Nak so a redelivery re-reads, rather than risk applying a
		// possibly-stale message or dropping a valid one on a blind guess. The
		// undecodable record was counted and logged once per key where it was read.
		if !errors.Is(err, errUndecodableGuardRecord) {
			c.logger.Warn("graph-ingest: redelivery-guard read failed; redelivering",
				slog.String("entity_id", work.entityID), slog.Any("error", err))
		}
		if nakErr := work.msg.Nak(); nakErr != nil {
			c.logger.Error("Failed to Nak after guard-read error", slog.Any("error", nakErr))
		}
		return err
	}
	if stale {
		c.redeliveriesDropped.Inc()
		if ackErr := work.msg.Ack(); ackErr != nil {
			c.logger.Error("Failed to ack stale redelivery", slog.Any("error", ackErr))
		}
		return nil
	}

	// Apply. Structural contract violations are terminal for this immutable
	// message; storage, timeout, and cancellation failures are retried.
	start := time.Now()
	ingestErr := c.ingestEntity(ctx, work.entity, work.importLane)
	c.processingDuration.Observe(time.Since(start).Seconds())
	if ingestErr != nil {
		var stateErr *graph.StateContractError
		if errors.As(ingestErr, &stateErr) {
			// Disposition split by FAULT (poison-response-scoping D8): this
			// arrival's own candidate is valid — the RESIDENT state it must
			// merge into is poisoned, which is the environment's fault and
			// repairable (delete + recreate). Nak so the valid data survives
			// the repair window and applies on redelivery, bounded by the
			// consumer's existing MaxDeliver; backoff is the consumer's
			// delivery policy. A structurally-invalid CANDIDATE (its own
			// fault, can never succeed) stays Term'd. The inventory record and
			// once-per-entity ERROR happened in the write seam's callback
			// (decodeStoredForWrite or classifyStoredStateRMWError), so a
			// redelivery loop cannot spam the log.
			if nakErr := work.msg.Nak(); nakErr != nil {
				c.logger.Error("Failed to Nak ingest blocked by resident poisoned state", slog.Any("error", nakErr))
			}
		} else {
			c.recordEntityStateContractRejection("graphable", ingestErr)
			c.recordPredicateContractRejections("graphable", ingestErr)
			if errs.IsInvalid(ingestErr) {
				if termErr := work.msg.Term(); termErr != nil {
					c.logger.Error("Failed to terminate structurally invalid ingest", slog.Any("error", termErr))
				}
			} else if errs.IsFatal(ingestErr) {
				if c.logger != nil {
					c.logger.Error("graph-ingest: fatal ingest failure; terminating",
						slog.String("class", errs.ErrorFatal.String()))
				}
				if termErr := work.msg.Term(); termErr != nil {
					c.logger.Error("Failed to terminate fatal ingest", slog.Any("error", termErr))
				}
			} else if nakErr := work.msg.Nak(); nakErr != nil {
				c.logger.Error("Failed to Nak after transient ingest error", slog.Any("error", nakErr))
			}
		}
		return ingestErr
	}

	// Guard stamp AFTER side effects, BEFORE ack — durable FIRST, then in-memory.
	// On a durable-write failure, Nak and leave the in-memory tier un-updated so
	// the two tiers never diverge (a tier-1 update ahead of a failed durable write
	// would let a post-restart older redelivery slip past a stale durable stamp).
	if derr := c.ingestGuardStampDurable(ctx, work); derr != nil {
		c.logger.Warn("graph-ingest: redelivery-guard durable stamp failed; redelivering",
			slog.String("entity_id", work.entityID), slog.Any("error", derr))
		if nakErr := work.msg.Nak(); nakErr != nil {
			c.logger.Error("Failed to Nak after guard-stamp error", slog.Any("error", nakErr))
		}
		return derr
	}
	c.ingestGuardMem[lane].set(guardKey(work.entityID, work.stream), work.seq)

	if ackErr := work.msg.Ack(); ackErr != nil {
		c.logger.Error("Failed to ack JetStream message", slog.Any("error", ackErr))
	}
	// Age the readiness view from the newest message whose graph write is durable.
	// Stamped AFTER the ack attempt, on the success path only: an un-acked message
	// has not been applied, and stamping earlier would age the view from work that
	// may still Nak.
	c.recordApplied(work.deliveredAt)
	return nil
}

// recordStructuralRejection counts and logs one stream message refused because its candidate
// is structurally invalid: processIngest's record for a candidate that fails its contract, and
// the consume closure's for a refused statement (design D15). The caller terminates the message.
func (c *Component) recordStructuralRejection(lane string, err error) {
	c.recordEntityStateContractRejection(lane, err)
	c.recordPredicateContractRejections(lane, err)
	c.logStructuralContractRejection(lane, err)
}

func (c *Component) logStructuralContractRejection(lane string, err error) {
	if c.logger == nil {
		return
	}
	field, reason, tripleIndex, ok := entityStateContractRejectionLabels(err)
	var refusal *statementRefusal
	if !ok && errors.As(err, &refusal) {
		field, reason, tripleIndex, ok = refusal.field, refusal.reason, refusal.index, true
	}
	if !ok {
		predicateReason, predicate := predicateContractReason(err)
		if predicate {
			field, reason, tripleIndex = contractFieldPredicate, predicateReason, -1
		} else {
			field, reason, tripleIndex = string(graph.EntityStateContractFieldID), contractReasonUnknown, -1
		}
	}
	attrs := []any{
		slog.String("lane", lane),
		slog.String("field", field),
		slog.String("reason", reason),
	}
	if tripleIndex >= 0 {
		attrs = append(attrs, slog.Int("triple_index", tripleIndex))
	}
	c.logger.Warn("graph-ingest: structural contract rejection; terminating", attrs...)
}

// ingestGuardStale reports whether work is a stale redelivery — its stream
// sequence is not newer than the last already applied to (entity, stream). It
// checks the in-memory tier first (lane-local, no KV op), falling back to the
// durable tier on a miss (cold / evicted / post-restart) and warming the cache.
func (c *Component) ingestGuardStale(ctx context.Context, lane int, work ingestWork) (bool, error) {
	key := guardKey(work.entityID, work.stream)

	if last, ok := c.ingestGuardMem[lane].get(key); ok {
		return work.seq <= last, nil
	}
	if c.ingestGuardBucket == nil {
		return false, nil // no durable tier provisioned → treat as first-seen
	}
	entry, err := c.ingestGuardBucket.Get(ctx, key)
	if err != nil {
		if natsclient.IsKVNotFoundError(err) {
			c.ingestGuardMem[lane].forgetRefused(key)
			return false, nil // never applied, or the record was deleted → not stale
		}
		return false, err
	}
	// ingestGuardStampDurable writes the one format: the sequence as eight big-endian bytes. A
	// record of any other length cannot be decoded. Reading it as first seen would reopen the
	// overwrite the record prevents, and reading a prefix of a longer one would be a guess, so the
	// input is refused until the key is deleted (design D21).
	if len(entry.Value) != 8 {
		c.refuseUndecodableGuardRecord(lane, key, len(entry.Value))
		return false, fmt.Errorf("%w: key %s holds %d bytes", errUndecodableGuardRecord, key, len(entry.Value))
	}
	c.ingestGuardMem[lane].forgetRefused(key)
	last := binary.BigEndian.Uint64(entry.Value)
	c.ingestGuardMem[lane].set(key, last) // warm the cache
	return work.seq <= last, nil
}

// errUndecodableGuardRecord marks an input refused because its stored applied-sequence record
// cannot be decoded. processIngest Naks it without its per-delivery warning: the refusal was
// counted and logged once for its key where the record was read.
var errUndecodableGuardRecord = errors.New("applied-sequence record cannot be decoded")

// refuseUndecodableGuardRecord counts one refused input and logs the key the first time this lane
// refuses it. Not applying the input and not acknowledging it is the safe answer: the record
// exists to stop an older input overwriting a newer one, and graph-ingest cannot tell which this
// is. Deleting the key repairs it; the redelivery is then applied as first seen.
func (c *Component) refuseUndecodableGuardRecord(lane int, key string, size int) {
	c.guardRecordRefusals.Inc()
	if !c.ingestGuardMem[lane].markRefused(key) || c.logger == nil {
		return
	}
	c.logger.Error("graph-ingest: applied-sequence record cannot be decoded; its inputs are refused until the key is deleted",
		slog.String("bucket", graph.BucketGraphIngestAppliedSeq),
		slog.String("key", key),
		slog.Int("bytes", size))
}

// ingestGuardStampDurable persists the last-applied sequence for (entity,
// stream) in the durable guard bucket. A plain Put (last-writer-wins) is safe:
// (entity, stream) is only ever written by one lane's goroutine (entity → one
// lane), so there is no concurrent writer to race.
func (c *Component) ingestGuardStampDurable(ctx context.Context, work ingestWork) error {
	if c.ingestGuardBucket == nil {
		return nil
	}
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], work.seq)
	_, err := c.ingestGuardBucket.Put(ctx, guardKey(work.entityID, work.stream), buf[:])
	return err
}
