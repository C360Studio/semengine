# Design: simplify-04a-02-cache-collapse

## Context

Issue #220 is part of Epic #223 (the pre-Change 3 simplification wave). Before claiming work, the owner confirmed two
scoping decisions under the Clarify gate:

1. **Scope**: merge the four stateful cache structs (`simpleCache`, `lruCache`, `ttlCache`, `hybridCache`) into one,
   make `Statistics` lock-free atomic with `UpdateSize` called under the cache lock, drop `error` from `Clear()`, and
   keep `Close() error` for `background-work` spec and `processor/graph-ingest` compatibility.
2. **Stop boundary**: run autonomously through worktree claim, OpenSpec change, implementation, local `task verify`,
   push, and posting the cross-agent `Review request` comment.

### Overlap inventory

`gh pr list --state open --json number,title,headRefName` shows one open PR:

- **#230 (`claude/test-infra-smaller-fixture`)**: touches `internal/harness/natsfixture` and
  `openspec/changes/test-infra-audit/`. Zero file or capability overlap with `internal/cache`,
  `docs/admission-ledger.yaml` (`pkg/cache` row), or `docs/testing.md`. Either PR can merge first.

### Caller inventory (SemEngine and SemStreams pin `8b99efe9`)

- **In SemEngine (`main` at `8b80014`)**:
  - `processor/graph-ingest/component.go:433, 1161, 1194`: holds `cache.Cache[graph.EntityState]`, constructs via
    `cache.NewFromConfig` (`StrategyHybrid`, `WithMetrics`), and calls `Get`, `Set`, `Delete`, and `Close() error`.
  - `processor/graph-ingest/query_prefix_regression_test.go:29`: calls `cache.NewSimple[graph.EntityState]()`.
  - `component/schema_tags.go:831` and `component/schema_tags_nested_test.go:55`: reflect over `cache.Config` tags
    (`enabled`, `strategy` with `enum:simple|lru|ttl|hybrid`, `max_size`, `ttl`, `cleanup_interval`).
- **In unported admitted packages at the SemStreams pin (`8b99efe9`)**:
  - `processor/graph-embedding/component.go:263, 749`: uses `cache.CoalescingSet` only.
  - `processor/rule/config.go:27, 239`, `processor.go:69, 281, 893, 1341`, `kv_writer.go:33, 41, 45`: uses
    `cache.Config` (`StrategyTTL`), `NewFromConfig`, `NewSimple`, `NewNoop`, `Get`, `Set`, and `Close() error`.
  - `processor/rule/expression/regex_cache.go:15, 20, 35, 52, 126, 131, 136`: uses `NewLRU`, `Get`, `Set`, `Clear()`
    (ignoring return value at `:126`), `Size()`, and `Stats()`.
  - `storage/objectstore/config.go:38, 102`, `store.go:49, 154, 206, 306, 364, 428`: uses `cache.Config`
    (`StrategyLRU`), `NewFromConfig`, `Get`, `Set`, `Delete`, and `Close() error`.

## Decisions

### D1 Collapse `simpleCache`, `lruCache`, `ttlCache`, and `hybridCache` into `memCache[V]`

