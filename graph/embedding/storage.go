package embedding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/internal/kvwatch"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/types"
)

// ErrRecordGone reports that the EMBEDDING_INDEX record a save was meant to
// UPDATE no longer exists, so the save was dropped without writing.
//
// It is a normal outcome, not a fault: the hop-1 entity tombstone deletes an
// entity's index key while a hop-2 worker may still be inside an embedder round
// trip for that same entity. Callers should treat it as "this entity is no longer
// supposed to have an embedding" and stop — in particular they must not report it
// as a generation failure or fire the generated callback, which would push a
// vector for a dead entity into caches.
var ErrRecordGone = errors.New("embedding index record no longer exists")

// ErrSupersededRevision reports that the save was dropped because a newer source
// revision's outcome already landed for this entity (the ordering guard). Like
// ErrRecordGone it is a normal, non-failure outcome: the caller must NOT fire the
// generated callback, because the vector it holds is the OLDER revision's and
// firing would push a stale vector into any WithOnGenerated consumer's cache.
// Callers must not report it as a failure either; the newer outcome is
// authoritative.
var ErrSupersededRevision = errors.New("embedding index write superseded by a newer source revision")

// Status represents the processing status of an embedding
type Status string

const (
	// StatusPending awaits generation
	StatusPending Status = "pending"
	// StatusGenerated is successfully generated
	StatusGenerated Status = "generated"
	// StatusFailed indicates generation failed
	StatusFailed Status = "failed"
)

// Record represents a stored embedding with metadata
type Record struct {
	EntityID    string    `json:"entity_id"`
	Vector      []float32 `json:"vector,omitempty"`
	ContentHash string    `json:"content_hash"`
	// SourceText is the INLINE lane's WHOLE embedding text (the identity triples
	// extracted at hop 1). It is meaningful ONLY on the inline lane (StorageRef == nil);
	// on the offloaded lane it is left EMPTY and the identity prefix travels in
	// IdentityText instead.
	SourceText string `json:"source_text,omitempty"`
	// IdentityText is the inline identity PREFIX (title/.signature/.comment, per the
	// configured text suffixes) for the OFFLOADED (StorageRef) lane ONLY: hop 2 embeds it
	// AHEAD of the fetched body, identity-first, in one vector so the text suffixes take
	// effect on offloaded entities too. Empty on the inline lane, and empty on an
	// offloaded record with no inline identity text (body-only).
	IdentityText string    `json:"identity_text,omitempty"`
	Model        string    `json:"model,omitempty"`
	Dimensions   int       `json:"dimensions,omitempty"`
	GeneratedAt  time.Time `json:"generated_at,omitempty"`
	Status       Status    `json:"status"`
	ErrorMsg     string    `json:"error_msg,omitempty"` // If status=failed
	// Reason is a BOUNDED classification of a failure (status=failed), stored next to
	// the raw ErrorMsg. It is the value the failures metric is labelled by — the raw
	// ErrorMsg is unbounded and must NEVER be a metric label (cardinality blowup). It is
	// also what the current-failed bootstrap scan reads to seed the reason breakdown
	// after a restart.
	Reason string `json:"reason,omitempty"` // Bounded reason enum when status=failed

	// SourceRevision is the ENTITY_STATES stream revision that produced this record.
	// It is threaded from the hop-1 watcher so hop 2 can complete the embedding
	// readiness watermark at the terminal transition (ADR-066 §3), and it is
	// persisted onto generated and failed records so SaveGenerated and SaveFailed can
	// order writes by it: a record already carrying a higher SourceRevision has a
	// newer outcome, so a late older-revision write is dropped. 0 means unknown and
	// is treated as oldest.
	SourceRevision uint64 `json:"source_revision,omitempty"`

	// StorageRef, when set, points at the entity's body in a content store; the
	// Worker fetches the text from there.
	StorageRef *StorageRef `json:"storage_ref,omitempty"`
}

// StorageRef is a simplified reference for embedding storage.
// Mirrors message.StorageReference structure.
type StorageRef struct {
	StorageInstance string `json:"storage_instance"`
	Key             string `json:"key"`
}

