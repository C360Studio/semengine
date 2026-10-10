package readiness

import (
	"time"

	"github.com/c360studio/semengine/graph"
)

// BacklogStatusInputs are the observations ComputeBacklogStatus projects into the
// readiness envelope. It is SEPARATE from the inputs of graph-index's revision-lag
// projection on purpose: the two producer shapes have disjoint inputs (a revision
// watermark vs. a message backlog), and merging them would make mutually-exclusive
// fields co-resident — an invalid state made representable in the one projection
// whose output gates authoritative-absence claims.
type BacklogStatusInputs struct {
	// Outstanding is total un-applied work across every bound consumer, IN MESSAGES —
	// the sum of NumPending (server-side, undelivered) and NumAckPending
	// (delivered, not yet acked) over all of them.
	//
	// THE SUM IS INVARIANT TO WHICH COUNTER HOLDS A MESSAGE, and that is why it is a
	// sum rather than either half. A message moves between the two continuously —
	// delivered (pending -> ack-pending), nak'd back (ack-pending -> pending),
	// redelivered again — so any single counter oscillates while work is steady.
	// Only the total is monotone with respect to real outstanding work. Do not
	// "simplify" this to NumPending: that under-reports by the whole in-process lane
	// queue (up to defaultIngestLanes(8) x ingestLaneQueueDepth(256) = 2048 messages
	// held delivered-but-unacked), which is precisely the backlog gh#712 tripped over.
	//
	// WHY NOT THE ACK FLOOR: measured 2026-07-30 against both deployed NATS versions
	// (2.10, 2.12), AckFloor.Stream does not advance past a MaxDeliver-exhausted
	// message, then jumps PAST it on the next unrelated ack — reading
	// permanently-not-caught-up while idle and falsely-covered under traffic. It never
	// means "everything at or below this is durable". See the change's design.md D0.
	//
	// HONESTY BOUNDARY: Outstanding == 0 means NO OUTSTANDING WORK, not EVERYTHING WAS
	// APPLIED. A MaxDeliver-parked message leaves BOTH counters (measured), so it is
	// invisible here. Caught-up is a backlog claim and cannot license an absence
	// claim; operator visibility for parked messages is gh#742.
	Outstanding uint64
	// BootstrapComplete is the producer's own initial-build latch. It is an INPUT, not
	// a projection: what constitutes the initial build differs per producer (a drained
	// boot sweep, a watcher replay sentinel), and only the producer can say.
	BootstrapComplete bool
	// BootstrapScope is the size of that initial build in the producer's unit; see the
	// field of the same name on graph.IndexStatusResponse. Passed through untouched — the
	// projection never compares it to anything.
	BootstrapScope uint64
	// ObservationFailed reports that the producer could not read its own backlog (a
	// consumer.Info() failure). It projects to degraded AND forces Ready false,
	// mirroring graph-index's precedent for a failed target read
	// (processor/graph-index/watermark.go:69-80): a backend fault cannot honestly
	// confirm caught-up, and "building" would read as ordinary progress rather than a
	// fault. See the divergence note in ComputeBacklogStatus for why this is stronger
	// than the revision-lag projection's FailedCount handling.
	//
	// Outstanding may still be a PARTIAL sum when this is set (some consumers read,
	// one failed). That partial is kept on Lag as an honest lower bound rather than
	// zeroed — a lower bound under a degraded verdict is strictly more useful to an
	// operator than a fabricated 0, and no gate proceeds on degraded anyway.
	ObservationFailed bool
	// OldestOutstandingAt is the JetStream timestamp of the oldest outstanding
	// message. ZERO means "not computable" — the projection then leaves StalenessMs at
	// 0 rather than fabricating a fresh-looking view, matching the presence encoding
	// on that field.
	OldestOutstandingAt time.Time
	// Now is the compute instant; the zero value means time.Now().
	Now time.Time
}

