# Tasks: simplify-04a-14-natsclient-request

Each task names its outcome and the gate that proves it. Tests come first. Record each one failing on the base commit
in `notes/<task>.md`; a compile failure on a missing symbol counts as failing.

## 1. Design

- [x] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal, design and spec delta pass pre-owner review and are accepted
  by the owner with rulings on owner questions 1–2 (#223 comment 6232621030). Gate: `task spec:check` and `task docs:check`.
- [x] 1.2 Merge `origin/main` into the claim branch if it moved (it had not, at `a163c61`). Gate: `task verify`
  on the merge.

## 2. Failing tests

- [x] 2.1 `TestRetryRequestNegativeCountMakesOneAttempt` and `TestRetryRequestRefusedConfigNeverDispatches`
  replace `TestNegativeMaxRetriesNeverDispatches` and `TestReviewerNegativeRequestRetryRefuses` (D2). Gate: fail to
  compile on base.
- [x] 2.2 `TestIntegration_RequestWithRetryClassified_ErrorReplyIsNotRetried` and
  `TestIntegration_RequestWithHeaders_ErrorReplyKeepsHeaders` (D2, D3). Gate: pass on base (the first written with
  `MaxRetries: 2`, moved in 2.3); for the first,
  `task mutate:check` with `ClassifyReply` moved inside the attempt reports a detection.
- [x] 2.3 Existing tests move to the new API (Design, Tests: kept, edited, deleted). Gate: fail to compile on base
  (`retry.Config` argument).

## 3. Implementation

- [x] 3.1 Remove `Client.Request` and `Client.RequestWithRetry` (D1). Gate: `go build ./...`; `task test:unit`.
- [x] 3.2 Remove `RetryConfig`, `DefaultRetryConfig` and `requestMsgWithRetry`; `RequestWithRetryClassified` takes
  `retry.Config` and runs `retry.Do` (D2). Gate: 2.1, 2.2, 2.3; `task test:integration`.
- [x] 3.3 `RequestWithHeaders` doc comment; `errors.go` package comment and `doc.go` footgun text; comments at
  `request.go:69-79`, `errors.go:212`, `:241-249` (D3, D4). Gate: `task lint`.

## 4. Documentation

- [x] 4.1 transport-client delta matches the code; ADR-112 lists and amendment line; `natsclient/README.md:10`, `:146`;
  `doc.go:394` (D4). Gate: `task spec:check`, `task docs:check`.
- [x] 4.2 Ledger natsclient row and the two contract sentences (D4). Gate: `task ledger:check`; `task verify`'s
  adapter check.
- [x] 4.3 Comment on #139 with the new line of the handler timeout (was `request.go:396`). Gate: the comment is read back
  (#139 comment 6232868784).

## 5. Verify, review and close

- [x] 5.1 `task verify` passes on a clean tree at the implementation head (exit 0 at `99c16d5`) (includes `task cover:check`, natsclient
  merged).
- [x] 5.2 Cross-agent review of record on the implementation commit; `reviewed-by:` in the PR body (Codex APPROVE
  at `ac7fd60`; owner directed docs, archive and merge on 2026-10-17).
- [x] 5.3 Archive (`openspec archive simplify-04a-14-natsclient-request`) as the last content commit. Gate:
  `task spec:check`.
