package follower

import (
	"errors"
	"fmt"
	"strings"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/pkg/errs"
)

// damagedSample is how many damaged entities Stuck names; the rest are counted.
const damagedSample = 10

// damagedReason is the envelope's failed_reasons key for recorded damaged entities.
const damagedReason = "damaged_entity"

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
		// A report with no refusal still names damage; failing closed keeps it recorded and readable,
		// with the key in its detail as the decoder's refusal has it.
		refusal = errs.ClassifiedCodeDetail(errs.ErrorFatal, graph.ErrorCodeGraphStateResetRequired,
			map[string]any{"entity_ids": []string{key}},
			&graph.StateContractError{Reason: graph.GraphStateReasonUnreadableEntity, EntityID: key,
				Err: errors.New("reported damaged with no refusal")})
	}
	return f.damaged.Report(key, revision, refusal)
}

// clearDamaged drops key's entry when revision, completed or deleted, is above the reported one.
func (f *Follower) clearDamaged(key string, revision uint64) {
	f.damaged.Clear(key, revision)
}

// DamagedCount returns how many damaged entities are recorded, of all of them when keep is nil, or
// of those whose ID keep returns true for. A reply counts with it what it may have left out.
func (f *Follower) DamagedCount(keep func(id string) bool) int {
	return f.damaged.Count(keep)
}

// DamagedRefusal returns the refusal recorded for id, or nil when id is not recorded damaged. A
// query that names a damaged entity answers with it.
func (f *Follower) DamagedRefusal(id string) error {
	entry, _ := f.damaged.Get(id)
	return entry.Refusal
}

// damagedSentence is Stuck's sentence for the recorded damaged entities, or "" when none is: the
// count, the first damagedSample IDs in sorted order with their reasons, the number not named, and
// the repair.
func (f *Follower) damagedSentence() string {
	// One read gives the count and the sample, so the two agree when a report lands meanwhile.
	entries := f.damaged.Sorted(0)
	total := len(entries)
	if total == 0 {
		return ""
	}
	named := make([]string, 0, min(total, damagedSample))
	for _, e := range entries[:min(total, damagedSample)] {
		named = append(named, fmt.Sprintf("%s (%s)", e.Key, damagedReasonOf(e.Refusal)))
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
