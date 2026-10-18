# Tasks: lifecycleguard-tidy

Tests come first where they compile on the base; record in `notes/<task>.md` how each fails on the base and which
mutant it detects. Review stop (Codex) after 2.3. The stop's task ends only when `task verify` passes on a clean tree
at its commit (#171).

## 1. Design

- [ ] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal and design pass pre-owner review and are accepted by the
  owner. Gate: `task spec:check` and `task docs:check`.

## 2. Implementation (D1-D5)

- [ ] 2.1 Tests `TestGuardRefusesNilAndEndedContextsAlike` (D1) and `TestGuardRefusesAConcurrentStop` (D3) in
  `internal/lifecycleguard/guard_test.go`. Gate: `go test -race -count=1 ./internal/lifecycleguard/`. On the base the
  D1 test fails at its Start-nil row and its two ended-context Stop rows, named in the note; the D3 test passes, since
  it pins behaviour that already exists, and its sensitivity is M3 in 2.3.
- [ ] 2.2 D1's refusal helper, called first in `Start` and `Stop`; D5's refusal sentences. Gate: `go test -race
  -count=1 ./internal/lifecycleguard/ ./internal/harness/lifecycletest/ ./processor/graph-ingest/`.
- [ ] 2.3 D2: `Stop(ctx)` runs the cleanup the admitted `Start` recorded. Also:
  - `TestGuardStopRunsTheCleanupStartWasGiven`, and the updated guard tests;
  - graph-ingest's `Stop` and the log moved into `release`;
  - the two graph-ingest helpers whose cleanup fails once and then runs `release`;
  - D5's cleanup sentences.

  Gates:
  - `go test -race -count=1 ./internal/lifecycleguard/ ./internal/harness/lifecycletest/ ./processor/graph-ingest/`
  - `task test:integration -- ./processor/graph-ingest/`
  - `task test:repeat -- ./internal/lifecycleguard/ ./processor/graph-ingest/`
  - `task mutate:check` gives `verdict: detection` for M1, M2 and M3 (design D3), each with `-pkg
    ./internal/lifecycleguard -file internal/lifecycleguard/guard.go` and its mutant copy kept outside the repository
  - `task verify` passes on a clean tree (review stop).

## 3. Verify, review and close

- [ ] 3.1 Cross-agent review of record on the implementation commit; `reviewed-by:` in the PR body. The body says
  `Addresses #196` and declares no close. Check with `gh pr view 308 --json closingIssuesReferences --jq
  '[.closingIssuesReferences[].number]'`, which must print `[]`.
- [ ] 3.2 Archive (`openspec archive lifecycleguard-tidy`) as the last content commit. Gate: `task spec:check`.
