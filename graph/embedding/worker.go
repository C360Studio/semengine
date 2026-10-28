package embedding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"
	"unicode/utf8"

	"github.com/nats-io/nats.go/jetstream"
	"golang.org/x/sync/singleflight"

	"github.com/c360studio/semengine/internal/dispatch"
	"github.com/c360studio/semengine/internal/kvwatch"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/storage"
)

// defaultMaxSourceTextLen is the text cap, in runes, when the configured cap is 0 or less.
// It applies to both lanes, inline and offloaded, so identical content embeds identical bytes
// whichever lane carried it.
const defaultMaxSourceTextLen = 8000

// identityBodySeparator joins an offloaded entity's inline identity text (title, .signature,
// .comment) to its fetched body, identity first, in getSourceText.
//
// It is frozen: the separator is part of the bytes embedded, and the dedup key (DedupKey over
// getSourceText's output) is derived over exactly those bytes, so changing it re-keys every
// offloaded entity that carries identity text and re-embeds that lane.
const identityBodySeparator = "\n\n"

// MaxSourceTextLenCeiling is the upper bound on the text cap, in runes. A larger configured cap
// is clamped to it on both lanes, so the offloaded lane's byte budget (utf8.UTFMax*cap+1) can
// never overflow into a negative io.LimitReader bound, which would read an empty body and
// remove the pending record as having no text. 1,000,000 characters is far past any real
// embedding input while bounding the worst-case offloaded read at utf8.UTFMax MB.
const MaxSourceTextLenCeiling = 1_000_000

// workerQueueCapacity is the Worker's keyed pool queue, split across its lanes: as many
// accepted entries as the watch's update channel buffers, which the pin's workers read from
// directly (256 at nats.go v1.54.0, jetstream/kv.go WatchFiltered, not promised).
const workerQueueCapacity = 256

// GeneratedCallback is called when an embedding has been stored. It receives the context Run
// was given, the entity ID and the stored vector, and must return promptly once that context
// has ended.
//
// It runs on the entity's lane of the Worker's keyed pool: callbacks for one entity, this one
// and TerminalCallback, arrive in the order the watch delivered its entries to the Worker,
// each entry's callbacks before the next entry of that entity is processed. A save that was
// refused (a newer source revision is stored, or the record is gone) fires no generated
// callback. Callbacks for different entities may run concurrently and are unordered.
type GeneratedCallback func(ctx context.Context, entityID string, embedding []float32)

// TerminalOutcome classifies how a pending embedding reached its terminal state, so the
// caller can route its current-failed map: OutcomeFailed adds the entity with its bounded
// reason, every other outcome removes it. Every outcome is terminal for readiness: a
// permanently failing or text-less entity still completes.
type TerminalOutcome int

const (
	// OutcomeGenerated means a usable vector was stored.
	OutcomeGenerated TerminalOutcome = iota
	// OutcomeFailed means a durable StatusFailed record was written for this entity, the only
	// outcome that adds to the current-failed map.
	OutcomeFailed
	// OutcomeSkipped means nothing to embed (no text, and the pending record was removed), a
	// corrupt record drained, or a failure that did not persist a durable failed record
	// (superseded, record gone, or the Worker's context cancelled). None is a current failure.
	OutcomeSkipped
	// OutcomeDeleted means the record was removed, or newer work for the entity was stored,
	// while this entry was processed, so its result was dropped. Not a current failure.
	OutcomeDeleted
)

// TerminalCallback is called once for each pending embedding the Worker carries to a terminal
// outcome, with the context Run was given, the entity ID, the ENTITY_STATES source revision
// that produced the record, the outcome and, for OutcomeFailed only, the bounded failure
// reason. It is what completes the hop-1 readiness watermark at the end of the two-hop
// pipeline (ADR-066, ADR-085): an entry that does not reach a terminal outcome (its processing
// panicked, a save's retries ran out, the record could not be marked failed, or its no-text
// removal failed) fires nothing and stays pending, so readiness stays withheld for it.
// sourceRevision 0 means unknown; ^uint64(0) drains an unreadable record whose revision cannot
// be recovered.
//
// Ordering is GeneratedCallback's: per entity, in the order the entries were delivered, each
// after that entry's store write; unordered across entities.
type TerminalCallback func(ctx context.Context, entityID string, sourceRevision uint64, outcome TerminalOutcome, reason string)

