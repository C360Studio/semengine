# Proposal: simplify-04a-02-cache-collapse

## Why

1. `internal/cache` carries four separate stateful cache implementations (`simpleCache[V]` in `simple.go`,
   `lruCache[V]` in `lru.go`, `ttlCache[V]` in `ttl.go`, and `hybridCache[V]` in `hybrid.go`) plus `noopCache[V]` in
   `config.go`. Because `hybridCache[V]` already implements optional `maxSize` LRU eviction and optional `ttl` /
   `cleanupInterval` expiration, `simpleCache`, `lruCache`, and `ttlCache` are strict subsets of `hybridCache` that
   duplicate ~700 lines of map, doubly-linked list, mutex, statistics, and Prometheus metrics plumbing.
2. `Statistics` (`internal/cache/stats.go:18-65`) uses `atomic.Int64` for `hits`, `misses`, `sets`, `deletes`, and
   `evictions`, but embeds a `sync.RWMutex` (`mu`) for `currentSize`, `maxSize`, and `startTime`. In addition,
   `ttlCache.Set` (`internal/cache/ttl.go:138-146`) and `ttlCache.Delete` (`:166-172`) release `c.mu.Unlock()` before
   calling `c.stats.UpdateSize(int64(currentSize))`, allowing concurrent mutations to record stale `currentSize` out of
   order while contending on `s.mu.Lock()`.
3. `Cache[V].Clear()` (`internal/cache/cache.go:33`) returns an `error` that every implementation unconditionally fills
   with `nil` and that no caller in SemEngine or at the SemStreams pin (`8b99efe9`) reads.

Issue #220 (PR 4 of the pre-Change 3 simplification wave, Epic #223) collapses the four stateful cache structs into a
single parameterized `memCache[V]` implementation, makes `Statistics` lock-free atomic with `UpdateSize` called under
the cache mutex, and drops the dead `error` return from `Clear()`.

## What Changes

- **Single stateful cache implementation (`internal/cache/cache.go`)**:
  - Collapse `simpleCache[V]`, `lruCache[V]`, `ttlCache[V]`, and `hybridCache[V]` into a single `memCache[V]` struct in
    `internal/cache/cache.go` parameterized by optional `maxSize > 0` (LRU eviction) and optional `ttl > 0` /
    `cleanupInterval > 0` (TTL expiration and background cleanup goroutine).
  - Delete `internal/cache/simple.go`, `internal/cache/lru.go`, `internal/cache/ttl.go`, and `internal/cache/hybrid.go`.
  - Keep `Cache[V]`, `noopCache[V]`, `Config`, and the constructors (`NewFromConfig`, `NewSimple`, `NewLRU`, `NewTTL`,
    `NewNoop`) intact so `processor/graph-ingest`, `component/schema_tags.go`, and future `processor/rule` /
    `storage/objectstore` callers require zero changes.
- **Lock-free atomic `Statistics` and serialized `UpdateSize` (`internal/cache/stats.go`, `internal/cache/cache.go`)**:
  - Replace `Statistics.mu` (`sync.RWMutex`), `currentSize`, `maxSize`, and `startTime` in `internal/cache/stats.go`
    with lock-free atomics (`currentSize` and `maxSize` `atomic.Int64` updated via CAS loop, and `startTime`
    `atomic.Pointer[time.Time]` preserving monotonic elapsed time).
  - Call `c.stats.UpdateSize(int64(len(c.items)))` and `c.metrics.updateSize(len(c.items))` while holding `c.mu.Lock()`
    on every size-changing path (`Set`, `Delete`, `Clear`, `Get` expired eviction, and `cleanup`) so `CurrentSize()`
    never lags behind a concurrent mutation.
- **Drop dead `error` return from `Clear()` (`internal/cache/cache.go`, `internal/cache/config.go`)**:
  - Change `Clear() error` on `Cache[V]`, `memCache[V]`, and `noopCache[V]` to `Clear()`, while keeping `Close() error`
    for `io.Closer`, `background-work` spec (`Close() error`), `processor/graph-ingest/component.go:1161`, and
    `storage/objectstore/store.go:428`.
- **Documentation and ledger sync**:
  - Update `internal/cache/doc.go`, `internal/cache/README.md`, and the `pkg/cache` row in
    `docs/admission-ledger.yaml`.

## Capabilities

No normative capability spec requirements change (`skip_specs: true`). `background-work` continues to govern
`internal/cache` background cleanup and `CoalescingSet.Shutdown(ctx)`.

## Impact

- `internal/cache/cache.go`, `internal/cache/config.go`, `internal/cache/stats.go`, `internal/cache/doc.go`,
  `internal/cache/README.md`, `internal/cache/*_test.go` (and removal of `simple.go`, `lru.go`, `ttl.go`, `hybrid.go`).
- `docs/admission-ledger.yaml`.
- Closes #220, Closes #233; addresses #223.
