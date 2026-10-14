# Tasks: simplify-04a-02-cache-collapse

Each task names the outcome and the gate that proves it.

## 1. Design

- [x] 1.1 Clarify scope, caller inventory, and overlap boundaries with the owner, and scaffold `.openspec.yaml`,
  `proposal.md`, `design.md`, and `tasks.md`. Gate: `task spec:check` and `task docs:check`.

## 2. Implementation and tests

- [x] 2.1 Replace `Statistics.mu` (`sync.RWMutex`) in `internal/cache/stats.go` with lock-free atomic fields
  (`currentSize` and `maxSize` `atomic.Int64` via CAS loop, `startTime` `atomic.Pointer[time.Time]`), collapse
  `simpleCache`, `lruCache`, `ttlCache`, and `hybridCache` into `memCache[V]` in `internal/cache/cache.go` (deleting
  `simple.go`, `lru.go`, `ttl.go`, and `hybrid.go`), update `UpdateSize` under `c.mu.Lock()`, drop the `error` return
  from `Clear()`, and update `internal/cache/*_test.go` with concurrent `Statistics` / `CurrentSize` and same-key
  `Get`/`Set` regression tests. Gate: `task test:unit` and `task test:repeat`.
- [x] 2.2 Update `internal/cache/doc.go`, `internal/cache/README.md`, and the `pkg/cache` row in
  `docs/admission-ledger.yaml` (and verify `docs/testing.md` remains truthful). Gate: `task docs:check`,
  `task ledger:check`, and `task verify`.

## 3. Review and close

- [ ] 3.1 Cross-agent review of record on the implementation commit; record `reviewed-by:` in the PR body once the
  reviewing agent's newest review record approves.
- [ ] 3.2 Archive the change (`openspec archive simplify-04a-02-cache-collapse`) as the last content commit. Gate:
  `task spec:check`.
