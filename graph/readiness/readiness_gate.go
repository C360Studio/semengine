package readiness

import (
	"slices"

	"github.com/c360studio/semengine/graph"
)

// The readiness gate. It is one function so that every consumer gates by the same
// rules: at the SemStreams pin, four consumers once had four gates, and a transport
// failure in one of them read in the logs like an index that was not ready (SemStreams
// gh#590).
//
// It asks ONE question, HEALTH (ADR-085 Decision 1): is this index sound to read from?
// Fresh status, an interpretable state, no hard stop, initial build complete. No
// consumer can opt out of it.
//
// How far behind the view is, graph.IndexStatusResponse.StalenessMs, is REPORTED, never
// gating (ADR-085 Decision 2): it rides on the answer, and a consumer stamps it on its
// output.
//
// Read-your-writes is not a gate question: a caller that knows its own revision
// compares it with graph.IndexStatusResponse.IndexedRevision. Ready does not appear in
// this gate: it is coverage, and coverage neither licenses nor withholds an answer.

// DeferReason is the typed cause of a defer. Its string form is the label value of the
// defer_total{reason} counter and of the structured defer log field, so the set is
// CLOSED: four reasons an operator acts on differently — a broken index, an index that
// has not finished building, a dead status feed, and an envelope this consumer cannot
// interpret.
type DeferReason string

// The closed defer-reason set.
const (
	// DeferNone is the reason attached to a proceed.
	DeferNone DeferReason = ""
	// DeferHardStop is a degraded or reset_required index: broken, not behind.
	DeferHardStop DeferReason = "hard_stop"
	// DeferStatusUnknown is a status feed the consumer cannot vouch for (never
	// received, or older than the freshness window). It fails closed and is never
	// mistaken for index state.
	DeferStatusUnknown DeferReason = "status_unknown"
	// DeferUnrecognizedState is an envelope whose State is blank or outside
	// graph.IndexStates(). It is NOT the same operator problem as a dead feed: the value
	// arrived and decoded, so the producer is talking — it is saying something this
	// consumer does not understand, which means version skew (a newer producer, a
	// partially-written envelope, a hand-edited key). Failing closed on it is the
	// whole posture of ADR-084: State became load-bearing when health replaced
	// coverage, so an uninterpretable State cannot be waved through as "not degraded".
	DeferUnrecognizedState DeferReason = "unrecognized_state"
	// DeferBootstrapIncomplete is a producer that has not finished its initial build
	// in this process lifetime, such as an index halfway through a format cutover
	// (SemStreams gh#474), where the keyset is half-built and a plausible small Lag
	// is actively misleading. TargetRevision==0 cannot stand in for it: that is false
	// during a cutover, and true for an empty graph whose build has finished.
	DeferBootstrapIncomplete DeferReason = "bootstrap_incomplete"
)

// DeferReasons returns the closed set of defer reasons a gate can return, as a new
// slice on every call, so no caller can change what the next one reads. It lives next to
// the const block so it is the single source of truth, as graph.IndexStates() is for wire
// states: adding a reason means adding it HERE, once, and every consumer that
// pre-initializes counter series or validates a label set picks it up automatically.
//
// The alternative — each consumer hand-maintaining its own copy — fails SILENTLY in the
// direction that matters. A counter keyed off a private list drops any reason missing
// from it (dropping is correct: an unbounded label is a cardinality leak), so a newly
// added reason would defer in production while incrementing nothing, and the dashboard
// would read as "no defers" rather than "unknown defers".
//
// DeferNone is deliberately absent: it is the reason attached to a PROCEED, not a defer,
// and pre-initializing a "" series would publish a permanently-zero label.
func DeferReasons() []DeferReason {
	return []DeferReason{
		DeferHardStop,
		DeferStatusUnknown,
		DeferUnrecognizedState,
		DeferBootstrapIncomplete,
	}
}

// EvaluateReadinessGate is the single home for readiness gate semantics. It answers the
// health question for one Reading and returns the typed defer reason; the caller proceeds
// when that is DeferNone. A Watcher reader passes w.Read(), so status and freshness come
// from one read; an IN-PROCESS producer that computes its own envelope passes
// Reading{Status: s, Fresh: true}, having no transport to lose. How a caller REACTS to
// a defer (error, skip a tick, serve an honest empty envelope) stays with the caller.
//
// Evaluation order is load-bearing:
//
//  1. Unknown status short-circuits BEFORE anything else. A dead status feed is a
//     transport fact; nothing licenses proceeding on state the consumer cannot vouch
//     for.
//  2. An UNRECOGNIZED State defers. The check is an allow-list over graph.IndexStates(),
//     not a deny-list for degraded/reset_required: a blank or future state must not
//     read as "not degraded, therefore healthy".
//  3. Hard stops (degraded, reset_required) defer.
//  4. An incomplete initial build defers likewise. This guards against reading an
//     index halfway through a format cutover (SemStreams gh#474), and is why
//     bootstrap_complete is on the wire.
//  5. Everything else proceeds, however far behind the view is. Lag is not a fault; it
//     is a property of the answer, carried on graph.IndexStatusResponse.StalenessMs for the
//     consumer to stamp on its output.
//
// Ready is deliberately absent. It is COVERAGE, and coverage answers no question this
// gate asks: it cannot license a proceed a health verdict already denied, and it cannot
// withhold one a health verdict already allowed. A caller with a specific revision in
// hand compares it against IndexedRevision itself.
//
// PRODUCER INVARIANT: an envelope with Ready == true must also carry
// BootstrapComplete == true. Health is answered before coverage is consulted anywhere,
// so a producer that published a caught-up envelope while reporting an unfinished build
// would see every consumer defer on it forever, pointing an operator at a cutover that
// already finished. A caller comparing its own revision with IndexedRevision also
// assumes the two agree. Every producer keeps the invariant; ComputeBacklogStatus does
// by construction.
func EvaluateReadinessGate(r Reading) DeferReason {
	status := r.Status
	if !r.Fresh {
		return DeferStatusUnknown
	}
	if !recognizedState(status.State) {
		// Allow-list, not deny-list. Checking only for degraded/reset_required would
		// let a blank or future state proceed as healthy — fail-OPEN on exactly the
		// field ADR-084 made load-bearing.
		return DeferUnrecognizedState
	}
	if status.State == graph.IndexStateDegraded || status.State == graph.IndexStateResetRequired {
		return DeferHardStop
	}
	if !status.BootstrapComplete {
		return DeferBootstrapIncomplete
	}
	return DeferNone
}

// recognizedState reports whether a wire State is one this consumer understands.
// Sourced from graph.IndexStates() so adding a state means adding it there, once.
func recognizedState(state graph.IndexState) bool {
	return slices.Contains(graph.IndexStates(), state)
}
