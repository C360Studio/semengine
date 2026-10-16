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
// plus the JetStream message, which only the delivery helper settles, and the
// (stream, generation, sequence) the redelivery guard keys on. Partitioned by
// entityID so same-entity updates serialize on one lane in arrival order
// (ADR-072, gh#480).
type ingestWork struct {
	entity *graph.EntityState
	msg    jetstream.Msg
	// consumer is the configuration the input's consumer was created with, and heartbeat the
	// interval at which its application tells the server it is still in progress: what
	// processIngest builds the input's delivery policy from (design D8, "Long apply").
	consumer  natsclient.StreamConsumerConfig
	heartbeat time.Duration
	entityID  string
	stream    string
	seq       uint64
	// generation is the input stream's generation: its creation time on the server, in
	// nanoseconds, read once before Start binds the consumer (design D8, #15).
	generation uint64
	// deliveredAt is the JetStream timestamp of this message, carried from the
	// consume closure, which read it, so the application, which holds no message,
	// can age the readiness view.
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

// guardRecord is what the redelivery guard keeps for one entity and stream: the stream
// generation the last applied input came from, and that input's sequence. Stored, it is
// sixteen bytes: the generation, then the sequence, each big-endian.
type guardRecord struct {
	generation uint64
	seq        uint64
}

const guardRecordSize = 16

func (r guardRecord) encode() []byte {
	buf := make([]byte, guardRecordSize)
	binary.BigEndian.PutUint64(buf[:8], r.generation)
	binary.BigEndian.PutUint64(buf[8:], r.seq)
	return buf
}

// decodeGuardRecord reads a stored record. A value of any other length than the one format
// is not a record graph-ingest can read (design D21).
func decodeGuardRecord(value []byte) (guardRecord, bool) {
	if len(value) != guardRecordSize {
		return guardRecord{}, false
	}
	return guardRecord{
		generation: binary.BigEndian.Uint64(value[:8]),
		seq:        binary.BigEndian.Uint64(value[8:]),
	}, true
}

// covers reports whether work is a redelivery of an input this record has seen: it comes
// from the same stream generation, at a sequence not newer than the last applied. An input
// from any other generation is not covered, whatever its sequence (#15). An input from a
// stream that was deleted and created again, or restored, can still carry this record's
// generation; those cases are declared limitations (graph-ingest-recovery, "Replay
// protection uses the generation read before the bind"). Generations are compared for
// equality, never order: they are creation times, and two servers' clocks need not agree.
func (r guardRecord) covers(work ingestWork) bool {
	return r.generation == work.generation && work.seq <= r.seq
}

// laneGuard is one lane's in-memory applied-sequence cache — tier 1 of the
// redelivery guard. It is touched by one goroutine at a time, so it needs no
// locking: an input's application, which runs on a goroutine of its own that
// natsclient.ConsumeDeliveryWithHeartbeat starts after the lane calls it and joins
// before it returns, so the lane's inputs are applied one after another.
// Bounded: past max, one arbitrary entry is evicted — a cache
// miss just falls through to the durable tier, so which entry leaves the cache
// never affects correctness (ADR-072 B3).
type laneGuard struct {
	seq      map[string]guardRecord
	capacity int
	// refused holds the keys whose durable record this lane refused as undecodable and logged,
	// so a redelivery loop logs once per key (design D21). A key leaves when its record is next
	// read absent or decodable, which is the repair. It is bounded like seq: past capacity one
	// arbitrary key leaves, and if that key's record is still undecodable it is logged again.
	refused map[string]struct{}
}

func newLaneGuard(capacity int) *laneGuard {
	return &laneGuard{seq: make(map[string]guardRecord), capacity: capacity, refused: make(map[string]struct{})}
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

func (g *laneGuard) get(key string) (guardRecord, bool) {
	v, ok := g.seq[key]
	return v, ok
}

func (g *laneGuard) set(key string, v guardRecord) {
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
// key character, so the composite is a valid durable-bucket key. The stream's
// generation is in the record, not the key, so a recreated stream's first apply
// replaces the record its predecessor left and the key set stays one per entity
// and stream.
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
		Process: func(ctx context.Context, lane int, work ingestWork) {
			if err := c.processIngest(ctx, lane, work); err != nil {
				c.logger.Debug("graph-ingest: processIngest returned an error",
					slog.String("entity_id", work.entityID), slog.Int("lane", lane), slog.Any("error", err))
			}
		},
		OnPanic: func(w ingestWork, _ any) {
			// The lane recovered a panic in processIngest's own code; Nak so the
			// server redelivers rather than silently losing the message. A panic in
			// the application is recovered by the delivery helper instead and is
			// answered by recordInterruptedWork.
			if err := w.msg.Nak(); err != nil {
				c.logger.Error("Failed to Nak after ingest panic", slog.Any("error", err))
			}
		},
		MetricsRegistry: c.metricsRegistry,
		Logger:          c.logger,
	})
	if err != nil {
		return errs.Wrap(err, "Component", "buildIngestPool", "keyed pool construction")
	}
	c.ingestPool = pool
	return nil
}