// DedupRecord stores content-addressed embeddings for deduplication.
//
// Model and Dimensions record WHICH vector space the stored vector belongs to.
// The dedup KEY already folds in embedder identity (see DedupKey), so a mismatch
// here should be unreachable; carrying the fields anyway makes a stale record
// detectable after the fact instead of silently servable. FirstGenerated has no
// reader in code; it is what an operator sees when listing the bucket.
type DedupRecord struct {
	Vector         []float32 `json:"vector"`
	FirstGenerated time.Time `json:"first_generated"`
	Model          string    `json:"model,omitempty"`
	Dimensions     int       `json:"dimensions,omitempty"`
}

// ScoredEntity pairs an entity ID with its cosine similarity score.
type ScoredEntity struct {
	EntityID   string
	Similarity float64
}

// ScoredEntityLess is the CANONICAL ordering for similarity results: by
// similarity descending, then entity ID ascending as a total-order tie-break.
// Every similarity path sorts through this ONE comparator so that an
// equal-similarity group straddling the top-k boundary resolves to the SAME subset
// regardless of map- or KV-enumeration order. Sorting by similarity alone leaves
// the k-boundary tie to Go's randomized map iteration or the KV's key-delivery
// order, which yields nondeterministic neighbor sets downstream. Entity IDs are
// unique, so this is a strict total order — deterministic even under the
// non-stable sort.Slice.
func ScoredEntityLess(a, b ScoredEntity) bool {
	if a.Similarity != b.Similarity {
		return a.Similarity > b.Similarity
	}
	return a.EntityID < b.EntityID
}

// Storage handles persistence of embeddings to two NATS KV buckets, EMBEDDING_INDEX
// and EMBEDDING_DEDUP.
//
// Every read-modify-write goes through natsclient.KVStore.UpdateWithRetryRead, which
// decides from the bytes and revision it read and retries a revision conflict
// (ADR-110). The retry budget and the per-call timeout are the KVStore's, set by
// whoever builds it; when the retries run out a save returns an error matching
// natsclient.ErrKVMaxRetriesExceeded and has written nothing.
//
// Every method that takes an entity ID refuses one that is not a literal six-part ID
// (types.ValidateEntityID), and every method that takes a dedup key refuses one that is
// not a literal KV key (natsclient.ValidateKVLiteralKey), with an invalid-input error
// before any bucket call.
type Storage struct {
	index *natsclient.KVStore // EMBEDDING_INDEX
	dedup *natsclient.KVStore // EMBEDDING_DEDUP
}

// NewStorage creates a new embedding storage instance over the EMBEDDING_INDEX and
// EMBEDDING_DEDUP buckets.
func NewStorage(index, dedup *natsclient.KVStore) *Storage {
	return &Storage{index: index, dedup: dedup}
}

// validateEntityID refuses an entity ID that is not one literal six-part ID
// (types.ValidateEntityID) before any bucket call. That grammar (at most 256 bytes, six
// non-empty tokens of letters, digits, '_' and '-') lies inside natsclient's literal KV
// key grammar, so a valid entity ID is a valid key and needs no second check.
func validateEntityID(entityID, method string) error {
	if err := types.ValidateEntityID(entityID); err != nil {
		return errs.WrapInvalid(err, "Storage", method, "validate entity id")
	}
	return nil
}

// validateDedupKey refuses a dedup key that is not a literal NATS KV key before any
// bucket call.
func validateDedupKey(key, method string) error {
	if err := natsclient.ValidateKVLiteralKey(key); err != nil {
		return errs.WrapInvalid(err, "Storage", method, "validate dedup key")
	}
	return nil
}

// decodeRecord decodes a stored index record, or returns nil for an absent key.
func decodeRecord(current []byte, method string) (*Record, error) {
	if current == nil {
		return nil, nil
	}
	var record Record
	if err := json.Unmarshal(current, &record); err != nil {
		return nil, errs.WrapInvalid(err, "Storage", method, "unmarshal embedding record")
	}
	return &record, nil
}