// ComputeBacklogStatus builds the readiness envelope for a BACKLOG producer — one
// whose "caught up" is the absence of un-applied messages rather than a revision
// watermark. It is a projection of its own beside graph-index's revision-lag
// projection, not a mode of it.
//
//   - Ready = Outstanding == 0 && BootstrapComplete.
//   - State = observationFailed ? "degraded" : (ready ? "ready" : "building").
//   - Lag = Outstanding, IN MESSAGES — a different unit from the revision-lag
//     producers' Lag, which the spec states explicitly.
//   - StalenessMs = 0 when Ready or when OldestOutstandingAt is unknown, else
//     now-OldestOutstandingAt with a 1ms floor (shared presence encoding).
//
// WHY NOT the revision-lag projection: it computes Ready = target > 0 && indexed >= target,
// which is FALSE at 0/0 — exactly the steady state of an idle backlog producer with
// nothing to do. Bending it to accommodate that would also risk byte-drift in
// graph-index's published output, which the current spec protects.
//
// IndexedRevision and TargetRevision are DELIBERATELY NOT SET, and both are omitempty
// so they stay off the wire. Those fields are contractually in the ENTITY_STATES KV
// revision space — ADR-084 D3 pins them as comparable to a caller's kv_revision by a
// test. A backlog producer consumes multiple streams whose sequence spaces are
// independent, so there is no single scalar revision to report; writing a stream
// sequence into a KV-revision field would silently corrupt every read-your-writes
// check in the system. A test asserts their absence on the wire.
//
// The gate's PRODUCER INVARIANT (Ready == true implies BootstrapComplete == true) holds
// here by construction: BootstrapComplete is a conjunct of Ready.
func ComputeBacklogStatus(in BacklogStatusInputs) graph.IndexStatusResponse {
	// ObservationFailed forces Ready false — it does NOT merely degrade State. This is
	// where this projection deliberately DIVERGES from the revision-lag projection's
	// FailedCount handling, and the difference is what is known:
	//
	//   - FailedCount > 0: the watermark WAS read and coverage IS accurate; only health
	//     is bad, so Ready stays coverage-accurate and State carries the verdict.
	//   - ObservationFailed: the coverage number itself is UNKNOWN. Outstanding == 0 is
	//     then the absence of a measurement, not a measurement of absence, and
	//     projecting Ready from it is a fail-open on exactly the field that licenses
	//     callers to stop waiting.
	//
	// graph-index's analogous failed-target-read path returns Ready: false with
	// State: degraded (processor/graph-index/watermark.go:69-80); this matches it.
	ready := in.Outstanding == 0 && in.BootstrapComplete && !in.ObservationFailed
	state := graph.IndexStateBuilding
	switch {
	case in.ObservationFailed:
		// First and unconditional: a producer that cannot observe its own backlog is
		// faulted, whatever the last-known counts happened to be.
		state = graph.IndexStateDegraded
	case ready:
		state = graph.IndexStateReady
	}
	return graph.IndexStatusResponse{
		Ready:             ready,
		State:             state,
		BootstrapComplete: in.BootstrapComplete,
		BootstrapScope:    in.BootstrapScope,
		Lag:               in.Outstanding,
		StalenessMs:       stalenessMs(ready, in.OldestOutstandingAt, in.Now),
	}
}

// stalenessMs projects the age of the view. It returns 0 — the "no information"
// encoding — when the view is caught up (nothing is stale) or when the floor's
// commit time is unknown, and otherwise at least 1ms so that a computed staleness
// is always distinguishable from an absent one.
func stalenessMs(ready bool, indexedAt, now time.Time) uint64 {
	if ready || indexedAt.IsZero() {
		return 0
	}
	if now.IsZero() {
		now = time.Now()
	}
	ms := now.Sub(indexedAt).Milliseconds()
	if ms < 1 {
		// Sub-millisecond age, or a local clock behind the server's. Report the
		// minimum COMPUTED value rather than 0, which would read as "absent".
		ms = 1
	}
	return uint64(ms)
}
