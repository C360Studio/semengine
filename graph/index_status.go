package graph

// IndexStatusResponse is the wire shape of the readiness envelope (gh#397,
// enriched by ADR-066, distributed as GRAPH_STATUS KV state by ADR-083): the
// deterministic-fusion honesty envelope's readiness signal.
//
// Ready reports COVERAGE: every committed ENTITY_STATES revision <= the query-time
// target has been applied (revision-lag, ADR-066), not merely "indexing started" (the
// old sticky NAME_INDEX-non-empty signal that fired minutes before the index was
// populated, gh#431). It guarantees revision coverage, not last-writer-wins freshness
// under same-key churn (ADR-066 §1 Scope boundary).
//
// Ready LICENSES NOTHING ABOUT ABSENCE (ADR-084). It once carried an "only Ready
// permits an authoritative not-found" contract; that is retired, because coverage
// cannot answer the question — an index caught up to every revision ever committed
// still knows nothing about a source that never published. No consumer may treat an
// empty result as an authoritative not-found under ANY envelope state.
//
// Nor does Ready == false withhold a response any more: reads gate on HEALTH (State
// plus BootstrapComplete), and a healthy index that is merely behind serves while
// reporting StalenessMs. Under continuous write Ready is false essentially always,
// which is what made it a poor proxy for soundness. Its remaining jobs are a caught-up
// fast path for the freshness question, and being the value a caller compares its own
// revision against via IndexedRevision — the one sound per-entity check.
//
// IndexedRevision is the one spelling of the revision; the pin's phase, revision
// and last_synced fields are not carried. The readiness computation that fills
// this envelope is in graph/readiness, so a reader of GRAPH_STATUS needs no NATS
// import.
type IndexStatusResponse struct {
	Ready bool   `json:"ready"` // target>0 && indexed_revision>=target_revision
	State string `json:"state"` // "building" | "ready" | "degraded" | "reset_required"
	// Code and Reason carry bounded operator-actionable fatal readiness state.
	// They are empty during ordinary building/ready/degraded operation.
	Code   string `json:"code,omitempty"`
	Reason string `json:"reason,omitempty"`
	// BootstrapComplete reports whether this producer has finished its INITIAL BUILD
	// in the current process lifetime: enumeration of ENTITY_STATES plus replay up to
	// the enumeration-time target, including the authoritatively-empty 0/0 outcome. It
	// latches true once and resets to false on restart, so a restart into a format
	// cutover re-gates (ADR-084 D2).
	//
	// It exists because health is otherwise not evaluable from the wire: an index
	// halfway through a gh#474 cutover reports State=building with a plausible Lag,
	// indistinguishable from an index that is merely behind. Ready cannot answer it
	// (a caught-up index is bootstrapped, but a bootstrapped index under write is not
	// caught up) and TargetRevision==0 cannot either (it is false during a cutover and
	// wrongly deferred the empty graph).
	//
	// NOT omitempty, and absent reads FALSE — fail closed. An envelope from a
	// pre-ADR-084 producer therefore defers every health gate until the lockstep
	// upgrade lands; that is the accepted migration cost, and the explicit `false` on
	// the wire keeps "old producer" distinguishable from "not yet bootstrapped" in a
	// `nats kv get GRAPH_STATUS <producer>` dump.
	BootstrapComplete bool `json:"bootstrap_complete"`
	// IndexedRevision is the low-water-of-pending watermark: every delivered
	// ENTITY_STATES revision <= this has been applied and nothing <= it is still
	// in flight. A consumer that knows its own target revision can gate on
	// IndexedRevision >= myRev instead of the coarse global Ready bool.
	IndexedRevision uint64 `json:"indexed_revision,omitempty"`
	// TargetRevision is the ENTITY_STATES stream LastSeq read at query time — the
	// latest committed write the index must catch up to.
	TargetRevision uint64 `json:"target_revision,omitempty"`
	// Lag is TargetRevision - IndexedRevision; 0 means caught up.
	Lag uint64 `json:"lag,omitempty"`
	// FailedCount / FailedReasons / FirstFailureAt are the bounded failure detail a
	// producer that tracks per-entity failures carries on a DEGRADED envelope so an
	// operator can tell a whole-dependency outage from a few persistently-failing
	// entities WITHOUT any unbounded per-entity list on the watched key (#613).
	//
	// All three are additive and omitempty: a producer with no failures — including
	// graph-index, which enforces the same rule caller-side via its watermark hole —
	// emits none, so the wire is byte-unchanged. FailedReasons is bounded to a fixed
	// reason enum (a handful of keys), keeping the watched key compact on the hot KV
	// path. FirstFailureAt is RFC3339. FailedCount is echoed from the projection input
	// (in graph-index's revision-lag projection it also drives State=degraded); the
	// other two are set by the producer after the projection.
	FailedCount    uint64            `json:"failed_count,omitempty"`
	FailedReasons  map[string]uint64 `json:"failed_reasons,omitempty"`
	FirstFailureAt string            `json:"first_failure_at,omitempty"`
	// StalenessMs is the AGE OF THE VIEW in milliseconds (ADR-083): now minus the
	// KV commit time of the newest ENTITY_STATES revision the index has fully
	// covered. It is the view-rate consumer's tolerance unit because it is
	// invariant to write rate and coalesce_ms, where a revision count is not
	// (gh#590: the correct revision bound shifted 2-4x with the coalesce dial
	// alone).
	//
	// PRESENCE ENCODING — 0 does NOT mean "zero staleness". It means the value
	// carries no information: either Ready is true (no staleness to report) or the
	// producer could not compute it (nothing covered yet / an early-return hard
	// stop). Any COMPUTED staleness is reported as at least 1ms, so a consumer can
	// treat `StalenessMs > 0` as the presence bit and never read an absent age as
	// "0ms fresh".
	//
	// It is REPORTED, never gating. readiness.EvaluateReadinessGate does not look at
	// this field: readiness withholds an answer only for index health, and how far
	// behind the view is rides on the answer instead (community detection stamps it on
	// staleness_at_detection_ms). A consumer that surfaces it must carry the presence
	// encoding with it — publishing a bare 0 as "caught up" is the one way to turn an
	// unknown age back into a false claim.
	//
	// It is a FLOOR, not an oracle: a revision still undelivered server-side cannot
	// age it. Total stalls surface through the wall-clock stuck detector
	// (State=degraded), not through this field.
	StalenessMs uint64 `json:"staleness_ms,omitempty"`
	// BootstrapScope is the SIZE OF THE INITIAL BUILD this producer latched
	// BootstrapComplete against, expressed in THE PRODUCER'S OWN UNIT — entities for
	// an enumerating producer, messages for a backlog producer, replayed values for a
	// watcher producer. It is deliberately not normalized across producers; a shared
	// unit would be a fiction, and every consumer reads it alongside the key it asked
	// for, so it already knows whose unit it is.
	//
	// It exists for exactly one distinction, which is NOT recoverable from the wire
	// today for any producer: `BootstrapComplete && BootstrapScope == 0` is
	// "authoritatively nothing to do" — the producer finished its initial build and
	// the build was empty — as opposed to "finished a build that had work in it".
	// gh#732 raises this: a consumer waiting for a bootstrap replay cannot otherwise
	// tell an empty-by-truth replay from one it observed too early.
	//
	// THE GATE MUST NOT READ IT. readiness.EvaluateReadinessGate does not look at this
	// field and must never start: the moment a verdict depends on a magnitude, this
	// becomes a threshold knob and readiness stops being a health question (ADR-085 deleted
	// max_staleness for the same reason). A test pins the gate's verdict as identical
	// across two envelopes differing only in this field.
	//
	// IT LICENSES NOTHING ABOUT ABSENCE. Scope 0 says the initial build found nothing
	// to do at the instant it latched — not that the underlying collection is empty,
	// and not that anything published later has been seen.
	BootstrapScope uint64 `json:"bootstrap_scope,omitempty"`
	// PublishedAt is when the producer's publisher wrote this value: UTC, RFC 3339
	// with nanoseconds, from the wall clock. graph/readiness.Publisher.Publish sets
	// it on every write, whatever the caller passed, so no producer can leave it
	// out. It states when the producer wrote, not when anything in the graph was
	// asserted.
	PublishedAt string `json:"published_at"`
}

// Index readiness states. Mirrors pkg/fusion.IndexState string values.
const (
	IndexStateBuilding      = "building"
	IndexStateReady         = "ready"
	IndexStateDegraded      = "degraded"
	IndexStateResetRequired = "reset_required"
)

// AllIndexStates is the closed iteration domain for the one-hot readiness `state`
// metric published by every producer of the envelope (graph-index, graph-embedding).
// It lives next to the const block so it is the single source of truth: adding a new
// readiness state means adding it HERE, and every one-hot state gauge picks it up
// automatically — otherwise a new state would render as all-zeros (silent "no data",
// not an alertable signal).
var AllIndexStates = []string{
	IndexStateBuilding,
	IndexStateReady,
	IndexStateDegraded,
	IndexStateResetRequired,
}