// WorkerMetrics receives the Worker's counts, so the package depends on no metrics library.
type WorkerMetrics interface {
	// IncDedupHits counts a stored vector that came from the dedup bucket. It fires only when
	// the save succeeded, so dedup hits stay a subset of stored vectors.
	IncDedupHits()
	// IncDedupSkipped counts an embedding generated without consulting the dedup bucket, with
	// the reason (today only an embedder whose vector width is unresolved, so no dedup key can
	// be derived).
	IncDedupSkipped(reason string)
	// IncTruncated counts one source text truncated at the cap.
	IncTruncated()
	// IncOffloadedIdentityIncluded counts one offloaded entity whose stored vector included its
	// inline identity text ahead of the body. It fires only when the save succeeded.
	IncOffloadedIdentityIncluded()
	// IncOffloadedIdentityAbsent counts one offloaded entity whose stored vector came from its
	// body alone. It fires only when the save succeeded.
	IncOffloadedIdentityAbsent()
	// IncFailed counts one embedding failure.
	IncFailed()
	// IncFailedReason counts one embedding failure by its bounded reason, on the same events
	// as IncFailed, never with the raw error message as the label.
	IncFailedReason(reason string)
	// IncContentResolveError counts a body fetch that failed after its store was resolved (a
	// read error, a deleted bucket), as distinct from an unresolved store.
	IncContentResolveError()
	// IncContentResolved counts a body fetched from a resolved store.
	IncContentResolved()
	// IncContentUnresolved counts a body excluded because no live store is registered for its
	// StorageInstance; the entity's inline identity text is still embedded.
	IncContentUnresolved()
}

// StoreResolver resolves a StorageReference's StorageInstance to its live streaming store
// (ADR-063); *storeregistry.Registry satisfies it. The Worker resolves per fetch and never
// caches or closes the returned store, which the storage component owns.
type StoreResolver interface {
	Streamable(instance string) (storage.StreamableStore, bool)
}

// errContentUnresolved says no store is registered for a body's StorageInstance, as distinct
// from a store that resolved and then failed to open or read. The first excludes only the
// body and keeps the inline identity text; the second is a content failure.
var errContentUnresolved = errors.New("storage instance unresolved")

// Worker turns the pending records of EMBEDDING_INDEX into generated or failed ones (hop 2).
// Run watches the bucket and processes each pending record on a keyed pool, one entity's
// entries at a time.
type Worker struct {
	storage  *Storage
	embedder Embedder

	// running refuses a second Run while one runs.
	running atomic.Bool

	// generateGroup collapses concurrent byte-identical embedder calls to one call, whose
	// vector each caller then saves for its own entity. Process-local; the dedup bucket
	// collapses sequential duplicates.
	generateGroup singleflight.Group

	// storeResolver resolves a StorageRef's StorageInstance per fetch (ADR-063). The resolved
	// store is borrowed, never cached or closed here.
	storeResolver StoreResolver
	// unresolvedStoreWarn logs an unresolved store once; IncContentUnresolved counts each.
	unresolvedStoreWarn sync.Once

	onGenerated GeneratedCallback
	onTerminal  TerminalCallback
	metrics     WorkerMetrics

	workers          int    // lanes of the keyed pool
	maxSourceTextLen int    // text cap in runes; 0 or less means defaultMaxSourceTextLen
	embedderType     string // the Type axis of the dedup key identity

	logger *slog.Logger
}

// dedupSkipReasonIdentityUnresolved labels a dedup skip caused by an embedder whose vector
// width is not resolved yet (Dimensions() == 0), so DedupKey withholds a key.
const dedupSkipReasonIdentityUnresolved = "identity_unresolved"

// NewWorker creates a Worker over storage's EMBEDDING_INDEX bucket, with 5 lanes. A nil logger
// means slog.Default.
func NewWorker(storage *Storage, embedder Embedder, logger *slog.Logger) *Worker {
	if logger == nil {
		logger = slog.Default()
	}
	return &Worker{
		storage:  storage,
		embedder: embedder,
		workers:  5,
		logger:   logger,
	}
}

// WithWorkers sets the number of lanes entities are processed on in parallel, floored at 1:
// a zero or negative count would be a Worker that consumes nothing while every health signal
// stays green.
func (w *Worker) WithWorkers(n int) *Worker {
	w.workers = max(1, n)
	return w
}

// WithStoreResolver sets the shared store resolver (ADR-063), the only path by which an
// offloaded body is fetched: a StorageRef's StorageInstance is resolved per fetch to the exact
// registered instance it names.
func (w *Worker) WithStoreResolver(r StoreResolver) *Worker {
	w.storeResolver = r
	return w
}

// WithMaxSourceTextLen sets the text cap in runes, applied on both lanes; text beyond it is
// truncated at a word boundary. 0 or less means 8,000; a cap above MaxSourceTextLenCeiling is
// clamped to it.
func (w *Worker) WithMaxSourceTextLen(n int) *Worker {
	w.maxSourceTextLen = n
	return w
}