// saveError returns a save's two drop sentinels bare, an invalid record as it is,
// and classifies anything else, retry exhaustion included, as transient.
func saveError(err error, method, action string) error {
	switch {
	case errors.Is(err, ErrRecordGone):
		return ErrRecordGone
	case errors.Is(err, ErrSupersededRevision):
		return ErrSupersededRevision
	case errs.IsInvalid(err):
		return err
	default:
		return errs.WrapTransient(err, "Storage", method, action)
	}
}

// SavePendingGuarded persists a hop-1 pending record with a generated-record guard.
// It is the write behind graph-embedding's sole hop-1 record writer, for both
// pending lanes (inline and storage-ref — the caller shapes the record; Status is
// forced to StatusPending here).
//
// The guard: when the stored record is a GENERATED vector at a same-or-newer
// source revision, nothing is written and saved is false. An unconditional write
// here would let a stale repair re-drive downgrade a fresh generated record to
// pending, and the guard makes a restart's re-delivery of an already-generated
// revision a cheap skip. Any other stored record, pending or failed at any
// revision, or generated at an OLDER revision, is overwritten, so an older pending
// record replaces a newer pending one. An absent key, a delete or purge marker
// included, is created.
//
// The decision and the write are one compare-and-set (UpdateWithRetryRead), so a
// concurrent hop-2 commit cannot bypass the guard.
func (s *Storage) SavePendingGuarded(ctx context.Context, record *Record) (saved bool, err error) {
	if record == nil {
		return false, errs.WrapInvalid(errs.ErrMissingConfig, "Storage", "SavePendingGuarded", "record is nil")
	}
	if err := validateEntityID(record.EntityID, "SavePendingGuarded"); err != nil {
		return false, err
	}
	record.Status = StatusPending
	data, err := json.Marshal(record)
	if err != nil {
		return false, errs.WrapInvalid(err, "Storage", "SavePendingGuarded", "marshal embedding record")
	}

	_, err = s.index.UpdateWithRetryRead(ctx, record.EntityID, func(current []byte, _ uint64) ([]byte, error) {
		existing, err := decodeRecord(current, "SavePendingGuarded")
		if err != nil {
			return nil, err
		}
		if existing != nil && existing.Status == StatusGenerated && existing.SourceRevision >= record.SourceRevision {
			saved = false // a same-or-newer generated vector stands; do not downgrade
			return nil, natsclient.ErrKVSkipWrite
		}
		saved = true
		return data, nil
	})
	if err != nil {
		return false, saveError(err, "SavePendingGuarded", "write pending embedding")
	}
	return saved, nil
}

// SaveGenerated persists a generated embedding, ordered by source revision.
//
// contentHash is the hop-2 dedup key of the exact bytes embedded; it is stored as
// the record's content hash rather than copied from the pending record.
// sourceRevision is the ENTITY_STATES revision this generation is completing.
// ContentHash and Vector are both taken from the arguments, never from the stored
// record, so they cannot desync across revisions.
//
// It returns ErrRecordGone, writing nothing, when the key is absent: the entity was
// tombstoned or its pending record removed, and writing would re-create a vector
// for an entity that must not have one. It returns ErrSupersededRevision, writing
// nothing, when the stored record carries a newer source revision.
func (s *Storage) SaveGenerated(
	ctx context.Context,
	entityID string,
	vector []float32,
	model string,
	dimensions int,
	contentHash string,
	sourceRevision uint64,
) error {
	if err := validateEntityID(entityID, "SaveGenerated"); err != nil {
		return err
	}

	_, err := s.index.UpdateWithRetryRead(ctx, entityID, func(current []byte, _ uint64) ([]byte, error) {
		existing, err := decodeRecord(current, "SaveGenerated")
		if err != nil {
			return nil, err
		}
		if existing == nil {
			return nil, ErrRecordGone
		}
		// Semantic ordering, not lost update: the compare-and-set alone would let a
		// stale write win if it merely read first, so the guard is on SourceRevision.
		if existing.SourceRevision > sourceRevision {
			return nil, ErrSupersededRevision
		}
		data, err := json.Marshal(&Record{
			EntityID:       entityID,
			Vector:         vector,
			ContentHash:    contentHash,
			Model:          model,
			Dimensions:     dimensions,
			GeneratedAt:    time.Now(),
			Status:         StatusGenerated,
			SourceRevision: sourceRevision,
		})
		if err != nil {
			return nil, errs.WrapInvalid(err, "Storage", "SaveGenerated", "marshal embedding record")
		}
		return data, nil
	})
	return saveError(err, "SaveGenerated", "write generated embedding")
}