- **Mechanism**: Replace `simpleCache[V]` (`simple.go`), `lruCache[V]` (`lru.go`), `ttlCache[V]` (`ttl.go`), and
  `hybridCache[V]` (`hybrid.go`) with a single `memCache[V]` in `internal/cache/cache.go`:
  - `maxSize int`: when `> 0` (`StrategyLRU` and `StrategyHybrid`), `Set` evicts the least-recently-used tail element
    when `len(c.items) > c.maxSize`, and `Get` / `Set` move accessed elements to the front of `c.order`. When `<= 0`
    (`StrategySimple` and `StrategyTTL`), `c.order` is nil and entries are stored directly in `c.items` without list
    allocation or LRU reordering (`Keys()` returns map keys when `c.order == nil`, and LRU-ordered keys from
    most-recently-used to least-recently-used when `c.order != nil`, matching the existing contracts of `simpleCache` /
    `ttlCache` and `lruCache` / `hybridCache`).
  - `ttl time.Duration` and `cleanupInterval time.Duration`: when `ttl > 0` (`StrategyTTL` and `StrategyHybrid`),
    `Set` stamps `expiresAt = time.Now().Add(c.ttl)`, `Get`lazily evicts expired entries, and `newMemCache` starts one
    background `cleanup(ctx)` goroutine joined by `Close()`. When `ttl <= 0` (`StrategySimple` and `StrategyLRU`),
    entries never expire, no background goroutine starts, and `Close()` is a no-op returning `nil`.
  - All constructors (`NewSimple`, `NewLRU`, `NewTTL`, `newHybrid`, and `NewFromConfig`) validate their arguments as
    before and return `(Cache[V], error)` backed by `*memCache[V]` (or `*noopCache[V]` when `!config.Enabled`),
    returning a `nil` interface on error.
- **Proving tests**: `internal/cache/cache_test.go`, `background_test.go`, `metrics_test.go`, `config_test.go`, and
  `processor/graph-ingest` unit and integration tests.

### D2 Lock-free atomic `Statistics` and serialized `UpdateSize` under `c.mu`

- **Pin defect**: `Statistics` (`internal/cache/stats.go:18-65`) protected `currentSize`, `maxSize`, and `startTime`
  with `mu sync.RWMutex`, while `ttlCache.Set` (`ttl.go:138-146`) and `ttlCache.Delete` (`ttl.go:166-172`) released
  `c.mu.Unlock()` before calling `c.stats.UpdateSize(int64(currentSize))`. Two concurrent callers that changed the cache
  size could reach `UpdateSize` in the opposite order from their map mutations, leaving `stats.CurrentSize()` holding
  the older size after both operations settled.
- **Change**:
  - Replace `currentSize` and `maxSize` on `Statistics` with `atomic.Int64` fields and `startTime` with
    `atomic.Pointer[time.Time]` (preserving Go's monotonic clock reading across `time.Since(*start)`), removing
    `mu sync.RWMutex` completely. Document on `Reset()` that callers must ensure cache mutations (`UpdateSize`) are
    quiescent when resetting statistics.
  - In `UpdateSize(size int64)`, store `s.currentSize.Store(size)` and advance `s.maxSize` with a compare-and-swap loop
    whenever `size > s.maxSize.Load()`.
  - In `memCache[V]`, call `c.stats.UpdateSize(int64(len(c.items)))` (and `c.metrics.updateSize(len(c.items))`) while
    still holding `c.mu.Lock()` on every size-changing path (`Set`, `Delete`, `Clear`, `Get` expired eviction, and
    `cleanup`), and snapshot `entry.value` and `entry.expiresAt` under `c.mu.RLock()` on non-LRU `Get` reads (#233).
- **Proving test**: `TestConcurrentSetDeleteStatsCurrentSizeMatchesSize`,
  `TestStatisticsConcurrentUpdateSizeTracksMax`, and `TestConcurrentSameKeyGetSet` in `internal/cache/cache_test.go`.

### D3 Drop dead `error` return from `Clear()`

- **Change**: `Cache[V].Clear()`, `(*memCache[V]).Clear()`, and `(*noopCache[V]).Clear()` return no value (`Clear()`),
  while `Close() error` remains unchanged.

### D4 Generated checks

Existing `synctest` lifecycle checks (`background_test.go`), `rapid` property checks (`coalescing_set_prop_test.go`),
and `FuzzConfigUnmarshalJSON` (`config_fuzz_test.go`) remain in place, supplemented by a deterministic concurrency test
for `Statistics.UpdateSize` and `memCache` size accounting (`docs/testing.md`, "Decide whether generated checks are
needed").
