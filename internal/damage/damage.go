// Package damage records keys whose stored value could not be read, one entry per key, until a
// repair of that key is proven.
//
// A reader that finds a damaged value keeps serving every other key, so it needs a list of the
// damaged ones to report, count against replies and name in health messages. An entry keeps the
// highest revision reported for its key and the refusal read there. A repair clears it only when
// the repair's revision is above the recorded one: a later damaged write must not be erased by a
// repair of an earlier revision. The caller decides what counts as a repair and which bound it
// passes; this package only applies the revision rule.
package damage

import (
	"slices"
	"strings"
	"sync"
	"sync/atomic"
)

// Entry is one damaged key: the highest revision reported for it and the refusal read there.
type Entry struct {
	Key      string
	Revision uint64
	Refusal  error
}

// Record holds the damaged keys. The zero value is an empty record ready to use. It is safe for
// concurrent use and must not be copied after first use.
type Record struct {
	mu      sync.Mutex
	entries map[string]Entry
	// size mirrors len(entries), written under mu, so Len costs one atomic load and callers on
	// a hot path can skip the lock while nothing is recorded.
	size atomic.Int64
}

// Report records key as damaged at revision with refusal, unless the key is already recorded at
// a higher revision; an equal or higher report replaces the entry. It returns true only when the
// key was not recorded before, so a caller can count or log each newly damaged key once.
func (r *Record) Report(key string, revision uint64, refusal error) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, recorded := r.entries[key]
	if recorded && revision < current.Revision {
		return false
	}
	if r.entries == nil {
		r.entries = map[string]Entry{}
	}
	r.entries[key] = Entry{Key: key, Revision: revision, Refusal: refusal}
	r.size.Store(int64(len(r.entries)))
	return !recorded
}

// Clear drops key's entry when it was recorded at a revision below below, and returns whether it
// dropped one. A below of 0 drops nothing.
func (r *Record) Clear(key string, below uint64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, recorded := r.entries[key]
	if !recorded || current.Revision >= below {
		return false
	}
	delete(r.entries, key)
	r.size.Store(int64(len(r.entries)))
	return true
}

// Len returns how many keys are recorded.
func (r *Record) Len() int {
	return int(r.size.Load())
}

// Count returns how many recorded keys keep returns true for, or all of them when keep is nil.
func (r *Record) Count(keep func(id string) bool) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	if keep == nil {
		return len(r.entries)
	}
	n := 0
	for id := range r.entries {
		if keep(id) {
			n++
		}
	}
	return n
}

// Get returns key's entry and whether the key is recorded.
func (r *Record) Get(key string) (Entry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[key]
	return e, ok
}

// Sorted returns the first limit entries in key order, or all of them when limit is 0 or less.
func (r *Record) Sorted(limit int) []Entry {
	r.mu.Lock()
	out := make([]Entry, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, e)
	}
	r.mu.Unlock()
	slices.SortFunc(out, func(a, b Entry) int { return strings.Compare(a.Key, b.Key) })
	if limit > 0 && limit < len(out) {
		out = out[:limit]
	}
	return out
}
