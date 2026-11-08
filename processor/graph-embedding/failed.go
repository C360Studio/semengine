package graphembedding

import (
	"slices"
	"sync"

	"github.com/c360studio/semengine/graph/embedding"
)

// strandedRevision is the revision of a failure no source revision names: a failed entity read,
// or a failed record delete for an entity found absent (design D3). No terminal outcome from the
// Worker clears or replaces it; only hop 1's convergence of the entity does.
const strandedRevision = ^uint64(0)

// outcome is one entity's entry in the failed map: the source revision of its latest terminal
// outcome and, for a failure, its bounded reason; stranded when hop 1 had no revision for it.
type outcome struct {
	revision uint64
	reason   string // empty for a success
	stranded bool
}

// hopOne reports whether o is a failure of hop 1's own, the kind hop 1's convergence clears.
func (o outcome) hopOne() bool { return repaired(o.reason) }

// failedMap is the per-entity failed map (design D3; pin component.go:1101-1157, :1657-1701): the
// latest terminal outcome of each entity, so a failure older than it is not counted. The Worker's
// outcomes reach it on the Worker's goroutines, hop 1's on the follower's or the coalescer's
// under hop1Mu, and the follower reads it on its heartbeat, so every method takes mu. Lock
// order: hop1Mu, then mu. The zero value is empty and ready to use.
type failedMap struct {
	mu      sync.Mutex
	entries map[string]outcome
	// failures holds the entries that are failures, so counts and repairTargets walk only those
	// and never every entity embedded. store and forget keep it in step with entries.
	failures map[string]outcome
}

// repaired reports whether reason is one of hop 1's own failures, the three the repair lane
// re-drives (pin component.go:1290-1300). They are in memory only and never in a stored record
// (graph/embedding/derived_reasons.go), so no Worker outcome carries one.
func repaired(reason string) bool {
	switch reason {
	case embedding.ReasonDeleteFailed, embedding.ReasonPendingWriteFailed, embedding.ReasonEntityReadFailed:
		return true
	}
	return false
}

// seed fills the map from the failed records EMBEDDING_INDEX holds at Start
// (Storage.ScanFailed), each as the Worker's failure at its record's revision.
func (m *failedMap) seed(entries []embedding.FailedEntry) {
	for _, e := range entries {
		m.failed(e.EntityID, e.SourceRevision, e.Reason)
	}
}

// failed records the Worker's failure of key at revision.
func (m *failedMap) failed(key string, revision uint64, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.worker(key, outcome{revision: revision, reason: reason})
}

// succeeded records the Worker's success of key at revision. A success at strandedRevision is
// the Worker's drain of a record it cannot decode (graph/embedding worker.go), which names no
// source revision: it forgets an entity that is not stranded, as the pin does (pin
// component.go:1152), so it never stands as a latest outcome no later failure can pass.
func (m *failedMap) succeeded(key string, revision uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if revision == strandedRevision {
		if !m.entries[key].stranded {
			m.forget(key)
		}
		return
	}
	m.worker(key, outcome{revision: revision})
}

// worker records, with mu held, the Worker's outcome o when it is at or above the entity's
// latest one: a failure below it is not counted, and a success clears a failure at or below it.
// A stranded failure stays: no Worker outcome names the revision it lacks.
func (m *failedMap) worker(key string, o outcome) {
	held, ok := m.entries[key]
	if ok && (held.stranded || o.revision < held.revision) {
		return
	}
	m.store(key, o)
}

// mark records hop 1's own failure of key, under hop1Mu: at the revision hop 1 read or was
// delivered, or stranded at strandedRevision when it has none. It replaces a stranded failure, so
// a read that succeeds before a write that fails re-marks the entity at the revision read (pin
// component.go:1864-1884, :1911-1941), and otherwise is not stored below the latest outcome.
func (m *failedMap) mark(key string, revision uint64, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	held, ok := m.entries[key]
	if ok && !held.stranded && revision < held.revision {
		return
	}
	m.store(key, outcome{revision: revision, reason: reason, stranded: revision == strandedRevision})
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
// removed: nothing is left to count, and with no revision read there is no outcome to keep.
func (m *failedMap) absent(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.forget(key)
}

func (m *failedMap) store(key string, o outcome) {
	if m.entries == nil {
		m.entries = make(map[string]outcome)
		m.failures = make(map[string]outcome)
	}
	m.entries[key] = o
	if o.reason == "" {
		delete(m.failures, key)
	} else {
		m.failures[key] = o
	}
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

// repairTargets returns, sorted, the entities whose failure is hop 1's own: the repair lane
// re-drives those, and the Worker's failures wait for a new revision or a restart (pin
// TestRepairScope_EmbedderSideReasonNotReDriven). It releases mu before the caller takes hop1Mu.
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
