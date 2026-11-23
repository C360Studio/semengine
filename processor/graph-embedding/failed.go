package graphembedding

import (
	"slices"
	"sync"

	"github.com/c360studio/semengine/graph/embedding"
)

// strandedRevision is the revision of a failure no source revision names: a failed entity read,
// or a failed record delete for an entity found absent. No terminal outcome from the Worker
// clears or replaces it; only hop 1's convergence of the entity does.
const strandedRevision = ^uint64(0)

// outcome is one entity's entry in the failed map: the source revision of its latest terminal
// outcome and, for a failure, its bounded reason; stranded when hop 1 had no revision for it.
type outcome struct {
	// revision is the fence below which a Worker outcome is not counted. A stranded failure
	// names no revision of its own and keeps the one of the outcome it replaced.
	revision uint64
	reason   string // empty for a success
	stranded bool
	// waiting is set while a pending record written after the Worker's failure, or a failed
	// record the Worker retries at Start, waits for the Worker: its outcome at or above waitFor
	// ends the wait. A retry is not sent again meanwhile.
	waiting bool
	waitFor uint64
}

// hopOne reports whether o is a failure of hop 1's own, the kind hop 1's convergence clears.
func (o outcome) hopOne() bool { return repaired(o.reason) }

// retry reports whether the repair lane re-drives o: a Worker failure whose reason names a fault
// that may clear on its own, with no pending record waiting for the Worker. Hop 1's own failures
// are not retryable reasons; the lane repairs them on every pass.
func (o outcome) retry() bool {
	return o.reason != "" && !o.stranded && !o.waiting && embedding.RetryableReason(o.reason)
}

// failedMap is the per-entity failed map: the latest terminal outcome of each entity, so a
// failure older than it is not counted. The Worker's outcomes reach it on the Worker's goroutines,
// hop 1's on the follower's or the coalescer's under hop1Mu, and the follower reads it on its
// heartbeat, so every method takes mu. Lock order: hop1Mu, then mu. The zero value is empty and
// ready to use.
type failedMap struct {
	mu      sync.Mutex
	entries map[string]outcome
	// failures holds the entries that are failures, so counts and repairTargets walk only those
	// and never every entity embedded. store and forget keep it in step with entries.
	failures map[string]outcome
	// stored is set when the Worker stores a vector for an entity that was failed, and read and
	// cleared by each pass's retryTargets: the provider answers again, so every retry is sent.
	stored bool
	// lastRetried is the last entity, in entity ID order, the latest pass returned; a pass that
	// returns one picks the one after it, so one entity whose own text keeps failing never holds
	// back the rest.
	lastRetried string
}

// repaired reports whether reason is one of hop 1's own failures, the three the repair lane
// re-drives on every pass. They are in memory only and never in a stored record
// (graph/embedding/derived_reasons.go), so no Worker outcome carries one.
func repaired(reason string) bool {
	switch reason {
	case embedding.ReasonDeleteFailed, embedding.ReasonPendingWriteFailed, embedding.ReasonEntityReadFailed:
		return true
	}
	return false
}

// seed fills the map from the failed records EMBEDDING_INDEX holds at Start
// (Storage.ScanFailed), each as the Worker's failure at its record's revision. Each waits: the
// Worker retries every failed record in its first snapshot and reports the outcome.
func (m *failedMap) seed(entries []embedding.FailedEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range entries {
		m.worker(e.EntityID, outcome{revision: e.SourceRevision, reason: e.Reason, waiting: true, waitFor: e.SourceRevision})
	}
}

// failed records the Worker's failure of key at revision and reports whether key entered the
// failed set: it was not failed before, and the failure is not below its latest outcome.
func (m *failedMap) failed(key string, revision uint64, reason string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.worker(key, outcome{revision: revision, reason: reason})
}

// succeeded records the Worker's success of key at revision, of kind generated, skipped or
// deleted. A success at strandedRevision is the Worker's drain of a record it cannot decode
// (graph/embedding worker.go), which names no source revision: it forgets an entity that is not
// stranded, so it never stands as a latest outcome no later failure can pass. A vector stored for
// a failed entity, the one kind that shows the provider answers, is noted for the next pass.
func (m *failedMap) succeeded(key string, revision uint64, kind embedding.TerminalOutcome) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if revision == strandedRevision {
		if !m.entries[key].stranded {
			m.forget(key)
		}
		return
	}
	_, wasFailed := m.failures[key]
	m.worker(key, outcome{revision: revision})
	if _, still := m.failures[key]; wasFailed && !still && kind == embedding.OutcomeGenerated {
		m.stored = true
	}
}

// worker records, with mu held, the Worker's outcome o when it is at or above the entity's
// latest one: a failure below it is not counted, and a success clears a failure at or below it.
// A stranded failure stays: no Worker outcome names the revision it lacks. An outcome below the
// revision a pending record waits on leaves the wait standing. It reports whether key entered the
// failed set.
func (m *failedMap) worker(key string, o outcome) bool {
	held, ok := m.entries[key]
	if ok && (held.stranded || o.revision < held.revision) {
		return false
	}
	if held.waiting && o.revision < held.waitFor {
		o.waiting, o.waitFor = true, held.waitFor
	}
	return m.store(key, o)
}

