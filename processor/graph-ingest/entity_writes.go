package graphingest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/retry"
	"github.com/c360studio/semengine/vocabulary"
)

// This file is graph-ingest's one write seam (design D15, #100): every write to the entity
// bucket (ENTITY_STATES) is made here, one method per write mode, so the rule each mode applies
// to a stored entity is read in one place. TestEntityWritesHaveOneSeam fails when the bucket is
// written from any other file.
//
// After each commit a method here does the bookkeeping every write owes: it clears
// the key's poison record at the revision it committed, or drops it on a delete, and invalidates
// the key's entity-query cache entry, in that order, before it returns. Callers keep their own
// validation, authority checks, metrics and replies; a method here takes what its mode needs and
// returns the revision it committed (where the caller uses it) and the bucket's error unwrapped,
// so callers classify it as before.
// Every lane that writes statements first applies the seam's statement-metadata rule,
// requireStatementMetadata, to the statements its caller gave.
//
// The modes, and the lanes on which each is used:
//
//   - create (createEntity): mutation create; in-process create (hierarchy containers).
//   - replace (replaceEntity): the stream lane. A birth when the key is absent (refused when the
//     lane's read before the hierarchy inference found the key; design D21), else
//     graph.ReplaceBySource: each (predicate, source) set the arrival carries replaces the stored
//     set of the same key, unless it is older, under the KV revision.
//   - conditional replace (reconcileCandidate, then replaceEntityAtRevision): mutation reconcile,
//     at the caller's expected revision.
//   - append (appendEntityTriples): mutation append; in-process append (hierarchy's inverse
//     edges). Identity is message.AppendIdentityKey, under the KV revision. An append that adds
//     nothing writes nothing and is unchanged at the revision it read.
//   - delete (deleteEntity): mutation delete, at the caller's expected revision.

// requireStatementMetadata is the seam's statement-metadata rule (design D15, #98): every
// statement the seam stores has a non-empty Source and a non-zero Timestamp, and neither is
// defaulted from the clock. It refuses the first of triples that lacks either as
// invalid_request, naming the statement's index and the missing field.
//
// Every lane applies it to the statements its caller gave, after its own validation and
// authority gate, before its first read or write of the bucket and before graph-ingest adds the
// statements it derives, so a refused write stores nothing and the index is the caller's: the
// mutation create, append and reconcile (handleCanonicalCreate, handleCanonicalAppend,
// handleCanonicalReconcile), the in-process create and append (createEntityWithReceipt,
// addTripleLane), and the stream lane (mergeEntityOnLane). A check inside the write methods below
// would come too late for both: an in-process create's hierarchy inference stores containers and
// edges before its write, and an append batch commits one subject at a time. On the stream lane,
// stampFromEnvelope has already filled both or refused the message, so the rule is a second line
// there.
func requireStatementMetadata(triples []message.Triple) error {
	for index := range triples {
		switch {
		case triples[index].Source == "":
			return refuseMissingMetadata(index, "source")
		case triples[index].Timestamp.IsZero():
			return refuseMissingMetadata(index, "timestamp")
		}
	}
	return nil
}

// requireOwnStatements is requireStatementMetadata for a write whose derived statements take
// their time from its own: a create on the mutation and in-process lanes, and the stream lane's
// merge. It also refuses a write that carries no statement: the statements graph-ingest derives
// take the latest Timestamp among the write's own, so with none there is no time to give them
// (design D15). On the stream lane extractEntityFromMessage has already refused such a message
// as poison, so the rule is a second line there.
func requireOwnStatements(triples []message.Triple) error {
	if len(triples) == 0 {
		return errs.ClassifiedCode(errs.ErrorInvalid, graph.ErrorCodeInvalidRequest,
			errors.New("triples cannot be empty: the statements graph-ingest derives take their time from the write's own"))
	}
	return requireStatementMetadata(triples)
}

func refuseMissingMetadata(index int, field string) error {
	return errs.ClassifiedCode(errs.ErrorInvalid, graph.ErrorCodeInvalidRequest,
		fmt.Errorf("triple[%d] has no %s", index, field))
}