// WithEmbedderType sets the Type axis of the dedup key identity ("bm25", or a remote
// embedder's type). The Worker folds it, with the embedder's live Model and Dimensions and the
// text cap, into the dedup key, so changing the embedder type cannot serve a vector from the
// previous vector space.
func (w *Worker) WithEmbedderType(t string) *Worker {
	w.embedderType = t
	return w
}

// WithOnGenerated sets the callback invoked after an embedding is stored (see
// GeneratedCallback for its ordering).
func (w *Worker) WithOnGenerated(cb GeneratedCallback) *Worker {
	w.onGenerated = cb
	return w
}

// WithOnTerminal sets the callback invoked when a pending embedding reaches a terminal outcome
// (see TerminalCallback).
func (w *Worker) WithOnTerminal(cb TerminalCallback) *Worker {
	w.onTerminal = cb
	return w
}

// WithMetrics sets the metrics receiver.
func (w *Worker) WithMetrics(m WorkerMetrics) *Worker {
	w.metrics = m
	return w
}

// watchEntry is one put the Worker accepted from its watch, with whether it arrived during the
// watch's initial snapshot, fixed when Run read it.
type watchEntry struct {
	entry          jetstream.KeyValueEntry
	duringSnapshot bool
}

// Run processes the pending records of EMBEDDING_INDEX until ctx ends or the watch is lost (the
// background-work spec's Run; the component that runs it owns ctx, ADR-113).
//
// Run opens one watch of the bucket under a context it derives from ctx and reads it on the
// caller's goroutine. It hands each put to a keyed pool (internal/dispatch.KeyedPool) under the
// entity ID, so one entity's entries are processed one at a time in the order the watch
// delivered them, through its store write and both callbacks, while different entities run in
// parallel on the WithWorkers lanes. A full lane makes the read wait. A pending record is
// always processed; a failed record only when it arrives in the watch's initial snapshot, so
// each Run retries failures once, and the Worker's own failed writes never loop.
//
// When ctx ends, or the watch's update channel closes while ctx is live, Run stops in this
// order: it stops reading and cancels the derived context, which ends a read waiting on a full
// lane and releases the goroutine nats.go keeps for the watch's context; it begins the pool's
// drain and joins its lanes (entries already accepted are processed under ctx, ended or not; an
// entry whose read was cut short stays pending in the bucket and returns in the next Run's
// snapshot); and it stops the watch through kvwatch.Stop, which reads the update channel until
// nats.go closes it, for at most kvwatch.DrainBudget (TestWorkerRunLeavesNoWatchDelivery drives
// a backlogged delivery). The join has no bound of its own: a callback that blocks holds Run.
//
// Run returns ctx.Err() when ctx ended and an error saying the watch was lost otherwise, joined
// with kvwatch.Stop's error when the watch did not stop cleanly. A watch that cannot open
// returns its error, or ctx.Err() when ctx has ended. Run refuses a nil context, and a second
// Run while one is running, before opening a watch. After Run returns, Run may be called again.
func (w *Worker) Run(ctx context.Context) error {
	if ctx == nil {
		return errs.WrapInvalid(errors.New("nil context"), "Worker", "Run", "check the context")
	}
	if !w.running.CompareAndSwap(false, true) {
		return errs.WrapInvalid(errors.New("already running"), "Worker", "Run", "run one at a time")
	}
	defer w.running.Store(false)

	pool, err := dispatch.NewKeyedPool(ctx, dispatch.KeyedConfig[watchEntry]{
		Lanes:      w.workers,
		QueueDepth: max(workerQueueCapacity/w.workers, 1),
		Process: func(ctx context.Context, lane int, item watchEntry) {
			w.handleKVEntrySafe(ctx, item.entry, item.duringSnapshot, lane)
		},
		Name:   "embedding_worker",
		Logger: w.logger,
	})
	if err != nil {
		return errs.WrapInvalid(err, "Worker", "Run", "start the keyed pool")
	}

	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	watcher, err := w.storage.index.Watch(watchCtx, jetstream.AllKeys)
	if err != nil {
		cancel()
		_ = pool.Shutdown(ctx) // nothing was accepted, so the lanes exit at once
		pool.Wait()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errs.WrapTransient(err, "Worker", "Run", "watch the EMBEDDING_INDEX bucket")
	}
	w.logger.Info("Embedding worker running", "workers", w.workers)

	w.follow(watchCtx, watcher, pool)
	cancel()
	// Shutdown only begins the drain: with ctx ended it returns ctx.Err() at once. Wait is the
	// join.
	_ = pool.Shutdown(ctx)
	pool.Wait()
	runErr := ctx.Err()
	if runErr == nil {
		runErr = errWatchLost
	}
	if err := kvwatch.Stop(ctx, watcher); err != nil {
		return errors.Join(runErr, fmt.Errorf("embedding: stop the EMBEDDING_INDEX watch: %w", err))
	}
	return runErr
}