// mark records hop 1's own failure of key, under hop1Mu: at the revision hop 1 read or was
// delivered, or stranded at strandedRevision when it has none. It replaces a stranded failure, so
// a read that succeeds before a write that fails re-marks the entity at the revision read, and
// otherwise is not stored below the latest outcome. It reports whether key entered the failed set.
func (m *failedMap) mark(key string, revision uint64, reason string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	held, ok := m.entries[key]
	if revision == strandedRevision {
		return m.store(key, outcome{revision: held.revision, reason: reason, stranded: true})
	}
	if ok && !held.stranded && revision < held.revision {
		return false
	}
	return m.store(key, outcome{revision: revision, reason: reason})
}

// queueing records, under hop1Mu, that hop 1 is about to write key's pending record for revision.
// A Worker failure of key then waits for the Worker's outcome of that record, so the repair lane
// does not send it again meanwhile. It is set before the write, not after, because the Worker can
// report the record's outcome before the write's call returns. A write that fails, or finds a
// generated record standing, replaces the entry (mark, ended), which ends the wait.
func (m *failedMap) queueing(key string, revision uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if held, ok := m.failures[key]; ok && !held.hopOne() {
		if !held.waiting || revision > held.waitFor {
			held.waitFor = revision
		}
		held.waiting = true
		m.store(key, held)
	}
}

// queued records, under hop1Mu, that hop 1 wrote key's pending record for revision: hop 1 has
// converged, so its own failure of key clears; the Worker owns the terminal outcome.
func (m *failedMap) queued(key string, revision uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if held, ok := m.entries[key]; ok && held.hopOne() {
		m.store(key, outcome{revision: revision})
	}
}

// ended records, under hop1Mu, that hop 1 ended key at revision: its record removed, or a
// generated one found standing. Hop 1's own failure clears, stranded or not; otherwise it is a
// success at revision, as the Worker's at a revision.
func (m *failedMap) ended(key string, revision uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if held, ok := m.entries[key]; ok && held.hopOne() {
		m.store(key, outcome{revision: revision})
		return
	}
	m.worker(key, outcome{revision: revision})
}

// absent records, under hop1Mu, that key is absent from ENTITY_STATES and its record is
// removed: nothing is left to count, but the entity's revision stays as a success. A Worker
// failure stored before the deletion can still have its callback to come; it is older than the
// tombstone that ended the entity, so it is not counted. A re-created entity's revisions are
// above the tombstone's (KV revisions are the stream's sequence), so the fence never hides them.
// With no outcome, or one at strandedRevision, which names no revision, the entity is forgotten.
func (m *failedMap) absent(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	held, ok := m.entries[key]
	if !ok || held.revision == strandedRevision {
		m.forget(key)
		return
	}
	m.store(key, outcome{revision: held.revision})
}

// store keeps o as key's latest outcome and reports whether key entered the failed set: o is a
// failure and key held none.
func (m *failedMap) store(key string, o outcome) bool {
	if m.entries == nil {
		m.entries = make(map[string]outcome)
		m.failures = make(map[string]outcome)
	}
	m.entries[key] = o
	if o.reason == "" {
		delete(m.failures, key)
		return false
	}
	_, held := m.failures[key]
	m.failures[key] = o
	return !held
}

func (m *failedMap) forget(key string) {
	delete(m.entries, key)
	delete(m.failures, key)
}

// counts is the follower's Failed: the number of failed entities and a fresh map of them per
// reason, nil when there are none. The follower owns the map.
func (m *failedMap) counts() (int64, map[string]uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var reasons map[string]uint64
	var n int64
	for _, o := range m.failures {
		if reasons == nil {
			reasons = make(map[string]uint64)
		}
		reasons[o.reason]++
		n++
	}
	return n, reasons
}

// withheld is the follower's Withhold: the number of entities whose failure is hop 1's own, the
// failures that withhold the query verbs.
func (m *failedMap) withheld() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for _, o := range m.failures {
		if o.hopOne() {
			n++
		}
	}
	return n
}

// repairTargets returns, sorted, the entities whose failure is hop 1's own: the repair lane
// re-drives every one on each pass. It releases mu before the caller takes hop1Mu.
func (m *failedMap) repairTargets() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var keys []string
	for key, o := range m.failures {
		if o.hopOne() {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	return keys
}

// retryTargets returns, sorted, the Worker failures this repair pass re-drives (outcome.retry).
// When the Worker stored a vector for a failed entity since the previous pass, the provider
// answers again and every one is returned. Otherwise only one is, the next in entity ID order
// after the last one the previous pass returned, wrapping, so a provider still down is sent one
// request per pass. It releases mu before the caller takes hop1Mu.
func (m *failedMap) retryTargets() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	all := m.stored
	m.stored = false
	var keys []string
	for key, o := range m.failures {
		if o.retry() {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	slices.Sort(keys)
	if all {
		m.lastRetried = keys[len(keys)-1]
		return keys
	}
	next := keys[0]
	if i, found := slices.BinarySearch(keys, m.lastRetried); found && i+1 < len(keys) {
		next = keys[i+1]
	} else if !found && i < len(keys) {
		next = keys[i]
	}
	m.lastRetried = next
	return []string{next}
}

// retryable reports whether key is still one the repair lane re-drives, under hop1Mu: a delivery
// can write its pending record between retryTargets and the lane taking hop1Mu.
func (m *failedMap) retryable(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.failures[key].retry()
}