// createEntity is the create mode: the bucket's atomic create-or-fail, the only admitted birth
// primitive. An existing key is refused with natsclient.ErrKVKeyExists, which is returned as is
// so each lane makes its own decision (entity_already_exists on the mutation lane).
func (c *Component) createEntity(ctx context.Context, entityID string, encoded []byte) (uint64, error) {
	revision, err := c.entityBucket.Create(ctx, entityID, encoded)
	if err != nil {
		return revision, err
	}
	c.committed(ctx, entityID, revision)
	return revision, nil
}

// replaceEntity is the stream lane's write: a read-modify-write under the KV revision
// (compare-and-set), so concurrent arrivals on the same subject converge without racing.
// When the key is absent the arrival is a birth: a copy of entity is written with
// hierarchyTriples appended and its profile stamped, unless readFound is set. readFound means
// the caller read the key before the hierarchy inference and found the entity, so it ran no
// inference; finding the key absent now means the entity was deleted in between, and a birth
// would lack its hierarchy. Nothing is written and the error is classified transient: the
// stream lane leaves the arrival unacknowledged, and its redelivery reads the key again (design
// D21). Otherwise each (predicate, source) set of
// entity's statements replaces the stored set of the same key (graph.ReplaceBySource), except a
// set older than the stored one, which is left out, counted on stale_sets_total and logged at
// debug level once the write commits or is declined (design D15, "Timestamp orders a replace").
// An arrival none of whose sets applies writes nothing over a profiled entity (design D23); over
// an unprofiled one it writes once, to stamp the profile (ADR-054). The bucket
// re-runs the callback after a create conflict or a retryable create error, so the birth never
// changes entity: a retry that finds the key present merges from the arrival as it came (#91,
// PR #93 comment 6060120246). A profile it stamps carries at, the arrival's triggeringTime. A
// stored value the seam's read check refuses (decodeStoredForWrite), or a merge the write gate
// refuses for a statement kept from it, writes nothing and is recorded at the revision read
// (design D23). It returns the revision it committed, or 0 when it wrote nothing, and the size
// of the value written.
func (c *Component) replaceEntity(ctx context.Context, entity *graph.EntityState, hierarchyTriples []message.Triple, readFound bool, at time.Time) (uint64, int, error) {
	var bytesWritten int
	// stale is the last attempt's sets not applied as older: the one that committed or declined.
	// It is assigned on each run of the callback, never accumulated, so a lost compare-and-set
	// does not count its sets twice.
	var stale []graph.StaleSet
	// casAttempt counts CAS-callback invocations; each re-run (attempt > 1) means
	// the prior revision-checked Put lost the CAS and retried (ADR-072
	// cas_retries — cross-entity contention observability, not a keying proof).
	casAttempt := 0
	// birth is the encoded birth. It depends only on the arrival, so it is built on the first
	// run that finds the key absent and reused by a retry, which then writes the same value.
	// It is not built before the loop: a merge never needs it, and stamping the profile fires
	// the default-profile metric.
	var birth []byte
	revision, err := c.entityBucket.UpdateWithRetryRead(ctx, entity.ID, func(current []byte, read uint64) ([]byte, error) {
		casAttempt++
		if casAttempt > 1 && c.casRetries != nil {
			c.casRetries.Inc()
		}
		stale = nil
		// First write: entity didn't exist (absence is revision 0, never an empty value; design
		// D23). Apply hierarchy triples (deterministic-per-ID so safe to apply once on create),
		// then store verbatim.
		if read == 0 {
			if readFound {
				// Not retried by the bucket (a callback error ends the call): the redelivery's
				// read runs the inference before the birth.
				return nil, errs.WrapTransient(
					fmt.Errorf("entity %s was deleted after the read that found it, so its hierarchy was not inferred", entity.ID),
					"Component", "replaceEntity", "birth entity")
			}
			if birth == nil {
				// A copy with its own statements: reconcileIndexingProfile filters in place.
				born := *entity
				born.Triples = slices.Concat(entity.Triples, hierarchyTriples)
				// ADR-054: first write is entity birth — stamp the profile
				// (explicit-if-declared via IndexingProfiler, else floor).
				c.reconcileIndexingProfile(&born, at)
				data, err := graph.MarshalEntityState(&born)
				if err != nil {
					return nil, err
				}
				birth = data
			}
			bytesWritten = len(birth)
			return birth, nil
		}
		// Existing entity: replace the arrival's sets, and take its message type and storage
		// reference when none of its sets was older than the stored one.
		// Hierarchy triples are NOT re-applied — they landed on the
		// original create and would only produce duplicates here.
		//
		// gh#562: the read check is the trusted decode plus the key check, on the
		// per-key-serialized ingest hot path; MarshalEntityState below re-validates the merged
		// candidate, so a poisoned statement kept from the stored value still fails the write
		// (classified via classifyStoredStateRMWError).
		existing, err := c.decodeStoredForWrite(ctx, entity.ID, current, read)
		if err != nil {
			return nil, err // non-retryable
		}
		// The indexing profile is create-time immutable (ADR-054). The arrival's declaration
		// would replace the stored one under the reserved source, where stampExplicitIndexingProfile
		// puts it, or sit beside it under the envelope's source, so it is dropped before the
		// replace WHEN the existing entity already carries one. An existing unprofiled entity
		// keeps the incoming declaration so reconcileIndexingProfile can apply it below.
		newer := entity.Triples
		profiled := hasIndexingProfileTriple(&existing)
		if profiled {
			newer = triplesWithoutPredicate(newer, vocabulary.EntityIndexingProfile)
		}
		// Each (predicate, source) set replaces the stored set of the same key whole; the
		// predicate's statements from other sources, and predicates the arrival does not carry
		// (e.g. lifecycle-managed ones, gh#177), stay. A re-arrival of the same set replaces it,
		// so a producer republishing an entity does not accumulate statements (gh#466).
		// ReplaceBySource returns a new slice whenever newer is non-empty, and existing's own
		// otherwise, so reconcileIndexingProfile's in-place filter below never writes into the
		// arrival's statements.
		replaced := graph.ReplaceBySource(existing.Triples, newer)
		stale = replaced.Stale
		// Every set is older: the arrival changes nothing on a profiled entity, so the write is
		// declined (design D23, "Every set is older"). A rewrite would bump the revision and
		// re-fire the ENTITY_STATES watchers for nothing. Each set is still counted below.
		if profiled && everySetStale(newer, stale) {
			return nil, natsclient.ErrKVSkipWrite
		}
		existing.Triples = replaced.Triples
		if len(stale) == 0 {
			existing.MessageType = entity.MessageType
			if entity.StorageRef != nil {
				existing.StorageRef = entity.StorageRef
			}
		}
		// ADR-054: when a producer merges into an existing unprofiled entity,
		// reconcile stamps the profile (kept from the incoming declaration, else floor). For an
		// already-profiled entity this is a no-op (keep-first preserves the
		// create-time value), so a re-arrival never re-profiles.
		c.reconcileIndexingProfile(&existing, at)
		existing.UpdatedAt = time.Now()
		data, err := graph.MarshalEntityState(&existing)
		if err != nil {
			return nil, c.classifyStoredStateRMWError(ctx, entity.ID, current, read, err)
		}
		bytesWritten = len(data)
		return data, nil
	})
	if err != nil {
		return revision, bytesWritten, err
	}
	// The write committed or was declined: the arrival is applied as far as it may be, and the
	// stream lane acknowledges it; each set left out is declared here.
	for _, set := range stale {
		if c.staleSets != nil {
			c.staleSets.Inc()
		}
		c.logger.Debug("statement set not applied: older than the stored set",
			slog.String("entity_id", entity.ID),
			slog.String("predicate", set.Predicate),
			slog.String("source", set.Source))
	}
	if revision == 0 {
		// The callback declined (UpdateWithRetryRead returns 0 only then) and nothing was
		// written. A skip clears no poison record and invalidates no cache entry: the next
		// valid read or commit clears a stale record (D3c).
		return 0, 0, nil
	}
	c.committed(ctx, entity.ID, revision)
	return revision, bytesWritten, nil
}