// follow reads the watch until ctx (Run's derived context) ends or the update channel closes,
// and submits each put to the pool under its entity ID. Deletes and purges are not work. A
// submit waiting on a full lane returns when ctx ends, and its entry is dropped: it is still
// pending in the bucket.
func (w *Worker) follow(ctx context.Context, watcher jetstream.KeyWatcher, pool *dispatch.KeyedPool[watchEntry]) {
	snapshotDone := false
	for {
		select {
		case <-ctx.Done():
			return
		case entry, ok := <-watcher.Updates():
			if !ok {
				return
			}
			if entry == nil { // the end of the initial snapshot
				snapshotDone = true
				continue
			}
			if entry.Operation() != jetstream.KeyValuePut {
				continue
			}
			item := watchEntry{entry: entry, duringSnapshot: !snapshotDone}
			if err := pool.SubmitBlocking(ctx, entry.Key(), item); err != nil {
				return
			}
		}
	}
}

// handleKVEntrySafe processes one entry with its panic recovered here, and is the one place
// the terminal callback fires.
//
// The callback fires only on a normal return from handleKVEntry, never from a defer inside
// it: Go runs deferred functions while a panic unwinds, so such a defer would complete the
// readiness watermark for a record that is still pending, since nothing on the panic path
// marks it failed or saves a vector. A panicking entry is logged with its stack and not
// retried; its record stays pending and pins the watermark until an operator removes it
// (ADR-084: readiness licenses health, not absence). The lane goes on to its next entry.
func (w *Worker) handleKVEntrySafe(ctx context.Context, entry jetstream.KeyValueEntry, duringSnapshot bool, lane int) {
	defer func() {
		if r := recover(); r != nil {
			w.logger.Error("Embedding worker panic recovered; entry stays pending and its readiness watermark is NOT advanced",
				"lane", lane,
				"entity_id", entry.Key(),
				"panic", r,
				"stack", string(debug.Stack()))
		}
	}()

	entityID, sourceRevision, terminal, outcome, reason := w.handleKVEntry(ctx, entry, duringSnapshot, lane)
	if terminal && w.onTerminal != nil {
		w.onTerminal(ctx, entityID, sourceRevision, outcome, reason)
	}
}

// handleKVEntry processes one watch entry. It completes nothing itself; it reports the
// outcome. terminal is true when the record reached an outcome that completes the readiness
// watermark for entityID at sourceRevision.
func (w *Worker) handleKVEntry(
	ctx context.Context, entry jetstream.KeyValueEntry, duringSnapshot bool, lane int,
) (entityID string, sourceRevision uint64, terminal bool, outcome TerminalOutcome, reason string) {
	var record Record
	if err := json.Unmarshal(entry.Value(), &record); err != nil {
		// A corrupt record cannot yield its source revision, and nothing guarantees it is
		// observed again, so drain the key at the maximum revision so one bad record cannot
		// hold readiness forever. Not a current failure.
		w.logger.Warn("Failed to unmarshal embedding record; draining readiness watermark for key",
			"key", entry.Key(), "error", err)
		return entry.Key(), ^uint64(0), true, OutcomeSkipped, ""
	}

	// Pending records are always processed. A failed record is processed only from the
	// initial snapshot, so a dependency outage recovers on the next Run; a failed record
	// arriving live is the Worker's own SaveFailed write, and processing it would re-embed,
	// fail and re-write in a loop. A generated record is done.
	reprocessFailed := record.Status == StatusFailed && duringSnapshot
	if record.Status != StatusPending && !reprocessFailed {
		return "", 0, false, OutcomeGenerated, ""
	}

	// Every path from here is terminal unless it says otherwise.
	entityID, sourceRevision, terminal, outcome = entry.Key(), record.SourceRevision, true, OutcomeGenerated

	w.logger.Debug("Processing pending embedding", "lane", lane, "entity_id", entityID)

	sourceText, err := w.getSourceText(ctx, &record)
	if err != nil {
		w.logger.Error("Failed to get source text", "entity_id", entityID, "error", err)
		outcome, reason, terminal = w.markFailed(ctx, entityID, fmt.Errorf("text extraction failed: %w", err), failReasonContentError, sourceRevision)
		return
	}

	if sourceText == "" {
		// Nothing to embed: remove the pending record, but only if the key is still at the
		// revision this entry was read at. A key that moved or is gone belongs to newer work.
		removed, err := w.storage.deleteAtRevision(ctx, entityID, entry.Revision())
		switch {
		case err != nil:
			w.logger.Error("Failed to remove the record of an entity with no text; it stays pending",
				"entity_id", entityID, "kv_revision", entry.Revision(), "error", err)
			terminal = false
		case removed:
			w.logger.Debug("No source text; pending record removed", "entity_id", entityID)
			outcome = OutcomeSkipped
		default:
			w.logger.Debug("No source text, but newer work owns the record; nothing removed", "entity_id", entityID)
			outcome = OutcomeDeleted
		}
		return
	}

	// The dedup key is derived here, over the exact bytes embedded (the resolved and
	// truncated text), so the inline and offloaded lanes key identical content alike.
	dedupKey := DedupKey(w.embedderIdentity(), sourceText)

	vector, wasDedupHit, genOutcome, genReason, genTerminal, err := w.getOrGenerateEmbedding(ctx, entityID, sourceText, dedupKey, sourceRevision)
	if err != nil {
		// Already logged and marked. genTerminal is false when the failure could not be
		// recorded, so the record is still pending.
		outcome, reason, terminal = genOutcome, genReason, genTerminal
		return
	}

	terminal, outcome, reason = w.saveAndNotify(ctx, entityID, &record, vector, dedupKey, sourceRevision, wasDedupHit)
	return
}

