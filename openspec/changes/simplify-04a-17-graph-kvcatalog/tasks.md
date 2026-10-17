# Tasks: simplify-04a-17-graph-kvcatalog

Each task names its outcome and the gate that proves it. Tests come first. Record each failing on the base commit in
`notes/<task>.md`. Tests in 2.1 are written against the base API and fail by assertion; only 2.2 counts a compile
failure as failing. Review stops (Codex) after 2.3 and 3.2. Each stop's task ends only when `task verify` passes on a
clean tree at its commit (#171).

## 1. Design

- [ ] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal and design pass pre-owner review and are accepted by the
  owner, with owner questions 1 and 2 answered. Gate: `task spec:check` and `task docs:check`.

## 2. Table, write check and errors (batch 1)

- [ ] 2.1 Failing tests against the base API (D4):
  - `TestOpenCatalogReader_InvalidNamePreservesClassification` asserts component `"kvcatalog"`, with an
    `EnsureCatalogBucket` row;
  - `TestIntegration_EnsureCatalogRetentionClean_RefusesNilContext` (outer class invalid, the error names the nil
    context).

  Gate: each fails by assertion on the base commit.
- [ ] 2.2 `TestCheckGenericWrite` (D3), and the six tests that assert `IsFrameworkOwnedBucket` moved to
  `CheckGenericWrite`; with owner question 1 answered "keep", also `TestKVCatalogReturnsACopy` (D1). Gate: fails to
  compile on the base commit.
- [ ] 2.3 Implement D1–D4:
  - the package-level table; `KVCatalog` removed or returning a copy (owner question 1); `natsclient/kvspec.go:120`;
  - `nameFamilies` removed, with `TestConfigBucketFamilyResolvesEveryMember` passing with its cases unchanged;
  - `CheckGenericWrite`, with the pair removed;
  - one resolver, component `"kvcatalog"`, and the nil-context refusal.

  Gate: 2.1 and 2.2 pass; `go test -race ./graph/kvcatalog/...`; `task mutate:check` with the lookup in
  `CheckGenericWrite` replaced by `return nil` is detected by `TestCheckGenericWrite`; `task verify` passes on a clean
  tree (review stop).

## 3. Tombstone and text (batch 2)

- [ ] 3.1 D5: `natsclient.IsKVTombstone` and `natsclient/kv_tombstone_test.go` (the moved table test); the callers in
  `graph/readiness/watcher.go`, `processor/graph-ingest/component.go` and `pkg/lifecycle/manager_query.go`;
  `graph/kvcatalog/tombstone.go` and its test removed. Hold: #289 (8e) merged and this branch merged with
  `origin/main`. Gate: `go test -race ./natsclient/ ./graph/readiness/... ./pkg/lifecycle/...
  ./processor/graph-ingest/...`; `git grep -n -e KeyValueDelete -e KeyValuePurge -- '*.go' ':!*_test.go'` lists only
  `natsclient/kv.go`.
- [ ] 3.2 D6: the package comment, citations and labels; `graph/README.md:129`; the `graph` ledger row; the
  `graph/readiness` row's dependency list when `go list -f '{{.Imports}}' ./graph/readiness` no longer lists
  `graph/kvcatalog`. Gate: `task verify` passes on a clean tree, which runs `ledger:check` and `docs:check` (review
  stop).

## 4. Verify, review and close

- [ ] 4.1 `task verify` passes on a clean tree at the implementation head.
- [ ] 4.2 Cross-agent review of record on the implementation commit; `reviewed-by:` in the PR body.
- [ ] 4.3 Any follow-up issues filed; the change is archived (`openspec archive simplify-04a-17-graph-kvcatalog`) as
  the last content commit. Gate: `task spec:check`.