// SaveFailed marks an embedding as failed under the same guard as SaveGenerated, so
// a stale failure cannot clobber a newer success, and the failed record keeps its
// own source revision for later ordering.
//
// reason is the BOUNDED classification stored alongside the raw errorMsg; it is
// what the failures metric is labelled by and what the current-failed bootstrap
// scan reads. Pass "" only for a failure that has no classified reason.
//
// It returns ErrRecordGone when the key is absent and ErrSupersededRevision when the
// stored record carries a newer source revision, or is generated at the same one: a
// generated outcome wins over a failure at an equal revision. Neither writes.
func (s *Storage) SaveFailed(ctx context.Context, entityID, errorMsg, reason string, sourceRevision uint64) error {
	if err := validateEntityID(entityID, "SaveFailed"); err != nil {
		return err
	}

	_, err := s.index.UpdateWithRetryRead(ctx, entityID, func(current []byte, _ uint64) ([]byte, error) {
		existing, err := decodeRecord(current, "SaveFailed")
		if err != nil {
			return nil, err
		}
		if existing == nil {
			return nil, ErrRecordGone
		}
		if existing.SourceRevision > sourceRevision {
			return nil, ErrSupersededRevision
		}
		if existing.Status == StatusGenerated && existing.SourceRevision == sourceRevision {
			return nil, ErrSupersededRevision
		}
		existing.Status = StatusFailed
		existing.ErrorMsg = errorMsg
		existing.Reason = reason
		existing.SourceRevision = sourceRevision
		data, err := json.Marshal(existing)
		if err != nil {
			return nil, errs.WrapInvalid(err, "Storage", "SaveFailed", "marshal embedding record")
		}
		return data, nil
	})
	return saveError(err, "SaveFailed", "write failed embedding")
}

// deleteAtRevision removes an entity's index record only if its key is still at
// kvRevision, the KV revision the caller read it at: the Worker's removal of a
// record whose text resolved empty. It makes one fenced attempt and never retries
// (natsclient.KVStore.DeleteAtRevision). removed is true when the record was
// removed; false with a nil error when the key moved or is gone, so newer work owns
// the entity and nothing was removed; false with the error otherwise.
func (s *Storage) deleteAtRevision(ctx context.Context, entityID string, kvRevision uint64) (removed bool, err error) {
	if err := validateEntityID(entityID, "deleteAtRevision"); err != nil {
		return false, err
	}
	err = s.index.DeleteAtRevision(ctx, entityID, kvRevision)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, natsclient.ErrKVRevisionMismatch), errors.Is(err, natsclient.ErrKVKeyNotFound):
		return false, nil
	default:
		return false, errs.WrapTransient(err, "Storage", "deleteAtRevision", "delete embedding at revision")
	}
}

// GetEmbedding retrieves an embedding by entity ID, or nil when there is none.
func (s *Storage) GetEmbedding(ctx context.Context, entityID string) (*Record, error) {
	if err := validateEntityID(entityID, "GetEmbedding"); err != nil {
		return nil, err
	}
	entry, err := s.index.Get(ctx, entityID)
	if err != nil {
		if errors.Is(err, natsclient.ErrKVKeyNotFound) {
			return nil, nil
		}
		return nil, errs.WrapTransient(err, "Storage", "GetEmbedding", "get embedding")
	}
	return decodeRecord(entry.Value, "GetEmbedding")
}