// embedderIdentity is everything the dedup key depends on besides the text: the embedder Type,
// its live Model and Dimensions, and the configured text cap. Model and Dimensions are read on
// each call, because a remote embedder may resolve its width from its first response
// (Embedder.Dimensions is safe to read concurrently).
func (w *Worker) embedderIdentity() EmbedderIdentity {
	id := EmbedderIdentity{
		Type:       w.embedderType,
		MaxTextLen: w.maxSourceTextLen,
	}
	if w.embedder != nil {
		id.Model = w.embedder.Model()
		id.Dimensions = w.embedder.Dimensions()
	}
	return id
}

// getOrGenerateEmbedding returns a vector from the dedup bucket or generates one.
//
// key is the dedup key over the resolved and truncated text. An empty key means the embedder
// cannot state its vector width yet, so dedup is skipped and the skip is counted.
//
// wasDedupHit reports a vector from the dedup bucket. The caller counts the hit only once the
// save succeeds, so dedup hits never exceed stored vectors.
func (w *Worker) getOrGenerateEmbedding(ctx context.Context, entityID, sourceText, key string, sourceRevision uint64) (vec []float32, wasDedupHit bool, outcome TerminalOutcome, reason string, terminal bool, err error) {
	dedupEnabled := key != ""

	if dedupEnabled {
		dedupRecord, derr := w.storage.GetByContentHash(ctx, key)
		if derr != nil {
			w.logger.Error("Failed to check dedup", "entity_id", entityID, "error", derr)
			outcome, reason, terminal = w.markFailed(ctx, entityID, fmt.Errorf("dedup check failed: %w", derr), failReasonInternal, sourceRevision)
			return nil, false, outcome, reason, terminal, derr
		}
		if dedupRecord != nil && w.dedupRecordUsable(entityID, key, dedupRecord) {
			w.logger.Debug("Deduplicating embedding", "entity_id", entityID, "dedup_key", key)
			return dedupRecord.Vector, true, OutcomeGenerated, "", true, nil
		}
	} else if w.metrics != nil {
		w.metrics.IncDedupSkipped(dedupSkipReasonIdentityUnresolved)
	}

	w.logger.Debug("Generating new embedding", "entity_id", entityID)
	vector, gerr := w.generateShared(ctx, key, sourceText)
	if gerr != nil {
		if errors.Is(gerr, errNoEmbeddingReturned) {
			w.logger.Error("No embedding generated", "entity_id", entityID)
			outcome, reason, terminal = w.markFailed(ctx, entityID, gerr, failReasonEmbedderError, sourceRevision)
			return nil, false, outcome, reason, terminal, gerr
		}
		w.logger.Error("Failed to generate embedding", "entity_id", entityID, "error", gerr)
		outcome, reason, terminal = w.markFailed(ctx, entityID, fmt.Errorf("generation failed: %w", gerr), classifyEmbedErr(gerr), sourceRevision)
		return nil, false, outcome, reason, terminal, gerr
	}

	if dedupEnabled {
		if serr := w.storage.SaveDedup(ctx, key, vector, w.embedder.Model(), w.embedder.Dimensions()); serr != nil {
			// The vector is still saved for the entity; only the reuse of it is lost.
			w.logger.Warn("Failed to save dedup record", "entity_id", entityID, "error", serr)
		}
	}

	return vector, false, OutcomeGenerated, "", true, nil
}