// processIngest is the keyed pool's per-item handler (buildIngestPool's Process
// calls it and logs a returned error at Debug), run on lane `lane`'s goroutine
// with the lane context, the pool context Start derives. It runs the
// input's application (applyIngest) inside natsclient.ConsumeDeliveryWithHeartbeat
// (design D8, "Long apply"): while the application runs, the server is told every
// heartbeat interval that the input is still in progress, and when it returns,
// the helper settles the input once, by the decision the application returned.
func (c *Component) processIngest(ctx context.Context, lane int, work ingestWork) error {
	// The application holds the input without its message: only the helper settles it.
	input := work
	input.msg = nil
	policy, err := natsclient.ValidateHeartbeatDeliveryPolicy(ctx, work.consumer, work.heartbeat,
		natsclient.ImmediateDeliveryRetry(),
		func(applyCtx context.Context, _ []byte) (natsclient.DeliveryDecision, error) {
			return c.applyIngest(applyCtx, lane, input)
		})
	if err != nil {
		// Start checked the port's pair before it created the consumer, so the policy is
		// refused only once the lane context has ended: when Start's context ends, or at
		// Stop, which ends it after the pool's Shutdown returns or runs out of time. Ending
		// it stops no lane, so every input still queued reaches here. Nothing is applied;
		// the input is negatively acknowledged, the shutdown disposition, and delivered
		// again.
		c.logger.Warn("graph-ingest: input not applied, its delivery policy was refused; redelivering",
			slog.String("entity_id", work.entityID), slog.Any("error", err))
		result := natsclient.SettleDelivery(work.msg, natsclient.DeliveryDecisionRetry, err)
		if result.SettlementMethodFailed() {
			c.logger.Error("Failed to Nak an input whose delivery policy was refused", slog.Any("error", result.Err()))
		}
		return result.Err()
	}
	result := natsclient.ConsumeDeliveryWithHeartbeat(ctx, work.msg, policy)
	if result.OwnerStopRequired() {
		c.recordInterruptedWork(work, result.Err())
		return result.Err()
	}
	if result.SettlementMethodFailed() {
		c.logger.Error("Failed to settle ingest",
			slog.String("entity_id", work.entityID), slog.Any("error", result.Err()))
	}
	return result.Err()
}

// applyIngest is one input's application, the delivery work processIngest runs. It
// applies the redelivery guard, ingests the entity, then stamps the guard (durable
// first, then in-memory) and ages the readiness view — the ADR-072 two-tier guard,
// updated AFTER side effects and BEFORE the ack. It settles nothing: work carries no
// message. It returns the settlement it calls for: Terminate for an input that can
// never apply, Retry (a plain Nak) for a failure a redelivery can outlive, Ack for a
// stale redelivery or an applied input. It never returns Quarantine.
func (c *Component) applyIngest(ctx context.Context, lane int, work ingestWork) (natsclient.DeliveryDecision, error) {
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
		return natsclient.DeliveryDecisionTerminate, validationErr
	}

	// Redelivery guard (ADR-072 B1/B2/B3): drop a stale re-delivery whose stream
	// sequence is not newer than the last already applied to this entity from the
	// same stream generation (#15).
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
		return natsclient.DeliveryDecisionRetry, err
	}
	if stale {
		c.redeliveriesDropped.Inc()
		return natsclient.DeliveryDecisionAck, nil
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
			return natsclient.DeliveryDecisionRetry, ingestErr
		}
		c.recordEntityStateContractRejection("graphable", ingestErr)
		c.recordPredicateContractRejections("graphable", ingestErr)
		if errs.IsInvalid(ingestErr) {
			return natsclient.DeliveryDecisionTerminate, ingestErr
		}
		if errs.IsFatal(ingestErr) {
			if c.logger != nil {
				c.logger.Error("graph-ingest: fatal ingest failure; terminating",
					slog.String("class", errs.ErrorFatal.String()))
			}
			return natsclient.DeliveryDecisionTerminate, ingestErr
		}
		return natsclient.DeliveryDecisionRetry, ingestErr
	}

	// Guard stamp AFTER side effects, BEFORE ack — durable FIRST, then in-memory.
	// On a durable-write failure, Nak and leave the in-memory tier un-updated so
	// the two tiers never diverge (a tier-1 update ahead of a failed durable write
	// would let a post-restart older redelivery slip past a stale durable stamp).
	if derr := c.ingestGuardStampDurable(ctx, work); derr != nil {
		c.logger.Warn("graph-ingest: redelivery-guard durable stamp failed; redelivering",
			slog.String("entity_id", work.entityID), slog.Any("error", derr))
		return natsclient.DeliveryDecisionRetry, derr
	}
	c.ingestGuardMem[lane].set(guardKey(work.entityID, work.stream), guardRecord{generation: work.generation, seq: work.seq})

	// Age the readiness view from the newest message whose graph write and guard
	// record are durable: on the applied path only, before the helper's Ack. A
	// stale redelivery applied nothing and does not age it.
	c.recordApplied(work.deliveredAt)
	return natsclient.DeliveryDecisionAck, nil
}