// GetByContentHash retrieves a dedup record by its key, or nil when there is none.
func (s *Storage) GetByContentHash(ctx context.Context, contentHash string) (*DedupRecord, error) {
	if err := validateDedupKey(contentHash, "GetByContentHash"); err != nil {
		return nil, err
	}
	entry, err := s.dedup.Get(ctx, contentHash)
	if err != nil {
		if errors.Is(err, natsclient.ErrKVKeyNotFound) {
			return nil, nil
		}
		return nil, errs.WrapTransient(err, "Storage", "GetByContentHash", "get dedup record")
	}
	var record DedupRecord
	if err := json.Unmarshal(entry.Value, &record); err != nil {
		return nil, errs.WrapInvalid(err, "Storage", "GetByContentHash", "unmarshal dedup record")
	}
	return &record, nil
}

// SaveDedup saves a content-addressed embedding for deduplication.
//
// model and dimensions identify the vector space the vector belongs to; callers
// pass the generating embedder's own values so a stale record is auditable.
// contentHash MUST come from DedupKey: the durable dedup bucket outlives any one
// embedder configuration.
//
// A stored record from the same vector space (same model and dimensions) stands and
// nothing is written. Any other record, or none, is replaced with this vector: it
// was generated by the current embedder and is authoritative for the key, so a
// record from another or unknown space must not merely be stamped with the current
// identity, which would serve its old vector under the new model's name.
func (s *Storage) SaveDedup(ctx context.Context, contentHash string, vector []float32, model string, dimensions int) error {
	if err := validateDedupKey(contentHash, "SaveDedup"); err != nil {
		return err
	}
	data, err := json.Marshal(&DedupRecord{
		Vector:         vector,
		FirstGenerated: time.Now(),
		Model:          model,
		Dimensions:     dimensions,
	})
	if err != nil {
		return errs.WrapInvalid(err, "Storage", "SaveDedup", "marshal dedup record")
	}

	_, err = s.dedup.UpdateWithRetryRead(ctx, contentHash, func(current []byte, _ uint64) ([]byte, error) {
		if current != nil {
			var existing DedupRecord
			if err := json.Unmarshal(current, &existing); err != nil {
				return nil, errs.WrapInvalid(err, "Storage", "SaveDedup", "unmarshal dedup record")
			}
			if existing.Model == model && existing.Dimensions == dimensions {
				return nil, natsclient.ErrKVSkipWrite
			}
		}
		return data, nil
	})
	return saveError(err, "SaveDedup", "write dedup record")
}

// DeleteEmbedding removes an embedding record. Removing an absent record is not an
// error.
func (s *Storage) DeleteEmbedding(ctx context.Context, entityID string) error {
	if err := validateEntityID(entityID, "DeleteEmbedding"); err != nil {
		return err
	}
	if err := s.index.Delete(ctx, entityID); err != nil && !errors.Is(err, natsclient.ErrKVKeyNotFound) {
		return errs.WrapTransient(err, "Storage", "DeleteEmbedding", "delete embedding")
	}
	return nil
}

// ListGeneratedEntityIDs returns every key in EMBEDDING_INDEX: pending and failed
// records are listed too, so the caller filters by status. The listing is bounded by
// the index KVStore's Timeout option, not by ctx alone.
func (s *Storage) ListGeneratedEntityIDs(ctx context.Context) ([]string, error) {
	keys, err := s.index.Keys(ctx)
	if err != nil {
		return nil, errs.WrapTransient(err, "Storage", "ListGeneratedEntityIDs", "list keys")
	}
	return keys, nil
}

// FailedEntry is one entity currently in a failed embedding terminal state, as read
// by the current-failed bootstrap scan: the entity ID, its bounded reason, and the
// source revision the failure was recorded at.
//
// Reason is ALWAYS a member of the bounded failure enum (or reasonUnknown) —
// ScanFailed normalizes it, so a stored record can never inject an arbitrary string
// into the readiness envelope's reason histogram.
//
// SourceRevision is the durable record's revision, seeded into the component's
// in-memory failed map so its revision check (a superseded older completion must
// NOT clear a newer failure) starts from the correct baseline after a restart.
type FailedEntry struct {
	EntityID       string
	Reason         string
	SourceRevision uint64
}

