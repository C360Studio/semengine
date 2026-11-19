package follower

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/pkg/errs"
)

// damagedSample is how many damaged entities Stuck names; the rest are counted.
const damagedSample = 10

// damagedReason is the envelope's failed_reasons key for recorded damaged entities.
const damagedReason = "damaged_entity"

// damagedEntry is one damaged entity: the highest revision a plug-in reported for it and the
// refusal it read there.
type damagedEntry struct {
	revision uint64
	refusal  error
}

// damagedRecord holds the damaged entities, one entry per key. A plug-in reports from its delivery
// and from repair lanes of its own, and queries read it, so every access holds the lock.
type damagedRecord struct {
	mu      sync.Mutex
	entries map[string]damagedEntry
}

// ReportDamaged records that key's stored value at revision, the revision the plug-in read, is
// damaged: refusal is what graph.UnmarshalEntityState returned for it. The entry is kept at the
// highest revision reported and dropped when the plug-in completes a higher revision of the key, or
// a delete of the key is delivered above it, so a valid write or a delete clears it with no
// restart. A report never withholds a query: the plug-in removes what it built from the entity's
// earlier value, completes the revision, and replies count the entity as left out.
//
// It returns true only when the key was not recorded before, so the plug-in counts one error per
// newly damaged entity, however often its repair reads the same damage.
func (f *Follower) ReportDamaged(key string, revision uint64, refusal error) bool {
	if refusal == nil {
		// A report with no refusal still names damage; failing closed keeps it recorded and readable.
		refusal = errs.ClassifiedCode(errs.ErrorFatal, graph.ErrorCodeGraphStateResetRequired,
			&graph.StateContractError{Reason: graph.GraphStateReasonUnreadableEntity, EntityID: key,
				Err: errors.New("reported damaged with no refusal")})
	}
	d := &f.damaged
	d.mu.Lock()
	defer d.mu.Unlock()
	current, recorded := d.entries[key]
	if recorded && revision < current.revision {
		return false
	}
	if d.entries == nil {
		d.entries = map[string]damagedEntry{}
	}
	d.entries[key] = damagedEntry{revision: revision, refusal: refusal}
	return !recorded
}

// clearDamaged drops key's entry when revision, completed or deleted, is above the reported one.
func (f *Follower) clearDamaged(key string, revision uint64) {
	d := &f.damaged
	d.mu.Lock()
	defer d.mu.Unlock()
	if current, recorded := d.entries[key]; recorded && revision > current.revision {
		delete(d.entries, key)
	}
}

// DamagedCount returns how many damaged entities are recorded, of all of them when keep is nil, or
// of those whose ID keep returns true for. A reply counts with it what it may have left out.
func (f *Follower) DamagedCount(keep func(id string) bool) int {
	d := &f.damaged
	d.mu.Lock()
	defer d.mu.Unlock()
	if keep == nil {
		return len(d.entries)
	}
	n := 0
	for id := range d.entries {
		if keep(id) {
			n++
		}
	}
	return n
}

// DamagedRefusal returns the refusal recorded for id, or nil when id is not recorded damaged. A
// query that names a damaged entity answers with it.
func (f *Follower) DamagedRefusal(id string) error {
	d := &f.damaged
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.entries[id].refusal
}

// damagedSentence is Stuck's sentence for the recorded damaged entities, or "" when none is: the
// count, the first damagedSample IDs in sorted order with their reasons, the number not named, and
// the repair.
func (f *Follower) damagedSentence() string {
	d := &f.damaged
	d.mu.Lock()
	defer d.mu.Unlock()
	total := len(d.entries)
	if total == 0 {
		return ""
	}
	ids := slices.Sorted(maps.Keys(d.entries))
	named := make([]string, 0, min(total, damagedSample))
	for _, id := range ids[:min(total, damagedSample)] {
		named = append(named, fmt.Sprintf("%s (%s)", id, damagedReasonOf(d.entries[id].refusal)))
	}
	noun := "entities"
	if total == 1 {
		noun = "entity"
	}
	sentence := fmt.Sprintf("%d damaged %s: %s", total, noun, strings.Join(named, ", "))
	if total > damagedSample {
		sentence += fmt.Sprintf(", and %d more", total-damagedSample)
	}
	return sentence + "; write a valid value for each key or delete it"
}

// damagedReasonOf is the refusal's bounded reason; a refusal that carries none reads as a value
// that does not decode.
func damagedReasonOf(refusal error) graph.StateResetReason {
	var stateErr *graph.StateContractError
	if errors.As(refusal, &stateErr) {
		return stateErr.Reason
	}
	return graph.GraphStateReasonUnreadableEntity
}