// everySetStale reports whether arrival carries at least one statement and each belongs to a
// set in stale, the sets graph.ReplaceBySource did not apply: then none of the arrival's sets
// applies.
func everySetStale(arrival []message.Triple, stale []graph.StaleSet) bool {
	if len(stale) == 0 {
		return false
	}
	notApplied := make(map[graph.StaleSet]struct{}, len(stale))
	for _, set := range stale {
		notApplied[set] = struct{}{}
	}
	for _, t := range arrival {
		if _, ok := notApplied[graph.StaleSet{Subject: t.Subject, Predicate: t.Predicate, Source: t.Source}]; !ok {
			return false
		}
	}
	return true
}

// decodeStoredForWrite is the write seam's read check (design D23) of current, a value stored
// under key and read at revision, which is nonzero: the trusted decode, then decodeStoredEntity's
// key check. Absence is revision 0, so an empty value here is unreadable, never a birth or an
// absent entity. A value it refuses is recorded in the poison inventory at revision and returned
// as its graph-state error, which the caller returns from its callback, so nothing is written.
// The full canonical rule is not run here: MarshalEntityState still gates every commit.
func (c *Component) decodeStoredForWrite(ctx context.Context, key string, current []byte, revision uint64) (graph.EntityState, error) {
	var stored graph.EntityState
	if err := graph.UnmarshalEntityStateTrusted(current, &stored); err != nil {
		return graph.EntityState{}, c.classifyStoredStateRMWError(ctx, key, current, revision, err)
	}
	if err := checkStoredEntityKey(key, stored.ID); err != nil {
		var contractErr *graph.StateContractError
		if errors.As(err, &contractErr) {
			c.inventoryEntityPoison(ctx, contractErr, revision)
		}
		return graph.EntityState{}, err
	}
	return stored, nil
}

