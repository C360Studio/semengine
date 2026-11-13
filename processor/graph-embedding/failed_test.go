package graphembedding

import (
	"fmt"
	"maps"
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/c360studio/semengine/graph/embedding"
)

// The reference model of the failed map, written from the graph-embedding spec
// ("The failed map never moves backwards"), not from failed.go: per entity, its latest terminal
// outcome, success or failure, with the failure's reason, or a stranded failure (no source
// revision). The rules:
//
//   - A Worker outcome at r is kept only when r is at or above the latest outcome's revision; a
//     stranded failure is never replaced or cleared by the Worker.
//   - The Worker's drain of an unreadable record (a success at ^uint64(0), graph/embedding
//     worker.go) names no source revision, so it is no latest outcome: the entity is forgotten,
//     unless its failure is stranded.
//   - Hop 1's own failure (delete_failed, pending_write_failed, entity_read_failed) with no
//     revision is stranded; with one it is kept at or above the latest outcome, and it replaces a
//     stranded one ("a read that succeeds while the following pending write fails re-marks").
//   - Hop 1's convergence clears hop 1's own failure, stranded or not: a pending record written
//     or a revision ended (record removed, generated record found) is then the latest outcome. An
//     ended revision otherwise counts as a success, as the Worker's; a written pending record
//     changes nothing else.
//   - An absent entity whose record is removed has no failure, but its latest outcome's revision
//     stays: a Worker failure for an older revision can still arrive after
//     the absence, and it is not counted. A stranded failure keeps the revision of the outcome it
//     replaced, so the fence survives it too. A fence at ^uint64(0) names no revision, and an
//     entity with no outcome has none: either is forgotten.
//   - Count and reasons are over the failures held; the repair lane retries exactly hop 1's own.
type modelEntry struct {
	revision uint64
	failed   bool
	reason   string
	stranded bool
	ownFail  bool // a failure of hop 1's own
}

type failedModel map[string]modelEntry

var hopOneReasons = []string{embedding.ReasonDeleteFailed, embedding.ReasonPendingWriteFailed, embedding.ReasonEntityReadFailed}

func (m failedModel) worker(key string, e modelEntry) {
	if held, ok := m[key]; ok && (held.stranded || e.revision < held.revision) {
		return
	}
	m[key] = e
}

func (m failedModel) drained(key string) {
	if m[key].stranded {
		return
	}
	delete(m, key)
}

func (m failedModel) hopOneFailed(key string, revision uint64, reason string) {
	if revision == strandedRevision {
		m[key] = modelEntry{revision: m[key].revision, failed: true, reason: reason, stranded: true, ownFail: true}
		return
	}
	if held, ok := m[key]; ok && !held.stranded && revision < held.revision {
		return
	}
	m[key] = modelEntry{revision: revision, failed: true, reason: reason, ownFail: true}
}

func (m failedModel) absent(key string) {
	held, ok := m[key]
	if !ok || held.revision == strandedRevision {
		delete(m, key)
		return
	}
	m[key] = modelEntry{revision: held.revision}
}

func (m failedModel) hopOneConverged(key string, revision uint64, ended bool) {
	if held, ok := m[key]; ok && held.ownFail {
		m[key] = modelEntry{revision: revision}
		return
	}
	if ended {
		m.worker(key, modelEntry{revision: revision})
	}
}

// failedOp is one generated or named step, applied to the failed map and the model alike.
type failedOp struct {
	kind     string // "worker failed", "succeeded", "hop 1 failed", "queued", "ended", "absent", "seed"
	key      string
	revision uint64
	reason   string
	seed     []embedding.FailedEntry
}

func (op failedOp) String() string {
	rev := fmt.Sprint(op.revision)
	if op.revision == strandedRevision {
		rev = "stranded"
	}
	if op.kind == "seed" {
		return fmt.Sprintf("seed %v", op.seed)
	}
	return fmt.Sprintf("%s %s@%s %s", op.kind, op.key, rev, op.reason)
}

