# Tasks: readiness-nil-context

Tests come first; record in `notes/<task>.md` how each fails on the base and which mutant it detects. Review stop
(Codex) after 2.2. The stop's task ends only when `task verify` passes on a clean tree at its commit (#171).

## 1. Design

- [ ] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal, design and spec delta pass pre-owner review and are accepted
  by the owner. Gate: `task spec:check` and `task docs:check`.

## 2. Implementation (D1, D2, D3)

- [ ] 2.1 Failing tests `TestPublishRefusesNilContext` and `TestWaitForFirstRefusesNilContext`. Gate: each nil row
  fails on the base (panic, named in the note); the valid-context rows pass on the base.
- [ ] 2.2 D1's two checks and doc comments; D3's ledger item. Gate: `go test -race ./graph/readiness/
  ./processor/graph-ingest/`; `task mutate:check` detects the `ctx = context.Background()` mutant in each method (WaitForFirst's
  run targets `TestWaitForFirstRefusesNilContext/after_delivery`);
  `task cover:check`; `task verify` passes on a clean tree (review stop).

## 3. Verify, review and close

- [ ] 3.1 Cross-agent review of record on the implementation commit; `reviewed-by:` in the PR body.
- [ ] 3.2 Archive (`openspec archive readiness-nil-context`) as the last content commit. Gate: `task spec:check`.