// errNoEmbeddingReturned marks an embedder that returned no vectors, so it is classified as an
// embedder error rather than a transport error.
var errNoEmbeddingReturned = errors.New("no embedding returned")

// generateShared makes one embedder call for concurrent byte-identical content: peers wait for
// the first caller's vector, or its error. key is the dedup key; when it is empty (the
// embedder's width is unresolved) a content-derived in-process key is used instead, so a
// cold-start burst also collapses to one call while the dedup bucket stays untouched.
func (w *Worker) generateShared(ctx context.Context, key, sourceText string) ([]float32, error) {
	sfKey := key
	if sfKey == "" {
		sfKey = InProcessDedupKey(w.embedderIdentity(), sourceText)
	}
	v, err, _ := w.generateGroup.Do(sfKey, func() (any, error) {
		return w.generateOne(ctx, sourceText)
	})
	if err != nil {
		return nil, err
	}
	return v.([]float32), nil
}

// generateOne is the single embedder call; an empty result is errNoEmbeddingReturned.
func (w *Worker) generateOne(ctx context.Context, sourceText string) ([]float32, error) {
	vectors, err := w.embedder.Generate(ctx, []string{sourceText})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, errNoEmbeddingReturned
	}
	return vectors[0], nil
}

// dedupRecordUsable reports whether a dedup record belongs to the vector space this Worker's
// embedder produces. The key already folds in the embedder's identity, so a mismatch means a
// record written under another key layout, logged loudly. A record with no model is unusable.
// A rejected record is regenerated once.
func (w *Worker) dedupRecordUsable(entityID, contentHash string, r *DedupRecord) bool {
	if r.Model == "" {
		w.logger.Warn("Dedup record has no embedder identity; regenerating",
			"entity_id", entityID, "content_hash", contentHash)
		return false
	}
	model, dims := w.embedder.Model(), w.embedder.Dimensions()
	if r.Model == model && r.Dimensions == dims {
		return true
	}
	w.logger.Warn("Dedup record belongs to a different vector space; regenerating",
		"entity_id", entityID, "content_hash", contentHash,
		"record_model", r.Model, "record_dimensions", r.Dimensions,
		"embedder_model", model, "embedder_dimensions", dims)
	return false
}

// saveAndNotify stores the generated vector and fires the generated callback. contentHash is
// the dedup key of the embedded bytes and sourceRevision the revision this generation
// completes; both are stored on the record.
//
// terminal is false when the record is still pending: the save's compare-and-set retries ran
// out (natsclient.ErrKVMaxRetriesExceeded), or a save failure could not be recorded as failed
// either. The record then waits for its next delivery. Every other outcome is terminal.
//
// The dedup hit and the offloaded-identity pair are counted here, after the save succeeded, so
// they count stored vectors, not attempts.
func (w *Worker) saveAndNotify(ctx context.Context, entityID string, record *Record, vector []float32, contentHash string, sourceRevision uint64, wasDedupHit bool) (terminal bool, outcome TerminalOutcome, reason string) {
	dimensions := len(vector)
	model := w.embedder.Model()
	if err := w.storage.SaveGenerated(ctx, entityID, vector, model, dimensions, contentHash, sourceRevision); err != nil {
		switch {
		case errors.Is(err, ErrRecordGone):
			// The entity was removed during generation: nothing to mark failed, and firing
			// onGenerated would put a removed entity's vector back into a cache.
			w.logger.Debug("Embedding record removed during generation; dropping vector",
				"entity_id", entityID)
			return true, OutcomeDeleted, ""
		case errors.Is(err, ErrSupersededRevision):
			// A newer source revision is stored; this vector is the older one, so it must not
			// reach an onGenerated consumer either.
			w.logger.Debug("Embedding superseded by a newer revision; dropping vector",
				"entity_id", entityID)
			return true, OutcomeDeleted, ""
		case errors.Is(err, natsclient.ErrKVMaxRetriesExceeded):
			// Not a generation failure: marking it failed could later flip a good vector at
			// the same revision, and completing it would report pending work as done.
			w.logger.Warn("Embedding save did not converge; leaving record pending for re-drive",
				"entity_id", entityID, "error", err)
			return false, OutcomeGenerated, ""
		}
		w.logger.Error("Failed to save generated embedding", "entity_id", entityID, "error", err)
		markOutcome, markReason, markTerminal := w.markFailed(ctx, entityID, fmt.Errorf("save failed: %w", err), failReasonInternal, sourceRevision)
		return markTerminal, markOutcome, markReason
	}

	w.logger.Debug("Embedding generated successfully", "entity_id", entityID, "dimensions", dimensions)

	if wasDedupHit && w.metrics != nil {
		w.metrics.IncDedupHits()
	}
	if record != nil && record.StorageRef != nil && w.metrics != nil {
		if record.IdentityText != "" {
			w.metrics.IncOffloadedIdentityIncluded()
		} else {
			w.metrics.IncOffloadedIdentityAbsent()
		}
	}

	if w.onGenerated != nil {
		w.onGenerated(ctx, entityID, vector)
	}
	return true, OutcomeGenerated, ""
}

