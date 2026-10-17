# Tasks: simplify-04a-13-natsclient-storage

Each task names its outcome and the gate that proves it. Tests come first. Record each one failing on the base commit
in `notes/<task>.md`; a compile failure on a missing symbol counts as failing.

## 1. Design

- [x] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal, design and spec delta pass pre-owner review and are accepted
  by the owner with rulings on owner questions 1–5 (#223 comment 6226158511). Gate: `task spec:check` and `task docs:check`.
- [x] 1.2 Merge `origin/main` into the claim branch (#278 merged as `838c1fa`; merge `fe52896`). Gate: `task verify`
  on the merge (exit 0 at `e7074f6`).

## 2. Failing tests

- [x] 2.1 The storage tests and `reviewer_storage_snapshot_probe_test.go` are moved with `git mv` to
  `natsclient/storagereport/`, edited only for the package clause, natsclient qualifiers and the D1 names; the
  integration test's shared fixture file is added; the storage rows of `nil_context_siblings_test.go` move (D2).
  Gate: fails to compile on the base commit (no package).
- [x] 2.2 `graph/kvcatalog` tests import `storagereport` (`kvcatalog_attribution_test.go`,
  `kvcatalog_storage_report_test.go`, `storage_report_integration_test.go`) (D2). Gate: fails to compile on base.
- [x] 2.3 `TestReportConsumer_AccountDeleteReachesObserver` and `TestReportConsumer_ResyncRetractionReachesObserver`;
  `recordingObserver` gains `ForgetAccount`; `TestReportConsumer_AccountRetractionIsLogged` deleted (D3). Gate: fails
  on the base commit.

## 3. Implementation

- [x] 3.1 `git mv` the seven files to `natsclient/storagereport/`; package clause, qualifiers, `doc.go` with the
  package comment, private `nilContextErrorOf`, the nine renames (D1, D2). Gate: 2.1, 2.2; `task lint`.
- [x] 3.2 `AccountStreamLister(client)` replaces the method and resolves `JetStream()` on each call; natsclient keeps
  no storage code (D2). Gate: 2.1 (the integration tests call it); `go build ./...`.
- [x] 3.3 `Observer.ForgetAccount`; `removeAccount` and `replaceWith` call it with the held row; the Warn log goes
  (D3, #85). Gate: 2.3.
- [x] 3.4 Coverage target `natsclient/storagereport merged` in `scripts/cover-check.sh` and `d11Targets` (D2). Gate:
  `task cover:check`; `TestCoverCheckSensitivity`.
- [x] 3.5 Unexport the 22 names of the Exports table's last row; in-package tests follow; kept doc comments that
  name them state the value instead (`storage_report_consumer.go:115`, `storage_growth.go:81`, and the custom-lister
  note at `storage_account.go:69`); `kvcatalog.go:91` and `storage_report.go:11` comments,
  `kvcatalog_storage_report_test.go:51` comment (D2, Exports). Gate: `go build
  ./...`; `task lint`; `task test:unit`.

## 4. Documentation

- [x] 4.1 Ledger natsclient row: destination note, contract D1–D3, #85 sentence removed (D4). Gate: `task ledger:check`.
- [x] 4.2 `docs/repository-map.md` row and target count; `graph/kvcatalog/kvcatalog.go:88,:91` comments (D4). Gate:
  `task docs:check`.
- [x] 4.3 Comment on #281 with the new path of `storage_pressure.go:82` (D4). Gate: the comment is read back
  (#281 comment 6232264355).

## 5. Verify, review and close

- [ ] 5.1 `task verify` passes on a clean tree at the implementation head.
- [ ] 5.2 Cross-agent review of record on the implementation commit; `reviewed-by:` in the PR body.
- [ ] 5.3 Archive (`openspec archive simplify-04a-13-natsclient-storage`) as the last content commit; write the
  `storage-report` Purpose and drop "how the storage report view changes" from the `transport-client` Purpose. Gate:
  `task spec:check`.
