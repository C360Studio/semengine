# Tasks: lifecycleguard-tidy

Tests come first where they compile on the base; record in `notes/<task>.md` how each fails on the base and which
mutant it detects. Review stop (Codex) after 2.3. The stop's task ends only when `task verify` passes on a clean tree
at its commit (#171).

## 1. Design

- [x] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal and design pass pre-owner review and are accepted by the
  owner (2026-10-18, PR #308 comment; design at `de6cde7`). Gate: `task spec:check` and `task docs:check`.

## 2. Implementation (D1-D7)

- [x] 2.1 Tests on the base, in `internal/lifecycleguard/guard_test.go`, written against the base's two-argument
  `Stop`. Task 2.3 rewrites both for the one-argument `Stop`, with the cleanup given to `Start`.
  - `TestGuardRefusesNilAndEndedContexts` (D1).
  - `TestGuardRefusesAConcurrentStop` (D3), with its blocking cleanup handed to Stop A, since on the base the cleanup
    given to `Start` runs only in a rollback.

  Gate: `go test -race -count=1 ./internal/lifecycleguard/`, with each failure on the base named in the note:
  - the D1 test's `Start(nil)` row and its nil message check fail;
  - the D1 ended rows and the D3 test pass on the base, because they pin behaviour that already exists; their
    sensitivity is M4 and M3, checked in 2.3.
- [x] 2.2 D1's nil-context helper, called first in `Start` and `Stop`; D5's sentences on the refusals and on what a
  panicking cleanup leaves behind (#309, D7). Gate: `go test -race -count=1 ./internal/lifecycleguard/
  ./internal/harness/lifecycletest/ ./processor/graph-ingest/`.
- [x] 2.3 D2: `Stop(ctx)` runs the cleanup the admitted `Start` recorded. Also:
  - `TestGuardStopRunsTheCleanupStartWasGiven`;
  - the two tests of 2.1 rewritten for `Stop(ctx)`, with the cleanup given to `Start`;
  - the existing guard tests D2 lists;
  - graph-ingest's `Stop` and the log moved into `release`;
  - the two graph-ingest helpers whose cleanup fails once and then runs `release`;
  - D5's cleanup sentences.

  Gates:
  - `go test -race -count=1 ./internal/lifecycleguard/ ./internal/harness/lifecycletest/ ./processor/graph-ingest/`
  - `task test:integration -- ./processor/graph-ingest/`
  - `task test:repeat -- ./internal/lifecycleguard/ ./processor/graph-ingest/`
  - `task mutate:check` gives `verdict: detection` for M1 to M4, each run with `-pkg ./internal/lifecycleguard -file
    internal/lifecycleguard/guard.go`, with `-test` and `-expect-text` taken from design D4's table, and with its mutant
    copy kept outside the repository
  - `task verify` passes on a clean tree (review stop).

## 3. Verify, review and close

- [x] 3.1 Cross-agent review of record on the implementation commit; `reviewed-by:` in the PR body. The body says
  `Addresses #196` and declares no close. Check with `gh pr view 308 --json closingIssuesReferences --jq
  '[.closingIssuesReferences[].number]'`, which must print `[]`.
- [x] 3.2 Archive (`openspec archive lifecycleguard-tidy`) as the last content commit. Gate: `task spec:check`.