// reconcileCandidate is the conditional replace's rule, applied to the state the caller read at
// its expected revision: desired (with repeats counted once) replaces source's stored statements
// of predicates whole, an empty desired set clears them, and the statements of predicates from
// every other source stay (design D15). unchanged reports that source's stored statements of
// predicates already equal desired in every field, in which case nothing is to be written and
// candidate is nil.
func reconcileCandidate(current *graph.EntityState, source string, desired []message.Triple, predicates map[string]struct{}) (candidate *graph.EntityState, unchanged bool) {
	desired = dedupeReconcileTriples(desired)
	if selectedPredicatesEqual(current.Triples, source, desired, predicates) {
		return nil, true
	}
	candidate = current.Clone()
	candidate.Triples = reconcileSelectedPredicates(current.Triples, source, desired, predicates)
	candidate.UpdatedAt = time.Now()
	return candidate, false
}

// replaceEntityAtRevision is the conditional replace's write: encoded (a reconcileCandidate
// result) is stored only if the entity is still at expectedRevision, the revision the caller
// read. natsclient.ErrKVRevisionMismatch and a not-found error are returned as is.
func (c *Component) replaceEntityAtRevision(ctx context.Context, entityID string, encoded []byte, expectedRevision uint64) (uint64, error) {
	revision, err := c.entityBucket.Update(ctx, entityID, encoded, expectedRevision)
	if err != nil {
		return revision, err
	}
	c.committed(ctx, entityID, revision)
	return revision, nil
}

// appendResult is what one append did. outcome is graph.MutationApplied when the append
// committed revision, or graph.MutationUnchanged when every triple was already stored: nothing
// was written, and revision is the one its compare-and-set read. suppressed is the number of
// triples not added, as counted against the state the last attempt read.
type appendResult struct {
	revision   uint64
	outcome    graph.MutationOutcome
	suppressed int
}

