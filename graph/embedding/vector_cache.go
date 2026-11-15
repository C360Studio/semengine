package embedding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync/atomic"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/graphview"
)

// errNoView says FindSimilar was called before any Run had built a view.
var errNoView = errors.New("embedding: the vector cache has not run")

// VectorCache is an in-memory copy of the generated vectors in EMBEDDING_INDEX, kept by Run
// on a graphview.View of the bucket. FindSimilar answers from the view while it is live, and
// refuses with an error matching graphview.ErrNotReady otherwise.
type VectorCache struct {
	storage *Storage
	// identity is the running embedder's identity, read on each FindSimilar (Record.GeneratedBy).
	identity func() EmbedderIdentity
	running  atomic.Bool
	// view is the view of the latest Run, nil before the first. A view runs once, so each Run
	// builds and stores a new one; the last stays after its Run returns and refuses every read.
	view atomic.Pointer[graphview.View[cachedVector]]
}

// cachedVector is one generated record as the cache keeps it: its vector and the identity it
// was made under.
type cachedVector struct {
	vector []float32
	id     EmbedderIdentity
}

// NewVectorCache returns a cache over the storage's EMBEDDING_INDEX bucket that scores only the
// vectors made under the identity identity returns (Record.GeneratedBy): a query the running
// embedder embeds is scored against vectors in its own space only. FindSimilar calls identity
// once per call, so an embedder that resolves its width after Start is matched by its resolved
// width; while the width is unresolved nothing is scored. A vector another embedder left, as
// after a restart under a different one, is not scored until the running embedder regenerates
// it. It answers nothing until Run has applied the watch's initial snapshot.
func NewVectorCache(storage *Storage, identity func() EmbedderIdentity) *VectorCache {
	return &VectorCache{storage: storage, identity: identity}
}

// Run keeps the cache in step with EMBEDDING_INDEX on the caller's goroutine until ctx ends or
// the watch is lost (the background-work spec's Run; the component that runs it owns ctx,
// ADR-113). Each Run builds a new graphview.View of the bucket and runs it, so it starts from an
// empty cache, which answers once the watch's initial snapshot has been applied.
//
// The view stops as graphview.View.Run says: it watches under a context derived from ctx, ends
// it, and stops the watch through kvwatch.Stop, which reads the update channel until nats.go
// closes it, for at most kvwatch.DrainBudget (TestVectorCacheRunLeavesNoWatchDelivery drives a
// backlogged delivery). nats.go's own goroutine that unsubscribes the watch when its context ends
// is released, not joined: TestVectorCacheRunReportsLostWatch waits for it to go after Run
// returns. Run does not wait for a FindSimilar in progress, which answers from what it read.
//
// Run returns ctx.Err() when ctx ended and an error matching graphview.ErrWatcherLost when the
// watch's update channel closed while ctx was live, joined with kvwatch.Stop's error when the
// watch did not stop cleanly. A watch that cannot open returns its error, or ctx.Err() when ctx
// has ended. Run refuses a nil context, and a second Run while one is running, before building
// a view. After Run returns, Run may be called again.
func (c *VectorCache) Run(ctx context.Context) error {
	if ctx == nil {
		return errs.WrapInvalid(errors.New("nil context"), "VectorCache", "Run", "check the context")
	}
	if !c.running.CompareAndSwap(false, true) {
		return errs.WrapInvalid(errors.New("already running"), "VectorCache", "Run", "run one at a time")
	}
	defer c.running.Store(false)

	view, err := graphview.New(indexSource{c.storage.index}, decodeVector)
	if err != nil {
		return err
	}
	c.view.Store(view)
	return view.Run(ctx)
}

// indexSource is the graphview.WatcherSource over the storage's EMBEDDING_INDEX KVStore. The
// view calls WatchAll with no options.
type indexSource struct{ index *natsclient.KVStore }

func (s indexSource) WatchAll(ctx context.Context, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	return s.index.Watch(ctx, jetstream.AllKeys)
}

// decodeVector is the view's decode: a generated record with a vector is kept as its vector
// and identity, any other record (pending, failed, generated without a vector) is absent, and a
// value that is not a record poisons its key until a valid write or a delete replaces it.
func decodeVector(_ string, value []byte, _ graphview.EntryMeta) (cachedVector, bool, error) {
	var record Record
	if err := json.Unmarshal(value, &record); err != nil {
		return cachedVector{}, false, err
	}
	if record.Status != StatusGenerated || len(record.Vector) == 0 {
		return cachedVector{}, false, nil
	}
	return cachedVector{vector: record.Vector, id: record.identity()}, true, nil
}

// FindSimilar returns up to limit cached entities by cosine similarity to query, best first,
// ties by entity ID (ScoredEntityLess), sorted before the limit applies, and the number of
// records in EMBEDDING_INDEX that could not be decoded, which are not scored. The vectors and the
// count are read at one moment (graphview.View.ListCounted); scoring runs after the read, so a
// write applied meanwhile shows in the next call. The count is every unreadable record, whether
// or not keep accepts its key. excludeID is never scored ("" excludes nothing). keep, when
// non-nil, is applied before scoring and before the limit: only the entity IDs it accepts are
// scored, so a scoped search (ADR-071) returns the best in-scope entities. A limit of 0 or less returns no entities.
// Only vectors made under the running embedder's identity, read once at the call, are scored
// (Record.GeneratedBy); another identity's vectors are left out as pending records are.
//
// It returns an error matching graphview.ErrNotReady, and no entities, before Run has applied
// the watch's initial snapshot, after the watch was lost, and after Run returned.
func (c *VectorCache) FindSimilar(excludeID string, query []float32, keep func(string) bool, limit int) ([]ScoredEntity, int, error) {
	view := c.view.Load()
	if view == nil {
		return nil, 0, fmt.Errorf("%w: %w", graphview.ErrNotReady, errNoView)
	}
	records, unreadable, err := view.ListCounted("")
	if err != nil {
		if errors.Is(err, graphview.ErrViewStopped) { // a stopped view does not say not-ready itself
			return nil, 0, fmt.Errorf("%w: %w", graphview.ErrNotReady, err)
		}
		return nil, 0, err
	}
	if limit <= 0 {
		return nil, unreadable, nil
	}
	id := c.identity()
	results := make([]ScoredEntity, 0, len(records))
	for _, record := range records {
		if record.Key == excludeID || (keep != nil && !keep(record.Key)) {
			continue
		}
		if !sameSpace(record.Value.id, id) {
			continue
		}
		results = append(results, ScoredEntity{EntityID: record.Key, Similarity: CosineSimilarity(query, record.Value.vector)})
	}
	sort.Slice(results, func(i, j int) bool { return ScoredEntityLess(results[i], results[j]) })
	if len(results) > limit {
		results = results[:limit]
	}
	return results, unreadable, nil
}