// textCap is the effective text cap in runes for both lanes: the configured cap, 8,000 when it
// is 0 or less, and at most MaxSourceTextLenCeiling.
func (w *Worker) textCap() int {
	limit := w.maxSourceTextLen
	if limit <= 0 {
		limit = defaultMaxSourceTextLen
	}
	return min(limit, MaxSourceTextLenCeiling)
}

// getSourceText produces the exact bytes the Worker embeds and derives the dedup key over.
//
//   - Offloaded (StorageRef set): fetch the body and embed the inline IdentityText ahead of it
//     when present, so the text suffixes take effect on offloaded entities as they do inline.
//     Identity first means the cap trims the body's tail and the identity survives. An
//     unresolved store excludes only the body.
//   - Inline (StorageRef nil): SourceText is the whole text.
//
// Both lanes truncate with truncateAtWord at textCap, so identical content embeds identical
// bytes on either lane. The offloaded-identity pair is counted by saveAndNotify, not here.
func (w *Worker) getSourceText(ctx context.Context, record *Record) (string, error) {
	limit := w.textCap()
	if record.StorageRef == nil {
		text := record.SourceText
		truncated := truncateAtWord(text, limit)
		if len(truncated) < len(text) && w.metrics != nil {
			w.metrics.IncTruncated()
		}
		return truncated, nil
	}

	// fetchTextFromStorage truncates the body and counts that truncation.
	body, bodyTruncated, err := w.fetchTextFromStorage(ctx, record.StorageRef)
	if errors.Is(err, errContentUnresolved) {
		body, bodyTruncated, err = "", false, nil
	}
	if err != nil {
		return "", err
	}

	// The separator joins identity and body only when both are present: an identity with an
	// empty body embeds the identity alone, as an inline entity with that text would.
	text := body
	switch {
	case record.IdentityText != "" && body != "":
		text = record.IdentityText + identityBodySeparator + body
	case record.IdentityText != "":
		text = record.IdentityText
	}

	// The cap applies again to the combined text. Count a truncation once: the body's was
	// counted at the fetch, so count here only a body that fit but was pushed over the cap by
	// the identity.
	truncated := truncateAtWord(text, limit)
	if !bodyTruncated && len(truncated) < len(text) && w.metrics != nil {
		w.metrics.IncTruncated()
	}
	return truncated, nil
}

// truncateAtWord truncates text to at most maxLen runes, preferring the last Unicode
// whitespace boundary in the back half of the cap. It is the one truncation routine for both
// lanes, so identical over-cap content yields identical bytes, and the same dedup key, on
// either. It never splits a multibyte rune, so the result is valid UTF-8. A maxLen of 0 or less
// returns text unchanged.
func truncateAtWord(text string, maxLen int) string {
	if maxLen <= 0 {
		return text
	}

	// The byte offset just past the maxLen-th rune; ranging a string yields each rune's start.
	runeCount := 0
	cut := len(text)
	for i := range text {
		if runeCount == maxLen {
			cut = i
			break
		}
		runeCount++
	}
	if cut == len(text) {
		return text
	}
	truncated := text[:cut]

	// Prefer the last whitespace boundary when it is past the halfway point, in runes.
	if sp := strings.LastIndexFunc(truncated, unicode.IsSpace); sp > 0 {
		if utf8.RuneCountInString(truncated[:sp]) > maxLen/2 {
			return truncated[:sp]
		}
	}
	return truncated
}

// resolveStore returns the registered owner of instance, resolved per fetch; the store is
// borrowed, never cached or closed. Another registered instance is never a candidate.
func (w *Worker) resolveStore(instance string) storage.StreamableStore {
	if w.storeResolver != nil && instance != "" {
		if s, ok := w.storeResolver.Streamable(instance); ok {
			return s
		}
	}
	return nil
}