// appendEntityTriples is the append mode: a read-modify-write under the KV revision that adds
// to subject's stored entity each of triples whose message.AppendIdentityKey is not stored yet.
// The entity must exist: an absent subject fails with natsclient.ErrKVKeyNotFound. A stored
// value is refused as replaceEntity refuses one, before any triple is compared with it. When
// every triple is already stored the result is unchanged at the revision read (design D23). On
// an error the result is the zero value.
func (c *Component) appendEntityTriples(ctx context.Context, subject string, triples []message.Triple) (appendResult, error) {
	// read and suppressed are assigned on each run of the callback, never accumulated, so they
	// describe the last run: the one that committed or declined.
	var (
		read       uint64
		suppressed int
	)
	revision, err := c.entityBucket.UpdateWithRetryRead(ctx, subject, func(current []byte, currentRevision uint64) ([]byte, error) {
		read = currentRevision
		if currentRevision == 0 {
			// Must-exist (ADR-055): a triple targeting an absent entity is
			// rejected, not silently auto-vivified. Absence is revision 0, never an
			// empty value (design D23). NonRetryable stops the CAS loop and surfaces
			// the sentinel for the handler to map.
			return nil, retry.NonRetryable(natsclient.ErrKVKeyNotFound)
		}
		// gh#562: the read check is the trusted decode plus the key check;
		// MarshalEntityState below re-validates the final candidate.
		entity, err := c.decodeStoredForWrite(ctx, subject, current, currentRevision)
		if err != nil {
			return nil, err // non-retryable
		}

		// Append deduplication, INSIDE the CAS closure and BEFORE the
		// revision-checked write. `entity` was decoded from the bytes read at
		// the revision this iteration will CAS against, so a request that loses
		// the CAS re-runs here against the winner's committed state and
		// suppresses on the retry. A pre-read outside UpdateWithRetry would
		// reintroduce exactly the time-of-check-to-time-of-use window in which
		// two concurrent identical appends both observe the tuple absent. This
		// also collapses repeats WITHIN triples, so one request commits at most
		// one copy.
		//
		// suppressed is ASSIGNED (never accumulated), so a CAS retry replaces
		// the losing attempt's count with the re-evaluation against the
		// winner's committed state rather than double-counting.
		// UpdateWithRetryRead invokes the closure synchronously on this
		// goroutine, so the capture needs no synchronization.
		var survivors []message.Triple
		survivors, suppressed = message.DedupeAppendTriples(entity.Triples, triples)
		if len(survivors) == 0 {
			// TRUE no-op: decline the write. Returning `current` instead would be
			// an identity rewrite (revision bump + ENTITY_STATES watcher re-fire),
			// and a restart replaying its derived triples must be invisible
			// downstream (gh#713).
			return nil, natsclient.ErrKVSkipWrite
		}

		entity.Triples = append(entity.Triples, survivors...)
		entity.UpdatedAt = time.Now()

		data, err := graph.MarshalEntityState(&entity)
		if err != nil {
			return nil, c.classifyStoredStateRMWError(ctx, subject, current, currentRevision, err)
		}
		return data, nil
	})
	if err != nil {
		return appendResult{}, err
	}
	if revision == 0 {
		// The callback declined (UpdateWithRetryRead returns 0 only then) and nothing was
		// written. A skip clears no poison record and invalidates no cache entry: the next
		// valid read or commit clears a stale record (D3c).
		return appendResult{revision: read, outcome: graph.MutationUnchanged, suppressed: suppressed}, nil
	}
	c.committed(ctx, subject, revision)
	return appendResult{revision: revision, outcome: graph.MutationApplied, suppressed: suppressed}, nil
}

// deleteEntity is the delete mode: the entity is removed only if it is still at revision, the
// caller's expected revision. The bucket's error is returned as is.
func (c *Component) deleteEntity(ctx context.Context, entityID string, revision uint64) error {
	if err := c.entityBucket.DeleteAtRevision(ctx, entityID, revision); err != nil {
		return err
	}
	// The poisoned bytes, if any, are gone with the key.
	c.clearEntityPoisonOnDelete(entityID)
	c.invalidateEntityCacheEntry(entityID)
	return nil
}

// committed is the bookkeeping after a write that committed revision under entityID: the
// write passed the MarshalEntityState gate, so a poison record at or below revision is stale
// (clear path (b), D3b), and the next query must not be served the entry cached before it.
func (c *Component) committed(ctx context.Context, entityID string, revision uint64) {
	c.clearEntityPoisonOnCommit(ctx, entityID, revision)
	c.invalidateEntityCacheEntry(entityID)
}