// apply applies op to m and model. For a failure (the Worker's or hop 1's) it returns whether m
// reported that op.key entered the failed set and whether the model says it did: it held no
// failure before op and holds one after. graph-embedding counts an error on exactly that report.
func (op failedOp) apply(m *failedMap, model failedModel) (entered, wantEntered, failure bool) {
	wasFailed := model[op.key].failed
	defer func() {
		if failure {
			wantEntered = !wasFailed && model[op.key].failed
		}
	}()
	switch op.kind {
	case "seed":
		m.seed(op.seed)
		for _, e := range op.seed {
			model.worker(e.EntityID, modelEntry{revision: e.SourceRevision, failed: true, reason: e.Reason})
		}
	case "worker failed":
		entered, failure = m.failed(op.key, op.revision, op.reason), true
		model.worker(op.key, modelEntry{revision: op.revision, failed: true, reason: op.reason})
	case "succeeded":
		m.succeeded(op.key, op.revision)
		if op.revision == strandedRevision {
			model.drained(op.key)
		} else {
			model.worker(op.key, modelEntry{revision: op.revision})
		}
	case "hop 1 failed":
		entered, failure = m.mark(op.key, op.revision, op.reason), true
		model.hopOneFailed(op.key, op.revision, op.reason)
	case "queued":
		m.queued(op.key, op.revision)
		model.hopOneConverged(op.key, op.revision, false)
	case "ended":
		m.ended(op.key, op.revision)
		model.hopOneConverged(op.key, op.revision, true)
	case "absent":
		m.absent(op.key)
		model.absent(op.key)
	}
	return entered, wantEntered, failure
}

// checkHistory applies ops in order and, after each, compares what the failed map shows (the
// follower's count and reasons, the repair lane's targets, and for a failure whether the entity
// entered the failed set) with the model. It also edits the
// reasons map counts returned, as the follower may, and requires the next read to be unchanged.
func checkHistory(t interface {
	Helper()
	Fatalf(string, ...any)
}, ops []failedOp) {
	t.Helper()
	var m failedMap
	model := failedModel{}
	for i, op := range ops {
		entered, wantEntered, failure := op.apply(&m, model)
		if failure && entered != wantEntered {
			t.Fatalf("after step %d (%v) of %v: entered the failed set = %t; want %t",
				i, op, ops, entered, wantEntered)
		}
		count, reasons := m.counts()
		wantReasons := map[string]uint64{}
		var wantTargets []string
		var wantCount int64
		for key, e := range model {
			if !e.failed {
				continue
			}
			wantCount++
			wantReasons[e.reason]++
			if e.ownFail {
				wantTargets = append(wantTargets, key)
			}
		}
		slices.Sort(wantTargets)
		if count != wantCount || !maps.Equal(reasons, wantReasons) {
			t.Fatalf("after step %d (%v) of %v: count %d, reasons %v; want %d, %v",
				i, op, ops, count, reasons, wantCount, wantReasons)
		}
		if count == 0 && reasons != nil {
			t.Fatalf("after step %d (%v): no failures but reasons %v; want nil", i, op, reasons)
		}
		if targets := m.repairTargets(); !slices.Equal(targets, wantTargets) {
			t.Fatalf("after step %d (%v) of %v: repair targets %v; want %v", i, op, ops, targets, wantTargets)
		}
		if reasons != nil {
			reasons["edited by the follower"] = 1
			if _, again := m.counts(); !maps.Equal(again, wantReasons) {
				t.Fatalf("after step %d: editing the returned reasons changed the map's: %v", i, again)
			}
		}
	}
}

