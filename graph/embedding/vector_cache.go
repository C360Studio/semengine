package embedding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/internal/kvwatch"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

// errWatchLost says the EMBEDDING_INDEX watch's update channel closed while Run's context was
// still live, as nats.go closes it when the bucket is deleted.
var errWatchLost = errors.New("embedding: the EMBEDDING_INDEX watch was lost")

// VectorCache is an in-memory copy of the generated vectors in EMBEDDING_INDEX, kept by Run
// from a watch of the bucket. FindSimilar answers from it only while it is authoritative;
// otherwise the caller reads the bucket.
type VectorCache struct {
	storage *Storage
	running atomic.Bool

	// mu guards vectors and authoritative together, so withdrawing the cache's authority
	// waits for a FindSimilar already reading it, and no FindSimilar starts reading after.
	mu            sync.RWMutex
	vectors       map[string][]float32
	authoritative bool
}

// NewVectorCache returns a cache over the storage's EMBEDDING_INDEX bucket. It answers nothing
// until Run has applied the watch's initial snapshot.
func NewVectorCache(storage *Storage) *VectorCache {
	return &VectorCache{storage: storage}
}

// Run keeps the cache in step with EMBEDDING_INDEX on the caller's goroutine until ctx ends or
// the watch is lost (the background-work spec's Run; the component that runs it owns ctx,
// ADR-113). Each Run starts from an empty cache, which answers once the watch's initial snapshot
// has been applied.
//
// Run watches under a context it derives from ctx. When ctx ends, or the watch's update channel
// closes while ctx is live, Run stops in this order: it withdraws the cache's authority, so
// FindSimilar answers false from then on; it cancels the derived context, so nothing nats.go
// runs under the watch's context outlives Run's wait; and it stops the watch through
// kvwatch.Stop, which reads the update channel until nats.go closes it, for at most
// kvwatch.DrainBudget (TestVectorCacheRunLeavesNoWatchDelivery drives a backlogged delivery).
// nats.go's own goroutine that unsubscribes the watch when its context ends is released, not
// joined: TestVectorCacheRunReportsLostWatch waits for it to go after Run returns.
//
// Run returns ctx.Err() when ctx ended and an error saying the watch was lost otherwise, joined
// with kvwatch.Stop's error when the watch did not stop cleanly. A watch that cannot open
// returns its error, or ctx.Err() when ctx has ended. Run refuses a nil context, and a second
// Run while one is running, before opening a watch. After Run returns, Run may be called again.
func (c *VectorCache) Run(ctx context.Context) error {
	if ctx == nil {
		return errs.WrapInvalid(errors.New("nil context"), "VectorCache", "Run", "check the context")
	}
	if !c.running.CompareAndSwap(false, true) {
		return errs.WrapInvalid(errors.New("already running"), "VectorCache", "Run", "run one at a time")
	}
	defer c.running.Store(false)

	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	watcher, err := c.storage.index.Watch(watchCtx, jetstream.AllKeys)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errs.WrapTransient(err, "VectorCache", "Run", "watch the EMBEDDING_INDEX bucket")
	}

	c.follow(ctx, watcher)
	c.withdraw()
	cancel()
	runErr := ctx.Err()
	if runErr == nil {
		runErr = errWatchLost
	}
	if err := kvwatch.Stop(ctx, watcher); err != nil {
		return errors.Join(runErr, fmt.Errorf("embedding: stop the EMBEDDING_INDEX watch: %w", err))
	}
	return runErr
}

// follow applies the watch's entries to an empty cache until ctx ends or the update channel
// closes. The cache is authoritative from the end of the initial snapshot until a record fails
// to decode: a record that will not decode may have replaced a vector the cache still holds, so
// the cache stays withdrawn for the rest of this Run and callers read the bucket.
func (c *VectorCache) follow(ctx context.Context, watcher jetstream.KeyWatcher) {
	c.mu.Lock()
	c.vectors = make(map[string][]float32)
	c.mu.Unlock()
	snapshotApplied, undecodable := false, false
	for {
		select {
		case <-ctx.Done():
			return
		case entry, ok := <-watcher.Updates():
			if !ok {
				return
			}
			c.mu.Lock()
			switch {
			case entry == nil: // the end of the initial snapshot
				snapshotApplied = true
			case natsclient.IsKVTombstone(entry.Operation()):
				delete(c.vectors, entry.Key())
			default:
				var record Record
				if err := json.Unmarshal(entry.Value(), &record); err != nil {
					undecodable = true
				} else if record.Status == StatusGenerated && len(record.Vector) > 0 {
					c.vectors[entry.Key()] = record.Vector
				} else {
					delete(c.vectors, entry.Key())
				}
			}
			c.authoritative = snapshotApplied && !undecodable
			c.mu.Unlock()
		}
	}
}

// withdraw ends the cache's authority and drops its vectors.
func (c *VectorCache) withdraw() {
	c.mu.Lock()
	c.authoritative = false
	c.vectors = nil
	c.mu.Unlock()
}

// FindSimilar returns up to limit cached entities by cosine similarity to query, best first,
// ties by entity ID (ScoredEntityLess), sorted before the limit applies. excludeID is never
// scored ("" excludes nothing). keep, when non-nil, is applied before scoring: only the entity
// IDs it accepts are scored, so a scoped search (ADR-071) filters as the bucket scan does. keep
// runs while the cache is read-locked, so it must not call the cache. A limit of 0 or less
// returns no entities.
//
// The second result is false, with no entities, when the cache is not authoritative: before
// Run has applied the watch's initial snapshot, after a record failed to decode, after the
// watch was lost, and after Run returned. The caller then reads EMBEDDING_INDEX.
func (c *VectorCache) FindSimilar(excludeID string, query []float32, keep func(string) bool, limit int) ([]ScoredEntity, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.authoritative {
		return nil, false
	}
	if limit <= 0 {
		return nil, true
	}
	results := make([]ScoredEntity, 0, len(c.vectors))
	for entityID, vector := range c.vectors {
		if entityID == excludeID || (keep != nil && !keep(entityID)) {
			continue
		}
		results = append(results, ScoredEntity{EntityID: entityID, Similarity: CosineSimilarity(query, vector)})
	}
	sort.Slice(results, func(i, j int) bool { return ScoredEntityLess(results[i], results[j]) })
	if len(results) > limit {
		results = results[:limit]
	}
	return results, true
}
