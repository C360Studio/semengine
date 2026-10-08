package graphingest

import (
	"context"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/retry"
	"github.com/c360studio/semengine/vocabulary"
)

// This file is graph-ingest's one write seam (design D15, #100): every write to the entity
// bucket (ENTITY_STATES) is made here, one method per write mode, so the rule each mode applies
// to a stored entity is read in one place. TestEntityWritesHaveOneSeam fails when the bucket is
// written from any other file.
//
// Callers keep their own validation, authority checks, metrics, cache and poison bookkeeping,
// and replies; a method here takes what its mode needs and returns the revision it committed
// (where the caller uses it) and the bucket's error unwrapped, so callers classify it as before.
//
// The modes, and the lanes on which each is used:
//
//   - create (createEntity): mutation create; in-process create (hierarchy containers).
//   - replace (replaceEntity): the stream lane. A birth when the key is absent, else a replace by
//     predicate (replaceByPredicate), under the KV revision.
//   - conditional replace (reconcileCandidate, then replaceEntityAtRevision): mutation reconcile,
//     at the caller's expected revision.
//   - append (appendEntityTriples): mutation append; in-process append (hierarchy's inverse
//     edges). Identity is message.AppendIdentityKey, under the KV revision.
//   - delete (deleteEntity): mutation delete, at the caller's expected revision.

// createEntity is the create mode: the bucket's atomic create-or-fail, the only admitted birth
// primitive. An existing key is refused with natsclient.ErrKVKeyExists, which is returned as is
// so each lane makes its own decision (entity_already_exists on the mutation lane).
func (c *Component) createEntity(ctx context.Context, entityID string, encoded []byte) (uint64, error) {
	return c.entityBucket.Create(ctx, entityID, encoded)
}

// replaceEntity is the stream lane's write: a read-modify-write under the KV revision
// (compare-and-set), so concurrent arrivals on the same subject converge without racing.
// When the key is absent the arrival is a birth: hierarchyTriples are appended to entity and
// its profile is stamped, and entity is changed in place. Otherwise the stored entity's
// statements are replaced by predicate with the arrival's. It returns the size of the value
// written.
func (c *Component) replaceEntity(ctx context.Context, entity *graph.EntityState, hierarchyTriples []message.Triple) (int, error) {
	var bytesWritten int
	// casAttempt counts CAS-callback invocations; each re-run (attempt > 1) means
	// the prior revision-checked Put lost the CAS and retried (ADR-072
	// cas_retries — cross-entity contention observability, not a keying proof).
	casAttempt := 0
	err := c.entityBucket.UpdateWithRetry(ctx, entity.ID, func(current []byte) ([]byte, error) {
		casAttempt++
		if casAttempt > 1 && c.casRetries != nil {
			c.casRetries.Inc()
		}
		// First write: entity didn't exist. Apply hierarchy triples
		// (deterministic-per-ID so safe to apply once on create),
		// then store verbatim.
		if len(current) == 0 {
			if len(hierarchyTriples) > 0 {
				entity.Triples = append(entity.Triples, hierarchyTriples...)
			}
			// ADR-054: first write is entity birth — stamp the profile
			// (explicit-if-declared via IndexingProfiler, else floor).
			c.reconcileIndexingProfile(entity)
			data, err := graph.MarshalEntityState(entity)
			if err == nil {
				bytesWritten = len(data)
			}
			return data, err
		}
		// Existing entity: merge triples + refresh latest-wins metadata.
		// Hierarchy triples are NOT re-applied — they landed on the
		// original create and would only produce duplicates here.
		//
		// gh#562: trusted decode — this is the owner's own RMW read on the
		// per-key-serialized ingest hot path; MarshalEntityState below
		// re-validates the merged candidate, so resident poison still fails
		// the write (classified via classifyStoredStateRMWError).
		var existing graph.EntityState
		if err := graph.UnmarshalEntityStateTrusted(current, &existing); err != nil {
			return nil, c.classifyStoredStateRMWError(ctx, entity.ID, current, err) // non-retryable
		}
		// gh#466: predicate-level merge (replace per (subject,predicate)), NOT raw
		// append — otherwise a producer republishing the same entity accumulates
		// duplicate triples forever. replaceByPredicate lets the incoming arrival win on
		// a (subject,predicate) conflict while preserving non-conflicting existing
		// triples (e.g. lifecycle-managed predicates the arrival doesn't carry —
		// gh#177).
		//
		// The indexing profile is the exception: it is create-time-immutable
		// (ADR-054), but replaceByPredicate is newer-wins, so a re-arrival declaring a
		// different profile would override the create-time one. Drop the incoming
		// profile before merging WHEN the existing entity already carries one. An
		// existing unprofiled entity keeps the incoming declaration so
		// reconcileIndexingProfile can apply it below.
		newer := entity.Triples
		if hasIndexingProfileTriple(&existing) {
			newer = triplesWithoutPredicate(newer, vocabulary.EntityIndexingProfile)
		}
		existing.Triples = replaceByPredicate(existing.Triples, newer)
		existing.MessageType = entity.MessageType
		if entity.StorageRef != nil {
			existing.StorageRef = entity.StorageRef
		}
		// ADR-054: when a producer merges into an existing unprofiled entity,
		// reconcile stamps the profile (kept from the incoming declaration, else floor). For an
		// already-profiled entity this is a no-op (keep-first preserves the
		// create-time value), so a re-arrival never re-profiles.
		c.reconcileIndexingProfile(&existing)
		existing.UpdatedAt = time.Now()
		data, err := graph.MarshalEntityState(&existing)
		if err != nil {
			return nil, c.classifyStoredStateRMWError(ctx, entity.ID, current, err)
		}
		bytesWritten = len(data)
		return data, nil
	})
	return bytesWritten, err
}