// Bounded failure-reason enum. These are the ONLY values that ever reach the
// failures_total{reason} metric label — the raw ErrorMsg is unbounded and must never
// be a label. Keep the set small and closed; classifyEmbedErr maps embedder error
// shapes into the network/timeout/dimension buckets, defaulting to embedder_error.
const (
	failReasonContentError      = "content_error"      // source-text extraction failed
	failReasonInternal          = "internal"           // dedup-check / save write fault
	failReasonConnectionRefused = "connection_refused" // embedder endpoint unreachable
	failReasonTimeout           = "timeout"            // embedder call timed out
	failReasonDimensionMismatch = "dimension_mismatch" // embedder returned an off-width vector
	failReasonEmbedderError     = "embedder_error"     // any other embedder-side failure
)

// reasonUnknown is the fallback for a stored failure reason that is empty or not a
// member of the bounded failure enum. It keeps the readiness envelope's reason
// histogram (and any label derived from it) bounded whatever a stored record holds.
const reasonUnknown = "unknown"

// normalizeFailureReason collapses any stored reason to the closed failure enum,
// mapping an empty or unrecognized value to reasonUnknown. Called at the scan
// boundary so an arbitrary Record.Reason never reaches a published label.
func normalizeFailureReason(reason string) string {
	switch reason {
	case failReasonContentError, failReasonInternal, failReasonConnectionRefused,
		failReasonTimeout, failReasonDimensionMismatch, failReasonEmbedderError:
		return reason
	default:
		return reasonUnknown
	}
}

// ScanFailed reads EMBEDDING_INDEX through a watch's initial snapshot and returns
// every record currently in the StatusFailed terminal state, with its bounded
// reason. It seeds the component's current-failed map at start so its failed count
// is accurate immediately after bootstrap, independent of re-delivery timing. It
// returns at the snapshot's end. A record that will not decode is skipped
// (best-effort seed); aborting would leave the failed count at 0, which is worse
// than a partial seed corrected by re-delivery. When ctx ends during the scan, the
// entries read so far are returned with a nil error, for the same reason; when it
// ends before the watch opens, none are. The watch is stopped through kvwatch.Stop;
// if its delivery does not close, ScanFailed returns that error and no entries.
func (s *Storage) ScanFailed(ctx context.Context) ([]FailedEntry, error) {
	watcher, err := s.index.Watch(ctx, jetstream.AllKeys)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, nil
		}
		return nil, errs.WrapTransient(err, "Storage", "ScanFailed", "watch index bucket")
	}
	failed := scanFailed(ctx, watcher)
	if err := kvwatch.Stop(ctx, watcher); err != nil {
		return nil, fmt.Errorf("embedding: stop the EMBEDDING_INDEX watch: %w", err)
	}
	return failed, nil
}

// scanFailed collects the failed records from watcher's snapshot until its end
// sentinel, its channel's close, or ctx's end.
func scanFailed(ctx context.Context, watcher jetstream.KeyWatcher) []FailedEntry {
	var failed []FailedEntry
	for {
		select {
		case <-ctx.Done():
			return failed
		case entry, ok := <-watcher.Updates():
			// nil is the end of the snapshot, which is all the seed needs.
			if !ok || entry == nil {
				return failed
			}
			if entry.Operation() != jetstream.KeyValuePut {
				continue
			}
			var rec Record
			if err := json.Unmarshal(entry.Value(), &rec); err != nil {
				continue
			}
			if rec.Status == StatusFailed {
				failed = append(failed, FailedEntry{
					EntityID:       entry.Key(),
					Reason:         normalizeFailureReason(rec.Reason),
					SourceRevision: rec.SourceRevision,
				})
			}
		}
	}
}