// failedWitnesses are the named histories run before the generated ones, each reaching an edge
// value every time (docs/testing.md, "Show that each assertion runs").
var failedWitnesses = map[string][]failedOp{
	"a failure below a held failure is not stored": {
		{kind: "worker failed", key: "a", revision: 3, reason: "timeout"},
		{kind: "worker failed", key: "a", revision: 2, reason: "content_error"},
	},
	"a success below, at and above a failure": {
		{kind: "worker failed", key: "a", revision: 2, reason: "timeout"},
		{kind: "succeeded", key: "a", revision: 1},
		{kind: "succeeded", key: "a", revision: 2},
		{kind: "worker failed", key: "a", revision: 2, reason: "timeout"},
		{kind: "succeeded", key: "a", revision: 3},
	},
	"no Worker outcome clears or replaces a stranded failure": {
		{kind: "hop 1 failed", key: "a", revision: strandedRevision, reason: embedding.ReasonEntityReadFailed},
		{kind: "succeeded", key: "a", revision: 7},
		{kind: "succeeded", key: "a", revision: strandedRevision}, // the Worker's drain of an unreadable record
		{kind: "worker failed", key: "a", revision: strandedRevision, reason: "timeout"},
		{kind: "worker failed", key: "a", revision: 9, reason: "timeout"},
	},
	"hop 1's convergence clears its own failures only": {
		{kind: "hop 1 failed", key: "a", revision: strandedRevision, reason: embedding.ReasonDeleteFailed},
		{kind: "worker failed", key: "b", revision: 4, reason: "embedder_error"},
		{kind: "queued", key: "b", revision: 5},
		{kind: "queued", key: "a", revision: 5},
		{kind: "worker failed", key: "a", revision: 4, reason: "timeout"}, // below the queued revision
		{kind: "hop 1 failed", key: "c", revision: strandedRevision, reason: embedding.ReasonEntityReadFailed},
		{kind: "ended", key: "c", revision: 2},
		{kind: "hop 1 failed", key: "c", revision: strandedRevision, reason: embedding.ReasonDeleteFailed},
		{kind: "absent", key: "c"},
	},
	// The spec's scenario "A late failure for an older revision".
	"a late failure for an older revision is not counted": {
		{kind: "succeeded", key: "a", revision: 7},
		{kind: "worker failed", key: "a", revision: 5, reason: "timeout"},
		{kind: "ended", key: "b", revision: 7},
		{kind: "worker failed", key: "b", revision: 5, reason: "timeout"},
		{kind: "hop 1 failed", key: "b", revision: 6, reason: embedding.ReasonPendingWriteFailed},
	},
	"a read that succeeds before a failed write re-marks the entity": {
		{kind: "hop 1 failed", key: "a", revision: strandedRevision, reason: embedding.ReasonEntityReadFailed},
		{kind: "hop 1 failed", key: "a", revision: 5, reason: embedding.ReasonPendingWriteFailed},
		{kind: "succeeded", key: "a", revision: 4},
		{kind: "succeeded", key: "a", revision: 5},
	},
	// Found by a generated run: stranded is hop 1's having no revision, not the value of one.
	"a Worker failure at the highest revision is not stranded": {
		{kind: "worker failed", key: "a", revision: strandedRevision, reason: "timeout"},
		{kind: "hop 1 failed", key: "a", revision: 0, reason: embedding.ReasonDeleteFailed},
		{kind: "succeeded", key: "a", revision: strandedRevision},
	},
	// A drain stored as a success at the highest revision would hide every later failure of
	// the entity.
	"a drain forgets an entity that is not stranded": {
		{kind: "worker failed", key: "a", revision: 3, reason: "timeout"},
		{kind: "succeeded", key: "a", revision: strandedRevision}, // the Worker's drain of an unreadable record
		{kind: "hop 1 failed", key: "a", revision: 5, reason: embedding.ReasonPendingWriteFailed},
		{kind: "worker failed", key: "a", revision: 9, reason: "timeout"},
		{kind: "succeeded", key: "b", revision: strandedRevision},
		{kind: "worker failed", key: "b", revision: 2, reason: "timeout"},
	},
	// The Worker's failure at 4 is stored, its
	// callback still to come; the tombstone at 5 ends the entity, a batch extracted before it
	// reads the entity absent, and then the failure's callback arrives.
	"an absence keeps the fence against an older failure": {
		{kind: "ended", key: "a", revision: 5},
		{kind: "absent", key: "a"},
		{kind: "worker failed", key: "a", revision: 4, reason: "content_error"},
		{kind: "hop 1 failed", key: "b", revision: 5, reason: embedding.ReasonDeleteFailed},
		{kind: "absent", key: "b"},
		{kind: "worker failed", key: "b", revision: 4, reason: "timeout"},
		{kind: "worker failed", key: "b", revision: 6, reason: "timeout"}, // a re-created entity's failure counts
	},
	"a stranded failure keeps the fence it replaced": {
		{kind: "ended", key: "a", revision: 5},
		{kind: "hop 1 failed", key: "a", revision: strandedRevision, reason: embedding.ReasonEntityReadFailed},
		{kind: "absent", key: "a"},
		{kind: "worker failed", key: "a", revision: 4, reason: "timeout"},
		{kind: "hop 1 failed", key: "b", revision: strandedRevision, reason: embedding.ReasonDeleteFailed},
		{kind: "absent", key: "b"},
		{kind: "worker failed", key: "b", revision: 0, reason: "timeout"},
		{kind: "worker failed", key: "c", revision: strandedRevision, reason: "timeout"},
		{kind: "absent", key: "c"},
		{kind: "worker failed", key: "c", revision: 2, reason: "timeout"},
	},
	"hop 1's failure at a revision follows revision order": {
		{kind: "worker failed", key: "a", revision: 6, reason: "timeout"},
		{kind: "hop 1 failed", key: "a", revision: 5, reason: embedding.ReasonDeleteFailed},
		{kind: "hop 1 failed", key: "a", revision: 6, reason: embedding.ReasonDeleteFailed},
		{kind: "worker failed", key: "a", revision: 7, reason: "timeout"},
	},
	"seeded failures, then none": {
		{kind: "seed", seed: []embedding.FailedEntry{
			{EntityID: "a", Reason: "timeout", SourceRevision: 2}, {EntityID: "b", Reason: "unknown", SourceRevision: 0},
		}},
		{kind: "succeeded", key: "a", revision: 1},
		{kind: "succeeded", key: "b", revision: 0},
		{kind: "succeeded", key: "a", revision: 2},
		{kind: "seed"},
	},
	// An entity is counted once when it enters the
	// failed set, not on each failure while it is failed, and not on a failure that is refused.
	"an entity enters the failed set once": {
		{kind: "hop 1 failed", key: "a", revision: strandedRevision, reason: embedding.ReasonEntityReadFailed},
		{kind: "hop 1 failed", key: "a", revision: strandedRevision, reason: embedding.ReasonEntityReadFailed},
		{kind: "hop 1 failed", key: "a", revision: 4, reason: embedding.ReasonPendingWriteFailed},
		{kind: "worker failed", key: "a", revision: 5, reason: "timeout"},
		{kind: "succeeded", key: "a", revision: 6},
		{kind: "worker failed", key: "a", revision: 5, reason: "timeout"},
		{kind: "hop 1 failed", key: "a", revision: 5, reason: embedding.ReasonDeleteFailed},
		{kind: "worker failed", key: "a", revision: 6, reason: "timeout"},
	},
	"repair targets leave the Worker's failures out": {
		{kind: "worker failed", key: "a", revision: 1, reason: "connection_refused"},
		{kind: "hop 1 failed", key: "b", revision: 1, reason: embedding.ReasonPendingWriteFailed},
		{kind: "hop 1 failed", key: "c", revision: strandedRevision, reason: embedding.ReasonEntityReadFailed},
	},
}