// recordInterruptedWork answers an input whose application the delivery helper reports
// the owner must stop for: the application panicked, an in-progress signal could not be
// sent, or the delivery's metadata or the application's decision was invalid. The
// helper settled nothing, so the server delivers the input again after its ack wait;
// the applied-sequence record makes the redelivery apply once, and MaxDeliver bounds
// the attempts. The lane keeps taking input (owner ruling, #91 comment 6109278240,
// item 1): graph-ingest is the only writer of entity state, so one bad input must not
// stop ingest. This departs from OwnerStopRequired's "must stop its lane" (design D8,
// "Long apply"). The interruption is declared: an ERROR line and one in
// work_interrupted_total under its cause. It adds nothing to the error count Health
// reads (owner ruling, #91 comment 6124130342): a write the interruption cut has counted
// once at its own site, and an interruption anywhere else leaves that count unchanged.
func (c *Component) recordInterruptedWork(work ingestWork, err error) {
	cause := interruptedWorkCause(err)
	c.workInterrupted.WithLabelValues(cause).Inc()
	c.logger.Error("graph-ingest: input's application interrupted; left unsettled for redelivery",
		slog.String("cause", cause),
		slog.String("entity_id", work.entityID),
		slog.String("stream", work.stream),
		slog.Uint64("seq", work.seq),
		slog.Any("error", err))
}

// interruptedWorkCause names the cause of an interrupted application, the label of
// work_interrupted_total: heartbeat_failed when an in-progress signal could not be
// sent, otherwise panic when the application panicked, otherwise invalid.
func interruptedWorkCause(err error) string {
	var panicked *natsclient.DeliveryWorkPanicError
	switch {
	case errors.Is(err, natsclient.ErrHeartbeatFailed):
		return "heartbeat_failed"
	case errors.As(err, &panicked):
		return "panic"
	default:
		return "invalid"
	}
}

// recordStructuralRejection counts and logs one stream message refused because its candidate
// is structurally invalid: applyIngest's record for a candidate that fails its contract, and
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
// sequence is not newer than the last already applied to (entity, stream) from
// the same stream generation. It checks the in-memory tier first (lane-local, no
// KV op), falling back to the durable tier on a miss (cold / evicted /
// post-restart) and warming the cache.
func (c *Component) ingestGuardStale(ctx context.Context, lane int, work ingestWork) (bool, error) {
	key := guardKey(work.entityID, work.stream)

	if last, ok := c.ingestGuardMem[lane].get(key); ok {
		return last.covers(work), nil
	}
	if c.ingestGuardBucket == nil {
		c.recordUnguardedInput(work)
		return false, nil
	}
	entry, err := c.ingestGuardBucket.Get(ctx, key)
	if err != nil {
		if natsclient.IsKVNotFoundError(err) {
			c.ingestGuardMem[lane].forgetRefused(key)
			return false, nil // never applied, or the record was deleted → not stale
		}
		return false, err
	}
	// ingestGuardStampDurable writes the one format, guardRecord's. A value of any other length,
	// the pin's eight-byte sequence among them, cannot be decoded. Reading it as first seen would
	// reopen the overwrite the record prevents, and reading part of it would be a guess, so the
	// input is refused until the key is deleted (design D21).
	last, ok := decodeGuardRecord(entry.Value)
	if !ok {
		c.refuseUndecodableGuardRecord(lane, key, len(entry.Value))
		return false, fmt.Errorf("%w: key %s holds %d bytes", errUndecodableGuardRecord, key, len(entry.Value))
	}
	c.ingestGuardMem[lane].forgetRefused(key)
	c.ingestGuardMem[lane].set(key, last) // warm the cache
	return last.covers(work), nil
}

// recordUnguardedInput declares the degrade of design D8: with no applied-sequence bucket,
// which only a component built without Start can lack (Start fails when it cannot acquire
// one), an input the lane's cache has not seen is applied as first seen. Each such input is
// counted and logged, naming it: nothing redelivers it in a loop, since it is applied and
// acknowledged, so a line per input is a line per apply the guard did not check.
func (c *Component) recordUnguardedInput(work ingestWork) {
	c.unguardedInputs.Inc()
	if c.logger == nil {
		return
	}
	c.logger.Warn("graph-ingest: no applied-sequence bucket; applying the input as first seen",
		slog.String("entity_id", work.entityID),
		slog.String("stream", work.stream),
		slog.Uint64("seq", work.seq))
}

// errUndecodableGuardRecord marks an input refused because its stored applied-sequence record
// cannot be decoded. applyIngest asks for a Nak without its per-delivery warning: the refusal was
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

// ingestGuardStampDurable persists the stream generation and last-applied
// sequence for (entity, stream) in the durable guard bucket. A plain Put
// (last-writer-wins) is safe: (entity, stream) is only ever written by one lane,
// one input at a time (entity → one lane), so there is no concurrent writer to race.
func (c *Component) ingestGuardStampDurable(ctx context.Context, work ingestWork) error {
	if c.ingestGuardBucket == nil {
		return nil
	}
	record := guardRecord{generation: work.generation, seq: work.seq}
	_, err := c.ingestGuardBucket.Put(ctx, guardKey(work.entityID, work.stream), record.encode())
	return err
}