// fetchTextFromStorage streams the body from its store, reading at most enough bytes for
// textCap runes, and truncates it with truncateAtWord. The store holds raw bytes. bodyTruncated
// reports whether the body itself exceeded the cap, so getSourceText counts a truncation once.
func (w *Worker) fetchTextFromStorage(ctx context.Context, ref *StorageRef) (text string, bodyTruncated bool, err error) {
	store := w.resolveStore(ref.StorageInstance)
	if store == nil {
		if w.metrics != nil {
			w.metrics.IncContentUnresolved()
		}
		w.unresolvedStoreWarn.Do(func() {
			w.logger.Warn("offloaded body excluded from embedding: no live store is registered for the StorageInstance; start or restore that storage component to include the body",
				slog.String("storage_instance", ref.StorageInstance))
		})
		return "", false, fmt.Errorf("%w: %q", errContentUnresolved, ref.StorageInstance)
	}

	reader, err := store.Open(ctx, ref.Key)
	if err != nil {
		if w.metrics != nil {
			w.metrics.IncContentResolveError()
		}
		return "", false, fmt.Errorf("failed to open content from instance %q: %w", ref.StorageInstance, err)
	}
	defer reader.Close()

	// The cap is in runes and a rune is up to utf8.UTFMax bytes, so this budget always holds
	// limit whole runes when the body has them, and the truncation matches the inline lane's.
	limit := w.textCap()
	readCap := int64(limit)*utf8.UTFMax + 1
	data, err := io.ReadAll(io.LimitReader(reader, readCap))
	if err != nil {
		if w.metrics != nil {
			w.metrics.IncContentResolveError()
		}
		return "", false, fmt.Errorf("failed to read content from instance %q: %w", ref.StorageInstance, err)
	}

	if len(data) > 0 && data[0] == '{' {
		w.logger.Debug("stored content appears JSON-wrapped, expected raw text",
			slog.String("key", ref.Key),
			slog.String("hint", "store the raw body text, not a JSON envelope"))
	}

	raw := string(data)
	truncated := truncateAtWord(raw, limit)
	bodyTruncated = len(truncated) < len(raw)
	if bodyTruncated && w.metrics != nil {
		w.metrics.IncTruncated()
	}
	if w.metrics != nil {
		w.metrics.IncContentResolved()
	}
	return truncated, bodyTruncated, nil
}

// classifyEmbedErr maps a generation error to the bounded reason enum (the failReason*
// constants in storage.go). It matches the context deadline and message substrings, because
// a remote embedder's errors often arrive flattened into text; anything else is
// embedder_error.
func classifyEmbedErr(err error) string {
	if err == nil {
		return failReasonEmbedderError
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return failReasonTimeout
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "connection refused"):
		return failReasonConnectionRefused
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline exceeded") ||
		strings.Contains(msg, "context deadline"):
		return failReasonTimeout
	case strings.Contains(msg, "dimension"):
		return failReasonDimensionMismatch
	default:
		return failReasonEmbedderError
	}
}

// markFailed records an embedding failure, cause, under its bounded reason at sourceRevision,
// and reports the outcome for the caller to propagate:
//
//   - OutcomeFailed with the reason only when a durable failed record was written; otherwise
//     OutcomeSkipped with no reason.
//   - terminal is false only when the failed record could not be written (its retries ran out,
//     or the write failed), so the record is still pending and waits for its next delivery.
//
// A failure is the Worker shutting down, not counted and not recorded, only when cause matches
// context.Canceled and ctx, the Worker's, has ended; an error whose text merely mentions
// cancellation is a failure. The failure counters fire on every real failure, a record-gone or
// unrecorded one included, but not on a superseded one or a shutdown.
func (w *Worker) markFailed(ctx context.Context, entityID string, cause error, reason string, sourceRevision uint64) (outcome TerminalOutcome, reportReason string, terminal bool) {
	if errors.Is(cause, context.Canceled) && ctx.Err() != nil {
		w.logger.Debug("Skipping failure metric for context cancellation", "entity_id", entityID)
		return OutcomeSkipped, "", true
	}

	err := w.storage.SaveFailed(ctx, entityID, cause.Error(), reason, sourceRevision)
	switch {
	case err == nil:
		w.incFailure(reason)
		return OutcomeFailed, reason, true
	case errors.Is(err, ErrSupersededRevision):
		w.logger.Debug("Embedding failure superseded by a newer revision; not counting",
			"entity_id", entityID)
		return OutcomeSkipped, "", true
	case errors.Is(err, ErrRecordGone):
		w.logger.Debug("Embedding record removed before failure could be recorded",
			"entity_id", entityID)
		w.incFailure(reason)
		return OutcomeSkipped, "", true
	default:
		w.incFailure(reason)
		w.logger.Error("Failed to mark embedding as failed; leaving record pending for re-drive",
			"entity_id", entityID, "error", err)
		return OutcomeSkipped, "", false
	}
}

// incFailure fires the failure counters with the bounded reason.
func (w *Worker) incFailure(reason string) {
	if w.metrics != nil {
		w.metrics.IncFailed()
		w.metrics.IncFailedReason(reason)
	}
}