// reconcileCandidate is the conditional replace's rule, applied to the state the caller read at
// its expected revision: desired (with repeats counted once) replaces the stored statements of
// predicates whole, and an empty desired set clears them. unchanged reports that the stored
// statements of predicates already equal desired, in which case nothing is to be written and
// candidate is nil.
func reconcileCandidate(current *graph.EntityState, desired []message.Triple, predicates map[string]struct{}) (candidate *graph.EntityState, unchanged bool) {
	desired = dedupeReconcileTriples(desired)
	if selectedPredicatesEqual(current.Triples, desired, predicates) {
		return nil, true
	}
	candidate = current.Clone()
	candidate.Triples = reconcileSelectedPredicates(current.Triples, desired, predicates)
	candidate.UpdatedAt = time.Now()
	return candidate, false
}

// replaceEntityAtRevision is the conditional replace's write: encoded (a reconcileCandidate
// result) is stored only if the entity is still at expectedRevision, the revision the caller
// read. natsclient.ErrKVRevisionMismatch and a not-found error are returned as is.
func (c *Component) replaceEntityAtRevision(ctx context.Context, entityID string, encoded []byte, expectedRevision uint64) (uint64, error) {
	return c.entityBucket.Update(ctx, entityID, encoded, expectedRevision)
}

// appendEntityTriples is the append mode: a read-modify-write under the KV revision that adds
// to subject's stored entity each of triples whose message.AppendIdentityKey is not stored yet.
// The entity must exist: an absent subject fails with natsclient.ErrKVKeyNotFound. When every
// triple is already stored nothing is written and the error is errNoOpAddDuplicate. revision is
// the exact revision this write committed (0 when nothing committed), and suppressed is the
// number of triples not added, as counted against the state the last attempt read.
func (c *Component) appendEntityTriples(ctx context.Context, subject string, triples []message.Triple) (revision uint64, suppressed int, err error) {
	revision, err = c.entityBucket.UpdateWithRetryRev(ctx, subject, func(current []byte) ([]byte, error) {
		var entity graph.EntityState

		if len(current) > 0 {
			// gh#562: trusted decode on the owner's own RMW read;
			// MarshalEntityState below re-validates the final candidate.
			if err := graph.UnmarshalEntityStateTrusted(current, &entity); err != nil {
				return nil, c.classifyStoredStateRMWError(ctx, subject, current, err) // Non-retryable
			}
		} else {
			// Must-exist (ADR-055): a triple targeting an absent entity is
			// rejected, not silently auto-vivified. NonRetryable stops the CAS
			// loop and surfaces the sentinel for the handler to map.
			return nil, retry.NonRetryable(natsclient.ErrKVKeyNotFound)
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
		// UpdateWithRetryRev invokes the closure synchronously on this
		// goroutine, so the capture needs no synchronization.
		var survivors []message.Triple
		survivors, suppressed = message.DedupeAppendTriples(entity.Triples, triples)
		if len(survivors) == 0 {
			// TRUE no-op — exit via the sentinel, not by returning `current`.
			return nil, errNoOpAddDuplicate
		}

		entity.Triples = append(entity.Triples, survivors...)
		entity.UpdatedAt = time.Now()

		data, err := graph.MarshalEntityState(&entity)
		if err != nil {
			return nil, c.classifyStoredStateRMWError(ctx, subject, current, err)
		}
		return data, nil
	})
	return revision, suppressed, err
}

// deleteEntity is the delete mode: the entity is removed only if it is still at revision, the
// caller's expected revision. The bucket's error is returned as is.
func (c *Component) deleteEntity(ctx context.Context, entityID string, revision uint64) error {
	return c.entityBucket.DeleteAtRevision(ctx, entityID, revision)
}