// TestFailedMapHistoryMatchesModel: every history of Worker outcomes, hop 1 failures and
// convergences, and a Start seed leaves the failed map showing what the model says:
// the count and reasons the follower publishes, and the entities the repair lane retries. The
// named witnesses run first; the generated histories draw revisions 0 to 3 and stranded, so
// below, at and above a held revision are each one draw apart, over three entities.
//
// Replay: go test ./processor/graph-embedding -run '^TestFailedMapHistoryMatchesModel$' -count=1
// -v -rapid.checks=1000 -rapid.seed=<seed>.
func TestFailedMapHistoryMatchesModel(t *testing.T) {
	for _, name := range slices.Sorted(maps.Keys(failedWitnesses)) {
		t.Run(name, func(t *testing.T) { checkHistory(t, failedWitnesses[name]) })
	}
	t.Run("generated", func(t *testing.T) {
		rapid.Check(t, func(rt *rapid.T) {
			keys := []string{"a", "b", "c"}
			revisions := []uint64{0, 1, 2, 3, strandedRevision}
			workerReasons := []string{"timeout", "content_error", "unknown"}
			var ops []failedOp
			if rapid.Bool().Draw(rt, "seed at start?") {
				var seed []embedding.FailedEntry
				for _, key := range rapid.SliceOfDistinct(rapid.SampledFrom(keys), rapid.ID[string]).Draw(rt, "seeded") {
					seed = append(seed, embedding.FailedEntry{EntityID: key,
						Reason:         rapid.SampledFrom(workerReasons).Draw(rt, "seeded reason"),
						SourceRevision: rapid.Uint64Range(0, 3).Draw(rt, "seeded revision")})
				}
				ops = append(ops, failedOp{kind: "seed", seed: seed})
			}
			for range rapid.IntRange(1, 30).Draw(rt, "steps") {
				op := failedOp{
					kind: rapid.SampledFrom([]string{"worker failed", "succeeded", "hop 1 failed", "queued", "ended", "absent"}).Draw(rt, "kind"),
					key:  rapid.SampledFrom(keys).Draw(rt, "key"),
				}
				switch op.kind {
				case "worker failed":
					op.revision = rapid.SampledFrom(revisions).Draw(rt, "revision")
					op.reason = rapid.SampledFrom(workerReasons).Draw(rt, "reason")
				case "succeeded", "queued", "ended":
					op.revision = rapid.SampledFrom(revisions).Draw(rt, "revision")
				case "hop 1 failed":
					op.revision = rapid.SampledFrom(revisions).Draw(rt, "revision")
					op.reason = rapid.SampledFrom(hopOneReasons).Draw(rt, "reason")
				}
				ops = append(ops, op)
			}
			checkHistory(rt, ops)
		})
	})
}
